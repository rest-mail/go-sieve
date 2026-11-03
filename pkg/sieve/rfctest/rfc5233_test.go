package rfctest

import (
	"fmt"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// These tests check RFC 5233 (the "subaddress" extension) clause by clause.

// toMessage is a message whose To header and envelope recipient are addr.
func toMessage(addr string) *sieve.Message {
	return &sieve.Message{
		Headers:  sieve.Headers{To: []sieve.Address{{Address: addr}}},
		Envelope: sieve.Envelope{To: []string{addr}},
	}
}

// TestSubaddress_RequiresCapability checks that the capability string is
// "subaddress" and that :user and :detail are errors unless the script
// declares it (RFC 5233 §3; RFC 5228 §3.2).
//
// checklist: SUBADDRESS-01
func TestSubaddress_RequiresCapability(t *testing.T) {
	good := []string{
		`require "subaddress"; if address :user "to" "ken" { keep; }`,
		`require "subaddress"; if address :detail "to" "sieve" { keep; }`,
		`require ["envelope", "subaddress"]; if envelope :detail "to" "sieve" { keep; }`,
	}
	for _, s := range good {
		if err := sieve.Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`if address :user "to" "ken" { keep; }`,
		`if address :detail "to" "sieve" { keep; }`,
		`require "envelope"; if envelope :user "to" "ken" { keep; }`,
		`require "envelope"; if false { if envelope :detail "to" "x" { keep; } }`,
		`require "sub-address"; if address :user "to" "ken" { keep; }`,
	}
	for _, s := range bad {
		if err := sieve.Validate(s); err == nil {
			t.Errorf("%q: accepted without require \"subaddress\", want an error", s)
		}
	}
}

// TestSubaddress_Syntax checks that :user and :detail are ADDRESS-PARTs under
// the rules of RFC 5228 §2.7.4 (RFC 5233 §4): they apply only to tests that act
// on addresses, and a test takes at most one address part.
//
// checklist: SUBADDRESS-02
func TestSubaddress_Syntax(t *testing.T) {
	req := `require ["subaddress", "envelope", "body", "relational"];` + "\n"
	good := []string{
		`address :user "to" "ken"`,
		`address :detail :is "to" "sieve"`,
		`address :matches :detail :comparator "i;octet" "to" "s*"`,
		`envelope :user "to" "ken"`,
		`envelope :detail :contains "to" "iev"`,
		`address :detail :value "gt" "to" "a"`,
		`address :user :count "eq" "to" "1"`,
	}
	for _, test := range good {
		if err := sieve.Validate(req + `if ` + test + ` { keep; }`); err != nil {
			t.Errorf("%s: %v", test, err)
		}
	}
	bad := []string{
		`header :user "to" "ken"`,
		`header :detail "to" "sieve"`,
		`body :detail "sieve"`,
		`exists :user "to"`,
		`address :user :detail "to" "ken"`,
		`address :user :user "to" "ken"`,
		`address :localpart :user "to" "ken"`,
		`address :detail :domain "to" "ken"`,
		`envelope :all :detail "to" "ken"`,
	}
	for _, test := range bad {
		if err := sieve.Validate(req + `if ` + test + ` { keep; }`); err == nil {
			t.Errorf("%s: accepted, want an error", test)
		}
	}
}

// TestSubaddress_User checks the :user address part: the user sub-part of the
// local part, or the whole local part when the address encodes no detail
// (RFC 5233 §4).
//
// checklist: SUBADDRESS-03
func TestSubaddress_User(t *testing.T) {
	cases := []struct {
		addr, user string
	}{
		{"ken+sieve@example.org", "ken"},
		{"ken@example.org", "ken"},             // no detail: the whole local part
		{"ken+@example.org", "ken"},            // empty detail
		{"+sieve@example.org", ""},             // empty user
		{"ken+a+b@example.org", "ken"},         // split at the first separator
		{"Ken+Sieve@Example.org", "Ken"},       // case is the comparator's business
		{"ken+x@[192.0.2.1]", "ken"},           // domain literal
		{"ken-sieve@example.org", "ken-sieve"}, // not the separator
		{"ken+sieve", "ken"},                   // no domain: the whole string is the local part
	}
	req := withFileinto("subaddress", "envelope")
	for _, c := range cases {
		msg := toMessage(c.addr)
		for _, test := range []string{
			fmt.Sprintf(`address :user :comparator "i;octet" "to" %q`, c.user),
			fmt.Sprintf(`envelope :user :comparator "i;octet" "to" %q`, c.user),
		} {
			if !evalTest(t, hit(req, test), msg) {
				t.Errorf("%s: %s did not match", c.addr, test)
			}
		}
		if evalTest(t, hit(req, fmt.Sprintf(`address :user :comparator "i;octet" "to" %q`, c.user+"x")), msg) {
			t.Errorf("%s: :user matched %q", c.addr, c.user+"x")
		}
	}
	// The default comparator folds ASCII case for :user as for any part.
	if !evalTest(t, hit(req, `address :user "to" "KEN"`), toMessage("ken+sieve@example.org")) {
		t.Error(":user ignored the default comparator")
	}
}

// TestSubaddress_Detail checks the :detail address part: the detail sub-part
// of the local part; an address that encodes no detail matches no key, under
// any match type; a zero-length detail is the empty value "" (RFC 5233 §4).
//
// checklist: SUBADDRESS-04
func TestSubaddress_Detail(t *testing.T) {
	req := withFileinto("subaddress", "envelope", "relational", "comparator-i;ascii-numeric")
	type check struct {
		test string
		want bool
	}
	cases := []struct {
		addr   string
		checks []check
	}{
		{"ken+sieve@example.org", []check{
			{`address :detail "to" "sieve"`, true},
			{`envelope :detail "to" "sieve"`, true},
			{`address :detail "to" "ken"`, false},
			{`address :detail "to" ""`, false},
			{`address :detail :matches "to" "*"`, true},
		}},
		{"ken+a+b@example.org", []check{
			{`address :detail "to" "a+b"`, true}, // everything after the first separator
			{`address :detail "to" "b"`, false},
		}},
		{"ken@example.org", []check{
			// No detail encoded: the address fails to match any key.
			{`address :detail "to" ""`, false},
			{`address :detail :matches "to" "*"`, false},
			{`address :detail :contains "to" ""`, false},
			{`address :detail :value "ne" "to" "anything"`, false},
			{`address :detail :value "ge" "to" ""`, false},
			{`envelope :detail :matches "to" "*"`, false},
			{`not address :detail :matches "to" "*"`, true},
			// :count still counts the address: it counts entities, not parts.
			{`address :detail :count "eq" :comparator "i;ascii-numeric" "to" "1"`, true},
			{`address :user "to" "ken"`, true},
		}},
		{"ken+@example.org", []check{
			// A zero-length detail resolves to "".
			{`address :detail "to" ""`, true},
			{`address :detail :matches "to" "*"`, true},
			{`address :detail :matches "to" "?*"`, false},
			{`envelope :detail "to" ""`, true},
		}},
	}
	for _, c := range cases {
		msg := toMessage(c.addr)
		for _, ch := range c.checks {
			if got := evalTest(t, hit(req, ch.test), msg); got != ch.want {
				t.Errorf("%s: %s = %v, want %v", c.addr, ch.test, got, ch.want)
			}
		}
	}
	// With several addresses, only those that carry a detail take part.
	mixed := &sieve.Message{Headers: sieve.Headers{To: []sieve.Address{
		{Address: "ann@example.org"}, {Address: "ken+lists@example.org"},
	}}}
	if !evalTest(t, hit(req, `address :detail "to" "lists"`), mixed) {
		t.Error("detail of the second address did not match")
	}
	if evalTest(t, hit(req, `address :detail :value "lt" "to" "a"`), mixed) {
		t.Error(`the address with no detail took part in :value "lt"`)
	}
}

// TestSubaddress_HostEncoding checks that the detail encoding comes from the
// host, which must match the encompassing mail system (RFC 5233 §4): its
// separator, a detail before the separator, whether a detail may be empty, and
// no subaddressing at all. Repeated separators split at the first occurrence,
// the implementation-defined choice this package documents.
//
// checklist: SUBADDRESS-05
func TestSubaddress_HostEncoding(t *testing.T) {
	type want struct {
		user      string
		detail    string
		hasDetail bool
	}
	cases := []struct {
		name string
		enc  *sieve.Subaddress // nil: the default
		addr string
		want want
	}{
		{"default plus", nil, "ken+sieve@example.org", want{"ken", "sieve", true}},
		{"default ignores minus", nil, "ken-sieve@example.org", want{"ken-sieve", "", false}},
		{"default empty detail", nil, "ken+@example.org", want{"ken", "", true}},
		{"minus", &sieve.Subaddress{Separator: "-"}, "ken-sieve@example.org", want{"ken", "sieve", true}},
		{"minus ignores plus", &sieve.Subaddress{Separator: "-"}, "ken+sieve@example.org", want{"ken+sieve", "", false}},
		{"multi-octet separator", &sieve.Subaddress{Separator: "--"}, "ken--sieve@example.org", want{"ken", "sieve", true}},
		{"multi-octet partial", &sieve.Subaddress{Separator: "--"}, "ken-sieve@example.org", want{"ken-sieve", "", false}},
		{"hash, voice mail", &sieve.Subaddress{Separator: "#"}, "5551212#123@example.com", want{"5551212", "123", true}},
		{"repeated separator", &sieve.Subaddress{Separator: "+"}, "a+b+c@example.org", want{"a", "b+c", true}},
		{"detail first", &sieve.Subaddress{Separator: "--", DetailFirst: true}, "sieve--ken@example.org", want{"ken", "sieve", true}},
		{"detail first repeated", &sieve.Subaddress{Separator: "--", DetailFirst: true}, "a--b--c@example.org", want{"b--c", "a", true}},
		{"detail first empty", &sieve.Subaddress{Separator: "--", DetailFirst: true}, "--ken@example.org", want{"ken", "", true}},
		{"no empty detail", &sieve.Subaddress{Separator: "+", NoEmptyDetail: true}, "ken+@example.org", want{"ken+", "", false}},
		{"no empty detail, with detail", &sieve.Subaddress{Separator: "+", NoEmptyDetail: true}, "ken+x@example.org", want{"ken", "x", true}},
		{"no empty detail first", &sieve.Subaddress{Separator: "--", DetailFirst: true, NoEmptyDetail: true}, "--ken@example.org", want{"--ken", "", false}},
		{"no subaddressing", &sieve.Subaddress{}, "ken+sieve@example.org", want{"ken+sieve", "", false}},
	}
	req := withFileinto("subaddress", "envelope")
	for _, c := range cases {
		var opts []sieve.Option
		if c.enc != nil {
			opts = append(opts, sieve.WithSubaddress(*c.enc))
		}
		msg := toMessage(c.addr)
		for _, test := range []string{"address", "envelope"} {
			user := fmt.Sprintf(`%s :user :comparator "i;octet" "to" %q`, test, c.want.user)
			if !evalTest(t, hit(req, user), msg, opts...) {
				t.Errorf("%s: %s did not match", c.name, user)
			}
			detail := fmt.Sprintf(`%s :detail :comparator "i;octet" "to" %q`, test, c.want.detail)
			if got := evalTest(t, hit(req, detail), msg, opts...); got != c.want.hasDetail {
				t.Errorf("%s: %s = %v, want %v", c.name, detail, got, c.want.hasDetail)
			}
			any := fmt.Sprintf(`%s :detail :matches "to" "*"`, test)
			if got := evalTest(t, hit(req, any), msg, opts...); got != c.want.hasDetail {
				t.Errorf("%s: %s = %v, want %v", c.name, any, got, c.want.hasDetail)
			}
		}
	}
	// The default is exported so a host can start from it.
	if sieve.DefaultSubaddress != (sieve.Subaddress{Separator: "+"}) {
		t.Errorf("DefaultSubaddress = %+v", sieve.DefaultSubaddress)
	}
	// Validate accepts the option like Parse.
	if err := sieve.Validate(`require "subaddress"; if address :detail "to" "x" { keep; }`, sieve.WithSubaddress(sieve.Subaddress{Separator: "-"})); err != nil {
		t.Error(err)
	}
}

// TestSubaddress_RFCExample runs the example script of RFC 5233 §4, where the
// detail follows "+" and one account receives mail for "ken" and
// "postmaster".
//
// checklist: SUBADDRESS-06
func TestSubaddress_RFCExample(t *testing.T) {
	script := `require ["envelope", "subaddress", "fileinto"];

   # In this example the same user account receives mail for both
   # "ken@example.com" and "postmaster@example.com"

   # File all messages to postmaster into a single mailbox,
   # ignoring the :detail part.
   if envelope :user "to" "postmaster" {
       fileinto "inbox.postmaster";
       stop;
   }

   # File mailing list messages (subscribed as "ken+mta-filters").
   if envelope :detail "to" "mta-filters" {
       fileinto "inbox.ietf-mta-filters";
   }

   # Redirect all mail sent to "ken+foo".
   if envelope :detail "to" "foo" {
       redirect "ken@example.net";
   }
`
	s, err := sieve.Parse(script)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := []struct {
		to   string
		want []string
		keep bool
	}{
		{"postmaster@example.com", []string{"fileinto:inbox.postmaster"}, false},
		{"postmaster+mta-filters@example.com", []string{"fileinto:inbox.postmaster"}, false},
		{"ken+mta-filters@example.com", []string{"fileinto:inbox.ietf-mta-filters"}, false},
		{"ken+foo@example.com", []string{"redirect:ken@example.net"}, false},
		{"ken@example.com", nil, true},
		{"ken+other@example.com", nil, true},
	}
	for _, c := range cases {
		ex := newRecExec()
		out := s.Evaluate(&sieve.Message{Envelope: sieve.Envelope{To: []string{c.to}}}, ex)
		if fmt.Sprint(ex.applied) != fmt.Sprint(c.want) || out.ImplicitKeep != c.keep {
			t.Errorf("%s: actions %v keep %v, want %v keep %v", c.to, ex.applied, out.ImplicitKeep, c.want, c.keep)
		}
	}
}
