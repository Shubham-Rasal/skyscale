package dataplane

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ObjectBlobStore stores immutable payload shards in S3-compatible storage.
type ObjectBlobStore struct {
	client *minio.Client
	bucket string
}

func NewObjectBlobStore(client *minio.Client, bucket string) (*ObjectBlobStore, error) {
	if client == nil || bucket == "" {
		return nil, errors.New("object client and bucket are required")
	}
	return &ObjectBlobStore{client: client, bucket: bucket}, nil
}

func (s *ObjectBlobStore) PutIfAbsent(ctx context.Context, key string, value []byte, checksum string) error {
	stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		if stat.Metadata.Get("X-Amz-Meta-Sha256") != checksum {
			return ErrDuplicateConflict
		}
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.Code != "NoSuchKey" && response.Code != "NoSuchObject" && response.StatusCode != 404 {
		return err
	}
	_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(value), int64(len(value)), minio.PutObjectOptions{
		ContentType: "application/json", UserMetadata: map[string]string{"sha256": checksum},
	})
	return err
}

func (s *ObjectBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()
	return io.ReadAll(object)
}

func (s *ObjectBlobStore) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *ObjectBlobStore) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	signed, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}

type sampleRow struct {
	TenantID       string `gorm:"primaryKey;index:idx_sample_group_v2,priority:1"`
	RunID          string `gorm:"primaryKey;index:idx_sample_group_v2,priority:2"`
	SampleID       string `gorm:"primaryKey"`
	ProjectID      string `gorm:"index"`
	PromptGroupID  string `gorm:"index:idx_sample_group_v2,priority:3"`
	PolicyVersion  string `gorm:"index"`
	BlobKey        string
	Checksum       string
	State          string `gorm:"index"`
	CreatedAt      time.Time
	ExpiresAt      time.Time `gorm:"index"`
	LeaseID        string    `gorm:"index"`
	LeaseOwner     string
	LeaseExpiresAt time.Time `gorm:"index"`
}

// TableName intentionally uses a v2 table. The legacy table used sample_id as
// a global primary key and cannot be safely migrated in place while serving.
func (sampleRow) TableName() string { return "rl_grouped_samples_v2" }

type legacySampleRow struct {
	SampleID       string `gorm:"primaryKey"`
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

func (legacySampleRow) TableName() string { return "sample_rows" }

// SQLMetadataStore uses a transactional SQL database. PostgreSQL is the
// production target; SQLite is supported only for local development.
type SQLMetadataStore struct {
	db *gorm.DB
}

func NewSQLMetadataStore(db *gorm.DB) (*SQLMetadataStore, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	if err := db.AutoMigrate(&sampleRow{}); err != nil {
		return nil, err
	}
	if err := migrateLegacySamples(db); err != nil {
		return nil, err
	}
	return &SQLMetadataStore{db: db}, nil
}

func migrateLegacySamples(db *gorm.DB) error {
	if !db.Migrator().HasTable(&legacySampleRow{}) {
		return nil
	}
	var legacy []legacySampleRow
	if err := db.Find(&legacy).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, old := range legacy {
			parts := strings.Split(strings.TrimPrefix(old.BlobKey, "/"), "/")
			if len(parts) < 4 || parts[0] != "samples" || parts[1] == "" || parts[2] != old.RunID {
				return fmt.Errorf("cannot safely migrate legacy sample %s: blob key lacks tenant/run lineage", old.SampleID)
			}
			row := sampleRow{
				TenantID: parts[1], RunID: old.RunID, SampleID: old.SampleID,
				PromptGroupID: old.PromptGroupID, PolicyVersion: old.PolicyVersion,
				BlobKey: old.BlobKey, Checksum: old.Checksum, State: old.State,
				CreatedAt: old.CreatedAt, ExpiresAt: old.ExpiresAt,
				LeaseID: old.LeaseID, LeaseOwner: old.LeaseOwner, LeaseExpiresAt: old.LeaseExpiresAt,
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			var migrated sampleRow
			if err := tx.First(&migrated, "tenant_id = ? AND run_id = ? AND sample_id = ?", row.TenantID, row.RunID, row.SampleID).Error; err != nil {
				return err
			}
			if migrated.Checksum != row.Checksum {
				return fmt.Errorf("legacy sample %s conflicts with scoped migration target", old.SampleID)
			}
		}
		return nil
	})
}

func toRecord(row sampleRow) SampleRecord {
	return SampleRecord{
		TenantID: row.TenantID, ProjectID: row.ProjectID, SampleID: row.SampleID, RunID: row.RunID, PromptGroupID: row.PromptGroupID,
		PolicyVersion: row.PolicyVersion, BlobKey: row.BlobKey, Checksum: row.Checksum,
		State: row.State, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt,
		LeaseID: row.LeaseID, LeaseOwner: row.LeaseOwner, LeaseExpiresAt: row.LeaseExpiresAt,
	}
}

