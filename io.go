package st8

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// persist writes the state atomically to disk via a temp file and rename.
func (db *DB[T]) persist(data T) error {
	if db.path == "" {
		return ErrInvalidPath
	}

	dir := filepath.Dir(db.path)

	tmp, err := os.CreateTemp(dir, "st8-file*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := db.serializer.Serialize(tmp, data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("serialize state: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// todo: handle rename for other os
	if err := os.Rename(tmpPath, db.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	cleanupTmp = false

	// fsync for dir is not supported on Windows.
	if runtime.GOOS != "windows" {
		syncDir(dir)
	}

	return nil
}

// syncDir performs a best-effort directory fsync.
// Any failure is intentionally ignored because commit success is determined by rename.
func syncDir(dir string) {
	// #nosec G304 -- dir is derived from db.path and only used for fsync
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() {
		_ = d.Close()
	}()

	_ = d.Sync()
}
