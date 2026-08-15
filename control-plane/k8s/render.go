package k8s

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/bluequbit/faas/control-plane/contracts"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const Finalizer = "rl.skyscale.dev/finalizer"

func NamespaceFor(tenantID, projectID string) string {
	value := strings.ToLower("skyscale-" + tenantID + "-" + projectID)
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, value)
	value = strings.Trim(value, "-")
	if len(value) > 63 {
		value = value[:63]
	}
	return value
}

func labels(spec contracts.RLRunSpec) map[string]any {
	return map[string]any{
		"app.kubernetes.io/managed-by": "skyscale",
		"app.kubernetes.io/part-of":    "skyscale-rl",
		"rl.skyscale.dev/tenant":       spec.Metadata.TenantID,
		"rl.skyscale.dev/project":      spec.Metadata.ProjectID,
		"rl.skyscale.dev/run":          spec.Metadata.RunID,
	}
}

func metadata(spec contracts.RLRunSpec, name string) map[string]any {
	return map[string]any{
		"name":       name,
		"namespace":  NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID),
		"labels":     labels(spec),
		"finalizers": []any{Finalizer},
	}
}

func sharedMetadata(spec contracts.RLRunSpec, name string) map[string]any {
	return map[string]any{
		"name": name, "namespace": NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID),
		"labels": map[string]any{
			"app.kubernetes.io/managed-by": "skyscale",
			"rl.skyscale.dev/tenant":       spec.Metadata.TenantID,
			"rl.skyscale.dev/project":      spec.Metadata.ProjectID,
		},
	}
}

func image(spec contracts.RLRunSpec) string {
	if strings.Contains(spec.Image.Slime, "@sha256:") || spec.Image.Digest == "" {
		return spec.Image.Slime
	}
	return spec.Image.Slime + "@" + spec.Image.Digest
}

func slimeArgs(spec contracts.RLRunSpec) []any {
	args := []string{
		"python", strategyEntrypoint(spec.Algorithm.Strategy),
		"--model", spec.Model.Source,
		"--tensor-model-parallel-size", strconv.Itoa(spec.Topology.Trainer.TP),
		"--pipeline-model-parallel-size", strconv.Itoa(spec.Topology.Trainer.PP),
		"--context-parallel-size", strconv.Itoa(spec.Topology.Trainer.CP),
	}
	if spec.Topology.Mode == "disaggregated" {
		address := fmt.Sprintf("%s-rollout.%s.svc.cluster.local:8000", spec.Metadata.RunID, NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID))
		args = append(args, "--rollout-external-engine-addrs", address)
	}
	if spec.Checkpoint.ResumeFrom != "" {
		args = append(args, "--load", spec.Checkpoint.ResumeFrom)
	}
	keys := make([]string, 0, len(spec.Algorithm.Arguments))
	for key := range spec.Algorithm.Arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "max_retries" {
			continue
		}
		args = append(args, key, spec.Algorithm.Arguments[key])
	}
	out := make([]any, len(args))
	for i := range args {
		out[i] = args[i]
	}
	return out
}

func strategyEntrypoint(strategy string) string {
	if strategy == "synchronous" {
		return "train.py"
	}
	return "train_async.py"
}

func resources(r contracts.ResourceSpec) map[string]any {
	requests := map[string]any{}
	limits := map[string]any{}
	if r.CPU != "" {
		requests["cpu"], limits["cpu"] = r.CPU, r.CPU
	}
	if r.Memory != "" {
		requests["memory"], limits["memory"] = r.Memory, r.Memory
	}
	if r.GPUs > 0 {
		requests["nvidia.com/gpu"], limits["nvidia.com/gpu"] = int64(r.GPUs), int64(r.GPUs)
	}
	return map[string]any{"requests": requests, "limits": limits}
}

