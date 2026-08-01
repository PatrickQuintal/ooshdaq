package sqliterepo

import (
	"context"
	"errors"
	"time"
)

// CheckpointResult records SQLite's measured WAL state for one checkpoint.
type CheckpointResult struct {
	Busy               int
	LogFrames          int
	CheckpointedFrames int
	Duration           time.Duration
}

// Checkpoint folds committed WAL frames into the database. Truncate is used
// after HTTP and maintenance work have drained; passive mode is suitable for
// observing behavior while the service remains online.
func (store *Store) Checkpoint(
	ctx context.Context,
	truncate bool,
) (CheckpointResult, error) {
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	pragma := "PRAGMA wal_checkpoint(PASSIVE)"
	if truncate {
		pragma = "PRAGMA wal_checkpoint(TRUNCATE)"
	}
	started := store.now()
	var result CheckpointResult
	err := store.database.QueryRowContext(ctx, pragma).Scan(
		&result.Busy,
		&result.LogFrames,
		&result.CheckpointedFrames,
	)
	result.Duration = store.now().Sub(started)
	if err != nil {
		return CheckpointResult{}, classify("checkpoint SQLite", err)
	}
	if result.Busy != 0 {
		return result, errors.New("SQLite checkpoint could not drain every active reader")
	}
	return result, nil
}
