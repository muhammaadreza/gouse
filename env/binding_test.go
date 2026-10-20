package env_test

import (
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env"
)

func TestBindingResolution(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, binding, fallback, text  string
		hasBinding, hasFallback, found bool
		want                           string
		category                       error
	}{
		{name: "absent"},
		{name: "absent ignores returned text", text: "ignored"},
		{name: "explicit optional", hasBinding: true, binding: "optional"},
		{name: "required absent", hasBinding: true, binding: "required", category: env.ErrRequired},
		{name: "required empty", hasBinding: true, binding: "required", found: true},
		{name: "required whitespace", hasBinding: true, binding: "required", found: true, text: " \t\n\u2003"},
		{name: "required nonempty absent", hasBinding: true, binding: "required, nonempty", category: env.ErrRequired},
		{name: "required nonempty empty", hasBinding: true, binding: "required,nonempty", found: true, category: env.ErrEmptyValue},
		{name: "nonempty absent", hasBinding: true, binding: "nonempty"},
		{name: "nonempty whitespace", hasBinding: true, binding: "nonempty", found: true, text: " \t\n\u2003", category: env.ErrEmptyValue},
		{name: "nonempty notrim whitespace", hasBinding: true, binding: "nonempty,notrim", found: true, text: " \t\n\u2003", want: " \t\n\u2003"},
		{name: "default trim", found: true, text: " \t\n\u2003a  b\t\n", want: "a  b"},
		{name: "notrim", hasBinding: true, binding: "notrim", found: true, text: "  a  ", want: "  a  "},
		{name: "fallback absent", hasFallback: true, fallback: " x ", want: "x"},
		{name: "fallback empty", hasFallback: true},
		{name: "fallback empty nonempty", hasBinding: true, binding: "nonempty", hasFallback: true, category: env.ErrEmptyValue},
		{name: "fallback notrim", hasBinding: true, binding: "notrim", hasFallback: true, fallback: " x ", want: " x "},
		{name: "present empty blocks fallback", hasFallback: true, fallback: "x", found: true},
		{name: "present whitespace blocks fallback", hasFallback: true, fallback: "x", found: true, text: "  "},
		{name: "present wins", hasFallback: true, fallback: "x", found: true, text: " y ", want: "y"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tags := `env:"V"`
			if tt.hasBinding {
				tags += " binding:" + strconv.Quote(tt.binding)
			}
			if tt.hasFallback {
				tags += " fallback:" + strconv.Quote(tt.fallback)
			}
			dst := dynamicConfig(reflect.TypeFor[string](), tags)
			err := env.Parse(dst, env.WithLookup(fixedLookup(tt.text, tt.found)))
			if !errors.Is(err, tt.category) {
				t.Fatalf("Parse(%s) = %v, want %v", tt.name, err, tt.category)
			}
			if tt.category == env.ErrEmptyValue && (!errors.Is(err, env.ErrInvalidValue) || errors.Is(err, env.ErrRequired)) {
				t.Errorf("Parse(nonempty) categories = %v, want empty and invalid, without required", err)
			}
			if tt.category == nil && reflect.ValueOf(dst).Elem().Field(0).String() != tt.want {
				t.Errorf("Parse(%s) = %+v, want %q", tt.name, dst, tt.want)
			}
		})
	}
}

func TestBindingGrammarBeforeLookup(t *testing.T) {
	t.Parallel()
	for _, binding := range []string{"", " \t ", "unknown", "trim", "required,optional", "optional,required", "required,required", "required, required", "notrim,notrim", "nonempty,nonempty", ",required", "required,", "required,,nonempty"} {
		t.Run(binding, func(t *testing.T) {
			t.Parallel()
			err := env.Parse(dynamicConfig(reflect.TypeFor[unexpectedDecoder](), `env:"V" binding:`+strconv.Quote(binding)), env.WithFiles("missing.env"))
			if !errors.Is(err, env.ErrInvalidSchema) || errors.Is(err, env.ErrSource) {
				t.Errorf("Parse(binding %q) = %v, want schema before I/O", binding, err)
			}
		})
	}
	for _, fallback := range []string{"", "unused"} {
		err := env.Parse(dynamicConfig(reflect.TypeFor[string](), `env:"V" binding:"required" fallback:`+strconv.Quote(fallback)), env.WithFiles("missing.env"))
		if !errors.Is(err, env.ErrInvalidSchema) {
			t.Errorf("Parse(required + fallback %q) = %v, want schema", fallback, err)
		}
	}
	for _, binding := range []string{"optional", "required", "nonempty", "notrim"} {
		typ := reflect.StructOf([]reflect.StructField{{Name: "Child", Type: reflect.TypeFor[struct{}](), Tag: reflect.StructTag("binding:" + strconv.Quote(binding))}})
		err := env.Parse(reflect.New(typ).Interface(), env.WithFiles("missing.env"))
		if !errors.Is(err, env.ErrInvalidSchema) {
			t.Errorf("Parse(container %s) = %v, want schema", binding, err)
		}
	}
}

