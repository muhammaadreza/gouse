package env_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env"
)

type bench8 struct {
	Name    string        `env:"NAME"`
	Debug   bool          `env:"DEBUG"`
	Port    int           `env:"PORT"`
	Limit   uint64        `env:"LIMIT"`
	Ratio   float64       `env:"RATIO"`
	Timeout time.Duration `env:"TIMEOUT"`
	Retries *int          `env:"RETRIES"`
	Peers   []string      `env:"PEERS"`
}

type bench32 struct {
	A bench8 `prefix:"A_"`
	B bench8 `prefix:"B_"`
	C bench8 `prefix:"C_"`
	D bench8 `prefix:"D_"`
}

type bench128 struct {
	A bench32 `prefix:"A_"`
	B bench32 `prefix:"B_"`
	C bench32 `prefix:"C_"`
	D bench32 `prefix:"D_"`
}

// Full Load calls include schema reflection, lookup caching, decoding and
// allocation. Only building the immutable in-memory input map is untimed.
func benchmarkLoad[T any](b *testing.B, count int) {
	b.Helper()
	values := make(map[string]string, count)
	var populate func(reflect.Type, string)
	populate = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.Type.Kind() == reflect.Struct {
				populate(field.Type, prefix+field.Tag.Get("prefix"))
				continue
			}
			key := field.Tag.Get("env")
			text := map[string]string{"NAME": "service", "DEBUG": "false", "PORT": "8080", "LIMIT": "100", "RATIO": "1.5", "TIMEOUT": "30s", "RETRIES": "3", "PEERS": "a,b,c"}[key]
			values[prefix+key] = text
		}
	}
	populate(reflect.TypeFor[T](), "")
	if len(values) != count {
		b.Fatalf("benchmark has %d distinct fields, want %d", len(values), count)
	}
	lookup := source(values)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := env.Load[T](lookup); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoad8(b *testing.B)   { benchmarkLoad[bench8](b, 8) }
func BenchmarkLoad32(b *testing.B)  { benchmarkLoad[bench32](b, 32) }
func BenchmarkLoad128(b *testing.B) { benchmarkLoad[bench128](b, 128) }
