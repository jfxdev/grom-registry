package bunstore

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	"github.com/jfxdev/grom/backend/internal/platform/database"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

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
		at := now.Add(time.Duration(i) * time.Minute)
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

	assertKeysetPage(t, func(request foundation.PageRequest) (int, string, error) {
		page, err := store.ListManifestInventoryPage(ctx, repositoryID, request)
		return len(page.Items), page.NextCursor, err
	}, "inventory:"+repositoryID.String())
	assertKeysetPage(t, func(request foundation.PageRequest) (int, string, error) {
		page, err := store.ListArtifactDeletionsPage(ctx, repositoryID, request)
		return len(page.Items), page.NextCursor, err
	}, "deletions:"+repositoryID.String())
	assertKeysetPage(t, func(request foundation.PageRequest) (int, string, error) {
		page, err := store.ListLifecycleRunsPage(ctx, repositoryID, request)
		return len(page.Items), page.NextCursor, err
	}, "runs:"+repositoryID.String())
}

func assertKeysetPage(t *testing.T, list func(foundation.PageRequest) (int, string, error), scope string) {
	t.Helper()
	firstCount, cursor, err := list(foundation.PageRequest{Limit: 2, Scope: scope})
	if err != nil || firstCount != 2 || cursor == "" {
		t.Fatalf("unexpected first page: count=%d cursor=%q err=%v", firstCount, cursor, err)
	}
	secondCount, next, err := list(foundation.PageRequest{Limit: 2, Scope: scope, Cursor: cursor})
	if err != nil || secondCount != 1 || next != "" {
		t.Fatalf("unexpected second page: count=%d cursor=%q err=%v", secondCount, next, err)
	}
	if _, _, err := list(foundation.PageRequest{Limit: 2, Scope: scope, Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("expected invalid cursor rejection")
	}
}
