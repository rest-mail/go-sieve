package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// referenceMatches interprets RFC 5228 §2.7.1 with a set of reachable value
// offsets after each pattern token. Unlike wildcardMatch, it neither retries
// from a greedy star nor shares comparator folding with the implementation.
// A terminal backslash follows the evaluator's existing literal convention.
func referenceMatches(value, pattern, comparator string) bool {
	equal := func(a, b byte) bool {
		if comparator == "i;ascii-casemap" {
			// Uppercase only ASCII lowercase letters; all other octets are opaque.
			if a >= 'a' && a <= 'z' {
				a -= 'a' - 'A'
			}
			if b >= 'a' && b <= 'z' {
				b -= 'a' - 'A'
			}
		}
		return a == b
	}
	reachable := make([]bool, len(value)+1)
	reachable[0] = true
	for p := 0; p < len(pattern); p++ {
		token := pattern[p]
		literal := false
		if token == '\\' {
			literal = true
			if p+1 < len(pattern) {
				p++
				token = pattern[p]
			}
		}
		next := make([]bool, len(value)+1)
		for v := range reachable {
			if !literal && token == '*' {
				// A star can consume nothing or extend a previously reachable prefix.
				next[v] = reachable[v] || (v > 0 && next[v-1])
			} else if v < len(value) && reachable[v] &&
				((!literal && token == '?') || equal(value[v], token)) {
				next[v+1] = true
			}
		}
		reachable = next
	}
	return reachable[len(value)]
}

var matchesReferenceCases = []struct {
	value, pattern string
	octet, casemap bool
}{
	{"", "", true, true},
	{"", "***", true, true},
	{"", "?", false, false},
	{"x", "", false, false},
	{"ab", "a", false, false},
	{"ab", "b", false, false},
	{"ab", "*b", true, true},
	{"ab", "a*", true, true},
	{"ab", "a?", true, true},
	{"ab", "a??", false, false},
	{"ababac", "*ab*ac", true, true},
	{"ababac", "*ab*ad", false, false},
	{"aaaaab", "*a*a*a*b", true, true},
	{"aaaaab", "*a*a*a*c", false, false},
	{"*?", `\*\?`, true, true},
	{"ab", `\*\?`, false, false},
	{`a\b`, `a\\b`, true, true},
	{`a\`, `a\`, true, true},
	{"ab", `a\b`, true, true},
	{"*ab?", `*\?`, true, true},
	{"*ab?", `\**\?`, true, true},
	{"A*b", `a\*B`, false, true},
	{"é", "?", false, false},
	{"é", "??", true, true},
	{"ſ", "s", false, false},
	{"K", "k", false, false},
	{"\x00\xff", "??", true, true},
	{"a\x00b", "a*b", true, true},
	{"\xff", "\xfe", false, false},
}

// matchesScriptLiteral encodes all pattern octets so source quoting and
// wildcard escaping remain separate stages, including NUL and invalid UTF-8.
func matchesScriptLiteral(pattern string) string {
	if pattern == "" {
		return `""`
	}
	var out strings.Builder
	out.WriteString(`"${hex:`)
	for i := 0; i < len(pattern); i++ {
		fmt.Fprintf(&out, "%02x ", pattern[i])
	}
	out.WriteString(`}"`)
	return out.String()
}

func checkMatchesReference(t *testing.T, value, pattern string, public bool) {
	t.Helper()
	for _, comparator := range []string{"i;octet", "i;ascii-casemap"} {
		want := referenceMatches(value, pattern, comparator)
		if got := matchOne(value, pattern, ":matches", comparator); got != want {
			t.Fatalf("%s value %q pattern %q: got %v, reference %v", comparator, value, pattern, got, want)
		}
		if public {
			src := `require ["body", "encoded-character"]; if body :matches :comparator "` +
				comparator + `" ` + matchesScriptLiteral(pattern) + ` { discard; }`
			if got := bodyMatches(t, src, &Message{Body: Body{ContentType: "text/plain", Content: value}}); got != want {
				t.Fatalf("public %s value %q pattern %q: got %v, reference %v", comparator, value, pattern, got, want)
			}
		}
	}
}

// checklist: SIEVE-MT-03
func TestMatchesReferenceExamples(t *testing.T) {
	for _, tc := range matchesReferenceCases {
		for _, c := range []struct {
			name string
			want bool
		}{
			{"i;octet", tc.octet}, {"i;ascii-casemap", tc.casemap},
		} {
			if got := referenceMatches(tc.value, tc.pattern, c.name); got != c.want {
				t.Fatalf("reference %s value %q pattern %q: got %v, want %v", c.name, tc.value, tc.pattern, got, c.want)
			}
		}
		checkMatchesReference(t, tc.value, tc.pattern, true)
	}
	// Every octet is one character, can be escaped literally, and survives the
	// public encoded-character-to-body-matcher path without Unicode conversion.
	for b := 0; b < 256; b++ {
		value := string([]byte{byte(b)})
		for _, pattern := range []string{"?", "??", "*", "\\" + value, "*\\" + value + "*"} {
			checkMatchesReference(t, value, pattern, true)
		}
	}
}

// checklist: SIEVE-MT-03
func TestMatchesReferenceExhaustive(t *testing.T) {
	words := func(alphabet string, maxLen int) []string {
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
	// 21 values × 781 patterns × two comparators; includes escaped wildcards,
	// escaped backslashes, adjacent stars and failed suffix backtracking.
	for _, value := range words("aA*\xff", 2) {
		for _, pattern := range words("aA*?\\", 4) {
			checkMatchesReference(t, value, pattern, false)
		}
	}
}

// FuzzMatchesReference checks arbitrary byte strings against an independent
// dynamic-programming oracle through both matchOne and Parse/Evaluate.
// Limits bound the O(value × pattern) oracle and public parsing work per input.
//
// checklist: SIEVE-MT-03
func FuzzMatchesReference(f *testing.F) {
	for _, tc := range matchesReferenceCases {
		f.Add(tc.value, tc.pattern)
	}
	f.Fuzz(func(t *testing.T, value, pattern string) {
		if len(value) > 128 || len(pattern) > 128 {
			return
		}
		checkMatchesReference(t, value, pattern, true)
	})
}
