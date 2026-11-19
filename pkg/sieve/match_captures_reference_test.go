package sieve

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve/internal/match"
)

// capturesOracle answers the RFC 5229 §3.2 match list with Go's regexp
// package, independently of match.Captures: each "*" becomes a lazy group
// and each "?" a one-character group, anchored at both ends. regexp chooses
// the submatches a backtracking matcher would, so lazy groups take as little
// as possible, leftmost first, which is the non-greedy rule of §3.2. The
// alphabets below are ASCII, so a regexp character is one octet.
func capturesOracle(pattern string, fold bool) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`^(?s)`)
	if fold {
		b.WriteString(`(?i)`)
	}
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			b.WriteString(`(.*?)`)
		case '?':
			b.WriteString(`(.)`)
		case '\\':
			if i+1 < len(pattern) {
				i++
			}
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(`$`)
	return regexp.MustCompile(b.String())
}

// allStrings lists every string over alphabet of length 0..maxLen.
func allStrings(alphabet string, maxLen int) []string {
	out := []string{""}
	level := []string{""}
	for n := 0; n < maxLen; n++ {
		var next []string
		for _, prefix := range level {
			for i := 0; i < len(alphabet); i++ {
				next = append(next, prefix+alphabet[i:i+1])
			}
		}
		out = append(out, next...)
		level = next
	}
	return out
}

// TestMatchCapturesReference checks match.Captures, which fills the match
// variables, against the regexp oracle on every pattern of up to four
// symbols and every value of up to five octets, under both byte comparators.
// It also checks that Captures agrees with the :matches test itself, so a
// capture is set exactly when the test succeeds.
//
// checklist: VARIABLES-07
func TestMatchCapturesReference(t *testing.T) {
	patterns := allStrings(`aB*?\`, 4)
	values := allStrings("abB\n", 5)
	checked := 0
	for _, comparator := range []string{"i;octet", "i;ascii-casemap"} {
		fold := comparator == "i;ascii-casemap"
		for _, p := range patterns {
			oracle := capturesOracle(p, fold)
			for _, v := range values {
				got, ok := match.Captures(v, p, comparator)
				want := oracle.FindStringSubmatch(v)
				wantOK := want != nil
				if ok != wantOK || !reflect.DeepEqual(got, want) {
					t.Fatalf("Captures(%q, %q, %s) = %q, %v; oracle %q, %v", v, p, comparator, got, ok, want, wantOK)
				}
				if one := match.One(v, p, ":matches", comparator); one != ok {
					t.Fatalf(":matches %q %q under %s = %v, Captures says %v", v, p, comparator, one, ok)
				}
				checked++
			}
		}
	}
	if checked < 2_000_000 {
		t.Fatalf("only %d cases checked", checked)
	}
}
