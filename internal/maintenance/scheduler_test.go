package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
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
