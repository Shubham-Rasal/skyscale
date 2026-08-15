// Package dataplane implements the grouped slime sample data plane. Production
// deployments provide transactional metadata and object-store implementations;
// the in-memory implementation is deterministic and intended for tests/local use.
package dataplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bluequbit/faas/control-plane/contracts"
	"github.com/google/uuid"
)

var (
	ErrDuplicateConflict = errors.New("sample id already exists with different content")
	ErrBackpressure      = errors.New("sample high watermark reached")
	ErrNoCompleteGroup   = errors.New("no complete sample group available")
	ErrLeaseLost         = errors.New("lease is missing, expired, or owned by another consumer")
)

type BlobStore interface {
	PutIfAbsent(context.Context, string, []byte, string) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
	SignedURL(context.Context, string, time.Duration) (string, error)
}

type SampleRecord struct {
	TenantID       string
	ProjectID      string
	SampleID       string
	RunID          string
	PromptGroupID  string
	PolicyVersion  string
	BlobKey        string
	Checksum       string
	State          string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LeaseID        string
	LeaseOwner     string
	LeaseExpiresAt time.Time
}

type Lease struct {
	ID            string         `json:"id"`
	Owner         string         `json:"owner"`
	RunID         string         `json:"run_id"`
	PromptGroupID string         `json:"prompt_group_id"`
	Samples       []SampleRecord `json:"samples"`
	ExpiresAt     time.Time      `json:"expires_at"`
}

type MetadataStore interface {
	InsertSample(context.Context, SampleRecord) (SampleRecord, bool, error)
	CountAvailableGroups(context.Context, string, string) (int, error)
	ClaimGroup(context.Context, string, string, int, string, string, time.Time) (Lease, error)
	FinishLease(context.Context, string, string, bool, string) error
	ReleaseExpired(context.Context, time.Time) (int, error)
	DeleteExpired(context.Context, time.Time) ([]SampleRecord, error)
	ListRun(context.Context, string, string) ([]SampleRecord, error)
}

type Store struct {
	metadata      MetadataStore
	blobs         BlobStore
	maxTokens     int
	highWatermark int
	lowWatermark  int
	ttl           time.Duration
}

type Options struct {
	MaxTokens           int
	HighWatermarkGroups int
	LowWatermarkGroups  int
	Retention           time.Duration
}

func New(metadata MetadataStore, blobs BlobStore, options Options) (*Store, error) {
	if metadata == nil || blobs == nil {
		return nil, errors.New("metadata and blob stores are required")
	}
	if options.MaxTokens <= 0 || options.HighWatermarkGroups <= options.LowWatermarkGroups {
		return nil, errors.New("invalid data-plane limits")
	}
	return &Store{metadata: metadata, blobs: blobs, maxTokens: options.MaxTokens, highWatermark: options.HighWatermarkGroups, lowWatermark: options.LowWatermarkGroups, ttl: options.Retention}, nil
}

