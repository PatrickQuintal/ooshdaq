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
	"time"

	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
)

var completedBackupName = regexp.MustCompile(
	`^ooshdaq-\d{8}T\d{6}\.\d{9}Z\.sqlite$`,
)

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
		store:   store,
		config:  config,
		now:     time.Now,
		started: make(chan struct{}),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
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
	scheduler.runOnce(ctx)
	ticker := time.NewTicker(scheduler.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-scheduler.stop:
			return
		case <-ticker.C:
			scheduler.runOnce(ctx)
		}
	}
}

func (scheduler *Scheduler) runOnce(ctx context.Context) {
	started := scheduler.now().UTC()
	destination := filepath.Join(
		scheduler.config.Directory,
		"ooshdaq-"+started.Format("20060102T150405.000000000Z")+".sqlite",
	)
	result, err := scheduler.store.Backup(ctx, destination)
	if err != nil {
		if ctx.Err() == nil {
			scheduler.config.Logger.Error(
				"SQLite online backup failed",
				"error_type",
				fmt.Sprintf("%T", err),
			)
		}
		return
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
		scheduler.config.Logger.Error(
			"SQLite backup retention failed",
			"error_type",
			fmt.Sprintf("%T", err),
		)
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
