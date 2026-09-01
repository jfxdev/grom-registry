package bunstore

import (
	"context"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func TestSearchTagNamesPageFiltersByNameAndExcludesDetached(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)

		for _, tag := range []string{"v1.0.0", "v1.1.0", "release-candidate"} {
			observation := registrydomain.ManifestObservation{
				Digest: "sha256:manifest-" + tag, ManifestSize: 10, Tag: tag,
			}
			if err := store.UpsertManifestObservation(ctx, repositoryID, observation, now); err != nil {
				t.Fatal(err)
			}
		}

		byPrefix, err := store.SearchTagNamesPage(ctx, repositoryID, "v1", foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=v1"})
		if err != nil || len(byPrefix.Items) != 2 || byPrefix.Items[0] != "v1.0.0" || byPrefix.Items[1] != "v1.1.0" {
			t.Fatalf("search by name prefix: items=%#v err=%v", byPrefix.Items, err)
		}

		noMatch, err := store.SearchTagNamesPage(ctx, repositoryID, "does-not-exist", foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=does-not-exist"})
		if err != nil || len(noMatch.Items) != 0 {
			t.Fatalf("no match search: items=%#v err=%v", noMatch.Items, err)
		}

		if err := store.MarkManifestDeleted(ctx, repositoryID, "sha256:manifest-release-candidate", now); err != nil {
			t.Fatal(err)
		}
		afterDeletion, err := store.SearchTagNamesPage(ctx, repositoryID, "release", foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=release"})
		if err != nil || len(afterDeletion.Items) != 0 {
			t.Fatalf("deleted manifest tag should not appear in search: items=%#v err=%v", afterDeletion.Items, err)
		}
	})
}

func TestSearchTagNamesPagePaginatesWithKeysetCursor(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)

		for _, tag := range []string{"alpha", "beta", "gamma"} {
			observation := registrydomain.ManifestObservation{Digest: "sha256:manifest-" + tag, ManifestSize: 10, Tag: tag}
			if err := store.UpsertManifestObservation(ctx, repositoryID, observation, now); err != nil {
				t.Fatal(err)
			}
		}

		scope := "repository-tags:test:api:q="
		first, err := store.SearchTagNamesPage(ctx, repositoryID, "", foundation.PageRequest{Limit: 2, Scope: scope})
		if err != nil || len(first.Items) != 2 || first.Items[0] != "alpha" || first.Items[1] != "beta" || first.NextCursor == "" {
			t.Fatalf("first page: items=%#v cursor=%q err=%v", first.Items, first.NextCursor, err)
		}

		second, err := store.SearchTagNamesPage(ctx, repositoryID, "", foundation.PageRequest{Limit: 2, Scope: scope, Cursor: first.NextCursor})
		if err != nil || len(second.Items) != 1 || second.Items[0] != "gamma" || second.NextCursor != "" {
			t.Fatalf("second page: items=%#v cursor=%q err=%v", second.Items, second.NextCursor, err)
		}
	})
}
