package decoder

import (
	"encoding"
	"math"
	"reflect"
	"strconv"
	"time"
)

type leafDecoder struct {
	kind   reflect.Kind
	custom bool
	elem   *leafDecoder
}

func hasTextDecoder(t reflect.Type) bool {
	text := reflect.TypeFor[encoding.TextUnmarshaler]()
	return t.Implements(text) || (t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(text))
}

func newLeafDecoder(t reflect.Type) *leafDecoder {
	kind := t.Kind()
	if kind == reflect.Pointer {
		// A single pointer may refer to a scalar or custom leaf, never a
		// built-in aggregate. Named pointer types have no methods of their own.
		switch t.Elem().Kind() {
		case reflect.Pointer, reflect.Map, reflect.Array:
			// Custom map/array support does not extend collection pointers.
			return nil
		}
		elem := newLeafDecoder(t.Elem())
		if elem == nil || (elem.kind == reflect.Slice && !elem.custom) {
			return nil
		}
		return &leafDecoder{kind: kind, elem: elem}
	}
	// Interface fields cannot identify a concrete value to allocate. All other
	// concrete non-pointer kinds may opt in through TextUnmarshaler.
	if kind == reflect.Interface {
		return nil
	}
	if hasTextDecoder(t) {
		return &leafDecoder{kind: kind, custom: true}
	}
	switch kind {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.Slice:
	default:
		return nil
	}

	if kind == reflect.Slice {
		if t.Elem().Kind() == reflect.Pointer {
			return nil
		}
		// Reject nested built-in slices before recursing: a defined slice
		// may contain itself (type Recursive []Recursive).
		if t.Elem().Kind() == reflect.Slice && !hasTextDecoder(t.Elem()) {
			return nil
		}
		elem := newLeafDecoder(t.Elem())
		if elem == nil {
			return nil
		}
		return &leafDecoder{kind: kind, elem: elem}
	}
	return &leafDecoder{kind: kind}
}

// decode deliberately returns only success/failure. Parser/custom errors never
// escape into diagnostics, even when a custom error's Error method is unsafe.
func (d *leafDecoder) decode(dst reflect.Value, text string) bool {
	if d.kind == reflect.Pointer {
		dst.Set(reflect.New(dst.Type().Elem()))
		return d.elem.decode(dst.Elem(), text)
	}
	if d.custom {
		// Every staged leaf is addressable. Its pointer method set includes
		// both value-receiver and pointer-receiver UnmarshalText methods.
		if decoder, ok := dst.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return decoder.UnmarshalText([]byte(text)) == nil
		}
		return false
	}
	if dst.Type() == reflect.TypeFor[time.Duration]() {
		value, err := time.ParseDuration(text)
		if err != nil {
			return false
		}
		dst.SetInt(int64(value))
		return true
	}
	switch d.kind {
	case reflect.String:
		dst.SetString(text)
	case reflect.Bool:
		value, err := strconv.ParseBool(text)
		if err != nil {
			return false
		}
		dst.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(text, 10, dst.Type().Bits())
		if err != nil {
			return false
		}
		dst.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(text, 10, dst.Type().Bits())
		if err != nil {
			return false
		}
		dst.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(text, dst.Type().Bits())
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		dst.SetFloat(value)
	default:
		return false
	}
	return true
}
