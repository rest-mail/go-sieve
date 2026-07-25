package sieve

import "testing"

// TestComparator_UnknownRejected covers issue #19 part (a): an unknown or
// unsupported comparator must be an error, never a silent fall-back to the
// default i;ascii-casemap (RFC 5228 §2.7.3 — a comparator must be one the
// implementation supports, declared via require). Rejection holds whether or
// not the script tries to declare the bogus name with require: an undeclared
// comparator is unavailable, and a require of an unsupported comparator
// capability is itself an error.
func TestComparator_UnknownRejected(t *testing.T) {
	bad := []struct {
		name   string
		script string
	}{
		{"bogus, undeclared, :is", `if header :is :comparator "i;bogus" "Subject" "x" { keep; }`},
		{"bogus, undeclared, :contains", `if header :contains :comparator "i;bogus" "Subject" "x" { keep; }`},
		{"bogus, undeclared, :matches", `if header :matches :comparator "i;bogus" "Subject" "x*" { keep; }`},
		{"bogus declared with require", `require "comparator-i;bogus";
if header :is :comparator "i;bogus" "Subject" "x" { keep; }`},
		{"bogus on address test", `if address :is :comparator "i;bogus" "From" "a@b.com" { keep; }`},
	}
	for _, tc := range bad {
		if err := Validate(tc.script); err == nil {
			t.Errorf("%s: expected an unknown comparator to be rejected, got nil:\n%s", tc.name, tc.script)
		}
	}
}

// TestComparator_AsciiNumericRejectsSubstringMatchTypes covers issue #19 part
// (b): i;ascii-numeric must not silently degrade to casemap folding when used
// with a substring match type. RFC 4790 §9.1 defines the comparator only for
// equality and ordering of decimal integers and gives it no substring
// operation, so combining it with :contains, :matches, or :regex is an error
// rather than a casemap match. (This package has no relational :value/:count
// extension, so :is is the only match type i;ascii-numeric applies to.)
func TestComparator_AsciiNumericRejectsSubstringMatchTypes(t *testing.T) {
	bad := []struct {
		name   string
		script string
	}{
		{"header :contains", `require "comparator-i;ascii-numeric";
if header :comparator "i;ascii-numeric" :contains "X-Priority" "5" { keep; }`},
		{"header :matches", `require "comparator-i;ascii-numeric";
if header :comparator "i;ascii-numeric" :matches "X-Priority" "5*" { keep; }`},
		{"header :regex", `require ["comparator-i;ascii-numeric", "regex"];
if header :comparator "i;ascii-numeric" :regex "X-Priority" "5.*" { keep; }`},
		{"comparator before match type", `require "comparator-i;ascii-numeric";
if header :contains :comparator "i;ascii-numeric" "X-Priority" "5" { keep; }`},
		{"address :contains", `require "comparator-i;ascii-numeric";
if address :comparator "i;ascii-numeric" :contains "From" "5" { keep; }`},
		{"envelope :matches", `require ["envelope", "comparator-i;ascii-numeric"];
if envelope :comparator "i;ascii-numeric" :matches "from" "5*" { keep; }`},
		{"body :contains", `require ["body", "comparator-i;ascii-numeric"];
if body :comparator "i;ascii-numeric" :contains "5" { keep; }`},
	}
	for _, tc := range bad {
		if err := Validate(tc.script); err == nil {
			t.Errorf("%s: expected i;ascii-numeric with a substring match type to be rejected, got nil:\n%s", tc.name, tc.script)
		}
	}
}

// TestComparator_ValidCombinationsStillParse guards against over-rejection: the
// supported comparators must keep working with the match types they do define.
// i;octet and i;ascii-casemap support every match type; i;ascii-numeric supports
// the equality match type :is.
func TestComparator_ValidCombinationsStillParse(t *testing.T) {
	good := []struct {
		name   string
		script string
	}{
		{"octet :is", `if header :is :comparator "i;octet" "Subject" "x" { keep; }`},
		{"octet :contains", `if header :contains :comparator "i;octet" "Subject" "x" { keep; }`},
		{"octet :matches", `if header :matches :comparator "i;octet" "Subject" "x*" { keep; }`},
		{"casemap :contains", `if header :contains :comparator "i;ascii-casemap" "Subject" "x" { keep; }`},
		{"casemap :matches", `if header :matches :comparator "i;ascii-casemap" "Subject" "x*" { keep; }`},
		{"ascii-numeric :is", `require "comparator-i;ascii-numeric";
if header :is :comparator "i;ascii-numeric" "X-Priority" "1" { keep; }`},
		{"ascii-numeric default match type is :is", `require "comparator-i;ascii-numeric";
if header :comparator "i;ascii-numeric" "X-Priority" "1" { keep; }`},
	}
	for _, tc := range good {
		if err := Validate(tc.script); err != nil {
			t.Errorf("%s: expected a valid comparator/match-type combination to parse, got: %v\n%s", tc.name, err, tc.script)
		}
	}
}

// TestComparator_AsciiNumericIsSemantics confirms the :is numeric semantics of
// i;ascii-numeric (RFC 4790 §9.1): equal decimal integers match, unequal ones do
// not, and every value that does not start with a digit is "positive infinity",
// so two non-numeric values are equal to each other but a numeric value never
// equals a non-numeric one.
func TestComparator_AsciiNumericIsSemantics(t *testing.T) {
	numericEq := func(headerVal, key string) bool {
		email := sieveEmail()
		email.Headers.Raw = map[string][]string{"X-Test": {headerVal}}
		script := `require ["fileinto", "comparator-i;ascii-numeric"];
if header :is :comparator "i;ascii-numeric" "X-Test" "` + key + `" { fileinto "Match"; }`
		return folderOf(runSieve(t, script, email)) == "Match"
	}

	cases := []struct {
		headerVal, key string
		want           bool
	}{
		{"5", "5", true},
		{"05", "5", true},    // leading zeros are not significant
		{"5", "6", false},    // unequal integers
		{"5", "5x", true},    // key truncated at first non-digit -> 5
		{"cat", "dog", true}, // both non-numeric -> both +infinity -> equal
		{"5", "cat", false},  // numeric vs infinity -> unequal
		{"cat", "5", false},  // infinity vs numeric -> unequal
	}
	for _, tc := range cases {
		if got := numericEq(tc.headerVal, tc.key); got != tc.want {
			t.Errorf("i;ascii-numeric :is (%q == %q): got match=%v, want %v", tc.headerVal, tc.key, got, tc.want)
		}
	}
}
