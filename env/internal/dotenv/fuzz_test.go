package dotenv_test

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muhammaadreza/gouse/env/internal/dotenv"
)

func FuzzDotenv(f *testing.F) {
	for _, seed := range []string{"", "A=value", "A=\nA=", "export A=' x # '", `A="a\nb"`, "\ufeffA=x\r\n", "A=\xff", "A=\x00", "A='one\ntwo'", `A="\q"`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 64*1024 {
			t.Skip() // Bound harness work, not production line length.
		}
		first, err := dotenv.Parse([]byte(input))
		second, again := dotenv.Parse([]byte(input))
		if !reflect.DeepEqual(first, second) || (err == nil) != (again == nil) {
			t.Fatal("Parse is nondeterministic")
		}
		if err != nil {
			var syntax *dotenv.Error
			if first != nil || !errors.As(err, &syntax) || syntax.Line < 1 || syntax.Line > strings.Count(input, "\n")+1 || err.Error() != again.Error() {
				t.Fatalf("Parse invalid result: %v, %v", first, err)
			}
		}
		// A generated literal has an independent, exact oracle. Its duplicate
		// must fail even when both values are identical. Error data is inspected
		// directly as well as through formatting to catch retained input.
		canary := "CANARY_DOTENV_" + hex.EncodeToString([]byte(input))
		assignment := "KEY='" + canary + "'"
		values, validErr := dotenv.Parse([]byte(assignment))
		if validErr != nil || len(values) != 1 || values["KEY"] != canary {
			t.Fatal("Parse rejected generated valid literal")
		}
		for _, invalid := range []string{assignment + "\n" + assignment, "KEY=\"\\q" + canary + "\"", "KEY='" + canary} {
			values, invalidErr := dotenv.Parse([]byte(invalid))
			var syntax *dotenv.Error
			if values != nil || !errors.As(invalidErr, &syntax) {
				t.Fatal("Parse accepted generated invalid input")
			}
			if strings.Contains(fmt.Sprintf("%s %+v %#v %+v", invalidErr, invalidErr, invalidErr, *syntax), "CANARY_DOTENV_") {
				t.Fatal("Parse error retained a value")
			}
		}
	})
}
