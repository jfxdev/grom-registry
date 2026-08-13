package bunstore

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func forStorageDatabases(t *testing.T, test func(*testing.T, *bun.DB)) {
	t.Helper()
	for _, testCase := range []struct {
		name string
		url  string
	}{
		{name: "sqlite", url: "sqlite://" + filepath.Join(t.TempDir(), "storage.db")},
		{name: "postgres", url: os.Getenv("GROM_TEST_POSTGRES_URL")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.url == "" {
				t.Skip("GROM_TEST_POSTGRES_URL is not configured")
			}
			ctx := context.Background()
			db, kind, err := database.Open(ctx, testCase.url)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
				t.Fatal(err)
			}
			test(t, db)
		})
	}
}

func seedProjectWithRepository(t *testing.T, ctx context.Context, db *bun.DB, store *Store, now time.Time) (foundation.ID, foundation.ID) {
	t.Helper()
	projectID, repositoryID := foundation.NewID(), foundation.NewID()
	if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectID.String(), projectID.String(), "test", "test", now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return projectID, repositoryID
}

func TestStorageAccountingDeduplicatesByDigestAndRefreshesOnDeletion(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
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
		usages, err := store.StorageUsageForProjects(ctx, []foundation.ID{projectOne, projectTwo, foundation.NewID()})
		if err != nil || usages[projectOne].AccountedBytes == nil || *usages[projectOne].AccountedBytes != 120 || usages[projectTwo].AccountedBytes == nil || *usages[projectTwo].AccountedBytes != 110 {
			t.Fatalf("batch project usage: usages=%+v err=%v", usages, err)
		}
		if err := store.MarkManifestDeleted(ctx, first, "sha256:manifest-"+first.String(), now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		usage, err = store.StorageUsageForProject(ctx, projectOne)
		if err != nil || usage.AccountedBytes == nil || *usage.AccountedBytes != 110 {
			t.Fatalf("deletion refresh: usage=%+v err=%v", usage, err)
		}
		var descriptorCount int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM registry_blob_descriptors WHERE digest = ?", "sha256:manifest-"+first.String()).Scan(&descriptorCount); err != nil {
			t.Fatal(err)
		}
		if descriptorCount != 0 {
			t.Fatalf("deleted manifest descriptor was not cleaned up: count=%d", descriptorCount)
		}
	})
}

func TestStorageAccountingRejectsInconsistentDescriptorSize(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Now().UTC()
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
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
	})
}

func TestProjectStorageAccountsEmptyRepositoryAsZero(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Now().UTC()
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: foundation.NewID(), ProjectID: projectID, Name: "empty", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:one", ManifestSize: 1, Tag: "latest", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:layer", SizeBytes: 10, Role: "layer"}}}, now); err != nil {
			t.Fatal(err)
		}
		usage, err := store.StorageUsageForProject(ctx, projectID)
		if err != nil || usage.Status != storageReady || usage.AccountedBytes == nil || *usage.AccountedBytes != 11 {
			t.Fatalf("empty repository must not make project storage pending: usage=%+v err=%v", usage, err)
		}
	})
}

