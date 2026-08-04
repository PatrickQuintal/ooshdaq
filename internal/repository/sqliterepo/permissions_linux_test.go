//go:build linux

package sqliterepo

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
)

func TestSQLiteArtifactsStayPrivateWithPermissiveUmask(t *testing.T) {
	previousUmask := syscall.Umask(0o022)
	t.Cleanup(func() {
		syscall.Umask(previousUmask)
	})

	ctx := context.Background()
	root := t.TempDir()
	dataDirectory := filepath.Join(root, "data")
	databasePath := filepath.Join(dataDirectory, "ooshdaq.sqlite")
	store, err := Open(ctx, Config{Path: databasePath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.UpsertUser(ctx, domain.User{
		ID:          "permission-member",
		DisplayName: "Permission Member",
	}); err != nil {
		t.Fatalf("write WAL record: %v", err)
	}
	assertMode(t, dataDirectory, privateDirectoryMode)
	assertMode(t, databasePath, privateFileMode)
	assertMode(t, databasePath+"-wal", privateFileMode)
	assertMode(t, databasePath+"-shm", privateFileMode)

	backupDirectory := filepath.Join(root, "backups")
	backupPath := filepath.Join(backupDirectory, "backup.sqlite")
	if _, err := store.Backup(ctx, backupPath); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	assertMode(t, backupDirectory, privateDirectoryMode)
	assertMode(t, backupPath, privateFileMode)
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulate an older installation that created its main file under 0022.
	if err := os.Chmod(databasePath, 0o644); err != nil {
		t.Fatalf("make restart fixture shared: %v", err)
	}
	reopened, err := Open(ctx, Config{Path: databasePath})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	assertMode(t, databasePath, privateFileMode)
	assertMode(t, databasePath+"-wal", privateFileMode)
	assertMode(t, databasePath+"-shm", privateFileMode)
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened database: %v", err)
	}

	restoreDirectory := filepath.Join(root, "restore")
	restorePath := filepath.Join(restoreDirectory, "restored.sqlite")
	if _, err := RestoreVerified(ctx, backupPath, restorePath); err != nil {
		t.Fatalf("RestoreVerified: %v", err)
	}
	assertMode(t, restoreDirectory, privateDirectoryMode)
	assertMode(t, restorePath, privateFileMode)
}

func TestExistingAndLinkedStorageDirectoriesKeepOperatorModes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "shared-target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create shared target: %v", err)
	}
	linked := filepath.Join(root, "linked-data")
	if err := os.Symlink(target, linked); err != nil {
		t.Fatalf("link shared target: %v", err)
	}

	databasePath := filepath.Join(linked, "ooshdaq.sqlite")
	store, err := Open(context.Background(), Config{Path: databasePath})
	if err != nil {
		t.Fatalf("Open through linked directory: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertMode(t, target, 0o755)
	assertMode(t, databasePath, privateFileMode)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", filepath.Base(path), err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s = %#o, want %#o", filepath.Base(path), got, want)
	}
}
