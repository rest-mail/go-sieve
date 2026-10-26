package rfctest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// These tests check RFC 5232 (the "imap4flags" extension) clause by clause.
// The library hands each setflag, addflag and removeflag to Executor.Flag; the
// host holds the flag set itself.

// flagOps parses and runs script and returns the imap4flags operations passed to
// the Executor, in order, as "op:flag flag".
func flagOps(t *testing.T, script string) []string {
	t.Helper()
	s, err := sieve.Parse(script)
	if err != nil {
		t.Fatalf("Parse(%q): %v", script, err)
	}
	ex := newRecExec()
	s.Evaluate(sieveEmail(), ex)
	var ops []string
	for _, a := range ex.applied {
		if strings.HasPrefix(a, "setflag:") || strings.HasPrefix(a, "addflag:") || strings.HasPrefix(a, "removeflag:") {
			ops = append(ops, a)
		}
	}
	return ops
}

// TestFlags_RequiresCapability checks that the actions and the :flags tagged
// argument are errors unless the script declares "imap4flags" (RFC 5232 §1;
// RFC 5228 §3.2).
//
// checklist: FLAGS-01
func TestFlags_RequiresCapability(t *testing.T) {
	good := []string{
		`require "imap4flags"; setflag "a";`,
		`require "imap4flags"; addflag "a";`,
		`require "imap4flags"; removeflag "a";`,
		`require "imap4flags"; keep :flags "a";`,
		`require ["imap4flags", "fileinto"]; fileinto :flags "a" "Archive";`,
	}
	for _, s := range good {
		if err := sieve.Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`setflag "a";`,
		`addflag "a";`,
		`removeflag "a";`,
		`keep :flags "a";`,
		`require "fileinto"; fileinto :flags "a" "Archive";`,
	}
	for _, s := range bad {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%q: accepted without require \"imap4flags\", want an error", s)
		}
	}
}

// TestFlags_ActionsReachHost checks that setflag, addflag and removeflag are
// each passed to the host as its own operation, in script order (RFC 5232 §3,
// §3.1–3.3).
//
// checklist: FLAGS-02
func TestFlags_ActionsReachHost(t *testing.T) {
	got := flagOps(t, `require "imap4flags";
setflag "a";
addflag "b";
removeflag "a";`)
	want := []string{"setflag:a", "addflag:b", "removeflag:a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ops = %q, want %q", got, want)
	}
}

