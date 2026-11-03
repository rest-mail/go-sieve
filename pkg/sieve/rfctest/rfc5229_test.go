package rfctest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// These tests check RFC 5229 (the "variables" extension) clause by clause.
// Each script files into a folder built from the string under test, so the
// folder the host is asked for is the expanded string.

// varsReq declares the capabilities the variables tests use.
const varsReq = `require ["fileinto", "variables"];` + "\n"

// expanded runs script and returns the folder it filed into.
func expanded(t *testing.T, script string, msg *sieve.Message) string {
	t.Helper()
	if msg == nil {
		msg = sieveEmail()
	}
	s, err := sieve.Parse(crlf(script))
	if err != nil {
		t.Fatalf("Parse(crlf(%q)): %v", script, err)
	}
	ex := newRecExec()
	if out := s.Evaluate(msg, ex); out.Error != nil {
		t.Fatalf("Evaluate(%q): %v", script, out.Error)
	}
	return ex.meta["deliver_to_folder"]
}

// headerMessage is a message carrying one raw header, which replaces the
// base message's Subject when it is a Subject.
func headerMessage(name, value string) *sieve.Message {
	msg := sieveEmail()
	if strings.EqualFold(name, "Subject") {
		msg.Headers.Subject = ""
	}
	msg.Headers.Raw = map[string][]string{name: {value}}
	return msg
}

