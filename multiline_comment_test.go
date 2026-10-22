package sieve

import (
	"strings"
	"testing"
)

// TestMultiLineString_HashCommentAfterMarker covers the RFC 5228 §8.1 rule
//
//	multi-line = "text:" *(SP / HTAB) (hash-comment / CRLF)
//	             *(multiline-literal / multiline-dotstart) "." CRLF
//
// The line that introduces a multi-line string may end in a hash comment. The
// comment is not part of the string, and nothing else may follow the marker.
//
// checklist: SIEVE-LEX-05
func TestMultiLineString_HashCommentAfterMarker(t *testing.T) {
	valid := []struct {
		name, src, want string
	}{
		{"space then comment", "text: # note\nbody\n.\n", "body\n"},
		{"tab then comment, CRLF", "text:\t# note\r\nbody\r\n.\r\n", "body\r\n"},
		{"comment right after the colon", "text:# note\n.\n", ""},
		{"empty comment", "text: #\nbody\n.\n", "body\n"},
		{"upper-case marker", "TEXT: # note\nbody\n.\n", "body\n"},
		// The comment runs to the end of the line, so tokens and quotes in it
		// are not lexed.
		{"comment holding tokens", "text: # \"x\" ; { /* \nbody\n.\n", "body\n"},
		// A body line that looks like a comment is string content.
		{"body line starting with #", "text: # note\n# not a comment\n.\n", "# not a comment\n"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			toks, err := newLexer(tc.src + "keep").tokenize()
			if err != nil {
				t.Fatalf("tokenize: %v", err)
			}
			if len(toks) != 3 || toks[0].kind != tString || toks[1].kind != tIdent || toks[2].kind != tEOF {
				t.Fatalf("tokens = %+v, want string, keep, EOF", toks)
			}
			if toks[0].str != tc.want {
				t.Errorf("string = %q, want %q", toks[0].str, tc.want)
			}
			if want := strings.Count(tc.src, "\n") + 1; toks[1].line != want {
				t.Errorf("keep is on line %d, want %d", toks[1].line, want)
			}
		})
	}

	malformed := map[string]string{
		// The comment ends the source before the line break, so the string
		// has no body and no closing ".".
		"comment at end of script": "require \"vacation\"; vacation text: # note",
		// The comment and its line break are there, but the closing "." is not.
		"no terminating dot": "require \"vacation\"; vacation text: # note\nbody\n",
		// Only SP and HTAB may stand between the marker and the comment.
		"word before comment": "require \"vacation\"; vacation text: x # note\nbody\n.\n;",
		// A bracket comment is not a hash comment, so it cannot end the line.
		"bracket comment": "require \"vacation\"; vacation text: /* note */\nbody\n.\n;",
	}
	for name, src := range malformed {
		t.Run(name, func(t *testing.T) {
			if err := Validate(src); err == nil {
				t.Errorf("%q was accepted", src)
			}
		})
	}

	// The comment does not reach the string a command receives.
	script := "require \"vacation\";\n" +
		"vacation :subject \"OOO\" text: # reply body\n" +
		"I am away.\n" +
		".\n" +
		";"
	r := runSieve(t, script, sieveEmail())
	if body := r.meta["vacation_reply_body"]; body != "I am away.\n" {
		t.Errorf("vacation body = %q, want %q", body, "I am away.\n")
	}
}
