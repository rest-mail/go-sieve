package sieve

import (
	"strings"
	"testing"
)

// These tests pin down the RFC 5228 §2.4.2.4 "encoded-character" extension
// (issue #24). Before the fix the capability was not recognised by require and
// the ${hex:...} / ${unicode:...} sequences were never decoded, so a string
// using them was matched/stored as its literal bytes rather than the octets it
// denotes.

// TestEncodedCharacter_HexDecodesInStringConstant covers the core case: with
// require "encoded-character", ${hex:24 24} decodes to the two "$" octets.
func TestEncodedCharacter_HexDecodesInStringConstant(t *testing.T) {
	script := `require ["encoded-character", "fileinto"];
if true { fileinto "${hex:24 24}"; }`
	r := runSieve(t, script, sieveEmail())
	if got := folderOf(r); got != "$$" {
		t.Errorf("hex encoded-character not decoded: fileinto folder = %q, want %q", got, "$$")
	}
}

// TestEncodedCharacter_HexKeyMatchesHeader mirrors the issue report: a key
// written as ${hex:24 24} must compare equal to the literal "$$".
func TestEncodedCharacter_HexKeyMatchesHeader(t *testing.T) {
	script := `require "encoded-character";
if header :is "Subject" "${hex:24 24}" { discard; }`
	msg := sieveEmail()
	msg.Headers.Subject = "$$"
	r := runSieve(t, script, msg)
	if r.disposition != Discard {
		t.Errorf("decoded key should match Subject %q: disposition = %v, want Discard", "$$", r.disposition)
	}
}

// TestEncodedCharacter_UnicodeDecodesToUTF8 covers ${unicode:...}: a code point
// is replaced by its UTF-8 encoding (U+1F4A9 is a 4-octet sequence).
func TestEncodedCharacter_UnicodeDecodesToUTF8(t *testing.T) {
	script := `require ["encoded-character", "fileinto"];
if true { fileinto "pile-${unicode:1F4A9}"; }`
	r := runSieve(t, script, sieveEmail())
	want := "pile-\U0001F4A9"
	if got := folderOf(r); got != want {
		t.Errorf("unicode encoded-character not decoded: %q, want %q", got, want)
	}
}

// TestEncodedCharacter_MalformedLeftLiteral covers §2.4.2.4: a ${...} run that
// does not match the grammar is left literal rather than raising an error.
func TestEncodedCharacter_MalformedLeftLiteral(t *testing.T) {
	script := `require ["encoded-character", "fileinto"];
if true { fileinto "a${hex:zz}b"; }`
	r := runSieve(t, script, sieveEmail())
	if got := folderOf(r); got != "a${hex:zz}b" {
		t.Errorf("malformed hex should stay literal: got %q", got)
	}
}

// TestEncodedCharacter_NotRequiredStaysLiteral guards against over-decoding:
// without require "encoded-character" the sequence is left untouched.
func TestEncodedCharacter_NotRequiredStaysLiteral(t *testing.T) {
	script := `require "fileinto";
if true { fileinto "x${hex:20}y"; }`
	r := runSieve(t, script, sieveEmail())
	if got := folderOf(r); got != "x${hex:20}y" {
		t.Errorf("without the require, the sequence must stay literal: got %q", got)
	}
}

// TestDecodeEncodedCharacters_RFCExamples walks the worked examples from
// RFC 5228 §2.4.2.4, including the case-insensitive keywords, the left-to-right
// (non-recursive) transform, and the several forms that must stay literal.
func TestDecodeEncodedCharacters_RFCExamples(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"$${hex:24 24}", "$$$"},           // leading literal "$", then "$$"
		{"${hex:40}", "@"},                 // at-sign
		{"${hex: 40 }", "@"},               // blanks around the pair
		{"${hex:24 24}", "$$"},             // two pairs
		{"${HEX:40}", "@"},                 // case-insensitive keyword
		{"${hex:4${hex:30}}", "${hex:40}"}, // left-to-right, output not re-scanned
		{"${unicode:40}", "@"},             // unicode at-sign
		{"${UNICODE:24 24}", "$$"},         // case-insensitive keyword
		{"${unicode:1F4A9}", "\U0001F4A9"},
		{"${hex:40", "${hex:40"},               // no closing brace: literal
		{"${hex:400}", "${hex:400}"},           // three digits, not paired: literal
		{"${ unicode:40}", "${ unicode:40}"},   // space before keyword: literal
		{"${Unicode:Cool}", "${Unicode:Cool}"}, // non-hex payload: literal
		{"no sequences here", "no sequences here"},
	}
	for _, c := range cases {
		got, err := decodeEncodedCharacters(c.in)
		if err != nil {
			t.Errorf("decodeEncodedCharacters(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("decodeEncodedCharacters(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDecodeEncodedCharacters_OutOfRangeUnicode covers the one case that is an
// error rather than a literal: a well-formed ${unicode:...} whose value is a
// surrogate or exceeds U+10FFFF.
func TestDecodeEncodedCharacters_OutOfRangeUnicode(t *testing.T) {
	for _, in := range []string{
		"${unicode:D800}",       // first surrogate
		"${unicode:DF01}",       // surrogate (RFC example)
		"${unicode:DFFF}",       // last surrogate
		"${unicode:110000}",     // one past U+10FFFF
		"${unicode:FFFFFFFFFF}", // too large to hold
	} {
		if _, err := decodeEncodedCharacters(in); err == nil {
			t.Errorf("decodeEncodedCharacters(%q): expected an out-of-range error", in)
		}
	}
}

// TestEncodedCharacter_InvalidUnicodeIsError covers §2.4.2.4: a well-formed
// ${unicode:...} whose value is a surrogate (D800..DFFF) or exceeds 10FFFF is an
// error, not a literal.
func TestEncodedCharacter_InvalidUnicodeIsError(t *testing.T) {
	bad := []string{
		`require "encoded-character"; if header :is "Subject" "${unicode:DF01}" { keep; }`,
		`require "encoded-character"; if header :is "Subject" "${unicode:110000}" { keep; }`,
	}
	for _, s := range bad {
		err := Validate(s)
		if err == nil {
			t.Errorf("expected an out-of-range/surrogate unicode value to be an error:\n%s", s)
			continue
		}
		if !strings.Contains(err.Error(), "range") {
			t.Errorf("expected the error to explain the out-of-range value, got: %v", err)
		}
	}
}
