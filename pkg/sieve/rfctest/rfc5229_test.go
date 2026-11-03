package rfctest

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// RFC 5229, the variables extension: set, string, ${name} interpolation and
// match variables, through the public API.

// varsRun parses and evaluates a script, failing the test on a parse error.
func varsRun(t *testing.T, script string, msg *sieve.Message) (*recExec, sieve.Outcome) {
	t.Helper()
	s, err := sieve.Parse(crlf(script))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, script)
	}
	ex := newRecExec()
	return ex, s.Evaluate(msg, ex)
}

// expandIn runs setup and then files into "=" + expr, and returns what expr
// expanded to. expr is script source: write it as it appears between quotes.
func expandIn(t *testing.T, setup, expr string) string {
	t.Helper()
	return expandInMsg(t, setup, expr, sieveEmail())
}

func expandInMsg(t *testing.T, setup, expr string, msg *sieve.Message) string {
	t.Helper()
	ex, out := varsRun(t, `require ["fileinto", "variables"]; `+setup+` fileinto "=`+expr+`";`, msg)
	if out.Error != nil {
		t.Fatalf("runtime error: %v", out.Error)
	}
	folder, ok := strings.CutPrefix(ex.meta["deliver_to_folder"], "=")
	if !ok {
		t.Fatalf("no fileinto ran for %q", expr)
	}
	return folder
}

// listMessage carries the headers of the RFC 5229 §3.2 examples.
func listMessage() *sieve.Message {
	msg := sieveEmail()
	msg.Headers.Subject = "[acme-users] [fwd] version 1.0 is out"
	msg.Headers.To = []sieve.Address{{Address: "coyote@ACME.Example.COM"}}
	msg.Headers.Raw = map[string][]string{"List-ID": {"Acme users <acme-users@lists.example.org>"}}
	return msg
}

// TestVariables_RequiresCapability checks that set, string and interpolation
// exist only once "variables" is declared (RFC 5229 §1, §2).
//
// checklist: VARIABLES-01
func TestVariables_RequiresCapability(t *testing.T) {
	for _, s := range []string{
		`set "a" "b";`,
		`if string "a" "a" { keep; }`,
		`require "fileinto"; set "a" "b";`,
	} {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%s: accepted without require \"variables\"", s)
		}
	}
	for _, s := range []string{
		`require "variables"; set "a" "b";`,
		`require "variables"; if string "a" "a" { keep; }`,
		`require ["variables"]; set :lower "a" "B";`,
	} {
		if err := sieve.Validate(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	// Without the extension a reference is ordinary text.
	ex, _ := varsRun(t, `require "fileinto"; fileinto "${x}";`, sieveEmail())
	if got := ex.meta["deliver_to_folder"]; got != "${x}" {
		t.Errorf("without variables: folder %q, want the literal ${x}", got)
	}
	// Even a reference that would be out of range is not checked.
	if err := sieve.Validate(`require "fileinto"; fileinto "${10}";`); err != nil {
		t.Errorf("without variables ${10} is text: %v", err)
	}
}

// TestVariables_Interpolation checks substitution, case-insensitive names,
// unknown variables expanding to the empty string, the RFC's examples of text
// that is not a reference, and the single pass (RFC 5229 §3).
//
// checklist: VARIABLES-02 VARIABLES-03
func TestVariables_Interpolation(t *testing.T) {
	const company = `set "company" "ACME";`
	for _, tc := range []struct{ expr, want string }{
		// §3's examples.
		{`&%${}!`, `&%${}!`},
		{`${doh!}`, `${doh!}`},
		{`${full}`, ``},
		{`${company}`, `ACME`},
		{`${BAD${Company}`, `${BADACME`},
		{`${President, ${Company} Inc.}`, `${President, ACME Inc.}`},
		// Names are case-insensitive.
		{`${COMPANY}/${cOmPaNy}`, `ACME/ACME`},
		// Malformed references stay as they are.
		{`${company`, `${company`},
		{`$company`, `$company`},
		{`${com pany}`, `${com pany}`},
		{`${1company}`, `${1company}`},
		{`${a..b}`, `${a..b}`},
		{`${1.a}`, `${1.a}`},
		{`$${company}}`, `$ACME}`},
		{`${_}x`, `x`},
	} {
		if got := expandIn(t, company, tc.expr); got != tc.want {
			t.Errorf("%q expanded to %q, want %q", tc.expr, got, tc.want)
		}
	}
	// One pass only: a value holding a reference is not expanded again.
	setup := company + ` set "dollar" "$"; set "a" "${dollar}{company}";`
	if got := expandIn(t, setup, `${a}`); got != "${company}" {
		t.Errorf("value expanded twice: got %q, want ${company}", got)
	}
}

// TestVariables_CurrentValues checks that a string uses the values current when
// control reaches its statement (RFC 5229 §3).
//
// checklist: VARIABLES-04
func TestVariables_CurrentValues(t *testing.T) {
	ex, _ := varsRun(t, `require ["fileinto", "variables"];
set "a" "1"; fileinto "f${a}";
set "a" "${a}2"; fileinto "f${a}";
if string "${a}" "12" { fileinto "now12"; }
set "a" "3"; fileinto "f${a}";`, sieveEmail())
	want := []string{"fileinto:f1", "fileinto:f12", "fileinto:now12", "fileinto:f3"}
	if !reflect.DeepEqual(ex.applied, want) {
		t.Errorf("actions %q, want %q", ex.applied, want)
	}
}

// TestVariables_NamespaceRefused checks that a namespaced reference is an
// error, because no supported extension provides a namespace (RFC 5229 §3).
//
// checklist: VARIABLES-05
func TestVariables_NamespaceRefused(t *testing.T) {
	for _, s := range []string{
		`require "variables"; set "a" "${envelope.from}";`,
		`require ["variables", "envelope"]; set "a" "${envelope.from}";`,
		`require ["variables", "fileinto"]; if false { fileinto "${a.b.c}"; }`,
		`require "variables"; if header :is "${x.y}" "z" { keep; }`,
		`require "variables"; set "a" "x${ns.1}";`,
	} {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%s: accepted a namespace reference", s)
		}
	}
}

