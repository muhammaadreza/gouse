package env_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/muhammaadreza/gouse/env"
)

func fixedLookup(value string, found bool) env.LookupFunc {
	return func(string) (string, bool) { return value, found }
}

func TestSourceSelectionMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, first string
		found       bool
		secondFound bool
		want        int
		wantErr     error
		secondCalls int
	}{
		{"first", "9000", true, true, 9000, nil, 0},
		{"miss", "ignored nonempty", false, true, 8080, nil, 1},
		{"empty", "", true, true, 0, env.ErrInvalidValue, 0},
		{"whitespace", " \t\n\u2003", true, true, 0, env.ErrInvalidValue, 0},
		{"invalid", "invalid", true, true, 0, env.ErrInvalidValue, 0},
		{"all miss", "ignored", false, false, 3000, nil, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			type config struct {
				Port int `env:"PORT" fallback:"3000"`
			}
			calls := 0
			cfg, err := env.Load[config](env.WithSources(
				env.FromLookup(fixedLookup(tt.first, tt.found)),
				env.FromLookup(func(string) (string, bool) { calls++; return "8080", tt.secondFound }),
			))
			if !errors.Is(err, tt.wantErr) || cfg.Port != tt.want || calls != tt.secondCalls {
				t.Fatalf("Load(source matrix) = %+v, %v, %d later calls; want %d, %v, %d", cfg, err, calls, tt.want, tt.wantErr, tt.secondCalls)
			}
		})
	}
	_, err := env.Load[struct {
		Required string `env:"REQUIRED" binding:"required"`
		Optional *int   `env:"OPTIONAL"`
	}](env.WithSources(env.FromLookup(fixedLookup("ignored", false))))
	if !errors.Is(err, env.ErrRequired) {
		t.Errorf("Load(all miss required) = %v; want ErrRequired", err)
	}
}

type unexpectedDecoder string

func (*unexpectedDecoder) UnmarshalText([]byte) error {
	panic("decoder ran before all sources were prepared")
}

func TestSourcePreparationPhase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad, missing := filepath.Join(dir, "bad.env"), filepath.Join(dir, "missing.env")
	writeEnvFile(t, bad, "UNUSED='invalid")
	lookup := env.FromLookup(func(string) (string, bool) {
		t.Fatal("lookup ran before all sources were prepared")
		return "", false
	})
	type config struct {
		Value unexpectedDecoder `env:"VALUE" fallback:"decode"`
	}
	for _, tt := range []struct {
		name    string
		sources []env.Source
		path    string
		op      string
	}{
		{"shadowed invalid file", []env.Source{lookup, env.FromFiles(bad)}, bad, "parse"},
		{"shadowed missing file", []env.Source{lookup, env.FromFiles(missing)}, missing, "read"},
		{"first source fails", []env.Source{env.FromFiles(missing), env.FromFiles(bad), lookup}, missing, "read"},
		{"first parse fails", []env.Source{env.FromFiles(bad), env.FromFiles(missing), lookup}, bad, "parse"},
		{"file group order", []env.Source{lookup, env.FromFiles(missing, bad)}, missing, "read"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := env.Load[config](env.WithSources(tt.sources...))
			var sourceErr *env.SourceError
			if !errors.Is(err, env.ErrSource) || !errors.As(err, &sourceErr) || sourceErr.Path != tt.path || sourceErr.Op != tt.op || cfg != (config{}) {
				t.Fatalf("Load(preparation order) = %+v, %v; want %s %s and zero config", cfg, err, tt.op, tt.path)
			}
		})
	}
	_, err := env.Load[struct {
		Custom unexpectedDecoder `env:"VALUE"`
		Bad    int               `binding:"required"`
	}](env.WithSources(lookup, env.FromFiles(missing)))
	if !errors.Is(err, env.ErrInvalidSchema) || errors.Is(err, env.ErrSource) {
		t.Fatalf("Load(schema before sources) = %v; want ErrInvalidSchema", err)
	}
	// Even a schema with no keys must prepare every selected file.
	_, err = env.Load[struct{}](env.WithFiles(bad))
	if !errors.Is(err, env.ErrSource) {
		t.Fatalf("Load(no fields, invalid file) = %v; want ErrSource", err)
	}
}

