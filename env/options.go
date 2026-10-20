package env

import (
	"reflect"
	"slices"
)

// Option configures Load or Parse. Construct reusable options with WithPrefix,
// WithLookup, WithFiles, WithOptionalFiles, WithSources, or WithValidator.
// The last setting in each group (prefix, sources, validator) replaces earlier
// settings; sources never merge and validators never accumulate. Every option
// is validated before I/O, including overwritten options. A nil option is invalid.
type Option func(*options) error

type options struct {
	prefix    string
	sources   []Source
	parent    reflect.Type
	validator func(reflect.Value) error
}

// WithPrefix prepends prefix to every key, before nested container prefixes.
// Repeated prefixes use the last value, including an empty prefix. Other option
// groups are independent. Invalid options still fail even when overwritten.
func WithPrefix(prefix string) Option {
	return func(o *options) error {
		o.prefix = prefix
		return nil
	}
}

// WithLookup replaces os.LookupEnv completely. A nil lookup is invalid.
// The last WithLookup, WithFiles, WithOptionalFiles, or WithSources selects the
// complete source group without merging. Invalid options fail even if replaced.
func WithLookup(lookup LookupFunc) Option {
	return WithSources(FromLookup(lookup))
}

// WithSources replaces the entire source selection with a copied, ordered list.
// The first source reporting found wins, including blank values. No process
// environment is added implicitly. Empty lists and invalid sources are errors,
// even if a later option replaces them. The last valid source option wins.
func WithSources(sources ...Source) Option {
	selected := slices.Clone(sources)
	return func(o *options) error {
		if len(selected) == 0 {
			return invalidOption("empty source list")
		}
		for _, source := range selected {
			if source.lookup == nil && len(source.files) == 0 {
				return invalidOption("nil lookup")
			}
			if err := validatePaths(source.files); err != nil {
				return err
			}
		}
		o.sources = selected
		return nil
	}
}

// WithFiles replaces the source selection with required files, defaulting to .env.
// It is equivalent to WithSources(FromFiles(files...)); it performs no I/O until
// Load or Parse has validated all options and the schema. The last source option
// replaces the whole group; invalid options fail even when overwritten.
func WithFiles(files ...string) Option {
	return WithSources(FromFiles(files...))
}

// WithOptionalFiles replaces the entire source selection with optional files.
// Like WithFiles, it defers I/O and defaults to .env. The last source option
// wins; invalid options remain errors even when subsequently replaced.
func WithOptionalFiles(files ...string) Option {
	return WithSources(FromOptionalFiles(files...))
}

// WithValidator installs one explicit parent validator after successful decoding
// and before commit. T must be exactly the parent struct or its pointer type;
// pointer callbacks receive the fresh candidate and successful mutations commit.
// The last validator replaces earlier callbacks, without accumulating them.
// Every callback is checked before I/O, even when overwritten; nil is invalid.
// Errors match ErrValidation and retain the application's message and chain.
func WithValidator[T any](fn func(T) error) Option {
	return func(o *options) error {
		if fn == nil {
			return invalidOption("nil validator")
		}
		t := reflect.TypeFor[T]()
		if t != o.parent && t != reflect.PointerTo(o.parent) {
			return invalidOption("validator must accept the parent struct or its pointer")
		}
		o.validator = func(candidate reflect.Value) error {
			if t.Kind() == reflect.Pointer {
				candidate = candidate.Addr()
			}
			// Option validation established exact type identity before any I/O.
			return fn(candidate.Interface().(T))
		}
		return nil
	}
}
