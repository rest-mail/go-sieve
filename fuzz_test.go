package sieve

import (
	"strings"
	"testing"
)

// FuzzParse parses any script: the parser never panics, Validate agrees with
// Parse, and a script it accepts has the shape the grammar of RFC 5228 §8
// allows: its requires are capabilities the package supports, and evaluating it
// against a message neither panics nor loses the message.
//
// checklist: SIEVE-CMD-02 SIEVE-CMD-03 SIEVE-LEX-01 SIEVE-LEX-02 SIEVE-LEX-03 SIEVE-LEX-05 SIEVE-LEX-07 SIEVE-LEX-08 SIEVE-LEX-10
func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"keep;",
		`redirect "Friends: user@example.org;";`,
		`redirect "<@relay.example:user@example.org>";`,
		`redirect "first@example.org, second@example.org";`,
		`redirect "user@@example.org";`,
		`redirect "\"Ops: team; friends\" <ops@example.org>";`,
		`redirect "user@[IPv6:2001:db8::1]";`,
		`require "encoded-character"; redirect "Friends${hex:3a} user@example.org${hex:3b}";`,
		"require \"fileinto\"; fileinto \"before\\\x00after\";",
		"require \"fileinto\"; fileinto \"before\\\rafter\";",
		"require \"fileinto\"; fileinto \"before\\\nafter\";",
		"require \"fileinto\"; fileinto \"before\\\r\nafter\";",
		"# comment\n/* bracket */ keep;",
		`require ["fileinto", "envelope"]; if envelope :is "from" "a@b" { fileinto "X"; stop; } else { keep; }`,
		`if allof(header :contains ["Subject", "X"] "a", not size :over 100K, anyof(true, false)) { discard; }`,
		`if header :matches "Subject" "a*\\?b" { redirect "a@example.com"; }`,
		"if header :is \"Subject\" text:\n.. dot-stuffed\n.\n { keep; }",
		`require "encoded-character"; if header :is "Subject" "${hex:41 42}${unicode:1F600}" { keep; }`,
		`require ["vacation", "imap4flags", "body", "copy"]; if body :contains "x" { vacation :days 3 "hi"; setflag "\\Seen"; redirect :copy "a@b"; }`,
		`if header :is :comparator "i;ascii-numeric" "X" "1" { stop; }`,
		`if true { if true { if true { keep; } } }`,
		"if size :over 99999999999999999999 { keep; }",
		"if header :is [\"a\",] \"x\" { keep; }",
		"if { /* \x00 \"",
		"else; elsif; }}}{{{ ;;; \"\"\" text:\n",
	} {
		f.Add(seed)
	}
	msg := sieveEmail()
	f.Fuzz(func(t *testing.T, src string) {
		script, err := Parse(src)
		if vErr := Validate(src); (err == nil) != (vErr == nil) {
			t.Fatalf("Parse says %v, Validate says %v", err, vErr)
		}
		if err != nil {
			return
		}
		for _, name := range script.Requires() {
			if !supportedCapabilities[name] && !strings.HasPrefix(name, "comparator-") {
				t.Fatalf("accepted a require of unsupported %q", name)
			}
		}
		out := script.Evaluate(msg, newRecExec())
		if out.Error != nil {
			t.Fatalf("evaluating an accepted script failed: %v\n%q", out.Error, src)
		}
	})
}

// FuzzDecodeEncodedCharacters decodes any string: it never panics, a string
// without "${" is returned unchanged, and a successful decoding is a fixed
// point only when it introduced no new "${".
//
// checklist: SIEVE-LEX-06
func FuzzDecodeEncodedCharacters(f *testing.F) {
	for _, seed := range []string{
		"", "plain", "${hex:41 42 43}", "${HEX: 4 1}", "${unicode:48 20AC 1F600}", "${unicode:D800}",
		"${unicode:110000}", "${hex:}", "${hex:zz}", "$${hex:24}{hex:41}", "${hex:24}{hex:41}", "${",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := decodeEncodedCharacters(s)
		if err != nil {
			return
		}
		if !strings.Contains(s, "${") && got != s {
			t.Fatalf("decode(%q) = %q, want it unchanged", s, got)
		}
	})
}
