// Command ooshdaq-qualify runs the deterministic SQLite capacity lane and
// emits its observed evidence as JSON.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/qualification"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ooshdaq-qualify:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		mode            = flag.String("mode", "full", "qualification mode: full or smoke")
		databasePath    = flag.String("database", "", "new SQLite database path (required)")
		reportPath      = flag.String("report", "", "new JSON report path (default stdout)")
		users           = flag.Int("users", 0, "override synthetic user count")
		brands          = flag.Int("brands", 0, "override fictional brand count")
		catalogs        = flag.Int("catalogs", 0, "override fictional catalog count")
		itemsPerCatalog = flag.Int("items-per-catalog", 0, "override items per catalog")
		collectionRows  = flag.Int("collection-rows", 0, "override sparse collection row count")
		workers         = flag.Int("workers", 0, "override concurrent worker count")
		duration        = flag.Duration("duration", 0, "override measured workload duration")
		memberThinkTime = flag.String(
			"member-think-time",
			"",
			"override pause after each member operation (full default 100ms)",
		)
		seed         = flag.Int64("seed", 0, "override deterministic random seed")
		maxOpenConns = flag.Int("max-open-conns", 0, "override SQLite connection-pool bound")
		busyTimeout  = flag.Duration("busy-timeout", 0, "override SQLite busy timeout")
		gomaxprocs   = flag.Int("gomaxprocs", 1, "Go scheduler CPU limit")
		goMemoryMiB  = flag.Int64("go-memory-limit-mib", 220, "Go soft memory limit in MiB")
	)
	flag.Parse()
	if *databasePath == "" {
		return errors.New("-database is required")
	}
	var config qualification.Config
	switch *mode {
	case "full":
		config = qualification.FullConfig(*databasePath)
	case "smoke":
		config = qualification.SmokeConfig(*databasePath)
	default:
		return fmt.Errorf("-mode must be full or smoke, got %q", *mode)
	}
	var thinkTimeOverride *time.Duration
	if *memberThinkTime != "" {
		parsed, err := time.ParseDuration(*memberThinkTime)
		if err != nil {
			return fmt.Errorf("parse -member-think-time: %w", err)
		}
		thinkTimeOverride = &parsed
	}
	applyOverrides(
		&config,
		*users,
		*brands,
		*catalogs,
		*itemsPerCatalog,
		*collectionRows,
		*workers,
		*duration,
		thinkTimeOverride,
		*seed,
		*maxOpenConns,
		*busyTimeout,
	)
	if *gomaxprocs < 1 {
		return errors.New("-gomaxprocs must be positive")
	}
	if *goMemoryMiB < 1 {
		return errors.New("-go-memory-limit-mib must be positive")
	}
	runtime.GOMAXPROCS(*gomaxprocs)
	debug.SetMemoryLimit(*goMemoryMiB << 20)

	report, runErr := qualification.Run(context.Background(), config)
	raw, encodeErr := report.JSON()
	if encodeErr != nil {
		return errors.Join(runErr, fmt.Errorf("encode report: %w", encodeErr))
	}
	raw = append(raw, '\n')
	if err := writeReport(*reportPath, raw); err != nil {
		return errors.Join(runErr, err)
	}
	return runErr
}

func applyOverrides(
	config *qualification.Config,
	users int,
	brands int,
	catalogs int,
	itemsPerCatalog int,
	collectionRows int,
	workers int,
	duration time.Duration,
	memberThinkTime *time.Duration,
	seed int64,
	maxOpenConns int,
	busyTimeout time.Duration,
) {
	if users > 0 {
		config.Users = users
	}
	if brands > 0 {
		config.Brands = brands
	}
	if catalogs > 0 {
		config.Catalogs = catalogs
	}
	if itemsPerCatalog > 0 {
		config.ItemsPerCatalog = itemsPerCatalog
	}
	if collectionRows > 0 {
		config.CollectionRows = collectionRows
	}
	if workers > 0 {
		config.Workers = workers
	}
	if duration > 0 {
		config.Duration = duration
	}
	if memberThinkTime != nil {
		config.MemberThinkTime = *memberThinkTime
	}
	if seed != 0 {
		config.Seed = seed
	}
	if maxOpenConns > 0 {
		config.MaxOpenConns = maxOpenConns
	}
	if busyTimeout > 0 {
		config.BusyTimeout = busyTimeout
	}
}

func writeReport(path string, raw []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(raw)
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return fmt.Errorf("create report %q: %w", path, err)
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return fmt.Errorf("write report %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync report %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close report %q: %w", path, err)
	}
	return nil
}
