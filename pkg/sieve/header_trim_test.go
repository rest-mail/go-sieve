package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// headerTrimLiteral uses encoded-character so control and opaque octets reach
// the comparator without depending on Go's or Sieve's quoted-string escapes.
func headerTrimLiteral(s string) string {
	if s == "" {
		return `""`
	}
	var b strings.Builder
	b.WriteString(`"${hex:`)
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&b, " %02x", s[i])
	}
	b.WriteString(`}"`)
	return b.String()
}

func headerTrimMatched(t *testing.T, expr string, msg *Message) bool {
	t.Helper()
	r := runSieve(t, `require ["fileinto", "encoded-character"]; if `+expr+` { fileinto "hit"; }`, msg)
	return folderOf(r) == "hit"
}

// RFC 5228 §5.7 ignores mail whitespace at the value boundaries, not Unicode
// White_Space or arbitrary ASCII controls (RFC 5322 §§2.2.3, 3.2.2).
// checklist: SIEVE-CORE-48 SIEVE-TST-08
func TestRFC5228HeaderTrimPreservesNonWSP(t *testing.T) {
	for _, edge := range []string{
		"\v", "\f", "\u0085", "\u00a0", "\u1680", "\u2000", "\u2001",
		"\u2002", "\u2003", "\u2004", "\u2005", "\u2006", "\u2007",
		"\u2008", "\u2009", "\u200a", "\u2028", "\u2029", "\u202f",
		"\u205f", "\u3000", "\xff", "\xc2", "\x00",
	} {
		for _, value := range []struct{ raw, want string }{
			{edge + "hit", edge + "hit"},
			{"hit" + edge, "hit" + edge},
			{edge, edge},
			{" \t" + edge + "hit" + edge + "\t ", edge + "hit" + edge},
			{" \t" + edge + "hi\r\n there" + edge + "\t ", edge + "hi there" + edge},
		} {
			for _, structured := range []bool{false, true} {
				msg := &Message{Headers: Headers{Raw: map[string][]string{"Subject": {value.raw}}}}
				if structured {
					msg.Headers = Headers{Subject: value.raw}
				}
				for _, comparator := range []string{"i;octet", "i;ascii-casemap"} {
					for _, match := range []string{":is", ":contains", ":matches"} {
						expr := fmt.Sprintf(`header %s :comparator %q "Subject" %s`, match, comparator, headerTrimLiteral(value.want))
						if !headerTrimMatched(t, expr, msg) {
							t.Errorf("structured=%v value=%q: %s did not match", structured, value.raw, expr)
						}
					}
					// Unicode trimming must not turn a significant edge into an
					// empty value or allow a shorter exact match.
					stripped := strings.TrimSpace(value.want)
					if stripped != value.want {
						expr := fmt.Sprintf(`header :is :comparator %q "Subject" %s`, comparator, headerTrimLiteral(stripped))
						if headerTrimMatched(t, expr, msg) {
							t.Errorf("structured=%v value=%q falsely matched stripped key %q", structured, value.raw, stripped)
						}
					}
				}
				if !headerTrimMatched(t, `exists "Subject"`, msg) {
					t.Errorf("structured=%v value=%q lost header existence", structured, value.raw)
				}
				if structured && msg.Headers.Subject != value.raw || !structured && msg.Headers.Raw["Subject"][0] != value.raw {
					t.Fatal("evaluation mutated the caller's header")
				}
			}
		}
	}
}

// checklist: SIEVE-CORE-48 SIEVE-TST-08
func TestRFC5228HeaderTrimBoundaryOctets(t *testing.T) {
	for n := 0; n < 256; n++ {
		// Line breaks are handled by unfolding, outside this boundary test.
		if n == '\r' || n == '\n' {
			continue
		}
		edge := string([]byte{byte(n)})
		value := edge + "hit" + edge
		want := value
		if n == ' ' || n == '\t' {
			want = "hit"
		}
		msg := &Message{Headers: Headers{Raw: map[string][]string{"X-Test": {value}}}}
		expr := `header :is :comparator "i;octet" "X-Test" ` + headerTrimLiteral(want)
		if !headerTrimMatched(t, expr, msg) {
			t.Errorf("boundary octet %02x: value=%q did not match %q", n, value, want)
		}
	}
}