func toRow(record SampleRecord) sampleRow {
	return sampleRow{
		TenantID: record.TenantID, ProjectID: record.ProjectID, SampleID: record.SampleID, RunID: record.RunID, PromptGroupID: record.PromptGroupID,
		PolicyVersion: record.PolicyVersion, BlobKey: record.BlobKey, Checksum: record.Checksum,
		State: record.State, CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt,
		LeaseID: record.LeaseID, LeaseOwner: record.LeaseOwner, LeaseExpiresAt: record.LeaseExpiresAt,
	}
}

func (s *SQLMetadataStore) InsertSample(ctx context.Context, record SampleRecord) (SampleRecord, bool, error) {
	row := toRow(record)
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return SampleRecord{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		return record, true, nil
	}
	if err := s.db.WithContext(ctx).First(&row, "tenant_id = ? AND run_id = ? AND sample_id = ?", record.TenantID, record.RunID, record.SampleID).Error; err != nil {
		return SampleRecord{}, false, err
	}
	return toRecord(row), false, nil
}

func (s *SQLMetadataStore) CountAvailableGroups(ctx context.Context, tenantID, runID string) (int, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&sampleRow{}).Distinct("prompt_group_id").
		Where("tenant_id = ? AND run_id = ? AND state = ?", tenantID, runID, "available").Count(&count).Error
	return int(count), err
}

func (s *SQLMetadataStore) ClaimGroup(ctx context.Context, tenantID, runID string, expected int, leaseID, owner string, expires time.Time) (Lease, error) {
	var lease Lease
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []sampleRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("tenant_id = ? AND run_id = ? AND state = ?", tenantID, runID, "available").
			Order("prompt_group_id, sample_id").Find(&rows).Error; err != nil {
			return err
		}
		groups := map[string][]sampleRow{}
		for _, row := range rows {
			groups[row.PromptGroupID] = append(groups[row.PromptGroupID], row)
		}
		keys := make([]string, 0, len(groups))
		for key, values := range groups {
			if len(values) == expected {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return ErrNoCompleteGroup
		}
		selected := groups[keys[0]]
		ids := make([]string, len(selected))
		for i := range selected {
			ids[i] = selected[i].SampleID
		}
		result := tx.Model(&sampleRow{}).Where("tenant_id = ? AND run_id = ? AND sample_id IN ? AND state = ?", tenantID, runID, ids, "available").Updates(map[string]any{
			"state": "leased", "lease_id": leaseID, "lease_owner": owner, "lease_expires_at": expires,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(expected) {
			return gorm.ErrInvalidTransaction
		}
		records := make([]SampleRecord, len(selected))
		for i := range selected {
			selected[i].State, selected[i].LeaseID, selected[i].LeaseOwner, selected[i].LeaseExpiresAt = "leased", leaseID, owner, expires
			records[i] = toRecord(selected[i])
		}
		lease = Lease{ID: leaseID, Owner: owner, RunID: runID, PromptGroupID: keys[0], Samples: records, ExpiresAt: expires}
		return nil
	})
	return lease, err
}

func (s *SQLMetadataStore) FinishLease(ctx context.Context, leaseID, owner string, ack bool, reason string) error {
	state := "available"
	if ack {
		state = "consumed"
	} else if reason == "malformed sample" || strings.HasPrefix(reason, "quarantine:") {
		state = "quarantined"
	}
	result := s.db.WithContext(ctx).Model(&sampleRow{}).
		Where("lease_id = ? AND lease_owner = ? AND lease_expires_at > ?", leaseID, owner, time.Now()).
		Updates(map[string]any{"state": state, "lease_id": "", "lease_owner": "", "lease_expires_at": time.Time{}})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (s *SQLMetadataStore) ReleaseExpired(ctx context.Context, now time.Time) (int, error) {
	result := s.db.WithContext(ctx).Model(&sampleRow{}).
		Where("state = ? AND lease_expires_at <= ?", "leased", now).
		Updates(map[string]any{"state": "available", "lease_id": "", "lease_owner": "", "lease_expires_at": time.Time{}})
	return int(result.RowsAffected), result.Error
}

func (s *SQLMetadataStore) DeleteExpired(ctx context.Context, now time.Time) ([]SampleRecord, error) {
	var rows []sampleRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("state != ? AND expires_at <= ?", "leased", now).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Delete(&rows).Error
	})
	records := make([]SampleRecord, len(rows))
	for i := range rows {
		records[i] = toRecord(rows[i])
	}
	return records, err
}

func (s *SQLMetadataStore) ListRun(ctx context.Context, tenantID, runID string) ([]SampleRecord, error) {
	var rows []sampleRow
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND run_id = ?", tenantID, runID).Order("sample_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]SampleRecord, len(rows))
	for i := range rows {
		records[i] = toRecord(rows[i])
	}
	return records, nil
}

func (s *SQLMetadataStore) String() string {
	return fmt.Sprintf("sql-metadata-store(%s)", s.db.Name())
}
