package bunstore

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

func TestStorageAccountingDeduplicatesByDigestAndRefreshesOnDeletion(t *testing.T) {
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "storage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	projectOne, projectTwo := foundation.NewID(), foundation.NewID()
	for _, p := range []foundation.ID{projectOne, projectTwo} {
		if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", p.String(), p.String(), p.String(), "test", now); err != nil {
			t.Fatal(err)
		}
	}
	first, second, third := foundation.NewID(), foundation.NewID(), foundation.NewID()
	for _, item := range []struct {
		id, project foundation.ID
		name        string
	}{{first, projectOne, "one"}, {second, projectOne, "two"}, {third, projectTwo, "three"}} {
		if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: item.id, ProjectID: item.project, Name: item.name, Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	shared := registrydomain.Descriptor{Digest: "sha256:shared", SizeBytes: 100, Role: "layer"}
	for _, repositoryID := range []foundation.ID{first, second, third} {
		observation := registrydomain.ManifestObservation{Digest: "sha256:manifest-" + repositoryID.String(), ManifestSize: 10, Tag: "latest", Descriptors: []registrydomain.Descriptor{shared}}
		if err := store.UpsertManifestObservation(ctx, repositoryID, observation, now); err != nil {
			t.Fatal(err)
		}
	}
	usage, err := store.StorageUsageForProject(ctx, projectOne)
	if err != nil || usage.AccountedBytes == nil || *usage.AccountedBytes != 120 {
		t.Fatalf("project deduplication: usage=%+v err=%v", usage, err)
	}
	other, err := store.StorageUsageForProject(ctx, projectTwo)
	if err != nil || other.AccountedBytes == nil || *other.AccountedBytes != 110 {
		t.Fatalf("cross-project usage: usage=%+v err=%v", other, err)
	}
	if err := store.MarkManifestDeleted(ctx, first, "sha256:manifest-"+first.String(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	usage, err = store.StorageUsageForProject(ctx, projectOne)
	if err != nil || usage.AccountedBytes == nil || *usage.AccountedBytes != 110 {
		t.Fatalf("deletion refresh: usage=%+v err=%v", usage, err)
	}
}

func TestStorageAccountingRejectsInconsistentDescriptorSize(t *testing.T) {
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "storage-inconsistent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Now().UTC()
	projectID, repositoryID := foundation.NewID(), foundation.NewID()
	if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectID.String(), projectID.String(), "test", "test", now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusActive, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:one", ManifestSize: 1, Tag: "one", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:shared", SizeBytes: 10, Role: "layer"}}}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:two", ManifestSize: 2, Tag: "two", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:shared", SizeBytes: 20, Role: "layer"}}}, now); err == nil {
		t.Fatal("expected immutable descriptor size failure")
	}
	usage, err := store.StorageUsageForProject(ctx, projectID)
	if err != nil || usage.AccountedBytes == nil || *usage.AccountedBytes != 11 {
		t.Fatalf("snapshot must survive failed observation: usage=%+v err=%v", usage, err)
	}
}
