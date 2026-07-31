package sqliterepo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/domain"
	"github.com/ooshdaq/ooshdaq/internal/repository"
)

const crashHelperEnvironment = "OOSHDAQ_SQLITE_CRASH_HELPER"

func TestAbruptProcessRecovery(t *testing.T) {
	if path := os.Getenv(crashHelperEnvironment); path != "" {
		runCrashHelper(path)
		return
	}

	path := filepath.Join(t.TempDir(), "abrupt.sqlite")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestAbruptProcessRecovery$",
	)
	command.Env = append(os.Environ(), crashHelperEnvironment+"="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("abrupt writer failed: %v\n%s", err, output)
	}

	recovered, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatalf("recover after abrupt exit: %v", err)
	}
	defer recovered.Close()
	user, err := recovered.GetUser(context.Background(), "crash-member")
	if err != nil {
		t.Fatalf("read committed WAL record: %v", err)
	}
	if user.DisplayName != "Crash Member" {
		t.Fatalf("recovered user = %#v", user)
	}
	if err := recovered.Check(context.Background(), true); err != nil {
		t.Fatalf("integrity after recovery: %v", err)
	}
}

func runCrashHelper(path string) {
	store, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if _, err := store.database.Exec("PRAGMA wal_autocheckpoint = 0"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := store.UpsertUser(context.Background(), domain.User{
		ID:          "crash-member",
		DisplayName: "Crash Member",
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Deliberately bypass Store.Close and all test cleanup. The parent process
	// must recover the committed WAL exactly as an abruptly killed pod would.
	os.Exit(0)
}

func TestMigrationFailureRollsBackSchemaAndHistory(t *testing.T) {
	store, err := Open(
		context.Background(),
		Config{Path: filepath.Join(t.TempDir(), "migration-rollback.sqlite")},
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	broken := migration{
		version:  5,
		name:     "005_broken.sql",
		checksum: strings.Repeat("0", 64),
		sql: `
			CREATE TABLE must_rollback (id INTEGER PRIMARY KEY);
			INSERT INTO table_that_does_not_exist(id) VALUES (1);
		`,
	}
	if err := applyMigration(
		context.Background(),
		store.database,
		broken,
		time.Now(),
	); err == nil {
		t.Fatal("broken migration unexpectedly succeeded")
	}

	var tableCount, historyCount int
	if err := store.database.QueryRow(
		"SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'must_rollback'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("query rolled-back table: %v", err)
	}
	if err := store.database.QueryRow(
		"SELECT count(*) FROM schema_migrations WHERE version = 5",
	).Scan(&historyCount); err != nil {
		t.Fatalf("query rolled-back history: %v", err)
	}
	if tableCount != 0 || historyCount != 0 {
		t.Fatalf("partial migration survived: table=%d history=%d", tableCount, historyCount)
	}
}

func TestWriteFailuresAreClassified(t *testing.T) {
	t.Run("full", func(t *testing.T) {
		store, err := Open(
			context.Background(),
			Config{Path: filepath.Join(t.TempDir(), "full.sqlite")},
		)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer store.Close()

		var pages int
		if err := store.database.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
			t.Fatalf("page_count: %v", err)
		}
		if _, err := store.database.Exec(
			fmt.Sprintf("PRAGMA max_page_count = %d", pages+1),
		); err != nil {
			t.Fatalf("max_page_count: %v", err)
		}

		var writeErr error
		for index := range 10_000 {
			writeErr = store.UpsertUser(context.Background(), domain.User{
				ID:          fmt.Sprintf("full-member-%05d", index),
				DisplayName: strings.Repeat("x", 100),
			})
			if writeErr != nil {
				break
			}
		}
		if kind := repository.Classify(writeErr); kind != repository.FailureFull {
			t.Fatalf("write error = %v, class = %q, want %q", writeErr, kind, repository.FailureFull)
		}
	})

	t.Run("read only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "readonly.sqlite")
		writable, err := Open(context.Background(), Config{Path: path})
		if err != nil {
			t.Fatalf("open writable: %v", err)
		}
		if err := writable.Close(); err != nil {
			t.Fatalf("close writable: %v", err)
		}
		readOnly, err := Open(context.Background(), Config{
			Path:           path,
			ReadOnly:       true,
			SkipMigrations: true,
		})
		if err != nil {
			t.Fatalf("open read-only: %v", err)
		}
		defer readOnly.Close()
		err = readOnly.UpsertUser(context.Background(), domain.User{
			ID:          "readonly-member",
			DisplayName: "Read Only",
		})
		if kind := repository.Classify(err); kind != repository.FailureReadOnly {
			t.Fatalf("write error = %v, class = %q, want %q", err, kind, repository.FailureReadOnly)
		}
	})
}

func TestStructurallyCorruptDatabaseIsClassified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.sqlite")
	store, err := Open(context.Background(), Config{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open database for corruption fixture: %v", err)
	}
	_, writeErr := file.WriteAt(make([]byte, 96), 16)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("corrupt database: write=%v close=%v", writeErr, closeErr)
	}

	_, err = Open(context.Background(), Config{Path: path})
	if kind := repository.Classify(err); kind != repository.FailureCorrupt {
		t.Fatalf("Open error = %v, class = %q, want %q", err, kind, repository.FailureCorrupt)
	}
}