// TestVariables_RequiresCapability checks that the capability string is
// "variables", that set and string are errors without it (RFC 5229 §2; RFC
// 5228 §3.2), and that without it ${name} is ordinary text.
//
// checklist: VARIABLES-01
func TestVariables_RequiresCapability(t *testing.T) {
	for _, s := range []string{
		`require "variables"; set "a" "b";`,
		`require "variables"; if string "a" "a" { keep; }`,
	} {
		if err := sieve.Validate(crlf(s)); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{
		`set "a" "b";`,
		`if string "a" "a" { keep; }`,
		`if false { set "a" "b"; }`,
		`require "variable"; set "a" "b";`,
	} {
		if err := sieve.Validate(crlf(s)); err == nil {
			t.Errorf("%q: accepted without require \"variables\", want an error", s)
		}
	}
	// Without the extension nothing is expanded, and a namespaced or
	// out-of-range reference is just text.
	for _, folder := range []string{"${x}", "${a.b}", "${10}"} {
		if got := expanded(t, `require "fileinto"; fileinto "`+folder+`";`, nil); got != folder {
			t.Errorf("fileinto %q without variables = %q, want it unchanged", folder, got)
		}
	}
}

// TestVariables_Interpolation checks the §3 rules and every §3 example: names
// are case-insensitive, unknown variables are empty, a malformed reference is
// left verbatim, and the string is scanned once.
//
// checklist: VARIABLES-02
func TestVariables_Interpolation(t *testing.T) {
	setup := varsReq + `set "company" "ACME";` + "\n"
	cases := []struct{ in, want string }{
		// RFC 5229 §3 examples.
		{`&%${}!`, `&%${}!`},
		{`${doh!}`, `${doh!}`},
		{`${full}`, ``},
		{`${company}`, `ACME`},
		{`${BAD${Company}`, `${BADACME`},
		{`${President, ${Company} Inc.}`, `${President, ACME Inc.}`},
		// Names are case-insensitive.
		{`${COMPANY}-${CoMpAnY}`, `ACME-ACME`},
		// Not identifiers: a leading digit mixed with letters, a dash, a
		// space, an unterminated reference.
		{`${1a}`, `${1a}`},
		{`${com-pany}`, `${com-pany}`},
		{`${ company}`, `${ company}`},
		{`${company`, `${company`},
		{`$company`, `$company`},
		{`${_}x`, `x`},
	}
	for _, c := range cases {
		if got := expanded(t, setup+`fileinto "`+c.in+`";`, nil); got != c.want {
			t.Errorf("%q expanded to %q, want %q", c.in, got, c.want)
		}
	}

	// One pass only: a value that itself looks like a reference stays text.
	one := varsReq + `set "dollar" "$"; set "ref" "${dollar}{company}"; set "company" "ACME"; fileinto "${ref}";`
	if got := expanded(t, one, nil); got != "${company}" {
		t.Errorf("one pass: got %q, want ${company}", got)
	}
	// The value current when control reaches the statement is used.
	current := varsReq + `set "a" "1"; set "a" "${a}2"; set "b" "${a}"; set "a" "x"; fileinto "${b}${a}";`
	if got := expanded(t, current, nil); got != "12x" {
		t.Errorf("current values: got %q, want 12x", got)
	}
}

// TestVariables_QuotingAndEncoding checks §3.1: backslash quoting and
// encoded-character are resolved before substitution, with every example.
//
// checklist: VARIABLES-03
func TestVariables_QuotingAndEncoding(t *testing.T) {
	setup := varsReq + `set "foo" "FOO";` + "\n"
	cases := []struct{ in, want string }{
		{`${fo\o}`, `FOO`},
		{`${fo\\o}`, `${fo\o}`},
		{`\${foo}`, `FOO`},
		{`\\${foo}`, `\FOO`},
	}
	for _, c := range cases {
		if got := expanded(t, setup+`fileinto "`+c.in+`";`, nil); got != c.want {
			t.Errorf("%s expanded to %q, want %q", c.in, got, c.want)
		}
	}
	dollar := varsReq + `set "dollar" "$"; set "text" "regarding ${dollar}{beep}"; fileinto "${text}";`
	if got := expanded(t, dollar, nil); got != "regarding ${beep}" {
		t.Errorf("dollar example: got %q", got)
	}
	// The §3.1 encoded-character example (with the semicolon the RFC's
	// text omits after the set).
	enc := `require ["encoded-character", "variables", "fileinto"];
set "name" "Ethelbert";
if header :contains "Subject" "dear${hex:20 24 7b 4e}ame}" {
    fileinto "Hit";
}`
	if got := expanded(t, enc, headerMessage("Subject", "Hello dear Ethelbert, how are you")); got != "Hit" {
		t.Errorf("encoded-character example did not match: %q", got)
	}
	if got := expanded(t, enc, headerMessage("Subject", "dear ${Name}")); got == "Hit" {
		t.Error("encoded-character example matched the unexpanded text")
	}
}

// TestVariables_Namespaces checks that a reference to a namespace no
// required extension defines is an error (§3). No extension this package
// implements defines a namespace.
//
// checklist: VARIABLES-04
func TestVariables_Namespaces(t *testing.T) {
	for _, s := range []string{
		varsReq + `fileinto "${a.b}";`,
		varsReq + `fileinto "${envelope.from}";`,
		varsReq + `fileinto "${variables.x}";`,
		varsReq + `fileinto "${a.1.b}";`,
		varsReq + `fileinto "${a.1}";`,
		varsReq + `if header :is "${ns.x}" "v" { keep; }`,
		varsReq + `if false { set "x" "${ns.x}"; }`,
	} {
		if err := sieve.Validate(crlf(s)); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
	// Not namespaces: a namespace must start with an identifier, and an
	// empty component is not a variable-name, so these are text.
	for _, in := range []string{`${1.a}`, `${a..b}`, `${.a}`, `${a.}`} {
		if got := expanded(t, varsReq+`fileinto "`+in+`";`, nil); got != in {
			t.Errorf("%q expanded to %q, want it verbatim", in, got)
		}
	}
}

// TestVariables_MatchVariables checks §3.2 with every example: :matches sets
// ${0} to the matched value and ${1}.. to what each wildcard matched,
// non-greedily; an index out of range is empty; and match variables change
// only on a successful :matches.
//
// checklist: VARIABLES-05
func TestVariables_MatchVariables(t *testing.T) {
	listID := varsReq + `if header :matches "List-ID" "*<*@*" { fileinto "INBOX.lists.${2}"; stop; }`
	if got := expanded(t, listID, headerMessage("List-ID", "ACME users <acme-users@lists.example.com>")); got != "INBOX.lists.acme-users" {
		t.Errorf("List-ID example: %q", got)
	}
	subject := varsReq + `if header :matches "Subject" "[*] *" { fileinto "${1}|${2}"; stop; }`
	if got := expanded(t, subject, headerMessage("Subject", "[acme-users] [fwd] version 1.0 is out")); got != "acme-users|[fwd] version 1.0 is out" {
		t.Errorf("Subject example: %q", got)
	}
	coyote := sieveEmail()
	coyote.Headers.To = []sieve.Address{{Address: "coyote@ACME.Example.COM"}}
	addr := varsReq + `if address :matches ["To", "Cc"] ["coyote@**.com", "wile@**.com"] {
    fileinto "${0}|${1}|${2}"; stop;
} else {
    fileinto "else:${0}${1}";
}`
	if got := expanded(t, addr, coyote); got != "coyote@ACME.Example.COM||ACME.Example" {
		t.Errorf("address example: %q", got)
	}
	other := sieveEmail()
	other.Headers.To = []sieve.Address{{Address: "roadrunner@example.org"}}
	if got := expanded(t, addr, other); got != "else:" {
		t.Errorf("address example, no match: %q, want no match variables", got)
	}
	shortCircuit := varsReq + `if anyof (true, address :domain :matches "To" "*.com") { fileinto "x${0}${1}"; stop; }`
	if got := expanded(t, shortCircuit, coyote); got != "x" {
		t.Errorf("anyof short circuit: %q, want no match variables", got)
	}

	msg := headerMessage("Subject", "alpha beta")
	cases := []struct {
		name, script, want string
	}{
		{"leading zeros", `if header :matches "Subject" "* *" { fileinto "${01}-${0002}-${000}"; }`, "alpha-beta-alpha beta"},
		{"index out of range", `if header :matches "Subject" "* *" { fileinto "[${3}${9}]"; }`, "[]"},
		{"question mark", `if header :matches "Subject" "?lph? *" { fileinto "${1}${2}${3}"; }`, "aabeta"},
		{"escaped wildcard is not a group", `if header :matches "Subject" "alpha\\*" { fileinto "no"; } else { fileinto "[${1}]"; }`, "[]"},
		{"failed match keeps them",
			`if header :matches "Subject" "* *" { } if header :matches "Subject" "x*" { } fileinto "${1}";`, "alpha"},
		{":is keeps them",
			`if header :matches "Subject" "* *" { } if header :is "Subject" "alpha beta" { fileinto "${2}"; }`, "beta"},
		{"later match replaces them",
			`if header :matches "Subject" "* *" { } if header :matches "Subject" "*a" { fileinto "${1}|${2}"; }`, "alpha bet|"},
		{"keys use the values current before the test",
			`if header :matches "Subject" "* *" { } if header :matches "Subject" "${1}*" { fileinto "${1}"; }`, " beta"},
		{"string test sets them", `if string :matches "a-b" "*-*" { fileinto "${2}${1}"; }`, "ba"},
		{"nine groups", `if string :matches "abcdefghijk" "?????????*" { fileinto "${9}${1}${0}"; }`, "iaabcdefghijk"},
		{"case-insensitive groups keep case", `if string :matches "ABC" "a*" { fileinto "${1}"; }`, "BC"},
		{"more wildcards than ${9}", `if string :matches "abcdefghijkl" "???????????*" { fileinto "${9}"; }`, "i"},
		{"? needs a character", `if string :matches "ab" "ab?" { fileinto "no"; } else { fileinto "[${0}]"; }`, "[]"},
		{"backtracking failure", `if string :matches "aaaaaaaa" "*a*a*a*b" { fileinto "no"; } else { fileinto "[${0}]"; }`, "[]"},
		{"trailing backslash is literal", `if string :matches "a\\" "*\\" { fileinto "${1}"; }`, "a"},
	}
	for _, c := range cases {
		if got := expanded(t, varsReq+c.script, msg); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	regexCases := []struct {
		name, script, want string
	}{
		{"octet comparator keeps case", `if string :regex :comparator "i;octet" "ABC" "^(a)" { fileinto "no"; } else { fileinto "[${1}]"; }`, "[]"},
		{"unmatched group is empty", `if string :regex "ABC" "(x)?(B)" { fileinto "[${1}|${2}]"; }`, "[|B]"},
		{"invalid expression does not match", `if string :regex "a" "(" { fileinto "no"; } else { fileinto "[${0}]"; }`, "[]"},
	}
	for _, c := range regexCases {
		if got := expanded(t, `require ["fileinto", "variables", "regex"];`+c.script, msg); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	// Only ${0}..${9} are supported: a higher index is a syntax error.
	for _, ref := range []string{`${10}`, `${010}`, `${99999999999999999999}`} {
		if err := sieve.Validate(crlf(varsReq + `fileinto "` + ref + `";`)); err == nil {
			t.Errorf("%s: accepted, want an error", ref)
		}
	}
	if err := sieve.Validate(crlf(varsReq + `fileinto "${0009}";`)); err != nil {
		t.Errorf("${0009}: %v", err)
	}

	// :regex sets ${0} to the matched text and ${1}.. to its groups.
	re := `require ["fileinto", "variables", "regex"]; if header :regex "Subject" "^(a[a-z]+) (b)" { fileinto "${0}|${1}|${2}|${3}"; }`
	if got := expanded(t, re, msg); got != "alpha b|alpha|b|" {
		t.Errorf("regex groups: %q", got)
	}
}

// TestVariables_BodyWildcards checks that a body test's wildcards set no
// match variables (RFC 5173 §6).
//
// checklist: BODY-17
func TestVariables_BodyWildcards(t *testing.T) {
	script := `require ["fileinto", "variables", "body"];
if header :matches "Subject" "* *" { }
if body :matches "Hello, * a test*" { fileinto "${1}"; }`
	if got := expanded(t, script, headerMessage("Subject", "alpha beta")); got != "alpha" {
		t.Errorf("body :matches changed the match variables: %q", got)
	}
}

// TestVariables_Set checks the §4 set action and its example: values are
// stored under case-insensitive names, and set does not affect the
// implicit keep.
//
// checklist: VARIABLES-06
func TestVariables_Set(t *testing.T) {
	script := "require [\"fileinto\", \"variables\"];\n" +
		"set \"honorific\"  \"Mr\";\n" +
		"set \"first_name\" \"Wile\";\n" +
		"set \"last_name\"  \"Coyote\";\n" +
		"set \"vacation\" text:\n" +
		"Dear ${HONORIFIC} ${last_name},\n" +
		"I'm out, please leave a message after the meep.\n" +
		".\n" +
		";\n" +
		"fileinto \"${vacation}\";\n"
	want := "Dear Mr Coyote,\r\nI'm out, please leave a message after the meep.\r\n"
	if got := expanded(t, script, nil); got != want {
		t.Errorf("set example: got %q, want %q", got, want)
	}
	r := runSieve(t, `require "variables"; set "a" "b"; set "A" "c";`, sieveEmail())
	if !r.implicitKeep || r.disposition != sieve.Continue {
		t.Errorf("set changed the implicit keep: %+v", r)
	}
	if got := expanded(t, varsReq+`set "Name" "one"; set "NAME" "two"; fileinto "${name}";`, nil); got != "two" {
		t.Errorf("case-insensitive set: %q", got)
	}
}

// TestVariables_SetName checks that the name is a constant identifier, not a
// match variable and without a namespace, and that an invalid name is a
// syntax error (§4).
//
// checklist: VARIABLES-07
func TestVariables_SetName(t *testing.T) {
	for _, name := range []string{"a", "_", "_x9", "A_B_C", strings.Repeat("n", 200)} {
		if err := sieve.Validate(crlf(varsReq + `set "` + name + `" "v";`)); err != nil {
			t.Errorf("set %q: %v", name, err)
		}
	}
	for _, name := range []string{"", "1", "0", "01", "a.b", "ns.x", "${a}", "a-b", "a b", "9a", "é"} {
		if err := sieve.Validate(crlf(varsReq + `set "` + name + `" "v";`)); err == nil {
			t.Errorf("set %q: accepted, want an error", name)
		}
	}
	for _, s := range []string{
		varsReq + `set "a";`,
		varsReq + `set "a" "b" "c";`,
		varsReq + `set ["a"] "b";`,
		varsReq + `set "a" ["b"];`,
		varsReq + `set 1 "b";`,
	} {
		if err := sieve.Validate(crlf(s)); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestVariables_Modifiers checks §4.1 with every example: modifiers apply in
// precedence order, two of the same precedence are an error, unknown ones
// are an error, and names are case-insensitive.
//
// checklist: VARIABLES-08
func TestVariables_Modifiers(t *testing.T) {
	setup := varsReq + `set "a" "juMBlEd lETteRS";` + "\n"
	cases := []struct{ set, want string }{
		// RFC 5229 §4.1 examples.
		{`set "b" "${a}";`, "juMBlEd lETteRS"},
		{`set :length "b" "${a}";`, "15"},
		{`set :lower "b" "${a}";`, "jumbled letters"},
		{`set :upperfirst "b" "${a}";`, "JuMBlEd lETteRS"},
		{`set :upperfirst :lower "b" "${a}";`, "Jumbled letters"},
		{`set :quotewildcard "b" "Rock*";`, `Rock\*`},
		// Order on the line does not matter, precedence does.
		{`set :lower :upperfirst "b" "${a}";`, "Jumbled letters"},
		{`set :upper :lowerfirst "b" "${a}";`, "jUMBLED LETTERS"},
		{`set :length :quotewildcard "b" "Rock*";`, "6"},
		{`set :quotewildcard :upper "b" "a?b\\c*";`, `A\?B\\C\*`},
		{`set :LOWER "b" "ABC";`, "abc"},
		{`set :Length "b" "";`, "0"},
		// Only US-ASCII letters change case; :length counts characters.
		{`set :upper "b" "éa";`, "éA"},
		{`set :upperfirst "b" "éa";`, "éa"},
		{`set :lowerfirst "b" "1A";`, "1A"},
		{`set :length "b" "日本語";`, "3"},
	}
	for _, c := range cases {
		if got := expanded(t, setup+c.set+` fileinto "${b}";`, nil); got != c.want {
			t.Errorf("%s: got %q, want %q", c.set, got, c.want)
		}
	}
	for _, s := range []string{
		`set :lower :upper "b" "x";`,
		`set :lower :lower "b" "x";`,
		`set :lowerfirst :upperfirst "b" "x";`,
		`set :length :length "b" "x";`,
		`set :quotewildcard :quotewildcard "b" "x";`,
		`set :reverse "b" "x";`,
		`set :lower "b" :upper "x";`,
	} {
		if err := sieve.Validate(crlf(varsReq + s)); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
	// A quoted value matches only literally under :matches.
	lit := varsReq + `set :quotewildcard "q" "a*b?\\"; if string :matches "a*b?\\" "${q}" { fileinto "lit"; }
if string :matches "aXXbY\\" "${q}" { fileinto "wild"; }`
	if got := expanded(t, lit, nil); got != "lit" {
		t.Errorf(":quotewildcard: %q", got)
	}
}

// TestVariables_StringTest checks §5: both sides come from the script, the
// match type defaults to :is, no whitespace is stripped, and :count counts
// non-empty strings.
//
// checklist: VARIABLES-09
func TestVariables_StringTest(t *testing.T) {
	req := `require ["fileinto", "variables", "relational", "comparator-i;ascii-numeric"];` + "\n"
	cases := []struct {
		name, script string
		want         bool
	}{
		{"RFC example", `set "state" "${state} pending"; if string :matches " ${state} " "* pending *" { fileinto "Hit"; }`, true},
		{"default :is", `if string "abc" "ABC" { fileinto "Hit"; }`, true},
		{":is is not :contains", `if string "abc" "b" { fileinto "Hit"; }`, false},
		{"no whitespace stripping", `if string " a " "a" { fileinto "Hit"; }`, false},
		{"whitespace kept", `if string :is " a " " a " { fileinto "Hit"; }`, true},
		{"any source any key", `set "x" "2"; if string ["1", "${x}"] ["3", "2"] { fileinto "Hit"; }`, true},
		{"octet comparator", `if string :comparator "i;octet" "abc" "ABC" { fileinto "Hit"; }`, false},
		{":contains", `set "x" "mailing"; if string :contains "${x}" "ail" { fileinto "Hit"; }`, true},
		{":value", `set "n" "10"; if string :value "gt" :comparator "i;ascii-numeric" "${n}" "9" { fileinto "Hit"; }`, true},
		{":count empty string is 0", `if string :count "eq" :comparator "i;ascii-numeric" "${unset}" "0" { fileinto "Hit"; }`, true},
		{":count non-empty is 1", `if string :count "eq" :comparator "i;ascii-numeric" "x" "1" { fileinto "Hit"; }`, true},
		{":count sums the list", `set "a" "x"; if string :count "eq" :comparator "i;ascii-numeric" ["${a}", "", "${b}", "y"] "2" { fileinto "Hit"; }`, true},
		{"unset is empty", `if string :is "${nothing}" "" { fileinto "Hit"; }`, true},
	}
	for _, c := range cases {
		if got := expanded(t, req+c.script, nil) == "Hit"; got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	for _, s := range []string{
		`if string "a" { keep; }`,
		`if string :localpart "a" "a" { keep; }`,
		`if string :is :is "a" "a" { keep; }`,
		`if string :is { keep; }`,
	} {
		if err := sieve.Validate(crlf(req + s)); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestVariables_Limits checks §6: at least 128 distinct variables, names of
// at least 32 characters and values of at least 4000 characters; a constant
// that is too long is a compile-time error and a value built at run time is
// truncated, not an error.
//
// checklist: VARIABLES-10
func TestVariables_Limits(t *testing.T) {
	var b strings.Builder
	b.WriteString(varsReq)
	for i := 0; i < sieve.MaxVariables; i++ {
		fmt.Fprintf(&b, "set \"v%d\" \"%d\";\n", i, i)
	}
	b.WriteString(`set "V0" "again";`)
	if err := sieve.Validate(crlf(b.String())); err != nil {
		t.Fatalf("%d variables: %v", sieve.MaxVariables, err)
	}
	if sieve.MaxVariables < 128 {
		t.Errorf("MaxVariables = %d, RFC 5229 requires at least 128", sieve.MaxVariables)
	}
	if err := sieve.Validate(crlf(b.String() + `set "one_too_many" "x";`)); err == nil {
		t.Errorf("%d distinct variables accepted", sieve.MaxVariables+1)
	}

	long := strings.Repeat("x", 32)
	if got := expanded(t, varsReq+`set "`+long+`" "ok"; fileinto "${`+strings.ToUpper(long)+`}";`, nil); got != "ok" {
		t.Errorf("32-character name: %q", got)
	}

	value4000 := strings.Repeat("é", 4000)
	if got := expanded(t, varsReq+`set "v" "`+value4000+`"; fileinto "${v}";`, nil); got != value4000 {
		t.Errorf("4000-character value not held: %d octets", len(got))
	}
	tooLong := strings.Repeat("x", sieve.MaxVariableValue+1)
	if err := sieve.Validate(crlf(varsReq + `set "v" "` + tooLong + `";`)); err == nil {
		t.Error("constant value longer than MaxVariableValue accepted")
	}
	if err := sieve.Validate(crlf(varsReq + `set :length "v" "` + tooLong + `";`)); err != nil {
		t.Errorf(":length of a long constant is short: %v", err)
	}

	half := strings.Repeat("y", sieve.MaxVariableValue/2+1)
	grow := varsReq + `set "h" "` + half + `"; set "v" "${h}${h}"; set :length "n" "${v}"; fileinto "${n}";`
	if got := expanded(t, grow, nil); got != fmt.Sprint(sieve.MaxVariableValue) {
		t.Errorf("run-time value not truncated to %d: length %s", sieve.MaxVariableValue, got)
	}
	// Truncation never splits a UTF-8 character.
	wide := strings.Repeat("é", sieve.MaxVariableValue/4+1)
	utf := varsReq + `set "h" "` + wide + `"; set "v" "x${h}${h}"; fileinto "${v}";`
	got := expanded(t, utf, nil)
	if len(got) > sieve.MaxVariableValue || !strings.HasSuffix(got, "é") || strings.ContainsRune(got, '�') {
		t.Errorf("truncated value is %d octets, ends %q", len(got), got[len(got)-3:])
	}
}

// vacationExec records the Vacation callback in full.
type vacationExec struct {
	*recExec
	got []sieve.Vacation
}

func (e *vacationExec) Vacation(v sieve.Vacation) { e.got = append(e.got, v) }

// TestVariables_Vacation checks that vacation's arguments are expanded
// before the host derives the response-tracking identity (RFC 5230 §4.2),
// and that a :from built from variables is checked once expanded.
//
// checklist: VACATION-21
func TestVariables_Vacation(t *testing.T) {
	script := `require ["vacation", "variables"];
set "who" "Coyote";
if header :matches "Subject" "* *" { }
vacation :handle "away-${who}" :subject "Re: ${1}" :from "${who} <c@example.com>" :addresses ["${who}@example.org"] "Dear ${2}, ${who} is away";`
	msg := headerMessage("Subject", "Meeting tomorrow")
	msg.Headers.To = []sieve.Address{{Address: "Coyote@example.org"}}
	msg.Envelope.To = []string{"other@example.org"}
	s, err := sieve.Parse(crlf(script))
	if err != nil {
		t.Fatal(err)
	}
	ex := &vacationExec{recExec: newRecExec()}
	if out := s.Evaluate(msg, ex); out.Error != nil {
		t.Fatal(out.Error)
	}
	if len(ex.got) != 1 {
		t.Fatalf("vacation calls = %d, want 1 (the expanded :addresses make it personal)", len(ex.got))
	}
	v := ex.got[0]
	if v.Handle != "away-Coyote" || v.Subject != "Re: Meeting" || v.From != "Coyote <c@example.com>" || v.Body != "Dear tomorrow, Coyote is away" {
		t.Errorf("vacation = %+v", v)
	}

	bad := `require ["vacation", "variables"]; set "f" "not a mailbox <"; vacation :from "${f}" "away";`
	s, err = sieve.Parse(crlf(bad))
	if err != nil {
		t.Fatalf("Parse: %v (a :from from variables is checked when expanded)", err)
	}
	out := s.Evaluate(msg, &vacationExec{recExec: newRecExec()})
	if out.Error == nil || !out.ImplicitKeep {
		t.Errorf("invalid expanded :from: %+v, want a run-time error with the implicit keep", out)
	}
}

// TestVariables_Actions checks that the other actions and tests use the
// expanded strings, and that redirect checks an expanded address.
//
// checklist: VARIABLES-11
func TestVariables_Actions(t *testing.T) {
	script := `require ["fileinto", "variables", "reject", "envelope", "copy"];
set "box" "Lists";
set "hdr" "x-list";
set "part" "to";
set "rcpt" "@example.com";
if allof (exists "${hdr}", header :contains "${hdr}" "sieve", envelope :domain :is "${part}" "example.com") {
    fileinto "${box}.sieve";
    fileinto "${box}.sieve";
    redirect :copy "list${rcpt}";
}`
	msg := headerMessage("X-List", "the sieve list")
	r := runSieve(t, script, msg)
	if got := folderOf(r); got != "Lists.sieve" {
		t.Errorf("folder = %q", got)
	}
	if got := r.meta["redirect_to"]; got != "list@example.com" {
		t.Errorf("redirect = %q", got)
	}

	rej := runSieve(t, `require ["reject", "variables"]; set "why" "full"; reject "Mailbox ${why}";`, sieveEmail())
	if rej.disposition != sieve.Reject || rej.reject != "Mailbox full" {
		t.Errorf("reject = %+v", rej)
	}

	// A constant redirect address is still checked by Parse; one built
	// from variables is checked when it runs, and a bad one keeps the
	// message.
	if err := sieve.Validate(`require "variables"; redirect "not an address";`); err == nil {
		t.Error("invalid constant redirect accepted")
	}
	s, err := sieve.Parse(`require "variables"; set "a" "a@b, c@d"; redirect "${a}";`)
	if err != nil {
		t.Fatal(err)
	}
	ex := newRecExec()
	out := s.Evaluate(sieveEmail(), ex)
	if out.Error == nil || !out.ImplicitKeep || ex.meta["redirect_to"] != "" {
		t.Errorf("invalid expanded redirect: %+v, redirected to %q", out, ex.meta["redirect_to"])
	}
	// An envelope part built from variables that names no part has no
	// value; a constant unknown part is still refused.
	if got := expanded(t, `require ["fileinto", "variables", "envelope"]; set "p" "bogus"; if envelope :is "${p}" "" { fileinto "Hit"; }`, nil); got == "Hit" {
		t.Error("unknown expanded envelope part matched")
	}
	if err := sieve.Validate(`require ["variables", "envelope"]; if envelope "bogus" "" { keep; }`); err == nil {
		t.Error("unknown constant envelope part accepted")
	}
}

// TestVariables_ComparatorAndRelation checks that a comparator name and the
// relational operator of :value and :count are expanded like every other
// string (RFC 5229 §3). A constant one is still checked by Parse; one built
// from variables is checked once expanded, when its test runs, and a bad one
// is a run-time error that keeps the message (RFC 5228 §2.10.6).
//
// checklist: VARIABLES-12
func TestVariables_ComparatorAndRelation(t *testing.T) {
	const req = `require ["fileinto", "variables", "relational", "envelope", "body", "comparator-i;ascii-numeric"];` + "\n"
	run := func(t *testing.T, script string) (sieve.Outcome, string) {
		t.Helper()
		s, err := sieve.Parse(crlf(script))
		if err != nil {
			t.Fatalf("Parse(%q): %v", script, err)
		}
		ex := newRecExec()
		return s.Evaluate(headerMessage("X-Priority", "3"), ex), ex.meta["deliver_to_folder"]
	}
	for _, tc := range []struct {
		name, script string
		hit          bool
	}{
		{"comparator", `set "c" "i;octet"; if header :is :comparator "${c}" "X-Priority" "3" { fileinto "Hit"; }`, true},
		{"comparator in ASCII case", `set "c" "I;Octet"; if header :is :comparator "${c}" "X-Priority" "3" { fileinto "Hit"; }`, true},
		{"comparator built from parts", `set "n" "numeric"; if header :value "gt" :comparator "i;ascii-${n}" "X-Priority" "20" { fileinto "Hit"; }`, false},
		{"expanded comparator orders numerically", `set "c" "i;ascii-numeric"; if header :value "lt" :comparator "${c}" "X-Priority" "20" { fileinto "Hit"; }`, true},
		{"expanded default comparator", `set "c" "i;ascii-casemap"; if header :value "lt" :comparator "${c}" "X-Priority" "20" { fileinto "Hit"; }`, false},
		{"operator", `set "o" "gt"; if header :value "${o}" :comparator "i;ascii-numeric" "X-Priority" "2" { fileinto "Hit"; }`, true},
		{"operator in ASCII case", `set "o" "LT"; if header :value "${o}" :comparator "i;ascii-numeric" "X-Priority" "2" { fileinto "Hit"; }`, false},
		{"operator built from parts", `set "o" "e"; if header :value "${o}q" "X-Priority" "3" { fileinto "Hit"; }`, true},
		{"operator from a match variable", `if string :matches "NE" "*" { } if string :value "${0}" :comparator "i;ascii-numeric" "5" "4" { fileinto "Hit"; }`, true},
		{"header :count", `set "o" "eq"; if header :count "${o}" :comparator "i;ascii-numeric" "X-Priority" "1" { fileinto "Hit"; }`, true},
		{"address :count", `set "o" "eq"; if address :count "${o}" :comparator "i;ascii-numeric" "from" "1" { fileinto "Hit"; }`, true},
		{"envelope :count", `set "o" "ge"; if envelope :count "${o}" :comparator "i;ascii-numeric" "to" "1" { fileinto "Hit"; }`, true},
		{"body :value", `set "o" "ge"; set "c" "i;octet"; if body :value "${o}" :comparator "${c}" :text "H" { fileinto "Hit"; }`, true},
		{"string :value", `set "o" "ge"; if string :value "${o}" :comparator "i;ascii-numeric" "5" "3" { fileinto "Hit"; }`, true},
		{"string :count", `set "o" "eq"; if string :count "${o}" :comparator "i;ascii-numeric" ["a", ""] "1" { fileinto "Hit"; }`, true},
		// Only a test that runs checks its arguments.
		{"bad value in a test that does not run", `if false { if header :value "${o}" :comparator "${c}" "X-Priority" "3" { keep; } } fileinto "Hit";`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, folder := run(t, req+tc.script)
			if out.Error != nil {
				t.Fatalf("Evaluate: %v", out.Error)
			}
			if got := folder == "Hit"; got != tc.hit {
				t.Errorf("hit = %v, want %v", got, tc.hit)
			}
		})
	}

	// A bad expanded value stops the script with an error and keeps the
	// message, discarding nothing that has not already run.
	for _, tc := range []struct{ name, script, want string }{
		{"unknown comparator", `set "c" "i;bogus"; if header :comparator "${c}" "X-Priority" "3" { fileinto "Hit"; }`, `comparator "i;bogus"`},
		{"empty comparator", `if header :comparator "${unset}" "X-Priority" "3" { fileinto "Hit"; }`, `comparator ""`},
		{"comparator without a match type", `set "c" "i;ascii-numeric"; if header :contains :comparator "${c}" "X-Priority" "3" { fileinto "Hit"; }`, "does not support the :contains match type"},
		{"unknown operator", `set "o" "gte"; if header :value "${o}" "X-Priority" "3" { fileinto "Hit"; }`, `unknown relational operator "gte" after :value`},
		{"empty operator", `if string :count "${unset}" "a" "1" { fileinto "Hit"; }`, `unknown relational operator "" after :count`},
		{"operator with spaces", `set "o" " eq"; if header :value "${o}" "X-Priority" "3" { fileinto "Hit"; }`, `unknown relational operator " eq"`},
		{"in a test that runs late", `set "o" "nope"; if anyof (false, body :value "${o}" "x") { fileinto "Hit"; }`, `unknown relational operator "nope"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, folder := run(t, req+tc.script)
			if out.Error == nil || !strings.Contains(out.Error.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", out.Error, tc.want)
			}
			if !out.ImplicitKeep || out.Disposition != sieve.Continue || folder != "" {
				t.Errorf("outcome = %+v, folder %q; want the implicit keep only", out, folder)
			}
		})
	}
	// An undeclared comparator is refused at run time as Parse refuses a
	// constant one.
	out, _ := run(t, `require ["fileinto", "variables"]; set "c" "i;ascii-numeric"; if header :comparator "${c}" "X-Priority" "3" { fileinto "Hit"; }`)
	if out.Error == nil || !strings.Contains(out.Error.Error(), "neither built in nor declared") {
		t.Errorf("undeclared expanded comparator: %+v", out)
	}

	// Constant names keep the parse-time check, and so does a string without
	// variables or with a reference RFC 5229 makes an error.
	for _, s := range []string{
		`require ["variables"]; if header :comparator "i;bogus" "Subject" "x" { keep; }`,
		`require ["variables"]; if header :comparator "i;ascii-numeric" "Subject" "x" { keep; }`,
		`require ["variables", "relational"]; if header :value "gte" "Subject" "x" { keep; }`,
		`require ["variables", "relational"]; if header :value "${" "Subject" "x" { keep; }`,
		`require ["variables", "relational"]; if header :value "${env.o}" "Subject" "x" { keep; }`,
		`require ["variables"]; if header :comparator "${10}" "Subject" "x" { keep; }`,
		`require "relational"; if header :value "${o}" "Subject" "x" { keep; }`,
		`if header :comparator "${c}" "Subject" "x" { keep; }`,
	} {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestVariables_FlagVariables checks that with "variables" setflag, addflag
// and removeflag take a variable name before the list of flags and then work
// on that variable instead of the internal one (RFC 5232 §3), and that flag
// lists are expanded.
//
// checklist: FLAGS-22
func TestVariables_FlagVariables(t *testing.T) {
	script := `require ["imap4flags", "variables", "fileinto"];
setflag "fv" "\\Seen";
addflag "FV" ["\\Flagged", "$Work \\seen"];
removeflag "fv" "\\SEEN";
addflag "fv" "\\Recent bad(flag";
set "extra" "$Later";
addflag "${extra} \\Answered";
fileinto "${fv}";`
	r := runSieve(t, script, sieveEmail())
	if got := folderOf(r); got != `\Flagged $Work` {
		t.Errorf("flag variable = %q", got)
	}
	if got := r.meta["imap_flags"]; got != `$Later \Answered` {
		t.Errorf("internal flags = %q, want only the action without a variable name", got)
	}
	for _, s := range []string{
		`require ["imap4flags", "variables"]; setflag "1" "\\Seen";`,
		`require ["imap4flags", "variables"]; setflag "a.b" "\\Seen";`,
		`require ["imap4flags", "variables"]; setflag ["fv"] "\\Seen";`,
		`require ["imap4flags", "variables"]; setflag "fv" "\\Seen" "x";`,
		`require ["imap4flags", "variables"]; setflag "${10}" "\\Seen";`,
	} {
		if err := sieve.Validate(crlf(s)); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}