// TestFlags_VariableNameRefused checks that without the "variables" extension
// a variable name before the list of flags is an error (RFC 5232 §1).
//
// checklist: FLAGS-03
func TestFlags_VariableNameRefused(t *testing.T) {
	for _, s := range []string{
		`require "imap4flags"; setflag "flagvar" "\\Flagged";`,
		`require "imap4flags"; addflag "flagvar" ["\\Deleted", "\\Answered"];`,
		`require "imap4flags"; removeflag "flagvar" "$MDNRequired";`,
	} {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestFlags_FlagsTagSyntax checks that :flags takes one list of flags on keep
// and fileinto, and is not a tag of any other command (RFC 5232 §5).
//
// checklist: FLAGS-04
func TestFlags_FlagsTagSyntax(t *testing.T) {
	good := []string{
		`require "imap4flags"; keep :flags ["\\Seen", "$Work"];`,
		`require ["imap4flags", "fileinto"]; fileinto :flags "\\Seen" "Archive";`,
	}
	for _, s := range good {
		if err := sieve.Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`require "imap4flags"; keep :flags;`,
		`require ["imap4flags", "fileinto"]; fileinto :flags "a" :flags "b" "Archive";`,
		`require "imap4flags"; redirect :flags "a" "a@example.com";`,
		`require "imap4flags"; discard :flags "a";`,
	}
	for _, s := range bad {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestFlags_ListNormalised checks RFC 5232 §2's rules for a list of flags:
// empty strings are ignored, a space-separated string is the same as a string
// list, runs of spaces count as one, and leading and trailing spaces are
// ignored.
//
// checklist: FLAGS-05 FLAGS-06 FLAGS-07
func TestFlags_ListNormalised(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{"empty string", `addflag "";`, "addflag:"},
		{"empty in a list", `addflag ["", "a", ""];`, "addflag:a"},
		{"space separated", `addflag "a b";`, "addflag:a b"},
		{"runs of spaces", `addflag "a   b";`, "addflag:a b"},
		{"leading and trailing", `addflag "  a b  ";`, "addflag:a b"},
		{"mixed", `addflag [" a", "b  c ", ""];`, "addflag:a b c"},
		{"removeflag empty", `removeflag ["", "a"];`, "removeflag:a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flagOps(t, `require "imap4flags"; `+tt.action)
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("ops = %q, want [%q]", got, tt.want)
			}
		})
	}
}

// TestFlags_InvalidIgnored checks that a flag that is not valid IMAP flag
// syntax, including one with a non-ASCII character, is ignored and the valid
// flags around it kept (RFC 5232 §2; RFC 3501 §9).
//
// checklist: FLAGS-08 FLAGS-09
func TestFlags_InvalidIgnored(t *testing.T) {
	for _, bad := range []string{"(a", "a)", "{1}", "%", "a*", `a\"b`, "a]", "naïve", "\t", "a\x01"} {
		got := flagOps(t, `require "imap4flags"; addflag ["ok", "`+bad+`", "$Junk"];`)
		want := []string{"addflag:ok $Junk"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("flag %q: ops = %q, want %q", bad, got, want)
		}
	}
}

// TestFlags_SystemFlags checks that the settable IMAP system flags are kept in
// their canonical case, and that \Recent and any other system flag a client
// cannot set are ignored (RFC 5232 §2).
//
// checklist: FLAGS-10 FLAGS-11
func TestFlags_SystemFlags(t *testing.T) {
	got := flagOps(t, `require "imap4flags";
setflag ["\\seen", "\\ANSWERED", "\\Flagged", "\\Deleted", "\\Draft", "\\Recent", "\\Junk", "\\"];`)
	want := []string{`setflag:\Seen \Answered \Flagged \Deleted \Draft`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ops = %q, want %q", got, want)
	}
}

// TestFlags_DuplicatesRemoved checks that a flag named twice, in any case,
// reaches the host once (RFC 5232 §3 allows removing duplicates; §3.3 needs
// removeflag to remove every occurrence, which then holds trivially).
//
// checklist: FLAGS-11 FLAGS-13
func TestFlags_DuplicatesRemoved(t *testing.T) {
	got := flagOps(t, `require "imap4flags";
addflag ["Junk", "junk", "JUNK"];
removeflag "a A a";`)
	want := []string{"addflag:Junk", "removeflag:a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ops = %q, want %q", got, want)
	}
}

// TestFlags_RFCExamples runs the examples of RFC 5232 §3.1 and §3.2 that need
// no variables: §3.2's four forms of adding two flags give the same set.
//
// checklist: FLAGS-12
func TestFlags_RFCExamples(t *testing.T) {
	big := sieveEmail()
	big.RawSize = 600 * 1024
	s, err := sieve.Parse(`require "imap4flags"; if size :over 500K { setflag "\\Deleted"; }`)
	if err != nil {
		t.Fatal(err)
	}
	ex := newRecExec()
	s.Evaluate(big, ex)
	if got := ex.meta["imap_flags"]; got != `\Deleted` {
		t.Errorf("§3.1 example: flags = %q, want \\Deleted", got)
	}

	forms := []string{
		`addflag "\\Deleted"; addflag "\\Answered";`,
		`addflag ["\\Deleted", "\\Answered"];`,
		`addflag "\\Deleted \\Answered";`,
		`addflag "\\Answered \\Deleted";`,
	}
	for _, f := range forms {
		r := runSieve(t, `require "imap4flags"; `+f, sieveEmail())
		got := strings.Fields(r.meta["imap_flags"])
		if len(got) != 2 || !containsFold(got, `\Deleted`) || !containsFold(got, `\Answered`) {
			t.Errorf("§3.2 form %q: flags = %q, want \\Deleted and \\Answered", f, got)
		}
	}
}

// TestFlags_ImplicitKeepUnchanged checks that the flag actions do not cancel
// the implicit keep (RFC 5232 §6).
//
// checklist: FLAGS-14
func TestFlags_ImplicitKeepUnchanged(t *testing.T) {
	r := runSieve(t, `require "imap4flags"; setflag "\\Seen"; addflag "a"; removeflag "a";`, sieveEmail())
	if !r.implicitKeep {
		t.Error("implicit keep cancelled by a flag action")
	}
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