func TestEmptyConversion(t *testing.T) {
	t.Parallel()
	type cfg struct {
		Text          string           `env:"V"`
		Pointer       *string          `env:"V"`
		Slice         []string         `env:"V"`
		Custom        integrationText  `env:"V"`
		CustomPointer *integrationText `env:"V"`
	}
	absent, err := env.Load[cfg](source(nil))
	if err != nil || !reflect.DeepEqual(absent, cfg{}) {
		t.Fatalf("Load(absent) = %+v, %v, want zero", absent, err)
	}
	empty, err := env.Load[cfg](source(map[string]string{"V": " \t\n\u2003"}))
	if err != nil || empty.Pointer == nil || *empty.Pointer != "" || empty.Slice == nil || len(empty.Slice) != 0 || empty.Custom != "decoded:" || empty.CustomPointer == nil || *empty.CustomPointer != "decoded:" {
		t.Fatalf("Load(empty selected) = %+v, %v", empty, err)
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[int](), reflect.TypeFor[bool](), reflect.TypeFor[time.Duration](), reflect.TypeFor[*int](), reflect.TypeFor[*bool](), reflect.TypeFor[*time.Duration](), reflect.TypeFor[time.Time]()} {
		for _, tags := range []string{`env:"V"`, `env:"V" fallback:""`} {
			err := env.Parse(dynamicConfig(typ, tags), source(map[string]string{"V": ""}))
			if !errors.Is(err, env.ErrInvalidValue) || errors.Is(err, env.ErrEmptyValue) {
				t.Errorf("Parse(empty %s) = %v, want invalid value only", typ, err)
			}
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[*string](), reflect.TypeFor[[]string](), reflect.TypeFor[integrationText]()} {
		dst := dynamicConfig(typ, `env:"V" fallback:""`)
		if err := env.Parse(dst, source(nil)); err != nil {
			t.Errorf("Parse(empty fallback %s) = %v", typ, err)
		}
		value := reflect.ValueOf(dst).Elem().Field(0)
		if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Slice) && value.IsNil() {
			t.Errorf("Parse(empty fallback %s) left nil", typ)
		}
	}
	cfgTokens, err := env.Load[struct {
		Values []string `env:"V" binding:"nonempty"`
	}](source(map[string]string{"V": "a,,b"}))
	if err != nil || !reflect.DeepEqual(cfgTokens.Values, []string{"a", "", "b"}) {
		t.Fatalf("Load(tokens) = %+v, %v", cfgTokens, err)
	}
}

func TestSliceTrimAndNotrim(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, text, binding string
		typ                 reflect.Type
		want                any
		category            error
	}{
		{"int compact default", "1,2", "", reflect.TypeFor[[]int](), []int{1, 2}, nil},
		{"int compact notrim", "1,2", "notrim", reflect.TypeFor[[]int](), []int{1, 2}, nil},
		{"int spaced default", "1, 2", "", reflect.TypeFor[[]int](), []int{1, 2}, nil},
		{"int spaced notrim", "1, 2", "notrim", reflect.TypeFor[[]int](), nil, env.ErrInvalidValue},
		{"bool spaced notrim", "true, false", "notrim", reflect.TypeFor[[]bool](), nil, env.ErrInvalidValue},
		{"duration spaced notrim", "1s, 2s", "notrim", reflect.TypeFor[[]time.Duration](), nil, env.ErrInvalidValue},
		{"string trimmed", " \u2003a , b\n", "", reflect.TypeFor[[]string](), []string{"a", "b"}, nil},
		{"string raw", " \u2003a , b\n", "notrim", reflect.TypeFor[[]string](), []string{" \u2003a ", " b\n"}, nil},
		{"custom tokens raw", " a , b ", "notrim", reflect.TypeFor[[]integrationText](), []integrationText{"decoded: a ", "decoded: b "}, nil},
		{"custom tokens trimmed", " a , b ", "", reflect.TypeFor[[]integrationText](), []integrationText{"decoded:a", "decoded:b"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, fallback := range []bool{false, true} {
				tags := `env:"V"`
				if tt.binding != "" {
					tags += " binding:" + strconv.Quote(tt.binding)
				}
				values := map[string]string{"V": tt.text}
				if fallback {
					tags += " fallback:" + strconv.Quote(tt.text)
					values = nil
				}
				dst := dynamicConfig(tt.typ, tags)
				err := env.Parse(dst, source(values))
				if !errors.Is(err, tt.category) {
					t.Fatalf("Parse(%s fallback %v) = %v, want %v", tt.name, fallback, err, tt.category)
				}
				if err == nil && !reflect.DeepEqual(reflect.ValueOf(dst).Elem().Field(0).Interface(), tt.want) {
					t.Errorf("Parse(%s) = %+v, want %+v", tt.name, dst, tt.want)
				}
				if err != nil {
					var f *env.FieldError
					if !errors.As(err, &f) || f.Index != 1 {
						t.Errorf("Parse(%s) = %v, want index 1", tt.name, err)
					}
				}
			}
		})
	}
}
