package sieve

import "testing"

// TestValidateSieve_RejectsDuplicateOrInapplicableTags covers RFC 5228 §2.7.1
// (a test carries a single match type), §2.7.3 (a single comparator), §2.7.4 /
// §5.7 (address parts apply only to address/envelope), §5.9 (size takes exactly
// one of :over/:under), and the general rule that a tagged argument not defined
// for a command is a syntax error. Previously these were silently accepted
// (last one wins, or the stray tag was consumed and ignored), masking script
// mistakes; each must now be a parse error.
func TestValidateSieve_RejectsDuplicateOrInapplicableTags(t *testing.T) {
	bad := []struct {
		name   string
		script string
	}{
		// Duplicate / conflicting match types.
		{"header two match types", `if header :is :contains "Subject" "y" { keep; }`},
		{"header repeated match type", `if header :is :is "Subject" "y" { keep; }`},
		{"address conflicting match types", `if address :contains :matches "From" "y" { keep; }`},
		{"body two match types", `require "body";
if body :is :contains "y" { keep; }`},

		// Duplicate comparator.
		{"header two comparators", `if header :is :comparator "i;octet" :comparator "i;octet" "Subject" "y" { keep; }`},

		// Duplicate / conflicting address part.
		{"address two address parts", `if address :localpart :domain "From" "y" { keep; }`},
		{"address repeated address part", `if address :all :all "From" "y" { keep; }`},

		// Address part on a test that does not accept one.
		{"header with address part", `if header :localpart "Subject" "y" { keep; }`},
		{"body with address part", `require "body";
if body :domain "y" { keep; }`},

		// Body transform on a test that does not accept one.
		{"header with body transform", `if header :text "Subject" "y" { keep; }`},

		// Wholly inapplicable tag.
		{"header with size relation", `if header :over "Subject" "y" { keep; }`},
		{"address with bogus tag", `if address :bogus "From" "y" { keep; }`},

		// size: both relations, or a repeated relation, or a foreign tag.
		{"size over and under", `if size :over :under 5 { keep; }`},
		{"size repeated over", `if size :over :over 5 { keep; }`},
		{"size with match type", `if size :is 5 { keep; }`},
	}
	for _, tc := range bad {
		if err := Validate(tc.script); err == nil {
			t.Errorf("%s: expected Validate to reject the script, but it parsed:\n%s", tc.name, tc.script)
		}
	}
}

// TestValidateSieve_AcceptsSingleTaggedArgs guards against over-rejection: every
// valid single-tag (or single-of-each-group) script must still parse.
func TestValidateSieve_AcceptsSingleTaggedArgs(t *testing.T) {
	good := []string{
		`if header :is "Subject" "y" { keep; }`,
		`if header :contains :comparator "i;octet" "Subject" "y" { keep; }`,
		`if address :localpart :contains "From" "admin" { keep; }`,
		`if address :domain :is "From" "example.com" { keep; }`,
		`require "envelope";
if envelope :all :is "from" "a@b.example" { keep; }`,
		`require "body";
if body :contains "urgent" { keep; }`,
		`require ["body", "regex"];
if body :regex "urgent.*" { keep; }`,
		`if size :over 1K { keep; }`,
		`if size :under 500 { keep; }`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("Validate rejected a valid single-tag script: %v\n%s", err, s)
		}
	}
}