func podTemplate(spec contracts.RLRunSpec, role string, resource contracts.ResourceSpec, command []any) map[string]any {
	serviceAccount := spec.Security.ServiceAccountName
	if serviceAccount == "" {
		serviceAccount = "skyscale-rl-runner"
	}
	env := []any{
		map[string]any{"name": "SKYSCALE_RUN_ID", "value": spec.Metadata.RunID},
		map[string]any{"name": "SKYSCALE_TENANT_ID", "value": spec.Metadata.TenantID},
		map[string]any{"name": "SKYSCALE_PROJECT_ID", "value": spec.Metadata.ProjectID},
		map[string]any{"name": "SKYSCALE_CONTROL_PLANE_URL", "value": "http://skyscale-control-plane.skyscale-system.svc.cluster.local:8080"},
		// Credentials are mounted through explicit secret references, never serialized into samples.
	}
	container := map[string]any{
		"name": "slime-" + role, "image": image(spec), "imagePullPolicy": "IfNotPresent",
		"args": command, "env": env, "resources": resources(resource),
		"securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": false, "capabilities": map[string]any{"drop": []any{"ALL"}}},
	}
	podSpec := map[string]any{
		"serviceAccountName": serviceAccount,
		"restartPolicy":      "Always",
		"containers":         []any{container},
		"nodeSelector":       mapStringAny(resource.Selectors),
		"securityContext":    map[string]any{"runAsNonRoot": true, "seccompProfile": map[string]any{"type": "RuntimeDefault"}},
	}
	for _, secret := range spec.Security.SecretRefs {
		container["envFrom"] = appendAny(container["envFrom"], map[string]any{"secretRef": map[string]any{"name": secret}})
	}
	return map[string]any{"metadata": map[string]any{"labels": labels(spec)}, "spec": podSpec}
}

