// Package sqliterepo implements $OOSHDAQ persistence with one SQLite database.
package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	_ "modernc.org/sqlite"
)

// Store owns the database handle and serializes the application's short write
// transactions. Reads continue through database/sql's bounded connection pool.
type Store struct {
	database *sql.DB
	path     string
	writeMu  sync.Mutex
	now      func() time.Time
}

// Open validates the target, opens SQLite, applies migrations, and verifies
// the connection policy before returning a ready store.
func Open(ctx context.Context, config Config) (*Store, error) {
	normalized, err := config.withDefaults()
	if err != nil {
		return nil, err
	}
	if err := validateExistingFile(normalized.Path); err != nil {
		kind := repository.FailureUnavailable
		if errors.Is(err, errNonSQLite) {
			kind = repository.FailureCorrupt
		}
		return nil, repository.NewFailure(kind, "validate SQLite path", err)
	}
	parent := filepath.Dir(normalized.Path)
	if err := preparePrivateDirectory(parent, !normalized.ReadOnly); err != nil {
		return nil, repository.NewFailure(
			repository.FailureUnavailable,
			"protect SQLite directory",
			err,
		)
	}
	if err := preparePrivateDatabase(normalized.Path, !normalized.ReadOnly); err != nil {
		return nil, repository.NewFailure(
			repository.FailureUnavailable,
			"protect SQLite database",
			err,
		)
	}
	dsn, err := normalized.dsn()
	if err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	database.SetMaxOpenConns(normalized.MaxOpenConns)
	database.SetMaxIdleConns(normalized.MaxOpenConns)

	store := &Store{
		database: database,
		path:     normalized.Path,
		now:      time.Now,
	}
	closeOnError := func(openErr error) (*Store, error) {
		if closeErr := database.Close(); closeErr != nil {
			return nil, errors.Join(openErr, fmt.Errorf("close SQLite: %w", closeErr))
		}
		return nil, openErr
	}
	if err := database.PingContext(ctx); err != nil {
		return closeOnError(classify("ping SQLite", err))
	}
	if !normalized.SkipMigrations {
		if err := migrate(ctx, database, store.now()); err != nil {
			if errors.Is(err, errSchemaIncompatible) {
				err = repository.NewFailure(
					repository.FailureIncompatible,
					"migrate SQLite",
					err,
				)
			} else {
				err = classify("migrate SQLite", err)
			}
			return closeOnError(err)
		}
	}
	if normalized.ReadOnly {
		if err := store.verifyReadOnlyPolicy(ctx, normalized.BusyTimeout); err != nil {
			return closeOnError(err)
		}
	} else {
		if err := store.verifyConnectionPolicy(ctx, normalized.BusyTimeout); err != nil {
			return closeOnError(err)
		}
	}
	if err := store.Check(ctx, false); err != nil {
		return closeOnError(err)
	}
	if err := tightenSQLiteArtifacts(store.path, !normalized.ReadOnly); err != nil {
		return closeOnError(repository.NewFailure(
			repository.FailureUnavailable,
			"protect SQLite artifacts",
			err,
		))
	}
	return store, nil
}

// Close releases SQLite connections after the HTTP server has drained.
func (store *Store) Close() error {
	if store == nil || store.database == nil {
		return nil
	}
	return store.database.Close()
}

// Ping performs the bounded schema and catalog reads used by readiness.
func (store *Store) Ping(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return repository.NewFailure(
			repository.FailureIncompatible,
			"load SQLite readiness schema",
			err,
		)
	}
	var schemaVersion, catalogRows int
	if err := store.database.QueryRowContext(
		ctx,
		`SELECT
			COALESCE((SELECT max(version) FROM schema_migrations), 0),
			(SELECT count(*) FROM (
				SELECT id FROM catalogs ORDER BY sort_order, id LIMIT 1
			))`,
	).Scan(&schemaVersion, &catalogRows); err != nil {
		return classify("SQLite readiness query", err)
	}
	if schemaVersion != len(migrations) || catalogRows < 0 || catalogRows > 1 {
		return repository.NewFailure(
			repository.FailureIncompatible,
			"SQLite readiness query",
			fmt.Errorf(
				"schema version %d, expected %d",
				schemaVersion,
				len(migrations),
			),
		)
	}
	return nil
}

