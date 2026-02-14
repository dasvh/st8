package st8

import (
	"encoding/json"
	"errors"
	"io"
	"math"
)

var (
	ErrNilWriter         = errors.New("nil writer")
	ErrNilReader         = errors.New("nil reader")
	ErrNilDecodeTarget   = errors.New("nil decode target")
	ErrInvalidMaxBytes   = errors.New("MaxBytes must be >= 0 and < math.MaxInt64")
	ErrJSONPayloadTooBig = errors.New("JSON payload exceeds configured max size")
	ErrTrailingJSONData  = errors.New("trailing data after JSON value")
)

const (
	DefaultMaxBytes int64 = 32 << 20 // 32 MiB
)

// Serializer defines the interface for serializing and deserializing state data.
type Serializer[T any] interface {
	Serialize(w io.Writer, data T) error
	Deserialize(r io.Reader, data *T) error
}

// JSONSerializer is the default Serializer implementation.
type JSONSerializer[T any] struct {
	Indent                string
	DisallowUnknownFields bool
	MaxBytes              int64 // 0 means unbounded; valid bounded range is [1, math.MaxInt64-1]
}

// Serialize encodes the state to JSON and writes it to the provided writer.
func (j JSONSerializer[T]) Serialize(w io.Writer, data T) error {
	if w == nil {
		return ErrNilWriter
	}

	enc := json.NewEncoder(w)
	if j.Indent != "" {
		enc.SetIndent("", j.Indent)
	}
	return enc.Encode(data)
}

// Deserialize reads JSON from the provided reader and decodes it into the state.
func (j JSONSerializer[T]) Deserialize(r io.Reader, data *T) error {
	if r == nil {
		return ErrNilReader
	}
	if data == nil {
		return ErrNilDecodeTarget
	}

	guardLimit, err := maxBytesWithGuard(j.MaxBytes)
	if err != nil {
		return err
	}

	var lr *io.LimitedReader
	if guardLimit > 0 {
		// use a one-byte guard to accept payloads exactly at MaxBytes
		// while still detecting overflow deterministically.
		lr = &io.LimitedReader{R: r, N: guardLimit}
		r = lr
	}

	dec := json.NewDecoder(r)
	if j.DisallowUnknownFields {
		dec.DisallowUnknownFields()
	}

	if err := dec.Decode(data); err != nil {
		return checkPayloadSize(lr, err)
	}

	// ensure there is exactly one JSON value and nothing else (except whitespace).
	_, err = dec.Token()
	if err == nil {
		return checkPayloadSize(lr, ErrTrailingJSONData)
	}

	if errors.Is(err, io.EOF) {
		if limitReached(lr) {
			return ErrJSONPayloadTooBig
		}
		return nil
	}

	return checkPayloadSize(lr, err)
}

func maxBytesWithGuard(maxBytes int64) (int64, error) {
	if maxBytes < 0 || maxBytes == math.MaxInt64 {
		return 0, ErrInvalidMaxBytes
	}
	if maxBytes == 0 {
		return 0, nil
	}
	return maxBytes + 1, nil
}

func limitReached(lr *io.LimitedReader) bool {
	return lr != nil && lr.N <= 0
}

func checkPayloadSize(lr *io.LimitedReader, fallback error) error {
	if limitReached(lr) {
		return ErrJSONPayloadTooBig
	}
	return fallback
}
