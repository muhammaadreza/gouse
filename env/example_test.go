package env_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/muhammaadreza/gouse/env"
)

func ExampleLoad() {
	type database struct {
		Host string `env:"HOST" binding:"required"`
	}
	type config struct {
		Port    int           `env:"PORT" fallback:"8080"`
		Timeout time.Duration `env:"TIMEOUT" fallback:"30s"`
		DB      *database     `prefix:"DB_"`
	}
	values := map[string]string{"APP_DB_HOST": "localhost"}
	cfg, err := env.Load[config](env.WithPrefix("APP_"), env.WithLookup(func(key string) (string, bool) { value, ok := values[key]; return value, ok }))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.Port, cfg.Timeout, cfg.DB.Host)
	// Output: 8080 30s localhost
}

func ExampleParse() {
	type config struct {
		Port int `env:"PORT"`
		Old  string
	}
	cfg := config{Port: 9000, Old: "previous value"}
	err := env.Parse(&cfg, env.WithLookup(func(string) (string, bool) { return "", false }))
	fmt.Println(cfg.Port, cfg.Old == "", err)
	// Output: 0 true <nil>
}

type logLevel string

func (l *logLevel) UnmarshalText(text []byte) error {
	switch strings.ToLower(string(text)) {
	case "info", "debug":
		*l = logLevel(strings.ToLower(string(text)))
		return nil
	default:
		return errors.New("unsupported level")
	}
}

func Example_customDecoder() {
	type config struct {
		Level logLevel `env:"LEVEL" fallback:"INFO"`
	}
	cfg, err := env.Load[config](env.WithLookup(func(string) (string, bool) { return "", false }))
	fmt.Println(cfg.Level, err)
	// Output: info <nil>
}

func ExampleFieldError() {
	type config struct {
		Ports []int `env:"PORTS"`
	}
	_, err := env.Load[config](env.WithLookup(func(string) (string, bool) { return "1,secret", true }))
	var field *env.FieldError
	if errors.As(err, &field) {
		fmt.Println(field.Path, field.Key, field.Index, errors.Is(err, env.ErrInvalidValue))
	}
	// Output: Ports PORTS 1 true
}

