package decoder_test

import (
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muhammaadreza/gouse/env/internal/decoder"
)

func FuzzScalar(f *testing.F) {
	for _, seed := range []string{"", " \t ", "0", "false", "127", "-129", "0x10", "NaN", "1e400", "1h30m", "1d", "canary-secret", "\xff"} {
		for kind := uint8(0); kind < 6; kind++ {
			f.Add(seed, kind)
		}
	}
	f.Fuzz(func(t *testing.T, raw string, kind uint8) {
		text := strings.TrimSpace(raw)
		var want any
		var parseErr error
		switch kind % 6 {
		case 0:
			want = text
		case 1:
			var value int64
			value, parseErr = strconv.ParseInt(text, 10, 8)
			want = int8(value)
		case 2:
			want, parseErr = strconv.ParseUint(text, 10, 64)
		case 3:
			want, parseErr = strconv.ParseBool(text)
		case 4:
			var value float64
			value, parseErr = strconv.ParseFloat(text, 64)
			want = value
			if math.IsNaN(value) || math.IsInf(value, 0) {
				parseErr = strconv.ErrRange
			}
		case 5:
			want, parseErr = time.ParseDuration(text)
		}
		typ := dynamicType(reflect.TypeOf(want), `env:"V"`)
		calls := 0
		candidate, diagnostics := decodeType(typ, "", func(string) (string, bool) { calls++; return raw, true })
		if calls != 1 {
			t.Fatalf("Decode(scalar) made %d lookups, want 1", calls)
		}
		if parseErr != nil {
			if !hasCategory(diagnostics, decoder.InvalidValue) || candidate.IsValid() {
				t.Fatalf("Decode(invalid scalar) = %+v, want invalid candidate and InvalidValue", diagnostics)
			}
			return
		}
		if len(diagnostics) != 0 || !candidate.IsValid() {
			t.Fatalf("Decode(valid scalar) = %+v", diagnostics)
		}
		got := candidate.Field(0).Interface()
		if len(diagnostics) != 0 || !reflect.DeepEqual(got, want) {
			t.Fatalf("Decode(scalar) = %v, %v, want %v", got, diagnostics, want)
		}
	})
}

func FuzzSlice(f *testing.F) {
	for _, seed := range []string{"", " ", ",", "1,2", "1,bad,,4,bad", "a, ,c", `"a,b",c`, "\xff,0"} {
		f.Add(seed, ",")
	}
	f.Add(" 1 :: 2 :: ", "::")
	f.Add("anything", "")
	f.Fuzz(func(t *testing.T, raw, separator string) {
		if len(separator) > 128 {
			t.Skip()
		}
		tags := `env:"V" separator:` + strconv.Quote(separator)
		typ := dynamicType(reflect.TypeFor[[]string](), tags)
		calls := 0
		lookup := func(string) (string, bool) { calls++; return raw, true }
		candidate, diagnostics := decodeType(typ, "", lookup)
		if separator == "" {
			if !hasCategory(diagnostics, decoder.InvalidSchema) || calls != 0 || candidate.IsValid() {
				t.Fatalf("Decode(empty separator) = %v, %d lookups", diagnostics, calls)
			}
			return
		}
		want := []string{}
		text := strings.TrimSpace(raw)
		if text != "" {
			want = strings.Split(text, separator)
			for i := range want {
				want[i] = strings.TrimSpace(want[i])
			}
		}
		if len(diagnostics) != 0 || !candidate.IsValid() {
			t.Fatalf("Decode(string slice) = %+v", diagnostics)
		}
		got := candidate.Field(0).Interface()
		if len(diagnostics) != 0 || calls != 1 || !reflect.DeepEqual(got, want) {
			t.Fatalf("Decode(string slice) = %v, %v, want %v", got, diagnostics, want)
		}
		numbers := dynamicType(reflect.TypeFor[[]int64](), tags)
		numericCandidate, fields := decodeType(numbers, "", lookup)
		var indexes []int
		for i, token := range want {
			if _, parseErr := strconv.ParseInt(token, 10, 64); parseErr != nil {
				indexes = append(indexes, i)
			}
		}
		if len(fields) != len(indexes) {
			t.Fatalf("Decode(numeric slice) = %d errors, want %d", len(fields), len(indexes))
		}
		for i, field := range fields {
			if field.Index != indexes[i] || field.Category != decoder.InvalidValue {
				t.Fatalf("Decode(numeric slice) = %+v, want index %d", field, indexes[i])
			}
		}
		if len(fields) > 0 && numericCandidate.IsValid() {
			t.Fatal("Decode(numeric slice) returned partial candidate")
		}
	})
}

