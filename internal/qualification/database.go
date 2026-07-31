package qualification

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

func openQualificationDatabase(config Config) (*sql.DB, error) {
	absolutePath, err := filepath.Abs(config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve qualification database path: %w", err)
	}
	location := &url.URL{Scheme: "file", Path: absolutePath}
	query := location.Query()
	query.Set("mode", "rw")
	query.Add(
		"_pragma",
		"busy_timeout("+strconv.FormatInt(config.BusyTimeout.Milliseconds(), 10)+")",
	)
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "synchronous(FULL)")
	query.Add("_pragma", "journal_mode(WAL)")
	location.RawQuery = query.Encode()

	database, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, fmt.Errorf("open qualification SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	return database, nil
}
