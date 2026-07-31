package repository_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
	"github.com/ooshdaq/ooshdaq/internal/repository/conformance"
)

func TestMemoryStoreConformance(t *testing.T) {
	t.Parallel()

	conformance.Run(t, func(*testing.T) conformance.Store {
		return repository.NewMemoryStore()
	})
}

func TestMemoryStoreConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := repository.NewMemoryStore()
	loaded, err := catalog.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	for _, candidate := range loaded {
		if _, err := store.ImportManifest(ctx, repository.ManifestImport{
			Manifest: candidate.Manifest,
			SHA256:   candidate.SHA256,
		}); err != nil {
			t.Fatalf("ImportManifest(%s): %v", candidate.Manifest.Catalog.ID, err)
		}
	}
	user := domain.User{ID: "concurrent-member", DisplayName: "Concurrent Member"}
	if err := store.UpsertUser(ctx, user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	catalogID := loaded[0].Manifest.Catalog.ID
	itemID := loaded[0].Manifest.Items[0].ID
	const workers = 8
	const iterations = 100
	start := make(chan struct{})
	failures := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for iteration := range iterations {
				if worker%2 == 0 {
					owned := (worker + iteration) % 2
					if _, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
						UserID: user.ID,
						ItemID: itemID,
						Owned:  owned,
					}); err != nil {
						failures <- fmt.Errorf("worker %d write %d: %w", worker, iteration, err)
						return
					}
					continue
				}
				if _, err := store.ListCatalogs(ctx, repository.CatalogQuery{
					UserID: user.ID,
					Limit:  1,
				}); err != nil {
					failures <- fmt.Errorf("worker %d catalogs %d: %w", worker, iteration, err)
					return
				}
				if _, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
					UserID:    user.ID,
					CatalogID: catalogID,
					Limit:     1,
				}); err != nil {
					failures <- fmt.Errorf("worker %d collection %d: %w", worker, iteration, err)
					return
				}
				if _, err := store.GetCollectionSummary(ctx, user.ID); err != nil {
					failures <- fmt.Errorf("worker %d summary %d: %w", worker, iteration, err)
					return
				}
			}
		}()
	}
	close(start)
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}