// TestVariables_QuotingAndEncoded checks that backslash quoting and then
// encoded-character decoding happen before substitution (RFC 5229 §3.1), with
// the RFC's examples.
//
// checklist: VARIABLES-06
func TestVariables_QuotingAndEncoded(t *testing.T) {
	const foo = `set "foo" "FOO";`
	for _, tc := range []struct{ expr, want string }{
		{`${fo\o}`, `FOO`},
		{`${fo\\o}`, `${fo\o}`},
		{`\${foo}`, `FOO`},
		{`\\${foo}`, `\FOO`},
	} {
		if got := expandIn(t, foo, tc.expr); got != tc.want {
			t.Errorf("%q expanded to %q, want %q", tc.expr, got, tc.want)
		}
	}
	if got := expandIn(t, `set "dollar" "$"; set "text" "regarding ${dollar}{beep}";`, `${text}`); got != "regarding ${beep}" {
		t.Errorf("text = %q, want regarding ${beep}", got)
	}
	msg := sieveEmail()
	msg.Headers.Subject = "dear Ethelbert"
	ex, _ := varsRun(t, `require ["encoded-character", "variables", "fileinto"];
set "name" "Ethelbert";
if header :contains "Subject" "dear${hex:20 24 7b 4e}ame}" { fileinto "hit"; }`, msg)
	if folderOf(&sieveResult{meta: ex.meta}) != "hit" {
		t.Error("encoded-character was not decoded before substitution")
	}
}

// TestVariables_MatchVariables checks match variables from :matches: one per
// wildcard, non-greedy, ${0} the whole value, captured from the original value
// whatever the comparator folds, empty before any match and out of range,
// leading zeroes ignored, and kept across failed matches (RFC 5229 §3.2).
//
// checklist: VARIABLES-07 VARIABLES-19
func TestVariables_MatchVariables(t *testing.T) {
	msg := listMessage()
	for _, tc := range []struct{ test, expr, want string }{
		// §3.2's examples.
		{`header :matches "List-ID" "*<*@*"`, `INBOX.lists.${2}`, `INBOX.lists.acme-users`},
		{`header :matches "Subject" "[*] *"`, `${1}|${2}`, `acme-users|[fwd] version 1.0 is out`},
		{`address :matches ["To", "Cc"] ["coyote@**.com", "wile@**.com"]`, `${0}|${1}|${2}`, `coyote@ACME.Example.COM||ACME.Example`},
		// Non-greedy, left to right; ? captures one octet.
		{`string :matches "aXbYbZ" "a*b*"`, `${1}|${2}`, `X|YbZ`},
		{`string :matches "abc" "?*?"`, `${1}|${2}|${3}`, `a|b|c`},
		{`string :matches "abc" "*"`, `${0}|${1}`, `abc|abc`},
		// Escaped wildcards are literals and capture nothing.
		{`string :matches "a*b" "a\\**"`, `${1}|${2}`, `b|`},
		// Folding compares, the original value is captured.
		{`string :matches "HeLLo World" "hello *"`, `${1}`, `World`},
		{`envelope :matches "from" "*@*"`, `${1}`, `sender`},
		// Leading zeroes are ignored; an index past the list is empty.
		{`string :matches "ab" "?*"`, `${01}|${002}|${3}|${9}`, `a|b||`},
		// Empty before any match.
		{`true`, `${0}${1}`, ``},
		// The match inside not succeeded, so it sets the variables.
		{`not string :matches "xy" "x*"`, `${1}`, `y`},
	} {
		script := `require ["fileinto", "variables", "envelope"]; ` +
			`if ` + tc.test + ` { fileinto "=` + tc.expr + `"; } ` +
			`if not ` + tc.test + ` { fileinto "=` + tc.expr + `"; }`
		ex, _ := varsRun(t, script, msg)
		if got := strings.TrimPrefix(ex.meta["deliver_to_folder"], "="); got != tc.want {
			t.Errorf("%s: %q = %q, want %q", tc.test, tc.expr, got, tc.want)
		}
	}
	// A failed :matches keeps the variables of the last successful one.
	got := expandInMsg(t, `if string :matches "keep-me" "*-*" {} if string :matches "zz" "a*" {}`, `${1}${2}`, msg)
	if got != "keepme" {
		t.Errorf("after a failed match: %q, want keepme", got)
	}
	// Each successful match replaces the whole list.
	got = expandInMsg(t, `if string :matches "a-b-c" "*-*-*" {} if string :matches "x" "*" {}`, `${1}${2}${3}`, msg)
	if got != "x" {
		t.Errorf("after a second match: %q, want x", got)
	}
	// :is, :contains and :regex do not touch match variables.
	got = expandInMsg(t, `require "regex"; if string :matches "m" "*" {} if string :contains "abc" "b" {} if string :is "q" "q" {} if string :regex "xyz" "(y)" {}`, `${1}`, msg)
	if got != "m" {
		t.Errorf("a non-:matches test changed the match variables: %q", got)
	}
	// Nor do the relational :value and :count (RFC 5231).
	got = expandInMsg(t, `require ["relational", "comparator-i;ascii-numeric"]; if string :matches "m" "*" {} if string :value "gt" "b" "a" {} if string :count "eq" :comparator "i;ascii-numeric" "q" "1" {} if header :count "ge" :comparator "i;ascii-numeric" "Subject" "1" {}`, `${0}${1}`, msg)
	if got != "mm" {
		t.Errorf("a relational test changed the match variables: %q", got)
	}
}

