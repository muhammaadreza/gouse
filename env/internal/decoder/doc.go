// Package decoder compiles tagged struct schemas and decodes prepared lookups.
//
// Compile validates the complete schema without reading values or invoking
// custom decoders. Its opaque Plan decodes fresh candidates with a per-call
// lookup cache. Callers prepare sources between compilation and decoding and
// commit a candidate only after decoding succeeds.
//
// The decoder performs no I/O, reads no process environment, and returns only
// safe diagnostics. It does not depend on the public env or dotenv packages.
package decoder
