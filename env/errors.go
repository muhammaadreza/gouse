package env

import (
	"errors"
	"fmt"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

var (
	// ErrRequired indicates a required key is absent.
	ErrRequired = errors.New("required key missing")
	// ErrEmptyValue indicates selected text violates nonempty. Its FieldError
	// also matches ErrInvalidValue.
	ErrEmptyValue = errors.New("empty value")
	// ErrValidation indicates a parent validator returned an application error.
	ErrValidation = errors.New("validation failed")
	// ErrInvalidValue indicates selected text failed decoding.
	ErrInvalidValue = errors.New("invalid value")
	// ErrUnsupportedType indicates an unsupported annotated field type.
	// Its FieldError also matches ErrInvalidSchema through errors.Is.
	ErrUnsupportedType = errors.New("unsupported type")
	// ErrInvalidSchema indicates malformed tags or invalid field declarations.
	ErrInvalidSchema = errors.New("invalid schema")
	// ErrInvalidOption indicates an invalid option, destination, or Load type.
	ErrInvalidOption = errors.New("invalid option or input")
)

// FieldError exposes schema context for a parsing failure. Field errors retain
// neither selected text nor original parser/custom errors.
// Use errors.As with *FieldError to inspect an aggregate's first matching field.
type FieldError struct {
	Path     string // Dot-separated Go field path.
	Key      string // Complete ENV key when applicable; empty when unavailable.
	Index    int    // Zero-based slice element index; -1 for a whole field.
	Category error  // One of this package's sentinel categories.
}

func fieldError(path, key string, index int, category error) error {
	return &FieldError{
		Path:     path,
		Key:      key,
		Index:    index,
		Category: category,
	}
}

// Error returns safe schema context and the sentinel category.
func (e *FieldError) Error() string {
	context := fmt.Sprintf("field %q", e.Path)
	if e.Key != "" {
		context += fmt.Sprintf(" key %q", e.Key)
	}
	if e.Index >= 0 {
		context += fmt.Sprintf("[%d]", e.Index)
	}

	return fmt.Sprintf("%s: %v", context, e.Category)
}

// Unwrap exposes the sentinel category, never an original decoding error.
func (e *FieldError) Unwrap() error {
	return e.Category
}

// Is also classifies unsupported types as invalid schemas and empty-value
// failures as invalid values.
func (e *FieldError) Is(target error) bool {
	return (target == ErrInvalidSchema && e.Category == ErrUnsupportedType) ||
		(target == ErrInvalidValue && e.Category == ErrEmptyValue)
}

var (
	// ErrSource indicates a configuration source failure.
	ErrSource = errors.New("source failure")
	// ErrSourceRead includes all file read failures, including missing/permission.
	ErrSourceRead = errors.New("read failed")
	// ErrSourceNotFound indicates a file was not found. Its SourceError also
	// matches ErrSourceRead.
	ErrSourceNotFound = errors.New("file not found")
	// ErrSourcePermission indicates access was denied. Its SourceError also
	// matches ErrSourceRead.
	ErrSourcePermission = errors.New("permission denied")
	// ErrSourceSyntax indicates invalid dotenv syntax.
	ErrSourceSyntax = errors.New("invalid dotenv syntax")
	// ErrSourceSetenv indicates installing a process environment value failed.
	ErrSourceSetenv = errors.New("could not set environment variable")
)

// SourceError exposes safe file context without retaining contents or raw errors.
// Use errors.Is for ErrSource and its specific categories, and errors.As for
// *SourceError. Unwrap exposes only ErrSource; Is provides specific categories.
type SourceError struct {
	Path     string // File path as supplied by the caller, or .env for the default.
	Op       string // Operation: read, parse, or setenv.
	Line     int    // One-based physical parse line; zero when unavailable.
	category error
	reason   string
}

// Error returns source metadata and a static failure reason.
func (e *SourceError) Error() string {
	reason := e.reason
	if reason == "" {
		switch e.category {
		case ErrSourceNotFound, ErrSourcePermission:
			reason = e.category.Error()
		default:
			switch e.Op {
			case "parse":
				reason = "invalid assignment"
			case "setenv":
				reason = ErrSourceSetenv.Error()
			default:
				reason = ErrSourceRead.Error()
			}
		}
	}
	context := fmt.Sprintf("%s: %s %q", ErrSource, e.Op, e.Path)
	if e.Line > 0 {
		context += fmt.Sprintf(" line %d", e.Line)
	}
	return context + ": " + reason
}

// Unwrap returns only ErrSource, never a filesystem or parser error.
func (e *SourceError) Unwrap() error { return ErrSource }

// Is classifies source failures without exposing raw errors.
func (e *SourceError) Is(target error) bool {
	if target == e.category {
		return true
	}
	if target == ErrSourceRead {
		return e.Op == "read"
	}
	return (target == ErrSourceSyntax && e.Op == "parse") ||
		(target == ErrSourceSetenv && e.Op == "setenv")
}

func invalidOption(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidOption, detail)
}

type validationError struct{ cause error }

func (e *validationError) Error() string        { return e.cause.Error() }
func (e *validationError) Unwrap() error        { return e.cause }
func (e *validationError) Is(target error) bool { return target == ErrValidation }

// diagnosticErrors translates decoder metadata without retaining internal data
// in the public error chain. Unknown categories fail closed as invalid schema.
func diagnosticErrors(diagnostics []decoder.Diagnostic) error {
	errs := make([]error, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		category := ErrInvalidSchema
		switch diagnostic.Category {
		case decoder.Required:
			category = ErrRequired
		case decoder.EmptyValue:
			category = ErrEmptyValue
		case decoder.InvalidValue:
			category = ErrInvalidValue
		case decoder.UnsupportedType:
			category = ErrUnsupportedType
		}
		errs = append(errs, fieldError(diagnostic.Path, diagnostic.Key, diagnostic.Index, category))
	}
	return errors.Join(errs...)
}