// TestVariables_MatchVariablesShortCircuit checks that tests are evaluated left
// to right and no further than needed, so a test that is not evaluated sets no
// match variables (RFC 5229 §3.2).
//
// checklist: VARIABLES-08
func TestVariables_MatchVariablesShortCircuit(t *testing.T) {
	msg := listMessage()
	for _, tc := range []struct{ test, want string }{
		// §3.2's last example.
		{`anyof (true, address :domain :matches "To" "*.com")`, ``},
		{`allof (false, string :matches "a" "*")`, ``},
		{`anyof (string :matches "first" "f*", string :matches "second" "s*")`, `irst`},
		{`allof (string :matches "first" "f*", string :matches "second" "s*")`, `econd`},
	} {
		script := `require ["fileinto", "variables"]; if ` + tc.test + ` { } fileinto "=${1}";`
		ex, _ := varsRun(t, script, msg)
		if got := strings.TrimPrefix(ex.meta["deliver_to_folder"], "="); got != tc.want {
			t.Errorf("%s: ${1} = %q, want %q", tc.test, got, tc.want)
		}
	}
	// §3.2: control does not reach else when a match succeeded.
	ex, _ := varsRun(t, `require ["fileinto", "variables"];
if address :matches ["To", "Cc"] ["coyote@**.com", "wile@**.com"] {
    fileinto "INBOX.business.${2}"; stop;
} else { fileinto "else"; }`, msg)
	if !reflect.DeepEqual(ex.applied, []string{"fileinto:INBOX.business.ACME.Example"}) {
		t.Errorf("actions %q", ex.applied)
	}
}

// TestVariables_SetName checks that set's name is a constant identifier: match
// variables, namespaces and other forms are syntax errors, and names are
// case-insensitive (RFC 5229 §4).
//
// checklist: VARIABLES-09
func TestVariables_SetName(t *testing.T) {
	for _, name := range []string{`1`, `0`, `a.b`, `${a}`, ``, `a-b`, `a b`, `ünï`, `x${a}`} {
		s := fmt.Sprintf(`require "variables"; set %q "v";`, name)
		if err := sieve.Validate(s); err == nil {
			t.Errorf("set name %q accepted", name)
		}
	}
	for _, s := range []string{
		`require "variables"; set ["a"] "v";`,
		`require "variables"; set "a" ["v"];`,
		`require "variables"; set "a";`,
		`require "variables"; set "a" "v" "w";`,
		`require "variables"; set 1 "v";`,
	} {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%s: accepted", s)
		}
	}
	if got := expandIn(t, `set "_Name1" "x"; set "NAME1" "y";`, `${_name1}|${name1}`); got != "x|y" {
		t.Errorf("got %q, want x|y", got)
	}
}

