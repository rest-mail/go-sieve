package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// RFC 5228 §2 and the §8.1 grammar end every line of script source in CRLF.
// No production admits LF on its own, so a bare LF is refused in every lexical
// context, as NUL and bare CR are.

// checklist: SIEVE-CORE-44
func TestRFC5228SourceBareLFRejected(t *testing.T) {
	for name, src := range map[string]string{
		"leading":           "\nkeep;",
		"whitespace":        "keep;\ndiscard;",
		"trailing":          "keep;\n",
		"between tokens":    "if\ntrue { keep; }",
		"comment only":      "# before\nafter",
		"hash comment":      "# before\nkeep;",
		"hash comment CRLF": "# before\r\n# after\nkeep;",
		"bracket comment":   "/* before\nafter */ keep;",
		"multiline marker":  "require \"reject\"; reject text:\nbody\r\n.\r\n;",
		"marker comment":    "require \"reject\"; reject text: # comment\nbody\r\n.\r\n;",
		"multiline body":    "require \"reject\"; reject text:\r\nbefore\nafter\r\n.\r\n;",
		"multiline dot":     "require \"reject\"; reject text:\r\nbody\r\n.\n;",
		"doubled LF":        "require \"reject\"; reject text:\r\nbody\r\n\n.\r\n;",
		"LF before CRLF":    "keep;\n\r\n",
		"quoted":            "require \"reject\"; reject \"before\nafter\";",
		"escaped in quoted": "require \"reject\"; reject \"before\\\nafter\";",
		"unexecuted":        "stop; /* before\nafter */ keep;",
	} {
		t.Run(name, func(t *testing.T) {
			if script, err := Parse(src); err == nil || script != nil {
				t.Errorf("Parse accepted bare LF: script=%v, err=%v", script, err)
			} else if !strings.Contains(err.Error(), "bare LF in script") {
				t.Errorf("Parse error = %v, want a bare LF diagnostic", err)
			}
			if err := Validate(src); err == nil {
				t.Error("Validate accepted bare LF")
			}
		})
	}
}

// checklist: SIEVE-CORE-44
func TestRFC5228SourceLFDiagnostic(t *testing.T) {
	// The line is the one the LF would end: earlier CRLF pairs, including
	// those inside comments and strings, each count as a line.
	for _, prefix := range []string{"", "\r\n", "keep;\r\n/* a\r\nb */\r\n", "require \"reject\";\r\nreject text:\r\nx\r\n.\r\n;\r\n"} {
		for _, ctx := range []string{"# c\n", "/* c\n */", "keep;\n", "\"a\nb\""} {
			src := prefix + ctx + "keep;"
			want := fmt.Sprintf("sieve: line %d: bare LF in script", 1+strings.Count(prefix, "\n"))
			if _, err := Parse(src); err == nil || err.Error() != want {
				t.Errorf("Parse(%q) error = %v, want %q", src, err, want)
			}
		}
	}
	// The first offending octet wins, whichever line break it is.
	for src, want := range map[string]string{
		"keep;\r\n# a\n# b\r":    "sieve: line 2: bare LF in script",
		"keep;\r\n# a\r# b\n":    "sieve: line 2: bare CR in script",
		"keep;\r\n# a\r\n\x00\n": "sieve: line 3: NUL byte in script",
	} {
		if _, err := Parse(src); err == nil || err.Error() != want {
			t.Errorf("Parse(%q) error = %v, want %q", src, err, want)
		}
	}
}

// checklist: SIEVE-CORE-44
func TestRFC5228SourceLFValuesPreserved(t *testing.T) {
	// CRLF in quoted and multiline strings reaches the value unchanged.
	for _, literal := range []string{"\"a\r\nb\"", "text:\r\na\r\nb\r\n.\r\n"} {
		src := "require \"reject\";\r\nreject " + literal + ";\r\n"
		script, err := Parse(src)
		if err != nil {
			t.Fatalf("%q: %v", literal, err)
		}
		out := script.Evaluate(&Message{}, newRecExec())
		want := "a\r\nb"
		if strings.HasPrefix(literal, "text:") {
			want += "\r\n"
		}
		if out.Error != nil || out.RejectReason != want {
			t.Errorf("literal %q: %+v, want %q", literal, out, want)
		}
	}
	// Source validation precedes decoding: encoded-character may still
	// produce a bare LF in a value (RFC 5228 §2.4.2.4).
	for _, encoded := range []string{"${hex:0a}", "${unicode:a}", "${hex:0A 0A}"} {
		src := `require ["reject", "encoded-character"]; reject "before` + encoded + `after";`
		script, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(src); err != nil {
			t.Fatal(err)
		}
		out := script.Evaluate(&Message{}, newRecExec())
		want := "before\nafter"
		if strings.Contains(encoded, " ") {
			want = "before\n\nafter"
		}
		if out.Error != nil || out.Disposition != Reject || out.RejectReason != want {
			t.Errorf("encoded LF %s: %+v, want %q", encoded, out, want)
		}
	}
}
