package dotenv

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Error describes invalid syntax without retaining any input text.
type Error struct {
	Line int
	Kind string
}

// Error returns a physical line number and a static syntax category.
func (e *Error) Error() string {
	return fmt.Sprintf("dotenv: line %d: %s", e.Line, e.Kind)
}

// Parse validates all contents and returns assignments, or nil on failure.
// Duplicate keys are invalid. Lines have no fixed size limit.
func Parse(contents []byte) (map[string]string, error) {
	text := strings.TrimPrefix(string(contents), "\ufeff")
	values := make(map[string]string)
	for number := 1; text != ""; number++ {
		line, rest, newline := strings.Cut(text, "\n")
		text = rest
		if newline {
			line = strings.TrimSuffix(line, "\r")
		}
		if !utf8.ValidString(line) || strings.ContainsAny(line, "\x00\r\ufeff") {
			return nil, &Error{Line: number, Kind: "invalid encoding or control character"}
		}
		line = strings.Trim(line, " \t")
		if line == "" || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
			line = strings.TrimLeft(line[len("export"):], " \t")
		}
		key, raw, assignment := strings.Cut(line, "=")
		key = strings.Trim(key, " \t")
		if !assignment || !validKey(key) {
			return nil, &Error{Line: number, Kind: "invalid assignment"}
		}
		value, valid := parseValue(strings.TrimLeft(raw, " \t"))
		if !valid {
			return nil, &Error{Line: number, Kind: "invalid quoted value"}
		}
		if _, exists := values[key]; exists {
			return nil, &Error{Line: number, Kind: "duplicate key"}
		}
		values[key] = value
	}
	return values, nil
}

func validKey(key string) bool {
	if key == "" {
		return false
	}
	for i := range len(key) {
		c := key[i]
		letter := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
		if !letter && !(i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func parseValue(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	quote := raw[0]
	if quote != '\'' && quote != '"' {
		for i := range len(raw) {
			if raw[i] == '#' && (i == 0 || raw[i-1] == ' ' || raw[i-1] == '\t') {
				raw = raw[:i]
				break
			}
		}
		return strings.TrimRight(raw, " \t"), true
	}
	var value strings.Builder
	for i := 1; i < len(raw); i++ {
		c := raw[i]
		if c == quote {
			tail := strings.TrimLeft(raw[i+1:], " \t")
			return value.String(), tail == "" || tail[0] == '#'
		}
		if quote == '"' && c == '\\' {
			i++
			if i == len(raw) {
				return "", false
			}
			switch raw[i] {
			case '\\', '"':
				c = raw[i]
			case 'n':
				c = '\n'
			case 'r':
				c = '\r'
			case 't':
				c = '\t'
			default:
				return "", false
			}
		}
		value.WriteByte(c)
	}
	return "", false
}
