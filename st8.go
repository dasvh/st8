package st8

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

var (
	ErrNilSerializer = errors.New("nil serializer")
	ErrNilOption     = errors.New("nil option")
	ErrInvalidPath   = errors.New("invalid path")
)

// DB is a generic database structure that holds state and provides
// thread-safe access and persistence to a file.
type DB[T any] struct {
	mu         sync.RWMutex
	state      T
	path       string
	serializer Serializer[T]
}

// Option is a functional option type for configuring the DB.
type Option[T any] func(*config[T]) error

// config holds the configuration for the DB.
type config[T any] struct {
	Serializer Serializer[T]
}

// WithSerializer creates an Option to set a custom Serializer for the DB.
// Returns either a valid Option or an error if the provided Serializer is nil.
func WithSerializer[T any](serializer Serializer[T]) Option[T] {
	return func(cfg *config[T]) error {
		if serializer == nil {
			return ErrNilSerializer
		}
		cfg.Serializer = serializer
		return nil
	}
}

// Open initializes a new DB instance with the given path, initial state, and the provided options.
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
		path:       path,
		serializer: cfg.Serializer,
	}

	// #nosec G304 -- loading caller-provided path is the core behavior of this library
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := db.persist(initial); err != nil {
				return nil, fmt.Errorf("persist initial state %q: %w", path, err)
			}
			return db, nil
		}
		return nil, fmt.Errorf("open state file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := db.serializer.Deserialize(f, &db.state); err != nil {
		return nil, fmt.Errorf("deserialize state file %q: %w", path, err)
	}

	return db, nil
}