// TestVariables_SetScope checks that set stores its value for the running
// evaluation only, leaves the implicit keep alone and combines with other
// actions (RFC 5229 §4).
//
// checklist: VARIABLES-10
func TestVariables_SetScope(t *testing.T) {
	s, err := sieve.Parse(crlf(`require ["fileinto", "variables"];
if header :is "Subject" "first" { set "seen" "yes"; }
fileinto "seen=${seen}";`))
	if err != nil {
		t.Fatal(err)
	}
	first := sieveEmail()
	first.Headers.Subject = "first"
	for i, tc := range []struct {
		msg  *sieve.Message
		want string
	}{{first, "seen=yes"}, {sieveEmail(), "seen="}, {first, "seen=yes"}} {
		ex := newRecExec()
		s.Evaluate(tc.msg, ex)
		if got := ex.meta["deliver_to_folder"]; got != tc.want {
			t.Errorf("evaluation %d: %q, want %q", i, got, tc.want)
		}
	}
	ex, out := varsRun(t, `require "variables"; set "a" "b"; set :upper "c" "${a}";`, sieveEmail())
	if !out.ImplicitKeep || out.Disposition != sieve.Continue || len(ex.applied) != 0 {
		t.Errorf("set changed delivery: %+v, actions %q", out, ex.applied)
	}
	_, out = varsRun(t, `require ["variables", "reject"]; set "why" "no thanks"; reject "${why}";`, sieveEmail())
	if out.Disposition != sieve.Reject || out.RejectReason != "no thanks" {
		t.Errorf("reject after set: %+v", out)
	}
}

// vacationRec records vacation callbacks as well as the other actions.
type vacationRec struct {
	*recExec
	vacations []sieve.Vacation
}

func (e *vacationRec) Vacation(v sieve.Vacation) { e.vacations = append(e.vacations, v) }

// TestVariables_Actions checks that every action and test argument is expanded
// when it runs, that values only known then are checked then, and that a
// comparator name is still checked at parse time, so a reference in it is
// refused (rest-mail/go-sieve#100).
//
// checklist: VARIABLES-11
func TestVariables_Actions(t *testing.T) {
	const req = `require ["variables", "fileinto", "copy", "imap4flags", "envelope", "body", "notify", "vacation", "ereject"]; `
	msg := sieveEmail()
	msg.Headers.Raw = map[string][]string{"X-Tag": {"blue"}}
	ex := &vacationRec{recExec: newRecExec()}
	s, err := sieve.Parse(crlf(req + `
set "h" "x-tag"; set "c" "blue"; set "who" "sender"; set "f" "Seen";
if allof (header :is "${h}" "${c}", exists ["${h}", "Subject"], envelope :localpart :is "${w}from" "${who}",
          address :localpart :is "From" "${who}", body :content "${t}text" :contains "${k}test") {
    fileinto :copy "Folder-${c}";
    redirect :copy "${who}@example.net";
    setflag "\\${f} $${c}";
    notify :method "mailto:${who}@example.org" :message "hi ${c}";
    vacation :subject "Re: ${c}" :from "${who}@example.com" :addresses "${who}-alias@example.com" "Away (${c})";
}`))
	if err != nil {
		t.Fatal(err)
	}
	out := s.Evaluate(msg, ex)
	if out.Error != nil {
		t.Fatal(out.Error)
	}
	want := []string{"fileinto:Folder-blue", "redirect:sender@example.net", `setflag:\Seen $blue`, "notify:mailto:sender@example.org"}
	if !reflect.DeepEqual(ex.applied, want) {
		t.Errorf("actions %q, want %q", ex.applied, want)
	}
	if ex.meta["notify_message"] != "hi blue" {
		t.Errorf("notify message %q", ex.meta["notify_message"])
	}
	if len(ex.vacations) != 1 {
		t.Fatalf("vacations = %d, want 1", len(ex.vacations))
	}
	if v := ex.vacations[0]; v.Subject != "Re: blue" || v.Body != "Away (blue)" || v.From != "sender@example.com" {
		t.Errorf("vacation %+v", v)
	}
	_, out = varsRun(t, req+`set "w" "no"; ereject "${w}";`, sieveEmail())
	if out.RejectReason != "no" || !out.ExtendedReject {
		t.Errorf("ereject: %+v", out)
	}

	// Values built from variables are checked when they are expanded; an
	// invalid one is a runtime error with the fail-safe keep.
	for _, script := range []string{
		`set "a" "not an address"; redirect "${a}";`,
		`set "a" "a@b.example, c@d.example"; redirect "${a}";`,
		`set "a" "group: a@b.example;"; redirect "${a}";`,
		`set "a" "bogus"; if envelope :is "${a}" "x" { keep; }`,
		`set "a" "<<<"; vacation :from "${a}" "away";`,
	} {
		ex, out := varsRun(t, req+script, sieveEmail())
		if out.Error == nil || !out.ImplicitKeep || len(ex.applied) != 0 {
			t.Errorf("%s: outcome %+v, actions %q, want a runtime error and the implicit keep", script, out, ex.applied)
		}
	}
	// The same constant values are refused at parse time, as before.
	for _, script := range []string{
		`redirect "not an address";`,
		`if envelope :is "bogus" "x" { keep; }`,
		`vacation :from "<<<" "away";`,
	} {
		if err := sieve.Validate(req + script); err == nil {
			t.Errorf("%s: accepted", script)
		}
	}
	// A valid expanded redirect address passes.
	if _, out := varsRun(t, req+`set "a" "Name <a@b.example>"; redirect "${a}";`, sieveEmail()); out.Error != nil {
		t.Errorf("valid expanded redirect: %v", out.Error)
	}
	// The relational operator is a string like any other: built from a
	// variable it is expanded and checked when its test runs (RFC 5229 §3).
	const relReq = `require ["variables", "fileinto", "relational", "comparator-i;ascii-numeric"]; `
	for _, tc := range []struct{ script, want string }{
		{`set "op" "GT"; if string :value "${op}" "b" "a" { fileinto "gt"; }`, "fileinto:gt"},
		{`set "op" "l"; if string :value "${op}e" "b" "b" { fileinto "le"; }`, "fileinto:le"},
		{`set "op" "eq"; if header :count "${op}" :comparator "i;ascii-numeric" "Subject" "1" { fileinto "one"; }`, "fileinto:one"},
	} {
		ex, out := varsRun(t, relReq+tc.script, sieveEmail())
		if out.Error != nil || !reflect.DeepEqual(ex.applied, []string{tc.want}) {
			t.Errorf("%s: actions %q, error %v, want %s", tc.script, ex.applied, out.Error, tc.want)
		}
	}
	for _, script := range []string{
		`set "op" "bogus"; if string :value "${op}" "b" "a" { fileinto "x"; }`,
		`if string :count "${unset}" "b" "1" { fileinto "x"; }`,
		`set "op" "gt"; if header :value "${op} " "Subject" "a" { fileinto "x"; }`,
	} {
		ex, out := varsRun(t, relReq+script, sieveEmail())
		if out.Error == nil || !out.ImplicitKeep || len(ex.applied) != 0 {
			t.Errorf("%s: outcome %+v, actions %q, want a runtime error and the implicit keep", script, out, ex.applied)
		}
	}
	if err := sieve.Validate(relReq + `if string :value "bogus" "b" "a" { keep; }`); err == nil {
		t.Error("a constant unknown operator was accepted")
	}

	// Gap rest-mail/go-sieve#100: a comparator name is not expanded.
	if err := sieve.Validate(req + `set "c" "i;octet"; if header :comparator "${c}" :is "Subject" "x" { keep; }`); err == nil {
		t.Error("a comparator name built from a variable was accepted; update #100 and VARIABLES-11")
	}
}

