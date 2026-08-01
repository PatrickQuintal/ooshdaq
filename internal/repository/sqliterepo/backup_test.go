package sqliterepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlineBackupAndIsolatedRestore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "live.sqlite")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if _, err := store.database.ExecContext(ctx, `
		INSERT INTO brands(id, name, status) VALUES ('backup-brand', 'Backup Brand', 'active');
		INSERT INTO catalogs(id, brand_id, name, status, sort_order)
			VALUES ('backup-catalog', 'backup-brand', 'Backup Catalog', 'active', 1);
	`); err != nil {
		t.Fatalf("seed backup: %v", err)
	}

	backupPath := filepath.Join(t.TempDir(), "backups", "backup.sqlite")
	result, err := store.Backup(ctx, backupPath)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if result.Path != backupPath || result.SizeBytes <= 0 || len(result.SHA256) != 64 ||
		result.CreatedAt.IsZero() {
		t.Fatalf("Backup result = %#v", result)
	}
	if err := VerifyDatabase(ctx, backupPath, true); err != nil {
		t.Fatalf("VerifyDatabase: %v", err)
	}

	restorePath := filepath.Join(t.TempDir(), "restored", "ooshdaq.sqlite")
	restored, err := RestoreVerified(ctx, backupPath, restorePath)
	if err != nil {
		t.Fatalf("RestoreVerified: %v", err)
	}
	if restored.Path != restorePath || restored.SizeBytes <= 0 || len(restored.SHA256) != 64 {
		t.Fatalf("Restore result = %#v", restored)
	}
	reopened, err := Open(ctx, Config{Path: restorePath})
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer reopened.Close()
	var catalogName string
	if err := reopened.database.QueryRowContext(
		ctx,
		"SELECT name FROM catalogs WHERE id = 'backup-catalog'",
	).Scan(&catalogName); err != nil {
		t.Fatalf("query restored catalog: %v", err)
	}
	if catalogName != "Backup Catalog" {
		t.Fatalf("restored catalog name = %q", catalogName)
	}
}

func TestBackupAndRestoreRefuseUnsafeDestinations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "live.sqlite")
	store, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if _, err := store.Backup(ctx, path); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("same-path Backup error = %v", err)
	}
	destination := filepath.Join(t.TempDir(), "exists.sqlite")
	if err := os.WriteFile(destination, []byte("reserved"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := store.Backup(ctx, destination); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing-path Backup error = %v", err)
	}
	if _, err := RestoreVerified(ctx, path, destination); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing-path Restore error = %v", err)
	}
}
