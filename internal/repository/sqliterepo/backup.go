package sqliterepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	sqlite "modernc.org/sqlite"
)

const backupPagesPerStep = 128

type backupConnection interface {
	NewBackup(string) (*sqlite.Backup, error)
}

// BackupResult is durable evidence about one atomically published backup.
type BackupResult struct {
	Path      string
	SizeBytes int64
	SHA256    string
	CreatedAt time.Time
}

// Backup creates and verifies a consistent online backup before atomically
// publishing it at destination. Destination must not already exist.
func (store *Store) Backup(ctx context.Context, destination string) (BackupResult, error) {
	destination, err := cleanDestination(store.path, destination)
	if err != nil {
		return BackupResult{}, err
	}
	if err := preparePrivateDirectory(filepath.Dir(destination), true); err != nil {
		return BackupResult{}, fmt.Errorf("create backup directory: %w", err)
	}
	if _, err := os.Stat(destination); err == nil {
		return BackupResult{}, errors.New("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return BackupResult{}, fmt.Errorf("inspect backup destination: %w", err)
	}

	temporary, cleanup, err := temporarySQLitePath(filepath.Dir(destination), ".ooshdaq-backup-")
	if err != nil {
		return BackupResult{}, err
	}
	defer cleanup()

	if err := backupDatabase(ctx, store.database, temporary); err != nil {
		return BackupResult{}, fmt.Errorf("create online backup: %w", classify("backup SQLite", err))
	}
	if err := tightenPrivateFile(temporary, true); err != nil {
		return BackupResult{}, fmt.Errorf("protect online backup: %w", err)
	}
	if err := VerifyDatabase(ctx, temporary, true); err != nil {
		return BackupResult{}, fmt.Errorf("verify online backup: %w", err)
	}
	if err := tightenSQLiteArtifacts(temporary, true); err != nil {
		return BackupResult{}, fmt.Errorf("protect verified online backup: %w", err)
	}
	result, err := inspectBackup(temporary)
	if err != nil {
		return BackupResult{}, err
	}
	if err := syncFile(temporary); err != nil {
		return BackupResult{}, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return BackupResult{}, fmt.Errorf("publish online backup: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return BackupResult{}, err
	}
	result.Path = destination
	result.CreatedAt = store.now().UTC()
	return result, nil
}

// RestoreVerified creates a new isolated SQLite database from a verified
// backup. It never replaces an existing destination.
func RestoreVerified(ctx context.Context, backupPath, destination string) (BackupResult, error) {
	backupPath = filepath.Clean(backupPath)
	destination, err := cleanDestination(backupPath, destination)
	if err != nil {
		return BackupResult{}, err
	}
	if err := VerifyDatabase(ctx, backupPath, true); err != nil {
		return BackupResult{}, fmt.Errorf("verify restore source: %w", err)
	}
	if _, err := os.Stat(destination); err == nil {
		return BackupResult{}, errors.New("restore destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return BackupResult{}, fmt.Errorf("inspect restore destination: %w", err)
	}
	if err := preparePrivateDirectory(filepath.Dir(destination), true); err != nil {
		return BackupResult{}, fmt.Errorf("create restore directory: %w", err)
	}

	sourceDSN, err := readOnlyDSN(backupPath)
	if err != nil {
		return BackupResult{}, err
	}
	source, err := sql.Open("sqlite", sourceDSN)
	if err != nil {
		return BackupResult{}, fmt.Errorf("open restore source: %w", err)
	}
	defer source.Close()
	source.SetMaxOpenConns(1)

	temporary, cleanup, err := temporarySQLitePath(filepath.Dir(destination), ".ooshdaq-restore-")
	if err != nil {
		return BackupResult{}, err
	}
	defer cleanup()
	if err := backupDatabase(ctx, source, temporary); err != nil {
		return BackupResult{}, fmt.Errorf(
			"restore through SQLite backup API: %w",
			classify("restore SQLite", err),
		)
	}
	if err := tightenPrivateFile(temporary, true); err != nil {
		return BackupResult{}, fmt.Errorf("protect isolated restore: %w", err)
	}
	if err := VerifyDatabase(ctx, temporary, true); err != nil {
		return BackupResult{}, fmt.Errorf("verify isolated restore: %w", err)
	}
	if err := tightenSQLiteArtifacts(temporary, true); err != nil {
		return BackupResult{}, fmt.Errorf("protect verified isolated restore: %w", err)
	}
	result, err := inspectBackup(temporary)
	if err != nil {
		return BackupResult{}, err
	}
	if err := syncFile(temporary); err != nil {
		return BackupResult{}, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return BackupResult{}, fmt.Errorf("publish isolated restore: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return BackupResult{}, err
	}
	result.Path = destination
	result.CreatedAt = time.Now().UTC()
	return result, nil
}

// VerifyDatabase opens a database read-only and verifies migration history,
// foreign keys, and either quick or full integrity.
func VerifyDatabase(ctx context.Context, path string, thorough bool) error {
	if err := validateExistingFile(path); err != nil {
		return err
	}
	dsn, err := readOnlyDSN(path)
	if err != nil {
		return err
	}
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open SQLite verification database: %w", err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)

	if err := verifyMigrationHistory(ctx, database); err != nil {
		return err
	}
	store := &Store{database: database}
	return store.Check(ctx, thorough)
}

// CheckpointDatabase folds a stopped database's WAL into its main file and
// truncates the WAL before an offline replacement. Callers must ensure no
// serving process has the database open.
func CheckpointDatabase(ctx context.Context, path string) error {
	if err := VerifyDatabase(ctx, path, true); err != nil {
		return err
	}
	store, err := Open(ctx, Config{Path: path, SkipMigrations: true})
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := store.Checkpoint(ctx, true); err != nil {
		return err
	}
	if err := store.Check(ctx, true); err != nil {
		return err
	}
	return nil
}

func backupDatabase(ctx context.Context, source *sql.DB, destination string) error {
	connection, err := source.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire source connection: %w", err)
	}
	defer connection.Close()

	return connection.Raw(func(driverConnection any) error {
		backuper, ok := driverConnection.(backupConnection)
		if !ok {
			return errors.New("SQLite driver does not expose the online backup API")
		}
		backup, err := backuper.NewBackup(destination)
		if err != nil {
			return err
		}
		finished := false
		defer func() {
			if !finished {
				_ = backup.Finish()
			}
		}()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			more, err := backup.Step(backupPagesPerStep)
			if err != nil {
				return err
			}
			if !more {
				if err := backup.Finish(); err != nil {
					return err
				}
				finished = true
				return nil
			}
		}
	})
}

func verifyMigrationHistory(ctx context.Context, database *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	rows, err := database.QueryContext(
		ctx,
		"SELECT version, name, checksum FROM schema_migrations ORDER BY version",
	)
	if err != nil {
		return fmt.Errorf("read verified migration history: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return fmt.Errorf("scan verified migration history: %w", err)
		}
		count++
		if version < 1 || version > len(migrations) {
			return fmt.Errorf("verified SQLite schema version %d is unsupported", version)
		}
		expected := migrations[version-1]
		if name != expected.name || checksum != expected.checksum {
			return fmt.Errorf("verified SQLite migration %d does not match the application", version)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate verified migration history: %w", err)
	}
	if count != len(migrations) {
		return fmt.Errorf("verified SQLite schema has %d migrations, want %d", count, len(migrations))
	}
	return nil
}

func temporarySQLitePath(directory, pattern string) (string, func(), error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temporary SQLite path: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", nil, fmt.Errorf("close temporary SQLite path: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(path)
		_ = os.Remove(path + "-wal")
		_ = os.Remove(path + "-shm")
	}
	return path, cleanup, nil
}

func inspectBackup(path string) (BackupResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return BackupResult{}, fmt.Errorf("open completed backup: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return BackupResult{}, fmt.Errorf("stat completed backup: %w", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return BackupResult{}, fmt.Errorf("hash completed backup: %w", err)
	}
	return BackupResult{
		SizeBytes: info.Size(),
		SHA256:    hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func cleanDestination(source, destination string) (string, error) {
	destination = filepath.Clean(destination)
	if destination == "." || destination == string(filepath.Separator) {
		return "", errors.New("SQLite destination must name a file")
	}
	sourceAbsolute, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite source: %w", err)
	}
	destinationAbsolute, err := filepath.Abs(destination)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite destination: %w", err)
	}
	if sourceAbsolute == destinationAbsolute {
		return "", errors.New("SQLite source and destination must differ")
	}
	return destinationAbsolute, nil
}

func readOnlyDSN(path string) (string, error) {
	return (Config{
		Path:           path,
		MaxOpenConns:   1,
		BusyTimeout:    5 * time.Second,
		ReadOnly:       true,
		SkipMigrations: true,
	}).dsn()
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open SQLite file for sync: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync SQLite file: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open SQLite directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync SQLite directory: %w", err)
	}
	return nil
}
