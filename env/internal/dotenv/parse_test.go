package dotenv_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muhammaadreza/gouse/env/internal/dotenv"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input string
		want        map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"comments", " \t# comment\n\t\n", map[string]string{}},
		{"assignment", " \tKEY \t= \tvalue \t\n_empty=\na1=last", map[string]string{"KEY": "value", "_empty": "", "a1": "last"}},
		{"case sensitive", "A=one\na=two", map[string]string{"A": "one", "a": "two"}},
		{"export", "export KEY=one\n\texport\t\tOTHER = two\nexport=key\nexported=three", map[string]string{"KEY": "one", "OTHER": "two", "export": "key", "exported": "three"}},
		{"comments and hashes", "A=# comment\nB=a#b # comment\nC=a\t# comment\nD=###\nE=a#b", map[string]string{"A": "", "B": "a#b", "C": "a", "D": "", "E": "a#b"}},
		{"single quotes", "A='  # $VAR ${VAR} \\n  '\t# comment\nB=''", map[string]string{"A": "  # $VAR ${VAR} \\n  ", "B": ""}},
		{"double quotes", `A="  # $VAR ${VAR} \\ \" \n \r \t  "` + "\nB=\"\"# comment", map[string]string{"A": "  # $VAR ${VAR} \\ \" \n \r \t  ", "B": ""}},
		{"literal interpolation", "A=$VAR/${VAR}/$(command)\nB=path\\to\\file", map[string]string{"A": "$VAR/${VAR}/$(command)", "B": `path\to\file`}},
		{"unicode", "A=こんにちは\nB=' café\u2003 '", map[string]string{"A": "こんにちは", "B": " café\u2003 "}},
		{"BOM and CRLF", "\ufeffA=one\r\nB='two'\r\n\r\n", map[string]string{"A": "one", "B": "two"}},
		{"BOM only", "\ufeff", map[string]string{}},
		{"unquoted punctuation", "A=a=b 'c'\nB=tail\\\nC=next", map[string]string{"A": "a=b 'c'", "B": "tail\\", "C": "next"}},
		{"long line", "A=" + strings.Repeat("x", 256*1024), map[string]string{"A": strings.Repeat("x", 256*1024)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := dotenv.Parse([]byte(tt.input))
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse(%s) = %v, %v; want %v", tt.name, got, err, tt.want)
			}
		})
	}
}

func TestInvalidSyntax(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"NO_ASSIGNMENT", "=empty", "1KEY=x", "A-B=x", "A.B=x", "é=x", "A B=x", "export A", "export\vA=x",
		"A='unterminated", `A="unterminated`, `A="bad\q"`, `A="bad\'"`, "A=\"trailing\\", "A='a'b", `A="a" trailing`,
		"A='one\ntwo'", "A=\"one\ntwo\"", "A=one\rB=two", "A=one\r", "A=\x00", "# \x00", "A=\xff", "# \xff", "A=\ufeff",
		"\ufeffA=x", // A BOM is allowed only at the beginning of the file.
		"A=x\nA=x", "A=x\nexport A=y",
	} {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			t.Parallel()
			prefix := "# first\r\n\n"
			got, err := dotenv.Parse([]byte(prefix + input))
			wantLine := 3
			if strings.HasPrefix(input, "A=x\n") {
				wantLine = 4
			}
			var syntax *dotenv.Error
			if got != nil || !errors.As(err, &syntax) || syntax.Line != wantLine {
				t.Fatalf("Parse(invalid) = %v, %v; want nil and line %d", got, err, wantLine)
			}
		})
	}
}
