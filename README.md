# gouse

Focused Go utilities with explicit APIs and predictable behavior.

[Quick start](#quick-start) · [Installation](#installation) ·
[Packages](#packages) · [Documentation](#documentation)

Go **1.27.1+** · Standard library only

## Quick start

Read configuration into a Go struct with the `env` package:

```go
package main

import (
    "fmt"
    "log"

    "github.com/muhammaadreza/gouse/env"
)

type Config struct {
    Port  int  `env:"PORT" fallback:"8080"`
    Debug bool `env:"DEBUG"`
}

func main() {
    cfg, err := env.Load[Config](env.WithPrefix("APP_"))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(cfg.Port, cfg.Debug)
}
```

With `APP_PORT=9000` and `APP_DEBUG=true`, this prints `9000 true`.

## Installation

All packages belong to one module:

```text
github.com/muhammaadreza/gouse
```

For a local checkout before publication, add these lines to your application's
`go.mod`. Replace `../gouse` with the path to the repository root:

```go
require github.com/muhammaadreza/gouse v0.0.0

replace github.com/muhammaadreza/gouse => ../gouse
```

Import the package you need, such as `github.com/muhammaadreza/gouse/env`.
Packages share one module version.

## Packages

| Package | Purpose |
| --- | --- |
| [env](env/README.md) | Decode environment variables, dotenv files, and ordered custom sources into typed structs. |

The `env` package provides explicit tags, required/nonempty checks, defaults,
custom text decoding, optional files, and parent validation. Failed decoding or
validation leaves the destination unchanged. Process environment changes require
an explicit `Export` call.

## Documentation

- [Configuration guide and API reference](env/README.md)
- [Tags, defaults, and empty values](env/README.md#tags-absent-values-and-empty-text)
- [Supported types and custom decoders](env/README.md#type-support-map)
- [Validation](env/README.md#parent-validation)
- [Errors and privacy](env/README.md#error-categories-and-privacy)

Browse the package documentation locally:

```sh
go doc github.com/muhammaadreza/gouse/env
```

## Testing

Run all package checks from the repository root:

```sh
go test ./...
go vet ./...
go test -race ./...
```
