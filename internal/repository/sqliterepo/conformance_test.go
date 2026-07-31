package sqliterepo

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ooshdaq/ooshdaq/internal/repository/conformance"
)

func TestRepositoryConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) conformance.Store {
		t.Helper()
		store, err := Open(context.Background(), Config{
			Path: filepath.Join(t.TempDir(), "conformance.sqlite"),
		})
		if err != nil {
			t.Fatalf("Open conformance store: %v", err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Errorf("Close conformance store: %v", err)
			}
		})
		return store
	})
}
