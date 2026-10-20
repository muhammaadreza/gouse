package env

import "os"

// LookupFunc returns the value of key and whether it exists.
// Implementations used concurrently must provide their own synchronization.
type LookupFunc func(key string) (value string, found bool)

// Source describes a lookup or file group, prepared after schema
// validation. Construct one with FromLookup, FromFiles, or FromOptionalFiles;
// its zero value is invalid.
type Source struct {
	lookup LookupFunc
	files  []string
	policy filePolicy
}

// FromLookup describes a custom lookup. A nil lookup is invalid when used.
// Its values are read on demand, with one cached result per key per operation.
func FromLookup(lookup LookupFunc) Source {
	return Source{lookup: lookup}
}

// FromFiles describes required files, defaulting to .env, without reading them.
// Paths are copied; an empty path is invalid when used. Files are read anew
// for every Load or Parse after option and schema validation.
func FromFiles(files ...string) Source {
	return Source{files: filePaths(files), policy: requiredFiles}
}

// FromOptionalFiles describes optional files without I/O. It copies paths and
// defaults to .env. Only not-found files are skipped; every other failure remains
// an error. Each Load or Parse reads the files anew after schema validation.
func FromOptionalFiles(files ...string) Source {
	return Source{files: filePaths(files), policy: optionalFiles}
}

func prepareSources(sources []Source) (LookupFunc, error) {
	lookups := make([]LookupFunc, 0, len(sources))
	for _, source := range sources {
		lookup := source.lookup
		if len(source.files) > 0 {
			values, err := readFiles(source.files, source.policy, os.ReadFile)
			if err != nil {
				return nil, err
			}
			lookup = snapshotLookup(values)
		}
		lookups = append(lookups, lookup)
	}
	return func(key string) (string, bool) {
		for _, lookup := range lookups {
			if value, found := lookup(key); found {
				return value, true
			}
		}
		return "", false
	}, nil
}
