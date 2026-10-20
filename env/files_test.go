package env_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/muhammaadreza/gouse/env"
)

func writeEnvFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestReadSnapshotAndPriority(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.env"), filepath.Join(dir, "second.env")
	writeEnvFile(t, first, "A=first\nEMPTY=\nSPACE='  x  '\nLITERAL=${A}")
	writeEnvFile(t, second, "A=second\nEMPTY=second\nB=second")
	lookup, err := env.Read(first, second)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "first", "EMPTY": "", "SPACE": "  x  ", "LITERAL": "${A}", "B": "second"}
	writeEnvFile(t, first, "A=changed")
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	for key, value := range want {
		if got, found := lookup(key); !found || got != value {
			t.Errorf("snapshot(%s) = %q, %v; want %q, true", key, got, found, value)
		}
	}
	if got, found := lookup("MISSING"); got != "" || found {
		t.Errorf("snapshot(missing) = %q, %v; want empty, false", got, found)
	}
	for i := range 32 {
		t.Run(fmt.Sprintf("concurrent %d", i), func(t *testing.T) {
			t.Parallel()
			for range 100 {
				for key, value := range want {
					if got, found := lookup(key); !found || got != value {
						t.Fatalf("concurrent snapshot(%s) = %q, %v", key, got, found)
					}
				}
			}
		})
	}
}

func TestReadDefaultAndProcessIsolation(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DOTENV_ONLY_PROCESS", "process")
	t.Setenv("DOTENV_EXISTING", "old")
	before := os.Environ()
	writeEnvFile(t, ".env", "DOTENV_EXISTING=new\nDOTENV_FILE_ONLY=file")
	lookup, err := env.Read()
	if err != nil {
		t.Fatal(err)
	}
	if value, found := lookup("DOTENV_EXISTING"); value != "new" || !found {
		t.Errorf("Read default = %q, %v; want new, true", value, found)
	}
	if value, found := lookup("DOTENV_ONLY_PROCESS"); value != "" || found {
		t.Errorf("Read process miss = %q, %v; want empty, false", value, found)
	}
	type config struct {
		Value string `env:"DOTENV_EXISTING"`
	}
	cfg, err := env.Load[config](env.WithFiles())
	if err != nil || cfg.Value != "new" || !reflect.DeepEqual(os.Environ(), before) {
		t.Fatalf("Load(default file) = %+v, %v; process environment must remain unchanged", cfg, err)
	}
	if err := os.Remove(".env"); err != nil {
		t.Fatal(err)
	}
	missing, err := env.Read()
	if missing != nil || !errors.Is(err, env.ErrSource) {
		t.Errorf("Read(missing default) = %v; want nil lookup and ErrSource", err)
	}
}

func TestReadFailuresAndPrivacy(t *testing.T) {
	t.Parallel()
	const secret = "CANARY_SOURCE_98c9"
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.env"), filepath.Join(dir, "bad.env")
	writeEnvFile(t, good, "KEY=usable")
	for _, contents := range []string{
		"KEY=" + secret + "\nKEY=" + secret,
		"# first\nKEY='" + secret,
		"# first\n" + secret,
		"# first\nKEY=\xff" + secret,
		"# first\nKEY=\"\\q" + secret + "\"",
	} {
		writeEnvFile(t, bad, contents)
		lookup, err := env.Read(good, bad)
		var sourceErr *env.SourceError
		if lookup != nil || !errors.Is(err, env.ErrSource) || !errors.As(err, &sourceErr) || sourceErr.Path != bad || sourceErr.Op != "parse" || sourceErr.Line != 2 {
			t.Fatalf("Read(parse failure) = %v; want safe parse error at line 2", err)
		}
		assertPrivate(t, err, secret)
		if sourceErr.Unwrap() != env.ErrSource || errors.Unwrap(err) != env.ErrSource {
			t.Fatal("source error unwrap exposes more than the sentinel")
		}
	}
	for _, path := range []string{filepath.Join(dir, "missing.env"), dir} {
		lookup, err := env.Read(good, path)
		var sourceErr *env.SourceError
		var pathErr *os.PathError
		if lookup != nil || !errors.As(err, &sourceErr) || sourceErr.Path != path || sourceErr.Op != "read" || sourceErr.Line != 0 || errors.As(err, &pathErr) {
			t.Fatalf("Read(read failure) = %v; want sanitized read error", err)
		}
	}
	lookup, err := env.Read(filepath.Join(dir, "missing.env"), "")
	if lookup != nil || !errors.Is(err, env.ErrInvalidOption) {
		t.Errorf("Read(empty path) = %v; want nil and ErrInvalidOption before I/O", err)
	}
}

