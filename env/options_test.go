package env_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/muhammaadreza/gouse/env"
)

func TestOptions(t *testing.T) {
	type config struct {
		Value string `env:"VALUE"`
	}
	t.Setenv("APP_VALUE", "os value")
	got, err := env.Load[config](env.WithPrefix("APP_"))
	if err != nil || got.Value != "os value" {
		t.Fatalf("Load(default lookup) = %+v, %v", got, err)
	}
	got, err = env.Load[config](env.WithPrefix("APP_"), source(nil))
	if err != nil || got.Value != "" {
		t.Errorf("Load(isolated lookup) = %+v, %v, want zero", got, err)
	}
	got, err = env.Load[config](source(nil), source(map[string]string{"VALUE": "last"}), env.WithPrefix("OLD_"), env.WithPrefix(""))
	if err != nil || !reflect.DeepEqual(got, config{Value: "last"}) {
		t.Errorf("Load(last wins) = %+v, %v", got, err)
	}
}

func TestSourceOptionReplacement(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "file.env")
	writeEnvFile(t, path, "APP_VALUE=file\nVALUE=unprefixed")
	file := env.WithFiles(path)
	lookup := env.WithLookup(fixedLookup("lookup", true))
	sources := env.WithSources(env.FromLookup(fixedLookup("sources", true)))
	type config struct {
		Value string `env:"VALUE"`
	}
	for _, tt := range []struct {
		name string
		opts []env.Option
		want string
	}{
		{"file after lookup", []env.Option{lookup, file}, "file"},
		{"lookup after file", []env.Option{file, lookup}, "lookup"},
		{"sources after file", []env.Option{file, sources}, "sources"},
		{"file after sources", []env.Option{sources, file}, "file"},
		{"lookup after sources", []env.Option{sources, lookup}, "lookup"},
		{"sources after lookup", []env.Option{lookup, sources}, "sources"},
		{"overwritten missing file", []env.Option{env.WithFiles("does-not-exist"), lookup}, "lookup"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]env.Option{env.WithPrefix("OLD_")}, tt.opts...)
			opts = append(opts, env.WithPrefix("APP_"))
			cfg, err := env.Load[config](opts...)
			if err != nil || cfg.Value != tt.want {
				t.Fatalf("Load(%s) = %+v, %v; want %s", tt.name, cfg, err, tt.want)
			}
		})
	}
	for _, invalid := range []env.Option{nil, env.WithLookup(nil), env.WithSources(), env.WithSources(env.Source{}), env.WithSources(env.FromLookup(nil)), env.WithFiles(""), env.WithSources(env.FromFiles(path, "")), env.WithSources(env.FromLookup(fixedLookup("ok", true)), env.Source{})} {
		_, err := env.Load[config](invalid, lookup)
		if !errors.Is(err, env.ErrInvalidOption) {
			t.Errorf("Load(overwritten invalid source) = %v; want ErrInvalidOption", err)
		}
	}
}

func TestValidatorOptionsAndPhases(t *testing.T) {
	t.Parallel()
	valid := env.WithValidator(func(validatedConfig) error { return nil })
	for _, invalid := range []env.Option{
		env.WithValidator[validatedConfig](nil), env.WithValidator[*validatedConfig](nil),
		env.WithValidator(func(string) error { return nil }),
		env.WithValidator(func(any) error { return nil }),
		env.WithValidator(func(**validatedConfig) error { return nil }),
		env.WithValidator(func(struct {
			Value string `env:"VALUE"`
		}) error {
			return nil
		}),
	} {
		_, err := env.Load[validatedConfig](env.WithFiles("missing.env"), invalid, valid)
		if !errors.Is(err, env.ErrInvalidOption) || errors.Is(err, env.ErrSource) || errors.Is(err, env.ErrValidation) {
			t.Errorf("Load(overwritten invalid validator) = %v, want option before I/O", err)
		}
	}
	calls := 0
	_, err := env.Load[validatedConfig](source(nil), env.WithValidator(func(validatedConfig) error { t.Fatal("overwritten callback ran"); return nil }), env.WithValidator(func(validatedConfig) error { calls++; return nil }))
	if err != nil || calls != 1 {
		t.Fatalf("Load(last validator) = %v, calls %d", err, calls)
	}
	never := env.WithValidator(func(validatedConfig) error { t.Fatal("validator ran after source/value failure"); return nil })
	for _, opt := range []env.Option{env.WithFiles("missing.env"), source(map[string]string{"NUMBERS": "bad"})} {
		_, err := env.Load[validatedConfig](opt, never)
		if err == nil || errors.Is(err, env.ErrValidation) {
			t.Errorf("Load(pre-validator failure) = %v", err)
		}
	}
	type bad struct {
		Value int `binding:"required"`
	}
	_, err = env.Load[bad](env.WithFiles("missing.env"), env.WithValidator(func(bad) error { t.Fatal("validator ran after schema failure"); return nil }))
	if !errors.Is(err, env.ErrInvalidSchema) {
		t.Errorf("Load(schema before validator) = %v", err)
	}
}