func (s *Store) Put(ctx context.Context, sample contracts.SampleEnvelope) (SampleRecord, bool, error) {
	if err := sample.Validate(s.maxTokens); err != nil {
		return SampleRecord{}, false, err
	}
	groups, err := s.metadata.CountAvailableGroups(ctx, sample.TenantID, sample.RunID)
	if err != nil {
		return SampleRecord{}, false, err
	}
	if groups >= s.highWatermark {
		return SampleRecord{}, false, ErrBackpressure
	}
	raw, err := json.Marshal(sample)
	if err != nil {
		return SampleRecord{}, false, err
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("samples/%s/%s/%s-%s.json", sample.TenantID, sample.RunID, sample.PromptGroupID, checksum)
	if err := s.blobs.PutIfAbsent(ctx, key, raw, checksum); err != nil {
		return SampleRecord{}, false, err
	}
	now := time.Now().UTC()
	record := SampleRecord{
		TenantID: sample.TenantID, ProjectID: sample.ProjectID, SampleID: sample.SampleID, RunID: sample.RunID, PromptGroupID: sample.PromptGroupID,
		PolicyVersion: sample.PolicyVersion, BlobKey: key, Checksum: checksum, State: "available",
		CreatedAt: now, ExpiresAt: now.Add(s.ttl),
	}
	stored, inserted, err := s.metadata.InsertSample(ctx, record)
	if err != nil {
		return SampleRecord{}, false, err
	}
	if !inserted && stored.Checksum != checksum {
		return SampleRecord{}, false, ErrDuplicateConflict
	}
	return stored, inserted, nil
}

func (s *Store) Claim(ctx context.Context, tenantID, runID string, expectedGroupSize int, owner string, duration time.Duration) (Lease, []contracts.SampleEnvelope, error) {
	if tenantID == "" || runID == "" || expectedGroupSize <= 0 || owner == "" || duration <= 0 {
		return Lease{}, nil, errors.New("tenant, run, group size, owner, and lease duration are required")
	}
	leaseID := "lease-" + uuid.NewString()
	lease, err := s.metadata.ClaimGroup(ctx, tenantID, runID, expectedGroupSize, leaseID, owner, time.Now().Add(duration))
	if err != nil {
		return Lease{}, nil, err
	}
	payloads := make([]contracts.SampleEnvelope, 0, len(lease.Samples))
	for _, record := range lease.Samples {
		raw, err := s.blobs.Get(ctx, record.BlobKey)
		if err != nil {
			_ = s.metadata.FinishLease(ctx, lease.ID, owner, false, "blob read failed")
			return Lease{}, nil, err
		}
		var sample contracts.SampleEnvelope
		if err := json.Unmarshal(raw, &sample); err != nil {
			_ = s.metadata.FinishLease(ctx, lease.ID, owner, false, "malformed sample")
			return Lease{}, nil, fmt.Errorf("decode sample %s: %w", record.SampleID, err)
		}
		if err := sample.Validate(s.maxTokens); err != nil {
			_ = s.metadata.FinishLease(ctx, lease.ID, owner, false, "malformed sample")
			return Lease{}, nil, fmt.Errorf("validate sample %s: %w", record.SampleID, err)
		}
		payloads = append(payloads, sample)
	}
	return lease, payloads, nil
}

func (s *Store) Ack(ctx context.Context, leaseID, owner string) error {
	return s.metadata.FinishLease(ctx, leaseID, owner, true, "")
}

func (s *Store) Nack(ctx context.Context, leaseID, owner, reason string) error {
	return s.metadata.FinishLease(ctx, leaseID, owner, false, reason)
}

func (s *Store) Backpressure(ctx context.Context, tenantID, runID string) (paused bool, resumeBelow int, err error) {
	groups, err := s.metadata.CountAvailableGroups(ctx, tenantID, runID)
	return groups >= s.highWatermark, s.lowWatermark, err
}

func (s *Store) Sweep(ctx context.Context, now time.Time) (released, deleted int, err error) {
	released, err = s.metadata.ReleaseExpired(ctx, now)
	if err != nil {
		return
	}
	records, err := s.metadata.DeleteExpired(ctx, now)
	if err != nil {
		return released, 0, err
	}
	for _, record := range records {
		if deleteErr := s.blobs.Delete(ctx, record.BlobKey); deleteErr != nil {
			return released, deleted, deleteErr
		}
		deleted++
	}
	return
}

func (s *Store) Export(ctx context.Context, tenantID, runID string) ([]contracts.SampleEnvelope, error) {
	records, err := s.metadata.ListRun(ctx, tenantID, runID)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.SampleEnvelope, 0, len(records))
	for _, record := range records {
		raw, err := s.blobs.Get(ctx, record.BlobKey)
		if err != nil {
			return nil, err
		}
		var sample contracts.SampleEnvelope
		if err := json.Unmarshal(raw, &sample); err != nil {
			return nil, err
		}
		out = append(out, sample)
	}
	return out, nil
}

type MemoryBlobStore struct {
	mu        sync.RWMutex
	data      map[string][]byte
	checksums map[string]string
}

func NewMemoryBlobStore() *MemoryBlobStore {
	return &MemoryBlobStore{data: map[string][]byte{}, checksums: map[string]string{}}
}

func (m *MemoryBlobStore) PutIfAbsent(_ context.Context, key string, value []byte, checksum string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.checksums[key]; ok && existing != checksum {
		return ErrDuplicateConflict
	}
	if _, ok := m.data[key]; !ok {
		m.data[key] = append([]byte(nil), value...)
		m.checksums[key] = checksum
	}
	return nil
}

func (m *MemoryBlobStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.data[key]
	if !ok {
		return nil, errors.New("blob not found")
	}
	return append([]byte(nil), value...), nil
}

