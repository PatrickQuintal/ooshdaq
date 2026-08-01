package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ooshdaq/ooshdaq/internal/repository/sqliterepo"
)

type RestoreConfig struct {
	Backup         string
	Database       string
	RetainReplaced string
}

// RestoreOffline verifies all inputs, checkpoints the stopped live database,
// preserves it as a verified backup, and atomically promotes an isolated
// restore. The serving process must be stopped by the operator.
func RestoreOffline(ctx context.Context, config RestoreConfig) error {
	backup, database, retained, err := normalizeRestorePaths(config)
	if err != nil {
		return err
	}
	if _, err := os.Stat(database); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("maintenance restore: live database does not exist")
		}
		return fmt.Errorf("maintenance restore: inspect live database: %w", err)
	}
	if _, err := os.Stat(retained); err == nil {
		return errors.New("maintenance restore: retained destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("maintenance restore: inspect retained destination: %w", err)
	}
	if err := sqliterepo.VerifyDatabase(ctx, backup, true); err != nil {
		return fmt.Errorf("maintenance restore: verify selected backup: %w", err)
	}
	if err := sqliterepo.CheckpointDatabase(ctx, database); err != nil {
		return fmt.Errorf("maintenance restore: checkpoint live database: %w", err)
	}
	if _, err := sqliterepo.RestoreVerified(ctx, database, retained); err != nil {
		return fmt.Errorf("maintenance restore: retain live database: %w", err)
	}

	staged := database + ".restore-staged"
	if _, err := os.Stat(staged); err == nil {
		return errors.New("maintenance restore: staged destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("maintenance restore: inspect staged destination: %w", err)
	}
	if _, err := sqliterepo.RestoreVerified(ctx, backup, staged); err != nil {
		return fmt.Errorf("maintenance restore: create isolated restore: %w", err)
	}
	defer os.Remove(staged)
	for _, sidecar := range []string{database + "-wal", database + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("maintenance restore: remove checkpoint sidecar: %w", err)
		}
	}
	if err := os.Rename(staged, database); err != nil {
		return fmt.Errorf("maintenance restore: atomically promote restore: %w", err)
	}
	if err := syncDirectory(filepath.Dir(database)); err != nil {
		return fmt.Errorf("maintenance restore: sync database directory: %w", err)
	}
	if err := sqliterepo.VerifyDatabase(ctx, database, true); err != nil {
		return fmt.Errorf("maintenance restore: verify promoted database: %w", err)
	}
	return nil
}

func normalizeRestorePaths(config RestoreConfig) (string, string, string, error) {
	if config.Backup == "" || config.Database == "" || config.RetainReplaced == "" {
		return "", "", "", errors.New(
			"maintenance restore: backup, database, and retained path are required",
		)
	}
	paths := make([]string, 3)
	for index, candidate := range []string{
		config.Backup,
		config.Database,
		config.RetainReplaced,
	} {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return "", "", "", fmt.Errorf("maintenance restore: resolve path: %w", err)
		}
		paths[index] = filepath.Clean(absolute)
	}
	if paths[0] == paths[1] || paths[0] == paths[2] || paths[1] == paths[2] {
		return "", "", "", errors.New("maintenance restore: all paths must differ")
	}
	return paths[0], paths[1], paths[2], nil
}
