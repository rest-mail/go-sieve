package sieve

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// numericBodyMatch exercises both comparator operands through the public API.
// Encoded-character strings let every octet reach the comparator unchanged.
func numericBodyMatch(t *testing.T, value, key string) bool {
	t.Helper()
	var encoded strings.Builder
	for i := 0; i < len(key); i++ {
		fmt.Fprintf(&encoded, "${hex:%02x}", key[i])
	}
	src := `require ["body", "encoded-character", "comparator-i;ascii-numeric"]; if body :is :comparator "i;ascii-numeric" "` + encoded.String() + `" { discard; }`
	if err := Validate(src); err != nil {
		t.Fatal(err)
	}
	s, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	out := s.Evaluate(&Message{Body: Body{ContentType: "text/plain", Content: value}}, newRecExec())
	if out.Error != nil {
		t.Fatal(out.Error)
	}
	return out.Disposition == Discard
}

// RFC 4790 §9.1.1: only an ASCII digit in the first octet starts a number.
// In particular, whitespace is data, not padding that the comparator strips.
//
// checklist: SIEVE-MT-09
func TestASCIINumericFirstOctet(t *testing.T) {
	for b := 0; b < 256; b++ {
		t.Run(fmt.Sprintf("%02x", b), func(t *testing.T) {
			value := string([]byte{byte(b), '5'})
			finite := b >= '0' && b <= '9'
			for _, pair := range [][2]string{{value, "infinity"}, {"infinity", value}} {
				if got := numericBodyMatch(t, pair[0], pair[1]); got != !finite {
					t.Errorf("%q == %q: got %v, want %v", pair[0], pair[1], got, !finite)
				}
			}
			// A leading zero does not rescue a non-digit prefix.
			for _, pair := range [][2]string{{value, "0" + value}, {"0" + value, value}} {
				if got := numericBodyMatch(t, pair[0], pair[1]); got != finite {
					t.Errorf("%q == %q: got %v, want %v", pair[0], pair[1], got, finite)
				}
			}
		})
	}
}

// checklist: SIEVE-MT-09
func TestASCIINumericRFCExamplesAndPrefixes(t *testing.T) {
	cases := []struct {
		value, key string
		want       bool
	}{
		// The equality examples in RFC 4790 §9.1.1.
		{"4294967298", "04294967298", true},
		{"4294967298", "4294967298b", true},
		{"", "x", true},
		{"x", "y", true},
		{"04294967298", "", false},
		{"0", "1", false},
		{"1", "4294967298", false},
		// Leading whitespace, signs, Unicode digits and encoded octets are
		// non-numeric; trailing non-digits terminate an already started run.
		{" 5", "5", false},
		{"\t5", "5", false},
		{"\r\n5", "5", false},
		{"\u00a05", "5", false},
		{"\u20035", "5", false},
		{"\u30005", "5", false},
		{"+5", "5", false},
		{"-5", "5", false},
		{"５", "5", false},
		{"\x005", "5", false},
		{" 5", "\t99", true},
		{"\u00a05", "nonnumeric", true},
		{"5 6", "0005", true},
		{"5\x006", "5", true},
		{"5\u00a06", "5", true},
		{"0 ", "0", true},
		{strings.Repeat("0", 256) + "123456789012345678901234567890suffix", "123456789012345678901234567890", true},
	}
	for _, tc := range cases {
		for _, pair := range [][2]string{{tc.value, tc.key}, {tc.key, tc.value}} {
			if got := numericBodyMatch(t, pair[0], pair[1]); got != tc.want {
				t.Errorf("%q == %q: got %v, want %v", pair[0], pair[1], got, tc.want)
			}
		}
	}
}

// RFC 5228 §5.7 trims field values before the comparator sees them, but it
// does not trim a script's key. Keep that field-specific behavior separate.
//
// checklist: SIEVE-MT-09 SIEVE-TST-08
func TestASCIINumericHeaderWhitespace(t *testing.T) {
	msg := &Message{Headers: Headers{Raw: map[string][]string{"X-Score": {" \t5\r\n\t "}}}}
	for key, want := range map[string]bool{"5": true, "005suffix": true, " 5": false, "\t5": false, "infinity": false} {
		src := fmt.Sprintf(`require ["fileinto", "comparator-i;ascii-numeric"]; if header :is :comparator "i;ascii-numeric" "X-Score" "%s" { fileinto "hit"; }`, key)
		got := folderOf(runSieve(t, src, msg)) == "hit"
		if got != want {
			t.Errorf("header against key %q: got %v, want %v", key, got, want)
		}
	}
}

// Compare leading non-digits against the independent foxcpp implementation.
// The field is 12 in diffEmail; only the literal key changes here.
//
// checklist: SIEVE-MT-09
func TestDifferentialNumericPrefixes(t *testing.T) {
	for _, key := range []string{"12", "0012", "12suffix", "12 99", " 12", "\t12", "\u00a012", "\u200312", "+12", "-12", "", "x"} {
		script := `require ["fileinto", "comparator-i;ascii-numeric"]; if header :is :comparator "i;ascii-numeric" "X-Spam-Score" "` + key + `" { fileinto "hit"; }`
		got, err := ours(script)
		if err != nil {
			t.Fatal(err)
		}
		want, err := reference(t, script)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("key %q: ours %v, reference %v", key, got, want)
		}
	}
}