// TestVariables_Modifiers checks set's modifiers: case-insensitive names,
// unknown modifiers and two of one precedence refused, and application in
// precedence order whatever the order written (RFC 5229 §4.1).
//
// checklist: VARIABLES-12
func TestVariables_Modifiers(t *testing.T) {
	for _, s := range []string{
		`set :bogus "a" "b";`,
		`set :lower :upper "a" "b";`,
		`set :lowerfirst :upperfirst "a" "b";`,
		`set :lower :lower "a" "b";`,
		`set :length :length "a" "b";`,
		`set :quotewildcard :QuoteWildcard "a" "b";`,
		`set :comparator "i;octet" "a" "b";`,
		`set "a" :lower "b";`,
	} {
		if err := sieve.Validate(`require "variables"; ` + s); err == nil {
			t.Errorf("%s: accepted", s)
		}
	}
	const a = `set "a" "juMBlEd lETteRS";`
	for _, tc := range []struct{ set, want string }{
		{`set :LOWER "b" "${a}";`, `jumbled letters`},
		{`set :upperfirst :lower "b" "${a}";`, `Jumbled letters`},
		{`set :lower :upperfirst "b" "${a}";`, `Jumbled letters`},
		{`set :upper :lowerfirst "b" "${a}";`, `jUMBLED LETTERS`},
		{`set :length :quotewildcard "b" "a*?";`, `5`},
		{`set :quotewildcard :upper "b" "a*";`, `A\*`},
		{`set :length :upper :upperfirst :quotewildcard "b" "x\\";`, `3`},
	} {
		if got := expandIn(t, a+tc.set, `${b}`); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.set, got, tc.want)
		}
	}
}

// TestVariables_ModifierValues checks what each modifier produces, with the
// RFC's examples: :length counts characters, :quotewildcard quotes "*", "?" and
// "\", and the case modifiers change only US-ASCII letters (RFC 5229 §4.1.1–
// §4.1.3).
//
// checklist: VARIABLES-13
func TestVariables_ModifierValues(t *testing.T) {
	const a = `set "a" "juMBlEd lETteRS";`
	for _, tc := range []struct{ set, want string }{
		{`set "b" "${a}";`, `juMBlEd lETteRS`},
		{`set :length "b" "${a}";`, `15`},
		{`set :lower "b" "${a}";`, `jumbled letters`},
		{`set :upperfirst "b" "${a}";`, `JuMBlEd lETteRS`},
		{`set :quotewildcard "b" "Rock*";`, `Rock\*`},
		{`set :quotewildcard "b" "a?b\\c*";`, `a\?b\\c\*`},
		{`set :length "b" "";`, `0`},
		{`set :length "b" "héllo wörld";`, `11`},
		{`set :length "b" "日本語";`, `3`},
		{`set :upper "b" "éa";`, `éA`},
		{`set :lower "b" "ÉA";`, `Éa`},
		{`set :upperfirst "b" "élan";`, `élan`},
		{`set :lowerfirst "b" "ABC";`, `aBC`},
		{`set :lowerfirst "b" "1BC";`, `1BC`},
		{`set :upperfirst "b" "";`, ``},
	} {
		if got := expandIn(t, a+tc.set, `${b}`); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.set, got, tc.want)
		}
	}
	// A quoted value matches only its literal text under :matches.
	ex, _ := varsRun(t, `require ["fileinto", "variables"];
set :quotewildcard "q" "Rock*?";
if string :matches "Rock*?" "${q}" { fileinto "literal"; }
if string :matches "Rockxy" "${q}" { fileinto "wildcard"; }`, sieveEmail())
	if !reflect.DeepEqual(ex.applied, []string{"fileinto:literal"}) {
		t.Errorf("actions %q, want only the literal match", ex.applied)
	}
}

