package rlservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
	sk8s "github.com/bluequbit/faas/control-plane/k8s"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Reconciler struct {
	state      *state.StateManager
	client     sk8s.Client
	logger     *logrus.Logger
	interval   time.Duration
	promotions PromotionController
}

type PromotionController interface {
	MarkGood(context.Context, string, string) error
	Rollback(context.Context, string, string) (string, error)
}

func NewReconciler(manager *state.StateManager, client sk8s.Client, logger *logrus.Logger, interval time.Duration) *Reconciler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Reconciler{state: manager, client: client, logger: logger, interval: interval}
}

func (r *Reconciler) SetPromotionController(controller PromotionController) {
	r.promotions = controller
}

func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		if err := r.ReconcileAll(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.Errorf("slime reconciler: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Reconciler) ReconcileAll(ctx context.Context) error {
	runs, err := r.state.ListRLRuns()
	if err != nil {
		return err
	}
	for i := range runs {
		if runs[i].Backend != "slime" || terminal(runs[i].Status) {
			continue
		}
		if err := r.Reconcile(ctx, &runs[i]); err != nil {
			runs[i].FailureReason = err.Error()
			runs[i].UpdatedAt = time.Now()
			_ = r.state.SaveRLRun(&runs[i])
		}
	}
	return nil
}

func terminal(status string) bool {
	return status == "completed" || status == "cancelled" || status == "failed"
}

func (r *Reconciler) Reconcile(ctx context.Context, run *state.RLRun) error {
	var snapshot contracts.RunSnapshot
	if err := json.Unmarshal([]byte(run.SnapshotJSON), &snapshot); err != nil {
		return fmt.Errorf("decode immutable run snapshot: %w", err)
	}
	if snapshot.SHA256 != run.SnapshotSHA256 || !snapshot.Immutable {
		return errors.New("run snapshot integrity check failed")
	}
	recomputed, err := contracts.Snapshot(snapshot.Spec)
	if err != nil || recomputed.SHA256 != snapshot.SHA256 {
		return errors.New("run snapshot content hash verification failed")
	}
	spec := snapshot.Spec
	if err := r.discoverOwned(ctx, run, spec); err != nil {
		return err
	}
	if run.DesiredState == "cancelled" || run.Status == "cancelling" {
		if run.Status != "cancelling" {
			if err := r.suspendOwned(ctx, run); err != nil {
				return err
			}
			run.Status, run.ObservedState, run.RolloutDraining, run.UpdatedAt = "cancelling", "draining", true, time.Now()
			return r.state.SaveRLRun(run)
		}
		run.Status, run.ObservedState = "cancelling", "deleting"
		_ = r.state.SaveRLRun(run)
		done, err := r.deleteOwned(ctx, run)
		if err != nil {
			return err
		}
		if !done {
			run.UpdatedAt = time.Now()
			return r.state.SaveRLRun(run)
		}
		run.Status, run.ObservedState, run.FailureReason = "cancelled", "cancelled", ""
		run.UpdatedAt = time.Now()
		return r.state.SaveRLRun(run)
	}
	if run.DesiredState == "suspended" {
		if err := r.suspendOwned(ctx, run); err != nil {
			return err
		}
		run.Status, run.ObservedState, run.UpdatedAt = "suspended", "resources-suspended", time.Now()
		return r.state.SaveRLRun(run)
	}
	if run.Status == "evaluating" || run.Status == "canary" {
		return r.reconcileEvaluation(ctx, run, spec)
	}
	if err := r.ensureTenantInfrastructure(ctx, spec); err != nil {
		return err
	}
	attempt, err := r.ensureAttempt(run, spec)
	if err != nil {
		return err
	}
	if spec.Topology.Mode == "disaggregated" {
		if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.Services, sk8s.RenderRolloutService(spec)); err != nil {
			return err
		}
		current := run.RolloutDesiredReplicas
		if current <= 0 {
			current = spec.Topology.Rollout.Replicas
		}
		desired := DesiredRolloutReplicas(spec.Topology.Rollout, current, RolloutMetrics{
			QueueDepth: run.RolloutQueueDepth, GenerationP95Sec: run.RolloutGenerationP95,
			TokensPerSecond: run.RolloutTokensPerSecond, TrainerDemand: run.RolloutTrainerDemand,
			Backpressured: run.Backpressured,
		})
		draining := run.RolloutDraining || run.Backpressured
		if draining {
			desired = 0
		}
		run.RolloutDesiredReplicas = desired
		if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.Deployments, sk8s.RenderRolloutDeploymentState(spec, run.PolicyVersion, desired, draining)); err != nil {
			return err
		}
		if err := r.state.SaveRLRun(run); err != nil {
			return err
		}
	}
	executionSpec := spec
	if run.CheckpointID == "" {
		if checkpoint, err := r.state.LatestRLCheckpoint(run.ID); err == nil {
			run.CheckpointID, run.PolicyVersion, run.OptimizerStep = checkpoint.ID, checkpoint.PolicyVersion, checkpoint.OptimizerStep
			run.UpdatedAt = time.Now()
			if err := r.state.SaveRLRun(run); err != nil {
				return err
			}
		}
	}
	if run.CheckpointID != "" {
		checkpoint, err := r.state.GetRLCheckpoint(run.CheckpointID)
		if err != nil {
			return fmt.Errorf("load resume checkpoint: %w", err)
		}
		executionSpec.Checkpoint.ResumeFrom = checkpoint.ResumeURI
	}
	job := sk8s.RenderRayJob(executionSpec, attempt.ID)
	if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.RayJobs, job); err != nil {
		return err
	}
	observed, err := r.client.Get(ctx, sk8s.RayJobs, job.GetNamespace(), job.GetName())
	if err != nil {
		return fmt.Errorf("observe RayJob: %w", err)
	}
	return r.observeRayJob(run, attempt, spec, observed)
}

