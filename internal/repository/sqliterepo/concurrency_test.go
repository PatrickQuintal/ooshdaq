package sqliterepo

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	catalogdata "github.com/ooshdaq/ooshdaq/internal/catalog"
	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

func TestConcurrentReadsAndSerializedWritesDoNotExposeBusy(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), Config{
		Path:        filepath.Join(t.TempDir(), "concurrent.sqlite"),
		BusyTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	loaded, err := catalogdata.LoadEmbeddedManifests()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifests: %v", err)
	}
	if err := store.ImportManifests(context.Background(), loaded); err != nil {
		t.Fatalf("ImportManifests: %v", err)
	}
	const workers = 32
	for worker := range workers {
		user := domain.User{
			ID:          fmt.Sprintf("concurrent-%02d", worker),
			DisplayName: fmt.Sprintf("Concurrent %02d", worker),
		}
		if err := store.UpsertUser(context.Background(), user); err != nil {
			t.Fatalf("UpsertUser: %v", err)
		}
	}
	itemID := loaded[0].Manifest.Items[0].ID
	catalogID := loaded[0].Manifest.Catalog.ID

	start := make(chan struct{})
	failures := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := range workers {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			userID := fmt.Sprintf("concurrent-%02d", worker)
			if worker%4 == 0 {
				_, err := store.SaveCollectionEntry(ctx, repository.CollectionWrite{
					UserID:    userID,
					ItemID:    itemID,
					Owned:     2,
					Available: 1,
				})
				failures <- err
				return
			}
			_, err := store.ListCollectionPage(ctx, repository.CollectionQuery{
				UserID:    userID,
				CatalogID: catalogID,
				Limit:     10,
			})
			failures <- err
		}(worker)
	}
	close(start)
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Errorf("concurrent operation: %v (class %q)", err, repository.Classify(err))
		}
	}
}
