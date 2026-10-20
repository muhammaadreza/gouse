package env_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env"
)

func source(values map[string]string) env.Option {
	return env.WithLookup(func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	})
}

func TestInvalidInputs(t *testing.T) {
	t.Parallel()
	type config struct {
		Value string `env:"VALUE"`
	}
	var ptr *config
	var number int
	for _, dst := range []any{nil, ptr, config{}, &ptr, &number, 1} {
		if err := env.Parse(dst); !errors.Is(err, env.ErrInvalidOption) {
			t.Errorf("Parse(%T) = %v, want ErrInvalidOption", dst, err)
		}
	}
	for _, opt := range []env.Option{nil, env.WithLookup(nil)} {
		if _, err := env.Load[config](opt); !errors.Is(err, env.ErrInvalidOption) {
			t.Errorf("Load(invalid option) = %v, want ErrInvalidOption", err)
		}
	}
	_, intErr := env.Load[int]()
	_, ptrErr := env.Load[*config]()
	_, interfaceErr := env.Load[any]()
	for _, err := range []error{intErr, ptrErr, interfaceErr} {
		var field *env.FieldError
		if !errors.Is(err, env.ErrInvalidOption) || errors.As(err, &field) {
			t.Errorf("Load(invalid type) = %v, want ErrInvalidOption without FieldError", err)
		}
	}
}

func TestIndependentConcurrentCalls(t *testing.T) {
	t.Parallel()
	type child struct {
		Value int `env:"V"`
	}
	type config struct {
		Child  *child
		Values []int `env:"VALUES"`
	}
	lookup := source(map[string]string{"V": "42", "VALUES": "1,2"})
	for i := range 64 {
		t.Run(fmt.Sprintf("call %d", i), func(t *testing.T) {
			t.Parallel()
			loaded, err := env.Load[config](lookup)
			if err != nil || loaded.Child == nil || loaded.Child.Value != 42 || !reflect.DeepEqual(loaded.Values, []int{1, 2}) {
				t.Fatalf("Load(concurrent) = %+v, %v", loaded, err)
			}
			var parsed config
			if err := env.Parse(&parsed, lookup); err != nil || !reflect.DeepEqual(parsed, loaded) {
				t.Errorf("Parse(concurrent) = %+v, %v, want %+v", parsed, err, loaded)
			}
		})
	}
}

func TestAtomicReplacement(t *testing.T) {
	t.Parallel()
	type child struct {
		Value int `env:"VALUE"`
	}
	type config struct {
		Name     string `env:"NAME"`
		Numbers  []int  `env:"NUMBERS"`
		Child    *child
		Optional *int `env:"OPTIONAL"`
		Untagged string
		Last     bool `env:"LAST"`
	}
	oldChild := &child{Value: 9}
	oldNumbers := []int{8, 9}
	oldOptional := 7
	cfg := config{Name: "old", Numbers: oldNumbers, Child: oldChild, Optional: &oldOptional, Untagged: "old", Last: true}
	original := cfg
	err := env.Parse(&cfg, source(map[string]string{"NAME": "new", "NUMBERS": "1,2", "VALUE": "5", "LAST": "invalid"}))
	if !errors.Is(err, env.ErrInvalidValue) || !reflect.DeepEqual(cfg, original) || cfg.Child != oldChild || cfg.Optional != &oldOptional || &cfg.Numbers[0] != &oldNumbers[0] || oldChild.Value != 9 {
		t.Fatalf("Parse(partial failure) = %+v, %v, want untouched original", cfg, err)
	}
	err = env.Parse(&cfg, source(map[string]string{"NAME": "new"}))
	want := config{Name: "new", Child: &child{}}
	if err != nil || !reflect.DeepEqual(cfg, want) || cfg.Child == oldChild {
		t.Errorf("Parse(success replacement) = %+v, %v, want %+v with new pointer", cfg, err, want)
	}
	failed, err := env.Load[config](source(map[string]string{"NAME": "new", "NUMBERS": "1,2", "LAST": "bad"}))
	if !errors.Is(err, env.ErrInvalidValue) || !reflect.DeepEqual(failed, config{}) {
		t.Errorf("Load(failure) = %+v, %v, want zero", failed, err)
	}
}