var runResourceTypes = []struct {
	gvr  schema.GroupVersionResource
	kind string
}{
	{sk8s.RayJobs, "RayJob"},
	{sk8s.Deployments, "Deployment"},
	{sk8s.Services, "Service"},
	{sk8s.ConfigMaps, "ConfigMap"},
	{sk8s.Jobs, "Job"},
	{sk8s.PVCs, "PersistentVolumeClaim"},
}

// discoverOwned rebuilds durable ownership after API/database restarts. Only
// resources carrying both SkyScale's manager and exact run label are adopted.
func (r *Reconciler) discoverOwned(ctx context.Context, run *state.RLRun, spec contracts.RLRunSpec) error {
	namespace := sk8s.NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID)
	selector := "app.kubernetes.io/managed-by=skyscale,rl.skyscale.dev/run=" + run.ID
	for _, resourceType := range runResourceTypes {
		list, err := r.client.List(ctx, resourceType.gvr, namespace, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return fmt.Errorf("discover owned %s resources: %w", resourceType.kind, err)
		}
		for i := range list.Items {
			object := &list.Items[i]
			if object.GetLabels()["rl.skyscale.dev/run"] != run.ID ||
				object.GetLabels()["app.kubernetes.io/managed-by"] != "skyscale" {
				continue
			}
			attemptID := object.GetLabels()["rl.skyscale.dev/attempt"]
			record := &state.BackendResource{
				RunID: run.ID, AttemptID: attemptID, APIVersion: object.GetAPIVersion(), Kind: resourceType.kind,
				Namespace: namespace, Name: object.GetName(), UID: string(object.GetUID()),
				Generation: object.GetGeneration(), ResourceVersion: object.GetResourceVersion(),
				DesiredState: "present", ObservedState: "discovered", UpdatedAt: time.Now(),
			}
			if err := r.state.SaveBackendResource(record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) suspendOwned(ctx context.Context, run *state.RLRun) error {
	resources, err := r.state.ListBackendResources(run.ID)
	if err != nil {
		return err
	}
	for i := range resources {
		resource := &resources[i]
		gvr, ok := gvrForKind(resource.Kind)
		if !ok {
			continue
		}
		object, err := r.client.Get(ctx, gvr, resource.Namespace, resource.Name)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		switch resource.Kind {
		case "RayJob":
			if err := unstructured.SetNestedField(object.Object, true, "spec", "suspend"); err != nil {
				return err
			}
		case "Deployment":
			if err := unstructured.SetNestedField(object.Object, int64(0), "spec", "replicas"); err != nil {
				return err
			}
			annotations := object.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations["rl.skyscale.dev/draining"] = "true"
			annotations["rl.skyscale.dev/drain-reason"] = "run-suspended"
			object.SetAnnotations(annotations)
		default:
			continue
		}
		if _, err := r.client.Apply(ctx, gvr, object); err != nil {
			return fmt.Errorf("suspend %s/%s: %w", resource.Kind, resource.Name, err)
		}
		resource.DesiredState, resource.ObservedState, resource.UpdatedAt = "suspended", "suspended", time.Now()
		if err := r.state.SaveBackendResource(resource); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) ensureTenantInfrastructure(ctx context.Context, spec contracts.RLRunSpec) error {
	maxGPUs := 8
	if raw := os.Getenv("SKYSCALE_RL_TENANT_MAX_GPUS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxGPUs = parsed
		}
	}
	resources := []struct {
		gvr    schema.GroupVersionResource
		object *unstructured.Unstructured
	}{
		{sk8s.Namespaces, sk8s.RenderTenantNamespace(spec)},
		{sk8s.ServiceAccounts, sk8s.RenderTenantServiceAccount(spec)},
		{sk8s.ResourceQuotas, sk8s.RenderTenantQuota(spec, maxGPUs)},
		{sk8s.LimitRanges, sk8s.RenderTenantLimitRange(spec)},
		{sk8s.NetworkPolicies, sk8s.RenderTenantNetworkPolicy(spec)},
	}
	for _, resource := range resources {
		if _, err := r.client.Apply(ctx, resource.gvr, resource.object); err != nil {
			return fmt.Errorf("apply tenant %s: %w", resource.object.GetKind(), err)
		}
	}
	return nil
}

func (r *Reconciler) ensureAttempt(run *state.RLRun, spec contracts.RLRunSpec) (*state.RLAttempt, error) {
	if run.CurrentAttemptID != "" {
		return r.state.GetRLAttempt(run.CurrentAttemptID)
	}
	attempts, err := r.state.ListRLAttempts(run.ID)
	if err != nil {
		return nil, err
	}
	number := len(attempts) + 1
	id := fmt.Sprintf("attempt-%03d", number)
	attempt := &state.RLAttempt{
		ID: id + "-" + run.ID, RunID: run.ID, Number: number, Status: "starting",
		RayJobName: spec.Metadata.RunID + "-" + id + "-" + run.ID,
		Namespace:  sk8s.NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID), StartedAt: time.Now(),
		ResumeCheckpointID: run.CheckpointID,
	}
	run.CurrentAttemptID = attempt.ID
	run.Status, run.ObservedState, run.UpdatedAt = "starting", "reconciling", time.Now()
	if err := r.state.SaveRLAttempt(attempt); err != nil {
		return nil, err
	}
	if err := r.state.SaveRLRun(run); err != nil {
		return nil, err
	}
	return attempt, nil
}

func (r *Reconciler) applyAndTrack(ctx context.Context, run *state.RLRun, attempt *state.RLAttempt, gvr schema.GroupVersionResource, object *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	applied, err := r.client.Apply(ctx, gvr, object)
	if err != nil {
		return nil, fmt.Errorf("apply %s/%s: %w", object.GetKind(), object.GetName(), err)
	}
	resource := &state.BackendResource{
		RunID: run.ID, AttemptID: attempt.ID, APIVersion: object.GetAPIVersion(), Kind: object.GetKind(),
		Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(applied.GetUID()),
		Generation: applied.GetGeneration(), ResourceVersion: applied.GetResourceVersion(),
		DesiredState: "present", ObservedState: "present", UpdatedAt: time.Now(),
	}
	return applied, r.state.SaveBackendResource(resource)
}

func (r *Reconciler) observeRayJob(run *state.RLRun, attempt *state.RLAttempt, spec contracts.RLRunSpec, job *unstructured.Unstructured) error {
	jobStatus, _, _ := unstructured.NestedString(job.Object, "status", "jobStatus")
	deploymentStatus, _, _ := unstructured.NestedString(job.Object, "status", "jobDeploymentStatus")
	if deploymentStatus == "" {
		deploymentStatus, _, _ = unstructured.NestedString(job.Object, "status", "deploymentStatus")
	}
	observed := jobStatus
	if observed == "" {
		observed = deploymentStatus
	}
	switch observed {
	case "SUCCEEDED", "Complete", "Succeeded":
		attempt.Status, attempt.FinishedAt = "completed", time.Now()
		if spec.Evaluation.SuiteURI != "" || len(spec.Evaluation.Gates) > 0 {
			evaluation := &state.RLEvaluationRecord{
				ID: "eval-" + run.ID + "-" + run.PolicyVersion, RunID: run.ID,
				PolicyVersion: run.PolicyVersion, CheckpointID: run.CheckpointID,
				Status: "pending", Final: true, CanaryState: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}
			if err := r.state.SaveRLEvaluation(evaluation); err != nil {
				return err
			}
			run.Status, run.ObservedState, run.FailureReason = "evaluating", "evaluation-pending", ""
		} else {
			run.Status, run.ObservedState, run.FailureReason = "completed", "succeeded", ""
		}
	case "FAILED", "Failed":
		reason, _, _ := unstructured.NestedString(job.Object, "status", "message")
		if reason == "" {
			reason = "RayJob failed"
		}
		attempt.Status, attempt.FinishedAt, attempt.FailureReason = "failed", time.Now(), reason
		maxRetries := 2
		if raw := spec.Algorithm.Arguments["max_retries"]; raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 && parsed <= 10 {
				maxRetries = parsed
			}
		}
		if attempt.Number <= maxRetries {
			run.CurrentAttemptID, run.Status, run.ObservedState = "", "starting", "retrying"
		} else {
			run.Status, run.ObservedState, run.FailureReason = "failed", "failed", reason
		}
	default:
		attempt.Status = "running"
		run.Status, run.ObservedState = "running", observed
		if run.ObservedState == "" {
			run.ObservedState = "pending"
		}
	}
	run.UpdatedAt = time.Now()
	if err := r.state.SaveRLAttempt(attempt); err != nil {
		return err
	}
	return r.state.SaveRLRun(run)
}

