package decoder

import "reflect"

// Plan is an immutable compiled schema. Obtain one through Compile; its zero
// value is not usable. Decode keeps all mutable state local to each call.
type Plan struct {
	typ    reflect.Type
	fields []fieldPlan
}

type fieldPlan struct {
	field       int
	path        string
	key         string
	required    bool
	nonempty    bool
	notrim      bool
	hasFallback bool
	fallback    string
	separator   string
	leaf        *leafDecoder
	children    []fieldPlan
}

// Compile validates an entire struct schema without lookup or custom decoding.
// t must be a non-nil struct type already validated by the caller. Failure
// returns a nil Plan and diagnostics in depth-first declaration order.
func Compile(t reflect.Type, prefix string) (*Plan, []Diagnostic) {
	var diagnostics []Diagnostic
	fields := walkSchema(t, "", prefix, make(map[reflect.Type]bool), &diagnostics)
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return &Plan{typ: t, fields: fields}, nil
}

func walkSchema(t reflect.Type, path, prefix string, stack map[reflect.Type]bool, diagnostics *[]Diagnostic) []fieldPlan {
	stack[t] = true
	defer delete(stack, t)
	var fields []fieldPlan
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldPath := field.Name
		if path != "" {
			fieldPath = path + "." + field.Name
		}
		tags, valid := readTags(field.Tag)
		if !valid {
			*diagnostics = append(*diagnostics, diagnostic(fieldPath, "", -1, InvalidSchema))
			continue
		}
		plan := fieldPlan{field: i, path: fieldPath, separator: ","}
		if tags.hasEnv && tags.env != "-" {
			plan.key = prefix + tags.env
		}
		fail := func(category Category) {
			*diagnostics = append(*diagnostics, diagnostic(fieldPath, plan.key, -1, category))
		}
		otherTags := tags.hasBinding || tags.hasFallback || tags.hasSeparator || tags.hasPrefix
		// An explicit skip takes precedence over visibility, embedding and children.
		if tags.hasEnv && tags.env == "-" {
			if otherTags {
				fail(InvalidSchema)
			}
			continue
		}
		if field.PkgPath != "" {
			if tags.hasEnv || otherTags {
				fail(InvalidSchema)
			}
			// Untagged unexported anonymous fields are still unsupported.
			if field.Anonymous && !tags.hasEnv && !otherTags {
				fail(InvalidSchema)
			}
			continue
		}
		if field.Anonymous {
			fail(InvalidSchema)
			continue
		}
		if tags.hasEnv {
			if category := plan.compileLeaf(field.Type, tags); category != 0 {
				fail(category)
				continue
			}
			fields = append(fields, plan)
			continue
		}
		child, valid := containerType(field.Type, tags)
		if !valid {
			fail(InvalidSchema)
			continue
		}
		if child == nil {
			continue
		}
		if stack[child] {
			fail(InvalidSchema)
			continue
		}
		plan.children = walkSchema(child, fieldPath, prefix+tags.prefix, stack, diagnostics)
		fields = append(fields, plan)
	}
	return fields
}

// compileLeaf sets conversion and binding rules, returning zero on success.
// Check tags before type support, and type support before separator placement,
// so fields violating several rules keep their original diagnostic category.
func (p *fieldPlan) compileLeaf(t reflect.Type, tags fieldTags) Category {
	binding, valid := parseBinding(tags)
	if tags.env == "" || tags.hasPrefix || !valid {
		return InvalidSchema
	}
	leaf := newLeafDecoder(t)
	if leaf == nil {
		return UnsupportedType
	}
	if tags.hasSeparator && (t.Kind() != reflect.Slice || tags.separator == "") {
		return InvalidSchema
	}
	if tags.hasSeparator {
		p.separator = tags.separator
	}
	p.required = binding.required
	p.nonempty = binding.nonempty
	p.notrim = binding.notrim
	p.hasFallback = tags.hasFallback
	p.fallback = tags.fallback
	p.leaf = leaf
	return 0
}

// containerType checks untagged fields. A nil type with valid=true means an
// ignored non-container; valid=false means misplaced leaf or prefix tags.
// Recursive type detection belongs to the traversal's active stack.
func containerType(t reflect.Type, tags fieldTags) (reflect.Type, bool) {
	if tags.hasBinding || tags.hasFallback || tags.hasSeparator {
		return nil, false
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, !tags.hasPrefix
	}
	return t, true
}
