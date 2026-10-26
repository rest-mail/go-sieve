package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// checklist: SIEVE-CORE-49
func TestRFC5228SourceBareCRRejected(t *testing.T) {
	for name, src := range map[string]string{
		"whitespace":       "keep;\rdiscard;",
		"trailing":         "keep;\r",
		"comment only":     "# before\rafter",
		"hash comment":     "# before\rafter\r\nkeep;",
		"bracket comment":  "/* before\rafter */ keep;",
		"multiline marker": "require \"reject\"; reject text:\r # comment\r\nbody\r\n.\r\n;",
		"marker comment":   "require \"reject\"; reject text:# before\rafter\r\nbody\r\n.\r\n;",
		"multiline body":   "require \"reject\"; reject text:\r\nbefore\rafter\r\n.\r\n;",
		"doubled CR":       "require \"reject\"; reject text:\r\nbody\r\r\n.\r\n;",
		"quoted":           "require \"reject\"; reject \"before\rafter\";",
		"unexecuted":       "stop; /* before\rafter */ keep;",
	} {
		t.Run(name, func(t *testing.T) {
			if script, err := Parse(src); err == nil || script != nil {
				t.Errorf("Parse accepted bare CR: script=%v, err=%v", script, err)
			}
			if err := Validate(src); err == nil {
				t.Error("Validate accepted bare CR")
			}
		})
	}
}

// checklist: SIEVE-CORE-49
func TestRFC5228SourceCRFollowingOctets(t *testing.T) {
	// In a hash comment every non-NUL octet is otherwise opaque. Only LF may
	// immediately follow CR; Unicode or a later LF must not repair the pair.
	for n := 1; n <= 255; n++ {
		src := "# before\r" + string([]byte{byte(n)}) + "#after\r\nkeep;"
		_, err := Parse(src)
		if wantOK := n == '\n'; (err == nil) != wantOK {
			t.Errorf("following octet %02x: Parse error = %v, want success %v", n, err, wantOK)
		}
		if vErr := Validate(src); (vErr == nil) != (n == '\n') {
			t.Errorf("following octet %02x: Validate error = %v", n, vErr)
		}
	}
}

// checklist: SIEVE-CORE-49
func TestRFC5228SourceCRDiagnostic(t *testing.T) {
	for _, prefix := range []string{"", "\r\n", "keep;\r\n/* a\r\nb */\r\n", "# legacy LF\n"} {
		for _, suffix := range []string{"", "\r", "x\r\nkeep;"} {
			src := prefix + "# before\r" + suffix
			want := fmt.Sprintf("sieve: line %d: bare CR in script", 1+strings.Count(prefix, "\n"))
			if _, err := Parse(src); err == nil || err.Error() != want {
				t.Errorf("Parse(%q) error = %v, want %q", src, err, want)
			}
		}
	}
}

// checklist: SIEVE-CORE-49 SIEVE-CORE-38
func TestRFC5228SourceCRLFPreserved(t *testing.T) {
	for _, value := range []string{"before\r\nafter", "before\r\n\r\nafter", "\r\n", "opaque\xff\r\n\xfe"} {
		for _, literal := range []string{"\"" + value + "\"", "text: # marker\r\n" + value + "\r\n.\r\n"} {
			src := "# comment\r\n/* bracket\r\ncomment */\r\nrequire \"reject\";\r\nreject " + literal + ";\r\n"
			script, err := Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(src); err != nil {
				t.Fatal(err)
			}
			want := value
			if strings.HasPrefix(literal, "text:") {
				want += "\r\n"
			}
			out := script.Evaluate(&Message{}, newRecExec())
			if out.Error != nil || out.Disposition != Reject || out.RejectReason != want {
				t.Errorf("literal %q: outcome=%+v, want rejection %q", literal, out, want)
			}
		}
	}
	// Source validation precedes decoding: encoded CR remains a legal value.
	for _, encoded := range []string{"${hex:0d}", "${unicode:d}"} {
		src := `require ["reject", "encoded-character"]; reject "before` + encoded + `after";`
		script, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(src); err != nil {
			t.Fatal(err)
		}
		out := script.Evaluate(&Message{}, newRecExec())
		if out.Error != nil || out.Disposition != Reject || out.RejectReason != "before\rafter" {
			t.Errorf("encoded CR %s: %+v", encoded, out)
		}
	}
}
