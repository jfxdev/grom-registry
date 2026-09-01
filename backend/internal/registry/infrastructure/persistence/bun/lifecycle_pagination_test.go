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
)

func TestManifestPlatformBulkInsertAndOrderingAcrossDatabases(t *testing.T) {
	ctx := context.Background()
	for _, testCase := range []struct {
		name string
		url  func(*testing.T) string
	}{
		{name: "sqlite", url: func(t *testing.T) string { return "sqlite://" + filepath.Join(t.TempDir(), "platforms.db") }},
		{name: "postgres", url: func(t *testing.T) string {
			url := os.Getenv("GROM_TEST_POSTGRES_URL")
			if url == "" {
				t.Skip("GROM_TEST_POSTGRES_URL is not configured")
			}
			return url
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, kind, err := database.Open(ctx, testCase.url(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if err := database.Migrate(ctx, db, kind, 5*time.Second, slog.Default()); err != nil {
				t.Fatal(err)
			}
			store := New(db)
			projectID, repositoryID := foundation.NewID(), foundation.NewID()
			now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectID.String(), "platforms-"+projectID.String(), "Platforms", "test", now); err != nil {
				t.Fatal(err)
			}
			if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Profile: constants.RepositoryProfileUnknown, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = db.ExecContext(context.Background(), "DELETE FROM registry_manifest_platforms WHERE manifest_id IN (SELECT id FROM registry_manifests WHERE repository_id = ?)", repositoryID.String())
				_, _ = db.ExecContext(context.Background(), "DELETE FROM registry_manifests WHERE repository_id = ?", repositoryID.String())
				_, _ = db.ExecContext(context.Background(), "DELETE FROM registry_repositories WHERE id = ?", repositoryID.String())
				_, _ = db.ExecContext(context.Background(), "DELETE FROM projects WHERE id = ?", projectID.String())
			})
			observation := registrydomain.ManifestObservation{
				Digest: "sha256:index",
				Platforms: []registrydomain.ManifestPlatform{
					{OS: "linux", Architecture: "amd64", Digest: "sha256:two", CompressedSize: 2},
					{OS: "linux", Architecture: "amd64", Digest: "sha256:one", CompressedSize: 1},
				},
			}
			if err := store.UpsertManifestObservation(ctx, repositoryID, observation, now); err != nil {
				t.Fatal(err)
			}
			inventory, err := store.ListManifestInventory(ctx, repositoryID)
			if err != nil {
				t.Fatal(err)
			}
			if len(inventory) != 1 || len(inventory[0].Platforms) != 2 ||
				inventory[0].Platforms[0].Digest != "sha256:one" || inventory[0].Platforms[1].Digest != "sha256:two" {
				t.Fatalf("unexpected platform bulk insert or ordering: %#v", inventory)
			}
		})
	}
}

