package maintenance

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	"github.com/PatrickQuintal/ooshdaq/internal/repository/sqliterepo"
)

type fakeBackuper struct {
	destinations chan string
}

func (fake *fakeBackuper) Backup(
	_ context.Context,
	destination string,
) (sqliterepo.BackupResult, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return sqliterepo.BackupResult{}, err
	}
	if err := os.WriteFile(destination, []byte("fixture"), 0o600); err != nil {
		return sqliterepo.BackupResult{}, err
	}
	if fake.destinations != nil {
		fake.destinations <- destination
	}
	return sqliterepo.BackupResult{
		Path:      destination,
		SizeBytes: 7,
		SHA256:    "fixture",
		CreatedAt: time.Now(),
	}, nil
}

func TestSchedulerPublishesAndRetainsOnlyOwnedBackupNames(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	scheduler, err := NewScheduler(&fakeBackuper{}, SchedulerConfig{
		Directory:      directory,
		Interval:       time.Hour,
		RetentionCount: 2,
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	now := time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC)
	scheduler.now = func() time.Time {
		value := now
		now = now.Add(time.Second)
		return value
	}
	if err := os.WriteFile(
		filepath.Join(directory, "operator-notes.txt"),
		[]byte("keep"),
		0o600,
	); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	for range 3 {
		scheduler.runOnce(context.Background())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var backups int
	for _, entry := range entries {
		if completedBackupName.MatchString(entry.Name()) {
			backups++
		}
	}
	if backups != 2 {
		t.Fatalf("retained backups = %d, want 2", backups)
	}
	if _, err := os.Stat(filepath.Join(directory, "operator-notes.txt")); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestSchedulerStartAndStopDrainsImmediateBackup(t *testing.T) {
	t.Parallel()

	fake := &drainingBackuper{
		started: make(chan context.Context, 1),
		release: make(chan struct{}),
	}
	scheduler, err := NewScheduler(fake, SchedulerConfig{
		Directory:      t.TempDir(),
		Interval:       time.Hour,
		RetentionCount: 1,
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	scheduler.Start(context.Background())
	var operationContext context.Context
	select {
	case operationContext = <-fake.started:
	case <-time.After(2 * time.Second):
		t.Fatal("immediate backup did not start")
	}
	stopResult := make(chan error, 1)
	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		stopResult <- scheduler.Stop(stopContext)
	}()
	select {
	case <-scheduler.stop:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not signal the scheduler")
	}
	if err := operationContext.Err(); err != nil {
		t.Fatalf("Stop canceled the active backup: %v", err)
	}
	close(fake.release)
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not drain the immediate backup")
	}
	if !fake.completed {
		t.Fatal("immediate backup did not complete")
	}
}

func TestSchedulerRetriesOnceAndRecovers(t *testing.T) {
	t.Parallel()

	const sensitiveDetail = "/private/member-backups: operator detail"
	backuper := &sequenceBackuper{failures: []error{
		fmt.Errorf("%s: %w", sensitiveDetail, syscall.ENOSPC),
	}}
	var logs bytes.Buffer
	scheduler, err := NewScheduler(backuper, SchedulerConfig{
		Directory:      t.TempDir(),
		Interval:       time.Hour,
		RetentionCount: 1,
		Logger: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	scheduler.retryDelay = time.Millisecond
	scheduler.runWithRetry(context.Background())

	if backuper.calls != 2 {
		t.Fatalf("backup attempts = %d, want 2", backuper.calls)
	}
	logged := logs.String()
	for _, want := range []string{
		`"operation":"online_backup"`,
		`"failure_class":"storage_full"`,
		`"retry_scheduled":true`,
		`SQLite online backup complete`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("logs do not contain %q:\n%s", want, logged)
		}
	}
	if strings.Contains(logged, sensitiveDetail) {
		t.Fatalf("logs exposed wrapped failure detail: %s", logged)
	}
}

func TestSchedulerCapsRetryAndDoesNotRetryCancellation(t *testing.T) {
	t.Parallel()

	t.Run("capped unavailable retry", func(t *testing.T) {
		backuper := &sequenceBackuper{failures: []error{
			repository.ErrUnavailable,
			repository.ErrUnavailable,
		}}
		scheduler := newTestScheduler(t, backuper)
		var logs bytes.Buffer
		scheduler.config.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
		scheduler.retryDelay = time.Millisecond
		scheduler.runWithRetry(context.Background())
		if backuper.calls != 2 {
			t.Fatalf("backup attempts = %d, want initial plus one retry", backuper.calls)
		}
		for _, want := range []string{`"attempt":2`, `"retry_scheduled":false`} {
			if !strings.Contains(logs.String(), want) {
				t.Errorf("exhausted retry log does not contain %q: %s", want, logs.String())
			}
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		backuper := &sequenceBackuper{failures: []error{context.Canceled}}
		scheduler := newTestScheduler(t, backuper)
		scheduler.retryDelay = time.Millisecond
		scheduler.runWithRetry(context.Background())
		if backuper.calls != 1 {
			t.Fatalf("backup attempts = %d, want 1", backuper.calls)
		}
	})

	t.Run("stop interrupts retry wait", func(t *testing.T) {
		backuper := &retryWaitBackuper{attempts: make(chan struct{}, 1)}
		scheduler := newTestScheduler(t, backuper)
		scheduler.retryDelay = time.Hour
		scheduler.Start(context.Background())
		select {
		case <-backuper.attempts:
		case <-time.After(time.Second):
			t.Fatal("initial backup did not run")
		}

		stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := scheduler.Stop(stopContext); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		select {
		case <-backuper.attempts:
			t.Fatal("backup retried after Stop")
		case <-time.After(20 * time.Millisecond):
		}
	})
}

func TestBackupFailureClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantClass string
		wantRetry bool
	}{
		{name: "permission", err: os.ErrPermission, wantClass: "permission", wantRetry: true},
		{name: "full", err: syscall.ENOSPC, wantClass: "storage_full", wantRetry: true},
		{name: "canceled", err: context.Canceled, wantClass: "canceled", wantRetry: false},
		{name: "corrupt", err: repository.ErrCorrupt, wantClass: "corrupt", wantRetry: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			class, retry := backupFailureClass(context.Background(), test.err)
			if class != test.wantClass || retry != test.wantRetry {
				t.Fatalf("backupFailureClass() = (%q, %t), want (%q, %t)", class, retry, test.wantClass, test.wantRetry)
			}
		})
	}
}

type sequenceBackuper struct {
	failures []error
	calls    int
}

type retryWaitBackuper struct {
	attempts chan struct{}
}

func (fake *retryWaitBackuper) Backup(
	context.Context,
	string,
) (sqliterepo.BackupResult, error) {
	fake.attempts <- struct{}{}
	return sqliterepo.BackupResult{}, repository.ErrUnavailable
}

func (fake *sequenceBackuper) Backup(
	ctx context.Context,
	destination string,
) (sqliterepo.BackupResult, error) {
	fake.calls++
	if fake.calls <= len(fake.failures) {
		return sqliterepo.BackupResult{}, fake.failures[fake.calls-1]
	}
	return (&fakeBackuper{}).Backup(ctx, destination)
}

func newTestScheduler(t *testing.T, backuper onlineBackuper) *Scheduler {
	t.Helper()
	scheduler, err := NewScheduler(backuper, SchedulerConfig{
		Directory:      t.TempDir(),
		Interval:       time.Hour,
		RetentionCount: 1,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	return scheduler
}

type drainingBackuper struct {
	started   chan context.Context
	release   chan struct{}
	completed bool
}

func (fake *drainingBackuper) Backup(
	ctx context.Context,
	destination string,
) (sqliterepo.BackupResult, error) {
	fake.started <- ctx
	select {
	case <-ctx.Done():
		return sqliterepo.BackupResult{}, ctx.Err()
	case <-fake.release:
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return sqliterepo.BackupResult{}, err
	}
	if err := os.WriteFile(destination, []byte("fixture"), 0o600); err != nil {
		return sqliterepo.BackupResult{}, err
	}
	fake.completed = true
	return sqliterepo.BackupResult{
		Path:      destination,
		SizeBytes: 7,
		SHA256:    "fixture",
		CreatedAt: time.Now(),
	}, nil
}
