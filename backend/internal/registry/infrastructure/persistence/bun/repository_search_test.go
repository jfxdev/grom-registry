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

func TestListRepositoriesPageFiltersByNameAndDescription(t *testing.T) {
	forStorageDatabases(t, func(t *testing.T, db *bun.DB) {
		ctx := context.Background()
		store := New(db)
		now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
		projectID, _ := seedProjectWithRepository(t, ctx, db, store, now)

		if err := store.CreateRepository(ctx, &registrydomain.Repository{
			ID: foundation.NewID(), ProjectID: projectID, Name: "worker", Description: "Handles background payments jobs",
			Status: constants.RepositoryStatusActive, CreationSource: constants.RepositoryCreationManual,
			Policies: []registrydomain.Policy{}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		byName, err := store.ListRepositoriesPage(ctx, projectID, "ap", foundation.PageRequest{Limit: 10, Scope: "repositories:test:q=ap"})
		if err != nil || len(byName.Items) != 1 || byName.Items[0].Name != "api" {
			t.Fatalf("search by name: items=%#v err=%v", byName.Items, err)
		}

		byDescription, err := store.ListRepositoriesPage(ctx, projectID, "PAYMENTS", foundation.PageRequest{Limit: 10, Scope: "repositories:test:q=payments"})
		if err != nil || len(byDescription.Items) != 1 || byDescription.Items[0].Name != "worker" {
			t.Fatalf("case-insensitive search by description: items=%#v err=%v", byDescription.Items, err)
		}

		noMatch, err := store.ListRepositoriesPage(ctx, projectID, "does-not-exist", foundation.PageRequest{Limit: 10, Scope: "repositories:test:q=does-not-exist"})
		if err != nil || len(noMatch.Items) != 0 {
			t.Fatalf("no match search: items=%#v err=%v", noMatch.Items, err)
		}

		unfiltered, err := store.ListRepositoriesPage(ctx, projectID, "", foundation.PageRequest{Limit: 10, Scope: "repositories:test:q="})
		if err != nil || len(unfiltered.Items) != 2 {
			t.Fatalf("empty search returns all: items=%#v err=%v", unfiltered.Items, err)
		}
	})
}
