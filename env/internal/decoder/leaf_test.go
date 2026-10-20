package decoder_test

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

func TestPointerLeaves(t *testing.T) {
	t.Parallel()
	type config struct {
		String   *string        `env:"STRING"`
		Int      *int           `env:"INT"`
		Duration *time.Duration `env:"DURATION"`
	}
	for _, values := range []map[string]string{nil} {
		cfg := mustDecode(t, reflect.TypeFor[config](), "", lookup(values)).Interface().(config)
		if !reflect.DeepEqual(cfg, config{}) {
			t.Errorf("Decode(blank pointers) = %+v; want zero configuration", cfg)
		}
	}
	cfg := mustDecode(t, reflect.TypeFor[config](), "", lookup(map[string]string{"STRING": " hello ", "INT": "0", "DURATION": "1s"})).Interface().(config)
	if cfg.String == nil || *cfg.String != "hello" || cfg.Int == nil || *cfg.Int != 0 || cfg.Duration == nil || *cfg.Duration != time.Second {
		t.Errorf("Decode(selected pointers) = %+v; want trimmed string, zero integer, and one second", cfg)
	}
	type fallbackConfig struct {
		Value *int `env:"V" fallback:"7"`
	}
	fallback := mustDecode(t, reflect.TypeFor[fallbackConfig](), "", lookup(nil)).Interface().(fallbackConfig)
	if fallback.Value == nil || *fallback.Value != 7 {
		t.Errorf("Decode(pointer fallback) = %+v; want pointer to 7", fallback)
	}
	candidate, diagnostics := decodeType(reflect.TypeFor[config](), "", lookup(map[string]string{"INT": "bad"}))
	if candidate.IsValid() || !hasCategory(diagnostics, decoder.InvalidValue) {
		t.Errorf("Decode(bad pointer) = %v, %+v; want invalid candidate and invalid value", candidate, diagnostics)
	}
}

func TestScalarConversions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, text string
		want       any
	}{
		{"string whitespace", " \n a  b\tc \u2003", "a  b\tc"},
		{"bool false", "false", false}, {"bool true", "true", true},
		{"bool 1", "1", true}, {"bool 0", "0", false},
		{"bool t", "t", true}, {"bool f", "f", false},
		{"bool T", "T", true}, {"bool F", "F", false},
		{"bool TRUE", "TRUE", true}, {"bool FALSE", "FALSE", false},
		{"bool True", "True", true}, {"bool False", "False", false},
		{"int", "-27", int(-27)}, {"int8 min", "-128", int8(-128)}, {"int8 max", "127", int8(127)},
		{"int16", "-32768", int16(-32768)}, {"int32", "2147483647", int32(2147483647)},
		{"int64", "-9223372036854775808", int64(math.MinInt64)},
		{"uint", "27", uint(27)}, {"uint8", "255", uint8(255)},
		{"uint16", "65535", uint16(65535)}, {"uint32", "4294967295", uint32(math.MaxUint32)},
		{"uint64", "18446744073709551615", uint64(math.MaxUint64)},
		{"decimal leading zero", "012", 12}, {"signed plus", "+12", 12},
		{"float32", "1.25", float32(1.25)}, {"float64", "-1.25e2", float64(-125)},
		{"duration compound", "1h30m", 90 * time.Minute}, {"duration zero", "0", time.Duration(0)},
		{"duration ms", "500ms", 500 * time.Millisecond}, {"duration negative", "-30s", -30 * time.Second},
		{"named string", "hello", namedString("hello")}, {"named bool", "true", namedBool(true)},
		{"named int", "42", namedInt(42)}, {"named uint", "42", namedUint(42)},
		{"named float", "1.5", namedFloat(1.5)}, {"distinct duration integer", "12", distinctDuration(12)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			typ := dynamicType(reflect.TypeOf(tt.want), `env:"V" fallback:"unused invalid"`)
			values := map[string]string{"V": tt.text}
			if tt.name == "fallback" {
				values = nil
			}
			candidate := mustDecode(t, typ, "", lookup(values))
			got := candidate.Field(0).Interface()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decode(%q, %T) = %v, want %v", tt.text, tt.want, got, tt.want)
			}
		})
	}
}

