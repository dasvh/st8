package st8

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type store[T any] struct {
	path       string
	serializer Serializer[T]
}

func newStore[T any](path string, serializer Serializer[T]) *store[T] {
	return &store[T]{
		path:       path,
		serializer: serializer,
	}
}

func (s *store[T]) load(dst *T) error {
	if s.path == "" {
		return ErrInvalidPath
	}
	if s.serializer == nil {
		return ErrNilSerializer
	}

	// #nosec G304 -- loading caller-provided path is the core behavior of this library
	f, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("open state file %q: %w", s.path, err)
	}
	defer func() { _ = f.Close() }()

	if err := s.serializer.Deserialize(f, dst); err != nil {
		return fmt.Errorf("deserialize state file %q: %w", s.path, err)
	}

	return nil
}

func (s *store[T]) save(data T) error {
	if s.path == "" {
		return ErrInvalidPath
	}
	if s.serializer == nil {
		return ErrNilSerializer
	}

	dir := filepath.Dir(s.path)

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

	if err := s.serializer.Serialize(tmp, data); err != nil {
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

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	cleanupTmp = false

	// fsync for dir is not supported on Windows.
	if runtime.GOOS != "windows" {
		syncDir(dir)
	}

	return nil
}

func (db *DB[T]) persist(data T) error {
	return db.store.save(data)
}

// syncDir performs a best-effort directory fsync.
// Any failure is intentionally ignored because commit success is determined by rename.
func syncDir(dir string) {
	// #nosec G304 -- dir comes from the configured file store path and is only used for fsync
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() {
		_ = d.Close()
	}()

	_ = d.Sync()
}
