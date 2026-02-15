package st8

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Run("returns_error_for_invalid_storage_path", func(t *testing.T) {
		_, err := Open[fixtureState]("", newFixtureState())
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath, got: %v", err)
		}
	})

	t.Run("returns_error_when_option_is_nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		var opt Option[fixtureState]
		_, err := Open(path, newFixtureState(), opt)
		if !errors.Is(err, ErrNilOption) {
			t.Fatalf("expected ErrNilOption, got: %v", err)
		}
	})

	t.Run("returns_error_when_serializer_option_is_nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		_, err := Open(path, newFixtureState(), WithSerializer[fixtureState](nil))
		if !errors.Is(err, ErrNilSerializer) {
			t.Fatalf("expected ErrNilSerializer, got: %v", err)
		}
	})

	t.Run("persists_committed_state_across_reopen", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "persist.json")

		first, err := Open(path, newFixtureState())
		if err != nil {
			t.Fatalf("open first db: %v", err)
		}

		if err := first.Update(func(s *fixtureState) error {
			s.Revision = 7
			s.Buckets["settings"] = map[string]string{"theme": "contrast"}
			return nil
		}); err != nil {
			t.Fatalf("update first db: %v", err)
		}

		second, err := Open(path, newFixtureState())
		if err != nil {
			t.Fatalf("open second db: %v", err)
		}

		if err := second.View(func(s fixtureState) error {
			if s.Revision != 7 {
				t.Fatalf("expected revision 7, got %d", s.Revision)
			}
			if got := s.Buckets["settings"]["theme"]; got != "contrast" {
				t.Fatalf("unexpected persisted value: %q", got)
			}
			return nil
		}); err != nil {
			t.Fatalf("view second db: %v", err)
		}
	})

	t.Run("fails_when_persisted_state_is_corrupt", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "corrupt.json")
		if err := os.WriteFile(path, []byte("{not-valid-json"), 0o600); err != nil {
			t.Fatalf("write corrupt file: %v", err)
		}

		_, err := Open[map[string]string](path, map[string]string{})
		if err == nil {
			t.Fatalf("expected decode error for corrupt state")
		}

		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Fatalf("expected wrapped *json.SyntaxError, got: %v", err)
		}
	})

	t.Run("uses_custom_serializer_from_options", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		wantErr := errors.New("serialize failed")

		_, err := Open(path, newFixtureState(), WithSerializer[fixtureState](failingOpenSerializer{
			err: wantErr,
		}))
		if err == nil {
			t.Fatalf("expected custom serializer error")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected wrapped custom serializer error, got: %v", err)
		}
	})
}

type failingOpenSerializer struct {
	err error
}

func (s failingOpenSerializer) Serialize(_ io.Writer, _ fixtureState) error {
	return s.err
}

func (s failingOpenSerializer) Deserialize(_ io.Reader, _ *fixtureState) error {
	return nil
}
