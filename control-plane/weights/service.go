package weights

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

type TensorLayout struct {
	Name  string  `json:"name"`
	Shape []int64 `json:"shape"`
	DType string  `json:"dtype"`
	Shard string  `json:"shard"`
}

type Manifest struct {
	APIVersion        string            `json:"api_version"`
	Version           string            `json:"version"`
	RunID             string            `json:"run_id"`
	BaseCheckpoint    string            `json:"base_checkpoint"`
	ParentVersion     string            `json:"parent_version,omitempty"`
	Format            string            `json:"format"`
	DType             string            `json:"dtype"`
	Tensors           []TensorLayout    `json:"tensors"`
	Checksums         map[string]string `json:"checksums"`
	ByteSize          int64             `json:"byte_size"`
	OptimizerStep     int64             `json:"optimizer_step"`
	Compatibility     map[string]string `json:"compatibility"`
	ArtifactURI       string            `json:"artifact_uri"`
	UploadComplete    bool              `json:"upload_complete"`
	ManifestCommitted bool              `json:"manifest_committed"`
	Verified          bool              `json:"verified"`
	CreatedAt         time.Time         `json:"created_at"`
}

func (m Manifest) Validate() error {
	if m.APIVersion != "rl.skyscale.dev/v1alpha1" || m.Version == "" || m.RunID == "" || m.BaseCheckpoint == "" {
		return errors.New("version, run, base checkpoint, and API version are required")
	}
	if m.Format != "full" && m.Format != "delta" {
		return errors.New("format must be full or delta")
	}
	if m.Format == "delta" && m.ParentVersion == "" {
		return errors.New("delta manifest requires parent version")
	}
	if m.ByteSize <= 0 || m.ArtifactURI == "" || len(m.Checksums) == 0 {
		return errors.New("artifact URI, size, and checksums are required")
	}
	return nil
}

type EngineState struct {
	EngineID        string    `json:"engine_id"`
	DesiredVersion  string    `json:"desired_version"`
	CurrentVersion  string    `json:"current_version"`
	LastGoodVersion string    `json:"last_good_version"`
	AcknowledgedAt  time.Time `json:"acknowledged_at,omitempty"`
	Error           string    `json:"error,omitempty"`
	Draining        bool      `json:"draining"`
}

type StateRepository interface {
	LoadWeightState(context.Context, string) ([]byte, error)
	SaveWeightState(context.Context, string, []byte) error
}

type ArtifactStore interface {
	Fetch(context.Context, string) ([]byte, error)
	PutImmutable(context.Context, string, []byte, string) error
	Delete(context.Context, string) error
}

type DurablePublication struct {
	Manifest        Manifest             `json:"manifest"`
	Required        map[string]bool      `json:"required_engines"`
	Acks            map[string]time.Time `json:"acknowledgements"`
	State           string               `json:"state"`
	Deadline        time.Time            `json:"deadline"`
	MaterializedURI string               `json:"materialized_uri,omitempty"`
	ArtifactSHA256  string               `json:"artifact_sha256"`
	ResultSHA256    string               `json:"result_sha256"`
}

type RunState struct {
	RunID        string                         `json:"run_id"`
	Publications map[string]*DurablePublication `json:"publications"`
	Engines      map[string]*EngineState        `json:"engines"`
	Active       string                         `json:"active,omitempty"`
	LastGood     string                         `json:"last_good,omitempty"`
	UpdatedAt    time.Time                      `json:"updated_at"`
}

type EngineInstruction struct {
	DesiredVersion  string    `json:"desired_version"`
	CurrentVersion  string    `json:"current_version"`
	LastGoodVersion string    `json:"last_good_version"`
	Draining        bool      `json:"draining"`
	Manifest        *Manifest `json:"manifest,omitempty"`
	ArtifactURI     string    `json:"artifact_uri,omitempty"`
	SHA256          string    `json:"sha256,omitempty"`
}

type Service struct {
	repository StateRepository
	artifacts  ArtifactStore
	mu         sync.Mutex
}

func NewService(repository StateRepository, artifacts ArtifactStore) (*Service, error) {
	if repository == nil || artifacts == nil {
		return nil, errors.New("weight state repository and artifact store are required")
	}
	return &Service{repository: repository, artifacts: artifacts}, nil
}

func (s *Service) load(ctx context.Context, runID string) (RunState, error) {
	raw, err := s.repository.LoadWeightState(ctx, runID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RunState{RunID: runID, Publications: map[string]*DurablePublication{}, Engines: map[string]*EngineState{}}, nil
	}
	if err != nil {
		return RunState{}, err
	}
	var state RunState
	if err := json.Unmarshal(raw, &state); err != nil {
		return RunState{}, fmt.Errorf("decode durable weight state: %w", err)
	}
	if state.RunID != runID {
		return RunState{}, errors.New("durable weight state run mismatch")
	}
	if state.Publications == nil {
		state.Publications = map[string]*DurablePublication{}
	}
	if state.Engines == nil {
		state.Engines = map[string]*EngineState{}
	}
	return state, nil
}

