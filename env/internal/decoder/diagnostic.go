package decoder

// Category identifies a schema or value failure. Its zero value is invalid.
type Category uint8

const (
	// Required indicates a missing required key.
	Required Category = iota + 1
	// InvalidValue indicates that selected text failed decoding.
	InvalidValue
	// UnsupportedType indicates an unsupported annotated field type.
	UnsupportedType
	// EmptyValue indicates selected text violates nonempty.
	EmptyValue
	// InvalidSchema indicates malformed tags or invalid field declarations.
	InvalidSchema
)

// Diagnostic retains only safe field metadata and a category, never input text,
// fallback text, raw tags, a Plan, or original parser/custom errors.
type Diagnostic struct {
	Path     string   // Dot-separated Go field path.
	Key      string   // Complete lookup key, or empty when unavailable.
	Index    int      // Zero-based slice index; -1 for a whole field.
	Category Category // One of the defined nonzero categories.
}

func diagnostic(path, key string, index int, category Category) Diagnostic {
	return Diagnostic{Path: path, Key: key, Index: index, Category: category}
}