func rayJobStatus(job *unstructured.Unstructured) string {
	status, _, _ := unstructured.NestedString(job.Object, "status", "jobStatus")
	if status == "" {
		status, _, _ = unstructured.NestedString(job.Object, "status", "jobDeploymentStatus")
	}
	return status
}

func (r *Reconciler) reconcileEvaluation(ctx context.Context, run *state.RLRun, spec contracts.RLRunSpec) error {
	evaluation, err := r.state.GetRLEvaluation(run.ID, run.PolicyVersion)
	if err != nil {
		return fmt.Errorf("load evaluation: %w", err)
	}
	attempt, err := r.state.GetRLAttempt(run.CurrentAttemptID)
	if err != nil {
		return err
	}
	stableVersion := run.LastGoodPolicyVersion
	if stableVersion == "" {
		stableVersion = run.PolicyVersion
	}
	if spec.Topology.Mode == "disaggregated" {
		stable := sk8s.RenderRolloutDeploymentState(spec, stableVersion, spec.Topology.Rollout.MinReplicas, false)
		if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.Deployments, stable); err != nil {
			return err
		}
	}
	if run.Status == "evaluating" {
		job := sk8s.RenderEvaluatorRayJob(spec, evaluation.ID, evaluation.PolicyVersion)
		evaluation.RayJobName = job.GetName()
		if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.RayJobs, job); err != nil {
			return err
		}
		observed, err := r.client.Get(ctx, sk8s.RayJobs, job.GetNamespace(), job.GetName())
		if err != nil {
			return err
		}
		switch rayJobStatus(observed) {
		case "FAILED", "Failed":
			evaluation.Status, evaluation.FailureReason = "failed", "evaluator RayJob failed"
		case "SUCCEEDED", "Succeeded", "Complete":
			if evaluation.Status == "passed" && evaluation.GatesPassed {
				percent := spec.Evaluation.CanaryPercent
				if percent <= 0 {
					percent = 10
				}
				replicas := (spec.Topology.Rollout.Replicas*percent + 99) / 100
				if replicas < 1 {
					replicas = 1
				}
				canary := sk8s.RenderCanaryRolloutDeployment(spec, run.PolicyVersion, replicas, false)
				if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.Deployments, canary); err != nil {
					return err
				}
				if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.Services, sk8s.RenderCanaryRolloutService(spec)); err != nil {
					return err
				}
				evaluation.CanaryState, run.Status, run.ObservedState = "running", "canary", "canary-running"
			} else if evaluation.Status != "failed" {
				evaluation.Status, run.ObservedState = "awaiting-metrics", "evaluation-awaiting-metrics"
			}
		default:
			evaluation.Status, run.ObservedState = "running", "evaluation-running"
		}
	}
	if run.Status == "canary" && evaluation.CanaryState == "running" {
		job := sk8s.RenderEvaluatorRayJobPhase(spec, evaluation.ID, evaluation.PolicyVersion, "canary")
		if _, err := r.applyAndTrack(ctx, run, attempt, sk8s.RayJobs, job); err != nil {
			return err
		}
		observed, err := r.client.Get(ctx, sk8s.RayJobs, job.GetNamespace(), job.GetName())
		if err != nil {
			return err
		}
		switch rayJobStatus(observed) {
		case "FAILED", "Failed":
			evaluation.CanaryState, evaluation.FailureReason = "failed", "canary evaluator RayJob failed"
		case "SUCCEEDED", "Succeeded", "Complete":
			run.ObservedState = "canary-awaiting-metrics"
		}
	}
	if evaluation.Status == "failed" || evaluation.CanaryState == "failed" {
		if r.promotions == nil {
			return errors.New("promotion controller is unavailable")
		}
		rollback, err := r.promotions.Rollback(ctx, run.ID, evaluation.FailureReason)
		if err != nil {
			return err
		}
		run.PolicyVersion, run.RolloutDraining = rollback, true
		if evaluation.Final {
			run.Status, run.ObservedState = "completed", "rolled-back"
		} else {
			run.Status, run.ObservedState = "running", "periodic-evaluation-rolled-back"
		}
	} else if run.Status == "canary" && evaluation.CanaryState == "passed" {
		if r.promotions == nil {
			return errors.New("promotion controller is unavailable")
		}
		if err := r.promotions.MarkGood(ctx, run.ID, run.PolicyVersion); err != nil {
			return err
		}
		run.LastGoodPolicyVersion, run.RolloutDraining = run.PolicyVersion, false
		if evaluation.Final {
			run.Status, run.ObservedState = "completed", "promoted"
		} else {
			run.Status, run.ObservedState = "running", "periodic-evaluation-promoted"
		}
		canary := sk8s.RenderCanaryRolloutDeployment(spec, run.PolicyVersion, 0, true)
		_, _ = r.client.Apply(ctx, sk8s.Deployments, canary)
	}
	evaluation.UpdatedAt, run.UpdatedAt = time.Now(), time.Now()
	if err := r.state.SaveRLEvaluation(evaluation); err != nil {
		return err
	}
	return r.state.SaveRLRun(run)
}