func TestAtomicReconciliationRollsBackFactsAndSnapshots(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Now().UTC()
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		initial := registrydomain.ManifestObservation{Digest: "sha256:initial", ManifestSize: 1, Tag: "latest", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:shared", SizeBytes: 10, Role: "layer"}}}
		if err := store.UpsertManifestObservation(ctx, repositoryID, initial, now); err != nil {
			t.Fatal(err)
		}
		err := store.ReconcileManifestObservationsAtomically(ctx, repositoryID, []registrydomain.ManifestObservation{
			{Digest: "sha256:new", ManifestSize: 2, Tag: "new", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:new-layer", SizeBytes: 20, Role: "layer"}}},
			{Digest: "sha256:broken", ManifestSize: 3, Tag: "broken", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:shared", SizeBytes: 99, Role: "layer"}}},
		}, []string{"new", "broken"}, []string{"sha256:new", "sha256:broken"}, now.Add(time.Minute))
		if err == nil {
			t.Fatal("expected inconsistent descriptor failure")
		}
		usage, err := store.StorageUsageForProject(ctx, projectID)
		if err != nil || usage.AccountedBytes == nil || *usage.AccountedBytes != 11 || usage.Status != storageReady {
			t.Fatalf("old snapshot must survive failed reconciliation: usage=%+v err=%v", usage, err)
		}
		inventory, err := store.ListManifestInventory(ctx, repositoryID)
		if err != nil || len(inventory) != 1 || inventory[0].Digest != initial.Digest {
			t.Fatalf("failed reconciliation must not publish partial inventory: %#v err=%v", inventory, err)
		}
	})
}

func TestStorageRebuildsRestoreReadySnapshotsFromFacts(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Now().UTC()
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:one", ManifestSize: 1, Tag: "latest", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:layer", SizeBytes: 10, Role: "layer"}}}, now); err != nil {
			t.Fatal(err)
		}
		otherRepositoryID := foundation.NewID()
		if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: otherRepositoryID, ProjectID: projectID, Name: "worker", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertManifestObservation(ctx, otherRepositoryID, registrydomain.ManifestObservation{Digest: "sha256:two", ManifestSize: 1, Tag: "latest", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:other-layer", SizeBytes: 10, Role: "layer"}}}, now); err != nil {
			t.Fatal(err)
		}
		if err := store.MarkRepositoryStorageStale(ctx, repositoryID); err != nil {
			t.Fatal(err)
		}
		if err := store.RebuildRepositoryStorage(ctx, repositoryID, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		usage, err := store.StorageUsageForProject(ctx, projectID)
		if err != nil || usage.Status != storageReady || usage.AccountedBytes == nil || *usage.AccountedBytes != 22 {
			t.Fatalf("repository rebuild did not restore snapshot: usage=%+v err=%v", usage, err)
		}
		if err := store.MarkRepositoryStorageStale(ctx, repositoryID); err != nil {
			t.Fatal(err)
		}
		if err := store.RebuildProjectStorage(ctx, projectID, now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		usage, err = store.StorageUsageForProject(ctx, projectID)
		if err != nil || usage.Status != storageStale {
			t.Fatalf("project rebuild must preserve the stale repository state: usage=%+v err=%v", usage, err)
		}
		if err := store.RebuildAllStorage(ctx, now.Add(3*time.Minute)); err != nil {
			t.Fatal(err)
		}
		usage, err = store.StorageUsageForProject(ctx, projectID)
		if err != nil || usage.Status != storageReady {
			t.Fatalf("full rebuild did not refresh repository snapshots: usage=%+v err=%v", usage, err)
		}
		emptyProjectID := foundation.NewID()
		if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", emptyProjectID.String(), emptyProjectID.String(), "empty", "test", now); err != nil {
			t.Fatal(err)
		}
		if err := store.RebuildAllStorage(ctx, now.Add(4*time.Minute)); err != nil {
			t.Fatal(err)
		}
		empty, err := store.StorageUsageForProject(ctx, emptyProjectID)
		if err != nil || empty.Status != storageReady || empty.AccountedBytes == nil || *empty.AccountedBytes != 0 {
			t.Fatalf("global rebuild must account empty projects as zero: usage=%+v err=%v", empty, err)
		}
	})
}

func TestStorageSnapshotsRejectOlderRefreshes(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Now().UTC()
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:one", ManifestSize: 1, Tag: "latest", Descriptors: []registrydomain.Descriptor{{Digest: "sha256:layer", SizeBytes: 10, Role: "layer"}}}, now); err != nil {
			t.Fatal(err)
		}
		var initialVersion int64
		if err := db.QueryRowContext(ctx, "SELECT inventory_version FROM repository_storage_snapshots WHERE repository_id = ?", repositoryID.String()).Scan(&initialVersion); err != nil {
			t.Fatal(err)
		}
		if err := store.RebuildRepositoryStorage(ctx, repositoryID, now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		var refreshedVersion int64
		if err := db.QueryRowContext(ctx, "SELECT inventory_version FROM repository_storage_snapshots WHERE repository_id = ?", repositoryID.String()).Scan(&refreshedVersion); err != nil {
			t.Fatal(err)
		}
		if refreshedVersion != initialVersion {
			t.Fatalf("older refresh advanced snapshot version: got %d, want %d", refreshedVersion, initialVersion)
		}
	})
}