// TestVariables_StringTest checks the string test: the default :is, any source
// against any key, match types and comparators, and no whitespace stripping
// (RFC 5229 §5).
//
// checklist: VARIABLES-14
func TestVariables_StringTest(t *testing.T) {
	for _, tc := range []struct {
		test string
		want bool
	}{
		{`string "a" "a"`, true},
		{`string "a" "A"`, true},
		{`string :comparator "i;octet" "a" "A"`, false},
		{`string " a " "a"`, false},
		{`string :is " a " " a "`, true},
		{`string ["x", "y"] ["z", "y"]`, true},
		{`string ["x", "y"] ["z", "w"]`, false},
		{`string :contains "a pending b" "pending"`, true},
		{`string "" ""`, true},
		{`string "${unset}" ""`, true},
		{`string :is :comparator "i;ascii-numeric" "007" "7"`, true},
		{`string :matches " ${state} " "* pending *"`, true},
		{`string :regex "abc" "^a.c$"`, true},
	} {
		script := `require ["fileinto", "variables", "regex", "comparator-i;ascii-numeric"];
set "state" "${state} pending";
if ` + tc.test + ` { fileinto "yes"; }`
		ex, _ := varsRun(t, script, sieveEmail())
		if got := ex.meta["deliver_to_folder"] == "yes"; got != tc.want {
			t.Errorf("%s = %v, want %v", tc.test, got, tc.want)
		}
	}
	for _, s := range []string{
		`if string "a" { keep; }`,
		`if string { keep; }`,
		`if string :localpart "a" "a" { keep; }`,
		`if string :comparator "i;ascii-numeric" :contains "1" "1" { keep; }`,
	} {
		if err := sieve.Validate(`require ["variables", "comparator-i;ascii-numeric"]; ` + s); err == nil {
			t.Errorf("%s: accepted", s)
		}
	}
}

// TestVariables_Limits checks the minimum limits: 128 distinct variables,
// names of 32 characters and values of 4000 characters (RFC 5229 §6). This
// implementation allows names up to 400 characters.
//
// checklist: VARIABLES-15
func TestVariables_Limits(t *testing.T) {
	var b strings.Builder
	b.WriteString(`require ["variables", "imap4flags"];`)
	for i := 0; i < 128; i++ {
		// Names are case-insensitive: v1 and V1 are one variable.
		fmt.Fprintf(&b, ` set "v%d" "x"; set "V%d" "y";`, i, i)
	}
	if err := sieve.Validate(b.String()); err != nil {
		t.Errorf("128 distinct variables: %v", err)
	}
	if err := sieve.Validate(b.String() + ` set "one_more" "x";`); err == nil {
		t.Error("129 distinct variables accepted")
	}
	if err := sieve.Validate(b.String() + ` addflag "one_more" "x";`); err == nil {
		t.Error("a flag variable past the limit was accepted")
	}
	for _, n := range []int{32, 400} {
		name := strings.Repeat("n", n)
		if got := expandIn(t, `set "`+name+`" "ok";`, `${`+name+`}`); got != "ok" {
			t.Errorf("name of %d characters: %q", n, got)
		}
	}
	if err := sieve.Validate(`require "variables"; set "` + strings.Repeat("n", 401) + `" "x";`); err == nil {
		t.Error("a 401-character name was accepted")
	}
	// 4000 characters, each two octets in UTF-8, are kept whole.
	value := strings.Repeat("é", 4000)
	if got := expandIn(t, `set "v" "`+value+`"; set :length "n" "${v}";`, `${n}`); got != "4000" {
		t.Errorf(":length of a 4000-character value = %s", got)
	}
	if got := expandIn(t, `set "v" "`+value+`";`, `${v}`); got != value {
		t.Errorf("a 4000-character value was changed (%d octets back)", len(got))
	}
}

