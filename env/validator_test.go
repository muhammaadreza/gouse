package env_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/muhammaadreza/gouse/env"
)

type validatedConfig struct {
	Value   string `env:"VALUE" fallback:"default"`
	Numbers []int  `env:"NUMBERS"`
	Child   *struct {
		Value string `env:"CHILD"`
	}
}

func (c validatedConfig) Validate() error {
	if c.Value != "default" {
		return validationCause("value must be default")
	}
	return nil
}

func (c *validatedConfig) Normalize() error { c.Value += "!"; return nil }

type validationCause string

func (e validationCause) Error() string { return string(e) }

func TestValidatorParentAndCommit(t *testing.T) {
	t.Parallel()
	for _, opt := range []env.Option{
		env.WithValidator(validatedConfig.Validate),
		env.WithValidator(func(c validatedConfig) error { return c.Validate() }),
		env.WithValidator(func(c *validatedConfig) error { return c.Validate() }),
	} {
		c, err := env.Load[validatedConfig](source(nil), opt)
		if err != nil || c.Value != "default" || c.Child == nil {
			t.Fatalf("Load(parent validator) = %+v, %v", c, err)
		}
	}
	c, err := env.Load[validatedConfig](source(nil), env.WithValidator((*validatedConfig).Normalize))
	if err != nil || c.Value != "default!" {
		t.Fatalf("Load(pointer method) = %+v, %v", c, err)
	}
	old := validatedConfig{Value: "old", Numbers: []int{9}, Child: &struct {
		Value string `env:"CHILD"`
	}{Value: "old"}}
	for _, fail := range []bool{false, true} {
		dst := old
		calls := 0
		cause := validationCause("application message with CANARY_APPLICATION_VALUE")
		opt := env.WithValidator(func(candidate *validatedConfig) error {
			calls++
			if candidate == &dst || candidate.Child == old.Child || &candidate.Numbers[0] == &old.Numbers[0] {
				t.Fatal("validator received old destination")
			}
			if candidate.Value != "new" || candidate.Child.Value != "child" || !reflect.DeepEqual(candidate.Numbers, []int{1, 2}) {
				t.Fatalf("candidate not decoded: %+v", candidate)
			}
			candidate.Value = "normalized"
			candidate.Numbers[0] = 3
			candidate.Child.Value = "normalized child"
			if fail {
				return cause
			}
			return nil
		})
		err := env.Parse(&dst, source(map[string]string{"VALUE": "new", "NUMBERS": "1,2", "CHILD": "child"}), opt)
		if calls != 1 {
			t.Fatalf("Parse(validator) calls = %d, want 1", calls)
		}
		if fail {
			if !errors.Is(err, env.ErrValidation) || !errors.Is(err, cause) || !reflect.DeepEqual(dst, old) || dst.Child != old.Child || &dst.Numbers[0] != &old.Numbers[0] {
				t.Fatalf("Parse(rejected candidate) = %+v, %v", dst, err)
			}
		} else if err != nil || dst.Value != "normalized" || dst.Numbers[0] != 3 || dst.Child.Value != "normalized child" {
			t.Fatalf("Parse(mutated candidate) = %+v, %v", dst, err)
		}
	}
	// Value callbacks can mutate referenced data on the new candidate too.
	c, err = env.Load[validatedConfig](source(map[string]string{"NUMBERS": "1"}), env.WithValidator(func(c validatedConfig) error { c.Numbers[0] = 4; return nil }))
	if err != nil || c.Numbers[0] != 4 {
		t.Fatalf("Load(value mutation) = %+v, %v", c, err)
	}
}

func TestValidatorOriginalErrorChain(t *testing.T) {
	t.Parallel()
	first := errors.New("env: application owns this message")
	typed := validationCause("second application cause")
	for _, cause := range []error{first, typed, errors.Join(first, typed)} {
		cfg, err := env.Load[validatedConfig](source(nil), env.WithValidator(func(validatedConfig) error { return cause }))
		if !errors.Is(err, env.ErrValidation) || !errors.Is(err, cause) || err.Error() != cause.Error() || !reflect.DeepEqual(cfg, validatedConfig{}) {
			t.Fatalf("Load(validator cause) = %+v, %v, want zero and unchanged cause", cfg, err)
		}
		if errors.Is(cause, first) && !errors.Is(err, first) {
			t.Error("validator lost joined sentinel")
		}
		if errors.Is(cause, typed) {
			var got validationCause
			if !errors.As(err, &got) || got != typed {
				t.Error("validator lost custom type")
			}
		}
		var field *env.FieldError
		if errors.As(err, &field) || errors.Is(err, env.ErrInvalidValue) {
			t.Errorf("validator incorrectly classified: %v", err)
		}
	}
}

func TestValidatorParentOwnsChildStrategy(t *testing.T) {
	t.Parallel()
	a, b := errors.New("child a"), errors.New("child b")
	for _, collect := range []bool{false, true} {
		calls := []string{}
		childA := func() error { calls = append(calls, "a"); return a }
		childB := func() error { calls = append(calls, "b"); return b }
		_, err := env.Load[validatedConfig](source(nil), env.WithValidator(func(validatedConfig) error {
			if collect {
				return errors.Join(childA(), childB())
			}
			if err := childA(); err != nil {
				return err
			}
			return childB()
		}))
		want := []string{"a"}
		if collect {
			want = append(want, "b")
		}
		if !errors.Is(err, env.ErrValidation) || !errors.Is(err, a) || errors.Is(err, b) != collect || !reflect.DeepEqual(calls, want) {
			t.Errorf("Load(child strategy %v) = %v, calls %v, want %v", collect, err, calls, want)
		}
	}
}