func TestInvalidScalarConversions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		typ    reflect.Type
		values []string
	}{
		{"bool", reflect.TypeFor[bool](), []string{"yes", "no", "on", "off", "TrUe", "2"}},
		{"int", reflect.TypeFor[int](), []string{"9223372036854775808", "-9223372036854775809", "0x10", "1_000", "1.0"}},
		{"int8", reflect.TypeFor[int8](), []string{"128", "-129"}},
		{"int16", reflect.TypeFor[int16](), []string{"32768", "-32769"}},
		{"int32", reflect.TypeFor[int32](), []string{"2147483648", "-2147483649"}},
		{"int64", reflect.TypeFor[int64](), []string{"9223372036854775808", "-9223372036854775809"}},
		{"uint", reflect.TypeFor[uint](), []string{"-1", "18446744073709551616", "+1", "0x10"}},
		{"uint8", reflect.TypeFor[uint8](), []string{"256", "-1"}},
		{"uint16", reflect.TypeFor[uint16](), []string{"65536", "-1"}},
		{"uint32", reflect.TypeFor[uint32](), []string{"4294967296", "-1"}},
		{"uint64", reflect.TypeFor[uint64](), []string{"18446744073709551616", "-1"}},
		{"float32", reflect.TypeFor[float32](), []string{"NaN", "+Inf", "-Inf", "Infinity", "1e39", "-1e39", "bad"}},
		{"float64", reflect.TypeFor[float64](), []string{"NaN", "+Inf", "-Inf", "Infinity", "1e309", "-1e309", "bad"}},
		{"duration", reflect.TypeFor[time.Duration](), []string{"1d", "30", "9223372036854775808ns"}},
		{"distinct duration", reflect.TypeFor[distinctDuration](), []string{"1s"}},
	}
	for _, tt := range tests {
		for _, text := range tt.values {
			t.Run(tt.name+"/"+text, func(t *testing.T) {
				t.Parallel()
				candidate, diagnostics := decodeType(dynamicType(tt.typ, `env:"V"`), "", lookup(map[string]string{"V": text}))
				if !hasCategory(diagnostics, decoder.InvalidValue) || candidate.IsValid() {
					t.Errorf("Decode(%q, %s) = %v, want InvalidValue", text, tt.typ, diagnostics)
				}
			})
		}
	}
}

