package env_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/muhammaadreza/gouse/env"
)

func TestEngineDiagnosticsBecomePublicFieldErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		typ      reflect.Type
		tags     string
		lookup   env.LookupFunc
		category error
	}{
		{name: "required", typ: reflect.TypeFor[int](), tags: `env:"VALUE" binding:"required"`, lookup: fixedLookup("ignored", false), category: env.ErrRequired},
		{name: "invalid value", typ: reflect.TypeFor[int](), tags: `env:"VALUE"`, lookup: fixedLookup("CANARY_DIAGNOSTIC_VALUE", true), category: env.ErrInvalidValue},
		{name: "unsupported type", typ: reflect.TypeFor[map[string]int](), tags: `env:"VALUE"`, lookup: fixedLookup("ignored", false), category: env.ErrUnsupportedType},
		{name: "invalid schema", typ: reflect.TypeFor[int](), tags: `env:"VALUE" binding:"CANARY_DIAGNOSTIC_TAG"`, lookup: fixedLookup("ignored", false), category: env.ErrInvalidSchema},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := env.Parse(dynamicConfig(tt.typ, tt.tags), env.WithPrefix("APP_"), env.WithLookup(tt.lookup))
			var field *env.FieldError
			if !errors.Is(err, tt.category) || !errors.As(err, &field) {
				t.Fatalf("Parse(%s) = %v; want public FieldError with %v", tt.name, err, tt.category)
			}
			if field.Path != "Value" || field.Key != "APP_VALUE" || field.Index != -1 || field.Category != tt.category || errors.Unwrap(field) != tt.category {
				t.Errorf("Parse(%s) metadata = %+v; want Value/APP_VALUE/-1 and original sentinel", tt.name, field)
			}
			joined, ok := err.(interface{ Unwrap() []error })
			if !ok || len(joined.Unwrap()) != 1 || joined.Unwrap()[0] != field {
				t.Errorf("Parse(%s) chain = %T; want aggregate containing only public FieldError", tt.name, err)
			}
			if tt.category == env.ErrUnsupportedType && !errors.Is(err, env.ErrInvalidSchema) {
				t.Error("unsupported type no longer matches invalid schema")
			}
			assertPrivate(t, err, "CANARY_DIAGNOSTIC_VALUE", "CANARY_DIAGNOSTIC_TAG")
		})
	}
	const publicPackage = "github.com/muhammaadreza/gouse/env"
	for _, typ := range []reflect.Type{reflect.TypeFor[env.FieldError](), reflect.TypeFor[env.SourceError]()} {
		if typ.PkgPath() != publicPackage {
			t.Errorf("%s owner = %s; want %s", typ.Name(), typ.PkgPath(), publicPackage)
		}
	}
}

func TestPublicValueErrorOrder(t *testing.T) {
	t.Parallel()
	type config struct {
		Missing string `env:"MISSING" binding:"required"`
		Ports   []int  `env:"PORTS"`
		Last    bool   `env:"LAST"`
	}
	_, err := env.Load[config](source(map[string]string{"PORTS": "1,bad", "LAST": "bad"}))
	fields := fieldErrors(err)
	want := []env.FieldError{
		{Path: "Missing", Key: "MISSING", Index: -1, Category: env.ErrRequired},
		{Path: "Ports", Key: "PORTS", Index: 1, Category: env.ErrInvalidValue},
		{Path: "Last", Key: "LAST", Index: -1, Category: env.ErrInvalidValue},
	}
	if !errors.Is(err, env.ErrRequired) || !errors.Is(err, env.ErrInvalidValue) || len(fields) != len(want) {
		t.Fatalf("Load(public aggregate) = %v; want required and invalid value in three field errors", err)
	}
	for i, field := range fields {
		if *field != want[i] || !errors.Is(field, field.Category) {
			t.Errorf("Load(public aggregate)[%d] = %+v; want %+v with sentinel classification", i, field, want[i])
		}
	}
	var first *env.FieldError
	if !errors.As(err, &first) || first != fields[0] {
		t.Error("errors.As(public aggregate) did not return the first field")
	}
}

