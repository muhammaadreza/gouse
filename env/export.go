package env

import (
	"maps"
	"os"
	"slices"
)

// Export reads and validates all files, defaulting to .env, then installs only
// keys absent from the process environment. Existing empty values also win.
// Keys are installed in sorted order. The first setenv failure stops the
// operation without rolling back keys already installed. Read/parse failures
// cause no mutation. Use at startup: lookup and set are not atomic together
// against other environment writers. Export cannot change the parent shell.
func Export(files ...string) error {
	return exportFiles(files, requiredFiles, os.ReadFile, os.LookupEnv, os.Setenv)
}

// ExportOptional is Export with each not-found file skipped. Other failures
// remain errors; all files missing succeeds without mutation. Partial setenv
// success is retained on failure, just as with Export.
func ExportOptional(files ...string) error {
	return exportFiles(files, optionalFiles, os.ReadFile, os.LookupEnv, os.Setenv)
}

func exportFiles(files []string, policy filePolicy, readFile func(string) ([]byte, error), lookup LookupFunc, setenv func(string, string) error) error {
	values, err := readFiles(filePaths(files), policy, readFile)
	if err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		if _, found := lookup(key); found {
			continue
		}
		value := values[key]
		if err := setenv(key, value.text); err != nil {
			return &SourceError{Path: value.path, Op: "setenv", category: ErrSourceSetenv}
		}
	}
	return nil
}