// TestVariables_ValueTruncation checks that a constant value over the limit is
// a parse error, and that a longer value found at run time is truncated on a
// character boundary without an error (RFC 5229 §6).
//
// checklist: VARIABLES-16
func TestVariables_ValueTruncation(t *testing.T) {
	long := strings.Repeat("a", 4001)
	if err := sieve.Validate(`require "variables"; set "v" "` + long + `";`); err == nil {
		t.Error("a constant 4001-character value was accepted")
	}
	if err := sieve.Validate(`require "variables"; set :quotewildcard "v" "` + strings.Repeat("*", 2001) + `";`); err == nil {
		t.Error("a constant value made too long by :quotewildcard was accepted")
	}
	if err := sieve.Validate(`require "variables"; set :length "v" "` + long + `";`); err != nil {
		t.Errorf("a long constant whose stored value is short: %v", err)
	}
	half := strings.Repeat("é", 2500)
	for _, tc := range []struct{ setup, expr, want string }{
		{`set "x" "` + half + `"; set "y" "${x}${x}"; set :length "n" "${y}";`, `${n}`, "4000"},
		{`set "x" "` + half + `"; set "y" "${x}${x}";`, `${y}`, strings.Repeat("é", 4000)},
		// A cut through an escape drops the dangling backslash.
		{`set "s" "` + strings.Repeat("a", 3999) + `*"; set :quotewildcard "q" "${s}"; set :length "n" "${q}";`, `${n}`, "3999"},
		// A cut after a whole escape keeps it.
		{`set "s" "` + strings.Repeat("a", 3997) + `*bc"; set :quotewildcard "q" "${s}";`, `${q}`, strings.Repeat("a", 3997) + `\*b`},
	} {
		got := expandIn(t, tc.setup, tc.expr)
		if got != tc.want {
			t.Errorf("%s: got %d octets, want %d (%.20q)", tc.expr, len(got), len(tc.want), got)
		}
	}
}

// TestVariables_MatchVariableLimit checks that ${0} to ${9} are supported and a
// higher index is a syntax error found at parse time, even in a branch that
// never runs (RFC 5229 §6).
//
// checklist: VARIABLES-17
func TestVariables_MatchVariableLimit(t *testing.T) {
	for _, ref := range []string{`${10}`, `${0010}`, `${99999999999999999999}`, `x${12}y`} {
		s := `require ["variables", "fileinto"]; if false { fileinto "` + ref + `"; }`
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%s accepted", ref)
		}
	}
	for _, ref := range []string{`${0}`, `${9}`, `${09}`, `${0000}`} {
		s := `require ["variables", "fileinto"]; fileinto "` + ref + `";`
		if err := sieve.Validate(s); err != nil {
			t.Errorf("%s: %v", ref, err)
		}
	}
	// A pattern with more wildcards than variables: ${9} is the ninth.
	got := expandIn(t, `if string :matches "abcdefghijkl" "????????????" {}`, `${1}${9}`)
	if got != "ai" {
		t.Errorf("${1}${9} = %q, want ai", got)
	}
}

// TestVariables_BodyNoMatchVariables checks that body's wildcards never set
// match variables, while its keys and content types are expanded (RFC 5173 §6).
//
// checklist: BODY-17
func TestVariables_BodyNoMatchVariables(t *testing.T) {
	got := expandIn(t, `require "body"; if header :matches "Subject" "* message" {}
if body :matches "Hello*" {}`, `${1}`)
	if got != "Test" {
		t.Errorf("${1} = %q, want the header's capture Test", got)
	}
	ex, _ := varsRun(t, `require ["variables", "body", "fileinto"];
set "k" "test message"; set "t" "text";
if body :content "${t}" :contains "${k}" { fileinto "hit"; }`, sieveEmail())
	if ex.meta["deliver_to_folder"] != "hit" {
		t.Error("body key or content type was not expanded")
	}
}

// TestVariables_VacationTracking checks that vacation's reply carries expanded
// arguments while the tracking arguments and :handle reach the host as
// written: RFC 5230 §4.2 forbids expansion before response tracking.
//
// checklist: VACATION-21
func TestVariables_VacationTracking(t *testing.T) {
	ex := &vacationRec{recExec: newRecExec()}
	s, err := sieve.Parse(crlf(`require ["vacation", "variables"];
if header :matches "subject" "*" {
    vacation :subject "Automatic response to: ${1}" :from "${me}" :handle "h-${1}"
             "I'm away -- ${1}";
}`))
	if err != nil {
		t.Fatal(err)
	}
	if out := s.Evaluate(sieveEmail(), ex); out.Error != nil {
		t.Fatal(out.Error)
	}
	if len(ex.vacations) != 1 {
		t.Fatalf("vacations = %d", len(ex.vacations))
	}
	v := ex.vacations[0]
	want := sieve.VacationTracking{Subject: "Automatic response to: ${1}", From: "${me}", Body: "I'm away -- ${1}"}
	if v.Subject != "Automatic response to: Test message" || v.Body != "I'm away -- Test message" || v.From != "" {
		t.Errorf("reply arguments %+v", v)
	}
	if v.Tracking != want || v.Handle != "h-${1}" || !v.HandleSet {
		t.Errorf("tracking %+v handle %q, want %+v and h-${1}", v.Tracking, v.Handle, want)
	}
}

