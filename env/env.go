package env

import (
	"os"
	"reflect"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

// Load decodes a fresh T, which must be a struct. On failure it returns zero T.
// Library decoding errors support errors.Is/As without exposing selected text;
// application validator errors retain their original messages and chains.
func Load[T any](opts ...Option) (T, error) {
	var result T
	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		return result, invalidOption("load type must be a struct")
	}
	if err := Parse(&result, opts...); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

// Parse replaces dst with a freshly decoded struct on total success. dst must
// be a non-nil pointer to a struct; failure leaves its previous contents intact.
// Callers must synchronize any concurrent access to their destination.
func Parse(dst any, opts ...Option) error {
	target := reflect.ValueOf(dst)
	if target.Kind() != reflect.Pointer || target.IsNil() || target.Elem().Kind() != reflect.Struct {
		return invalidOption("destination must be a non-nil pointer to a struct")
	}
	o := options{sources: []Source{FromLookup(os.LookupEnv)}, parent: target.Elem().Type()}
	for _, opt := range opts {
		if opt == nil {
			return invalidOption("nil option")
		}
		if err := opt(&o); err != nil {
			return err
		}
	}
	plan, diagnostics := decoder.Compile(target.Elem().Type(), o.prefix)
	if len(diagnostics) > 0 {
		return diagnosticErrors(diagnostics)
	}
	lookup, err := prepareSources(o.sources)
	if err != nil {
		return err
	}
	candidate, diagnostics := plan.Decode(lookup)
	if len(diagnostics) > 0 {
		return diagnosticErrors(diagnostics)
	}
	if o.validator != nil {
		if err := o.validator(candidate); err != nil {
			return &validationError{cause: err}
		}
	}
	target.Elem().Set(candidate)
	return nil
}
