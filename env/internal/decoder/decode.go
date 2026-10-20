package decoder

import (
	"reflect"
	"strings"
)

type lookupResult struct {
	value string
	found bool
}

type valueParser struct {
	lookup      func(string) (string, bool)
	cache       map[string]lookupResult
	diagnostics []Diagnostic
}

// Decode constructs a fresh candidate from a successfully compiled Plan and
// a non-nil prepared lookup. Each key, including a miss, is cached once per call.
// Failure returns an invalid reflect.Value and ordered, safe diagnostics.
// Independent calls may share a Plan; custom code must provide its own safety.
func (p *Plan) Decode(lookup func(string) (string, bool)) (reflect.Value, []Diagnostic) {
	candidate := reflect.New(p.typ).Elem()
	parser := valueParser{lookup: lookup, cache: make(map[string]lookupResult)}
	parser.walk(candidate, p.fields)
	if len(parser.diagnostics) > 0 {
		return reflect.Value{}, parser.diagnostics
	}
	return candidate, nil
}

func (p *valueParser) walk(dst reflect.Value, fields []fieldPlan) {
	for i := range fields {
		field := &fields[i]
		value := dst.Field(field.field)
		if field.leaf == nil {
			if value.Kind() == reflect.Pointer {
				value.Set(reflect.New(value.Type().Elem()))
				value = value.Elem()
			}
			p.walk(value, field.children)
			continue
		}
		text, selected, category := p.selectText(field)
		if category != 0 {
			p.diagnostics = append(p.diagnostics, diagnostic(field.path, field.key, -1, category))
			continue
		}
		if !selected {
			continue
		}
		if field.leaf.kind == reflect.Slice && !field.leaf.custom {
			p.decodeSlice(value, field, text)
			continue
		}
		if !field.leaf.decode(value, text) {
			p.diagnostics = append(p.diagnostics, diagnostic(field.path, field.key, -1, InvalidValue))
		}
	}
}

// selectText separates a skipped optional field from selected text (including
// empty text) and a binding failure. Only selected text reaches a decoder.
func (p *valueParser) selectText(field *fieldPlan) (text string, selected bool, category Category) {
	result, cached := p.cache[field.key]
	if !cached {
		result.value, result.found = p.lookup(field.key)
		p.cache[field.key] = result
	}
	if !result.found && field.required {
		return "", false, Required
	}
	if !result.found && !field.hasFallback {
		return "", false, 0
	}
	text = result.value
	if !result.found {
		text = field.fallback
	}
	if !field.notrim {
		text = strings.TrimSpace(text)
	}
	if field.nonempty && text == "" {
		return "", false, EmptyValue
	}
	return text, true, 0
}

// decodeSlice handles only built-in slices; custom slices decode the whole text.
// Empty selected text creates a non-nil empty slice, while empty tokens in
// nonempty text remain elements and can produce indexed conversion errors.
func (p *valueParser) decodeSlice(dst reflect.Value, field *fieldPlan, text string) {
	var tokens []string
	if text != "" {
		tokens = strings.Split(text, field.separator)
	}
	dst.Set(reflect.MakeSlice(dst.Type(), len(tokens), len(tokens)))
	for i, token := range tokens {
		if !field.notrim {
			token = strings.TrimSpace(token)
		}
		if !field.leaf.elem.decode(dst.Index(i), token) {
			p.diagnostics = append(p.diagnostics, diagnostic(field.path, field.key, i, InvalidValue))
		}
	}
}