func dynamicConfig(t reflect.Type, tags string) any {
	schema := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: t, Tag: reflect.StructTag(tags)}})
	return reflect.New(schema).Interface()
}

func fieldErrors(err error) []*env.FieldError {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var fields []*env.FieldError
		for _, child := range joined.Unwrap() {
			fields = append(fields, fieldErrors(child)...)
		}
		return fields
	}
	var field *env.FieldError
	if errors.As(err, &field) {
		return []*env.FieldError{field}
	}
	return nil
}

type secretError string

func (s secretError) Error() string { return "decoder secret: " + string(s) }

type rejecting string

func (*rejecting) UnmarshalText(text []byte) error { return secretError(string(text)) }

type hostileError struct{}

func (hostileError) Error() string { panic("must never render raw custom error") }

type hostile string

func (*hostile) UnmarshalText([]byte) error { return hostileError{} }

func TestErrorPrivacy(t *testing.T) {
	t.Parallel()
	const envSecret = "CANARY_ENV_3ae275"
	const fallbackSecret = "CANARY_FALLBACK_e5c172"
	type config struct {
		Number         int         `env:"NUMBER"`
		Custom         rejecting   `env:"CUSTOM"`
		Fallback       int         `env:"FALLBACK" fallback:"CANARY_FALLBACK_e5c172"`
		CustomFallback rejecting   `env:"CUSTOM_FALLBACK" fallback:"CANARY_FALLBACK_e5c172"`
		Slice          []int       `env:"SLICE"`
		CustomSlice    []rejecting `env:"CUSTOM_SLICE"`
		Hostile        hostile     `env:"HOSTILE"`
	}
	_, err := env.Load[config](source(map[string]string{"NUMBER": envSecret, "CUSTOM": envSecret, "SLICE": envSecret + "," + envSecret, "CUSTOM_SLICE": envSecret + "," + envSecret, "HOSTILE": envSecret}))
	if !errors.Is(err, env.ErrInvalidValue) {
		t.Fatalf("Load(secrets) = %v", err)
	}
	assertPrivate(t, err, envSecret, fallbackSecret)
	var numErr *strconv.NumError
	var decoderErr secretError
	if errors.As(err, &numErr) || errors.As(err, &decoderErr) {
		t.Error("Load(secrets) exposes original parser/custom error via errors.As")
	}
	fields := fieldErrors(err)
	if len(fields) != 9 {
		t.Errorf("Load(secrets) returned %d field errors, want 9", len(fields))
	}
	for _, field := range fields {
		if field.Category != env.ErrInvalidValue || field.Unwrap() != env.ErrInvalidValue {
			t.Errorf("FieldError = %+v, want sentinel-only cause", field)
		}
	}
	// Schema diagnostics must not retain fallback text or malformed binding text.
	err = env.Parse(dynamicConfig(reflect.TypeFor[int](), `env:"V" binding:"CANARY_ENV_3ae275" fallback:"CANARY_FALLBACK_e5c172"`), source(nil))
	assertPrivate(t, err, envSecret, fallbackSecret)
}

func assertPrivate(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a non-nil error for privacy inspection")
	}
	for _, formatted := range []string{err.Error(), fmt.Sprintf("%s %v %+v %#v", err, err, err, err)} {
		for _, secret := range secrets {
			if strings.Contains(formatted, secret) {
				t.Errorf("error surface leaks canary: %s", formatted)
			}
		}
	}
	value := reflect.ValueOf(err)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() == reflect.Struct {
		for i := 0; i < value.NumField(); i++ {
			if value.Type().Field(i).IsExported() {
				metadata := fmt.Sprintf("%+v %#v", value.Field(i).Interface(), value.Field(i).Interface())
				for _, secret := range secrets {
					if strings.Contains(metadata, secret) {
						t.Errorf("exported error metadata leaks canary: %s", metadata)
					}
				}
			}
		}
	}
	switch chain := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range chain.Unwrap() {
			assertPrivate(t, child, secrets...)
		}
	case interface{ Unwrap() error }:
		if child := chain.Unwrap(); child != nil {
			assertPrivate(t, child, secrets...)
		}
	}
}

