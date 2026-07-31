package sqliterepo

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCollectionStateMigrationNormalizesLegacyConflicts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-state.sqlite")
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) < 4 {
		t.Fatalf("migration count = %d, want at least 4", len(migrations))
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := applyMigration(ctx, database, migrations[0], time.Now()); err != nil {
		database.Close()
		t.Fatalf("apply legacy migration: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO brands(id, name, status)
			VALUES ('legacy-brand', 'Legacy Brand', 'active');
		INSERT INTO catalogs(id, brand_id, name, status, sort_order)
			VALUES ('legacy-catalog', 'legacy-brand', 'Legacy Catalog', 'active', 1);
		INSERT INTO catalog_items(id, catalog_id, number, name, status)
			VALUES ('legacy-item', 'legacy-catalog', 1, 'Legacy Item', 'active');
		INSERT INTO users(id, display_name, created_at, updated_at)
			VALUES ('legacy-member', 'Legacy Member', '2026-07-31', '2026-07-31');
		INSERT INTO collection_entries(
			user_id, item_id, owned, available, wanted, updated_at
		) VALUES ('legacy-member', 'legacy-item', 3, 2, 4, '2026-07-31');
	`); err != nil {
		database.Close()
		t.Fatalf("seed legacy conflict: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("Open migrated database: %v", err)
	}
	defer store.Close()
	var owned, available, wanted int
	if err := store.database.QueryRowContext(
		ctx,
		`SELECT owned, available, wanted
		 FROM collection_entries
		 WHERE user_id = 'legacy-member' AND item_id = 'legacy-item'`,
	).Scan(&owned, &available, &wanted); err != nil {
		t.Fatalf("read normalized state: %v", err)
	}
	if owned != 0 || available != 0 || wanted != 4 {
		t.Fatalf(
			"normalized state = owned %d, available %d, wanted %d; want 0/0/4",
			owned,
			available,
			wanted,
		)
	}
	var availableSince, wantedSince, lastSeen sql.NullString
	if err := store.database.QueryRowContext(
		ctx,
		`SELECT entry.available_since_at, entry.wanted_since_at, user.last_seen_at
		 FROM collection_entries entry
		 JOIN users user ON user.id = entry.user_id
		 WHERE entry.user_id = 'legacy-member' AND entry.item_id = 'legacy-item'`,
	).Scan(&availableSince, &wantedSince, &lastSeen); err != nil {
		t.Fatalf("read migrated timing: %v", err)
	}
	if availableSince.Valid ||
		!wantedSince.Valid ||
		wantedSince.String != "2026-07-31" ||
		lastSeen.Valid {
		t.Fatalf(
			"migrated timing = available %#v, wanted %#v, last seen %#v",
			availableSince,
			wantedSince,
			lastSeen,
		)
	}
	// Migration 002 historically normalized this v1 conflict before migration
	// 003 relaxed owned+wanted coexistence. Keep that checksum-stable history,
	// then verify the current schema accepts the newly supported state.
	if _, err := store.database.ExecContext(
		ctx,
		`UPDATE collection_entries
		 SET owned = 1, wanted = 1
		 WHERE user_id = 'legacy-member' AND item_id = 'legacy-item'`,
	); err != nil {
		t.Fatalf("migrated schema rejected wanted and owned together: %v", err)
	}
	if _, err := store.database.ExecContext(
		ctx,
		`UPDATE collection_entries
		 SET available = 1
		 WHERE user_id = 'legacy-member' AND item_id = 'legacy-item'`,
	); err == nil {
		t.Fatal("migrated schema accepted wanted and available together")
	}
}

func TestOpenMigratesAndReopensDurably(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "database with spaces.sqlite")
	store, err := Open(ctx, Config{
		Path:         path,
		MaxOpenConns: 4,
		BusyTimeout:  750 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var version int
	if err := store.database.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("query user_version: %v", err)
	}
	if version != 4 {
		t.Fatalf("user_version = %d, want 4", version)
	}
	if _, err := store.database.ExecContext(ctx, `
		INSERT INTO brands(id, name, status) VALUES ('fixture-brand', 'Fixture Brand', 'active');
		INSERT INTO catalogs(id, brand_id, name, status, sort_order)
			VALUES ('fixture-catalog', 'fixture-brand', 'Fixture Catalog', 'active', 1);
		INSERT INTO catalog_items(id, catalog_id, number, name, status)
			VALUES ('fixture-item', 'fixture-catalog', 1, 'Fixture Item', 'active');
		INSERT INTO item_assets(
			item_id, placeholder, rights_status, reviewed_at, review_reference, transformation
		) VALUES (
			'fixture-item', 1, 'unknown', '2026-07-30', 'test', 'placeholder'
		);
		INSERT INTO users(id, display_name, created_at, updated_at)
			VALUES ('member', 'Member', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');
		INSERT INTO collection_entries(
			user_id, item_id, owned, available, available_since_at,
			wanted, wanted_since_at, updated_at
		) VALUES (
			'member', 'fixture-item', 2, 1, '2026-07-30T00:00:00Z',
			0, NULL, '2026-07-30T00:00:00Z'
		);
	`); err != nil {
		t.Fatalf("insert durable fixture: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	var available int
	if err := reopened.database.QueryRowContext(
		ctx,
		`SELECT available FROM collection_entries
		 WHERE user_id = 'member' AND item_id = 'fixture-item'`,
	).Scan(&available); err != nil {
		t.Fatalf("read reopened entry: %v", err)
	}
	if available != 1 {
		t.Fatalf("available = %d, want 1", available)
	}
	if err := reopened.Check(ctx, true); err != nil {
		t.Fatalf("thorough Check: %v", err)
	}
}

func TestOpenRejectsNonSQLiteBytes(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "not.sqlite")
	if err := os.WriteFile(path, []byte("definitely not a SQLite file"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := Open(context.Background(), Config{Path: path})
	if err == nil || !strings.Contains(err.Error(), "non-SQLite") {
		t.Fatalf("Open error = %v, want non-SQLite rejection", err)
	}
}

func TestOpenRejectsChangedOrNewerMigrationHistory(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		change      string
		wantInError string
	}{
		{
			name:        "changed checksum",
			change:      "UPDATE schema_migrations SET checksum = printf('%064d', 0) WHERE version = 1",
			wantInError: "checksum",
		},
		{
			name: "newer version",
			change: `INSERT INTO schema_migrations(version, name, checksum, applied_at)
				VALUES (5, '005_future.sql', printf('%064d', 0), '2026-07-30T00:00:00Z')`,
			wantInError: "newer",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "history.sqlite")
			store, err := Open(context.Background(), Config{Path: path})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if _, err := store.database.Exec(test.change); err != nil {
				t.Fatalf("mutate migration history: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			_, err = Open(context.Background(), Config{Path: path})
			if err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("reopen error = %v, want %q", err, test.wantInError)
			}
		})
	}
}

func TestEveryPhysicalConnectionUsesRequiredPragmas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, Config{
		Path:         filepath.Join(t.TempDir(), "connections.sqlite"),
		MaxOpenConns: 4,
		BusyTimeout:  625 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	connections := make([]interface{ Close() error }, 0, 4)
	for range 4 {
		connection, err := store.database.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn: %v", err)
		}
		connections = append(connections, connection)
		var foreignKeys, synchronous, busyTimeout int
		var journal string
		if err := connection.QueryRowContext(
			ctx,
			`SELECT
				(SELECT journal_mode FROM pragma_journal_mode),
				(SELECT synchronous FROM pragma_synchronous),
				(SELECT foreign_keys FROM pragma_foreign_keys),
				(SELECT timeout FROM pragma_busy_timeout)`,
		).Scan(&journal, &synchronous, &foreignKeys, &busyTimeout); err != nil {
			t.Fatalf("query PRAGMAs: %v", err)
		}
		if journal != "wal" || synchronous != 2 || foreignKeys != 1 || busyTimeout != 625 {
			t.Fatalf(
				"PRAGMAs = journal=%q synchronous=%d foreign_keys=%d busy_timeout=%d",
				journal,
				synchronous,
				foreignKeys,
				busyTimeout,
			)
		}
	}
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Errorf("close connection: %v", err)
		}
	}
}

func TestOpenReadOnlyCannotMutateVerifiedDatabase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "readonly.sqlite")
	writable, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatalf("open writable database: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable database: %v", err)
	}

	readOnly, err := Open(ctx, Config{
		Path:           path,
		ReadOnly:       true,
		SkipMigrations: true,
	})
	if err != nil {
		t.Fatalf("open read-only database: %v", err)
	}
	defer readOnly.Close()
	if _, err := readOnly.database.ExecContext(
		ctx,
		"INSERT INTO users(id, display_name, created_at, updated_at) VALUES ('nope', 'Nope', '', '')",
	); err == nil {
		t.Fatal("read-only database accepted a write")
	}
}

func TestReadinessReadsRequiredSchema(t *testing.T) {
	t.Parallel()

	store, err := Open(
		context.Background(),
		Config{Path: filepath.Join(t.TempDir(), "readiness.sqlite")},
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if err := store.Ping(context.Background()); err != nil {
		t.Fatalf("initial Ping: %v", err)
	}
	if _, err := store.database.Exec(
		"ALTER TABLE catalogs RENAME TO catalogs_unavailable",
	); err != nil {
		t.Fatalf("break required schema: %v", err)
	}
	if err := store.Ping(context.Background()); err == nil {
		t.Fatal("Ping succeeded without the required catalogs table")
	}
}

func TestSchemaEnforcesHistoryReferencesAndSparseQuantities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, Config{Path: filepath.Join(t.TempDir(), "constraints.sqlite")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if _, err := store.database.ExecContext(ctx, `
		INSERT INTO brands(id, name, status) VALUES ('history-brand', 'History Brand', 'active');
		INSERT INTO catalogs(id, brand_id, name, status, sort_order)
			VALUES ('history-catalog', 'history-brand', 'History Catalog', 'active', 10);
		INSERT INTO catalog_items(id, catalog_id, number, name, status)
			VALUES ('history-item', 'history-catalog', 1, 'History Item', 'active');
		INSERT INTO item_assets(
			item_id, placeholder, rights_status, reviewed_at, review_reference, transformation
		) VALUES ('history-item', 1, 'unknown', '2026-07-30', 'test', 'none');
		INSERT INTO users(id, display_name, created_at, updated_at)
			VALUES ('history-member', 'History Member', '2026-07-30', '2026-07-30');
	`); err != nil {
		t.Fatalf("seed constraints: %v", err)
	}

	for name, statement := range map[string]string{
		"foreign item": `INSERT INTO collection_entries(
				user_id, item_id, owned, available, wanted, updated_at
			) VALUES ('history-member', 'missing-item', 1, 0, 0, '2026-07-30')`,
		"zero sparse entry": `INSERT INTO collection_entries(
				user_id, item_id, owned, available, wanted, updated_at
			) VALUES ('history-member', 'history-item', 0, 0, 0, '2026-07-30')`,
		"available above owned": `INSERT INTO collection_entries(
				user_id, item_id, owned, available, available_since_at, wanted, updated_at
			) VALUES ('history-member', 'history-item', 1, 2, '2026-07-30', 0, '2026-07-30')`,
		"wanted while available": `INSERT INTO collection_entries(
				user_id, item_id, owned, available, available_since_at,
				wanted, wanted_since_at, updated_at
			) VALUES (
				'history-member', 'history-item', 1, 1, '2026-07-30',
				1, '2026-07-30', '2026-07-30'
			)`,
		"mutable item number": "UPDATE catalog_items SET number = 2 WHERE id = 'history-item'",
		"delete item history": "DELETE FROM catalog_items WHERE id = 'history-item'",
	} {
		if _, err := store.database.ExecContext(ctx, statement); err == nil {
			t.Errorf("%s unexpectedly succeeded", name)
		}
	}
	if _, err := store.database.ExecContext(
		ctx,
		`INSERT INTO collection_entries(
			user_id, item_id, owned, available, wanted, wanted_since_at, updated_at
		) VALUES (
			'history-member', 'history-item', 1, 0, 1, '2026-07-30', '2026-07-30'
		)`,
	); err != nil {
		t.Fatalf("owned-and-wanted entry: %v", err)
	}
	if _, err := store.database.ExecContext(
		ctx,
		`UPDATE collection_entries SET available = 1
		 WHERE user_id = 'history-member' AND item_id = 'history-item'`,
	); err == nil {
		t.Fatal("wanted entry became available")
	}
	if _, err := store.database.ExecContext(
		ctx,
		"UPDATE catalog_items SET status = 'retired', retirement_effective_date = '2026-08-01', retirement_reason = 'test' WHERE id = 'history-item'",
	); err != nil {
		t.Fatalf("retire item: %v", err)
	}
	if _, err := store.database.ExecContext(
		ctx,
		"UPDATE catalog_items SET status = 'active', retirement_effective_date = '', retirement_reason = '' WHERE id = 'history-item'",
	); err == nil {
		t.Fatal("retired item was reactivated")
	}
}