func TestSchemaOrderAndPhase(t *testing.T) {
	t.Parallel()
	type config struct {
		Good   int `env:"GOOD"`
		First  int `env:"FIRST" binding:"unknown"`
		Nested struct {
			Bad   int            `env:""`
			Worse map[string]int `env:"MAP"`
		} `prefix:"N_"`
		Last int `fallback:"secret fallback"`
	}
	for range 20 {
		calls := 0
		_, err := env.Load[config](env.WithPrefix("APP_"), env.WithLookup(func(string) (string, bool) { calls++; return "invalid", true }))
		fields := fieldErrors(err)
		var paths []string
		for _, field := range fields {
			paths = append(paths, field.Path)
		}
		want := []string{"First", "Nested.Bad", "Nested.Worse", "Last"}
		if !reflect.DeepEqual(paths, want) || calls != 0 {
			t.Fatalf("Load(schema errors) = %v, %d lookups, want %v, 0", paths, calls, want)
		}
		if fields[2].Key != "APP_N_MAP" || !errors.Is(err, env.ErrUnsupportedType) {
			t.Errorf("Load(schema metadata) = %+v, want full key and unsupported category", fields[2])
		}
	}
}

func TestCustomFailureAndSchemaPhase(t *testing.T) {
	t.Parallel()
	type badSchema struct {
		Custom hostile `env:"V"`
		Bad    int     `binding:"required"`
	}
	_, err := env.Load[badSchema](source(map[string]string{"V": "should not decode"}))
	if !errors.Is(err, env.ErrInvalidSchema) {
		t.Errorf("Load(schema before custom) = %v", err)
	}
}

type integrationText string

func (v *integrationText) UnmarshalText(text []byte) error {
	*v = integrationText("decoded:" + string(text))
	return nil
}

func TestPublicDecodingIntegration(t *testing.T) {
	t.Parallel()
	type database struct {
		Host string `env:"HOST"`
	}
	type config struct {
		Name        string          `env:"NAME"`
		Enabled     bool            `env:"ENABLED"`
		Port        *int            `env:"PORT"`
		Numbers     []int           `env:"NUMBERS"`
		Timeout     time.Duration   `env:"TIMEOUT" fallback:"2s"`
		Level       integrationText `env:"LEVEL"`
		DB          *database       `prefix:"DB_"`
		Ignored     int
		hidden      int
		Unsupported map[string]int
		Skipped     map[string]int `env:"-"`
	}
	values := map[string]string{
		"APP_NAME": " service ", "APP_ENABLED": "true", "APP_PORT": "8080",
		"APP_NUMBERS": "1, 2", "APP_LEVEL": "debug", "APP_DB_HOST": "db.local",
	}
	opts := []env.Option{env.WithPrefix("APP_"), source(values)}
	port := 8080
	want := config{Name: "service", Enabled: true, Port: &port, Numbers: []int{1, 2}, Timeout: 2 * time.Second, Level: "decoded:debug", DB: &database{Host: "db.local"}}
	loaded, err := env.Load[config](opts...)
	if err != nil || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("Load(public integration) = %+v, %v; want %+v", loaded, err, want)
	}
	parsed := config{Ignored: 1, hidden: 2, Unsupported: map[string]int{"old": 3}, Skipped: map[string]int{"old": 4}}
	if err := env.Parse(&parsed, opts...); err != nil || !reflect.DeepEqual(parsed, want) {
		t.Errorf("Parse(public integration) = %+v, %v; want %+v with ignored fields cleared", parsed, err, want)
	}
}

func TestMalformedSchemaPreservesDestination(t *testing.T) {
	t.Parallel()
	dst := dynamicConfig(reflect.TypeFor[string](), `env:"VALUE" fallback:"CANARY_MALFORMED_`)
	value := reflect.ValueOf(dst).Elem().Field(0)
	value.SetString("old")
	err := env.Parse(dst, env.WithLookup(func(string) (string, bool) {
		t.Fatal("malformed schema invoked lookup")
		return "", false
	}))
	if !errors.Is(err, env.ErrInvalidSchema) || value.String() != "old" {
		t.Fatalf("Parse(malformed schema) = %v, value %q; want invalid schema and old", err, value.String())
	}
	assertPrivate(t, err, "CANARY_MALFORMED_")
}