func TestFileReloadAndInputOwnership(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.env")
	paths := []string{path}
	fromFile := env.FromFiles(paths...)
	fileOption := env.WithFiles(paths...)
	paths[0] = filepath.Join(dir, "wrong.env")
	sources := []env.Source{fromFile}
	sourcesOption := env.WithSources(sources...)
	sources[0] = env.Source{}
	type config struct {
		Value string `env:"VALUE"`
	}
	// Constructors did not read the file: it does not exist until now.
	writeEnvFile(t, path, "VALUE=first")
	snapshot, err := env.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"first", "second"} {
		writeEnvFile(t, path, "VALUE="+value)
		for _, opt := range []env.Option{fileOption, sourcesOption} {
			cfg, err := env.Load[config](opt)
			if err != nil || cfg.Value != value {
				t.Fatalf("Load(reused file option) = %+v, %v; want %s", cfg, err, value)
			}
			var parsed config
			if err := env.Parse(&parsed, opt); err != nil || parsed != cfg {
				t.Fatalf("Parse(reused file option) = %+v, %v; want %+v", parsed, err, cfg)
			}
		}
	}
	if value, found := snapshot("VALUE"); !found || value != "first" {
		t.Fatalf("reused snapshot = %q, %v; want first, true", value, found)
	}
}

func TestDefaultFileReadTime(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	writeEnvFile(t, filepath.Join(first, ".env"), "VALUE=first")
	writeEnvFile(t, filepath.Join(second, ".env"), "VALUE=second")
	t.Chdir(first)
	opt := env.WithSources(env.FromFiles())
	snapshot, err := env.Read()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(second)
	cfg, err := env.Load[struct {
		Value string `env:"VALUE"`
	}](opt)
	if err != nil || cfg.Value != "second" {
		t.Fatalf("Load(relative read time) = %+v, %v; want second", cfg, err)
	}
	if value, _ := snapshot("VALUE"); value != "first" {
		t.Fatalf("snapshot after cwd change = %q; want first", value)
	}
}

func TestSourcesCacheAndPreparedSnapshot(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.env")
	writeEnvFile(t, path, "VALUE=12\nOTHER=old")
	type config struct {
		Number  int    `env:"VALUE"`
		Text    string `env:"VALUE"`
		Default int    `env:"MISSING" fallback:"5"`
		Second  int    `env:"MISSING" fallback:"7"`
		Other   string `env:"OTHER"`
	}
	firstCalls, lastCalls := map[string]int{}, map[string]int{}
	opt := env.WithSources(
		env.FromLookup(func(key string) (string, bool) {
			firstCalls[key]++
			writeEnvFile(t, path, "VALUE=24\nOTHER=new")
			return "ignored", false
		}),
		env.FromFiles(path),
		env.FromLookup(func(key string) (string, bool) { lastCalls[key]++; return "ignored", false }),
	)
	cfg, err := env.Load[config](opt)
	want := config{Number: 12, Text: "12", Default: 5, Second: 7, Other: "old"}
	if err != nil || cfg != want || !reflect.DeepEqual(firstCalls, map[string]int{"VALUE": 1, "MISSING": 1, "OTHER": 1}) || !reflect.DeepEqual(lastCalls, map[string]int{"MISSING": 1}) {
		t.Fatalf("Load(prepared snapshot and cache) = %+v, %v; calls %v / %v", cfg, err, firstCalls, lastCalls)
	}
	cfg, err = env.Load[config](opt)
	if err != nil || cfg.Number != 24 || cfg.Other != "new" || firstCalls["VALUE"] != 2 || lastCalls["MISSING"] != 2 {
		t.Fatalf("Load(next operation) = %+v, %v; calls %v / %v", cfg, err, firstCalls, lastCalls)
	}
}

func TestSourcesProcessEnvironment(t *testing.T) {
	t.Setenv("SOURCE_VALUE", "process")
	path := filepath.Join(t.TempDir(), "config.env")
	writeEnvFile(t, path, "SOURCE_VALUE=file")
	type config struct {
		Value string `env:"SOURCE_VALUE"`
	}
	for _, tt := range []struct {
		sources []env.Source
		want    string
	}{
		{[]env.Source{env.FromLookup(os.LookupEnv), env.FromFiles(path)}, "process"},
		{[]env.Source{env.FromFiles(path), env.FromLookup(os.LookupEnv)}, "file"},
		{[]env.Source{env.FromLookup(fixedLookup("ignored", false))}, ""},
	} {
		cfg, err := env.Load[config](env.WithSources(tt.sources...))
		if err != nil || cfg.Value != tt.want {
			t.Errorf("Load(process source) = %+v, %v; want %q", cfg, err, tt.want)
		}
	}
	// os.LookupEnv remains live until each uncached key is requested.
	cfg, err := env.Load[config](env.WithSources(env.FromLookup(func(string) (string, bool) {
		t.Setenv("SOURCE_VALUE", "changed")
		return "", false
	}), env.FromLookup(os.LookupEnv)))
	if err != nil || cfg.Value != "changed" {
		t.Fatalf("Load(live process lookup) = %+v, %v; want changed", cfg, err)
	}
}

