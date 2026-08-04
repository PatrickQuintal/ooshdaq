package sqliterepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var errSchemaIncompatible = errors.New("SQLite schema is incompatible")

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration %q must start with a numeric version", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration %q has an invalid version", entry.Name())
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		digest := sha256.Sum256(body)
		migrations = append(migrations, migration{
			version:  version,
			name:     entry.Name(),
			sql:      string(body),
			checksum: hex.EncodeToString(digest[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	for index, candidate := range migrations {
		expected := index + 1
		if candidate.version != expected {
			return nil, fmt.Errorf(
				"migration versions must be contiguous: got %d, want %d",
				candidate.version,
				expected,
			)
		}
	}
	if len(migrations) == 0 {
		return nil, errors.New("no embedded SQLite migrations")
	}
	return migrations, nil
}

func migrate(ctx context.Context, database *sql.DB, now time.Time) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	hasTable, err := schemaMigrationsExists(ctx, database)
	if err != nil {
		return err
	}
	if !hasTable {
		empty, err := databaseIsEmpty(ctx, database)
		if err != nil {
			return err
		}
		if !empty {
			return fmt.Errorf(
				"%w: database has tables but no schema migration history",
				errSchemaIncompatible,
			)
		}
	}

	applied := make(map[int]migration)
	if hasTable {
		rows, err := database.QueryContext(
			ctx,
			"SELECT version, name, checksum FROM schema_migrations ORDER BY version",
		)
		if err != nil {
			return fmt.Errorf("read schema migration history: %w", err)
		}
		for rows.Next() {
			var record migration
			if err := rows.Scan(&record.version, &record.name, &record.checksum); err != nil {
				rows.Close()
				return fmt.Errorf("scan schema migration history: %w", err)
			}
			applied[record.version] = record
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close schema migration history: %w", err)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate schema migration history: %w", err)
		}
	}

	for version, record := range applied {
		if version > len(migrations) {
			return fmt.Errorf(
				"%w: schema version %d is newer than supported version %d",
				errSchemaIncompatible,
				version,
				len(migrations),
			)
		}
		expected := migrations[version-1]
		if record.name != expected.name || record.checksum != expected.checksum {
			return fmt.Errorf(
				"%w: migration %d checksum or name does not match the application",
				errSchemaIncompatible,
				version,
			)
		}
	}
	for _, candidate := range migrations {
		if _, exists := applied[candidate.version]; exists {
			continue
		}
		if err := applyMigration(ctx, database, candidate, now); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, database *sql.DB, candidate migration, now time.Time) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", candidate.version, err)
	}
	defer transaction.Rollback()

	if _, err := transaction.ExecContext(ctx, candidate.sql); err != nil {
		return fmt.Errorf("execute migration %d: %w", candidate.version, err)
	}
	if _, err := transaction.ExecContext(
		ctx,
		`INSERT INTO schema_migrations(version, name, checksum, applied_at)
		 VALUES (?, ?, ?, ?)`,
		candidate.version,
		candidate.name,
		candidate.checksum,
		now.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration %d: %w", candidate.version, err)
	}
	if _, err := transaction.ExecContext(
		ctx,
		"PRAGMA user_version = "+strconv.Itoa(candidate.version),
	); err != nil {
		return fmt.Errorf("record SQLite user version %d: %w", candidate.version, err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", candidate.version, err)
	}
	return nil
}

func schemaMigrationsExists(ctx context.Context, database *sql.DB) (bool, error) {
	var count int
	err := database.QueryRowContext(
		ctx,
		`SELECT count(*) FROM sqlite_schema
		 WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect SQLite schema: %w", err)
	}
	return count == 1, nil
}

func databaseIsEmpty(ctx context.Context, database *sql.DB) (bool, error) {
	var count int
	err := database.QueryRowContext(
		ctx,
		`SELECT count(*) FROM sqlite_schema
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect empty SQLite database: %w", err)
	}
	return count == 0, nil
}
