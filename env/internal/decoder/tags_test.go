package decoder_test

import (
	"reflect"
	"testing"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

func TestMalformedOwnedTags(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"env", "binding", "fallback", "separator", "prefix"} {
		for _, entry := range []string{
			key, key + ":", key + ":unquoted", key + `:"unterminated`,
			key + `:"trailing\`, key + `:"bad\qescape"`, key + ` "missing colon"`,
		} {
			t.Run(entry, func(t *testing.T) {
				t.Parallel()
				for _, prefix := range []string{"", `json:"unrelated" env:"VALUE" `} {
					plan, diagnostics := decoder.Compile(dynamicType(reflect.TypeFor[string](), prefix+entry), "")
					if plan != nil || !hasCategory(diagnostics, decoder.InvalidSchema) {
						t.Errorf("Compile(malformed %q) = %v, %+v; want nil and invalid schema", prefix+entry, plan, diagnostics)
					}

				}
			})
		}
	}
}

func TestTagSyntaxCompatibility(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, tags, want string }{
		{"unrelated value containing owned text", `json:"binding:required env:\"broken" env:"V" fallback:"ok"`, "ok"},
		{"escaped quote", `env:"V" fallback:"say \"hello\""`, `say "hello"`},
		{"escaped backslash", `env:"V" fallback:"a\\b"`, `a\b`},
		{"unicode escape", `env:"V" fallback:"\u0061"`, "a"},
		{"trailing spaces", `env:"V" fallback:"ok"   `, "ok"},
		{"spaces only", "   ", ""},
		{"unrelated malformed", `json:unquoted`, ""},
		{"unrelated unterminated", `json:"unterminated`, ""},
		{"unrelated invalid escape", `json:"\q" env:"V" fallback:"ok"`, "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			typ := dynamicType(reflect.TypeFor[string](), tt.tags)
			candidate := mustDecode(t, typ, "", lookup(nil))
			got := candidate.Field(0).String()
			if got != tt.want {
				t.Errorf("Decode(%q) = %q; want %q", tt.tags, got, tt.want)
			}
		})
	}
}

func TestMalformedTagOrderAndPrivacy(t *testing.T) {
	t.Parallel()
	const secret = "CANARY_MALFORMED_71e6"
	fields := []reflect.StructField{
		{Name: "First", Type: reflect.TypeFor[string](), Tag: reflect.StructTag(`env:"FIRST" binding:` + secret)},
		{Name: "Nested", Type: reflect.StructOf([]reflect.StructField{
			{Name: "Child", Type: reflect.TypeFor[string](), Tag: reflect.StructTag(`env:"CHILD" fallback:"` + secret)},
		})},
		{Name: "Last", Type: reflect.TypeFor[int](), Tag: reflect.StructTag(`env:"LAST" separator:"\q` + secret + `"`)},
	}
	plan, diagnostics := decoder.Compile(reflect.StructOf(fields), "")
	if plan != nil {
		t.Fatal("Compile(malformed tags) returned partial Plan")
	}
	var paths []string
	for _, field := range diagnostics {
		paths = append(paths, field.Path)
	}
	want := []string{"First", "Nested.Child", "Last"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("Decode(malformed order) = %v, want %v", paths, want)
	}
	assertDiagnosticPrivacy(t, diagnostics, secret)
	// A malformed conflicting tag on a skip field must also be rejected.
	plan, diagnostics = decoder.Compile(dynamicType(reflect.TypeFor[string](), `env:"-" binding:required`), "")
	if plan != nil || !hasCategory(diagnostics, decoder.InvalidSchema) {
		t.Errorf("Decode(skip with malformed binding) = %+v", diagnostics)
	}
}
