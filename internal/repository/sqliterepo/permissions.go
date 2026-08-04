package sqliterepo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	privateDirectoryMode = 0o700
	privateFileMode      = 0o600
)

// preparePrivateDirectory creates a private directory when needed. Existing
// operator-managed directories, including symlinks to directories, are never
// repermissioned.
func preparePrivateDirectory(path string, writable bool) error {
	cleaned := filepath.Clean(path)
	_, inspectErr := os.Stat(cleaned)
	created := errors.Is(inspectErr, os.ErrNotExist)
	if created && writable {
		if err := os.MkdirAll(cleaned, privateDirectoryMode); err != nil {
			return fmt.Errorf("create private SQLite directory: %w", err)
		}
	} else if inspectErr != nil {
		return fmt.Errorf("inspect SQLite directory: %w", inspectErr)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("inspect SQLite directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("SQLite parent path must be a directory")
	}
	if !created || runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm() != privateDirectoryMode {
		return errors.New("created SQLite directory lacks owner-only permissions")
	}
	return nil
}

// preparePrivateDatabase creates a zero-length private database placeholder or
// tightens an existing file before SQLite opens it. SQLite derives new WAL and
// shared-memory file modes from the main database mode.
func preparePrivateDatabase(path string, writable bool) error {
	if writable {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFileMode)
		if err == nil {
			if err := file.Close(); err != nil {
				return fmt.Errorf("close private SQLite database: %w", err)
			}
			return tightenPrivateFile(path, true)
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create private SQLite database: %w", err)
		}
	}
	return tightenPrivateFile(path, writable)
}

func tightenPrivateFile(path string, writable bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private SQLite file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("private SQLite path must be a regular file, not a symlink")
	}
	if runtime.GOOS == "windows" {
		return nil
	}

	mode := info.Mode().Perm() & privateFileMode
	if mode != info.Mode().Perm() {
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("remove shared SQLite file access: %w", err)
		}
		info, err = os.Lstat(path)
		if err != nil {
			return fmt.Errorf("verify private SQLite file: %w", err)
		}
		mode = info.Mode().Perm()
	}
	if mode&0o177 != 0 {
		return errors.New("SQLite file still permits execute, group, or world access")
	}
	required := os.FileMode(0o400)
	if writable {
		required = privateFileMode
	}
	if mode&required != required {
		return errors.New("private SQLite file lacks required owner permissions")
	}
	return nil
}

func tightenSQLiteArtifacts(path string, writable bool) error {
	for _, artifact := range []string{path, path + "-wal", path + "-shm"} {
		if err := tightenPrivateFile(artifact, writable); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
	}
	return nil
}