func mapStringAny(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func appendAny(value any, item any) []any {
	if value == nil {
		return []any{item}
	}
	return append(value.([]any), item)
}

// RenderRayJob creates a finite trainer/evaluator execution. Ray's observed
// deploymentStatus/jobStatus fields remain authoritative for reconciliation.
func RenderRayJob(spec contracts.RLRunSpec, attemptID string) *unstructured.Unstructured {
	name := spec.Metadata.RunID + "-" + attemptID
	head := podTemplate(spec, "head", contracts.ResourceSpec{CPU: "2", Memory: "8Gi", Selectors: spec.Topology.Trainer.Resources.Selectors}, []any{})
	worker := podTemplate(spec, "trainer", spec.Topology.Trainer.Resources, []any{})
	submitter := podTemplate(spec, "submitter", contracts.ResourceSpec{CPU: "1", Memory: "2Gi"}, []any{})
	submitter["spec"].(map[string]any)["restartPolicy"] = "Never"
	workerReplicas := int64(spec.Topology.Trainer.Nodes)
	object := map[string]any{
		"apiVersion": "ray.io/v1", "kind": "RayJob", "metadata": metadata(spec, name),
		"spec": map[string]any{
			"entrypoint":               shellCommand(slimeArgs(spec)),
			"suspend":                  false,
			"shutdownAfterJobFinishes": true,
			"ttlSecondsAfterFinished":  int64(3600),
			"submitterPodTemplate":     submitter,
			"rayClusterSpec": map[string]any{
				"rayVersion":    "2.52.0",
				"headGroupSpec": map[string]any{"rayStartParams": map[string]any{"dashboard-host": "0.0.0.0"}, "template": head},
				"workerGroupSpecs": []any{map[string]any{
					"groupName": "trainer", "replicas": workerReplicas, "minReplicas": workerReplicas, "maxReplicas": workerReplicas,
					"rayStartParams": map[string]any{}, "template": worker,
				}},
			},
		},
	}
	object["metadata"].(map[string]any)["labels"].(map[string]any)["rl.skyscale.dev/attempt"] = attemptID
	return &unstructured.Unstructured{Object: object}
}

func anyStrings(values []any) []string {
	out := make([]string, len(values))
	for i := range values {
		out[i] = fmt.Sprint(values[i])
	}
	return out
}

func shellCommand(values []any) string {
	quoted := make([]string, len(values))
	for i := range values {
		value := fmt.Sprint(values[i])
		quoted[i] = "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func RenderRolloutDeployment(spec contracts.RLRunSpec, policyVersion string) *unstructured.Unstructured {
	return RenderRolloutDeploymentState(spec, policyVersion, spec.Topology.Rollout.Replicas, false)
}

func RenderRolloutDeploymentState(spec contracts.RLRunSpec, policyVersion string, replicas int, draining bool) *unstructured.Unstructured {
	name := spec.Metadata.RunID + "-rollout"
	r := spec.Topology.Rollout
	args := []any{"python", "-m", "sglang.launch_server", "--model-path", spec.Model.Source,
		"--tp-size", strconv.Itoa(r.TP), "--dp-size", strconv.Itoa(r.DP),
		"--mem-fraction-static", strconv.FormatFloat(r.MemoryFraction, 'f', 2, 64), "--host", "0.0.0.0", "--port", "8000"}
	template := podTemplate(spec, "rollout", r.Resources, args)
	templateSpec := template["spec"].(map[string]any)
	containers := templateSpec["containers"].([]any)
	c := containers[0].(map[string]any)
	c["image"] = spec.Image.SGLang
	if c["image"] == "" {
		c["image"] = image(spec)
	}
	c["readinessProbe"] = map[string]any{"httpGet": map[string]any{"path": "/server_info", "port": int64(8000)}, "periodSeconds": int64(5), "failureThreshold": int64(12)}
	c["lifecycle"] = map[string]any{"preStop": map[string]any{"exec": map[string]any{"command": []any{"sh", "-c", "sleep 15"}}}}
	c["env"] = append(c["env"].([]any), map[string]any{"name": "SKYSCALE_POLICY_VERSION", "value": policyVersion})
	c["volumeMounts"] = []any{map[string]any{"name": "weights", "mountPath": "/var/lib/skyscale/weights"}}
	registrar := map[string]any{
		"name":  "engine-registrar",
		"image": image(spec),
		"args":  []any{"python", "/opt/skyscale/slime/register_engine.py"},
		"env": []any{
			map[string]any{"name": "SKYSCALE_ENGINE_ID", "valueFrom": map[string]any{"fieldRef": map[string]any{"fieldPath": "metadata.name"}}},
			map[string]any{"name": "SKYSCALE_RUN_ID", "value": spec.Metadata.RunID},
			map[string]any{"name": "SKYSCALE_POLICY_VERSION", "value": policyVersion},
			map[string]any{"name": "SKYSCALE_CONTROL_PLANE_URL", "value": "http://skyscale-control-plane.skyscale-system.svc.cluster.local:8080"},
		},
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "50m", "memory": "64Mi"},
			"limits":   map[string]any{"cpu": "200m", "memory": "128Mi"},
		},
		"securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []any{"ALL"}}},
		"volumeMounts":    []any{map[string]any{"name": "weights", "mountPath": "/var/lib/skyscale/weights"}},
	}
	for _, secret := range spec.Security.SecretRefs {
		registrar["envFrom"] = appendAny(registrar["envFrom"], map[string]any{"secretRef": map[string]any{"name": secret}})
	}
	templateSpec["containers"] = append(containers, registrar)
	templateSpec["volumes"] = []any{map[string]any{"name": "weights", "emptyDir": map[string]any{}}}
	object := map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment", "metadata": metadata(spec, name),
		"spec": map[string]any{
			"replicas": int64(replicas),
			"strategy": map[string]any{"type": "RollingUpdate", "rollingUpdate": map[string]any{"maxUnavailable": int64(0), "maxSurge": int64(1)}},
			"selector": map[string]any{"matchLabels": map[string]any{"rl.skyscale.dev/run": spec.Metadata.RunID, "rl.skyscale.dev/role": "rollout"}},
			"template": template,
		},
	}
	object["metadata"].(map[string]any)["annotations"] = map[string]any{
		"rl.skyscale.dev/draining":   strconv.FormatBool(draining),
		"rl.skyscale.dev/autoscaler": "skyscale-controller",
	}
	templateSpec["terminationGracePeriodSeconds"] = int64(45)
	if draining {
		c["env"] = append(c["env"].([]any), map[string]any{"name": "SKYSCALE_DRAINING", "value": "1"})
	}
	templateLabels := object["spec"].(map[string]any)["template"].(map[string]any)["metadata"].(map[string]any)["labels"].(map[string]any)
	templateLabels["rl.skyscale.dev/role"] = "rollout"
	return &unstructured.Unstructured{Object: object}
}

