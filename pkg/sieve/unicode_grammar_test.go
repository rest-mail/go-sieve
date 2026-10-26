package sieve

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve/internal/grammar"
)

// checklist: SIEVE-LEX-06 SIEVE-LEX-12
func TestEncodedCharacterUnicodeMalformedRangeLiteral(t *testing.T) {
	for _, value := range []string{"D800", "DFFF", "110000", "FFFFFFFFFFFFFFFFFFFFFFFF"} {
		for _, payload := range []string{value + "z", value + " 41z", "41 " + value + " z", value + ";", value + " -41", value + " 41_", value + "\\", value + "\""} {
			in := "before${unicode:" + payload + "}after"
			t.Run(fmt.Sprintf("%q", payload), func(t *testing.T) {
				got, err := grammar.DecodeEncodedCharacters(in)
				if err != nil || got != in {
					t.Fatalf("decode(%q) = %q, %v; want literal", in, got, err)
				}
				// Sieve quoted strings escape only quotes and backslashes here.
				quoted := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(in) + `"`
				src := `require ["encoded-character", "reject"]; reject ` + quoted + `;`
				if err := Validate(src); err != nil {
					t.Fatalf("Validate malformed encoding: %v", err)
				}
				script, err := Parse(src)
				if err != nil {
					t.Fatal(err)
				}
				out := script.Evaluate(&Message{}, newRecExec())
				if out.Error != nil || out.Disposition != Reject || out.RejectReason != in {
					t.Fatalf("outcome = %+v; want literal rejection %q", out, in)
				}
			})
		}
	}
	// An invalid outer sequence does not suppress independently valid inner or
	// later sequences, and output is still decoded only once.
	for in, want := range map[string]string{
		"${unicode:D800 ${hex:41}}":                         "${unicode:D800 A}",
		"${unicode:110000z}${unicode:41}":                   "${unicode:110000z}A",
		"${unicode:D800z}${hex:24 7b 68 65 78 3a 34 31 7d}": "${unicode:D800z}${hex:41}",
	} {
		got, err := grammar.DecodeEncodedCharacters(in)
		if err != nil || got != want {
			t.Errorf("decode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	// A separate, well-formed sequence still produces the mandated range error.
	for _, in := range []string{"${unicode:D800z}${unicode:DFFF}", "${unicode:D800 ${unicode:110000}}", "${unicode:41 D800 42}", "${unicode:D800 41}", "${unicode:D800 DFFF}"} {
		if _, err := grammar.DecodeEncodedCharacters(in); err == nil {
			t.Errorf("decode(%q): missing range error", in)
		}
	}
}
