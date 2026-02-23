// Package st8 gives you a transactional in-memory store with file-backed persistence.
//
// It is built for app state where you want simple transactions without pulling in a DB.
//
// Guarantees:
// - Update is atomic for in-process state.
// - If Update returns an error, in-memory state is unchanged.
// - Commit happens when st8 atomically replaces the state file.
//
// Durability:
// - st8 syncs directory metadata on supported platforms as best effort.
// - That best-effort sync does not change commit success or failure.
//
// Scope:
// - st8 coordinates goroutines in one process.
// - st8 does not coordinate multiple processes writing the same file.
package st8

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
)

var (
	ErrNilSerializer = errors.New("nil serializer")
	ErrNilOption     = errors.New("nil option")
	ErrInvalidPath   = errors.New("invalid path")
	ErrNilCloner     = errors.New("nil cloner")
	ErrNilValidator  = errors.New("nil validator")
)

// DB is a transactional in-memory store instance.
type DB[T any] struct {
	mu         sync.RWMutex
	buf        bytes.Buffer
	state      T
	serializer Serializer[T]
	cloner     Cloner[T]
	store      *store[T]
	validator  func(T) error
}

// Option is a functional option type for configuring the DB.
type Option[T any] func(*config[T]) error

// config holds the configuration for the DB.
type config[T any] struct {
	Serializer Serializer[T]
	Cloner     Cloner[T]
	Validator  func(T) error
}

// Cloner lets you provide state cloning for Update.
// Clone must return an independent copy of src: mutating the result must not affect src.
type Cloner[T any] interface {
	Clone(src T) (T, error)
}

// WithSerializer sets the Serializer used for disk persistence.
// The returned Option will fail with ErrNilSerializer if serializer is nil.
func WithSerializer[T any](serializer Serializer[T]) Option[T] {
	return func(cfg *config[T]) error {
		if serializer == nil {
			return ErrNilSerializer
		}
		cfg.Serializer = serializer
		return nil
	}
}

// WithCloner sets the Cloner used by Update. If unset, Update clones via Serializer round-trip.
// The returned Option fails with ErrNilCloner if cloner is nil.
func WithCloner[T any](cloner Cloner[T]) Option[T] {
	return func(cfg *config[T]) error {
		if cloner == nil {
			return ErrNilCloner
		}
		cfg.Cloner = cloner
		return nil
	}
}

// WithValidator sets a state validator used by [Open] and [DB.Update].
// Validation failures abort open/commit and leave in-memory state unchanged.
// The returned [Option] fails with [ErrNilValidator] if fn is nil.
func WithValidator[T any](fn func(T) error) Option[T] {
	return func(cfg *config[T]) error {
		if fn == nil {
			return ErrNilValidator
		}
		cfg.Validator = fn
		return nil
	}
}

// Open initializes a new [DB] instance with the given path, initial state, and the provided options.
func Open[T any](path string, initial T, opts ...Option[T]) (*DB[T], error) {
	if path == "" {
		return nil, ErrInvalidPath
	}

	// default configuration with JSONSerializer
	cfg := config[T]{
		Serializer: JSONSerializer[T]{
			Indent:                "  ",
			DisallowUnknownFields: true,
			MaxBytes:              DefaultMaxBytes,
		},
	}

	for i, opt := range opts {
		if opt == nil {
			return nil, ErrNilOption
		}
		if err := opt(&cfg); err != nil {
			return nil, fmt.Errorf("apply option[%d]: %w", i, err)
		}
	}
	if cfg.Serializer == nil {
		return nil, ErrNilSerializer
	}

	db := &DB[T]{
		state:      initial,
		serializer: cfg.Serializer,
		cloner:     cfg.Cloner,
		store:      newStore(path, cfg.Serializer),
		validator:  cfg.Validator,
	}

	var loaded T
	if err := db.store.load(&loaded); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := db.validate(initial); err != nil {
				return nil, fmt.Errorf("validate initial state: %w", err)
			}
			if err := db.persist(initial); err != nil {
				return nil, fmt.Errorf("persist initial state: %w", err)
			}
			return db, nil
		}
		return nil, fmt.Errorf("load state %q: %w", path, err)
	}

	if err := db.validate(loaded); err != nil {
		return nil, fmt.Errorf("validate loaded state: %w", err)
	}

	db.state = loaded
	return db, nil
}

func (db *DB[T]) validate(s T) error {
	if db.validator == nil {
		return nil
	}
	return db.validator(s)
}