func TestRegistryHistoryKeysetPages(t *testing.T) {
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:registry-page-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	store := New(db)
	repositoryID, projectID := foundation.NewID(), foundation.NewID()
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectID.String(), "pagination", "Pagination", "test", now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Profile: constants.RepositoryProfileUnknown, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for i, digest := range []string{"sha256:one", "sha256:two", "sha256:three"} {
		at := now
		if i == 2 {
			at = now.Add(time.Minute)
		}
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: digest, MediaType: "application/vnd.oci.image.manifest.v1+json", Tag: "v" + string(rune('1'+i))}, at); err != nil {
			t.Fatal(err)
		}
		deletion := &registrydomain.ArtifactDeletion{ID: foundation.NewID(), RepositoryID: repositoryID, Digest: digest, AffectedTags: []string{"v" + string(rune('1'+i))}, ActorID: foundation.NewID(), Status: constants.ArtifactDeletionCompleted, StartedAt: at}
		if err := store.CreateArtifactDeletion(ctx, deletion); err != nil {
			t.Fatal(err)
		}
		previewID := foundation.NewID()
		if _, err := db.NewInsert().Model(&lifecyclePreviewModel{ID: previewID.String(), RepositoryID: repositoryID.String(), PolicySnapshotJSON: "[]", Status: constants.LifecyclePreviewReady, PolicyVersion: 0, EvaluatorVersion: 1, InventoryAt: at, CreatedBy: foundation.NewID().String(), CreatedAt: at, ExpiresAt: at.Add(time.Hour)}).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := db.NewInsert().Model(&lifecycleRunModel{ID: foundation.NewID().String(), PreviewID: previewID.String(), RepositoryID: repositoryID.String(), ActorID: foundation.NewID().String(), Reason: "test", Status: constants.LifecycleRunCompleted, StartedAt: at}).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	assertKeysetPage(t, func(request foundation.PageRequest) ([]string, string, error) {
		page, err := store.ListManifestInventoryPage(ctx, repositoryID, "", request)
		return manifestIDs(page.Items), page.NextCursor, err
	}, "inventory:"+repositoryID.String())
	assertKeysetPage(t, func(request foundation.PageRequest) ([]string, string, error) {
		page, err := store.ListArtifactDeletionsPage(ctx, repositoryID, request)
		return deletionIDs(page.Items), page.NextCursor, err
	}, "deletions:"+repositoryID.String())
	assertKeysetPage(t, func(request foundation.PageRequest) ([]string, string, error) {
		page, err := store.ListLifecycleRunsPage(ctx, repositoryID, request)
		return lifecycleRunIDs(page.Items), page.NextCursor, err
	}, "runs:"+repositoryID.String())
}

func TestListManifestInventoryPageUsesEmptyTagsArrayForUntaggedManifest(t *testing.T) {
	ctx := context.Background()
	db, kind, err := database.Open(ctx, "sqlite://file:untagged-inventory-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db, kind, time.Second, slog.Default()); err != nil {
		t.Fatal(err)
	}
	store := New(db)
	repositoryID, projectID := foundation.NewID(), foundation.NewID()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectID.String(), "untagged", "Untagged", "test", now); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: repositoryID, ProjectID: projectID, Name: "api", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Profile: constants.RepositoryProfileUnknown, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:untagged", Tag: "latest"}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkManifestDeleted(ctx, repositoryID, "sha256:untagged", now); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListManifestInventoryPage(ctx, repositoryID, "", foundation.PageRequest{Limit: 20, Scope: "inventory:untagged"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Tags == nil || len(page.Items[0].Tags) != 0 {
		t.Fatalf("untagged manifest must expose an empty tags array: %#v", page.Items)
	}
}

func assertKeysetPage(t *testing.T, list func(foundation.PageRequest) ([]string, string, error), scope string) {
	t.Helper()
	expected, _, err := list(foundation.PageRequest{Limit: 10, Scope: scope})
	if err != nil || len(expected) != 3 {
		t.Fatalf("unexpected complete page: ids=%v err=%v", expected, err)
	}
	first, cursor, err := list(foundation.PageRequest{Limit: 2, Scope: scope})
	if err != nil || len(first) != 2 || cursor == "" {
		t.Fatalf("unexpected first page: ids=%v cursor=%q err=%v", first, cursor, err)
	}
	second, next, err := list(foundation.PageRequest{Limit: 2, Scope: scope, Cursor: cursor})
	if err != nil || len(second) != 1 || next != "" {
		t.Fatalf("unexpected second page: ids=%v cursor=%q err=%v", second, next, err)
	}
	seen := make(map[string]bool, len(expected))
	for _, id := range append(first, second...) {
		if seen[id] {
			t.Fatalf("duplicate item %q across pages", id)
		}
		seen[id] = true
	}
	for _, id := range expected {
		if !seen[id] {
			t.Fatalf("missing item %q across pages", id)
		}
	}
	if _, _, err := list(foundation.PageRequest{Limit: 2, Scope: scope, Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("expected invalid cursor rejection")
	}
}

func manifestIDs(items []registrydomain.ManifestInventory) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID.String()
	}
	return ids
}
func deletionIDs(items []registrydomain.ArtifactDeletion) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID.String()
	}
	return ids
}
func lifecycleRunIDs(items []registrydomain.LifecycleRun) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID.String()
	}
	return ids
}