func ExampleRead() {
	dir, err := os.MkdirTemp("", "env-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.env")
	if err := os.WriteFile(path, []byte("APP_PORT=8080\nAPP_NAME=' service '\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	lookup, err := env.Read(path) // No arguments reads .env in the current directory.
	if err != nil {
		fmt.Println(err)
		return
	}
	value, found := lookup("APP_NAME") // Snapshot preserves quoted whitespace.
	fmt.Printf("%q %v\n", value, found)
	type config struct {
		Port int `env:"PORT"`
	}
	cfg, err := env.Load[config](env.WithPrefix("APP_"), env.WithLookup(lookup))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.Port)
	// Output:
	// " service " true
	// 8080
}

func ExampleWithFiles() {
	dir, err := os.MkdirTemp("", "env-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.env")
	if err := os.WriteFile(path, []byte("PORT=8080"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	type config struct {
		Port int `env:"PORT" fallback:"3000"`
	}
	cfg, err := env.Load[config](env.WithFiles(path)) // WithFiles() reads .env.
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.Port)
	// Output: 8080
}

func ExampleWithSources() {
	dir, err := os.MkdirTemp("", "env-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.env")
	if err := os.WriteFile(path, []byte("APP_PORT=8080\nAPP_HOST=file-host"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	type config struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	cfg, err := env.Load[config](env.WithPrefix("APP_"), env.WithSources(
		env.FromLookup(func(key string) (string, bool) {
			return "custom-host", key == "APP_HOST"
		}),
		env.FromFiles(path),
		env.FromLookup(os.LookupEnv),
	))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(cfg.Host, cfg.Port)
	// Output: custom-host 8080
}

func ExampleSourceError() {
	_, err := env.Read("\x00") // An invalid filesystem path, on every platform.
	var source *env.SourceError
	if errors.As(err, &source) {
		fmt.Println(source.Op, source.Line, errors.Is(err, env.ErrSource))
	}
	// Output: read 0 true
}

func exampleFiles(contents ...string) ([]string, func()) {
	dir, err := os.MkdirTemp("", "gouse-example-")
	if err != nil {
		panic(err)
	}
	cleanup := func() {
		if err := os.RemoveAll(dir); err != nil {
			panic(err)
		}
	}
	paths := make([]string, len(contents))
	for i, content := range contents {
		paths[i] = filepath.Join(dir, fmt.Sprintf("%d.env", i))
		if err := os.WriteFile(paths[i], []byte(content), 0o600); err != nil {
			cleanup()
			panic(err)
		}
	}
	return paths, cleanup
}

func ExampleReadOptional() {
	paths, cleanup := exampleFiles("NAME=' service '")
	defer cleanup()
	missing := paths[0] + ".missing"
	lookup, err := env.ReadOptional(missing, paths[0], missing)
	if err != nil {
		fmt.Println(err)
		return
	}
	text, found := lookup("NAME")
	fmt.Printf("%q %v\n", text, found)
	empty, err := env.ReadOptional(missing)
	_, found = empty("NAME")
	fmt.Println(found, err)
	// Output:
	// " service " true
	// false <nil>
}

func ExampleFromOptionalFiles() {
	paths, cleanup := exampleFiles("NAME=from-file")
	defer cleanup()
	type config struct {
		Name string `env:"NAME"`
	}
	cfg, err := env.Load[config](env.WithSources(
		env.FromOptionalFiles(paths[0]+".missing"),
		env.FromOptionalFiles(paths[0]),
		env.FromLookup(func(string) (string, bool) { return "later", true }),
	))
	fmt.Println(cfg.Name, err)
	// Output: from-file <nil>
}

func ExampleWithOptionalFiles() {
	paths, cleanup := exampleFiles("NAME=present")
	defer cleanup()
	type config struct {
		Name string `env:"NAME" fallback:"default"`
	}
	cfg, err := env.Load[config](env.WithOptionalFiles(paths[0] + ".missing"))
	fmt.Println(cfg.Name, err)
	cfg, err = env.Load[config](env.WithOptionalFiles(paths[0]+".missing", paths[0]))
	fmt.Println(cfg.Name, err)
	// Output:
	// default <nil>
	// present <nil>
}

// Example exports restore both pre-existing values and absence after completion.
func exampleEnvironment(key string) func() {
	old, found := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		panic(err)
	}
	return func() {
		if found {
			if err := os.Setenv(key, old); err != nil {
				panic(err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			panic(err)
		}
	}
}

func ExampleExport() {
	const key = "GOUSE_EXAMPLE_EXPORT"
	restore := exampleEnvironment(key)
	defer restore()
	paths, cleanup := exampleFiles(key+"=' first '", key+"=second")
	defer cleanup()
	err := env.Export(paths...)
	fmt.Printf("%q %v\n", os.Getenv(key), err)
	// Export preserves an existing value even when it is empty.
	if err := os.Setenv(key, ""); err != nil {
		panic(err)
	}
	err = env.Export(paths...)
	fmt.Printf("%q %v\n", os.Getenv(key), err)
	// Output:
	// " first " <nil>
	// "" <nil>
}

func ExampleExportOptional() {
	const key = "GOUSE_EXAMPLE_EXPORT_OPTIONAL"
	restore := exampleEnvironment(key)
	defer restore()
	paths, cleanup := exampleFiles(key + "=installed")
	defer cleanup()
	err := env.ExportOptional(paths[0]+".missing", paths[0])
	fmt.Println(os.Getenv(key), err)
	// Output: installed <nil>
}

func Example_binding() {
	type config struct {
		Name        string `env:"NAME" fallback:"service"`
		Description string `env:"DESCRIPTION" binding:"required"`
		Label       string `env:"LABEL" binding:"optional,nonempty"`
		Raw         string `env:"RAW" binding:"nonempty,notrim"`
	}
	for _, present := range []bool{false, true} {
		values := map[string]string{"DESCRIPTION": "", "RAW": "  "}
		if present {
			values["NAME"] = ""
		}
		cfg, err := env.Load[config](env.WithLookup(func(key string) (string, bool) { v, ok := values[key]; return v, ok }))
		fmt.Printf("name=%q description=%q label=%q raw=%q error=%v\n", cfg.Name, cfg.Description, cfg.Label, cfg.Raw, err)
	}
	// Required checks key existence; nonempty checks selected text after trimming.
	_, err := env.Load[struct {
		Token string `env:"TOKEN" binding:"required,nonempty"`
	}](env.WithLookup(func(string) (string, bool) { return "  ", true }))
	fmt.Println(errors.Is(err, env.ErrEmptyValue), errors.Is(err, env.ErrInvalidValue))
	// Output:
	// name="service" description="" label="" raw="  " error=<nil>
	// name="" description="" label="" raw="  " error=<nil>
	// true true
}

func Example_notrim() {
	type trimmed struct {
		Numbers []int `env:"N"`
	}
	type raw struct {
		Numbers []int `env:"N" binding:"notrim"`
	}
	// notrim preserves each token. Numeric/bool/duration tokens with whitespace
	// can fail conversion; compact numeric tokens remain valid.
	for _, text := range []string{"1,2", "1, 2", " 1,2 "} {
		lookup := env.WithLookup(func(string) (string, bool) { return text, true })
		a, ea := env.Load[trimmed](lookup)
		b, eb := env.Load[raw](lookup)
		fmt.Printf("%q: default=%v/%v notrim=%v/%v\n", text, a.Numbers, ea == nil, b.Numbers, eb == nil)
	}
	// The outer whitespace above comes from lookup; use quoted values in dotenv.

	// Output:
	// "1,2": default=[1 2]/true notrim=[1 2]/true
	// "1, 2": default=[1 2]/true notrim=[]/false
	// " 1,2 ": default=[1 2]/true notrim=[]/false
}

type exampleConfig struct {
	Port int `env:"PORT" fallback:"8080"`
}

var errExamplePort = errors.New("port must be positive")

var errExampleEven = errors.New("port must be even")

func validateExamplePort(c exampleConfig) error {
	if c.Port <= 0 {
		return errExamplePort
	}
	return nil
}

func validateExampleEven(c exampleConfig) error {
	if c.Port%2 != 0 {
		return errExampleEven
	}
	return nil
}

// Validate chooses a first-error policy and explicitly invokes its child rules.
func (c exampleConfig) Validate() error {
	if err := validateExamplePort(c); err != nil {
		return err
	}
	return validateExampleEven(c)
}

func (c *exampleConfig) Normalize() error {
	if c.Port == 0 {
		c.Port = 8080
	}
	return c.Validate()
}

func ExampleWithValidator() {
	absent := env.WithLookup(func(string) (string, bool) { return "", false })
	// A free function and value or pointer method expressions are all accepted.
	for _, validator := range []env.Option{
		env.WithValidator(validateExamplePort),
		env.WithValidator(exampleConfig.Validate),
		env.WithValidator((*exampleConfig).Normalize),
	} {
		cfg, err := env.Load[exampleConfig](absent, validator)
		fmt.Println(cfg.Port, err)
	}
	// Output:
	// 8080 <nil>
	// 8080 <nil>
	// 8080 <nil>
}

func ExampleWithValidator_collect() {
	lookup := env.WithLookup(func(string) (string, bool) { return "-1", true })
	_, first := env.Load[exampleConfig](lookup, env.WithValidator(exampleConfig.Validate))
	_, all := env.Load[exampleConfig](lookup, env.WithValidator(func(c exampleConfig) error {
		return errors.Join(validateExamplePort(c), validateExampleEven(c))
	}))
	fmt.Println(errors.Is(first, env.ErrValidation), errors.Is(first, errExamplePort), errors.Is(first, errExampleEven))
	fmt.Println(errors.Is(all, env.ErrValidation), errors.Is(all, errExamplePort), errors.Is(all, errExampleEven))
	// Application error messages and chains are preserved by the validator.

	// Output:
	// true true false
	// true true true
}

func ExampleOption() {
	paths, cleanup := exampleFiles("APP_PORT=1000\nAPP_NAME=first", "APP_PORT=2000")
	defer cleanup()
	type config struct {
		Port int    `env:"PORT"`
		Name string `env:"NAME"`
	}
	calls := 0
	cfg, err := env.Load[config](
		env.WithPrefix("OLD_"), env.WithPrefix("APP_"),
		env.WithFiles(paths[0]), env.WithOptionalFiles(paths[1]), // Replaces all sources.
		env.WithValidator(func(config) error { panic("overwritten callback") }),
		env.WithValidator(func(config) error { calls++; return nil }),
	)
	fmt.Println(cfg.Port, cfg.Name == "", calls, err)
	cfg, err = env.Load[config](env.WithPrefix("APP_"), env.WithFiles(paths...))
	fmt.Println(cfg.Port, cfg.Name, err) // Within one group, the first file wins.
	_, err = env.Load[config](env.WithLookup(nil), env.WithFiles(paths...))
	fmt.Println(errors.Is(err, env.ErrInvalidOption)) // Invalid stays invalid.
	_, err = env.Load[config](env.WithValidator[config](nil), env.WithValidator(func(config) error { return nil }))
	fmt.Println(errors.Is(err, env.ErrInvalidOption)) // No accumulation or nil reset.
	// Output:
	// 2000 true 1 <nil>
	// 1000 first <nil>
	// true
	// true
}

type exampleEndpoint struct {
	Host string
	Port int
}

func (v *exampleEndpoint) UnmarshalText(text []byte) error {
	type plain exampleEndpoint
	return json.Unmarshal(text, (*plain)(v))
}

type exampleWords []string

func (v *exampleWords) UnmarshalText(text []byte) error {
	*v = strings.Fields(string(text))
	return nil
}

type exampleLabels map[int]string

func (v *exampleLabels) UnmarshalText(text []byte) error {
	type plain exampleLabels
	return json.Unmarshal(text, (*plain)(v))
}

type examplePair [2]int

func (v *examplePair) UnmarshalText(text []byte) error {
	type plain examplePair
	return json.Unmarshal(text, (*plain)(v))
}

func Example_customTypes() {
	type config struct {
		Level    logLevel        `env:"LEVEL"`
		Endpoint exampleEndpoint `env:"ENDPOINT"`
		Words    exampleWords    `env:"WORDS"`
		Labels   exampleLabels   `env:"LABELS"`
		Pair     examplePair     `env:"PAIR"`
		Time     time.Time       `env:"TIME"`
	}
	values := map[string]string{"LEVEL": "DEBUG", "ENDPOINT": `{"Host":"db","Port":5432}`, "WORDS": "a b", "LABELS": `{"7":"seven"}`, "PAIR": "[1,2]", "TIME": "2026-10-21T01:02:03Z"}
	lookup := env.WithLookup(func(key string) (string, bool) { v, ok := values[key]; return v, ok })
	cfg, err := env.Load[config](lookup)
	fmt.Println(cfg.Level, cfg.Endpoint.Host, cfg.Endpoint.Port, cfg.Words, cfg.Labels[7], cfg.Pair, cfg.Time.Format(time.RFC3339), err)
	values["LABELS"] = "CANARY_INVALID_JSON"
	_, err = env.Load[config](lookup)
	fmt.Println(err) // Custom decoder errors discard raw text and original chain.
	// Output:
	// debug db 5432 [a b] seven [1 2] 2026-10-21T01:02:03Z <nil>
	// field "Labels" key "LABELS": invalid value
}