func FuzzSchema(f *testing.F) {
	for _, tags := range []string{`env:"V"`, `env:""`, `env:"-"`, `env:"-" fallback:"secret"`, `binding:"required"`, `prefix:"P_"`, `separator:"" env:"V"`, `env:"V" binding:"unknown"`, `env:"V" binding:"required,nonempty"`, `env:"V" binding:"notrim"`, `env:"V" binding:"required,"`, `env:"unterminated`, `json:"unrelated"`, `env:bad`} {
		for kind := uint8(0); kind < 8; kind++ {
			f.Add(tags, kind, "bad")
		}
	}
	f.Fuzz(func(t *testing.T, tags string, kind uint8, text string) {
		// reflect.StructOf itself interns generated types. Bound fuzz tag size
		// to keep the harness's reflection cache small during longer runs.
		if len(tags) > 128 {
			t.Skip()
		}
		types := []reflect.Type{
			reflect.TypeFor[int](), reflect.TypeFor[[]int](), reflect.TypeFor[*int](),
			reflect.TypeFor[recursive](), reflect.TypeFor[*recursive](),
			reflect.TypeFor[map[string]int](), reflect.TypeFor[customText](), reflect.TypeFor[**int](),
		}
		typ := dynamicType(types[int(kind)%len(types)], tags)
		calls := 0
		candidate, diagnostics := decodeType(typ, "APP_", func(string) (string, bool) { calls++; return text, true })
		if (hasCategory(diagnostics, decoder.InvalidSchema) || hasCategory(diagnostics, decoder.UnsupportedType)) && calls != 0 {
			t.Fatalf("Decode(invalid schema) made %d lookups", calls)
		}
		if len(diagnostics) > 0 && candidate.IsValid() {
			t.Fatal("Decode(failed schema/value) returned partial candidate")
		}
		if calls > 1 {
			t.Fatalf("Decode(single leaf) made %d lookups, want at most 1", calls)
		}
	})
}

func FuzzMalformedOwnedTags(f *testing.F) {
	for key := uint8(0); key < 5; key++ {
		for mode := uint8(0); mode < 3; mode++ {
			f.Add("canary", key, mode)
		}
	}
	f.Fuzz(func(t *testing.T, text string, keyIndex, mode uint8) {
		if len(text) > 128 {
			t.Skip()
		}
		keys := []string{"env", "binding", "fallback", "separator", "prefix"}
		key := keys[int(keyIndex)%len(keys)]
		quoted := strconv.Quote("CANARY_MALFORMED_" + text)
		var entry string
		switch mode % 3 {
		case 0:
			entry = key + ":unquoted_" + quoted
		case 1:
			entry = key + ":" + quoted[:len(quoted)-1]
		case 2:
			entry = key + `:"\q` + quoted[1:]
		}
		plan, diagnostics := decoder.Compile(dynamicType(reflect.TypeFor[string](), `json:"unrelated" `+entry), "")
		if plan != nil || !hasCategory(diagnostics, decoder.InvalidSchema) {
			t.Fatalf("Compile(malformed %s, mode %d) = %v, %+v; want nil Plan and invalid schema", key, mode%3, plan, diagnostics)
		}
		assertDiagnosticPrivacy(t, diagnostics, "CANARY_MALFORMED_")
	})
}