func (s *Service) save(ctx context.Context, state RunState) error {
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.repository.SaveWeightState(ctx, state.RunID, raw)
}

func (s *Service) RegisterEngine(ctx context.Context, runID, engineID, currentVersion string) error {
	if runID == "" || engineID == "" {
		return errors.New("run and engine identifiers are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return err
	}
	if engine := state.Engines[engineID]; engine != nil {
		if engine.CurrentVersion == "" && currentVersion != "" {
			engine.CurrentVersion, engine.LastGoodVersion = currentVersion, currentVersion
		}
	} else {
		state.Engines[engineID] = &EngineState{EngineID: engineID, CurrentVersion: currentVersion, LastGoodVersion: currentVersion}
	}
	return s.save(ctx, state)
}

func checksum(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (s *Service) Publish(ctx context.Context, manifest Manifest, requiredEngines []string, timeout time.Duration) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if timeout <= 0 {
		return errors.New("positive acknowledgement timeout required")
	}
	artifact, err := s.artifacts.Fetch(ctx, manifest.ArtifactURI)
	if err != nil {
		return fmt.Errorf("fetch weight artifact: %w", err)
	}
	artifactHash := checksum(artifact)
	if manifest.Checksums["artifact"] != artifactHash {
		return errors.New("weight artifact checksum mismatch")
	}
	// These are service-derived facts, never trusted client assertions.
	manifest.UploadComplete, manifest.ManifestCommitted, manifest.Verified = true, true, true
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, manifest.RunID)
	if err != nil {
		return err
	}
	if existing := state.Publications[manifest.Version]; existing != nil {
		if existing.ArtifactSHA256 == artifactHash {
			return nil
		}
		return errors.New("weight version already exists with different content")
	}
	if manifest.Format == "delta" {
		parent := state.Publications[manifest.ParentVersion]
		if parent == nil || parent.State == "failed" || parent.State == "garbage-collected" {
			return fmt.Errorf("delta parent %s is unavailable", manifest.ParentVersion)
		}
		if manifest.Checksums["result"] == "" {
			return errors.New("delta manifest requires result checksum")
		}
	}
	required := make(map[string]bool, len(requiredEngines))
	for _, engineID := range requiredEngines {
		engine := state.Engines[engineID]
		if engine == nil {
			return fmt.Errorf("required engine %s is not registered for run %s", engineID, manifest.RunID)
		}
		required[engineID] = true
		engine.DesiredVersion, engine.Draining = manifest.Version, true
	}
	state.Publications[manifest.Version] = &DurablePublication{
		Manifest: manifest, Required: required, Acks: map[string]time.Time{},
		State: "published", Deadline: time.Now().Add(timeout), ArtifactSHA256: artifactHash,
		ResultSHA256: manifest.Checksums["result"],
	}
	return s.save(ctx, state)
}

func applyXORDelta(parent, delta []byte) ([]byte, error) {
	if len(parent) != len(delta) {
		return nil, errors.New("delta and parent artifact sizes differ")
	}
	result := make([]byte, len(parent))
	for i := range parent {
		result[i] = parent[i] ^ delta[i]
	}
	return result, nil
}

func (s *Service) materializeLocked(ctx context.Context, state *RunState, version string, visiting map[string]bool) ([]byte, error) {
	if visiting[version] {
		return nil, errors.New("weight parent cycle detected")
	}
	publication := state.Publications[version]
	if publication == nil {
		return nil, fmt.Errorf("weight version %s not found", version)
	}
	if publication.MaterializedURI != "" {
		payload, err := s.artifacts.Fetch(ctx, publication.MaterializedURI)
		if err == nil && checksum(payload) == publication.ResultSHA256 {
			return payload, nil
		}
	}
	artifact, err := s.artifacts.Fetch(ctx, publication.Manifest.ArtifactURI)
	if err != nil {
		return nil, err
	}
	if checksum(artifact) != publication.ArtifactSHA256 {
		return nil, errors.New("stored weight artifact checksum mismatch")
	}
	result := artifact
	if publication.Manifest.Format == "delta" {
		visiting[version] = true
		parent, err := s.materializeLocked(ctx, state, publication.Manifest.ParentVersion, visiting)
		delete(visiting, version)
		if err != nil {
			return nil, err
		}
		result, err = applyXORDelta(parent, artifact)
		if err != nil {
			return nil, err
		}
	}
	expected := publication.Manifest.Checksums["result"]
	if expected == "" {
		expected = publication.ArtifactSHA256
	}
	if checksum(result) != expected {
		return nil, errors.New("materialized weight checksum mismatch")
	}
	uri := fmt.Sprintf("weights/%s/materialized/%s.bin", state.RunID, version)
	if err := s.artifacts.PutImmutable(ctx, uri, result, expected); err != nil {
		return nil, err
	}
	publication.MaterializedURI, publication.ResultSHA256, publication.State = uri, expected, "ready"
	return result, nil
}

