package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	"github.com/PatrickQuintal/ooshdaq/internal/repository/sqliterepo"
)

func TestRestoreOfflinePreservesAndAtomicallyReplacesDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	directory := t.TempDir()
	livePath := filepath.Join(directory, "live.sqlite")
	live, err := sqliterepo.Open(ctx, sqliterepo.Config{Path: livePath})
	if err != nil {
		t.Fatalf("open live database: %v", err)
	}
	if err := live.UpsertUser(ctx, domain.User{
		ID:          "old-member",
		DisplayName: "Old Member",
	}); err != nil {
		t.Fatalf("seed live database: %v", err)
	}
	if err := live.Close(); err != nil {
		t.Fatalf("close live database: %v", err)
	}

	sourcePath := filepath.Join(directory, "source.sqlite")
	source, err := sqliterepo.Open(ctx, sqliterepo.Config{Path: sourcePath})
	if err != nil {
		t.Fatalf("open source database: %v", err)
	}
	if err := source.UpsertUser(ctx, domain.User{
		ID:          "new-member",
		DisplayName: "New Member",
	}); err != nil {
		t.Fatalf("seed source database: %v", err)
	}
	backupPath := filepath.Join(directory, "selected-backup.sqlite")
	if _, err := source.Backup(ctx, backupPath); err != nil {
		t.Fatalf("create selected backup: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close source database: %v", err)
	}

	retainedPath := filepath.Join(directory, "retained-live.sqlite")
	if err := RestoreOffline(ctx, RestoreConfig{
		Backup:         backupPath,
		Database:       livePath,
		RetainReplaced: retainedPath,
	}); err != nil {
		t.Fatalf("RestoreOffline: %v", err)
	}
	if runtime.GOOS != "windows" {
		assertPrivateRestoreFile(t, livePath)
		assertPrivateRestoreFile(t, retainedPath)
	}
	restored, err := sqliterepo.Open(ctx, sqliterepo.Config{Path: livePath})
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restored.Close()
	if _, err := restored.GetUser(ctx, "new-member"); err != nil {
		t.Fatalf("restored member: %v", err)
	}
	if _, err := restored.GetUser(ctx, "old-member"); !isNotFound(err) {
		t.Fatalf("old member remained in restored database: %v", err)
	}
	retained, err := sqliterepo.Open(ctx, sqliterepo.Config{Path: retainedPath})
	if err != nil {
		t.Fatalf("open retained database: %v", err)
	}
	defer retained.Close()
	if _, err := retained.GetUser(ctx, "old-member"); err != nil {
		t.Fatalf("retained member: %v", err)
	}
}

func assertPrivateRestoreFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat restored artifact: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("restored artifact mode = %#o, want 0600", got)
	}
}

func isNotFound(err error) bool {
	return repository.Classify(err) == repository.FailureNotFound
}
