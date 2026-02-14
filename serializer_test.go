package st8

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"
)

type serializerFixture struct {
	Revision int               `json:"revision"`
	Meta     map[string]string `json:"meta"`
}

func TestJSONSerializer_Serialize(t *testing.T) {
	t.Run("encodes_valid_json", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{Indent: "  "}
		in := serializerFixture{
			Revision: 2,
			Meta:     map[string]string{"env": "test"},
		}

		var buf bytes.Buffer
		if err := ser.Serialize(&buf, in); err != nil {
			t.Fatalf("serialize: %v", err)
		}

		var out serializerFixture
		if err := ser.Deserialize(&buf, &out); err != nil {
			t.Fatalf("deserialize roundtrip: %v", err)
		}

		if out.Revision != in.Revision {
			t.Fatalf("expected revision %d, got %d", in.Revision, out.Revision)
		}
		if out.Meta["env"] != in.Meta["env"] {
			t.Fatalf("unexpected meta.env value: %q", out.Meta["env"])
		}
	})

	t.Run("returns_error_for_nil_writer", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}

		err := ser.Serialize(nil, serializerFixture{})
		if !errors.Is(err, ErrNilWriter) {
			t.Fatalf("expected ErrNilWriter, got: %v", err)
		}
	})
}

func TestJSONSerializer_Deserialize(t *testing.T) {
	t.Run("decodes_single_json_value", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":3,"meta":{"mode":"safe"}}`), &out)
		if err != nil {
			t.Fatalf("deserialize: %v", err)
		}
		if out.Revision != 3 {
			t.Fatalf("expected revision 3, got %d", out.Revision)
		}
		if out.Meta["mode"] != "safe" {
			t.Fatalf("unexpected meta.mode value: %q", out.Meta["mode"])
		}
	})

	t.Run("returns_error_for_nil_reader", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out serializerFixture

		err := ser.Deserialize(nil, &out)
		if !errors.Is(err, ErrNilReader) {
			t.Fatalf("expected ErrNilReader, got: %v", err)
		}
	})

	t.Run("returns_error_for_nil_decode_target", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out *serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{}}`), out)
		if !errors.Is(err, ErrNilDecodeTarget) {
			t.Fatalf("expected ErrNilDecodeTarget, got: %v", err)
		}
	})

	t.Run("returns_error_for_malformed_json", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":3`), &out)
		if err == nil {
			t.Fatalf("expected malformed JSON error")
		}
	})

	t.Run("returns_error_for_trailing_json_value", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out serializerFixture

		payload := `{"revision":1,"meta":{}} {"revision":2,"meta":{}}`
		err := ser.Deserialize(strings.NewReader(payload), &out)
		if !errors.Is(err, ErrTrailingJSONData) {
			t.Fatalf("expected ErrTrailingJSONData, got: %v", err)
		}
	})

	t.Run("returns_error_for_trailing_garbage", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{}} trailing`), &out)
		if err == nil {
			t.Fatalf("expected trailing data error")
		}
	})

	t.Run("allows_unknown_fields_by_default", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{},"extra":"ok"}`), &out)
		if err != nil {
			t.Fatalf("expected unknown fields to be allowed by default, got: %v", err)
		}
	})

	t.Run("rejects_unknown_fields_when_configured", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{DisallowUnknownFields: true}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{},"extra":"nope"}`), &out)
		if err == nil {
			t.Fatalf("expected unknown field error")
		}
		if !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("expected unknown field error, got: %v", err)
		}
	})

	t.Run("returns_error_when_payload_exceeds_max_bytes", func(t *testing.T) {
		payload := `{"revision":123,"meta":{"long":"value"}}`
		ser := JSONSerializer[serializerFixture]{MaxBytes: int64(len(payload) - 1)}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(payload), &out)
		if !errors.Is(err, ErrJSONPayloadTooBig) {
			t.Fatalf("expected ErrJSONPayloadTooBig, got: %v", err)
		}
	})

	t.Run("allows_payload_when_within_max_bytes", func(t *testing.T) {
		payload := `{"revision":123,"meta":{"long":"value"}}`
		ser := JSONSerializer[serializerFixture]{MaxBytes: int64(len(payload))}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(payload), &out)
		if err != nil {
			t.Fatalf("expected payload to fit max bytes, got: %v", err)
		}
	})

	t.Run("returns_error_when_max_bytes_is_negative", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{MaxBytes: -1}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{}}`), &out)
		if !errors.Is(err, ErrInvalidMaxBytes) {
			t.Fatalf("expected ErrInvalidMaxBytes, got: %v", err)
		}
	})

	t.Run("returns_error_when_max_bytes_exceeds_overflow_guard", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{MaxBytes: math.MaxInt64}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{}}`), &out)
		if !errors.Is(err, ErrInvalidMaxBytes) {
			t.Fatalf("expected ErrInvalidMaxBytes, got: %v", err)
		}
	})

	t.Run("allows_max_bytes_int64_minus_one", func(t *testing.T) {
		ser := JSONSerializer[serializerFixture]{MaxBytes: math.MaxInt64 - 1}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(`{"revision":1,"meta":{}}`), &out)
		if err != nil {
			t.Fatalf("expected max-int64-1 to be valid, got: %v", err)
		}
	})

	t.Run("returns_payload_too_big_when_trailing_data_exceeds_limit", func(t *testing.T) {
		first := `{"revision":1,"meta":{}}`
		payload := first + `{"revision":2,"meta":{}}`
		ser := JSONSerializer[serializerFixture]{MaxBytes: int64(len(first))}
		var out serializerFixture

		err := ser.Deserialize(strings.NewReader(payload), &out)
		if !errors.Is(err, ErrJSONPayloadTooBig) {
			t.Fatalf("expected ErrJSONPayloadTooBig, got: %v", err)
		}
	})
}
