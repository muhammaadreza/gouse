package decoder_test

import (
	"reflect"
	"testing"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

func TestSchemaValidation(t *testing.T) {
	t.Parallel()
	intType := reflect.TypeFor[int]()
	tests := []struct {
		name string
		dst  any
	}{
		{"empty key", dynamicConfig(intType, `env:""`)},
		{"unknown binding", dynamicConfig(intType, `env:"V" binding:"yes"`)},
		{"empty binding", dynamicConfig(intType, `env:"V" binding:""`)},
		{"binding without env", dynamicConfig(intType, `binding:"required"`)},
		{"fallback without env", dynamicConfig(intType, `fallback:"3"`)},
		{"separator without env", dynamicConfig(reflect.TypeFor[[]int](), `separator:";"`)},
		{"prefix leaf", dynamicConfig(intType, `prefix:"P_"`)},
		{"env and prefix", dynamicConfig(intType, `env:"V" prefix:"P_"`)},
		{"separator scalar", dynamicConfig(intType, `env:"V" separator:";"`)},
		{"empty separator", dynamicConfig(reflect.TypeFor[[]int](), `env:"V" separator:""`)},
		{"container fallback", &struct {
			Child Embedded `fallback:"x"`
		}{}},
		{"container binding", &struct {
			Child Embedded `binding:"optional"`
		}{}},
		{"container separator", &struct {
			Child Embedded `separator:";"`
		}{}},
		{"direct ordinary struct", &struct {
			Child Embedded `env:"V"`
		}{}},
		{"direct ordinary pointer struct", &struct {
			Child *Embedded `env:"V"`
		}{}},
		{"embedded", &struct{ Embedded }{}},
		{"unexported embedded", &struct{ recursive }{}},
		{"unexported tagged", &struct {
			hidden int `env:"HIDDEN"`
		}{}},
		{"unexported prefix", &struct {
			hidden Embedded `prefix:"H_"`
		}{}},
		{"recursive", &recursive{}},
	}
	for _, tag := range []string{`binding:"optional"`, `fallback:""`, `separator:","`, `prefix:""`} {
		tests = append(tests, struct {
			name string
			dst  any
		}{"skip conflict " + tag, dynamicConfig(intType, `env:"-" `+tag)})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan, diagnostics := decoder.Compile(reflect.TypeOf(tt.dst).Elem(), "")
			want := decoder.InvalidSchema
			switch tt.name {
			case "direct ordinary struct", "direct ordinary pointer struct":
				want = decoder.UnsupportedType
			}
			if plan != nil || len(diagnostics) != 1 || diagnostics[0].Category != want {
				t.Errorf("Compile(%s) = %v, %+v; want nil Plan and category %v", tt.name, plan, diagnostics, want)
			}
		})
	}
}

func TestUnsupportedTypes(t *testing.T) {
	t.Parallel()
	for _, typ := range []reflect.Type{
		reflect.TypeFor[uintptr](), reflect.TypeFor[map[string]int](), reflect.TypeFor[[2]int](),
		reflect.TypeFor[complex64](), reflect.TypeFor[complex128](), reflect.TypeFor[chan int](),
		reflect.TypeFor[any](), reflect.TypeFor[func()](), reflect.TypeFor[**int](),
		reflect.TypeFor[[]*int](), reflect.TypeFor[*[]int](), reflect.TypeFor[[][]int](),
		reflect.TypeFor[recursiveSlice](),
	} {
		t.Run(typ.String(), func(t *testing.T) {
			t.Parallel()
			plan, diagnostics := decoder.Compile(dynamicType(typ, `env:"V"`), "")
			if plan != nil || !hasCategory(diagnostics, decoder.UnsupportedType) {
				t.Errorf("Decode(%s) = %v, want unsupported schema", typ, diagnostics)
			}
		})
	}
}

func TestIgnoredFieldsAndSkippedSubtrees(t *testing.T) {
	t.Parallel()
	type config struct {
		Ignored     int `json:"ignored"`
		hidden      int
		Unsupported map[string]int
		Skipped     *recursive `env:"-"`
		Embedded    `env:"-"`
		hiddenSkip  int    `env:"-"`
		Active      string `env:"ACTIVE" binding:"optional"`
	}
	calls := []string{}
	candidate := mustDecode(t, reflect.TypeFor[config](), "", func(key string) (string, bool) { calls = append(calls, key); return " new ", true })
	cfg := candidate.Interface().(config)
	if !reflect.DeepEqual(cfg, config{Active: "new"}) || !reflect.DeepEqual(calls, []string{"ACTIVE"}) {
		t.Errorf("Decode(skips) = %+v, keys %v", cfg, calls)
	}
}

func TestCompileReturnsNoPartialPlan(t *testing.T) {
	t.Parallel()
	type config struct {
		Custom customText `env:"CUSTOM"`
		First  int        `binding:"required"`
		Nested struct {
			Bad map[string]int `env:"BAD"`
		} `prefix:"N_"`
	}
	plan, diagnostics := decoder.Compile(reflect.TypeFor[config](), "APP_")
	want := []decoder.Diagnostic{
		{Path: "First", Index: -1, Category: decoder.InvalidSchema},
		{Path: "Nested.Bad", Key: "APP_N_BAD", Index: -1, Category: decoder.UnsupportedType},
	}
	if plan != nil || !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("Compile(invalid schema) = %v, %+v; want nil, %+v", plan, diagnostics, want)
	}
}
