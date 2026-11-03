package rfctest

import (
	"slices"
	"testing"
)

const diffRelationalRequire = `require ["fileinto", "relational", "subaddress", "envelope", "variables", "comparator-i;ascii-numeric"]; `

// diffRelationalScripts are relational (RFC 5231) and subaddress (RFC 5233)
// scripts, alone and with variables (RFC 5229), that both implementations run
// on diffEmail. Each files into a folder naming what it found.
var diffRelationalScripts = func() []string {
	const req = diffRelationalRequire
	return []string{
		// A value that does not start with a digit, under i;ascii-numeric,
		// against another such value: both are positive infinity.
		req + `if header :value "eq" :comparator "i;ascii-numeric" "X-Mailer" "x" { fileinto "inf-eq"; }`,
		// :value with each comparator and operator.
		req + `if header :value "gt" :comparator "i;ascii-numeric" "X-Spam-Score" "9" { fileinto "gt"; }`,
		req + `if header :value "lt" :comparator "i;ascii-numeric" "X-Spam-Score" "9" { fileinto "lt"; } else { fileinto "nlt"; }`,
		req + `if header :value "ge" :comparator "i;ascii-numeric" "X-Spam-Score" "12" { fileinto "ge"; }`,
		req + `if header :value "le" :comparator "i;ascii-numeric" "X-Spam-Score" "012" { fileinto "le"; }`,
		req + `if header :value "eq" :comparator "i;ascii-numeric" "X-Spam-Score" "12" { fileinto "eq"; }`,
		req + `if header :value "ne" :comparator "i;ascii-numeric" "X-Spam-Score" "12" { fileinto "ne"; } else { fileinto "nne"; }`,
		req + `if header :value "gt" "Subject" "H" { fileinto "casemap"; }`,
		req + `if header :value "lt" :comparator "i;octet" "Subject" "a" { fileinto "octet"; }`,
		req + `if header :value "lt" :comparator "i;ascii-numeric" "X-Spam-Score" ["1", "100"] { fileinto "anypair"; }`,
		req + `if address :value "gt" :domain "To" "acme.example.c" { fileinto "addrvalue"; }`,
		// :count on header, address and envelope.
		req + `if header :count "eq" :comparator "i;ascii-numeric" "To" "1" { fileinto "hcount1"; }`,
		req + `if header :count "eq" :comparator "i;ascii-numeric" ["To", "Cc", "X-Missing"] "2" { fileinto "hcount2"; }`,
		req + `if address :count "eq" :comparator "i;ascii-numeric" ["To", "Cc"] "3" { fileinto "acount3"; }`,
		req + `if address :count "ge" :comparator "i;ascii-numeric" "To" "2" { fileinto "acountge"; }`,
		req + `if envelope :count "eq" :comparator "i;ascii-numeric" ["from", "to"] "2" { fileinto "ecount2"; }`,
		// :user and :detail.
		req + `if envelope :detail "to" "lists" { fileinto "detail"; }`,
		req + `if envelope :user "to" "roadrunner" { fileinto "user"; }`,
		req + `if envelope :user "from" "coyote" { fileinto "plainuser"; }`,
		req + `if envelope :detail :matches "from" "*" { fileinto "nodetail"; } else { fileinto "no-detail"; }`,
		// With variables.
		req + `if envelope :detail :matches "to" "l*" { fileinto "d-${1}"; }`,
		req + `if envelope :user :matches "to" "*runner" { fileinto "u-${1}"; }`,
		req + `set "n" "10"; if header :value "gt" :comparator "i;ascii-numeric" "X-Spam-Score" "${n}" { fileinto "var-key"; }`,
		req + `if string :count "eq" :comparator "i;ascii-numeric" ["a", "", "b"] "2" { fileinto "scount"; }`,
		req + `if string :value "lt" "abc" "abd" { fileinto "svalue"; }`,
		req + `if string :count "eq" :comparator "i;ascii-numeric" ["${unset}", "x"] "1" { fileinto "scount-unset"; }`,
		req + `if header :value "gt" :comparator "i;ascii-numeric" "X-Spam-Score" "1" { if header :matches "Subject" "I *" {} } if header :value "eq" "Subject" "x" {} fileinto "m-${1}";`,
		req + `if address :count "eq" :comparator "i;ascii-numeric" ["To", "From"] "3" { fileinto "acount-from"; }`,
		req + `if address :user :is "To" "beep" { fileinto "user-plain"; }`,
		req + `if address :detail :is "To" "" { fileinto "detail-empty"; } else { fileinto "detail-none"; }`,
		req + `if header :count "ne" :comparator "i;ascii-numeric" "Subject" "1" { fileinto "x"; } else { fileinto "hcount-ne"; }`,
	}
}()

