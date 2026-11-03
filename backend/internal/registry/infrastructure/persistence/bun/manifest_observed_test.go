package bunstore

import (
	"context"
	"testing"
	"time"

	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func TestManifestObservedTracksLiveDigestsAndAttachedTags(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
		_, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		observed := func(tag, digest string) bool {
			t.Helper()
			ok, err := store.ManifestObserved(ctx, repositoryID, tag, digest)
			if err != nil {
				t.Fatal(err)
			}
			return ok
		}

		if observed("", "sha256:a") || observed("latest", "sha256:a") {
			t.Fatal("unobserved manifest reported as observed")
		}
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:a", ManifestSize: 10, Tag: "latest"}, now); err != nil {
			t.Fatal(err)
		}
		if !observed("", "sha256:a") || !observed("latest", "sha256:a") || observed("latest", "sha256:b") || observed("stable", "sha256:a") {
			t.Fatal("first observation was not reported exactly")
		}

		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{Digest: "sha256:b", ManifestSize: 10, Tag: "latest"}, now.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		if observed("latest", "sha256:a") || !observed("latest", "sha256:b") {
			t.Fatal("moved tag still reports its previous digest")
		}

		if err := store.MarkManifestDeleted(ctx, repositoryID, "sha256:b", now.Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if observed("", "sha256:b") || observed("latest", "sha256:b") {
			t.Fatal("deleted manifest reported as observed")
		}
	})
}
