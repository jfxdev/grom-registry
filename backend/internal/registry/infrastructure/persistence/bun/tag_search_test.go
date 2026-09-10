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

		byPrefix, err := store.SearchTagNamesPage(ctx, repositoryID, "v1", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=v1"})
		if err != nil || len(byPrefix.Items) != 2 || byPrefix.Items[0] != "v1.0.0" || byPrefix.Items[1] != "v1.1.0" {
			t.Fatalf("search by name prefix: items=%#v err=%v", byPrefix.Items, err)
		}

		noMatch, err := store.SearchTagNamesPage(ctx, repositoryID, "does-not-exist", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=does-not-exist"})
		if err != nil || len(noMatch.Items) != 0 {
			t.Fatalf("no match search: items=%#v err=%v", noMatch.Items, err)
		}

		if err := store.MarkManifestDeleted(ctx, repositoryID, "sha256:manifest-release-candidate", now); err != nil {
			t.Fatal(err)
		}
		afterDeletion, err := store.SearchTagNamesPage(ctx, repositoryID, "release", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=release"})
		if err != nil || len(afterDeletion.Items) != 0 {
			t.Fatalf("deleted manifest tag should not appear in search: items=%#v err=%v", afterDeletion.Items, err)
		}
	})
}

func TestSearchTagNamesPageEscapesLikeMetacharacters(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)

		for _, tag := range []string{"50%_off", "zoff", "back\\slash"} {
			observation := registrydomain.ManifestObservation{
				Digest: "sha256:manifest-" + tag, ManifestSize: 10, Tag: tag,
			}
			if err := store.UpsertManifestObservation(ctx, repositoryID, observation, now); err != nil {
				t.Fatal(err)
			}
		}

		percent, err := store.SearchTagNamesPage(ctx, repositoryID, "50%", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=50%"})
		if err != nil || len(percent.Items) != 1 || percent.Items[0] != "50%_off" {
			t.Fatalf("literal %% search: items=%#v err=%v", percent.Items, err)
		}

		underscore, err := store.SearchTagNamesPage(ctx, repositoryID, "_off", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=_off"})
		if err != nil || len(underscore.Items) != 1 || underscore.Items[0] != "50%_off" {
			t.Fatalf("literal _ search must not match zoff via wildcard: items=%#v err=%v", underscore.Items, err)
		}

		backslash, err := store.SearchTagNamesPage(ctx, repositoryID, "back\\slash", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 10, Scope: "repository-tags:test:api:q=back\\slash"})
		if err != nil || len(backslash.Items) != 1 || backslash.Items[0] != "back\\slash" {
			t.Fatalf("literal backslash search: items=%#v err=%v", backslash.Items, err)
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

		scope := "repository-tags:test:api:q=:sort=" + string(registrydomain.TagSortNewest)
		first, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 2, Scope: scope})
		if err != nil || len(first.Items) != 2 || first.Items[0] != "alpha" || first.Items[1] != "beta" || first.NextCursor == "" {
			t.Fatalf("first page: items=%#v cursor=%q err=%v", first.Items, first.NextCursor, err)
		}

		second, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 2, Scope: scope, Cursor: first.NextCursor})
		if err != nil || len(second.Items) != 1 || second.Items[0] != "gamma" || second.NextCursor != "" {
			t.Fatalf("second page: items=%#v cursor=%q err=%v", second.Items, second.NextCursor, err)
		}
	})
}

func TestSearchTagNamesPageSortsByNameDescending(t *testing.T) {
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

		scope := "repository-tags:test:api:q=:sort=" + string(registrydomain.TagSortNameDesc)
		first, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortNameDesc, foundation.PageRequest{Limit: 2, Scope: scope})
		if err != nil || len(first.Items) != 2 || first.Items[0] != "gamma" || first.Items[1] != "beta" || first.NextCursor == "" {
			t.Fatalf("first page: items=%#v cursor=%q err=%v", first.Items, first.NextCursor, err)
		}

		second, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortNameDesc, foundation.PageRequest{Limit: 2, Scope: scope, Cursor: first.NextCursor})
		if err != nil || len(second.Items) != 1 || second.Items[0] != "alpha" || second.NextCursor != "" {
			t.Fatalf("second page: items=%#v cursor=%q err=%v", second.Items, second.NextCursor, err)
		}
	})
}

func TestSearchTagNamesPageSortsByPushDate(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, base)

		pushes := []struct {
			tag string
			at  time.Time
		}{
			{"oldest-tag", base},
			{"middle-tag", base.Add(time.Hour)},
			{"newest-tag", base.Add(2 * time.Hour)},
		}
		for _, push := range pushes {
			observation := registrydomain.ManifestObservation{Digest: "sha256:manifest-" + push.tag, ManifestSize: 10, Tag: push.tag}
			if err := store.UpsertManifestObservation(ctx, repositoryID, observation, push.at); err != nil {
				t.Fatal(err)
			}
		}

		newestScope := "repository-tags:test:api:q=:sort=" + string(registrydomain.TagSortNewest)
		newestFirst, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 2, Scope: newestScope})
		if err != nil || len(newestFirst.Items) != 2 || newestFirst.Items[0] != "newest-tag" || newestFirst.Items[1] != "middle-tag" || newestFirst.NextCursor == "" {
			t.Fatalf("newest first page: items=%#v cursor=%q err=%v", newestFirst.Items, newestFirst.NextCursor, err)
		}
		newestSecond, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortNewest, foundation.PageRequest{Limit: 2, Scope: newestScope, Cursor: newestFirst.NextCursor})
		if err != nil || len(newestSecond.Items) != 1 || newestSecond.Items[0] != "oldest-tag" || newestSecond.NextCursor != "" {
			t.Fatalf("newest second page: items=%#v cursor=%q err=%v", newestSecond.Items, newestSecond.NextCursor, err)
		}

		oldestScope := "repository-tags:test:api:q=:sort=" + string(registrydomain.TagSortOldest)
		oldestFirst, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortOldest, foundation.PageRequest{Limit: 2, Scope: oldestScope})
		if err != nil || len(oldestFirst.Items) != 2 || oldestFirst.Items[0] != "oldest-tag" || oldestFirst.Items[1] != "middle-tag" || oldestFirst.NextCursor == "" {
			t.Fatalf("oldest first page: items=%#v cursor=%q err=%v", oldestFirst.Items, oldestFirst.NextCursor, err)
		}
		oldestSecond, err := store.SearchTagNamesPage(ctx, repositoryID, "", registrydomain.TagSortOldest, foundation.PageRequest{Limit: 2, Scope: oldestScope, Cursor: oldestFirst.NextCursor})
		if err != nil || len(oldestSecond.Items) != 1 || oldestSecond.Items[0] != "newest-tag" || oldestSecond.NextCursor != "" {
			t.Fatalf("oldest second page: items=%#v cursor=%q err=%v", oldestSecond.Items, oldestSecond.NextCursor, err)
		}
	})
}
