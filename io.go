package st8

import (
	"fmt"
	"os"
	"path/filepath"
)

// persist write the state atomically to disk via a temp file and rename.
func (db *DB[T]) persist(data T) error {
	if db.path == "" {
		return ErrInvalidPath
	}

	dir := filepath.Dir(db.path)

	tmp, err := os.CreateTemp(dir, "st8-file*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
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
	if err := os.Rename(tmp.Name(), db.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}

	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync state dir: %w", err)
	}

	return nil
}

func syncDir(dir string) error {
	// #nosec G304 -- dir is derived from db.path and only used for fsync
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open state dir: %w", err)
	}

	if err := d.Sync(); err != nil {
		_ = d.Close()
		return fmt.Errorf("fsync state dir: %w", err)
	}

	_ = d.Close()

	return nil
}
