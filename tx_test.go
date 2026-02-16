package st8

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
)

type deepCloneState struct {
	Revision int                          `json:"revision"`
	Buckets  map[string]map[string]string `json:"buckets"`
}

const fixtureStatusDone = "done"

func newDeepCloneState() deepCloneState {
	return deepCloneState{
		Revision: 0,
		Buckets:  map[string]map[string]string{},
	}
}

func TestUpdate(t *testing.T) {
	t.Run("rolls_back_all_changes_when_update_fails", func(t *testing.T) {
		db, err := Open(filepath.Join(t.TempDir(), "rollback.json"), newDeepCloneState())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}

		updateErr := db.Update(func(s *deepCloneState) error {
			s.Revision++
			s.Buckets["team-a"] = map[string]string{"item-1": "alpha"}
			s.Buckets["team-b"] = map[string]string{"item-2": "beta"}
			return errors.New("force rollback")
		})
		if updateErr == nil {
			t.Fatalf("expected update to fail")
		}

		if err := db.View(func(s deepCloneState) error {
			if s.Revision != 0 {
				t.Fatalf("expected revision 0, got %d", s.Revision)
			}
			if len(s.Buckets) != 0 {
				t.Fatalf("expected no buckets, got %d", len(s.Buckets))
			}
			return nil
		}); err != nil {
			t.Fatalf("view state: %v", err)
		}
	})

	t.Run("commits_all_changes_atomically_on_success", func(t *testing.T) {
		db, err := Open(filepath.Join(t.TempDir(), "commit.json"), newDeepCloneState())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}

		if err := db.Update(func(s *deepCloneState) error {
			s.Revision++
			s.Buckets["team-a"] = map[string]string{
				"item-1": "alpha",
				"item-2": "bravo",
			}
			s.Buckets["team-b"] = map[string]string{
				"item-3": "charlie",
			}
			return nil
		}); err != nil {
			t.Fatalf("update state: %v", err)
		}

		if err := db.View(func(s deepCloneState) error {
			if s.Revision != 1 {
				t.Fatalf("expected revision 1, got %d", s.Revision)
			}
			if len(s.Buckets) != 2 {
				t.Fatalf("expected 2 buckets, got %d", len(s.Buckets))
			}
			if got := s.Buckets["team-a"]["item-2"]; got != "bravo" {
				t.Fatalf("unexpected value for team-a/item-2: %q", got)
			}
			if got := s.Buckets["team-b"]["item-3"]; got != "charlie" {
				t.Fatalf("unexpected value for team-b/item-3: %q", got)
			}
			return nil
		}); err != nil {
			t.Fatalf("view state: %v", err)
		}
	})

	t.Run("keeps_nested_state_isolated_on_rollback_for_all_clone_paths", func(t *testing.T) {
		cases := []struct {
			name string
			opts []Option[deepCloneState]
		}{
			{
				name: "serializer_round_trip",
			},
			{
				name: "custom_cloner",
				opts: []Option[deepCloneState]{
					WithCloner(deepCloneStateCloner{}),
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				db, err := Open(filepath.Join(t.TempDir(), "rollback-isolation.json"),
					newDeepCloneState(), tc.opts...)
				if err != nil {
					t.Fatalf("open db: %v", err)
				}

				if err := db.Update(func(s *deepCloneState) error {
					s.Buckets["team-a"] = map[string]string{"item-1": "alpha"}
					return nil
				}); err != nil {
					t.Fatalf("seed committed state: %v", err)
				}

				updateErr := db.Update(func(s *deepCloneState) error {
					s.Buckets["team-a"]["item-1"] = "mutated"
					s.Buckets["team-a"]["item-2"] = "new"
					return errors.New("force rollback nested mutation")
				})
				if updateErr == nil {
					t.Fatalf("expected update to fail")
				}

				if err := db.View(func(s deepCloneState) error {
					if got := s.Buckets["team-a"]["item-1"]; got != "alpha" {
						t.Fatalf("expected original value after rollback, got %q", got)
					}
					if _, exists := s.Buckets["team-a"]["item-2"]; exists {
						t.Fatalf("unexpected rolled-back key team-a/item-2 to exist")
					}
					return nil
				}); err != nil {
					t.Fatalf("view state: %v", err)
				}
			})
		}
	})

	t.Run("returns_cloner_error_without_running_update_fn", func(t *testing.T) {
		wantErr := errors.New("clone failed")
		db, err := Open(
			filepath.Join(t.TempDir(), "clone-error.json"),
			newDeepCloneState(),
			WithCloner(failingDeepCloneStateCloner{err: wantErr}),
		)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}

		updateFnCalled := false
		err = db.Update(func(s *deepCloneState) error {
			updateFnCalled = true
			s.Revision = 99
			return nil
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected cloner error, got: %v", err)
		}
		if updateFnCalled {
			t.Fatalf("expected update fn to not run when clone fails")
		}

		if err := db.View(func(s deepCloneState) error {
			if s.Revision != 0 {
				t.Fatalf("expected unchanged revision 0, got %d", s.Revision)
			}
			if len(s.Buckets) != 0 {
				t.Fatalf("expected no buckets, got %d", len(s.Buckets))
			}
			return nil
		}); err != nil {
			t.Fatalf("view state: %v", err)
		}
	})

	t.Run("serializes_concurrent_updates_without_losing_data", func(t *testing.T) {
		db, err := Open(filepath.Join(t.TempDir(), "concurrency.json"), newDeepCloneState())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}

		const workers = 25
		errCh := make(chan error, workers)
		var wg sync.WaitGroup

		for i := range workers {
			wg.Go(func() {
				id := fmt.Sprintf("item-%d", i)
				if err := db.Update(func(s *deepCloneState) error {
					s.Revision++
					if s.Buckets["jobs"] == nil {
						s.Buckets["jobs"] = map[string]string{}
					}
					s.Buckets["jobs"][id] = fixtureStatusDone
					return nil
				}); err != nil {
					errCh <- err
				}
			})
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Fatalf("unexpected concurrent update error: %v", err)
		}

		if err := db.View(func(s deepCloneState) error {
			if s.Revision != workers {
				t.Fatalf("expected revision %d, got %d", workers, s.Revision)
			}
			if got := len(s.Buckets["jobs"]); got != workers {
				t.Fatalf("expected %d jobs, got %d", workers, got)
			}
			return nil
		}); err != nil {
			t.Fatalf("view state: %v", err)
		}
	})

	t.Run("blocks_view_until_update_commits", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "visibility.json"), newDeepCloneState())
			if err != nil {
				t.Fatalf("open db: %v", err)
			}

			started := make(chan struct{})
			release := make(chan struct{})
			updateErrCh := make(chan error, 1)

			go func() {
				updateErrCh <- db.Update(func(s *deepCloneState) error {
					s.Revision = 1
					close(started)
					<-release
					return nil
				})
			}()

			<-started

			viewStarted := make(chan struct{})
			viewDone := make(chan int, 1)
			viewErrCh := make(chan error, 1)
			go func() {
				close(viewStarted)
				err := db.View(func(s deepCloneState) error {
					viewDone <- s.Revision
					return nil
				})
				viewErrCh <- err
			}()

			<-viewStarted
			select {
			case rev := <-viewDone:
				t.Fatalf("view returned before update commit with revision %d", rev)
			default:
				// Expected: view is blocked while update holds write lock.
			}

			close(release)
			synctest.Wait()

			if err := <-updateErrCh; err != nil {
				t.Fatalf("update failed: %v", err)
			}
			if err := <-viewErrCh; err != nil {
				t.Fatalf("view failed: %v", err)
			}

			rev := <-viewDone
			if rev != 1 {
				t.Fatalf("expected committed revision 1, got %d", rev)
			}
		})
	})
}

type deepCloneStateCloner struct{}

func (deepCloneStateCloner) Clone(src deepCloneState) (deepCloneState, error) {
	return cloneDeepCloneState(src), nil
}

type failingDeepCloneStateCloner struct {
	err error
}

func (c failingDeepCloneStateCloner) Clone(_ deepCloneState) (deepCloneState, error) {
	return deepCloneState{}, c.err
}

func cloneDeepCloneState(src deepCloneState) deepCloneState {
	clone := deepCloneState{
		Revision: src.Revision,
		Buckets:  make(map[string]map[string]string, len(src.Buckets)),
	}

	for bucketID, bucketValues := range src.Buckets {
		next := make(map[string]string, len(bucketValues))
		maps.Copy(next, bucketValues)
		clone.Buckets[bucketID] = next
	}

	return clone
}
