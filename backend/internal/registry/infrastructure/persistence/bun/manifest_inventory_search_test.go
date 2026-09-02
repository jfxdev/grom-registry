package bunstore

import (
	"context"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func TestListManifestInventoryPageFiltersByTagNameOrDigest(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)

		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{
			Digest: "sha256:aaaa", ManifestSize: 10, Tag: "release-v1",
		}, now); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{
			Digest: "sha256:bbbb", ManifestSize: 10, Tag: "nightly",
		}, now); err != nil {
			t.Fatal(err)
		}

		byTag, err := store.ListManifestInventoryPage(ctx, repositoryID, "release", foundation.PageRequest{Limit: 10, Scope: "repository-inventory:test:api:q=release"})
		if err != nil || len(byTag.Items) != 1 || byTag.Items[0].Digest != "sha256:aaaa" {
			t.Fatalf("search by tag name: items=%#v err=%v", byTag.Items, err)
		}

		byDigest, err := store.ListManifestInventoryPage(ctx, repositoryID, "bbbb", foundation.PageRequest{Limit: 10, Scope: "repository-inventory:test:api:q=bbbb"})
		if err != nil || len(byDigest.Items) != 1 || byDigest.Items[0].Digest != "sha256:bbbb" {
			t.Fatalf("search by digest: items=%#v err=%v", byDigest.Items, err)
		}

		noMatch, err := store.ListManifestInventoryPage(ctx, repositoryID, "does-not-exist", foundation.PageRequest{Limit: 10, Scope: "repository-inventory:test:api:q=does-not-exist"})
		if err != nil || len(noMatch.Items) != 0 {
			t.Fatalf("no match search: items=%#v err=%v", noMatch.Items, err)
		}
	})
}
