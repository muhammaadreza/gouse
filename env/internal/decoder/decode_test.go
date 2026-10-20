package decoder_test

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

func TestResolutionMatrix(t *testing.T) {
	t.Parallel()
	inputs := []struct {
		name, text string
		found      bool
	}{
		{"absent", "ignored even if nonempty", false}, {"empty", "", true},
		{"blank", " \t\n\u2003 ", true}, {"valid", " 42 ", true},
		{"zero", "0", true}, {"invalid", "bad", true},
	}
	for _, binding := range []string{"", "optional", "required"} {
		for _, fallback := range []string{"", " \t ", " 7 ", "bad fallback"} {
			for _, input := range inputs {
				name := fmt.Sprintf("%s/%q/%s", binding, fallback, input.name)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					tags := `env:"V" fallback:` + strconv.Quote(fallback)
					if binding != "" {
						tags += ` binding:` + strconv.Quote(binding)
					}
					typ := dynamicType(reflect.TypeFor[int](), tags)
					candidate, diagnostics := decodeType(typ, "", func(string) (string, bool) { return input.text, input.found })
					var wantCategory decoder.Category
					want := 0
					switch {
					case binding == "required":
						wantCategory = decoder.InvalidSchema // required + fallback is invalid, even unused.
					case !input.found:
						switch fallback {
						case " 7 ":
							want = 7
						default:
							wantCategory = decoder.InvalidValue
						}
					case input.name == "valid":
						want = 42
					case input.name == "zero":
					default:
						wantCategory = decoder.InvalidValue
					}
					if (wantCategory == 0 && len(diagnostics) != 0) || (wantCategory != 0 && !hasCategory(diagnostics, wantCategory)) {
						t.Fatalf("Decode(resolution matrix) = %v, want %v", diagnostics, wantCategory)
					}
					if len(diagnostics) > 0 && candidate.IsValid() {
						t.Fatal("Decode(failure) returned partial candidate")
					}
					if len(diagnostics) == 0 && candidate.Field(0).Int() != int64(want) {
						t.Errorf("Decode(resolution) = %v, want %d", candidate, want)
					}
				})
			}
		}
	}
}

func TestNestedContainers(t *testing.T) {
	t.Parallel()
	type leaf struct {
		Value string `env:"VALUE"`
	}
	type database struct {
		Read  *leaf `prefix:"READ_"`
		Write leaf  `prefix:"WRITE_"`
		Plain leaf
	}
	type config struct {
		DB      *database `prefix:"DB_"`
		Sibling leaf      `prefix:"SIB_"`
		Empty   *struct{}
	}
	calls := []string{}
	candidate := mustDecode(t, reflect.TypeFor[config](), "APP_", func(key string) (string, bool) {
		calls = append(calls, key)
		return key, true
	})
	cfg := candidate.Interface().(config)
	wantCalls := []string{"APP_DB_READ_VALUE", "APP_DB_WRITE_VALUE", "APP_DB_VALUE", "APP_SIB_VALUE"}
	if !reflect.DeepEqual(calls, wantCalls) || cfg.DB == nil || cfg.DB.Read == nil || cfg.Empty == nil {
		t.Fatalf("Decode(nested prefixes) = %+v, keys %v", cfg, calls)
	}
	if cfg.DB.Read.Value != wantCalls[0] || cfg.DB.Write.Value != wantCalls[1] || cfg.DB.Plain.Value != wantCalls[2] || cfg.Sibling.Value != wantCalls[3] {
		t.Errorf("Decode(nested values) = %+v", cfg)
	}
	cfg = mustDecode(t, reflect.TypeFor[config](), "", lookup(nil)).Interface().(config)
	if cfg.DB == nil || cfg.DB.Read == nil || cfg.Empty == nil || cfg.DB.Read.Value != "" {
		t.Errorf("Decode(absent nested) = %+v, want allocated zero containers", cfg)
	}
	type required struct{ Child *Embedded }
	failed, diagnostics := decodeType(reflect.TypeFor[required](), "", lookup(nil))
	if !hasCategory(diagnostics, decoder.Required) || failed.IsValid() {
		t.Errorf("Decode(required nested) = %+v, %v, want invalid candidate plus Required", failed, diagnostics)
	}
}

func TestLookupCache(t *testing.T) {
	t.Parallel()
	type config struct {
		Text    string `env:"DUP"`
		Number  int    `env:"DUP"`
		Missing *int   `env:"MISSING"`
		Default int    `env:"MISSING" fallback:"5"`
		Nested  struct {
			Text string `env:"DUP"`
		}
	}
	calls := map[string]int{}
	text := "12"
	lookup := func(key string) (string, bool) {
		calls[key]++
		return text, key == "DUP"
	}
	plan, diagnostics := decoder.Compile(reflect.TypeFor[config](), "")
	if len(diagnostics) != 0 {
		t.Fatalf("Compile(cache schema) = %+v", diagnostics)
	}
	firstValue, diagnostics := plan.Decode(lookup)
	if len(diagnostics) != 0 {
		t.Fatalf("Decode(cache first) = %+v", diagnostics)
	}
	first := firstValue.Interface().(config)
	if first.Text != "12" || first.Number != 12 || first.Nested.Text != "12" || first.Missing != nil || first.Default != 5 || calls["DUP"] != 1 || calls["MISSING"] != 1 {
		t.Fatalf("Decode(cache first) = %+v, %v, calls %v", first, diagnostics, calls)
	}
	text = "24"
	secondValue, diagnostics := plan.Decode(lookup)
	if len(diagnostics) != 0 {
		t.Fatalf("Decode(cache second) = %+v", diagnostics)
	}
	second := secondValue.Interface().(config)
	if second.Number != 24 || calls["DUP"] != 2 || calls["MISSING"] != 2 {
		t.Errorf("Decode(cache second) = %+v, %v, calls %v", second, diagnostics, calls)
	}
	text = "not integer"
	failed, fields := plan.Decode(lookup)
	if failed.IsValid() || len(fields) != 1 || fields[0].Path != "Number" || calls["DUP"] != 3 || calls["MISSING"] != 3 {
		t.Errorf("Decode(duplicate decoding) = %v, calls %v", fields, calls)
	}
}

