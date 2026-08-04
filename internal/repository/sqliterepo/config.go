package sqliterepo

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const sqliteHeader = "SQLite format 3\x00"

var (
	errNonSQLite      = errors.New("SQLite path contains non-SQLite bytes")
	errNotRegularFile = errors.New("SQLite path must be a regular file")
)

// Config controls one SQLite database. The defaults deliberately favor a
// small single-process deployment over maximum connection fan-out.
type Config struct {
	Path           string
	MaxOpenConns   int
	BusyTimeout    time.Duration
	ReadOnly       bool
	SkipMigrations bool
}

func (config Config) withDefaults() (Config, error) {
	config.Path = strings.TrimSpace(config.Path)
	if config.Path == "" {
		return Config{}, errors.New("sqlite path must not be empty")
	}
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns < 1 || config.MaxOpenConns > 32 {
		return Config{}, errors.New("sqlite max open connections must be between 1 and 32")
	}
	if config.BusyTimeout == 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout < time.Millisecond || config.BusyTimeout > 30*time.Second {
		return Config{}, errors.New("sqlite busy timeout must be between 1ms and 30s")
	}
	if config.ReadOnly && !config.SkipMigrations {
		return Config{}, errors.New("read-only SQLite requires skipped migrations")
	}
	return config, nil
}

func (config Config) dsn() (string, error) {
	absolutePath, err := filepath.Abs(config.Path)
	if err != nil {
		return "", fmt.Errorf("absolute SQLite path: %w", err)
	}
	mode := "rwc"
	if config.ReadOnly {
		mode = "ro"
	}

	location := &url.URL{Scheme: "file", Path: absolutePath}
	query := location.Query()
	query.Set("mode", mode)
	query.Add("_pragma", "busy_timeout("+strconv.FormatInt(config.BusyTimeout.Milliseconds(), 10)+")")
	query.Add("_pragma", "foreign_keys(1)")
	if config.ReadOnly {
		query.Add("_pragma", "query_only(1)")
	} else {
		query.Add("_pragma", "synchronous(FULL)")
		query.Add("_pragma", "journal_mode(WAL)")
	}
	location.RawQuery = query.Encode()
	return location.String(), nil
}

func validateExistingFile(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open SQLite path: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat SQLite path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errNotRegularFile
	}
	if info.Size() == 0 {
		return nil
	}

	header := make([]byte, len(sqliteHeader))
	count, err := file.Read(header)
	if err != nil {
		return fmt.Errorf("read SQLite header: %w", err)
	}
	if count != len(header) || string(header) != sqliteHeader {
		return errNonSQLite
	}
	return nil
}
