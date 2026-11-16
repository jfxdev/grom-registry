package bunstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func TestFindManifestInventoryIsScopedToItsRepository(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		otherID := foundation.NewID()
		if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: otherID, ProjectID: projectID, Name: "other", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := store.UpsertManifestObservation(ctx, repositoryID, registrydomain.ManifestObservation{
			Digest: "sha256:image", MediaType: "application/vnd.oci.image.manifest.v1+json", ManifestSize: 10, Tag: "v1",
			Platforms: []registrydomain.ManifestPlatform{{OS: "linux", Architecture: "amd64", Digest: "sha256:image", CompressedSize: 42}},
		}, now); err != nil {
			t.Fatal(err)
		}

		found, err := store.FindManifestInventory(ctx, repositoryID, "sha256:image")
		if err != nil {
			t.Fatal(err)
		}
		if found.Digest != "sha256:image" || found.RepositoryID != repositoryID || len(found.Tags) != 1 || found.Tags[0] != "v1" || len(found.Platforms) != 1 {
			t.Fatalf("unexpected manifest inventory: %#v", found)
		}
		if _, err := store.FindManifestInventory(ctx, otherID, "sha256:image"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected another repository lookup to miss, got %v", err)
		}
		if _, err := store.FindManifestInventory(ctx, repositoryID, "sha256:missing"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected a missing digest to miss, got %v", err)
		}
	})
}

func TestListManifestReferrersPageFiltersBySubjectAndPaginates(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		projectID, repositoryID := seedProjectWithRepository(t, ctx, db, store, now)
		otherID := foundation.NewID()
		if err := store.CreateRepository(ctx, &registrydomain.Repository{ID: otherID, ProjectID: projectID, Name: "other", Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual, Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		observe := func(repository foundation.ID, digest, subject string) {
			t.Helper()
			observation := registrydomain.ManifestObservation{Digest: digest, ManifestSize: 10, SubjectDigest: subject, ArtifactType: "application/spdx+json"}
			if subject != "" {
				observation.ArtifactRelationship = constants.ArtifactRelationshipReferrer
			}
			if err := store.UpsertManifestObservation(ctx, repository, observation, now); err != nil {
				t.Fatal(err)
			}
		}
		observe(repositoryID, "sha256:subject", "")
		// Equal first_seen_at values force the keyset to tie-break on id.
		observe(repositoryID, "sha256:sbom-1", "sha256:subject")
		observe(repositoryID, "sha256:sbom-2", "sha256:subject")
		observe(repositoryID, "sha256:sbom-3", "sha256:subject")
		observe(repositoryID, "sha256:unrelated", "sha256:elsewhere")
		observe(otherID, "sha256:foreign", "sha256:subject")

		scope := "mcp:list_referrers:test"
		seen := map[string]bool{}
		request := foundation.PageRequest{Limit: 2, Scope: scope}
		for page := 0; ; page++ {
			result, err := store.ListManifestReferrersPage(ctx, repositoryID, "sha256:subject", request)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range result.Items {
				if seen[item.Digest] {
					t.Fatalf("referrer %s returned twice", item.Digest)
				}
				if item.SubjectDigest != "sha256:subject" || item.RepositoryID != repositoryID {
					t.Fatalf("unexpected referrer: %#v", item)
				}
				seen[item.Digest] = true
			}
			if result.NextCursor == "" {
				break
			}
			if page > 2 {
				t.Fatal("referrer pagination did not terminate")
			}
			request.Cursor = result.NextCursor
		}
		if len(seen) != 3 || !seen["sha256:sbom-1"] || !seen["sha256:sbom-2"] || !seen["sha256:sbom-3"] {
			t.Fatalf("unexpected referrer set: %v", seen)
		}

		first, err := store.ListManifestReferrersPage(ctx, repositoryID, "sha256:subject", foundation.PageRequest{Limit: 1, Scope: scope})
		if err != nil || first.NextCursor == "" {
			t.Fatalf("expected a next cursor, got %#v err=%v", first, err)
		}
		if _, err := store.ListManifestReferrersPage(ctx, repositoryID, "sha256:subject", foundation.PageRequest{Limit: 1, Scope: "another-scope", Cursor: first.NextCursor}); err == nil {
			t.Fatal("expected a cursor from another scope to be rejected")
		}
	})
}