func (s *Service) Materialize(ctx context.Context, runID, version string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return "", "", err
	}
	if _, err := s.materializeLocked(ctx, &state, version, map[string]bool{}); err != nil {
		if publication := state.Publications[version]; publication != nil {
			publication.State = "failed"
		}
		_ = s.save(ctx, state)
		return "", "", err
	}
	publication := state.Publications[version]
	if err := s.save(ctx, state); err != nil {
		return "", "", err
	}
	return publication.MaterializedURI, publication.ResultSHA256, nil
}

func (s *Service) Acknowledge(ctx context.Context, runID, engineID, version, appliedChecksum string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return false, err
	}
	publication, engine := state.Publications[version], state.Engines[engineID]
	if publication == nil || engine == nil {
		return false, errors.New("publication or required engine not found in run")
	}
	if state.Active == version && engine.DesiredVersion == version {
		if appliedChecksum != publication.ResultSHA256 {
			engine.Draining, engine.Error = true, "rollback checksum mismatch"
			_ = s.save(ctx, state)
			return false, errors.New(engine.Error)
		}
		engine.CurrentVersion, engine.Draining, engine.Error, engine.AcknowledgedAt = version, false, "", time.Now().UTC()
		return true, s.save(ctx, state)
	}
	if !publication.Required[engineID] {
		return false, errors.New("engine is not required for publication")
	}
	if publication.State != "ready" && publication.State != "activating" {
		return false, errors.New("weight version is not materialized and ready")
	}
	if time.Now().After(publication.Deadline) {
		publication.State, engine.Draining, engine.Error = "failed", true, "acknowledgement deadline exceeded"
		_ = s.save(ctx, state)
		return false, errors.New(engine.Error)
	}
	if appliedChecksum != publication.ResultSHA256 {
		engine.Draining, engine.Error = true, "applied checksum mismatch"
		_ = s.save(ctx, state)
		return false, errors.New(engine.Error)
	}
	publication.State = "activating"
	publication.Acks[engineID] = time.Now().UTC()
	engine.CurrentVersion, engine.AcknowledgedAt, engine.Error = version, time.Now().UTC(), ""
	if len(publication.Acks) != len(publication.Required) {
		return false, s.save(ctx, state)
	}
	if previous := state.Publications[state.Active]; previous != nil && state.Active != version {
		previous.State = "superseded"
	}
	publication.State, state.Active = "active", version
	for id := range publication.Required {
		state.Engines[id].DesiredVersion = version
		state.Engines[id].Draining = false
	}
	if state.LastGood == "" {
		state.LastGood = version
	}
	return true, s.save(ctx, state)
}

func (s *Service) MarkGood(ctx context.Context, runID, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return err
	}
	if state.Active != version {
		return errors.New("only the active version can be promoted")
	}
	state.LastGood = version
	for _, engine := range state.Engines {
		if engine.CurrentVersion == version {
			engine.LastGoodVersion = version
		}
	}
	return s.save(ctx, state)
}

func (s *Service) Rollback(ctx context.Context, runID, reason string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return "", err
	}
	if state.LastGood == "" {
		return "", errors.New("no last-good version")
	}
	if active := state.Publications[state.Active]; active != nil {
		active.State = "rolled_back"
	}
	state.Active = state.LastGood
	for _, engine := range state.Engines {
		engine.DesiredVersion, engine.Error, engine.Draining = state.LastGood, reason, true
	}
	return state.LastGood, s.save(ctx, state)
}

func (s *Service) Expire(ctx context.Context, runID string, now time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return nil, err
	}
	var expired []string
	for version, publication := range state.Publications {
		if (publication.State == "published" || publication.State == "ready" || publication.State == "activating") && !now.Before(publication.Deadline) {
			publication.State = "failed"
			expired = append(expired, version)
			for engineID := range publication.Required {
				if _, ok := publication.Acks[engineID]; !ok {
					state.Engines[engineID].Draining = true
					state.Engines[engineID].Error = "weight acknowledgement timeout"
					if state.Active != "" {
						state.Engines[engineID].DesiredVersion = state.Active
					} else {
						state.Engines[engineID].DesiredVersion = state.LastGood
					}
				}
			}
		}
	}
	sort.Strings(expired)
	return expired, s.save(ctx, state)
}