func TestSourceAtomicReplacement(t *testing.T) {
	t.Parallel()
	type child struct {
		Value int `env:"VALUE"`
	}
	type config struct {
		Child    *child
		Numbers  []int `env:"NUMBERS"`
		Optional *int  `env:"OPTIONAL"`
		Untagged string
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "file.env")
	writeEnvFile(t, path, "VALUE=5\nNUMBERS=1,bad\nOPTIONAL=7")
	number, oldChild, oldSlice := 8, &child{Value: 9}, []int{8, 9}
	old := config{Child: oldChild, Numbers: oldSlice, Optional: &number, Untagged: "old"}
	invalid := filepath.Join(dir, "invalid.env")
	writeEnvFile(t, invalid, "UNUSED='CANARY_SOURCE_98c9")
	for _, opt := range []env.Option{env.WithFiles(filepath.Join(dir, "missing")), env.WithFiles(invalid), env.WithFiles(path)} {
		cfg := old
		err := env.Parse(&cfg, opt)
		if err == nil || !reflect.DeepEqual(cfg, old) || cfg.Child != oldChild || cfg.Optional != &number || &cfg.Numbers[0] != &oldSlice[0] {
			t.Fatalf("Parse(source/value failure) = %+v, %v; want unchanged identity", cfg, err)
		}
		failed, err := env.Load[config](opt)
		if err == nil || !reflect.DeepEqual(failed, config{}) {
			t.Fatalf("Load(source/value failure) = %+v, %v; want zero", failed, err)
		}
	}
	writeEnvFile(t, path, "NUMBERS=1,2")
	cfg := old
	err := env.Parse(&cfg, env.WithFiles(path))
	if err != nil || !reflect.DeepEqual(cfg, config{Child: &child{}, Numbers: []int{1, 2}}) || cfg.Child == oldChild || &cfg.Numbers[0] == &oldSlice[0] {
		t.Fatalf("Parse(file success) = %+v, %v; want complete replacement", cfg, err)
	}
}

func TestFileGroupsAndBlankSelection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.env"), filepath.Join(dir, "second.env")
	writeEnvFile(t, second, "APP_VALUE=8080\nAPP_OTHER=file")
	type config struct {
		Value int    `env:"VALUE" fallback:"3000"`
		Other string `env:"OTHER"`
	}
	for _, tt := range []struct {
		name, text string
		want       int
		wantErr    error
	}{
		{"first file", "9000", 9000, nil},
		{"blank file", "' \t '", 0, env.ErrInvalidValue},
		{"invalid file value", "invalid", 0, env.ErrInvalidValue},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeEnvFile(t, first, "APP_VALUE="+tt.text)
			for _, opt := range []env.Option{
				env.WithFiles(first, second),
				env.WithSources(env.FromFiles(first, second)),
				env.WithSources(env.FromLookup(fixedLookup("ignored", false)), env.FromFiles(first), env.FromFiles(second)),
			} {
				cfg, err := env.Load[config](env.WithPrefix("APP_"), opt)
				if !errors.Is(err, tt.wantErr) || cfg.Value != tt.want || err == nil && cfg.Other != "file" {
					t.Fatalf("Load(file groups) = %+v, %v; want %d, %v", cfg, err, tt.want, tt.wantErr)
				}
			}
		})
	}
	writeEnvFile(t, first, "APP_VALUE=' \t '")
	_, err := env.Load[struct {
		Value int `env:"VALUE" binding:"required"`
	}](env.WithPrefix("APP_"), env.WithFiles(first, second))
	if !errors.Is(err, env.ErrInvalidValue) || errors.Is(err, env.ErrRequired) {
		t.Fatalf("Load(blank first file required) = %v; want ErrInvalidValue", err)
	}
}

func TestSchemaFailurePreservesIdentityWithFiles(t *testing.T) {
	t.Parallel()
	type child struct {
		Value int `env:"VALUE"`
	}
	type config struct {
		Child   *child
		Numbers []int `env:"NUMBERS"`
		Invalid int   `binding:"required"`
	}
	oldChild, oldNumbers := &child{Value: 8}, []int{9}
	cfg := config{Child: oldChild, Numbers: oldNumbers}
	err := env.Parse(&cfg, env.WithFiles(filepath.Join(t.TempDir(), "missing.env")))
	if !errors.Is(err, env.ErrInvalidSchema) || cfg.Child != oldChild || oldChild.Value != 8 || &cfg.Numbers[0] != &oldNumbers[0] || oldNumbers[0] != 9 {
		t.Fatalf("Parse(schema failure with files) = %+v, %v; want unchanged identity", cfg, err)
	}
}

func TestConcurrentFileOptions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "file.env")
	writeEnvFile(t, path, "VALUE=42")
	opt := env.WithSources(env.FromLookup(fixedLookup("ignored", false)), env.FromFiles(path))
	for range 16 {
		t.Run("concurrent operation", func(t *testing.T) {
			t.Parallel()
			cfg, err := env.Load[struct {
				Value int `env:"VALUE"`
			}](opt)
			if err != nil || cfg.Value != 42 {
				t.Fatalf("Load(shared file option) = %+v, %v; want 42", cfg, err)
			}
		})
	}
}
