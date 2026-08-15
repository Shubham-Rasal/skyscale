package rlservice

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
	sk8s "github.com/bluequbit/faas/control-plane/k8s"
	"github.com/bluequbit/faas/control-plane/state"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

type fakeKube struct {
	objects        map[string]*unstructured.Unstructured
	retainOnDelete bool
}

func fakeKey(gvr schema.GroupVersionResource, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", gvr.Resource, namespace, name)
}

func (f *fakeKube) Apply(_ context.Context, gvr schema.GroupVersionResource, object *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	copy := object.DeepCopy()
	copy.SetUID(types.UID("uid-" + object.GetName()))
	copy.SetResourceVersion("1")
	key := fakeKey(gvr, copy.GetNamespace(), copy.GetName())
	if existing := f.objects[key]; existing != nil {
		if status, ok := existing.Object["status"]; ok {
			copy.Object["status"] = status
		}
	}
	f.objects[key] = copy
	return copy.DeepCopy(), nil
}

func (f *fakeKube) Get(_ context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if object := f.objects[fakeKey(gvr, namespace, name)]; object != nil {
		return object.DeepCopy(), nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, name)
}

func (f *fakeKube) List(_ context.Context, gvr schema.GroupVersionResource, namespace string, options metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}
	for key, object := range f.objects {
		if key == fakeKey(gvr, namespace, object.GetName()) &&
			object.GetLabels()["app.kubernetes.io/managed-by"] == "skyscale" {
			if options.LabelSelector == "" || object.GetLabels()["rl.skyscale.dev/run"] != "" {
				list.Items = append(list.Items, *object.DeepCopy())
			}
		}
	}
	return list, nil
}

func (f *fakeKube) Delete(_ context.Context, gvr schema.GroupVersionResource, namespace, name string, _ metav1.DeleteOptions) error {
	if f.retainOnDelete {
		return nil
	}
	delete(f.objects, fakeKey(gvr, namespace, name))
	return nil
}

func TestCleanupWaitsForForegroundDeletion(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}, retainOnDelete: true}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest, spec.CreatedAt = "sha256:test", time.Unix(1, 0)
	snapshot, _ := contracts.Snapshot(spec)
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{ID: "run", Backend: "slime", Status: "starting", DesiredState: "running", SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256}
	_ = manager.SaveRLRun(run)
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	run.Status, run.DesiredState = "cancelling", "cancelled"
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	if run.Status != "cancelling" {
		t.Fatalf("cleanup claimed completion before deletion: %s", run.Status)
	}
	resources, _ := manager.ListBackendResources("run")
	if len(resources) == 0 || !resources[0].DeletionPending {
		t.Fatalf("deletion pending state not persisted: %#v", resources)
	}
	kube.retainOnDelete = false
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	if run.Status != "cancelled" {
		t.Fatalf("cleanup did not finish after resources disappeared: %s", run.Status)
	}
}

func reconcilerState(t *testing.T) *state.StateManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&state.RLRun{}, &state.RLAttempt{}, &state.BackendResource{}, &state.RLEvaluationRecord{}, &state.RLCheckpointRecord{}); err != nil {
		t.Fatal(err)
	}
	return state.NewStateManagerFromDB(db, logrus.New())
}

type fakePromotions struct {
	good       string
	rolledBack string
}

func (f *fakePromotions) MarkGood(_ context.Context, _, version string) error {
	f.good = version
	return nil
}

func (f *fakePromotions) Rollback(_ context.Context, _, _ string) (string, error) {
	f.rolledBack = "v1"
	return "v1", nil
}