func (s *Service) GarbageCollect(ctx context.Context, runID string, retainNewest int) ([]Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	var retainChain func(string)
	retainChain = func(version string) {
		if version == "" || keep[version] {
			return
		}
		keep[version] = true
		if publication := state.Publications[version]; publication != nil {
			retainChain(publication.Manifest.ParentVersion)
		}
	}
	retainChain(state.Active)
	retainChain(state.LastGood)
	for _, engine := range state.Engines {
		retainChain(engine.DesiredVersion)
		retainChain(engine.CurrentVersion)
		retainChain(engine.LastGoodVersion)
	}
	all := make([]*DurablePublication, 0, len(state.Publications))
	for _, publication := range state.Publications {
		all = append(all, publication)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Manifest.OptimizerStep > all[j].Manifest.OptimizerStep })
	for i := 0; i < retainNewest && i < len(all); i++ {
		retainChain(all[i].Manifest.Version)
	}
	var removed []Manifest
	for version, publication := range state.Publications {
		if keep[version] || publication.State == "published" || publication.State == "ready" || publication.State == "activating" {
			continue
		}
		if publication.MaterializedURI != "" {
			if err := s.artifacts.Delete(ctx, publication.MaterializedURI); err != nil {
				return nil, err
			}
		}
		if err := s.artifacts.Delete(ctx, publication.Manifest.ArtifactURI); err != nil {
			return nil, err
		}
		publication.State = "garbage-collected"
		removed = append(removed, publication.Manifest)
		delete(state.Publications, version)
	}
	return removed, s.save(ctx, state)
}

func (s *Service) State(ctx context.Context, runID string) (RunState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(ctx, runID)
}

func (s *Service) EngineInstruction(ctx context.Context, runID, engineID string) (EngineInstruction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return EngineInstruction{}, err
	}
	engine := state.Engines[engineID]
	if engine == nil {
		return EngineInstruction{}, errors.New("engine not registered")
	}
	instruction := EngineInstruction{
		DesiredVersion: engine.DesiredVersion, CurrentVersion: engine.CurrentVersion,
		LastGoodVersion: engine.LastGoodVersion, Draining: engine.Draining,
	}
	if publication := state.Publications[engine.DesiredVersion]; publication != nil {
		manifest := publication.Manifest
		instruction.Manifest, instruction.ArtifactURI, instruction.SHA256 = &manifest, publication.MaterializedURI, publication.ResultSHA256
	}
	return instruction, nil
}

func (s *Service) MaterializedArtifact(ctx context.Context, runID, version string) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load(ctx, runID)
	if err != nil {
		return nil, "", err
	}
	publication := state.Publications[version]
	if publication == nil || publication.MaterializedURI == "" || publication.State == "failed" {
		return nil, "", errors.New("materialized weight version is unavailable")
	}
	payload, err := s.artifacts.Fetch(ctx, publication.MaterializedURI)
	if err != nil {
		return nil, "", err
	}
	if checksum(payload) != publication.ResultSHA256 {
		return nil, "", errors.New("materialized artifact checksum mismatch")
	}
	return payload, publication.ResultSHA256, nil
}

type MemoryRepository struct {
	mu   sync.Mutex
	data map[string][]byte
}

func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{data: map[string][]byte{}} }

func (m *MemoryRepository) LoadWeightState(_ context.Context, runID string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, ok := m.data[runID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return append([]byte(nil), raw...), nil
}

func (m *MemoryRepository) SaveWeightState(_ context.Context, runID string, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[runID] = append([]byte(nil), raw...)
	return nil
}

type MemoryArtifactStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func NewMemoryArtifactStore() *MemoryArtifactStore {
	return &MemoryArtifactStore{data: map[string][]byte{}}
}

func (m *MemoryArtifactStore) Fetch(_ context.Context, uri string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	payload, ok := m.data[uri]
	if !ok {
		return nil, errors.New("artifact not found")
	}
	return append([]byte(nil), payload...), nil
}

func (m *MemoryArtifactStore) PutImmutable(_ context.Context, uri string, payload []byte, expectedChecksum string) error {
	if checksum(payload) != expectedChecksum {
		return errors.New("artifact checksum mismatch")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.data[uri]; ok && !bytes.Equal(existing, payload) {
		return errors.New("immutable artifact conflict")
	}
	m.data[uri] = append([]byte(nil), payload...)
	return nil
}

func (m *MemoryArtifactStore) Delete(_ context.Context, uri string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, uri)
	return nil
}
