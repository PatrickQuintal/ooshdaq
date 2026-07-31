package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	sqlite "modernc.org/sqlite"
)

const (
	requiredSQLiteVersion  = "3.53.3"
	requiredSQLiteSourceID = "2026-06-26 20:14:12 d4c0e51e4aeb96955b99185ab9cde75c339e2c29c3f3f12428d364a10d782c62"
)

type onlineBackuper interface {
	NewBackup(string) (*sqlite.Backup, error)
}

func TestSelectedSQLiteEngineEvidence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "evidence.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	database.SetMaxOpenConns(1)

	for _, statement := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 250",
		"CREATE TABLE evidence (id INTEGER PRIMARY KEY, value TEXT NOT NULL UNIQUE)",
		"INSERT INTO evidence (value) VALUES ('expected')",
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	var version, sourceID string
	if err := database.QueryRow("SELECT sqlite_version(), sqlite_source_id()").Scan(&version, &sourceID); err != nil {
		t.Fatalf("query SQLite identity: %v", err)
	}
	if version != requiredSQLiteVersion || sourceID != requiredSQLiteSourceID {
		t.Fatalf("SQLite identity = %q / %q, want %q / %q", version, sourceID, requiredSQLiteVersion, requiredSQLiteSourceID)
	}

	var journalMode string
	var synchronous, foreignKeys, busyTimeout int
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if err := database.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatalf("synchronous: %v", err)
	}
	if err := database.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if err := database.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if journalMode != "wal" || synchronous != 2 || foreignKeys != 1 || busyTimeout != 250 {
		t.Fatalf(
			"PRAGMAs = journal=%q synchronous=%d foreign_keys=%d busy_timeout=%d",
			journalMode,
			synchronous,
			foreignKeys,
			busyTimeout,
		)
	}

	_, err = database.Exec("INSERT INTO evidence (value) VALUES ('expected')")
	var sqliteError *sqlite.Error
	if !errors.As(err, &sqliteError) || sqliteError.Code() != 2067 {
		t.Fatalf("duplicate error = %T %v, want SQLite extended code 2067", err, err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.sqlite")
	connection, err := database.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	err = connection.Raw(func(driverConnection any) error {
		backuper, ok := driverConnection.(onlineBackuper)
		if !ok {
			return errors.New("selected driver does not expose online backup")
		}
		backup, err := backuper.NewBackup(backupPath)
		if err != nil {
			return err
		}
		if _, err := backup.Step(-1); err != nil {
			_ = backup.Finish()
			return err
		}
		return backup.Finish()
	})
	closeErr := connection.Close()
	if err != nil {
		t.Fatalf("online backup: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("close raw connection: %v", closeErr)
	}

	restored, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow("SELECT value FROM evidence WHERE id = 1").Scan(&value); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if value != "expected" {
		t.Fatalf("backup value = %q, want expected", value)
	}
}

func TestSelectedSQLiteDriverHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	var result int64
	err = database.QueryRowContext(ctx, `
		WITH RECURSIVE numbers(value) AS (
			VALUES(1)
			UNION ALL
			SELECT value + 1 FROM numbers WHERE value < 1000000000
		)
		SELECT sum(value) FROM numbers
	`).Scan(&result)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recursive query error = %v, want context deadline exceeded", err)
	}
}