func TestReconcileCreatesObservesAndCancelsOwnedResources(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest = "sha256:test"
	spec.CreatedAt = time.Unix(1, 0)
	snapshot, err := contracts.Snapshot(spec)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{
		ID: "run", Backend: "slime", Status: "starting", DesiredState: "running",
		SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256,
	}
	if err := manager.SaveRLRun(run); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	updated, _ := manager.GetRLRun("run")
	if updated.Status != "running" || len(kube.objects) != 6 {
		t.Fatalf("unexpected reconciled state: status=%s objects=%d", updated.Status, len(kube.objects))
	}
	for key, object := range kube.objects {
		if object.GetKind() == "RayJob" {
			object.Object["status"] = map[string]any{"jobStatus": "SUCCEEDED"}
			kube.objects[key] = object
		}
	}
	if err := reconciler.Reconcile(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	updated, _ = manager.GetRLRun("run")
	if updated.Status != "completed" {
		t.Fatalf("expected authoritative RayJob completion, got %s", updated.Status)
	}
	updated.Status, updated.DesiredState = "running", "cancelled"
	if err := manager.SaveRLRun(updated); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	updated, _ = manager.GetRLRun("run")
	if updated.Status != "cancelling" || updated.ObservedState != "draining" {
		t.Fatalf("cancellation did not enter drain phase: %#v", updated)
	}
	if err := reconciler.Reconcile(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if len(kube.objects) != 5 {
		t.Fatalf("run resources were not deleted without removing tenant guardrails: %d", len(kube.objects))
	}
}

func TestReconcileRejectsTamperedSnapshotContent(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest = "sha256:test"
	spec.CreatedAt = time.Unix(1, 0)
	snapshot, err := contracts.Snapshot(spec)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Spec.Metadata.TenantID = "other-tenant"
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{
		ID: "run", Backend: "slime", Status: "starting", DesiredState: "running",
		SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256,
	}
	if err := reconciler.Reconcile(context.Background(), run); err == nil {
		t.Fatal("expected tampered snapshot content to be rejected")
	}
	if len(kube.objects) != 0 {
		t.Fatalf("tampered snapshot created %d Kubernetes objects", len(kube.objects))
	}
}

func TestSuspendResumeMutatesKubernetesResources(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest, spec.CreatedAt = "sha256:test", time.Unix(1, 0)
	spec.Topology.Mode, spec.Topology.Rollout.External = "disaggregated", true
	snapshot, _ := contracts.Snapshot(spec)
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{ID: "run", Backend: "slime", Status: "starting", DesiredState: "running", SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256}
	if err := manager.SaveRLRun(run); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	run.DesiredState = "suspended"
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	for _, object := range kube.objects {
		switch object.GetKind() {
		case "RayJob":
			suspended, _, _ := unstructured.NestedBool(object.Object, "spec", "suspend")
			if !suspended {
				t.Fatal("RayJob was not suspended")
			}
		case "Deployment":
			replicas, _, _ := unstructured.NestedInt64(object.Object, "spec", "replicas")
			if replicas != 0 || object.GetAnnotations()["rl.skyscale.dev/draining"] != "true" {
				t.Fatalf("rollout not drained: replicas=%d annotations=%v", replicas, object.GetAnnotations())
			}
		}
	}
	run, _ = manager.GetRLRun("run")
	run.DesiredState = "running"
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	for _, object := range kube.objects {
		switch object.GetKind() {
		case "RayJob":
			suspended, _, _ := unstructured.NestedBool(object.Object, "spec", "suspend")
			if suspended {
				t.Fatal("RayJob remained suspended after resume")
			}
		case "Deployment":
			replicas, _, _ := unstructured.NestedInt64(object.Object, "spec", "replicas")
			if replicas != int64(spec.Topology.Rollout.Replicas) {
				t.Fatalf("rollout replicas not restored: %d", replicas)
			}
		}
	}
}

func TestOrphanDiscoveryAdoptsLabeledResource(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest, spec.CreatedAt = "sha256:test", time.Unix(1, 0)
	snapshot, _ := contracts.Snapshot(spec)
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{ID: "run", Backend: "slime", DesiredState: "cancelled", Status: "cancelling", SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256}
	if err := manager.SaveRLRun(run); err != nil {
		t.Fatal(err)
	}
	orphan := sk8s.RenderRayJob(spec, "orphan-attempt")
	orphan.SetUID(types.UID("orphan-uid"))
	kube.objects[fakeKey(sk8s.RayJobs, orphan.GetNamespace(), orphan.GetName())] = orphan
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if len(kube.objects) != 0 {
		t.Fatalf("discovered orphan was not cleaned up: %d objects", len(kube.objects))
	}
	resources, _ := manager.ListBackendResources("run")
	if len(resources) != 0 {
		t.Fatalf("orphan ownership rows remained: %v", resources)
	}
}

func TestEvaluatorRayJobAndCanaryPromotion(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}}
	promotions := &fakePromotions{}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	reconciler.SetPromotionController(promotions)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest, spec.CreatedAt = "sha256:test", time.Unix(1, 0)
	spec.Topology.Mode, spec.Topology.Rollout.External = "disaggregated", true
	spec.Evaluation = contracts.EvaluationPolicy{
		SuiteURI: "s3://suites/frozen.json", SuiteHash: "abc", CanaryPercent: 50,
		Gates: []contracts.EvaluationGate{{Metric: "pass_rate", Operator: ">=", Threshold: 0.8}},
	}
	snapshot, _ := contracts.Snapshot(spec)
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{
		ID: "run", Backend: "slime", Status: "starting", DesiredState: "running",
		PolicyVersion: "v2", LastGoodPolicyVersion: "v1",
		SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256,
	}
	if err := manager.SaveRLRun(run); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	for _, object := range kube.objects {
		if object.GetKind() == "RayJob" && object.GetLabels()["rl.skyscale.dev/role"] != "evaluator" {
			object.Object["status"] = map[string]any{"jobStatus": "SUCCEEDED"}
		}
	}
	run, _ = manager.GetRLRun("run")
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	if run.Status != "evaluating" {
		t.Fatalf("trainer success did not start evaluation: %s", run.Status)
	}
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	evaluation, err := manager.GetRLEvaluation("run", "v2")
	if err != nil || evaluation.RayJobName == "" {
		t.Fatalf("evaluation record/job missing: %#v err=%v", evaluation, err)
	}
	for _, object := range kube.objects {
		if object.GetName() == evaluation.RayJobName {
			object.Object["status"] = map[string]any{"jobStatus": "SUCCEEDED"}
		}
	}
	evaluation.Status, evaluation.GatesPassed = "passed", true
	if err := manager.SaveRLEvaluation(evaluation); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	if run.Status != "canary" {
		t.Fatalf("evaluation did not start canary: %s", run.Status)
	}
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	foundCanaryJob, foundCanaryService := false, false
	for _, object := range kube.objects {
		foundCanaryJob = foundCanaryJob || (object.GetKind() == "RayJob" && object.GetLabels()["rl.skyscale.dev/role"] == "evaluator" && object.GetName() != evaluation.RayJobName)
		foundCanaryService = foundCanaryService || (object.GetKind() == "Service" && object.GetName() == "run-rollout-canary")
	}
	if !foundCanaryJob || !foundCanaryService {
		t.Fatalf("canary evaluator resources missing: job=%v service=%v", foundCanaryJob, foundCanaryService)
	}
	evaluation, _ = manager.GetRLEvaluation("run", "v2")
	evaluation.CanaryState = "passed"
	_ = manager.SaveRLEvaluation(evaluation)
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	run, _ = manager.GetRLRun("run")
	if run.Status != "completed" || run.ObservedState != "promoted" || promotions.good != "v2" {
		t.Fatalf("canary promotion incomplete: run=%#v promotion=%s", run, promotions.good)
	}
}

