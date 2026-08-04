// Package qualification provides the reproducible synthetic capacity lane for
// the accepted single-process SQLite deployment envelope.
package qualification

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	FullUserCount          = 10_000
	FullBrandCount         = 3
	FullCatalogCount       = 5
	FullItemsPerCatalog    = 40
	FullCollectionRowCount = 80_000
	FullWorkerCount        = 100
	FullDuration           = 60 * time.Second
	FullMemberThinkTime    = 100 * time.Millisecond

	defaultSeed         = int64(20_260_730)
	defaultConnections  = 8
	defaultBusyTimeout  = 5 * time.Second
	targetMemoryBytes   = int64(256 << 20)
	fixtureVersion      = "qualification-v1"
	minimumRowsPerUser  = 3
	minimumCatalogCount = 2
)

// Config defines one qualification run. FullConfig is the release fixture;
// SmokeConfig keeps the same catalog and query shapes with fewer members and a
// shorter duration.
type Config struct {
	DatabasePath    string
	Users           int
	Brands          int
	Catalogs        int
	ItemsPerCatalog int
	CollectionRows  int
	Workers         int
	Duration        time.Duration
	MemberThinkTime time.Duration
	Seed            int64
	MaxOpenConns    int
	BusyTimeout     time.Duration
}

// FullConfig returns the fixed qualification envelope.
func FullConfig(databasePath string) Config {
	return Config{
		DatabasePath:    databasePath,
		Users:           FullUserCount,
		Brands:          FullBrandCount,
		Catalogs:        FullCatalogCount,
		ItemsPerCatalog: FullItemsPerCatalog,
		CollectionRows:  FullCollectionRowCount,
		Workers:         FullWorkerCount,
		Duration:        FullDuration,
		MemberThinkTime: FullMemberThinkTime,
		Seed:            defaultSeed,
		MaxOpenConns:    defaultConnections,
		BusyTimeout:     defaultBusyTimeout,
	}
}

// SmokeConfig returns a fast normal-test lane without claiming release-scale
// evidence.
func SmokeConfig(databasePath string) Config {
	config := FullConfig(databasePath)
	config.Users = 100
	config.CollectionRows = 800
	config.Workers = 4
	config.Duration = 2 * time.Second
	config.MemberThinkTime = 0
	return config
}

func (config Config) validate() error {
	config.DatabasePath = strings.TrimSpace(config.DatabasePath)
	if config.DatabasePath == "" {
		return errors.New("qualification database path must not be empty")
	}
	if filepath.Ext(config.DatabasePath) == "" {
		return errors.New("qualification database path must name a file with an extension")
	}
	if config.Users < 8 || config.Users%2 != 0 {
		return errors.New("qualification users must be an even number of at least eight")
	}
	if config.Brands < 1 || config.Brands > config.Catalogs {
		return errors.New("qualification brands must be between one and the catalog count")
	}
	if config.Catalogs < minimumCatalogCount {
		return fmt.Errorf("qualification catalogs must be at least %d", minimumCatalogCount)
	}
	if config.ItemsPerCatalog < 2 {
		return errors.New("qualification items per catalog must be at least two")
	}
	itemCount := config.Catalogs * config.ItemsPerCatalog
	if config.CollectionRows%config.Users != 0 {
		return errors.New("qualification collection rows must divide evenly across users")
	}
	rowsPerUser := config.CollectionRows / config.Users
	if rowsPerUser < minimumRowsPerUser {
		return fmt.Errorf(
			"qualification rows per user must be at least %d",
			minimumRowsPerUser,
		)
	}
	if rowsPerUser > itemCount {
		return errors.New("qualification rows per user must not exceed the item count")
	}
	if pairCount := config.Users / 2; pairCount > itemCount*(itemCount-1)/2 {
		return errors.New("qualification needs more unique reciprocal item pairs")
	}
	if config.Workers < 1 || config.Workers > FullWorkerCount {
		return fmt.Errorf("qualification workers must be between 1 and %d", FullWorkerCount)
	}
	if config.Duration <= 0 {
		return errors.New("qualification duration must be positive")
	}
	if config.MemberThinkTime < 0 || config.MemberThinkTime > 10*time.Second {
		return errors.New("qualification member think time must be between zero and 10s")
	}
	if config.MaxOpenConns < 1 || config.MaxOpenConns > 32 {
		return errors.New("qualification max open connections must be between 1 and 32")
	}
	if config.BusyTimeout < time.Millisecond || config.BusyTimeout > 30*time.Second {
		return errors.New("qualification busy timeout must be between 1ms and 30s")
	}
	return nil
}

func (config Config) isFixedReleaseShape() bool {
	return config.Users == FullUserCount &&
		config.Brands == FullBrandCount &&
		config.Catalogs == FullCatalogCount &&
		config.ItemsPerCatalog == FullItemsPerCatalog &&
		config.CollectionRows == FullCollectionRowCount &&
		config.Seed == defaultSeed
}

func (config Config) isFixedReleaseWorkload() bool {
	return config.Workers == FullWorkerCount &&
		config.Duration == FullDuration &&
		config.MemberThinkTime == FullMemberThinkTime &&
		config.MaxOpenConns == defaultConnections &&
		config.BusyTimeout == defaultBusyTimeout
}