func RenderRolloutService(spec contracts.RLRunSpec) *unstructured.Unstructured {
	object := map[string]any{
		"apiVersion": "v1", "kind": "Service", "metadata": metadata(spec, spec.Metadata.RunID+"-rollout"),
		"spec": map[string]any{
			"clusterIP":             "None",
			"sessionAffinity":       "ClientIP",
			"sessionAffinityConfig": map[string]any{"clientIP": map[string]any{"timeoutSeconds": int64(300)}},
			"selector":              map[string]any{"rl.skyscale.dev/run": spec.Metadata.RunID, "rl.skyscale.dev/role": "rollout"},
			"ports":                 []any{map[string]any{"name": "http", "port": int64(8000), "targetPort": int64(8000)}},
		},
	}
	return &unstructured.Unstructured{Object: object}
}

func RenderEvaluatorRayJob(spec contracts.RLRunSpec, evaluationID, policyVersion string) *unstructured.Unstructured {
	return RenderEvaluatorRayJobPhase(spec, evaluationID, policyVersion, "evaluation")
}

func RenderEvaluatorRayJobPhase(spec contracts.RLRunSpec, evaluationID, policyVersion, phase string) *unstructured.Unstructured {
	evalSpec := spec
	evalSpec.Algorithm.Strategy = "synchronous"
	job := RenderRayJob(evalSpec, evaluationID+"-"+phase)
	job.SetName(spec.Metadata.RunID + "-eval-" + evaluationID + "-" + phase)
	jobLabels := job.GetLabels()
	jobLabels["rl.skyscale.dev/role"] = "evaluator"
	job.SetLabels(jobLabels)
	serviceSuffix := "-rollout"
	if phase == "canary" {
		serviceSuffix = "-rollout-canary"
	}
	entrypoint := []any{
		"python", "-m", "skyscale.evaluate",
		"--run-id", spec.Metadata.RunID,
		"--policy-version", policyVersion,
		"--suite-uri", spec.Evaluation.SuiteURI,
		"--suite-hash", spec.Evaluation.SuiteHash,
		"--engine-url", fmt.Sprintf("http://%s%s.%s.svc.cluster.local:8000", spec.Metadata.RunID, serviceSuffix, NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID)),
		"--control-plane-url", "http://skyscale-control-plane.skyscale-system.svc.cluster.local:8080",
		"--phase", phase,
	}
	_ = unstructured.SetNestedField(job.Object, shellCommand(entrypoint), "spec", "entrypoint")
	_ = unstructured.SetNestedField(job.Object, int64(1800), "spec", "ttlSecondsAfterFinished")
	return job
}

func RenderCanaryRolloutDeployment(spec contracts.RLRunSpec, policyVersion string, replicas int, draining bool) *unstructured.Unstructured {
	deployment := RenderRolloutDeploymentState(spec, policyVersion, replicas, draining)
	deployment.SetName(spec.Metadata.RunID + "-rollout-canary")
	labels := deployment.GetLabels()
	labels["rl.skyscale.dev/role"] = "rollout-canary"
	deployment.SetLabels(labels)
	selector := map[string]any{"rl.skyscale.dev/run": spec.Metadata.RunID, "rl.skyscale.dev/role": "rollout-canary"}
	_ = unstructured.SetNestedMap(deployment.Object, selector, "spec", "selector", "matchLabels")
	podLabels, _, _ := unstructured.NestedMap(deployment.Object, "spec", "template", "metadata", "labels")
	podLabels["rl.skyscale.dev/role"] = "rollout-canary"
	_ = unstructured.SetNestedMap(deployment.Object, podLabels, "spec", "template", "metadata", "labels")
	return deployment
}

