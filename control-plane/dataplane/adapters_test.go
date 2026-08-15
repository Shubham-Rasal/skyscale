package dataplane

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSQLMetadataAtomicLease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLMetadataStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"a", "b"} {
		if _, inserted, err := store.InsertSample(ctx, SampleRecord{
			TenantID: "tenant", ProjectID: "project", SampleID: id, RunID: "run", PromptGroupID: "group", State: "available",
			CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil || !inserted {
			t.Fatalf("insert %s: inserted=%v err=%v", id, inserted, err)
		}
	}
	lease, err := store.ClaimGroup(ctx, "tenant", "run", 2, "lease", "trainer", time.Now().Add(time.Minute))
	if err != nil || len(lease.Samples) != 2 {
		t.Fatalf("claim: %#v %v", lease, err)
	}
	if _, err := store.ClaimGroup(ctx, "tenant", "run", 2, "other", "trainer", time.Now().Add(time.Minute)); err != ErrNoCompleteGroup {
		t.Fatalf("leased group was claimed twice: %v", err)
	}
}

func TestSQLMetadataCompositeSampleIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLMetadataStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, record := range []SampleRecord{
		{TenantID: "a", RunID: "run", SampleID: "same", State: "available"},
		{TenantID: "b", RunID: "run", SampleID: "same", State: "available"},
		{TenantID: "a", RunID: "other", SampleID: "same", State: "available"},
	} {
		record.ExpiresAt = time.Now().Add(time.Hour)
		if _, inserted, err := store.InsertSample(ctx, record); err != nil || !inserted {
			t.Fatalf("composite insert %#v: inserted=%v err=%v", record, inserted, err)
		}
	}
	var count int64
	if err := db.Table("rl_grouped_samples_v2").Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("expected 3 scoped rows, count=%d err=%v", count, err)
	}
}

func TestSQLMetadataMigratesLegacyTenantFromBlobLineage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&legacySampleRow{}); err != nil {
		t.Fatal(err)
	}
	legacy := legacySampleRow{
		SampleID: "sample", RunID: "run", PromptGroupID: "group",
		BlobKey: "samples/tenant/run/group/hash.json", Checksum: "hash", State: "available",
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLMetadataStore(db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListRun(context.Background(), "tenant", "run")
	if err != nil || len(rows) != 1 || rows[0].TenantID != "tenant" {
		t.Fatalf("legacy row not migrated with tenant scope: %#v err=%v", rows, err)
	}
}
