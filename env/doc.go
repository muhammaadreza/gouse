// Package env decodes configuration sources into explicitly tagged struct fields.
// Load constructs a root struct; Parse replaces a non-nil pointer to a struct.
// Both validate all options and the schema before I/O, decode a fresh candidate,
// run an optional parent validator, and commit only on total success. Failure
// returns zero from Load and leaves the previous Parse destination untouched.
//
// # Binding and selection
//
// An env tag names a leaf key. Without binding, fields are optional. The binding
// tag accepts comma-separated optional, required, nonempty, and notrim flags;
// whitespace around flags is ignored. Empty/unknown/duplicate flags, required
// with optional or fallback, and binding on nested containers are schema errors.
// Required checks key existence, including an empty value. Fallback is selected
// only when the key is absent; an explicitly empty fallback still selects text.
// Nonempty rejects empty selected text after trimming, but permits optional
// absence without fallback. Default strings.TrimSpace applies to whole values
// and every built-in slice token. Notrim skips both trim stages, including for
// fallback. []int with "1,2" works with either policy; "1, 2" works by default
// but fails with notrim. Whitespace retained on numeric/bool/duration tokens may
// prevent conversion. Use lookup or quoted dotenv values to preserve outer space.
//
// Optional absence leaves leaf zero values, nil pointers, and nil slices without
// invoking decoders. Selected empty text allocates a string pointer or a non-nil
// empty built-in slice, but fails numeric/bool/duration conversion. Custom
// UnmarshalText receives empty text unless nonempty rejects it first. "a,,b"
// has three slice tokens; nonempty checks the whole text rather than each token.
//
// # Types and nesting
//
// Built-in leaves include strings, bools, decimal signed/unsigned integers,
// finite floats, exact time.Duration, named scalars, single leaf pointers, and
// one-dimensional leaf slices. []byte is numeric uint8 tokens, not raw text.
// A defined duration-like int64 is an integer; an alias retains duration parsing.
// Concrete types implementing encoding.TextUnmarshaler take priority, including
// named map/array, struct, slice, complex, uintptr, function, and channel types.
// The method has signature UnmarshalText([]byte) error on T or *T; functions
// named ParseX, UnmarshalJSON, and MarshalText are not automatically invoked.
// Custom collection methods own their grammar and nil/empty policy; the library
// passes whole text without built-in splitting. time.Time (RFC3339), netip.Addr,
// and other types with existing UnmarshalText work without a user wrapper.
//
// Maps/arrays and ordinary structs without UnmarshalText have no built-in leaf
// parser. Interface leaves, multiple pointers, pointer map/array fields, built-in
// pointer slices, pointer slice elements, and nested built-in collections remain
// unsupported. Existing single pointers to custom struct/scalar/named-slice
// leaves and slices of custom leaves remain supported. A pointer method receiver
// does not require a pointer field. See README for the full type map and examples.
//
// Named struct and pointer-to-struct containers without env are visited
// recursively; prefix tags compose after WithPrefix. Pointer containers allocate
// on success even if all children are absent. env:"-" skips a whole subtree.
// Active anonymous fields and tagged unexported fields are invalid.
//
// # Sources, options, and Export
//
// The default source is os.LookupEnv. WithLookup, WithFiles, WithOptionalFiles,
// and WithSources replace the whole source group; the last source option wins
// without merging. WithPrefix and WithValidator independently use their last
// setting; validators are not accumulated. Every supplied option is validated,
// even when overwritten. Nil options/lookups/validators, wrong parent callback
// types, empty source lists, and empty paths remain errors before I/O.
// Within the selected group, the first source/file finding a key wins, including
// empty values, without trying lower-priority values or fallback on conversion
// failure. WithValidator accepts exactly func(Config) error or func(*Config) error.
// It runs after successful decoding and before commit. Pointer callbacks receive
// a fresh candidate; their successful mutations commit. Applications explicitly
// compose child rules, choosing return-first or errors.Join themselves.
//
// Read and ReadOptional return immediate immutable file snapshots. FromFiles,
// FromOptionalFiles, WithFiles, and WithOptionalFiles defer I/O and read files
// anew for each Load/Parse after validation. Paths are copied and default to .env;
// relative paths use the working directory at read time. Every selected file
// must parse, even if shadowed or unused. Optional skips only not-found files;
// all missing returns a valid empty source and does not disable required fields.
// Read/Load/Parse never mutate process environment. Only Export/ExportOptional
// install keys absent from os.LookupEnv; existing empty values also win. They
// validate every file first and then install sorted keys. The first setenv
// failure stops without rollback of successful writes. Use during startup:
// lookup-plus-set is not atomic against other writers, and cannot change the
// parent shell. All optional files missing succeeds without mutation.
//
// Dotenv supports UTF-8, LF/CRLF, an initial BOM, export assignments, comments,
// single/double quotes, and limited double-quote escapes. Duplicate keys within
// a file, invalid syntax, physical multiline quotes, and control characters
// fail. Interpolation and shell execution are not performed.
//
// # Errors and concurrency
//
// Field errors aggregate in declaration order (slice indexes ascending) and
// support errors.Is/As with FieldError. An empty-value FieldError matches both
// ErrEmptyValue and ErrInvalidValue; an unsupported-type FieldError matches
// ErrUnsupportedType and ErrInvalidSchema. SourceError has Path, Op
// (read/parse/setenv), and Line. All source failures match ErrSource; read failures
// also match ErrSourceRead and, where applicable, ErrSourceNotFound or
// ErrSourcePermission. Parse and setenv match ErrSourceSyntax/ErrSourceSetenv.
// Library errors have no "env:" prefix and retain no input/fallback text or raw
// filesystem/parser/custom decoder errors. Invalid API inputs use ErrInvalidOption
// without empty field context. SourceError unwraps only ErrSource.
// Parent validator errors are the explicit exception: ErrValidation wraps them
// while preserving their message and original chain, including errors.Join.
// Applications are responsible for validator message privacy. No logging occurs.
//
// Independent operations share no mutable parsing state. Caller lookup/custom
// decoder/validator code needs its own concurrency safety; callers synchronize
// destination access. User callback side effects and panics are outside the
// library's control. There is no global schema cache, watcher, generator,
// public introspection/provenance API, secret-file/_FILE convention, or registry.
package env
