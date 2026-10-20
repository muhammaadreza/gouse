package decoder_test

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

type customText struct{ Text string }

func (c *customText) UnmarshalText(text []byte) error {
	c.Text = "custom:" + string(text)
	return nil
}

type customInt int

func (c *customInt) UnmarshalText(text []byte) error {
	if string(text) != "custom" {
		return secretError(string(text))
	}
	*c = 99
	return nil
}

// A named slice's decoder overrides the built-in splitting/element decoder,
// even though its underlying element type is otherwise unsupported.
type customSlice []map[string]string

func (c *customSlice) UnmarshalText(text []byte) error {
	*c = customSlice{{"text": string(text)}}
	return nil
}

type valueReceiver string

func (valueReceiver) UnmarshalText(text []byte) error {
	if string(text) == "accepted" {
		return nil
	}
	return secretError(string(text))
}

type secretError string

type customMap map[string]string

func (*customMap) UnmarshalText([]byte) error { return nil }

func (s secretError) Error() string { return "decoder secret: " + string(s) }

type rejecting string

func (*rejecting) UnmarshalText(text []byte) error { return secretError(string(text)) }

type hostileError struct{}

func (hostileError) Error() string { panic("must never render raw custom error") }

type hostile string

func (*hostile) UnmarshalText([]byte) error { return hostileError{} }

var _ encoding.TextUnmarshaler = (*customText)(nil)

type Embedded struct {
	Required string `env:"REQUIRED" binding:"required"`
}

type recursive struct {
	Next *recursive
}

type recursiveSlice []recursiveSlice

func dynamicConfig(t reflect.Type, tags string) any {
	schema := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: t, Tag: reflect.StructTag(tags)}})
	return reflect.New(schema).Interface()
}

type namedString string
type namedBool bool
type namedInt int16
type namedUint uint32
type namedFloat float32
type distinctDuration time.Duration

// decodeType exercises the decoder boundary without committing into a destination.
func decodeType(typ reflect.Type, prefix string, lookup func(string) (string, bool)) (reflect.Value, []decoder.Diagnostic) {
	plan, diagnostics := decoder.Compile(typ, prefix)
	if len(diagnostics) > 0 {
		return reflect.Value{}, diagnostics
	}
	return plan.Decode(lookup)
}

func mustDecode(t *testing.T, typ reflect.Type, prefix string, lookup func(string) (string, bool)) reflect.Value {
	t.Helper()
	candidate, diagnostics := decodeType(typ, prefix, lookup)
	if len(diagnostics) > 0 || !candidate.IsValid() {
		t.Fatalf("Decode(%v) = invalid candidate or diagnostics %+v", typ, diagnostics)
	}
	return candidate
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, found := values[key]; return value, found }
}

func dynamicType(typ reflect.Type, tags string) reflect.Type {
	return reflect.StructOf([]reflect.StructField{{Name: "Value", Type: typ, Tag: reflect.StructTag(tags)}})
}

func hasCategory(diagnostics []decoder.Diagnostic, category decoder.Category) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Category == category {
			return true
		}
	}
	return false
}

func assertDiagnosticPrivacy(t *testing.T, diagnostics []decoder.Diagnostic, secrets ...string) {
	t.Helper()
	if len(diagnostics) == 0 {
		t.Fatal("expected diagnostics for privacy inspection")
	}
	for _, secret := range secrets {
		if strings.Contains(fmt.Sprintf("%+v %#v", diagnostics, diagnostics), secret) {
			t.Error("diagnostics retain secret text")
		}
	}
}

type engineHostileError string

func (engineHostileError) Error() string { panic("raw decoder error was rendered") }

type engineRejecting string

func (*engineRejecting) UnmarshalText(text []byte) error { return engineHostileError(text) }
