package bunstore

import (
	"context"
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
	"github.com/jfxdev/grom/backend/internal/foundation"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
	"github.com/uptrace/bun"
)

func TestSearchRepositoriesAcrossProjectsFiltersAndCarriesProjectContext(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

		projectOne, projectTwo := foundation.NewID(), foundation.NewID()
		if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectOne.String(), "payments", "Payments", "test", now); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectTwo.String(), "checkout", "Checkout", "test", now); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateRepository(ctx, &registrydomain.Repository{
			ID: foundation.NewID(), ProjectID: projectOne, Name: "api", Description: "Payments API",
			Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual,
			Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateRepository(ctx, &registrydomain.Repository{
			ID: foundation.NewID(), ProjectID: projectTwo, Name: "worker", Description: "Checkout worker",
			Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual,
			Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		matched, err := store.SearchRepositoriesAcrossProjects(ctx, "api", foundation.PageRequest{Limit: 10, Scope: "repositories-search:q=api"})
		if err != nil || len(matched.Items) != 1 || matched.Items[0].ProjectSlug != "payments" || matched.Items[0].Name != "api" {
			t.Fatalf("cross-project search by name: items=%#v err=%v", matched.Items, err)
		}

		all, err := store.SearchRepositoriesAcrossProjects(ctx, "", foundation.PageRequest{Limit: 10, Scope: "repositories-search:q="})
		if err != nil || len(all.Items) != 2 {
			t.Fatalf("cross-project search with empty query returns all: items=%#v err=%v", all.Items, err)
		}

		noMatch, err := store.SearchRepositoriesAcrossProjects(ctx, "does-not-exist", foundation.PageRequest{Limit: 10, Scope: "repositories-search:q=does-not-exist"})
		if err != nil || len(noMatch.Items) != 0 {
			t.Fatalf("cross-project search no match: items=%#v err=%v", noMatch.Items, err)
		}
	})
}

func TestSearchRepositoriesAcrossProjectsPaginatesWithKeysetCursor(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

		projectID := foundation.NewID()
		if _, err := db.ExecContext(ctx, "INSERT INTO projects (id, slug, name, created_by, created_at) VALUES (?, ?, ?, ?, ?)", projectID.String(), "payments", "Payments", "test", now); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"alpha", "beta", "gamma"} {
			if err := store.CreateRepository(ctx, &registrydomain.Repository{
				ID: foundation.NewID(), ProjectID: projectID, Name: name,
				Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual,
				Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
		}

		scope := "repositories-search:q="
		first, err := store.SearchRepositoriesAcrossProjects(ctx, "", foundation.PageRequest{Limit: 2, Scope: scope})
		if err != nil || len(first.Items) != 2 || first.Items[0].Name != "alpha" || first.Items[1].Name != "beta" || first.NextCursor == "" {
			t.Fatalf("first page: items=%#v cursor=%q err=%v", first.Items, first.NextCursor, err)
		}

		second, err := store.SearchRepositoriesAcrossProjects(ctx, "", foundation.PageRequest{Limit: 2, Scope: scope, Cursor: first.NextCursor})
		if err != nil || len(second.Items) != 1 || second.Items[0].Name != "gamma" || second.NextCursor != "" {
			t.Fatalf("second page: items=%#v cursor=%q err=%v", second.Items, second.NextCursor, err)
		}
	})
}
