package sieve

import (
	"math/big"
	"strings"
	"testing"
)

// numericReference builds an arbitrary-precision integer one decimal place at
// a time, independently of asciiNumeric's string normalization. Nil is infinity.
// RFC 4790 section 9.1.1: https://www.rfc-editor.org/rfc/rfc4790#section-9.1.1
func numericReference(s string) *big.Int {
	var value *big.Int
	ten := big.NewInt(10)
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			break
		}
		if value == nil {
			value = new(big.Int)
		}
		value.Mul(value, ten)
		value.Add(value, big.NewInt(int64(b-'0')))
	}
	return value
}

func numericReferenceEqual(a, b string) bool {
	x, y := numericReference(a), numericReference(b)
	if x == nil || y == nil {
		return x == nil && y == nil
	}
	return x.Cmp(y) == 0
}

func checkNumericReference(t *testing.T, a, b string) {
	t.Helper()
	want := numericReferenceEqual(a, b)
	if got := compareIs(a, b, "i;ascii-numeric"); got != want {
		t.Fatalf("direct %q == %q: got %v, reference %v", a, b, got, want)
	}
	if got := numericBodyMatch(t, a, b); got != want {
		t.Fatalf("public %q == %q: got %v, reference %v", a, b, got, want)
	}
}

// Check the independent oracle against explicit numeric and infinity results.
// checklist: SIEVE-MT-07 SIEVE-MT-09
func TestASCIINumericReferenceExamples(t *testing.T) {
	cases := []struct{ input, decimal string }{
		{"", "infinity"}, {"x", "infinity"}, {" 12", "infinity"},
		{"+12", "infinity"}, {"-12", "infinity"}, {"１２", "infinity"},
		{"\x0012", "infinity"}, {"0", "0"}, {"000", "0"},
		{"00012suffix", "12"}, {"12 99", "12"}, {"12\x0099", "12"},
		{"4294967298b", "4294967298"},
		{"18446744073709551616", "18446744073709551616"},
		{strings.Repeat("9", 4096), strings.Repeat("9", 4096)},
	}
	for _, tc := range cases {
		got := "infinity"
		if n := numericReference(tc.input); n != nil {
			got = n.String()
		}
		if got != tc.decimal {
			t.Fatalf("reference(%q) = %s, want %s", tc.input, got, tc.decimal)
		}
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"4294967298", "04294967298", true},
		{"4294967298", "4294967298b", true},
		{"", "x", true}, {"x", "y", true},
		{"04294967298", "", false}, {"0", "1", false},
		{"1", "4294967298", false},
	} {
		if got := numericReferenceEqual(tc.a, tc.b); got != tc.want {
			t.Fatalf("reference %q == %q: got %v, want %v", tc.a, tc.b, got, tc.want)
		}
		checkNumericReference(t, tc.a, tc.b)
		checkNumericReference(t, tc.b, tc.a)
	}
}

// Cover overflow boundaries, long runs, and every octet before/after a number.
// Both operands pass through encoded-character parsing and public evaluation.
// checklist: SIEVE-MT-07 SIEVE-MT-09
func TestASCIINumericReferenceBoundaries(t *testing.T) {
	values := []string{"", "x", "0", "00", "1", "9", "10", "12", "0012suffix",
		"4294967295", "4294967296", "4294967298", "18446744073709551615",
		"18446744073709551616", "18446744073709551617",
		strings.Repeat("9", 128), strings.Repeat("0", 128) + "12"}
	for _, a := range values {
		for _, b := range values {
			checkNumericReference(t, a, b)
		}
	}
	for b := 0; b < 256; b++ {
		octet := string([]byte{byte(b)})
		for _, value := range []string{octet + "12", "12" + octet + "99", "000" + octet + "12"} {
			for _, key := range []string{"infinity", "12", "0"} {
				checkNumericReference(t, value, key)
				checkNumericReference(t, key, value)
			}
		}
	}
	long := strings.Repeat("9", 4096)
	for _, pair := range [][2]string{
		{long, "000" + long + "suffix"}, {long, long[:len(long)-1] + "8"},
		{long, "infinity"}, {strings.Repeat("0", 4096), "0"},
	} {
		checkNumericReference(t, pair[0], pair[1])
		checkNumericReference(t, pair[1], pair[0])
	}
}

// Bound arbitrary-precision arithmetic and public parser work per fuzz input.
// Derived pairs reach finite equality/inequality even from nonnumeric seeds.
// checklist: SIEVE-MT-07 SIEVE-MT-09
func FuzzASCIINumericReference(f *testing.F) {
	for _, seed := range [][2]string{
		{"", ""}, {"x", "y"}, {"0", "00"}, {"0", "1"}, {"1", "0001suffix"},
		{"4294967298", "04294967298"}, {"4294967298", "4294967298b"},
		{"18446744073709551616", "18446744073709551617"},
		{strings.Repeat("9", 128), strings.Repeat("0", 128) + strings.Repeat("9", 128)},
		{" 12", "12"}, {"\t12", "12"}, {"\r\n12", "12"}, {"+12", "12"},
		{"-12", "12"}, {"１２", "12"}, {"\u00a012", "12"}, {"\xff12", "12"},
		{"12\x0099", "12"}, {"12 99", "12"}, {"12\xff99", "12"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		if len(a) > 256 || len(b) > 256 {
			return
		}
		for _, pair := range [][2]string{
			{a, b}, {b, a}, {"1" + a, "0001" + a + "x"},
			{"1" + a, "2" + a}, {a, "x"},
		} {
			checkNumericReference(t, pair[0], pair[1])
		}
	})
}