// TestDifferentialRelationalSubaddress checks that this library and
// foxcpp/go-sieve deliver the message alike for the relational and
// subaddress scripts, including their use with variables.
//
// checklist: RELATIONAL-03 RELATIONAL-04 RELATIONAL-06 RELATIONAL-07 RELATIONAL-08 RELATIONAL-09 SUBADDRESS-03 SUBADDRESS-04 VARIABLES-18
func TestDifferentialRelationalSubaddress(t *testing.T) {
	for _, script := range diffRelationalScripts {
		want, refErr := reference(t, script)
		got, err := ours(script)
		if refErr != nil {
			t.Errorf("the reference refused a script this test expects it to run: %v\n%s", refErr, script)
			continue
		}
		if err != nil {
			t.Errorf("this library refused a script the reference runs: %v\n%s", err, script)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("actions differ\nscript: %s\nours:      %q\nreference: %q", script, got, want)
		}
	}
}

// TestDifferentialRelationalDepartures pins where foxcpp/go-sieve departs from
// the RFCs, with this library's result written out from them:
//
//   - RFC 5231 §4: the operator is an ABNF quoted string, which RFC 5234 §2.3
//     makes case-insensitive. The reference refuses "GT".
//   - RFC 4790 §9.1.1 (RFC 5231 §4.1): under i;ascii-numeric a value that does
//     not start with a digit is positive infinity, so it is greater than any
//     number. The reference finds "Acme Mailer 3000" not greater.
//   - RFC 5229 §3: with variables every string is expanded, the operator of
//     :value included. The reference refuses "${op}" at parse time.
//
// When the reference changes to agree, the test logs it so the script can
// move into TestDifferentialRelationalSubaddress.
//
// checklist: RELATIONAL-02 RELATIONAL-04 VARIABLES-11
func TestDifferentialRelationalDepartures(t *testing.T) {
	const req = diffRelationalRequire
	for _, tc := range []struct {
		why, script string
		want        []string
	}{
		{
			"operator case",
			req + `if header :value "GT" :comparator "i;ascii-numeric" "X-Spam-Score" "9" { fileinto "gt"; }`,
			[]string{"fileinto:gt"},
		},
		{
			"non-number is positive infinity",
			req + `if header :value "gt" :comparator "i;ascii-numeric" "X-Mailer" "99999999999999999999" { fileinto "infinity"; }`,
			[]string{"fileinto:infinity"},
		},
		{
			"operator from a variable",
			req + `set "op" "lt"; if header :value "${op}" :comparator "i;ascii-numeric" "X-Spam-Score" "13" { fileinto "var-op"; }`,
			[]string{"fileinto:var-op"},
		},
	} {
		got, err := ours(tc.script)
		if err != nil {
			t.Fatalf("%s: %v", tc.why, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: deliveries %q, want %q", tc.why, got, tc.want)
		}
		ref, refErr := reference(t, tc.script)
		if refErr == nil && slices.Equal(ref, tc.want) {
			t.Logf("%s: the reference now agrees; fold this script into TestDifferentialRelationalSubaddress", tc.why)
		} else {
			t.Logf("%s: the reference departs from the RFC: %q, %v", tc.why, ref, refErr)
		}
	}
}