func TestOptionalFileGroups(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing, first, second := filepath.Join(dir, "missing"), filepath.Join(dir, "first"), filepath.Join(dir, "second")
	writeEnvFile(t, first, "V=\nRAW=' x '\nA=first")
	writeEnvFile(t, second, "V=second\nA=second\nB=second")
	lookup, err := env.ReadOptional(missing, first, missing, second, missing)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"V": "", "RAW": " x ", "A": "first", "B": "second"} {
		if got, found := lookup(key); !found || got != want {
			t.Errorf("ReadOptional(%s) = %q, %v, want %q, true", key, got, found, want)
		}
	}
	empty, err := env.ReadOptional(missing, missing)
	if err != nil || empty == nil {
		t.Fatalf("ReadOptional(all missing) = %v, want valid empty lookup", err)
	}
	if got, found := empty("PATH"); got != "" || found {
		t.Errorf("empty lookup = %q, %v", got, found)
	}
	writeEnvFile(t, first, "V=changed")
	if v, _ := lookup("A"); v != "first" {
		t.Error("optional snapshot changed")
	}
	type config struct {
		Value string `env:"V" fallback:"fallback"`
	}
	for _, opt := range []env.Option{
		env.WithOptionalFiles(missing, first, second),
		env.WithSources(env.FromOptionalFiles(missing), env.FromOptionalFiles(first, second)),
	} {
		cfg, err := env.Load[config](opt)
		if err != nil || cfg.Value != "changed" {
			t.Fatalf("Load(optional sources) = %+v, %v", cfg, err)
		}
	}
	cfg, err := env.Load[config](env.WithSources(env.FromOptionalFiles(missing), env.FromLookup(fixedLookup("next", true))))
	if err != nil || cfg.Value != "next" {
		t.Fatalf("Load(optional miss continues) = %+v, %v", cfg, err)
	}
	_, err = env.Load[struct {
		Value string `env:"V" binding:"required"`
	}](env.WithOptionalFiles(missing))
	if !errors.Is(err, env.ErrRequired) {
		t.Errorf("Load(optional missing required) = %v", err)
	}
	for _, path := range []string{dir, second} {
		if path == second {
			writeEnvFile(t, path, "INVALID='secret")
		}
		lookup, err := env.ReadOptional(missing, path)
		if lookup != nil || !errors.Is(err, env.ErrSource) {
			t.Errorf("ReadOptional(non-missing failure) = %v", err)
		}
	}
	for _, opt := range []env.Option{env.WithOptionalFiles(""), env.WithSources(env.FromOptionalFiles(missing, ""))} {
		_, err := env.Load[config](opt, env.WithLookup(empty))
		if !errors.Is(err, env.ErrInvalidOption) {
			t.Errorf("Load(overwritten invalid optional) = %v", err)
		}
	}
	lookup, err = env.ReadOptional(missing, "")
	if lookup != nil || !errors.Is(err, env.ErrInvalidOption) {
		t.Errorf("ReadOptional(empty path before I/O) = %v", err)
	}
}

func TestOptionalDefaultsAndOwnership(t *testing.T) {
	t.Chdir(t.TempDir())
	lookup, err := env.ReadOptional()
	if err != nil || lookup == nil {
		t.Fatalf("ReadOptional(default missing) = %v", err)
	}
	if err := env.ExportOptional(); err != nil {
		t.Fatalf("ExportOptional(default missing) = %v", err)
	}
	paths := []string{".env"}
	source := env.FromOptionalFiles(paths...)
	opt := env.WithOptionalFiles(paths...)
	paths[0] = "wrong"
	type config struct {
		Value string `env:"V"`
	}
	for _, text := range []string{"first", "second"} {
		writeEnvFile(t, ".env", "V="+text)
		for _, selected := range []env.Option{opt, env.WithSources(source)} {
			cfg, err := env.Load[config](selected)
			if err != nil || cfg.Value != text {
				t.Fatalf("Load(optional ownership/reload) = %+v, %v", cfg, err)
			}
		}
	}
	// Source selection replaces the complete group; optional files do not merge.
	cfg, err := env.Load[config](env.WithFiles("missing"), env.WithOptionalFiles())
	if err != nil || cfg.Value != "second" {
		t.Fatalf("Load(last optional source) = %+v, %v", cfg, err)
	}
	writeEnvFile(t, ".env", "V=")
	cfg, err = env.Load[config](env.WithSources(env.FromOptionalFiles(), env.FromLookup(fixedLookup("later", true))))
	if err != nil || cfg.Value != "" {
		t.Fatalf("Load(empty optional stops search) = %+v, %v", cfg, err)
	}
}
