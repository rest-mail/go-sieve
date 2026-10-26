package sieve

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strings"
	"testing"

	sievegrammar "github.com/rest-mail/go-sieve/pkg/sieve/internal/grammar"
)

var sieveNumberGrammar = regexp.MustCompile(`^[0-9]+[kKmMgG]?$`)

// referenceSieveNumber uses the RFC 5228 §8.1 grammar and arbitrary-precision
// arithmetic, independently of readNumber's ParseInt and checked multiplication.
// The bool distinguishes valid grammar from the library's int64 range policy.
func referenceSieveNumber(literal string) (*big.Int, bool) {
	if !sieveNumberGrammar.MatchString(literal) {
		return nil, false
	}
	digits := literal
	var shift uint
	switch literal[len(literal)-1] {
	case 'k', 'K':
		shift = 10
	case 'm', 'M':
		shift = 20
	case 'g', 'G':
		shift = 30
	}
	if shift != 0 {
		digits = digits[:len(digits)-1]
	}
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		panic("number grammar accepted a non-decimal integer")
	}
	return n.Lsh(n, shift), true
}

func checkSieveNumberReference(t *testing.T, literal string) {
	t.Helper()
	want, grammar := referenceSieveNumber(literal)
	representable := grammar && want.IsInt64()
	l := sievegrammar.New(literal)
	got, err := l.ReadNumber()
	accepted := err == nil && l.Pos == len(literal)
	if accepted != representable {
		t.Fatalf("readNumber(%q): value=%d, consumed=%d, error=%v; representable=%v", literal, got, l.Pos, err, representable)
	}
	if accepted && got != want.Int64() {
		t.Fatalf("readNumber(%q)=%d, want %s", literal, got, want)
	}
	// Arbitrary malformed input may itself contain script syntax or trivia.
	// Only grammar-valid literals can safely be embedded as a single operand.
	if !grammar {
		return
	}
	for _, relation := range []string{"over", "under"} {
		src := fmt.Sprintf("if size :%s %s { discard; }", relation, literal)
		script, parseErr := Parse(src)
		if (parseErr == nil) != representable {
			t.Fatalf("Parse(%q): error=%v; representable=%v", src, parseErr, representable)
		}
		if (Validate(src) == nil) != representable {
			t.Fatalf("Validate(%q) disagrees with reference", src)
		}
		if !representable {
			continue
		}
		limit := want.Int64()
		sizes := []int64{0, 1, limit, math.MaxInt64}
		if limit > 0 {
			sizes = append(sizes, limit-1)
		}
		if limit < math.MaxInt64 {
			sizes = append(sizes, limit+1)
		}
		for _, size := range sizes {
			// RawSize=0 means reconstruct, so an empty header-only message
			// supplies a real zero without inventing a body separator.
			out := script.Evaluate(&Message{RawSize: size, NoBody: true}, newRecExec())
			comparison := big.NewInt(size).Cmp(want)
			match := relation == "over" && comparison > 0 || relation == "under" && comparison < 0
			if out.Error != nil || (out.Disposition == Discard) != match || out.ImplicitKeep != !match {
				t.Fatalf("%s, size=%d: outcome=%+v, want match=%v", src, size, out, match)
			}
		}
	}
}

// Anchor the independent reference to explicit RFC examples and range edges.
// checklist: SIEVE-CORE-01 SIEVE-LEX-08 SIEVE-TST-10
func TestSieveNumberReferenceExamples(t *testing.T) {
	for _, tc := range []struct{ literal, decimal string }{
		{"0", "0"}, {"00017", "17"}, {"4000", "4000"}, {"100K", "102400"},
		{"100k", "102400"}, {"1M", "1048576"}, {"1m", "1048576"},
		{"1G", "1073741824"}, {"1g", "1073741824"},
		{"2147483647", "2147483647"},
		{"9223372036854775807", "9223372036854775807"},
		{"9223372036854775808", "9223372036854775808"},
		{"8589934592G", "9223372036854775808"},
	} {
		n, ok := referenceSieveNumber(tc.literal)
		if !ok || n.String() != tc.decimal {
			t.Fatalf("reference(%q)=%v, %v; want %s", tc.literal, n, ok, tc.decimal)
		}
		checkSieveNumberReference(t, tc.literal)
	}
	for _, literal := range []string{"", "-1", "+1", " 1", "1 ", "1.0", "1KK", "1T", "１", "١", "1\x00"} {
		if _, ok := referenceSieveNumber(literal); ok {
			t.Fatalf("reference accepted invalid grammar %q", literal)
		}
		checkSieveNumberReference(t, literal)
	}
}

// checklist: SIEVE-CORE-01 SIEVE-LEX-08 SIEVE-TST-10
func TestSieveNumberReferenceBoundaries(t *testing.T) {
	for _, suffix := range []string{"", "K", "k", "M", "m", "G", "g"} {
		unit, _ := referenceSieveNumber("1" + suffix)
		maximum := new(big.Int).Quo(big.NewInt(math.MaxInt64), unit)
		for _, anchor := range []*big.Int{big.NewInt(0), big.NewInt(2147483647), big.NewInt(4294967295), maximum} {
			for delta := int64(-1); delta <= 1; delta++ {
				n := new(big.Int).Add(anchor, big.NewInt(delta))
				if n.Sign() < 0 {
					continue
				}
				for _, padding := range []string{"", "000", strings.Repeat("0", 256)} {
					checkSieveNumberReference(t, padding+n.String()+suffix)
				}
			}
		}
		checkSieveNumberReference(t, strings.Repeat("9", 4096)+suffix)
	}
}

// Every octet is tried where digits and quantifiers are expected. Full-token
// consumption prevents an accepted numeric prefix hiding an invalid suffix.
// checklist: SIEVE-LEX-08 SIEVE-TST-10
func TestSieveNumberReferenceOctets(t *testing.T) {
	for b := 0; b < 256; b++ {
		for _, literal := range []string{string(byte(b)), "17" + string(byte(b)), "1" + string(byte(b)) + "7"} {
			checkSieveNumberReference(t, literal)
		}
	}
}

// Fuzz arbitrary token bytes and derived decimal literals. The latter ensure
// random input keeps reaching quantifier/range handling and public evaluation.
// A 256-byte bound keeps the arbitrary-precision oracle and parser work small.
// checklist: SIEVE-LEX-08 SIEVE-TST-10
func FuzzSieveNumberReference(f *testing.F) {
	for _, seed := range []string{
		"", "0", "000", "1", "100K", "1m", "1G", "2147483647",
		"9223372036854775807", "9223372036854775808", "9007199254740991K",
		"9007199254740992K", "8796093022207M", "8796093022208M",
		"8589934591G", "8589934592G", "18446744073709551616", "1KK",
		"-1", "1\x00", "１", strings.Repeat("0", 128) + "1g",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 256 {
			return
		}
		checkSieveNumberReference(t, raw)
		digits := make([]byte, len(raw))
		for i := 0; i < len(raw); i++ {
			digits[i] = '0' + raw[i]%10
		}
		// Keep a finite variant even when the full digit run overflows,
		// so every fuzz input exercises evaluation with every quantifier.
		short := digits
		if len(short) > 8 {
			short = short[:8]
		}
		for _, decimal := range []string{"0" + string(digits), "000" + string(short)} {
			for _, suffix := range []string{"", "K", "k", "M", "m", "G", "g"} {
				checkSieveNumberReference(t, decimal+suffix)
			}
		}
	})
}
