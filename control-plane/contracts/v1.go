// Package contracts defines the durable, versioned API shared by the
// SkyScale control plane and the slime runtime.
package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

const APIVersion = "rl.skyscale.dev/v1alpha1"

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type ObjectMeta struct {
	TenantID  string            `json:"tenant_id"`
	ProjectID string            `json:"project_id"`
	RunID     string            `json:"run_id,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type ModelSpec struct {
	Source              string `json:"source"`
	Revision            string `json:"revision"`
	SHA256              string `json:"sha256,omitempty"`
	MegatronFormat      string `json:"megatron_format,omitempty"`
	TokenizerRevision   string `json:"tokenizer_revision,omitempty"`
	ArtifactManifestURI string `json:"artifact_manifest_uri,omitempty"`
}

type DataSpec struct {
	SourceURI          string `json:"source_uri"`
	Revision           string `json:"revision"`
	SHA256             string `json:"sha256,omitempty"`
	EnvironmentVersion string `json:"environment_version"`
	Seed               int64  `json:"seed"`
}

type RewardSpec struct {
	Name             string             `json:"name"`
	Version          string             `json:"version"`
	Endpoint         string             `json:"endpoint,omitempty"`
	SecretRef        string             `json:"secret_ref,omitempty"`
	Components       map[string]float64 `json:"components,omitempty"`
	MaxEvaluationSec int                `json:"max_evaluation_seconds,omitempty"`
}

type AlgorithmSpec struct {
	Name               string            `json:"name"`
	Strategy           string            `json:"strategy"`
	Arguments          map[string]string `json:"arguments,omitempty"`
	MaxPolicyLagSteps  int               `json:"max_policy_lag_steps"`
	MaxQueueAgeSeconds int               `json:"max_queue_age_seconds"`
	OffPolicyAction    string            `json:"off_policy_action"`
}

type ResourceSpec struct {
	CPU       string            `json:"cpu,omitempty"`
	Memory    string            `json:"memory,omitempty"`
	GPUs      int               `json:"gpus"`
	GPUType   string            `json:"gpu_type,omitempty"`
	Selectors map[string]string `json:"selectors,omitempty"`
}

type TrainerTopology struct {
	Nodes     int          `json:"nodes"`
	TP        int          `json:"tensor_parallel"`
	PP        int          `json:"pipeline_parallel"`
	CP        int          `json:"context_parallel"`
	Resources ResourceSpec `json:"resources"`
}

type RolloutTopology struct {
	Replicas       int          `json:"replicas"`
	MinReplicas    int          `json:"min_replicas"`
	MaxReplicas    int          `json:"max_replicas"`
	TP             int          `json:"tensor_parallel"`
	DP             int          `json:"data_parallel"`
	MemoryFraction float64      `json:"memory_fraction"`
	External       bool         `json:"external"`
	Resources      ResourceSpec `json:"resources"`
}

type TopologySpec struct {
	Mode          string          `json:"mode"`
	Trainer       TrainerTopology `json:"trainer"`
	Rollout       RolloutTopology `json:"rollout"`
	Queue         string          `json:"queue,omitempty"`
	PriorityClass string          `json:"priority_class,omitempty"`
}

type CheckpointPolicy struct {
	EverySteps        int    `json:"every_steps"`
	KeepLast          int    `json:"keep_last"`
	ResumeFrom        string `json:"resume_from,omitempty"`
	ServingFormat     string `json:"serving_format"`
	DeltaEnabled      bool   `json:"delta_enabled"`
	MinimumAcks       int    `json:"minimum_acknowledgements"`
	PublishTimeoutSec int    `json:"publish_timeout_seconds"`
}

type EvaluationGate struct {
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
}

type EvaluationPolicy struct {
	EverySteps    int              `json:"every_steps,omitempty"`
	SuiteURI      string           `json:"suite_uri,omitempty"`
	SuiteHash     string           `json:"suite_hash,omitempty"`
	Gates         []EvaluationGate `json:"gates,omitempty"`
	CanaryPercent int              `json:"canary_percent,omitempty"`
}

type RetentionPolicy struct {
	SampleTTLHours      int `json:"sample_ttl_hours"`
	CheckpointTTLHours  int `json:"checkpoint_ttl_hours"`
	QuarantineTTLHours  int `json:"quarantine_ttl_hours"`
	HighWatermarkGroups int `json:"high_watermark_groups"`
	LowWatermarkGroups  int `json:"low_watermark_groups"`
}

type SecuritySpec struct {
	ServiceAccountName string   `json:"service_account_name,omitempty"`
	ImageAllowlist     []string `json:"image_allowlist,omitempty"`
	SecretRefs         []string `json:"secret_refs,omitempty"`
	EgressAllowlist    []string `json:"egress_allowlist,omitempty"`
	EncryptionKeyRef   string   `json:"encryption_key_ref,omitempty"`
}

type ImageSpec struct {
	Slime  string `json:"slime"`
	SGLang string `json:"sglang,omitempty"`
	Digest string `json:"digest"`
}

type RLRunSpec struct {
	APIVersion string           `json:"api_version"`
	Kind       string           `json:"kind"`
	Metadata   ObjectMeta       `json:"metadata"`
	Backend    string           `json:"backend"`
	Model      ModelSpec        `json:"model"`
	Data       DataSpec         `json:"data"`
	Reward     RewardSpec       `json:"reward"`
	Algorithm  AlgorithmSpec    `json:"algorithm"`
	Topology   TopologySpec     `json:"topology"`
	Checkpoint CheckpointPolicy `json:"checkpoint"`
	Evaluation EvaluationPolicy `json:"evaluation"`
	Retention  RetentionPolicy  `json:"retention"`
	Security   SecuritySpec     `json:"security"`
	Image      ImageSpec        `json:"image"`
	CreatedAt  time.Time        `json:"created_at"`
}

type RunSnapshot struct {
	Spec      RLRunSpec `json:"spec"`
	SHA256    string    `json:"sha256"`
	Immutable bool      `json:"immutable"`
}

func DefaultRunSpec() RLRunSpec {
	return RLRunSpec{
		APIVersion: APIVersion,
		Kind:       "RLRun",
		Backend:    "slime",
		Model:      ModelSpec{Source: "Qwen/Qwen3-0.6B", Revision: "main", MegatronFormat: "torch_dist"},
		Data:       DataSpec{SourceURI: "skyscale://problems/default", Revision: "v1", EnvironmentVersion: "code-v1", Seed: 42},
		Reward:     RewardSpec{Name: "sandbox-tests", Version: "v1", MaxEvaluationSec: 120},
		Algorithm:  AlgorithmSpec{Name: "grpo", Strategy: "synchronous", MaxPolicyLagSteps: 0, MaxQueueAgeSeconds: 900, OffPolicyAction: "discard"},
		Topology: TopologySpec{
			Mode:    "colocated",
			Trainer: TrainerTopology{Nodes: 1, TP: 1, PP: 1, CP: 1, Resources: ResourceSpec{CPU: "8", Memory: "32Gi", GPUs: 1}},
			Rollout: RolloutTopology{Replicas: 1, MinReplicas: 1, MaxReplicas: 1, TP: 1, DP: 1, MemoryFraction: 0.35, Resources: ResourceSpec{GPUs: 1}},
		},
		Checkpoint: CheckpointPolicy{EverySteps: 1, KeepLast: 3, ServingFormat: "full", MinimumAcks: 1, PublishTimeoutSec: 600},
		Retention:  RetentionPolicy{SampleTTLHours: 168, CheckpointTTLHours: 720, QuarantineTTLHours: 168, HighWatermarkGroups: 1000, LowWatermarkGroups: 500},
		Security:   SecuritySpec{ImageAllowlist: []string{"ghcr.io/skyscale/"}},
		Image:      ImageSpec{Slime: "ghcr.io/skyscale/slime-runtime:latest"},
	}
}

func (s *RLRunSpec) Normalize() {
	if s.APIVersion == "" {
		s.APIVersion = APIVersion
	}
	if s.Kind == "" {
		s.Kind = "RLRun"
	}
	if s.Backend == "" {
		s.Backend = "skyscale"
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	sort.Strings(s.Security.ImageAllowlist)
	sort.Strings(s.Security.SecretRefs)
	sort.Strings(s.Security.EgressAllowlist)
}

func (s RLRunSpec) Validate() error {
	var problems []string
	if s.APIVersion != APIVersion {
		problems = append(problems, "api_version must be "+APIVersion)
	}
	if s.Kind != "RLRun" {
		problems = append(problems, "kind must be RLRun")
	}
	for name, value := range map[string]string{"tenant_id": s.Metadata.TenantID, "project_id": s.Metadata.ProjectID} {
		if len(value) == 0 || len(value) > 63 || !dnsLabel.MatchString(value) {
			problems = append(problems, name+" must be a DNS label")
		}
	}
	if s.Backend != "slime" && s.Backend != "skyscale" {
		problems = append(problems, "backend must be slime or skyscale")
	}
	if s.Model.Source == "" || s.Model.Revision == "" {
		problems = append(problems, "model source and immutable revision are required")
	}
	algorithms := map[string]bool{"grpo": true, "gspo": true, "cispo": true, "reinforce++": true, "ppo": true, "sft": true, "custom": true}
	if !algorithms[strings.ToLower(s.Algorithm.Name)] {
		problems = append(problems, "unsupported algorithm")
	}
	strategies := map[string]bool{"synchronous": true, "overlapped": true, "asynchronous": true}
	if !strategies[s.Algorithm.Strategy] {
		problems = append(problems, "invalid algorithm strategy")
	}
	actions := map[string]bool{"accept": true, "reweight": true, "quarantine": true, "discard": true}
	if !actions[s.Algorithm.OffPolicyAction] {
		problems = append(problems, "invalid off-policy action")
	}
	if s.Algorithm.MaxPolicyLagSteps < 0 || s.Algorithm.MaxQueueAgeSeconds < 0 {
		problems = append(problems, "policy lag and queue age bounds must be non-negative")
	}
	if s.Topology.Mode != "colocated" && s.Topology.Mode != "disaggregated" {
		problems = append(problems, "topology mode must be colocated or disaggregated")
	}
	if s.Topology.Trainer.Nodes < 1 || s.Topology.Trainer.TP < 1 || s.Topology.Trainer.PP < 1 || s.Topology.Trainer.CP < 1 {
		problems = append(problems, "trainer nodes and parallelism must be positive")
	}
	if s.Topology.Trainer.Resources.GPUs < 1 {
		problems = append(problems, "trainer requires at least one whole GPU")
	}
	r := s.Topology.Rollout
	if r.Replicas < 1 || r.MinReplicas < 0 || r.MaxReplicas < r.MinReplicas || r.Replicas < r.MinReplicas || r.Replicas > r.MaxReplicas {
		problems = append(problems, "invalid rollout replica bounds")
	}
	if r.MemoryFraction <= 0 || r.MemoryFraction >= 0.9 {
		problems = append(problems, "rollout memory_fraction must be > 0 and < 0.9")
	}
	if s.Topology.Mode == "colocated" && r.External {
		problems = append(problems, "colocated rollout cannot be external")
	}
	if s.Retention.LowWatermarkGroups < 0 || s.Retention.HighWatermarkGroups <= s.Retention.LowWatermarkGroups {
		problems = append(problems, "high watermark must exceed low watermark")
	}
	if s.Image.Slime == "" || (!strings.Contains(s.Image.Slime, "@sha256:") && s.Image.Digest == "") {
		problems = append(problems, "slime image and digest are required")
	}
	if s.Backend == "slime" && len(s.Security.ImageAllowlist) == 0 {
		problems = append(problems, "slime backend requires a non-empty image allowlist")
	}
	for _, candidate := range []string{s.Image.Slime, s.Image.SGLang} {
		if candidate == "" {
			continue
		}
		allowed := false
		for _, prefix := range s.Security.ImageAllowlist {
			if strings.HasPrefix(candidate, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			problems = append(problems, "runtime image is outside the image allowlist")
		}
	}
	for k := range s.Algorithm.Arguments {
		if k == "max_retries" {
			continue
		}
		if !strings.HasPrefix(k, "--") || strings.ContainsAny(k, " \t\n;&|`$") {
			problems = append(problems, "unsafe custom argument key")
			break
		}
	}
	for _, destination := range s.Security.EgressAllowlist {
		if _, _, err := net.ParseCIDR(destination); err != nil {
			problems = append(problems, "egress allowlist entries must be CIDRs")
			break
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func Snapshot(spec RLRunSpec) (RunSnapshot, error) {
	spec.Normalize()
	if err := spec.Validate(); err != nil {
		return RunSnapshot{}, err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return RunSnapshot{}, fmt.Errorf("marshal run snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	return RunSnapshot{Spec: spec, SHA256: hex.EncodeToString(sum[:]), Immutable: true}, nil
}

type PolicyVersion struct {
	ID            string    `json:"id"`
	RunID         string    `json:"run_id"`
	OptimizerStep int64     `json:"optimizer_step"`
	ParentVersion string    `json:"parent_version,omitempty"`
	CheckpointID  string    `json:"checkpoint_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type CheckpointRef struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	OptimizerStep int64  `json:"optimizer_step"`
	ResumeURI     string `json:"resume_uri"`
	ServingURI    string `json:"serving_uri,omitempty"`
	PolicyVersion string `json:"policy_version"`
}

type SampleEnvelope struct {
	APIVersion         string                     `json:"api_version"`
	TenantID           string                     `json:"tenant_id"`
	ProjectID          string                     `json:"project_id"`
	RunID              string                     `json:"run_id"`
	AttemptID          string                     `json:"attempt_id"`
	RolloutID          string                     `json:"rollout_id"`
	PromptGroupID      string                     `json:"prompt_group_id"`
	SampleID           string                     `json:"sample_id"`
	PromptTokenIDs     []int64                    `json:"prompt_token_ids"`
	ResponseTokenIDs   []int64                    `json:"response_token_ids"`
	ResponseStart      int                        `json:"response_start"`
	LossMask           []float32                  `json:"loss_mask"`
	BehaviorLogProbs   []float32                  `json:"behavior_log_probs,omitempty"`
	RewardComponents   map[string]float64         `json:"reward_components"`
	Status             string                     `json:"status"`
	PolicyVersion      string                     `json:"policy_version"`
	EnvironmentVersion string                     `json:"environment_version"`
	GeneratedAt        time.Time                  `json:"generated_at"`
	Metadata           map[string]json.RawMessage `json:"metadata,omitempty"`
}

func (s SampleEnvelope) Validate(maxTokens int) error {
	if s.APIVersion != APIVersion || s.TenantID == "" || s.ProjectID == "" || s.RunID == "" ||
		s.AttemptID == "" || s.RolloutID == "" || s.PromptGroupID == "" || s.SampleID == "" ||
		s.PolicyVersion == "" || s.EnvironmentVersion == "" || s.GeneratedAt.IsZero() {
		return errors.New("sample version and lineage identifiers are required")
	}
	total := len(s.PromptTokenIDs) + len(s.ResponseTokenIDs)
	if total == 0 || total > maxTokens {
		return fmt.Errorf("sample token count %d outside allowed range", total)
	}
	if s.ResponseStart != len(s.PromptTokenIDs) {
		return errors.New("response start must equal prompt token length")
	}
	if len(s.LossMask) != len(s.ResponseTokenIDs) {
		return errors.New("loss mask length must equal response token length")
	}
	if len(s.BehaviorLogProbs) != 0 && len(s.BehaviorLogProbs) != len(s.ResponseTokenIDs) {
		return errors.New("behavior log probability length must equal response token length")
	}
	return nil
}
