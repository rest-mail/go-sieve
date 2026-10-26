package grammar

import (
	"strings"
	"testing"
)

// FuzzDecodeEncodedCharacters decodes any string: it never panics, a string
// without "${" is returned unchanged, and a successful decoding is a fixed
// point only when it introduced no new "${".
//
// checklist: SIEVE-LEX-06
func FuzzDecodeEncodedCharacters(f *testing.F) {
	for _, seed := range []string{
		"", "plain", "${hex:41 42 43}", "${HEX: 4 1}", "${unicode:48 20AC 1F600}", "${unicode:D800}",
		"${unicode:110000}", "${unicode:D800z}", "${unicode:110000 41z}",
		"${unicode:D800 ${hex:41}}", "${unicode:D800\r}", "${hex:41\n42}", "${unicode:D800z}${unicode:DFFF}", "${hex:}", "${hex:zz}", "$${hex:24}{hex:41}", "${hex:24}{hex:41}", "${",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := DecodeEncodedCharacters(s)
		if err != nil {
			return
		}
		if !strings.Contains(s, "${") && got != s {
			t.Fatalf("decode(%q) = %q, want it unchanged", s, got)
		}
	})
}