func (m *MemoryBlobStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	delete(m.checksums, key)
	return nil
}

func (m *MemoryBlobStore) SignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		return "", errors.New("positive signed URL TTL required")
	}
	return "memory://signed/" + key, nil
}

type MemoryMetadataStore struct {
	mu      sync.Mutex
	samples map[string]SampleRecord
}

func sampleKey(tenantID, runID, sampleID string) string {
	return tenantID + "\x00" + runID + "\x00" + sampleID
}

func NewMemoryMetadataStore() *MemoryMetadataStore {
	return &MemoryMetadataStore{samples: map[string]SampleRecord{}}
}

func (m *MemoryMetadataStore) InsertSample(_ context.Context, record SampleRecord) (SampleRecord, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := sampleKey(record.TenantID, record.RunID, record.SampleID)
	if existing, ok := m.samples[key]; ok {
		return existing, false, nil
	}
	m.samples[key] = record
	return record, true, nil
}

func (m *MemoryMetadataStore) CountAvailableGroups(_ context.Context, tenantID, runID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	groups := map[string]bool{}
	for _, record := range m.samples {
		if record.TenantID == tenantID && record.RunID == runID && record.State == "available" {
			groups[record.PromptGroupID] = true
		}
	}
	return len(groups), nil
}

func (m *MemoryMetadataStore) ClaimGroup(_ context.Context, tenantID, runID string, expected int, leaseID, owner string, expires time.Time) (Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	byGroup := map[string][]SampleRecord{}
	for _, record := range m.samples {
		if record.TenantID == tenantID && record.RunID == runID && record.State == "available" {
			byGroup[record.PromptGroupID] = append(byGroup[record.PromptGroupID], record)
		}
	}
	keys := make([]string, 0, len(byGroup))
	for group, records := range byGroup {
		if len(records) == expected {
			keys = append(keys, group)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return Lease{}, ErrNoCompleteGroup
	}
	group := keys[0]
	records := byGroup[group]
	sort.Slice(records, func(i, j int) bool { return records[i].SampleID < records[j].SampleID })
	for i := range records {
		records[i].State, records[i].LeaseID, records[i].LeaseOwner, records[i].LeaseExpiresAt = "leased", leaseID, owner, expires
		m.samples[sampleKey(records[i].TenantID, records[i].RunID, records[i].SampleID)] = records[i]
	}
	return Lease{ID: leaseID, Owner: owner, RunID: runID, PromptGroupID: group, Samples: records, ExpiresAt: expires}, nil
}

func (m *MemoryMetadataStore) FinishLease(_ context.Context, leaseID, owner string, ack bool, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for id, record := range m.samples {
		if record.LeaseID != leaseID {
			continue
		}
		if record.LeaseOwner != owner || time.Now().After(record.LeaseExpiresAt) {
			return ErrLeaseLost
		}
		found = true
		if ack {
			record.State = "consumed"
		} else if reason == "malformed sample" || strings.HasPrefix(reason, "quarantine:") {
			record.State = "quarantined"
		} else {
			record.State = "available"
		}
		record.LeaseID, record.LeaseOwner = "", ""
		record.LeaseExpiresAt = time.Time{}
		m.samples[id] = record
	}
	if !found {
		return ErrLeaseLost
	}
	return nil
}

func (m *MemoryMetadataStore) ReleaseExpired(_ context.Context, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for id, record := range m.samples {
		if record.State == "leased" && !record.LeaseExpiresAt.After(now) {
			record.State, record.LeaseID, record.LeaseOwner, record.LeaseExpiresAt = "available", "", "", time.Time{}
			m.samples[id] = record
			count++
		}
	}
	return count, nil
}

func (m *MemoryMetadataStore) DeleteExpired(_ context.Context, now time.Time) ([]SampleRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deleted []SampleRecord
	for id, record := range m.samples {
		if record.State != "leased" && !record.ExpiresAt.After(now) {
			deleted = append(deleted, record)
			delete(m.samples, id)
		}
	}
	return deleted, nil
}

func (m *MemoryMetadataStore) ListRun(_ context.Context, tenantID, runID string) ([]SampleRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []SampleRecord
	for _, record := range m.samples {
		if record.TenantID == tenantID && record.RunID == runID {
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SampleID < out[j].SampleID })
	return out, nil
}
