package env_test

import (
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env"
)

type labels map[int]string

func (v *labels) UnmarshalText(text []byte) error {
	type plain labels
	return json.Unmarshal(text, (*plain)(v))
}

type pair [2]int

func (v *pair) UnmarshalText(text []byte) error {
	type plain pair
	return json.Unmarshal(text, (*plain)(v))
}

type customComplex complex128

func (v *customComplex) UnmarshalText([]byte) error { *v = 1 + 2i; return nil }

type customUintptr uintptr

func (v *customUintptr) UnmarshalText([]byte) error { *v = 42; return nil }

type customFunc func() string

func (v *customFunc) UnmarshalText(text []byte) error {
	s := string(text)
	*v = func() string { return s }
	return nil
}

type customChan chan string

func (v *customChan) UnmarshalText(text []byte) error {
	*v = make(chan string, 1)
	*v <- string(text)
	return nil
}

type wholeSlice []string

func (v *wholeSlice) UnmarshalText(text []byte) error { *v = []string{string(text)}; return nil }

type failingMap map[string]string

func (v *failingMap) UnmarshalText(text []byte) error {
	*v = failingMap{"partial": string(text)}
	return secretError(string(text))
}

type textInterface interface{ UnmarshalText([]byte) error }

func TestCustomFirstConcreteKinds(t *testing.T) {
	t.Parallel()
	type cfg struct {
		Labels  labels        `env:"LABELS" binding:"required,nonempty"`
		Pair    pair          `env:"PAIR"`
		Complex customComplex `env:"ANY"`
		Uintptr customUintptr `env:"ANY"`
		Func    customFunc    `env:"RAW" binding:"notrim"`
		Chan    customChan    `env:"RAW" binding:"notrim"`
		Whole   wholeSlice    `env:"RAW" binding:"notrim" separator:";"`
		Pointer *wholeSlice   `env:"RAW" binding:"notrim"`
		Time    time.Time     `env:"TIME"`
		Addr    netip.Addr    `env:"ADDR"`
	}
	c, err := env.Load[cfg](source(map[string]string{"LABELS": `{"7":"seven"}`, "PAIR": "[1,2]", "ANY": "custom", "RAW": " a, b ", "TIME": "2026-10-21T01:02:03Z", "ADDR": "127.0.0.1"}))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Labels, labels{7: "seven"}) || c.Pair != (pair{1, 2}) || c.Complex != 1+2i || c.Uintptr != 42 || c.Func() != " a, b " || <-c.Chan != " a, b " || !reflect.DeepEqual(c.Whole, wholeSlice{" a, b "}) || c.Pointer == nil || (*c.Pointer)[0] != " a, b " || c.Time.Format(time.RFC3339) != "2026-10-21T01:02:03Z" || c.Addr.String() != "127.0.0.1" {
		t.Fatalf("Load(custom kinds) = %+v", c)
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[labels](), reflect.TypeFor[pair](), reflect.TypeFor[customComplex](), reflect.TypeFor[wholeSlice]()} {
		for _, binding := range []string{"optional", "required", "nonempty", "notrim"} {
			err := env.Parse(dynamicConfig(typ, `env:"V" binding:"`+binding+`"`), env.WithFiles("missing.env"))
			if !errors.Is(err, env.ErrSourceNotFound) {
				t.Errorf("Parse(custom %s binding %s) = %v, want valid schema then missing source", typ, binding, err)
			}
		}
	}
}

func TestCustomShapesRemainRestricted(t *testing.T) {
	t.Parallel()
	for _, typ := range []reflect.Type{reflect.TypeFor[*labels](), reflect.TypeFor[*pair](), reflect.TypeFor[**wholeSlice](), reflect.TypeFor[[]*labels](), reflect.TypeFor[textInterface](), reflect.TypeFor[map[int]string](), reflect.TypeFor[[2]int](), reflect.TypeFor[*[]string]()} {
		err := env.Parse(dynamicConfig(typ, `env:"V"`), env.WithFiles("missing.env"))
		if !errors.Is(err, env.ErrUnsupportedType) || !errors.Is(err, env.ErrInvalidSchema) || errors.Is(err, env.ErrSource) {
			t.Errorf("Parse(restricted %s) = %v, want unsupported before I/O", typ, err)
		}
	}
	_, err := env.Load[labels](source(nil))
	if !errors.Is(err, env.ErrInvalidOption) {
		t.Errorf("Load(custom root map) = %v", err)
	}
}

func TestCustomEmptyAndPrivacy(t *testing.T) {
	t.Parallel()
	c, err := env.Load[struct {
		Whole wholeSlice `env:"V"`
	}](source(map[string]string{"V": ""}))
	if err != nil || !reflect.DeepEqual(c.Whole, wholeSlice{""}) {
		t.Fatalf("Load(custom empty) = %+v, %v", c, err)
	}
	for _, binding := range []string{"optional", "nonempty"} {
		type config struct {
			Value failingMap `env:"V"`
		}
		old := config{Value: failingMap{"old": "preserved"}}
		dst := old
		err := env.Parse(&dst, source(map[string]string{"V": "CANARY_MAP_SECRET"}))
		var raw secretError
		if !errors.Is(err, env.ErrInvalidValue) || errors.As(err, &raw) || !reflect.DeepEqual(dst, old) {
			t.Fatalf("Parse(custom map failure) = %+v, %v", dst, err)
		}
		assertPrivate(t, err, "CANARY_MAP_SECRET")
		err = env.Parse(dynamicConfig(reflect.TypeFor[unexpectedDecoder](), `env:"V" binding:"`+binding+`"`), source(nil))
		if err != nil {
			t.Errorf("Parse(absent %s custom) = %v", binding, err)
		}
	}
	err = env.Parse(dynamicConfig(reflect.TypeFor[unexpectedDecoder](), `env:"V" binding:"nonempty"`), source(map[string]string{"V": ""}))
	if !errors.Is(err, env.ErrEmptyValue) {
		t.Errorf("Parse(nonempty before custom) = %v", err)
	}
}
