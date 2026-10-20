package env

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func TestReadFailureClassification(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		cause    error
		category error
		reason   string
	}{
		{"missing", fs.ErrNotExist, ErrSourceNotFound, "file not found"},
		{"permission", fs.ErrPermission, ErrSourcePermission, "permission denied"},
		{"other", errors.New("CANARY_READ_SECRET"), ErrSourceRead, "read failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			read := func(string) ([]byte, error) { return nil, fmt.Errorf("CANARY_RAW_CONTEXT: %w", tt.cause) }
			for _, policy := range []filePolicy{requiredFiles, optionalFiles} {
				values, err := readFiles([]string{"safe.env"}, policy, read)
				if policy == optionalFiles && tt.category == ErrSourceNotFound {
					if err != nil || len(values) != 0 {
						t.Fatalf("optional missing = %v, %v", values, err)
					}
					continue
				}
				var source *SourceError
				if values != nil || !errors.Is(err, ErrSource) || !errors.Is(err, ErrSourceRead) || !errors.Is(err, tt.category) || !errors.As(err, &source) {
					t.Fatalf("readFiles(%s) = %v, want source/read/specific", tt.name, err)
				}
				if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || strings.Contains(fmt.Sprintf("%+v %#v", err, err), "CANARY_") {
					t.Errorf("read error leaked raw cause: %v", err)
				}
				want := `source failure: read "safe.env": ` + tt.reason
				if err.Error() != want || source.Line != 0 || source.Path != "safe.env" || source.Op != "read" {
					t.Errorf("readFiles(%s) = %v, want %s", tt.name, err, want)
				}
			}
		})
	}
	calls := 0
	_, err := readFiles([]string{"missing", ""}, optionalFiles, func(string) ([]byte, error) { calls++; return nil, fs.ErrNotExist })
	if !errors.Is(err, ErrInvalidOption) || calls != 0 {
		t.Errorf("readFiles(invalid paths) = %v, calls %d, want option/0", err, calls)
	}
}

func TestDiagnosticFallbackAndSourceDefaults(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		op, reason string
		category   error
	}{
		{"read", "read failed", ErrSourceRead},
		{"parse", "invalid assignment", ErrSourceSyntax},
		{"setenv", "could not set environment variable", ErrSourceSetenv},
	} {
		err := &SourceError{Path: "safe", Op: tt.op}
		if !errors.Is(err, tt.category) || !errors.Is(err, ErrSource) || !strings.HasSuffix(err.Error(), tt.reason) {
			t.Errorf("SourceError(%s) = %v", tt.op, err)
		}
	}
}
