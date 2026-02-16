package st8

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	t.Run("returns_error_for_invalid_storage_path", func(t *testing.T) {
		_, err := Open("", newDeepCloneState())
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath, got: %v", err)
		}
	})

	t.Run("returns_error_when_option_is_nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		var opt Option[deepCloneState]
		_, err := Open(path, newDeepCloneState(), opt)
		if !errors.Is(err, ErrNilOption) {
			t.Fatalf("expected ErrNilOption, got: %v", err)
		}
	})

	t.Run("returns_error_when_serializer_option_is_nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		_, err := Open(path, newDeepCloneState(), WithSerializer[deepCloneState](nil))
		if !errors.Is(err, ErrNilSerializer) {
			t.Fatalf("expected ErrNilSerializer, got: %v", err)
		}
	})

	t.Run("returns_error_when_cloner_option_is_nil", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		_, err := Open(path, newDeepCloneState(), WithCloner[deepCloneState](nil))
		if !errors.Is(err, ErrNilCloner) {
			t.Fatalf("expected ErrNilCloner, got: %v", err)
		}
	})

	t.Run("persists_committed_state_across_reopen", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "persist.json")

		first, err := Open(path, newDeepCloneState())
		if err != nil {
			t.Fatalf("open first db: %v", err)
		}

		if err := first.Update(func(s *deepCloneState) error {
			s.Revision = 7
			s.Buckets["settings"] = map[string]string{"theme": "contrast"}
			return nil
		}); err != nil {
			t.Fatalf("update first db: %v", err)
		}

		second, err := Open(path, newDeepCloneState())
		if err != nil {
			t.Fatalf("open second db: %v", err)
		}

		if err := second.View(func(s deepCloneState) error {
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

		_, err := Open(path, map[string]string{})
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

		_, err := Open(path, newDeepCloneState(), WithSerializer(failingOpenSerializer{
			err: wantErr,
		}))
		if err == nil {
			t.Fatalf("expected custom serializer error")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected wrapped custom serializer error, got: %v", err)
		}
	})

	t.Run("uses_custom_cloner_from_options", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state.json")
		cloner := &testCloner{}

		db, err := Open(path, newClonerState(), WithCloner(cloner))
		if err != nil {
			t.Fatalf("open db: %v", err)
		}

		if err := db.Update(func(s *clonerState) error {
			s.Classes["class-1"] = class{
				Title: "Math 101",
			}
			s.StudentsByClass["class-1"] = map[string]student{
				"student-1": {
					Name: "Ava",
				},
				"student-2": {
					Name: "Noah",
				},
			}
			return nil
		}); err != nil {
			t.Fatalf("update state: %v", err)
		}

		if cloner.calls != 1 {
			t.Fatalf("expected cloner to be called once, got %d", cloner.calls)
		}
	})
}

type failingOpenSerializer struct {
	err error
}

func (s failingOpenSerializer) Serialize(_ io.Writer, _ deepCloneState) error {
	return s.err
}

func (s failingOpenSerializer) Deserialize(_ io.Reader, _ *deepCloneState) error {
	return nil
}

type testCloner struct {
	calls int
}

func (tc *testCloner) Clone(src clonerState) (clonerState, error) {
	tc.calls++
	clone := clonerState{
		Classes:         make(map[string]class, len(src.Classes)),
		StudentsByClass: make(map[string]map[string]student, len(src.StudentsByClass)),
	}
	maps.Copy(clone.Classes, src.Classes)
	for classID, students := range src.StudentsByClass {
		next := make(map[string]student, len(students))
		maps.Copy(next, students)
		clone.StudentsByClass[classID] = next
	}
	return clone, nil
}

type clonerState struct {
	Classes         map[string]class              `json:"classes"`
	StudentsByClass map[string]map[string]student `json:"students_by_class"`
}

type class struct {
	Title string `json:"title"`
}

type student struct {
	Name string `json:"name"`
}

func newClonerState() clonerState {
	return clonerState{
		Classes:         map[string]class{},
		StudentsByClass: map[string]map[string]student{},
	}
}