func TestSliceConversions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, text, tags string
		want             any
	}{
		{"empty strings", "a, ,c", "", []string{"a", "", "c"}},
		{"edge empty strings", ", a,", "", []string{"", "a", ""}},
		{"only separators required", ",", `binding:"required"`, []string{"", ""}},
		{"no csv", `"a,b",c`, "", []string{`"a`, `b"`, "c"}},
		{"semicolon", "1; 2 ;3", `separator:";"`, []int{1, 2, 3}},
		{"multi character", " a :: b :: ", `separator:"::"`, []string{"a", "b", ""}},
		{"fallback", "", `fallback:" 1; 2 " separator:";"`, []int{1, 2}},
		{"bool", "true, 0, FALSE", "", []bool{true, false, false}},
		{"duration", "1s, 500ms", "", []time.Duration{time.Second, 500 * time.Millisecond}},
		{"named elements", "1, 2", "", []namedInt{1, 2}},
		{"unsigned", "1,2", "", []uint{1, 2}},
		{"float", "1.5, 2", "", []float64{1.5, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			typ := dynamicType(reflect.TypeOf(tt.want), `env:"V" `+tt.tags)
			values := map[string]string{"V": tt.text}
			if tt.name == "fallback" {
				values = nil
			}
			candidate := mustDecode(t, typ, "", lookup(values))
			got := candidate.Field(0).Interface()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decode(slice %q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
	for _, values := range []map[string]string{nil, {"V": "  "}} {
		typ := reflect.TypeFor[struct {
			Values []string `env:"V"`
		}]()
		candidate := mustDecode(t, typ, "", lookup(values))
		if candidate.Field(0).IsNil() != (values == nil) || candidate.Field(0).Len() != 0 {
			t.Errorf("Decode(absent/blank slice) = %v; want nil only when absent", candidate)
		}
	}
}

func TestCustomDecoding(t *testing.T) {
	t.Parallel()
	type config struct {
		Struct       customText    `env:"STRUCT"`
		Pointer      *customText   `env:"POINTER"`
		Int          customInt     `env:"INT"`
		Ints         []customInt   `env:"INTS"`
		Structs      []customText  `env:"STRUCTS"`
		Slice        customSlice   `env:"SLICE" separator:";"`
		PointerSlice *customSlice  `env:"POINTER_SLICE"`
		Slices       []customSlice `env:"SLICES"`
		Value        valueReceiver `env:"VALUE"`
		Container    customText    // No env: remains a container despite custom method.
	}
	candidate := mustDecode(t, reflect.TypeFor[config](), "", lookup(map[string]string{
		"STRUCT": " hello ", "POINTER": "world", "INT": "custom", "INTS": "custom,custom",
		"STRUCTS": "a,,b", "SLICE": "a,b", "POINTER_SLICE": "raw", "SLICES": "a,b", "VALUE": "accepted",
	}))
	cfg := candidate.Interface().(config)
	if len(cfg.Slices) != 2 || cfg.Slices[0][0]["text"] != "a" || cfg.Slices[1][0]["text"] != "b" {
		t.Errorf("Decode(custom slice elements) = %+v", cfg.Slices)
	}
	if cfg.Struct.Text != "custom:hello" || cfg.Pointer == nil || cfg.Pointer.Text != "custom:world" || cfg.Int != 99 || !reflect.DeepEqual(cfg.Ints, []customInt{99, 99}) || !reflect.DeepEqual(cfg.Structs, []customText{{Text: "custom:a"}, {Text: "custom:"}, {Text: "custom:b"}}) || cfg.Slice[0]["text"] != "a,b" || cfg.PointerSlice == nil || (*cfg.PointerSlice)[0]["text"] != "raw" || cfg.Container.Text != "" {
		t.Errorf("Decode(custom) = %+v, want custom decoded values", cfg)
	}
	empty := mustDecode(t, reflect.TypeFor[config](), "", lookup(nil)).Interface().(config)
	if empty.Pointer != nil || empty.Slice != nil || empty.PointerSlice != nil || empty.Struct.Text != "" {
		t.Errorf("Decode(absent custom) = %+v", empty)
	}

	type fallbackConfig struct {
		Value *customText `env:"V" fallback:" fallback text "`
	}
	fallback := mustDecode(t, reflect.TypeFor[fallbackConfig](), "", lookup(nil)).Interface().(fallbackConfig)
	if fallback.Value == nil || fallback.Value.Text != "custom:fallback text" {
		t.Errorf("Decode(custom fallback) = %+v", fallback)
	}
}

func TestCustomFailureDiagnostics(t *testing.T) {
	t.Parallel()
	type config struct {
		Custom  rejecting   `env:"V"`
		Slice   []customInt `env:"SLICE"`
		Hostile hostile     `env:"HOSTILE"`
	}
	candidate, fields := decodeType(reflect.TypeFor[config](), "", lookup(map[string]string{"V": "secret", "SLICE": "bad,,custom,bad", "HOSTILE": "secret"}))
	if candidate.IsValid() || len(fields) != 5 || fields[1].Index != 0 || fields[2].Index != 1 || fields[3].Index != 3 || fields[4].Path != "Hostile" {
		t.Fatalf("Decode(custom failures) = %+v; want invalid candidate and ordered field/element diagnostics", fields)
	}
	assertDiagnosticPrivacy(t, fields, "secret", "bad,,custom,bad")
	plan, diagnostics := decoder.Compile(dynamicType(reflect.TypeFor[customMap](), `env:"V"`), "")
	if plan == nil || len(diagnostics) != 0 {
		t.Errorf("Compile(custom map) = %v, %+v; want valid custom leaf", plan, diagnostics)
	}
}