func (r *Reconciler) deleteOwned(ctx context.Context, run *state.RLRun) (bool, error) {
	resources, err := r.state.ListBackendResources(run.ID)
	if err != nil {
		return false, err
	}
	allDeleted := true
	for i := range resources {
		resource := &resources[i]
		gvr, ok := gvrForKind(resource.Kind)
		if !ok {
			continue
		}
		object, err := r.client.Get(ctx, gvr, resource.Namespace, resource.Name)
		if err == nil {
			if resource.UID != "" && string(object.GetUID()) != resource.UID {
				return false, fmt.Errorf("refuse to delete replaced %s/%s: UID no longer matches", resource.Kind, resource.Name)
			}
			labels := object.GetLabels()
			if labels["app.kubernetes.io/managed-by"] != "skyscale" || labels["rl.skyscale.dev/run"] != run.ID {
				return false, fmt.Errorf("refuse to delete unowned %s/%s", resource.Kind, resource.Name)
			}
			finalizers := object.GetFinalizers()
			kept := finalizers[:0]
			for _, finalizer := range finalizers {
				if finalizer != sk8s.Finalizer {
					kept = append(kept, finalizer)
				}
			}
			if len(kept) != len(finalizers) {
				object.SetFinalizers(kept)
				if _, applyErr := r.client.Apply(ctx, gvr, object); applyErr != nil {
					return false, fmt.Errorf("remove finalizer from %s/%s: %w", resource.Kind, resource.Name, applyErr)
				}
			}
		} else if !apierrors.IsNotFound(err) {
			return false, err
		}
		propagation := metav1.DeletePropagationForeground
		if err := r.client.Delete(ctx, gvr, resource.Namespace, resource.Name, metav1.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("delete %s/%s: %w", resource.Kind, resource.Name, err)
		}
		if _, err := r.client.Get(ctx, gvr, resource.Namespace, resource.Name); err == nil {
			resource.DeletionPending, resource.DesiredState, resource.UpdatedAt = true, "absent", time.Now()
			if err := r.state.SaveBackendResource(resource); err != nil {
				return false, err
			}
			allDeleted = false
			continue
		} else if !apierrors.IsNotFound(err) {
			return false, err
		}
		if err := r.state.DeleteBackendResource(resource.ID); err != nil {
			return false, err
		}
	}
	return allDeleted, nil
}

func gvrForKind(kind string) (schema.GroupVersionResource, bool) {
	switch kind {
	case "RayJob":
		return sk8s.RayJobs, true
	case "Deployment":
		return sk8s.Deployments, true
	case "Service":
		return sk8s.Services, true
	case "ConfigMap":
		return sk8s.ConfigMaps, true
	case "Secret":
		return sk8s.Secrets, true
	case "Job":
		return sk8s.Jobs, true
	case "PersistentVolumeClaim":
		return sk8s.PVCs, true
	default:
		return schema.GroupVersionResource{}, false
	}
}
