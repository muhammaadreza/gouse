package env

import (
	"errors"
	"io/fs"
	"os"
	"slices"

	"github.com/muhammaadreza/gouse/env/internal/dotenv"
)

type fileValue struct {
	text string
	path string
}

type filePolicy uint8

const (
	requiredFiles filePolicy = iota + 1
	optionalFiles
)

// Read reads and validates all files immediately, defaulting to .env.
// Relative paths use the current working directory. Empty paths are invalid.
// The first file containing a key wins, including empty values. Success returns
// an immutable snapshot safe for concurrent reads; failure returns nil.
// No process environment is read or changed.
func Read(files ...string) (LookupFunc, error) {
	return readLookup(files, requiredFiles)
}

// ReadOptional is Read with each not-found file skipped. All other read and
// syntax failures remain errors. With no paths it reads .env optionally.
// If every file is missing, it returns a non-nil lookup with no keys.
func ReadOptional(files ...string) (LookupFunc, error) {
	return readLookup(files, optionalFiles)
}

func readLookup(files []string, policy filePolicy) (LookupFunc, error) {
	values, err := readFiles(filePaths(files), policy, os.ReadFile)
	if err != nil {
		return nil, err
	}
	return snapshotLookup(values), nil
}

func snapshotLookup(values map[string]fileValue) LookupFunc {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value.text, found
	}
}

func filePaths(files []string) []string {
	if len(files) == 0 {
		return []string{".env"}
	}
	return slices.Clone(files)
}

func validatePaths(paths []string) error {
	for _, path := range paths {
		if path == "" {
			return invalidOption("empty file path")
		}
	}
	return nil
}

// readFiles is shared by readers, deferred sources, and Export. Dependencies
// are passed per call so failure tests need no mutable global filesystem seam.
func readFiles(paths []string, policy filePolicy, readFile func(string) ([]byte, error)) (map[string]fileValue, error) {
	if err := validatePaths(paths); err != nil {
		return nil, err
	}
	values := make(map[string]fileValue)
	for _, path := range paths {
		contents, err := readFile(path)
		if err != nil {
			category := ErrSourceRead
			switch {
			case errors.Is(err, fs.ErrNotExist):
				if policy == optionalFiles {
					continue
				}
				category = ErrSourceNotFound
			case errors.Is(err, fs.ErrPermission):
				category = ErrSourcePermission
			}
			return nil, &SourceError{Path: path, Op: "read", category: category}
		}
		parsed, err := dotenv.Parse(contents)
		if err != nil {
			line, reason := 0, "invalid assignment"
			if syntax, ok := err.(*dotenv.Error); ok {
				line, reason = syntax.Line, syntax.Kind
			}
			return nil, &SourceError{Path: path, Op: "parse", Line: line, category: ErrSourceSyntax, reason: reason}
		}
		for key, value := range parsed {
			if _, found := values[key]; !found {
				values[key] = fileValue{text: value, path: path}
			}
		}
	}
	return values, nil
}
