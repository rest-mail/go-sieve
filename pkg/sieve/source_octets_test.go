package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// checklist: SIEVE-CORE-37
func TestRFC5228SourceNULRejected(t *testing.T) {
	// Inject at every byte boundary, including inside ignored comments, after
	// stop, inside multiline text and between the bytes of a CRLF. Checking the
	// entire source prevents an unexecuted branch or a comment hiding a NUL.
	for name, src := range map[string]string{
		"empty":            "",
		"commands":         "keep;\r\nstop; discard;\r\n",
		"hash comment":     "# comment\r\nkeep; # trailing\r\n",
		"bracket comment":  "/* before\r\n** after */ keep;",
		"quoted string":    `require "reject"; reject "before\"after";`,
		"multiline string": "require \"reject\"; reject text: # marker\r\nbefore\r\n..after\r\n.\r\n;",
		"unexecuted block": "stop; if false { /* ignored */ keep; }",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(src); err != nil {
				t.Fatalf("invalid test source: %v", err)
			}
			if err := Validate(src); err != nil {
				t.Fatalf("invalid test source: %v", err)
			}
			for i := 0; i <= len(src); i++ {
				bad := src[:i] + "\x00" + src[i:]
				if script, err := Parse(bad); err == nil || script != nil {
					t.Errorf("byte %d: Parse accepted raw NUL: script=%v, err=%v", i, script, err)
				}
				if err := Validate(bad); err == nil {
					t.Errorf("byte %d: Validate accepted raw NUL", i)
				}
			}
		})
	}
}

// checklist: SIEVE-CORE-37
func TestRFC5228SourceNULDiagnostic(t *testing.T) {
	for _, prefix := range []string{"", "\r\n", "keep;\r\n/* a\r\nb */\r\n"} {
		for _, suffix := range []string{"", "\x00", "\r\nkeep;"} {
			src := prefix + "# before\x00after" + suffix
			want := fmt.Sprintf("sieve: line %d: NUL byte in script", 1+strings.Count(prefix, "\n"))
			if _, err := Parse(src); err == nil || err.Error() != want {
				t.Errorf("Parse(%q) error = %v, want %q", src, err, want)
			}
		}
	}
}

// checklist: SIEVE-CORE-02 SIEVE-CORE-37
func TestRFC5228SourceNonNULOctets(t *testing.T) {
	// Raw strings and comments allow every non-linebreak octet except NUL,
	// including invalid UTF-8. Source validation must not filter these out.
	for n := 1; n <= 255; n++ {
		if n == '\r' || n == '\n' {
			continue
		}
		value := string([]byte{byte(n)})
		src := "# before" + value + "after\r\n/* before" + value + "after */\r\n" +
			"require \"reject\"; reject text: # before" + value + "after\r\n" +
			"before" + value + "after\r\n.\r\n;"
		if err := Validate(src); err != nil {
			t.Fatalf("octet %02x: Validate: %v", n, err)
		}
		script, err := Parse(src)
		if err != nil {
			t.Fatalf("octet %02x: Parse: %v", n, err)
		}
		out := script.Evaluate(&Message{}, newRecExec())
		if want := "before" + value + "after\r\n"; out.Error != nil || out.Disposition != Reject || out.RejectReason != want {
			t.Fatalf("octet %02x: outcome=%+v, want rejection %q", n, out, want)
		}
	}
	// RFC 5228 §2.4.2.4 explicitly permits an encoded NUL in a string value.
	for _, encoded := range []string{"${hex:00}", "${unicode:0}"} {
		src := `require ["reject", "encoded-character"]; reject "before` + encoded + `after";`
		if err := Validate(src); err != nil {
			t.Fatal(err)
		}
		script, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out := script.Evaluate(&Message{}, newRecExec())
		if out.Error != nil || out.Disposition != Reject || out.RejectReason != "before\x00after" {
			t.Fatalf("encoded NUL %s: %+v", encoded, out)
		}
	}
}