// TestFlags_VariableName checks that with "variables" a variable name before
// the list of flags makes setflag/addflag/removeflag change that variable
// instead of the internal one, and that the variable reads back as a
// space-separated list (RFC 5232 §3, with §3.1 and §3.2's examples).
//
// checklist: FLAGS-22
func TestFlags_VariableName(t *testing.T) {
	const req = `require ["imap4flags", "variables", "fileinto"]; `
	ex, _ := varsRun(t, req+`setflag "flagvar" "\\Flagged"; fileinto :flags "${flagvar}" "INBOX.From Boss"; fileinto "=${flagvar}";`, sieveEmail())
	if !reflect.DeepEqual(ex.applied, []string{"fileinto:INBOX.From Boss", `fileinto:=\Flagged`}) {
		t.Errorf("actions %q: a named variable must not reach Executor.Flag", ex.applied)
	}
	for _, adds := range []string{
		`addflag "flagvar" "\\Deleted"; addflag "flagvar" "\\Answered";`,
		`addflag "flagvar" ["\\Deleted", "\\Answered"];`,
		`addflag "flagvar" "\\Deleted \\Answered";`,
		`addflag "flagvar" "\\Answered \\Deleted";`,
	} {
		got := strings.Fields(expandIn(t, `require "imap4flags"; `+adds, `${flagvar}`))
		if len(got) != 2 || !contains(got, `\Deleted`) || !contains(got, `\Answered`) {
			t.Errorf("%s: flagvar = %q", adds, got)
		}
	}
	for _, tc := range []struct{ ops, want string }{
		{`set "v" "a b  c"; removeflag "v" ["B", "x"];`, `a c`},
		{`setflag "v" "a"; setflag "v" "b \\Recent";`, `b`},
		{`set "v" "a"; addflag "V" "A b";`, `a b`},
		{`set "f" "$Work"; setflag "v" "${f} \\seen";`, `$Work \Seen`},
	} {
		if got := expandIn(t, `require "imap4flags"; `+tc.ops, `${v}`); got != tc.want {
			t.Errorf("%s: v = %q, want %q", tc.ops, got, tc.want)
		}
	}
	// The internal variable still reaches the host, expanded.
	ex, _ = varsRun(t, req+`set "f" "$Work"; setflag "${f}"; addflag ["${f}2"];`, sieveEmail())
	if !reflect.DeepEqual(ex.applied, []string{"setflag:$Work", "addflag:$Work2"}) {
		t.Errorf("actions %q", ex.applied)
	}
	for _, s := range []string{
		req + `setflag "1" "a";`,
		req + `setflag "a.b" "a";`,
		req + `setflag ["v"] "a";`,
		req + `setflag "${10}" "a";`,
	} {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%s: accepted", s)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TestVariables_StringCount checks string :count with relational: an empty
// source counts 0 and any other 1, summed over the list, after expansion
// (RFC 5229 §5).
//
// checklist: VARIABLES-18
func TestVariables_StringCount(t *testing.T) {
	for _, tc := range []struct {
		test string
		want bool
	}{
		{`string :count "eq" :comparator "i;ascii-numeric" "a" "1"`, true},
		{`string :count "eq" :comparator "i;ascii-numeric" "" "0"`, true},
		{`string :count "eq" :comparator "i;ascii-numeric" ["a", "", "b"] "2"`, true},
		{`string :count "eq" :comparator "i;ascii-numeric" ["${unset}", "${empty}", "${x}"] "1"`, true},
		{`string :count "eq" :comparator "i;ascii-numeric" [" ", "${unset}"] "1"`, true},
		{`string :count "gt" :comparator "i;ascii-numeric" ["a", "b", "c"] "2"`, true},
		{`string :count "lt" :comparator "i;ascii-numeric" ["a", "b", "c"] ["1", "3"]`, false},
		{`string :count "ne" :comparator "i;ascii-numeric" ["a", "b"] ["2", "5"]`, true},
		{`string :count "eq" "" "0"`, true},
		{`string :value "lt" :comparator "i;ascii-numeric" ["10", "x"] "9"`, false},
		{`string :value "gt" :comparator "i;ascii-numeric" ["${x}0", "1"] "9"`, true},
	} {
		script := `require ["fileinto", "variables", "relational", "comparator-i;ascii-numeric"];
set "empty" ""; set "x" "1";
if ` + tc.test + ` { fileinto "yes"; }`
		ex, out := varsRun(t, script, sieveEmail())
		if out.Error != nil {
			t.Fatalf("%s: %v", tc.test, out.Error)
		}
		if got := ex.meta["deliver_to_folder"] == "yes"; got != tc.want {
			t.Errorf("%s = %v, want %v", tc.test, got, tc.want)
		}
	}
	// :count and :value on string need relational.
	for _, s := range []string{
		`if string :count "eq" "a" "1" { keep; }`,
		`if string :value "eq" "a" "a" { keep; }`,
	} {
		if err := sieve.Validate(`require "variables"; ` + s); err == nil {
			t.Errorf("%s: accepted without relational", s)
		}
	}
}
