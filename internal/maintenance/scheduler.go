// Package maintenance coordinates bounded SQLite backup and offline restore
// operations around the repository primitives.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	"github.com/PatrickQuintal/ooshdaq/internal/repository/sqliterepo"
)

var completedBackupName = regexp.MustCompile(
	`^ooshdaq-\d{8}T\d{6}\.\d{9}Z\.sqlite$`,
)

const defaultBackupRetryDelay = time.Minute

type onlineBackuper interface {
	Backup(context.Context, string) (sqliterepo.BackupResult, error)
}

type SchedulerConfig struct {
	Directory      string
	Interval       time.Duration
	RetentionCount int
	Logger         *slog.Logger
}

type Scheduler struct {
	store  onlineBackuper
	config SchedulerConfig
	now    func() time.Time

	started      chan struct{}
	stop         chan struct{}
	done         chan struct{}
	cancelActive context.CancelFunc
	startOnce    sync.Once
	stopOnce     sync.Once
	retryDelay   time.Duration
}

func NewScheduler(store onlineBackuper, config SchedulerConfig) (*Scheduler, error) {
	if store == nil {
		return nil, errors.New("maintenance: backup store is required")
	}
	config.Directory = filepath.Clean(config.Directory)
	if config.Directory == "." || config.Directory == string(filepath.Separator) {
		return nil, errors.New("maintenance: backup directory must be explicit")
	}
	if config.Interval < time.Minute || config.Interval > 31*24*time.Hour {
		return nil, errors.New("maintenance: backup interval must be between one minute and 31 days")
	}
	if config.RetentionCount < 1 || config.RetentionCount > 365 {
		return nil, errors.New("maintenance: backup retention must be between 1 and 365")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	return &Scheduler{
		store:      store,
		config:     config,
		now:        time.Now,
		started:    make(chan struct{}),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		retryDelay: defaultBackupRetryDelay,
	}, nil
}

// Start launches one immediate backup followed by the configured interval.
func (scheduler *Scheduler) Start(parent context.Context) {
	scheduler.startOnce.Do(func() {
		operationContext, cancel := context.WithCancel(parent)
		scheduler.cancelActive = cancel
		close(scheduler.started)
		go scheduler.loop(operationContext)
	})
}

// Stop prevents new backups and waits for an active backup to finish or the
// caller's shutdown deadline to expire.
func (scheduler *Scheduler) Stop(ctx context.Context) error {
	select {
	case <-scheduler.started:
	default:
		return nil
	}
	scheduler.stopOnce.Do(func() {
		close(scheduler.stop)
	})
	select {
	case <-scheduler.done:
		scheduler.cancelActive()
		return nil
	case <-ctx.Done():
		scheduler.cancelActive()
		return ctx.Err()
	}
}

func (scheduler *Scheduler) loop(ctx context.Context) {
	defer close(scheduler.done)
	scheduler.runWithRetry(ctx)
	ticker := time.NewTicker(scheduler.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-scheduler.stop:
			return
		case <-ticker.C:
			scheduler.runWithRetry(ctx)
		}
	}
}

func (scheduler *Scheduler) runWithRetry(ctx context.Context) {
	err := scheduler.runOnce(ctx)
	if err == nil {
		return
	}
	failureClass, retryable := backupFailureClass(ctx, err)
	if failureClass == "canceled" || failureClass == "deadline" {
		return
	}
	if !retryable {
		scheduler.logBackupFailure(failureClass, 1, 0, false)
		return
	}

	scheduler.logBackupFailure(failureClass, 1, scheduler.retryDelay, true)
	if !scheduler.waitForRetry(ctx, scheduler.retryDelay) {
		return
	}
	err = scheduler.runOnce(ctx)
	if err == nil {
		return
	}
	failureClass, _ = backupFailureClass(ctx, err)
	if failureClass != "canceled" && failureClass != "deadline" {
		scheduler.logBackupFailure(failureClass, 2, 0, false)
	}
}

func (scheduler *Scheduler) runOnce(ctx context.Context) error {
	started := scheduler.now().UTC()
	destination := filepath.Join(
		scheduler.config.Directory,
		"ooshdaq-"+started.Format("20060102T150405.000000000Z")+".sqlite",
	)
	result, err := scheduler.store.Backup(ctx, destination)
	if err != nil {
		return err
	}
	scheduler.config.Logger.Info(
		"SQLite online backup complete",
		"path",
		filepath.Base(result.Path),
		"size_bytes",
		result.SizeBytes,
		"sha256",
		result.SHA256,
		"duration_ms",
		scheduler.now().Sub(started).Milliseconds(),
	)
	if err := retainNewest(scheduler.config.Directory, scheduler.config.RetentionCount); err != nil {
		failureClass, _ := backupFailureClass(ctx, err)
		scheduler.config.Logger.Error(
			"SQLite backup retention failed",
			"operation",
			"backup_retention",
			"failure_class",
			failureClass,
		)
	}
	return nil
}

func (scheduler *Scheduler) waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-scheduler.stop:
		return false
	case <-timer.C:
		return true
	}
}

func (scheduler *Scheduler) logBackupFailure(
	class string,
	attempt int,
	retryAfter time.Duration,
	retryScheduled bool,
) {
	scheduler.config.Logger.Error(
		"SQLite online backup failed",
		"operation",
		"online_backup",
		"failure_class",
		class,
		"attempt",
		attempt,
		"retry_scheduled",
		retryScheduled,
		"retry_in_ms",
		retryAfter.Milliseconds(),
	)
}

func backupFailureClass(ctx context.Context, err error) (string, bool) {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "canceled", false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline", false
	}
	if errors.Is(err, os.ErrPermission) {
		return "permission", true
	}
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) {
		return "storage_full", true
	}
	switch repository.Classify(err) {
	case repository.FailureBusy:
		return "busy", true
	case repository.FailureUnavailable:
		return "unavailable", true
	case repository.FailureReadOnly:
		return "read_only", true
	case repository.FailureFull:
		return "storage_full", true
	case repository.FailureCorrupt:
		return "corrupt", false
	case repository.FailureIncompatible:
		return "incompatible", false
	default:
		return "unknown", false
	}
}

func retainNewest(directory string, keep int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read backup directory: %w", err)
	}
	completed := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && completedBackupName.MatchString(entry.Name()) {
			completed = append(completed, entry.Name())
		}
	}
	sort.Strings(completed)
	if len(completed) <= keep {
		return nil
	}
	for _, name := range completed[:len(completed)-keep] {
		path := filepath.Join(directory, name)
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove expired backup %q: %w", name, err)
		}
	}
	return syncDirectory(directory)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
