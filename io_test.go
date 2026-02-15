package st8

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPersist(t *testing.T) {
	t.Run("returns_error_when_path_is_empty", func(t *testing.T) {
		db := &DB[fixtureState]{
			serializer: JSONSerializer[fixtureState]{Indent: "  "},
		}

		err := db.persist(newFixtureState())
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath, got: %v", err)
		}
	})

	t.Run("returns_error_when_target_is_unwritable", func(t *testing.T) {
		db := &DB[fixtureState]{
			path:       "/invalid/path/state.json",
			serializer: JSONSerializer[fixtureState]{Indent: "  "},
		}

		if err := db.persist(newFixtureState()); err == nil {
			t.Fatalf("expected persist error for invalid path")
		}
	})

	t.Run("returns_error_when_parent_dir_does_not_exist", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "state.json")
		db := &DB[fixtureState]{
			path:       path,
			serializer: JSONSerializer[fixtureState]{Indent: "  "},
		}

		err := db.persist(newFixtureState())
		if err == nil {
			t.Fatalf("expected error when parent dir does not exist")
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("expected wrapped *os.PathError, got: %v", err)
		}
	})

	t.Run("does_not_overwrite_existing_file_when_serialize_fails", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")

		good := &DB[fixtureState]{
			path:       path,
			serializer: JSONSerializer[fixtureState]{Indent: "  "},
		}
		if err := good.persist(fixtureState{
			Revision: 1,
			Buckets:  map[string]map[string]string{"original": {"k": "v"}},
		}); err != nil {
			t.Fatalf("seed state: %v", err)
		}

		before, err := os.ReadFile(path) // #nosec G304 -- test path comes from t.TempDir.
		if err != nil {
			t.Fatalf("read seeded file: %v", err)
		}

		serializeErr := errors.New("boom")
		fail := &DB[fixtureState]{
			path: path,
			serializer: failingSerializer[fixtureState]{
				err: serializeErr,
			},
		}

		err = fail.persist(fixtureState{
			Revision: 2,
			Buckets:  map[string]map[string]string{"changed": {"x": "y"}},
		})
		if err == nil {
			t.Fatalf("expected serialize error")
		}
		if !errors.Is(err, serializeErr) {
			t.Fatalf("expected wrapped serializer error, got: %v", err)
		}

		after, err := os.ReadFile(path) // #nosec G304 -- test path comes from t.TempDir.
		if err != nil {
			t.Fatalf("read file after failed persist: %v", err)
		}
		if string(after) != string(before) {
			t.Fatalf("expected destination file to remain unchanged on serialize failure")
		}

		tmpFiles, err := filepath.Glob(filepath.Join(filepath.Dir(path), "st8-file*.tmp"))
		if err != nil {
			t.Fatalf("glob temp files: %v", err)
		}
		if len(tmpFiles) != 0 {
			t.Fatalf("expected no leftover temp files, got %d", len(tmpFiles))
		}
	})

	t.Run("cleans_up_temp_file_when_rename_fails", func(t *testing.T) {
		dir := t.TempDir()
		targetDir := filepath.Join(dir, "state.json")
		if err := os.Mkdir(targetDir, 0o750); err != nil {
			t.Fatalf("create target dir: %v", err)
		}

		db := &DB[fixtureState]{
			path:       targetDir,
			serializer: JSONSerializer[fixtureState]{Indent: "  "},
		}
		err := db.persist(newFixtureState())
		if err == nil {
			t.Fatalf("expected rename error")
		}
		var linkErr *os.LinkError
		if !errors.As(err, &linkErr) {
			t.Fatalf("expected wrapped *os.LinkError, got: %v", err)
		}

		tmpFiles, err := filepath.Glob(filepath.Join(dir, "st8-file*.tmp"))
		if err != nil {
			t.Fatalf("glob temp files: %v", err)
		}
		if len(tmpFiles) != 0 {
			t.Fatalf("expected no leftover temp files, got %d", len(tmpFiles))
		}
	})

	t.Run("writes_expected_state_on_success", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		db := &DB[fixtureState]{
			path:       path,
			serializer: JSONSerializer[fixtureState]{Indent: "  "},
		}

		want := fixtureState{
			Revision: 3,
			Buckets: map[string]map[string]string{
				"jobs": {"id-1": "done"},
			},
		}
		if err := db.persist(want); err != nil {
			t.Fatalf("persist state: %v", err)
		}

		reopened, err := Open(path, newFixtureState())
		if err != nil {
			t.Fatalf("reopen state: %v", err)
		}
		if err := reopened.View(func(got fixtureState) error {
			if got.Revision != want.Revision {
				t.Fatalf("expected revision %d, got %d", want.Revision, got.Revision)
			}
			if got.Buckets["jobs"]["id-1"] != "done" {
				t.Fatalf("unexpected stored value: %q", got.Buckets["jobs"]["id-1"])
			}
			return nil
		}); err != nil {
			t.Fatalf("view reopened state: %v", err)
		}
	})
}

type failingSerializer[T any] struct {
	err error
}

func (s failingSerializer[T]) Serialize(_ io.Writer, _ T) error {
	return s.err
}

func (s failingSerializer[T]) Deserialize(_ io.Reader, _ *T) error {
	return nil
}
