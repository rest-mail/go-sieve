package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// TestQuotedString_EscapedOctets compares every escaped octet with RFC 5228
// §8.1's quoted-special / quoted-other grammar. Quoting removes the backslash,
// but cannot make NUL, CR or LF valid; even an escaped CRLF is not quoted-safe.
// The accepted value must reach the host byte-for-byte, including high octets.
//
// checklist: SIEVE-LEX-03
func TestQuotedString_EscapedOctets(t *testing.T) {
	for octet := 0; octet <= 255; octet++ {
		t.Run(fmt.Sprintf("%02x", octet), func(t *testing.T) {
			value := string([]byte{byte(octet)})
			quoted := "\"before\\" + value + "after\""
			src := "# prelude\r\nrequire \"fileinto\"; fileinto " + quoted + ";"
			script, err := Parse(src)
			vErr := Validate(src)
			if octet == 0 || octet == 10 || octet == 13 {
				if err == nil || script != nil || vErr == nil {
					t.Fatalf("forbidden escaped octet accepted: Parse = (%v, %v), Validate = %v", script, err, vErr)
				}
				if !strings.Contains(err.Error(), "line 2:") {
					t.Fatalf("error lost the source line: %v", err)
				}
				return
			}
			if err != nil || vErr != nil {
				t.Fatalf("valid escaped octet rejected: Parse = %v, Validate = %v", err, vErr)
			}
			exec := newRecExec()
			out := script.Evaluate(sieveEmail(), exec)
			if out.Error != nil {
				t.Fatal(out.Error)
			}
			if got, want := exec.meta["deliver_to_folder"], "before"+value+"after"; got != want {
				t.Fatalf("host received %q, want %q", got, want)
			}
		})
	}
}

// checklist: SIEVE-LEX-03
func TestQuotedString_EscapedLineBreakAndTruncation(t *testing.T) {
	for name, quoted := range map[string]string{
		"escaped CRLF":          "\"before\\\r\nafter\";",
		"terminal backslash":    "\"before\\",
		"escaped closing quote": "\"before\\\"",
	} {
		t.Run(name, func(t *testing.T) {
			if script, err := Parse(`require "fileinto"; fileinto ` + quoted); err == nil || script != nil {
				t.Fatalf("malformed string accepted: (%v, %v)", script, err)
			}
		})
	}
}
