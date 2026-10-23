package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// RFC 5322 §2.2.3 unfolding removes only CRLF immediately followed by WSP.
// It must preserve the following SP/HTAB octets and any preceding whitespace.
// RFC 5228 §5.7 then ignores leading and trailing whitespace before matching.
//
// checklist: SIEVE-CORE-47 SIEVE-TST-08
func TestRFC5228HeaderUnfoldPreservesWhitespace(t *testing.T) {
	cases := []struct {
		name, value, want string
	}{
		{"space", "foo\r\n bar", "foo bar"},
		{"tab", "foo\r\n\tbar", "foo\tbar"},
		{"spaces", "foo\r\n   bar", "foo   bar"},
		{"mixed", "foo \t\r\n\t  bar", "foo \t\t  bar"},
		{"repeated", "foo\r\n \r\n\tbar", "foo \tbar"},
		{"separate", "foo\r\n\tbar\r\n  baz", "foo\tbar  baz"},
		{"edge padding", " \t\r\n foo\r\n\tbar \r\n\t", "foo\tbar"},
		{"already unfolded", "foo\t  bar", "foo\t  bar"},
		{"no folding whitespace", "foo\r\nbar", "foo\r\nbar"},
		{"bare CR", "foo\r bar", "foo\r bar"},
		{"bare LF", "foo\n\tbar", "foo\n\tbar"},
		{"truncated CR", "foo\rbar", "foo\rbar"},
		{"opaque octets", "\xfffoo\r\n\tbar\xfe", "\xfffoo\tbar\xfe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, structured := range []bool{false, true} {
				msg := &Message{Headers: Headers{Raw: map[string][]string{"X-Test": {tc.value}}}}
				name := "X-Test"
				if structured {
					msg.Headers = Headers{Subject: tc.value}
					name = "Subject"
				}
				for _, comparator := range []string{"i;octet", "i;ascii-casemap"} {
					for _, match := range []string{":is", ":contains", ":matches"} {
						script := fmt.Sprintf(`require "encoded-character"; if header %s :comparator "%s" "%s" "${hex:%s}" { discard; }`, match, comparator, name, fmt.Sprintf("% x", []byte(tc.want)))
						s, err := Parse(script)
						if err != nil {
							t.Fatal(err)
						}
						if out := s.Evaluate(msg, newRecExec()); out.Disposition != Discard {
							t.Errorf("structured=%v %s %s: %q did not match %q", structured, comparator, match, tc.value, tc.want)
						}
					}
				}
			}
		})
	}
}

// Every short SP/HTAB sequence must survive a fold byte for byte. A key with
// collapsed whitespace must not cause a false positive, even with :contains.
//
// checklist: SIEVE-CORE-47 SIEVE-TST-08
func TestRFC5228HeaderUnfoldWSPSequences(t *testing.T) {
	for width := 1; width <= 5; width++ {
		for bits := 0; bits < 1<<width; bits++ {
			wsp := make([]byte, width)
			for i := range wsp {
				wsp[i] = ' '
				if bits&(1<<i) != 0 {
					wsp[i] = '\t'
				}
			}
			value := "foo \r\n" + string(wsp) + "bar"
			want := "foo " + string(wsp) + "bar"
			msg := &Message{Headers: Headers{Raw: map[string][]string{"X-Test": {value}}}}
			for _, match := range []string{":is", ":contains", ":matches"} {
				for _, key := range []string{want, "foo bar"} {
					script := fmt.Sprintf(`require "encoded-character"; if header %s :comparator "i;octet" "X-Test" "${hex:%s}" { discard; }`, match, fmt.Sprintf("% x", []byte(key)))
					s, err := Parse(script)
					if err != nil {
						t.Fatal(err)
					}
					if got := s.Evaluate(msg, newRecExec()).Disposition == Discard; got != (key == want) {
						t.Errorf("%s %q against %q: match=%v", match, value, key, got)
					}
				}
			}
			// Unfolding must not alter the caller's map or accumulate changes
			// when a parsed script evaluates the same message again.
			if got := msg.Headers.Raw["X-Test"][0]; got != value {
				t.Fatalf("mutated host header: %q, want %q", got, value)
			}
			if got := unfoldAndTrim(want); got != strings.TrimSpace(want) {
				t.Errorf("already unfolded value changed: %q -> %q", want, got)
			}
		}
	}
}
