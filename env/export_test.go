package env

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
)

func writeEnvFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func preserveAbsent(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "") // Registers restoration of both existing and absent state.
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func TestExportProcessEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	const a = "GOUSE_EXPORT_TEST_A"
	const b = "GOUSE_EXPORT_TEST_B"
	const c = "GOUSE_EXPORT_TEST_C"
	const d = "GOUSE_EXPORT_TEST_D"
	preserveAbsent(t, d)
	preserveAbsent(t, a)
	t.Setenv(b, "")
	t.Setenv(c, "existing")
	writeEnvFile(t, ".env", a+"=' first \\n '\n"+b+"=file\n"+c+"=file\n"+d+"=\" line\\nnext \" ")
	// Single quotes preserve literal escape characters; no decoder trim applies.
	if err := Export(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(a); got != " first \\n " {
		t.Errorf("Export(quoted) = %q", got)
	}
	if got := os.Getenv(b); got != "" {
		t.Errorf("Export(existing empty) = %q", got)
	}
	if got := os.Getenv(c); got != "existing" {
		t.Errorf("Export(existing) = %q", got)
	}
	if got := os.Getenv(d); got != " line\nnext " {
		t.Errorf("Export(double-quoted escape) = %q, want decoded newline and preserved spaces", got)
	}
	preserveAbsent(t, a)
	writeEnvFile(t, "first", a+"=first")
	writeEnvFile(t, "second", a+"=second")
	if err := ExportOptional("missing", "first", "second"); err != nil || os.Getenv(a) != "first" {
		t.Fatalf("ExportOptional(priority) = %q, %v", os.Getenv(a), err)
	}
	preserveAbsent(t, a)
	for _, last := range []string{"missing", "bad"} {
		writeEnvFile(t, "bad", "UNUSED='secret")
		err := Export("first", last)
		if _, found := os.LookupEnv(a); err == nil || found {
			t.Fatalf("Export(read/parse failure) = %v, mutated=%v", err, found)
		}
	}
	if err := ExportOptional("missing"); err != nil {
		t.Errorf("ExportOptional(all missing) = %v", err)
	}
}

func TestExportSortedPartialFailure(t *testing.T) {
	t.Parallel()
	read := func(path string) ([]byte, error) {
		if path == "first.env" {
			return []byte("B=CANARY_FIRST\nD=CANARY_D\nC=CANARY_C"), nil
		}
		return []byte("A=CANARY_A\nB=CANARY_SECOND\nE=CANARY_E"), nil
	}
	installed := map[string]string{}
	var checked, set []string
	lookup := func(key string) (string, bool) { checked = append(checked, key); return "", key == "C" }
	setter := func(key, value string) error {
		set = append(set, key)
		if key == "D" {
			return errors.New("CANARY_SETTER_SECRET " + value)
		}
		installed[key] = value
		return nil
	}
	err := exportFiles([]string{"first.env", "second.env"}, requiredFiles, read, lookup, setter)
	var source *SourceError
	if !errors.Is(err, ErrSource) || !errors.Is(err, ErrSourceSetenv) || errors.Is(err, ErrSourceRead) || !errors.As(err, &source) {
		t.Fatalf("exportFiles(failed setter) = %v", err)
	}
	if !reflect.DeepEqual(checked, []string{"A", "B", "C", "D"}) || !reflect.DeepEqual(set, []string{"A", "B", "D"}) || !reflect.DeepEqual(installed, map[string]string{"A": "CANARY_A", "B": "CANARY_FIRST"}) {
		t.Errorf("exportFiles(partial) checked=%v set=%v installed=%v", checked, set, installed)
	}
	if source.Path != "first.env" || source.Op != "setenv" || source.Line != 0 || err.Error() != `source failure: setenv "first.env": could not set environment variable` || strings.Contains(fmt.Sprintf("%+v %#v", err, err), "CANARY_") {
		t.Errorf("exportFiles(error) = %+v", err)
	}
}

func TestExportPreflightBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		contents []byte
		cause    error
	}{
		{name: "missing", cause: fs.ErrNotExist},
		{name: "permission", cause: fs.ErrPermission},
		{name: "parse", contents: []byte("BAD='CANARY")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			read := func(path string) ([]byte, error) {
				if path == "first" {
					return []byte("A=CANARY"), nil
				}
				return tt.contents, tt.cause
			}
			failLookup := func(string) (string, bool) { t.Fatal("lookup before all files valid"); return "", false }
			failSet := func(string, string) error { t.Fatal("set before all files valid"); return nil }
			if err := exportFiles([]string{"first", "second"}, requiredFiles, read, failLookup, failSet); !errors.Is(err, ErrSource) {
				t.Errorf("exportFiles(preflight) = %v", err)
			}
		})
	}
}