func TestValueErrorOrder(t *testing.T) {
	t.Parallel()
	type config struct {
		Missing string `env:"MISSING" binding:"required"`
		Nested  struct {
			Ports []int `env:"PORTS"`
			Last  bool  `env:"LAST"`
		} `prefix:"DB_"`
		Last int `env:"LAST"`
	}
	values := map[string]string{"DB_PORTS": "1, bad, , 4, bad", "DB_LAST": "bad", "LAST": "bad"}
	for range 20 {
		candidate, fields := decodeType(reflect.TypeFor[config](), "", lookup(values))
		var got []string
		for _, field := range fields {
			got = append(got, fmt.Sprintf("%s/%s/%d", field.Path, field.Key, field.Index))
		}
		want := []string{"Missing/MISSING/-1", "Nested.Ports/DB_PORTS/1", "Nested.Ports/DB_PORTS/2", "Nested.Ports/DB_PORTS/4", "Nested.Last/DB_LAST/-1", "Last/LAST/-1"}
		if !reflect.DeepEqual(got, want) || candidate.IsValid() || !hasCategory(fields, decoder.Required) || !hasCategory(fields, decoder.InvalidValue) {
			t.Fatalf("Decode(value errors) = %v, %v, want %v", got, fields, want)
		}
		for _, field := range fields {
			if field.Category != decoder.Required && field.Category != decoder.InvalidValue {
				t.Errorf("Decode diagnostic has invalid category: %+v", field)
			}
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[[]bool](), reflect.TypeFor[[]time.Duration](), reflect.TypeFor[[]float64]()} {
		candidate, fields := decodeType(dynamicType(typ, `env:"V"`), "", lookup(map[string]string{"V": ",bad,"}))
		if candidate.IsValid() || len(fields) != 3 || fields[0].Index != 0 || fields[1].Index != 1 || fields[2].Index != 2 {
			t.Errorf("Decode(%s errors) = %v, want 3 indexed errors", typ, fields)
		}
	}
}

func TestDecodeReturnsOnlySafeDiagnosticsOnFailure(t *testing.T) {
	t.Parallel()
	type config struct {
		Number   int             `env:"NUMBER"`
		Missing  string          `env:"MISSING" binding:"required"`
		Custom   engineRejecting `env:"CUSTOM"`
		Fallback int             `env:"FALLBACK" fallback:"CANARY_ENGINE_FALLBACK"`
	}
	plan, diagnostics := decoder.Compile(reflect.TypeFor[config](), "")
	if plan == nil || len(diagnostics) != 0 {
		t.Fatalf("Compile(valid schema) = %v, %+v; want Plan without diagnostics", plan, diagnostics)
	}
	value, diagnostics := plan.Decode(func(key string) (string, bool) {
		return "CANARY_ENGINE_VALUE", key == "NUMBER" || key == "CUSTOM"
	})
	want := []decoder.Diagnostic{
		{Path: "Number", Key: "NUMBER", Index: -1, Category: decoder.InvalidValue},
		{Path: "Missing", Key: "MISSING", Index: -1, Category: decoder.Required},
		{Path: "Custom", Key: "CUSTOM", Index: -1, Category: decoder.InvalidValue},
		{Path: "Fallback", Key: "FALLBACK", Index: -1, Category: decoder.InvalidValue},
	}
	if value.IsValid() || !reflect.DeepEqual(diagnostics, want) {
		t.Fatalf("Decode(invalid values) = %v, %+v; want invalid candidate, %+v", value, diagnostics, want)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", diagnostics, diagnostics), "CANARY_ENGINE_") {
		t.Fatal("Decode diagnostics retain selected or fallback text")
	}
}

func TestPlanHasIndependentCandidatesAndCaches(t *testing.T) {
	t.Parallel()
	type child struct {
		Number int `env:"NUMBER"`
	}
	type config struct {
		Child   *child
		Numbers []int `env:"NUMBER"`
		Default int   `env:"MISSING" fallback:"5"`
		Other   int   `env:"MISSING" fallback:"7"`
	}
	plan, diagnostics := decoder.Compile(reflect.TypeFor[config](), "")
	if plan == nil || len(diagnostics) != 0 {
		t.Fatalf("Compile(valid schema) = %v, %+v", plan, diagnostics)
	}
	for number := range 16 {
		t.Run(fmt.Sprintf("call %d", number), func(t *testing.T) {
			t.Parallel()
			for range 2 {
				calls := make(map[string]int)
				candidate, diagnostics := plan.Decode(func(key string) (string, bool) {
					calls[key]++
					return fmt.Sprint(number), key == "NUMBER"
				})
				if len(diagnostics) != 0 || !candidate.IsValid() {
					t.Fatalf("Decode(shared Plan) = %v, %+v", candidate, diagnostics)
				}
				got := candidate.Interface().(config)
				want := config{Child: &child{Number: number}, Numbers: []int{number}, Default: 5, Other: 7}
				if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(calls, map[string]int{"NUMBER": 1, "MISSING": 1}) {
					t.Fatalf("Decode(shared Plan) = %+v, calls %v; want %+v with one hit and miss", got, calls, want)
				}
				got.Child.Number = -1
				got.Numbers[0] = -1
			}
		})
	}
}