// Check verifies foreign keys and, when thorough is true, the full database
// structure. quick_check is appropriate for startup and readiness.
func (store *Store) Check(ctx context.Context, thorough bool) error {
	var table, parent string
	var rowID, foreignKeyID int64
	err := store.database.QueryRowContext(
		ctx,
		`SELECT "table", rowid, parent, fkid
		 FROM pragma_foreign_key_check
		 LIMIT 1`,
	).Scan(&table, &rowID, &parent, &foreignKeyID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return classify("SQLite foreign key check", err)
	}
	if err == nil {
		return repository.NewFailure(
			repository.FailureCorrupt,
			"SQLite foreign key check",
			fmt.Errorf(
				"violation: table=%s row=%d parent=%s constraint=%d",
				table,
				rowID,
				parent,
				foreignKeyID,
			),
		)
	}

	pragma := "PRAGMA quick_check"
	if thorough {
		pragma = "PRAGMA integrity_check"
	}
	rows, err := store.database.QueryContext(ctx, pragma)
	if err != nil {
		return classify("SQLite integrity check", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return classify("scan SQLite integrity result", err)
		}
		if !strings.EqualFold(result, "ok") {
			return repository.NewFailure(
				repository.FailureCorrupt,
				"SQLite integrity check",
				errors.New(result),
			)
		}
	}
	if err := rows.Err(); err != nil {
		return classify("iterate SQLite integrity results", err)
	}
	return nil
}

func (store *Store) verifyConnectionPolicy(
	ctx context.Context,
	timeout time.Duration,
) (returnErr error) {
	connectionCount := store.database.Stats().MaxOpenConnections
	connections := make([]*sql.Conn, 0, connectionCount)
	defer func() {
		for _, connection := range connections {
			if err := connection.Close(); err != nil {
				returnErr = errors.Join(
					returnErr,
					fmt.Errorf("close verified SQLite connection: %w", err),
				)
			}
		}
	}()
	// Hold every connection until the pool is exhausted. Closing each one
	// inside this loop would allow database/sql to return the same physical
	// connection repeatedly and would not verify the configured pool.
	for range connectionCount {
		connection, err := store.database.Conn(ctx)
		if err != nil {
			return fmt.Errorf("acquire SQLite connection for PRAGMA verification: %w", err)
		}
		connections = append(connections, connection)
	}
	for _, connection := range connections {
		var journal string
		var synchronous, foreignKeys, busyTimeout int
		if err := connection.QueryRowContext(
			ctx,
			`SELECT
				(SELECT journal_mode FROM pragma_journal_mode),
				(SELECT synchronous FROM pragma_synchronous),
				(SELECT foreign_keys FROM pragma_foreign_keys),
				(SELECT timeout FROM pragma_busy_timeout)`,
		).Scan(&journal, &synchronous, &foreignKeys, &busyTimeout); err != nil {
			return fmt.Errorf("verify SQLite PRAGMAs: %w", err)
		}
		if journal != "wal" || synchronous != 2 || foreignKeys != 1 ||
			busyTimeout != int(timeout.Milliseconds()) {
			return fmt.Errorf(
				"unexpected SQLite policy journal=%q synchronous=%d foreign_keys=%d busy_timeout=%d",
				journal,
				synchronous,
				foreignKeys,
				busyTimeout,
			)
		}
	}
	return nil
}

func (store *Store) verifyReadOnlyPolicy(ctx context.Context, timeout time.Duration) error {
	connection, err := store.database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire read-only SQLite connection: %w", err)
	}
	defer connection.Close()
	var foreignKeys, busyTimeout, queryOnly int
	if err := connection.QueryRowContext(
		ctx,
		`SELECT
			(SELECT foreign_keys FROM pragma_foreign_keys),
			(SELECT timeout FROM pragma_busy_timeout),
			(SELECT query_only FROM pragma_query_only)`,
	).Scan(&foreignKeys, &busyTimeout, &queryOnly); err != nil {
		return fmt.Errorf("verify read-only SQLite PRAGMAs: %w", err)
	}
	if foreignKeys != 1 || busyTimeout != int(timeout.Milliseconds()) || queryOnly != 1 {
		return fmt.Errorf(
			"unexpected read-only SQLite policy foreign_keys=%d busy_timeout=%d query_only=%d",
			foreignKeys,
			busyTimeout,
			queryOnly,
		)
	}
	return nil
}
