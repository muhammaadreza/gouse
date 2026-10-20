# gouse/env

Part of [gouse](../README.md).

Decode environment or dotenv values into explicitly tagged Go structs using only
the standard library. Options and schemas are checked before I/O. `Load` and
`Parse` decode a fresh candidate and commit only after decoding and validation
succeed. Only the explicit Export API changes process environment.

[Quick start](#quick-start) · [API](#public-api) ·
[Tags](#tags-absent-values-and-empty-text) · [Types](#type-support-map) ·
[Validation](#parent-validation) · [Errors](#error-categories-and-privacy)

## Installation

Requires Go **1.27.1** or later. The package import is
`github.com/muhammaadreza/gouse/env`; its module is
`github.com/muhammaadreza/gouse`.

For a local checkout before publication, add these lines to your application's
`go.mod`. Replace `../gouse` with the path to the repository root:

```go
require github.com/muhammaadreza/gouse v0.0.0

replace github.com/muhammaadreza/gouse => ../gouse
```

## Quick start

```go
package main

import (
    "fmt"
    "time"

    "github.com/muhammaadreza/gouse/env"
)

type Database struct {
    Host string `env:"HOST" binding:"required,nonempty"`
    Port int    `env:"PORT" fallback:"5432"`
}
type Config struct {
    Name    string        `env:"NAME" fallback:"service"`
    Debug   bool          `env:"DEBUG"`
    Timeout time.Duration `env:"TIMEOUT" fallback:"30s"`
    Peers   []string      `env:"PEERS" separator:";"`
    DB      *Database     `prefix:"DB_"`
}
func main() {
    cfg, err := env.Load[Config](env.WithPrefix("APP_"))
    if err != nil {
        fmt.Println(err)
        return
    }
    fmt.Println(cfg.Name, cfg.Timeout, cfg.DB.Host)
}
```

With only `APP_DB_HOST=localhost` set, this prints `service 30s localhost`.
`Load[T]` requires a struct T and returns zero T on failure. `Parse(&cfg, ...)`
requires a non-nil pointer to a struct, preserving its old contents on failure.
Success replaces the entire destination, clearing old optional/untagged/skipped
values. Nested pointer containers allocate even when their children are absent.
This candidate/commit guarantee does not synchronize concurrent destination access.

## Public API

```go
type LookupFunc func(key string) (value string, found bool)
type Option func(/* private options */) error
type Source struct { /* private fields */ }

func Load[T any](opts ...Option) (T, error)
func Parse(dst any, opts ...Option) error
func WithPrefix(prefix string) Option
func WithLookup(lookup LookupFunc) Option
func WithSources(sources ...Source) Option
func WithValidator[T any](fn func(T) error) Option
func FromLookup(lookup LookupFunc) Source
func FromFiles(files ...string) Source
func FromOptionalFiles(files ...string) Source
func WithFiles(files ...string) Option
func WithOptionalFiles(files ...string) Option
func Read(files ...string) (LookupFunc, error)
func ReadOptional(files ...string) (LookupFunc, error)
func Export(files ...string) error
func ExportOptional(files ...string) error
```

### Repeated options and priorities

| Group | Options | Repeated calls |
| --- | --- | --- |
| Prefix | `WithPrefix` | Last prefix wins, including empty prefix |
| Sources | `WithLookup`, `WithFiles`, `WithOptionalFiles`, `WithSources` | Last call replaces the complete source selection; no merging |
| Validator | `WithValidator` | Last callback wins; callbacks do not accumulate |

Groups are independent. **Invalid options remain errors even when replaced**:
nil option/lookup/validator, wrong parent validator type, zero source, empty
source list, and empty file path all fail before I/O or callbacks.

```go
env.Load[Config](
    env.WithPrefix("OLD_"), env.WithPrefix("APP_"),
    env.WithFiles("a.env"), env.WithOptionalFiles("b.env"),
    env.WithValidator(validateA), env.WithValidator(validateB),
)
```

This uses `APP_`, only `b.env`, and only `validateB`. To combine checks, explicitly
call both within one parent callback. By contrast, `WithFiles("a.env", "b.env")`
uses both files and the **first file finding a key wins**, even for an empty value.
This source lookup priority is different from option replacement.

### Files, snapshots, and Optional

The default source is `os.LookupEnv`. Source options replace it completely; no
implicit OS fallback occurs. For explicit OS priority followed by optional files:

```go
cfg, err := env.Load[Config](
    env.WithPrefix("APP_"),
    env.WithSources(
        env.FromLookup(os.LookupEnv),
        env.FromOptionalFiles(".env.local"),
        env.FromFiles(".env"),
    ),
)
```

`Read()` immediately reads `.env` into an immutable lookup snapshot, safe for
concurrent reads. `Read("a.env", "b.env")` reads explicit paths. Failure returns
nil; editing files afterward cannot change a successful snapshot. Quoted
whitespace is preserved, without decoder trimming or environment mutation.

`FromFiles` and `FromOptionalFiles` describe sources without I/O. `WithFiles` and
`WithOptionalFiles` are shortcuts for `WithSources(From...(...))`. Descriptions
copy caller paths; `WithSources` copies its list. Every `Load`/`Parse` validates
the destination, all options, and full schema, then reads selected files anew.
Relative paths use the working directory at read time; no arguments select
`.env`; an explicitly empty path is invalid before any file I/O.

Optional skips **each not-found file only**, then continues. Directories,
permission/read failures, and invalid syntax remain errors. All optional files
missing yields a non-nil empty lookup/source with no implicit OS fallback.
Optional files do not disable required fields. All selected files are validated,
even lower-priority files and unused keys. The first file failure stops the
operation before value lookup or custom decoding. Valid overwritten file options
are not read. There is no atomic snapshot guarantee across multiple files.

The first source returning `found=true` wins, including empty/whitespace values.
Those values do not select fallback or lower-priority sources. `found=false`
ignores any returned text. Each full key, including misses, is looked up once
per operation; fields sharing a key independently apply binding and conversion.

### Export to the process environment

`Export(files...)` reads and validates every file first, then installs only keys
absent from `os.LookupEnv`. Existing values, including empty values, win.
`ExportOptional` skips not-found files; all missing succeeds without changes.
Values come directly from the dotenv parser, with quoted whitespace/escapes intact.

Keys are installed in sorted order. A read/parse failure causes **no mutation**.
The first setenv failure stops the operation; successful earlier writes remain,
with **no rollback**. Its sanitized `SourceError` has `Op="setenv"`, the winning
file path, `Line=0`, and matches `ErrSource` and `ErrSourceSetenv`.

Use Export during startup before other environment writers: checking existence
and setting a key are not atomic together. It affects this process, never the
parent shell. There is no overwrite mode, automatic unset, or logging.
`Read`, `Load`, and `Parse` do not install environment variables.

## Tags, absent values, and empty text

| Tag | Placement and meaning |
| --- | --- |
| `env:"KEY"` | Supported leaf; explicit key, no name inference/options in key |
| `binding:"optional"` | Leaf with env; default when binding is absent |
| `binding:"required"` | Requires the key to exist; an empty string is allowed |
| `binding:"nonempty"` | Rejects selected text empty after trimming; does not require an absent optional key |
| `binding:"notrim"` | Preserves whitespace of selected text and built-in slice tokens |
| `fallback:"TEXT"` | Leaf with env; selected only when key is absent, including explicitly empty fallback |
| `separator:"TEXT"` | Slice field with env; nonempty delimiter, default comma |
| `prefix:"TEXT"` | Named struct or pointer-to-struct container; concatenates prefixes |
| `env:"-"` | Skips field and subtree; cannot carry other env-owned tags |

Binding flags are comma-separated; whitespace around flags is ignored.
`required, nonempty` is valid. Unknown/duplicate flags, empty/whitespace-only tags,
empty entries (leading/trailing/repeated commas), `required,optional`, and
`required` together with any fallback are invalid schemas. Binding is permitted
on every supported env leaf, including custom map/array, but never on a nested
container. `nonempty` checks the whole selected text, not decoded values/elements.

```go
type Selection struct {
    Name  string `env:"NAME" fallback:"service"`
    Note  string `env:"NOTE" binding:"required"`
    Token string `env:"TOKEN" binding:"required,nonempty"`
    Label string `env:"LABEL" binding:"optional,nonempty"`
    Raw   string `env:"RAW" binding:"notrim"`
}
```

Absent NAME uses `service`; present NAME empty remains empty. NOTE needs a key;
TOKEN also needs nonempty text. LABEL may be absent but cannot select empty text.
Fallback conversion is lazy; unused invalid fallback text does not fail decoding.

| Selection | Result |
| --- | --- |
| Optional absent without fallback | Zero leaf, nil pointer/slice; custom decoder not called |
| Required absent | `ErrRequired` |
| Present empty or selected empty fallback | Converted as empty text; never rescued by fallback |
| Empty text with nonempty | `ErrEmptyValue` and `ErrInvalidValue`, before conversion |
| Empty built-in string / pointer string | Empty string / allocated pointer to empty string |
| Empty built-in numeric/bool/duration, including pointers | `ErrInvalidValue` |
| Empty built-in slice | Non-nil empty slice, zero tokens |
| Empty custom leaf | `UnmarshalText([]byte(""))`; method chooses result/error |

Default `strings.TrimSpace` removes outside Unicode whitespace, tabs, and newlines
from selected key/fallback text and every built-in slice token. Interior spaces
are preserved. `notrim` skips both trim stages, including on fallback. With
`nonempty,notrim`, spaces alone are nonempty.

**Slice warning:** `[]int` input `1,2` is valid both by default and with notrim.
`1, 2` is valid by default but fails with notrim because the second token retains
its space. Numeric/bool/duration tokens can fail when whitespace is preserved.
Lookup text `" 1,2 "` similarly fails with notrim; in dotenv, use a quoted value
to demonstrate outer whitespace, because the parser already trims unquoted text.

For strings, lookup `" apple , orange "` becomes `{"apple", "orange"}` by default
and `{" apple ", " orange "}` with notrim. `a,,b` has three tokens; empty tokens
succeed for strings, fail built-in numeric/bool/duration, and follow custom element
methods. An empty whole value has zero tokens. A whole-slice custom decoder owns
splitting and receives the complete text. No CSV quoting is added.

## Type support map

All leaves require `env`; untagged scalar fields are ignored. Named struct and
pointer-to-struct fields without env are recursive containers. Global `APP_`,
container `DB_`, child `READ_`, and leaf `HOST` form `APP_DB_READ_HOST`.

| Group | Types / forms | Input and limits |
| --- | --- | --- |
| Direct | `string`, `bool` | Text; standard `strconv.ParseBool` forms, no yes/no/on/off |
| Direct | Signed/unsigned integers, named scalar variants | Decimal with field bit-width/range checks; leading zero allowed, hex/underscores rejected |
| Direct | `float32/64`, named variants | Standard float text; NaN, infinity, overflow rejected |
| Direct | Exact `time.Duration` (including aliases) | `1h30m`, `500ms`, `0`; no `d` unit |
| Direct | `time.Time`, `net.IP`, `netip.Addr/Prefix/AddrPort`, `big.Int` | Existing `UnmarshalText`; RFC3339 time, standard IP/prefix/address-port and integer formats; no user method needed |
| Direct | One-level scalar/custom leaf pointers, one-dimensional leaf slices | Nil if absent; allocated if selected. Default comma or custom separator. Pointers to named slices require a custom decoder |
| User UnmarshalText needed | Domain scalar/struct/slice, named map/array, concrete complex/uintptr/function/channel | Entire chosen text; method controls grammar and content. Scalar/slice methods override built-in parsing |
| Rejected | Map/array without method; ordinary struct as env leaf | No built-in map/array/struct parser; untagged struct is a container |
| Rejected | Interface/any leaves, root other than struct, tagged unexported/active anonymous fields | A concrete field and valid schema are required, even if an interface declares UnmarshalText |
| Rejected | Multiple pointers, pointer map/array fields, built-in pointer slice fields, pointer slice elements, nested built-in collections | `*CustomSlice` and `[]CustomSlice` are supported when CustomSlice implements whole-value text decoding |

A new `type Delay time.Duration` has integer semantics unless it adds UnmarshalText;
`type Delay = time.Duration` retains duration parsing. `[]byte` is a numeric uint8
slice (`65,66`), **not** raw text, base64, or hex. `url.URL` needs a local wrapper
with UnmarshalText. Custom Unix timestamps/layouts also need a method; no unit
or format detection is performed. `[]CustomLeaf` may use concrete custom leaves,
including named maps/arrays; this does not introduce nested built-in parsing.

### Custom decoders

Implement `encoding.TextUnmarshaler` as `UnmarshalText([]byte) error` on a named
local type or its pointer. Every staged leaf is addressable; a pointer receiver
does **not** require the field to be a pointer. Merely defining a free function
ParseX/UnmarshalX, MarshalText, UnmarshalJSON, or UnmarshalBinary is insufficient.
Use a local wrapper for a type owned by another package. Data whose direct field
shape is rejected (for example a pointer map) can instead live inside a custom
struct leaf whose method owns allocation/parsing; that does not enable the
rejected original field shape. Methods run during
conversion, after source/binding resolution, never during schema validation.

This complete example shows custom scalar, struct, slice, map, and array leaves:

```go
package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "strings"

    "github.com/muhammaadreza/gouse/env"
)

type Level string
func (v *Level) UnmarshalText(text []byte) error {
    switch strings.ToLower(string(text)) {
    case "info", "debug":
        *v = Level(strings.ToLower(string(text)))
        return nil
    default:
        return errors.New("unsupported level")
    }
}
type Endpoint struct { Host string; Port int }
func (v *Endpoint) UnmarshalText(text []byte) error {
    type plain Endpoint // Avoid invoking this TextUnmarshaler from JSON itself.
    return json.Unmarshal(text, (*plain)(v))
}
type Words []string
func (v *Words) UnmarshalText(text []byte) error {
    *v = strings.Fields(string(text))
    return nil
}
type Labels map[int]string
func (v *Labels) UnmarshalText(text []byte) error {
    type plain Labels
    return json.Unmarshal(text, (*plain)(v))
}
type Pair [2]int
func (v *Pair) UnmarshalText(text []byte) error {
    type plain Pair
    return json.Unmarshal(text, (*plain)(v))
}
type Config struct {
    Level Level `env:"LEVEL"`
    Endpoint Endpoint `env:"ENDPOINT"`
    Words Words `env:"WORDS"`
    Labels Labels `env:"LABELS"`
    Pair Pair `env:"PAIR"`
}
func main() {
    values := map[string]string{
        "LEVEL":"DEBUG", "ENDPOINT":`{"Host":"db","Port":5432}`,
        "WORDS":"a b", "LABELS":`{"7":"seven"}`, "PAIR":"[1,2]",
    }
    cfg, err := env.Load[Config](env.WithLookup(func(key string) (string, bool) {
        value, found := values[key]
        return value, found
    }))
    fmt.Println(cfg.Level, cfg.Endpoint.Host, cfg.Words, cfg.Labels[7], cfg.Pair, err)
}
```

The method owns map key/value types, duplicate/empty entry policy, and nil/empty
results. Named arrays likewise own length/input policy. A custom struct with env
is a leaf; without env it remains a container. A whole named-slice decoder owns
its syntax even if a separator tag is present (the tag still needs a slice field).
Raw custom errors are discarded as sanitized `ErrInvalidValue`; no partial
candidate commits. The executable `Example_customTypes` shows success and failure.

## Parent validation

`WithValidator[T any](func(T) error)` accepts **exactly** `func(Config) error` or
`func(*Config) error` for the root Config. Use a free function, `Config.Validate`,
or `(*Config).Validate`; a bound `cfg.Validate` is a zero-argument method value
and does not fit this API. Every callback's nil/type is checked, including those
later overwritten, before reading sources. No automatic method discovery occurs.

The callback runs once after successful decoding and before commit. Pointer
callbacks receive the fresh candidate, never the old Parse destination. Mutations
commit when the callback returns nil; failure discards the candidate. Value
callbacks may also change referenced candidate data; this is not read-only access.
Application side effects outside the candidate cannot be rolled back.

The application chooses child composition and first-error versus collect-all:

```go
// Alternative A: stop on the first child error.
func (cfg Config) Validate() error {
    if err := cfg.Server.Validate(); err != nil {
        return err
    }
    return cfg.Database.Validate()
}
// Alternative B, instead of A: invoke both child checks.
func validateAll(cfg Config) error {
    return errors.Join(cfg.Server.Validate(), cfg.Database.Validate())
}
// Choose env.WithValidator(Config.Validate) or env.WithValidator(validateAll).
```

The library never discovers skipped checks, traverses child validators, accumulates
callbacks, logs, or exits. Parent errors match `ErrValidation` through an internal
wrapper. Their message, original identity/type, and all `errors.Join` branches
remain available through `errors.Is/As`. **Validator message privacy belongs to
the application**; unlike custom decoding, these errors retain application text.

## Dotenv syntax and schema boundaries

Files are valid UTF-8, optional initial BOM, LF/CRLF, optional final newline, with
no fixed line-size limit. NUL, bare CR, and other BOMs are rejected even in comments.

| Syntax | Behavior |
| --- | --- |
| `KEY=value` | ASCII `[A-Za-z_][A-Za-z0-9_]*`, case sensitive; spaces/tabs around `=` allowed |
| `export KEY=value` | export followed by spaces/tabs |
| Blank / `# comment` | Ignored with optional leading spaces/tabs |
| Unquoted | Outside spaces/tabs trimmed; # begins comment at value start or after space/tab |
| Single quoted | Literal text, whitespace/# preserved, no escapes |
| Double quoted | Preserves text; escapes `\\`, `\"`, `\n`, `\r`, `\t` |
| After closing quote | Spaces/tabs and optional comment only |
| `KEY=`, `KEY=''`, `KEY=""` | Present empty value |
| `$VAR`, `${VAR}`, shell expressions | Literal, never expanded or executed |

Duplicate keys inside one file and invalid assignments/quotes/escapes fail.
Physical multiline quotes/line continuations are unsupported; escaped `\n` works.
Read and Export preserve parser values; binding trims only during struct decoding.

Owned tags require proper Go quoted string syntax; malformed owned tags fail
schema validation. Absent tags differ from explicitly empty tags. Other packages'
tags follow Go's lookup semantics and are not interpreted. Skip cannot coexist
with other owned tags. Cycles in active containers fail; repeated sibling types
are allowed. Untagged unexported fields are ignored, tagged ones fail. Active
anonymous fields are invalid; `env:"-"` can skip them. Ordinary untagged unsupported
fields are ignored rather than inferred.

## Error categories and privacy

Library messages have no `env:` prefix. Field errors aggregate in depth-first
declaration order, with ascending slice indexes. Sources stop on the first file
failure. Invalid API calls have no empty `field ""` context.

| Failure | Matching sentinels via errors.Is |
| --- | --- |
| Required absent | `ErrRequired` |
| Nonempty violation | `ErrEmptyValue`, `ErrInvalidValue` |
| Conversion | `ErrInvalidValue` |
| Unsupported type | `ErrUnsupportedType`, `ErrInvalidSchema` |
| Other schema | `ErrInvalidSchema` |
| Invalid option/input | `ErrInvalidOption` |
| File missing | `ErrSource`, `ErrSourceRead`, `ErrSourceNotFound` |
| Permission | `ErrSource`, `ErrSourceRead`, `ErrSourcePermission` |
| Other read | `ErrSource`, `ErrSourceRead` |
| Dotenv syntax | `ErrSource`, `ErrSourceSyntax` |
| Environment installation | `ErrSource`, `ErrSourceSetenv` |
| Parent callback failure | `ErrValidation`, plus original application's categories |

Sentinels have distinct identities; the hierarchy above describes errors returned
by operations, rather than making the sentinel variables aliases.

`errors.As` retrieves `*FieldError` (`Path`, `Key`, `Index`, `Category`) or
`*SourceError` (`Path`, `Op`, `Line`). Field Index is -1 for the whole field;
source Line is one-based when known, otherwise zero. Source Op is read, parse,
or setenv. Paths/key names are intentionally public metadata. FieldError unwraps
to its category; SourceError unwraps only ErrSource, using Is for specific categories.

Examples of the message catalog:

```text
field "Token" key "TOKEN": required key missing
field "Token" key "TOKEN": empty value
field "Ports" key "PORTS"[2]: invalid value
field "Labels" key "LABELS": unsupported type
field "Token" key "TOKEN": invalid schema
source failure: read ".env": file not found
source failure: read ".env": permission denied
source failure: read ".env": read failed
source failure: parse ".env" line 12: invalid assignment
source failure: parse ".env" line 12: invalid quoted value
source failure: parse ".env" line 12: duplicate key
source failure: parse ".env" line 12: invalid encoding or control character
source failure: setenv ".env": could not set environment variable
invalid option or input: nil lookup
invalid option or input: nil validator
invalid option or input: empty file path
invalid option or input: destination must be a non-nil pointer to a struct
```

Library errors retain no selected/fallback/file-line text or raw filesystem/parser/
custom decoder errors in formatting, metadata, or chain. Parent validator errors
explicitly retain the application's message and chain. No error helper for listing
all fields is provided; standard aggregate traversal remains available.

## Concurrency and limits

Independent operations share no mutable parsing state. Shared custom lookup,
decoder, and validator code must provide its own synchronization. Destinations
require caller synchronization. User callback side effects/panics are outside the
library's control. There is no global schema cache, watcher, automatic reload,
patch mode, registry/DSL, public introspection/provenance, generator, secret-file/
_FILE convention, remote provider, built-in map/array grammar, or Export overwrite.

Executable examples cover the file APIs, binding/empty/fallback/notrim,
repeated options, custom types, and parent validation strategies. Export examples
restore process environment. From the repository root:

```sh
go test ./...
go vet ./...
go test -race ./...
go doc github.com/muhammaadreza/gouse/env
```
