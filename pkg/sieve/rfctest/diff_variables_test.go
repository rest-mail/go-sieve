package rfctest

import (
	"slices"
	"strings"
	"testing"
)

// TestDifferentialVariables checks that this library and foxcpp/go-sieve
// deliver the message alike for scripts using the variables extension: set and
// its modifiers, interpolation, match variables and the string test. Each
// script files into a folder named from variables, so the comparison sees the
// expanded values.
//
// checklist: VARIABLES-02 VARIABLES-07 VARIABLES-08 VARIABLES-12 VARIABLES-13 VARIABLES-14
func TestDifferentialVariables(t *testing.T) {
	const req = `require ["fileinto", "variables"]; `
	scripts := []string{
		req + `set "a" "x"; fileinto "f-${a}-${A}-${unset}-";`,
		req + `set "company" "ACME"; fileinto "${BAD${Company}";`,
		req + `set "company" "ACME"; fileinto "${President, ${Company} Inc.}";`,
		req + `set "dollar" "$"; set "text" "regarding ${dollar}{beep}"; fileinto "${text}";`,
		req + `set "a" "juMBlEd lETteRS"; set :length "b" "${a}"; fileinto "${b}";`,
		req + `set "a" "juMBlEd lETteRS"; set :lower "b" "${a}"; fileinto "${b}";`,
		req + `set "a" "juMBlEd lETteRS"; set :upper "b" "${a}"; fileinto "${b}";`,
		req + `set "a" "juMBlEd lETteRS"; set :upperfirst "b" "${a}"; fileinto "${b}";`,
		req + `set "a" "juMBlEd lETteRS"; set :lowerfirst :upper "b" "${a}"; fileinto "${b}";`,
		req + `set "a" "juMBlEd lETteRS"; set :upperfirst :lower "b" "${a}"; fileinto "${b}";`,
		req + `set :quotewildcard "b" "Rock*?\\"; fileinto "${b}";`,
		req + `set :quotewildcard :length "b" "Rock*?"; fileinto "${b}";`,
		req + `if header :matches "Subject" "I * a *" { fileinto "${1}|${2}|${0}"; }`,
		req + `if header :matches "Subject" "*e*" { fileinto "${1}|${2}"; }`,
		req + `if header :matches "Subject" "?*?" { fileinto "${1}|${2}|${3}|${4}"; }`,
		req + `if address :matches "From" "*@*.*" { fileinto "${1}|${2}|${3}"; }`,
		req + `if address :domain :matches "Cc" "*.example.*" { fileinto "${1}|${2}"; }`,
		req + `if header :matches "X-Mailer" "acme * 3*" { fileinto "${01}|${2}"; }`,
		req + `if header :matches "Subject" "nothing*" { fileinto "no"; } else { fileinto "x${1}x"; }`,
		req + `if header :matches "Subject" "I*" {} if header :matches "Subject" "zzz*" {} fileinto "${1}";`,
		req + `if anyof (true, header :matches "Subject" "I*") {} fileinto "x${1}x";`,
		req + `if allof (header :matches "Subject" "I*", header :matches "X-Mailer" "A*") {} fileinto "${1}";`,
		req + `set "state" "${state} pending"; if string :matches " ${state} " "* pending *" { fileinto "always|${1}|${2}"; }`,
		req + `if string :is "${missing}" "" { fileinto "empty"; }`,
		req + `if string " a " "a" { fileinto "stripped"; } else { fileinto "unstripped"; }`,
		req + `if string ["x", "y"] ["z", "Y"] { fileinto "any"; }`,
		req + `if string :contains :comparator "i;octet" "Hello" "hell" { fileinto "case"; } else { fileinto "octet"; }`,
		req + `set "a" "1"; fileinto "f${a}"; set "a" "${a}2"; fileinto "f${a}";`,
	}
	for _, script := range scripts {
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

// TestDifferentialVariablesDepartures pins where foxcpp/go-sieve departs from
// RFC 5229, with this library's result written out from the RFC:
//
//   - §3: "Only one pass through the string SHALL be done." The reference
//     expands a match key twice, so a value holding "${b}" is expanded again.
//   - RFC 5228 §2.7.1: "?" matches a single character, a line feed included.
//     The reference's "?" does not match a line feed.
//   - §6: each variable holds at least 4000 characters. The reference cuts
//     values at 4000 octets, so 4000 two-octet characters lose half.
//
// When the reference changes to agree, the test logs it so the script can
// move into TestDifferentialVariables.
//
// checklist: VARIABLES-03 VARIABLES-07 VARIABLES-15
func TestDifferentialVariablesDepartures(t *testing.T) {
	const req = `require ["fileinto", "variables", "encoded-character"]; `
	for _, tc := range []struct {
		why, script string
		want        []string
	}{
		{
			"one pass",
			req + `set "b" "x"; set "dollar" "$"; set "a" "${dollar}{b}";
			if string :is "${a}" "${a}" { fileinto "same"; } else { fileinto "differ"; }`,
			[]string{"fileinto:same"},
		},
		{
			"? matches a line feed",
			req + `if string :matches "a${hex:0A}b" "a?b" { fileinto "lf"; } else { fileinto "no-lf"; }`,
			[]string{"fileinto:lf"},
		},
		{
			"4000 characters, not octets",
			req + `set "v" "` + strings.Repeat("é", 4000) + `"; set :length "n" "${v}"; fileinto "n${n}";`,
			[]string{"fileinto:n4000"},
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
			t.Logf("%s: the reference now agrees; fold this script into TestDifferentialVariables", tc.why)
		} else {
			t.Logf("%s: the reference departs from RFC 5229: %q, %v", tc.why, ref, refErr)
		}
	}
}