func RenderCanaryRolloutService(spec contracts.RLRunSpec) *unstructured.Unstructured {
	service := RenderRolloutService(spec)
	service.SetName(spec.Metadata.RunID + "-rollout-canary")
	selector := map[string]any{"rl.skyscale.dev/run": spec.Metadata.RunID, "rl.skyscale.dev/role": "rollout-canary"}
	_ = unstructured.SetNestedMap(service.Object, selector, "spec", "selector")
	return service
}

func RenderTenantNamespace(spec contracts.RLRunSpec) *unstructured.Unstructured {
	name := NamespaceFor(spec.Metadata.TenantID, spec.Metadata.ProjectID)
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]any{
				"rl.skyscale.dev/tenant": spec.Metadata.TenantID, "rl.skyscale.dev/project": spec.Metadata.ProjectID,
				"pod-security.kubernetes.io/enforce": "restricted",
			},
		},
	}}
}

func RenderTenantQuota(spec contracts.RLRunSpec, maxGPUs int) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ResourceQuota", "metadata": sharedMetadata(spec, "skyscale-rl-quota"),
		"spec": map[string]any{"hard": map[string]any{
			"requests.nvidia.com/gpu": strconv.Itoa(maxGPUs), "limits.nvidia.com/gpu": strconv.Itoa(maxGPUs),
			"requests.cpu": "256", "requests.memory": "1Ti", "count/rayjobs.ray.io": "8", "count/jobs.batch": "32",
		}},
	}}
}

func RenderTenantServiceAccount(spec contracts.RLRunSpec) *unstructured.Unstructured {
	name := spec.Security.ServiceAccountName
	if name == "" {
		name = "skyscale-rl-runner"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "ServiceAccount", "metadata": sharedMetadata(spec, name),
		"automountServiceAccountToken": false,
	}}
}

func RenderTenantLimitRange(spec contracts.RLRunSpec) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "LimitRange", "metadata": sharedMetadata(spec, "skyscale-rl-defaults"),
		"spec": map[string]any{"limits": []any{map[string]any{
			"type": "Container", "defaultRequest": map[string]any{"cpu": "1", "memory": "2Gi"},
			"default": map[string]any{"cpu": "16", "memory": "128Gi"},
		}}},
	}}
}

func RenderTenantNetworkPolicy(spec contracts.RLRunSpec) *unstructured.Unstructured {
	controlPlaneSelector := map[string]any{
		"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "skyscale-system"}},
	}
	dnsSelector := map[string]any{
		"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "kube-system"}},
	}
	egress := []any{
		map[string]any{"to": []any{map[string]any{"podSelector": map[string]any{}}}},
		map[string]any{
			"to":    []any{dnsSelector},
			"ports": []any{map[string]any{"protocol": "UDP", "port": int64(53)}, map[string]any{"protocol": "TCP", "port": int64(53)}},
		},
		map[string]any{
			"to":    []any{controlPlaneSelector},
			"ports": []any{map[string]any{"protocol": "TCP", "port": int64(8080)}},
		},
	}
	for _, destination := range spec.Security.EgressAllowlist {
		if _, _, err := net.ParseCIDR(destination); err == nil {
			egress = append(egress, map[string]any{"to": []any{map[string]any{"ipBlock": map[string]any{"cidr": destination}}}})
		}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": sharedMetadata(spec, "skyscale-rl-isolation"),
		"spec": map[string]any{
			"podSelector": map[string]any{},
			"policyTypes": []any{"Ingress", "Egress"},
			"ingress": []any{map[string]any{"from": []any{
				map[string]any{"podSelector": map[string]any{}},
				map[string]any{"namespaceSelector": map[string]any{"matchLabels": map[string]any{"kubernetes.io/metadata.name": "skyscale-system"}}},
			}}},
			"egress": egress,
		},
	}}
}
