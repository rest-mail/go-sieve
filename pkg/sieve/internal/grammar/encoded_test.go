package grammar

import (
	"strings"
	"testing"
)

// checklist: SIEVE-LEX-06 SIEVE-LEX-12
func TestEncodedCharacterUnicodeMalformedRangeOctets(t *testing.T) {
	// Validate the entire payload, including tokens after an invalid scalar.
	// Exercise every non-hex, non-blank octet, including opaque non-UTF-8 bytes.
	for n := 0; n <= 255; n++ {
		c := byte(n)
		if strings.ContainsRune("0123456789abcdefABCDEF \t", rune(c)) {
			continue
		}
		for _, value := range []string{"D800", "110000", "FFFFFFFFFFFFFFFFFFFFFFFF"} {
			for _, payload := range []string{value + string(c), value + " 41" + string(c), "41 " + value + " " + string(c)} {
				if got, ok, err := decodeUnicodeSeq(payload); ok || err != nil || got != "" {
					t.Errorf("payload %q: got %q, %v, %v; want malformed without error", payload, got, ok, err)
				}
			}
		}
	}
}

// checklist: SIEVE-LEX-06 SIEVE-LEX-12
func TestEncodedCharacterBlankGrammar(t *testing.T) {
	for _, keyword := range []string{"hex", "unicode"} {
		for _, sep := range []string{"\r", "\n", "\n\r", "\r\r\n", "\r\n\n", " \r\t", "\v", "\f", "\u00a0"} {
			for _, payload := range []string{sep + "41", "41" + sep, "41" + sep + "42", "D800" + sep, "D800" + sep + "41"} {
				in := "${" + keyword + ":" + payload + "}"
				got, err := DecodeEncodedCharacters(in)
				if err != nil || got != in {
					t.Errorf("decode(%q) = %q, %v; want literal", in, got, err)
				}
			}
		}
		for _, sep := range []string{" ", "\t", "\r\n", " \t\r\n \r\n\t"} {
			in := "${" + keyword + ":" + sep + "41" + sep + "42" + sep + "}"
			if got, err := DecodeEncodedCharacters(in); err != nil || got != "AB" {
				t.Errorf("decode(%q) = %q, %v; want AB", in, got, err)
			}
			if keyword == "unicode" {
				if _, err := DecodeEncodedCharacters("${unicode:" + sep + "D800" + sep + "41" + sep + "}"); err == nil {
					t.Errorf("well-formed encoding with separator %q: missing range error", sep)
				}
			}
		}
	}
}