func TestErrorMessageCatalog(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		typ        reflect.Type
		tags, text string
		found      bool
		want       string
		category   error
	}{
		{"required", reflect.TypeFor[string](), `env:"V" binding:"required"`, "", false, `field "Value" key "V": required key missing`, env.ErrRequired},
		{"empty", reflect.TypeFor[string](), `env:"V" binding:"nonempty"`, "", true, `field "Value" key "V": empty value`, env.ErrEmptyValue},
		{"invalid", reflect.TypeFor[int](), `env:"V"`, "CANARY_INVALID", true, `field "Value" key "V": invalid value`, env.ErrInvalidValue},
		{"indexed", reflect.TypeFor[[]int](), `env:"V"`, "1,2,CANARY_INVALID", true, `field "Value" key "V"[2]: invalid value`, env.ErrInvalidValue},
		{"unsupported", reflect.TypeFor[map[string]int](), `env:"V"`, "", false, `field "Value" key "V": unsupported type`, env.ErrUnsupportedType},
		{"schema", reflect.TypeFor[string](), `env:"V" binding:"CANARY_FLAG"`, "", false, `field "Value" key "V": invalid schema`, env.ErrInvalidSchema},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := env.Parse(dynamicConfig(tt.typ, tt.tags), env.WithLookup(fixedLookup(tt.text, tt.found)))
			if !errors.Is(err, tt.category) || err.Error() != tt.want {
				t.Fatalf("Parse(%s) = %v, want %s and category %v", tt.name, err, tt.want, tt.category)
			}
			assertPrivate(t, err, "CANARY_INVALID", "CANARY_FLAG")
		})
	}
	for _, tt := range []struct{ input, reason string }{
		{"CANARY_LINE", "invalid assignment"},
		{"V='CANARY_LINE", "invalid quoted value"},
		{"V=CANARY_LINE\nV=CANARY_LINE", "duplicate key"},
		{"V=CANARY_LINE\xff", "invalid encoding or control character"},
	} {
		path := filepath.Join(t.TempDir(), "syntax.env")
		writeEnvFile(t, path, "# comment\n"+tt.input)
		_, err := env.Read(path)
		var source *env.SourceError
		if !errors.Is(err, env.ErrSourceSyntax) || errors.Is(err, env.ErrSourceRead) || !errors.As(err, &source) || !strings.HasSuffix(err.Error(), ": "+tt.reason) || !strings.Contains(err.Error(), " line ") {
			t.Fatalf("Read(%s) = %v", tt.reason, err)
		}
		assertPrivate(t, err, "CANARY_LINE")
	}
	for _, tt := range []struct {
		opt  env.Option
		want string
	}{
		{env.WithLookup(nil), "invalid option or input: nil lookup"},
		{env.WithValidator[struct{}](nil), "invalid option or input: nil validator"},
		{env.WithFiles(""), "invalid option or input: empty file path"},
	} {
		_, err := env.Load[struct{}](tt.opt)
		var field *env.FieldError
		if !errors.Is(err, env.ErrInvalidOption) || errors.As(err, &field) || err.Error() != tt.want {
			t.Errorf("Load(invalid API) = %v, want %s without FieldError", err, tt.want)
		}
	}
	if err := env.Parse(nil); err.Error() != "invalid option or input: destination must be a non-nil pointer to a struct" {
		t.Errorf("Parse(nil) = %v", err)
	}
	for _, sentinel := range []error{env.ErrRequired, env.ErrEmptyValue, env.ErrInvalidValue, env.ErrUnsupportedType, env.ErrInvalidSchema, env.ErrInvalidOption, env.ErrSource, env.ErrSourceRead, env.ErrSourceNotFound, env.ErrSourcePermission, env.ErrSourceSyntax, env.ErrSourceSetenv, env.ErrValidation} {
		if strings.Contains(sentinel.Error(), "env:") {
			t.Errorf("sentinel has prefix: %v", sentinel)
		}
	}
}