func TestReconcilerAppliesAutoscalingAndBackpressure(t *testing.T) {
	manager := reconcilerState(t)
	kube := &fakeKube{objects: map[string]*unstructured.Unstructured{}}
	reconciler := NewReconciler(manager, kube, logrus.New(), time.Second)
	spec := contracts.DefaultRunSpec()
	spec.Metadata = contracts.ObjectMeta{TenantID: "tenant", ProjectID: "project", RunID: "run"}
	spec.Image.Digest, spec.CreatedAt = "sha256:test", time.Unix(1, 0)
	spec.Topology.Mode, spec.Topology.Rollout.External = "disaggregated", true
	spec.Topology.Rollout.MinReplicas, spec.Topology.Rollout.MaxReplicas = 1, 4
	snapshot, _ := contracts.Snapshot(spec)
	raw, _ := json.Marshal(snapshot)
	run := &state.RLRun{
		ID: "run", Backend: "slime", Status: "starting", DesiredState: "running",
		RolloutQueueDepth: 100, RolloutTokensPerSecond: 25,
		SnapshotJSON: string(raw), SnapshotSHA256: snapshot.SHA256,
	}
	_ = manager.SaveRLRun(run)
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	deployment := kube.objects[fakeKey(sk8s.Deployments, sk8s.NamespaceFor("tenant", "project"), "run-rollout")]
	replicas, _, _ := unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	if replicas != 4 {
		t.Fatalf("controller did not apply queue-driven scale decision: %d", replicas)
	}
	run, _ = manager.GetRLRun("run")
	run.Backpressured = true
	if err := reconciler.Reconcile(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	deployment = kube.objects[fakeKey(sk8s.Deployments, sk8s.NamespaceFor("tenant", "project"), "run-rollout")]
	replicas, _, _ = unstructured.NestedInt64(deployment.Object, "spec", "replicas")
	if replicas != 0 || deployment.GetAnnotations()["rl.skyscale.dev/draining"] != "true" {
		t.Fatalf("backpressure did not drain rollout fleet: replicas=%d annotations=%v", replicas, deployment.GetAnnotations())
	}
}
