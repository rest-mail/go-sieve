package rfctest

import (
	"fmt"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// These tests check RFC 5231 (the "relational" extension) clause by clause.

const relationalNumeric = `require ["relational", "comparator-i;ascii-numeric"];
`

// evalTest parses script and reports whether it filed the message into "Hit".
func evalTest(t *testing.T, script string, msg *sieve.Message, opts ...sieve.Option) bool {
	t.Helper()
	s, err := sieve.Parse(crlf(script), opts...)
	if err != nil {
		t.Fatalf("Parse(crlf(%q)): %v", script, err)
	}
	ex := newRecExec()
	if out := s.Evaluate(msg, ex); out.Error != nil {
		t.Fatalf("Evaluate(%q): %v", script, out.Error)
	}
	return ex.meta["deliver_to_folder"] == "Hit"
}

// hit wraps a test in a script that files a matching message into "Hit".
func hit(require, test string) string {
	return require + `if ` + test + ` { fileinto "Hit"; }`
}

// withFileinto declares fileinto alongside the given capabilities.
func withFileinto(caps ...string) string {
	list := `"fileinto"`
	for _, c := range caps {
		list += fmt.Sprintf(", %q", c)
	}
	return "require [" + list + "];\n"
}

// TestRelational_RequiresCapability checks that the capability string is
// "relational" and that :value and :count are errors unless the script
// declares it (RFC 5231 §1, §9; RFC 5228 §3.2).
//
// checklist: RELATIONAL-01
func TestRelational_RequiresCapability(t *testing.T) {
	good := []string{
		`require "relational"; if header :value "gt" "X-Priority" "3" { keep; }`,
		`require "relational"; if header :count "gt" "Received" "3" { keep; }`,
		relationalNumeric + `if address :count "ge" :comparator "i;ascii-numeric" "to" "1" { keep; }`,
	}
	for _, s := range good {
		if err := sieve.Validate(crlf(s)); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`if header :value "gt" "X-Priority" "3" { keep; }`,
		`if header :count "gt" "Received" "3" { keep; }`,
		`require "comparator-i;ascii-numeric"; if header :value "gt" :comparator "i;ascii-numeric" "X-Priority" "3" { keep; }`,
		`require "envelope"; if envelope :count "eq" "to" "1" { keep; }`,
		// A require must name the capability exactly.
		`require "relational-ops"; if header :value "gt" "X-Priority" "3" { keep; }`,
	}
	for _, s := range bad {
		if err := sieve.Validate(crlf(s)); err == nil {
			t.Errorf("%q: accepted without require \"relational\", want an error", s)
		}
	}
	// The comparator still needs its own require (RFC 5231 §3, RFC 5228 §2.7.3).
	if err := sieve.Validate(`require "relational"; if header :value "gt" :comparator "i;ascii-numeric" "X-Priority" "3" { keep; }`); err == nil {
		t.Error("accepted i;ascii-numeric without require \"comparator-i;ascii-numeric\"")
	}
}

// TestRelational_Syntax checks the grammar of RFC 5231 §4: COUNT and VALUE are
// MATCH-TYPEs followed by one of the six relational operators as a string. The
// operator is an ABNF literal, so it matches regardless of ASCII case
// (RFC 5234 §2.3). A test still takes one match type, and :count is defined
// only for the address, envelope and header tests (§4.2).
//
// checklist: RELATIONAL-02
func TestRelational_Syntax(t *testing.T) {
	good := []string{
		`header :value "gt" "X-Priority" "3"`,
		`header :value "ge" "X-Priority" "3"`,
		`header :value "lt" "X-Priority" "3"`,
		`header :value "le" "X-Priority" "3"`,
		`header :value "eq" "X-Priority" "3"`,
		`header :value "ne" "X-Priority" "3"`,
		`header :value "GT" "X-Priority" "3"`,
		`header :value "Ne" "X-Priority" "3"`,
		`header :comparator "i;ascii-numeric" :value "gt" "X-Priority" "3"`,
		`header :value "gt" :comparator "i;octet" "X-Priority" "3"`,
		`address :value "gt" :domain "from" "m"`,
		`address :localpart :count "eq" "to" "1"`,
		`envelope :count "eq" "to" "1"`,
		`envelope :value "lt" :all "from" "m"`,
		`header :count "eq" ["Received", "Subject"] ["1", "2"]`,
		`body :value "gt" "m"`,
		`body :value "gt" :content "text" "m"`,
	}
	req := `require ["relational", "comparator-i;ascii-numeric", "envelope", "body"];` + "\n"
	for _, test := range good {
		if err := sieve.Validate(crlf(req + `if ` + test + ` { keep; }`)); err != nil {
			t.Errorf("%s: %v", test, err)
		}
	}
	bad := []string{
		`header :value "X-Priority" "3"`,           // operator missing: the header name is not one
		`header :value "gtx" "X-Priority" "3"`,     // unknown operator
		`header :value "" "X-Priority" "3"`,        // empty operator
		`header :value " gt" "X-Priority" "3"`,     // the literal admits no padding
		`header :value ">" "X-Priority" "3"`,       // C spelling is a comment in the ABNF, not syntax
		`header :count ["gt"] "Received" "3"`,      // the operator is a string, not a list
		`header :value 3 "X-Priority" "3"`,         // nor a number
		`header :value`,                            // nothing follows
		`header :count "gt" :is "Received" "3"`,    // two match types
		`header :is :value "gt" "X-Priority" "3"`,  // two match types
		`header :value "gt" :count "gt" "X" "3"`,   // two match types
		`header :value "gt" :value "lt" "X" "3"`,   // duplicate match type
		`header :value "gt" :matches "X" "3"`,      // two match types
		`body :count "gt" "3"`,                     // body has no entities to count
		`header :value "gt" :localpart "from" "a"`, // address parts are only for address/envelope
	}
	for _, test := range bad {
		if err := sieve.Validate(crlf(req + `if ` + test + ` { keep; }`)); err == nil {
			t.Errorf("%s: accepted, want an error", test)
		}
	}
}

// TestRelational_Operators checks that each operator is the C relation it
// names (RFC 5231 §4), with the message value as the left side and the key as
// the right side (§4.1).
//
// checklist: RELATIONAL-03
func TestRelational_Operators(t *testing.T) {
	// want[i] is the result for value 2 against keys 1, 2, 3.
	cases := []struct {
		op   string
		want [3]bool
	}{
		{"gt", [3]bool{true, false, false}},
		{"ge", [3]bool{true, true, false}},
		{"lt", [3]bool{false, false, true}},
		{"le", [3]bool{false, true, true}},
		{"eq", [3]bool{false, true, false}},
		{"ne", [3]bool{true, false, true}},
	}
	msg := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"X-Priority": {"2"}}}}
	for _, c := range cases {
		for i, key := range []string{"1", "2", "3"} {
			test := fmt.Sprintf(`header :value %q :comparator "i;ascii-numeric" "X-Priority" %q`, c.op, key)
			if got := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), msg); got != c.want[i] {
				t.Errorf("2 %s %s = %v, want %v", c.op, key, got, c.want[i])
			}
		}
	}
	// The sides are not interchangeable: the message value is on the left.
	asym := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"X-Priority": {"5"}}}}
	if !evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), `header :value "gt" :comparator "i;ascii-numeric" "X-Priority" "3"`), asym) {
		t.Error(`value 5 "gt" key 3 was false`)
	}
	if evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), `header :value "lt" :comparator "i;ascii-numeric" "X-Priority" "3"`), asym) {
		t.Error(`value 5 "lt" key 3 was true`)
	}
	// :count compares the count on the left with each key on the right.
	two := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"Received": {"a", "b"}}}}
	for _, c := range cases {
		for i, key := range []string{"1", "2", "3"} {
			test := fmt.Sprintf(`header :count %q :comparator "i;ascii-numeric" "Received" %q`, c.op, key)
			if got := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), two); got != c.want[i] {
				t.Errorf("count 2 %s %s = %v, want %v", c.op, key, got, c.want[i])
			}
		}
	}
}

// TestRelational_ValueComparators checks that :value orders strings with the
// test's comparator (RFC 5231 §4.1, RFC 4790 §9): i;ascii-numeric compares
// leading digit runs as integers and makes a string without one positive
// infinity, i;ascii-casemap folds ASCII case before octet order, and i;octet
// compares unsigned octets with a proper prefix first.
//
// checklist: RELATIONAL-04
func TestRelational_ValueComparators(t *testing.T) {
	cases := []struct {
		comparator, value, op, key string
		want                       bool
	}{
		// i;ascii-numeric (RFC 4790 §9.1.1).
		{"i;ascii-numeric", "10", "gt", "9", true},
		{"i;ascii-numeric", "007", "eq", "7", true},
		{"i;ascii-numeric", "0", "eq", "000", true},
		{"i;ascii-numeric", "5.9", "eq", "5", true}, // only the leading digit run counts
		{"i;ascii-numeric", "42abc", "lt", "43", true},
		{"i;ascii-numeric", "abc", "gt", "99999999999999999999", true}, // a non-number is infinity
		{"i;ascii-numeric", "abc", "eq", "xyz", true},                  // and all non-numbers are equal
		{"i;ascii-numeric", "abc", "ne", "xyz", false},
		{"i;ascii-numeric", "", "gt", "1", true},          // the empty string has no digit: infinity
		{"i;ascii-numeric", "\u00a05", "gt", "100", true}, // a leading no-break space is not a digit (and not trimmed)
		{"i;ascii-numeric", "-1", "gt", "1", true},        // no negative numbers (RFC 5231 §3)
		{"i;ascii-numeric", "3", "lt", "abc", true},
		{"i;ascii-numeric", "3", "le", "3", true},
		// i;ascii-casemap (RFC 4790 §9.2): ASCII case folds, then octet order.
		{"i;ascii-casemap", "Mary", "gt", "M", true},
		{"i;ascii-casemap", "apple", "lt", "Banana", true},
		{"i;ascii-casemap", "ABC", "eq", "abc", true},
		{"i;ascii-casemap", "abc", "ne", "ABC", false},
		{"i;ascii-casemap", "ab", "lt", "abc", true}, // a proper prefix sorts first
		{"i;ascii-casemap", "10", "lt", "9", true},   // strings, not numbers
		{"i;ascii-casemap", "Z", "lt", "a", false},   // folded: "z" > "a"
		{"i;ascii-casemap", "ä", "gt", "z", true},    // non-ASCII octets compare unfolded
		// i;octet (RFC 4790 §9.3): unsigned octets, no folding.
		{"i;octet", "Z", "lt", "a", true},
		{"i;octet", "ABC", "eq", "abc", false},
		{"i;octet", "abc", "eq", "abc", true},
		{"i;octet", "ab", "lt", "abc", true},
		{"i;octet", "\xff", "gt", "\x7f", true}, // octets are unsigned
		{"i;octet", "", "lt", "a", true},
		{"i;octet", "", "eq", "", true},
	}
	for _, c := range cases {
		msg := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"X-Test": {c.value}}}}
		test := fmt.Sprintf(`header :value %q :comparator %q "X-Test" %q`, c.op, c.comparator, c.key)
		got := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), msg)
		if got != c.want {
			t.Errorf("%s: %q %s %q = %v, want %v", c.comparator, c.value, c.op, c.key, got, c.want)
		}
	}
	// The default comparator is i;ascii-casemap (RFC 5228 §2.7.3).
	msg := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"X-Test": {"ABC"}}}}
	if !evalTest(t, hit(withFileinto("relational"), `header :value "eq" "X-Test" "abc"`), msg) {
		t.Error("default comparator did not fold ASCII case")
	}
	if !evalTest(t, hit(withFileinto("relational"), `header :value "gt" "X-Test" "10"`), msg) {
		t.Error(`default comparator: "abc" gt "10" was false`)
	}
}

// TestRelational_NumericRange checks that i;ascii-numeric supports at least
// 32-bit unsigned integers (RFC 5231 §3); this package compares digit runs of
// any length.
//
// checklist: RELATIONAL-05
func TestRelational_NumericRange(t *testing.T) {
	cases := []struct {
		value, op, key string
		want           bool
	}{
		{"4294967295", "eq", "4294967295", true},
		{"4294967295", "lt", "4294967296", true},
		{"4294967296", "gt", "4294967295", true},
		{"0", "lt", "4294967295", true},
		{"18446744073709551616", "gt", "18446744073709551615", true},
		{"123456789012345678901234567890", "gt", "123456789012345678901234567889", true},
		{"000000000000000000000000000001", "eq", "1", true},
		{"99999999999999999999999999999", "lt", "100000000000000000000000000000", true},
	}
	for _, c := range cases {
		msg := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"X-Score": {c.value}}}}
		test := fmt.Sprintf(`header :value %q :comparator "i;ascii-numeric" "X-Score" %q`, c.op, c.key)
		if got := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), msg); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.value, c.op, c.key, got, c.want)
		}
	}
}

// TestRelational_ValueAnyPair checks that with several values, several keys or
// both, :value is true if any value-key pair satisfies the relation
// (RFC 5231 §4.1), and false when none does or there is no value.
//
// checklist: RELATIONAL-06
func TestRelational_ValueAnyPair(t *testing.T) {
	msg := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{
		"X-Score": {"2", "8"},
		"X-Other": {"5"},
	}}}
	cases := []struct {
		test string
		want bool
	}{
		{`header :value "gt" :comparator "i;ascii-numeric" "X-Score" "7"`, true},  // 8 > 7
		{`header :value "gt" :comparator "i;ascii-numeric" "X-Score" "8"`, false}, // neither
		{`header :value "lt" :comparator "i;ascii-numeric" "X-Score" "3"`, true},  // 2 < 3
		{`header :value "eq" :comparator "i;ascii-numeric" "X-Score" ["1", "8"]`, true},
		{`header :value "eq" :comparator "i;ascii-numeric" "X-Score" ["1", "9"]`, false},
		{`header :value "ne" :comparator "i;ascii-numeric" "X-Score" "2"`, true}, // 8 != 2
		{`header :value "gt" :comparator "i;ascii-numeric" ["X-Missing", "X-Other"] "4"`, true},
		{`header :value "gt" :comparator "i;ascii-numeric" "X-Missing" "0"`, false}, // no value
		{`header :value "ne" :comparator "i;ascii-numeric" "X-Missing" "0"`, false}, // no value: no pair
	}
	for _, c := range cases {
		if got := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), c.test), msg); got != c.want {
			t.Errorf("%s = %v, want %v", c.test, got, c.want)
		}
	}
}

// TestRelational_CountAddress checks that the address test's :count is the
// number of mailboxes in the named fields, with group names ignored and their
// mailboxes counted, summed over the fields (RFC 5231 §4.2). The address part
// does not change what is counted.
//
// checklist: RELATIONAL-07
func TestRelational_CountAddress(t *testing.T) {
	msg := &sieve.Message{Headers: sieve.Headers{
		To: []sieve.Address{{Address: "foo@example.com"}, {Name: "Baz", Address: "baz@example.com"}},
		Raw: map[string][]string{
			"Cc":       {"qux@example.com"},
			"Reply-To": {"undisclosed: a@example.com, b@example.com;, c@example.com"},
			"Sender":   {"not an address"},
		},
	}}
	cases := []struct {
		fields string
		count  int
	}{
		{`"to"`, 2},
		{`"cc"`, 1},
		{`["to", "cc"]`, 3},
		{`"reply-to"`, 3}, // the group "undisclosed" holds two of them
		{`["to", "cc", "reply-to"]`, 6},
		{`"bcc"`, 0},
		{`"sender"`, 0},  // an unparseable value holds no mailbox
		{`"subject"`, 0}, // not an address field
	}
	for _, c := range cases {
		for _, part := range []string{"", ":all ", ":localpart ", ":domain "} {
			for _, op := range []struct {
				name string
				key  int
				want bool
			}{{"eq", c.count, true}, {"ne", c.count, false}, {"gt", c.count - 1, true}, {"lt", c.count, false}} {
				if op.key < 0 {
					continue
				}
				test := fmt.Sprintf(`address %s:count %q :comparator "i;ascii-numeric" %s "%d"`, part, op.name, c.fields, op.key)
				if got := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), msg); got != op.want {
					t.Errorf("%s = %v, want %v (count %d)", test, got, op.want, c.count)
				}
			}
		}
	}
}

// TestRelational_CountEnvelope checks that the envelope test's :count is the
// number of addresses in the named envelope parts: "from" is 0 for an empty
// MAIL FROM and 1 otherwise, and "to" is the recipient addresses the host
// supplied (RFC 5231 §4.2).
//
// checklist: RELATIONAL-08
func TestRelational_CountEnvelope(t *testing.T) {
	cases := []struct {
		name  string
		env   sieve.Envelope
		parts string
		count int
	}{
		{"sender", sieve.Envelope{From: "a@example.com"}, `"from"`, 1},
		{"null reverse-path", sieve.Envelope{FromNull: true}, `"from"`, 0},
		{"no MAIL FROM", sieve.Envelope{}, `"from"`, 0},
		{"one recipient", sieve.Envelope{To: []string{"me@example.com"}}, `"to"`, 1},
		{"no recipient", sieve.Envelope{}, `"to"`, 0},
		{"summed", sieve.Envelope{From: "a@example.com", To: []string{"me@example.com"}}, `["from", "to"]`, 2},
		{"summed null", sieve.Envelope{FromNull: true, To: []string{"me@example.com"}}, `["to", "from"]`, 1},
	}
	req := withFileinto("relational", "envelope", "comparator-i;ascii-numeric")
	for _, c := range cases {
		msg := &sieve.Message{Envelope: c.env}
		for k := 0; k <= 3; k++ {
			test := fmt.Sprintf(`envelope :count "eq" :comparator "i;ascii-numeric" %s "%d"`, c.parts, k)
			if got, want := evalTest(t, hit(req, test), msg), k == c.count; got != want {
				t.Errorf("%s: %s = %v, want %v", c.name, test, got, want)
			}
		}
	}
	// The null reverse-path still compares as "" under the other match types.
	null := &sieve.Message{Envelope: sieve.Envelope{FromNull: true}}
	if !evalTest(t, hit(req, `envelope :value "eq" :all "from" ""`), null) {
		t.Error(`envelope :value "eq" "from" "" did not match a null reverse-path`)
	}
}

// TestRelational_CountHeader checks that the header test's :count is the total
// number of instances of the named fields, not the addresses in them, summed
// over the names (RFC 5231 §4.2).
//
// checklist: RELATIONAL-09
func TestRelational_CountHeader(t *testing.T) {
	msg := &sieve.Message{Headers: sieve.Headers{
		Subject: "example",
		To:      []sieve.Address{{Address: "foo@example.com"}, {Address: "baz@example.com"}},
		Raw: map[string][]string{
			"Received": {"from a", "from b"},
			"received": {"from c"}, // field names are case-insensitive
			"Cc":       {"qux@example.com, quux@example.com"},
		},
	}}
	cases := []struct {
		fields string
		count  int
	}{
		{`"received"`, 3},
		{`"RECEIVED"`, 3},
		{`"subject"`, 1},
		{`"to"`, 1}, // one field with two addresses
		{`"cc"`, 1},
		{`["to", "cc"]`, 2},
		{`["received", "subject"]`, 4},
		{`"x-missing"`, 0},
		{`"bad name:"`, 0},
	}
	for _, c := range cases {
		for k := 0; k <= 5; k++ {
			test := fmt.Sprintf(`header :count "eq" :comparator "i;ascii-numeric" %s "%d"`, c.fields, k)
			if got, want := evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), msg), k == c.count; got != want {
				t.Errorf("%s = %v, want %v", test, got, want)
			}
		}
	}
	// Each structured field is one instance when set, whatever it holds.
	full := &sieve.Message{Headers: sieve.Headers{
		Subject:    "s",
		From:       []sieve.Address{{Address: "a@example.com"}, {Address: "b@example.com"}},
		To:         []sieve.Address{{Name: "Only A Name"}},
		Cc:         []sieve.Address{{Address: "c@example.com"}},
		Bcc:        []sieve.Address{{Address: "d@example.com"}},
		MessageID:  "<id@example.com>",
		InReplyTo:  "<parent@example.com>",
		Date:       "Mon, 17 Feb 2026 10:00:00 +0000",
		References: []string{"<a@example.com>", "<b@example.com>"},
	}}
	empty := &sieve.Message{Headers: sieve.Headers{To: []sieve.Address{{}}}}
	fields := []string{"subject", "from", "to", "cc", "bcc", "message-id", "in-reply-to", "date", "references"}
	for _, f := range fields {
		test := fmt.Sprintf(`header :count "eq" :comparator "i;ascii-numeric" %q "1"`, f)
		if !evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), full) {
			t.Errorf("structured %s did not count as one field", f)
		}
		test = fmt.Sprintf(`header :count "eq" :comparator "i;ascii-numeric" %q "0"`, f)
		if !evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), test), empty) {
			t.Errorf("unset structured %s did not count as zero fields", f)
		}
	}
	all := `header :count "eq" :comparator "i;ascii-numeric" ["subject", "from", "to", "cc", "bcc", "message-id", "in-reply-to", "date", "references"] "9"`
	if !evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), all), full) {
		t.Error("the nine structured fields did not sum to 9")
	}
	// The count is compared as a decimal string, so a non-numeric comparator
	// orders it as text (RFC 5231 §4.2 says it SHOULD be numeric).
	ten := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{"Received": make([]string, 10)}}}
	if !evalTest(t, hit(withFileinto("relational"), `header :count "lt" "Received" "9"`), ten) {
		t.Error(`count 10 "lt" "9" under i;ascii-casemap was false`)
	}
	if !evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), `header :count "gt" :comparator "i;ascii-numeric" "Received" "9"`), ten) {
		t.Error(`count 10 "gt" "9" under i;ascii-numeric was false`)
	}
	// A key that is not a number is infinity, which every count is below.
	if !evalTest(t, hit(withFileinto("relational", "comparator-i;ascii-numeric"), `header :count "lt" :comparator "i;ascii-numeric" "Received" "many"`), ten) {
		t.Error(`count "lt" a non-numeric key was false`)
	}
}

// rfc5231Message is the message of RFC 5231 §6, given once with structured
// fields and once with every field raw, as two hosts might adapt it.
func rfc5231Message() []*sieve.Message {
	structured := &sieve.Message{Headers: sieve.Headers{
		Subject: "example",
		To:      []sieve.Address{{Address: "foo@example.com"}, {Address: "baz@example.com"}},
		Cc:      []sieve.Address{{Address: "qux@example.com"}},
		Raw:     map[string][]string{"received": {"...", "..."}},
	}}
	raw := &sieve.Message{Headers: sieve.Headers{Raw: map[string][]string{
		"received": {"...", "..."},
		"subject":  {"example"},
		"to":       {"foo@example.com, baz@example.com"},
		"cc":       {"qux@example.com"},
	}}}
	return []*sieve.Message{structured, raw}
}

// TestRelational_RFCExamples runs the examples of RFC 5231 §6 against its
// message, and the extended example of §7.
//
// checklist: RELATIONAL-10
func TestRelational_RFCExamples(t *testing.T) {
	req := withFileinto("relational", "comparator-i;ascii-numeric")
	cases := []struct {
		test string
		want bool
	}{
		{`address :count "ge" :comparator "i;ascii-numeric"
                      ["to", "cc"] ["3"]`, true},
		{`anyof ( address :count "ge" :comparator "i;ascii-numeric"
                      ["to"] ["3"],
              address :count "ge" :comparator "i;ascii-numeric"
                      ["cc"] ["3"] )`, false},
		{`header :count "ge" :comparator "i;ascii-numeric"
                      ["received"] ["3"]`, false},
		{`header :count "ge" :comparator "i;ascii-numeric"
                      ["received", "subject"] ["3"]`, true},
		{`header :count "ge" :comparator "i;ascii-numeric"
                      ["to", "cc"] ["3"]`, false},
	}
	for i, msg := range rfc5231Message() {
		for _, c := range cases {
			if got := evalTest(t, hit(req, c.test), msg); got != c.want {
				t.Errorf("message %d: %s = %v, want %v", i, c.test, got, c.want)
			}
		}
	}

	extended := `require ["relational", "comparator-i;ascii-numeric", "fileinto"];

   if header :value "lt" :comparator "i;ascii-numeric"
             ["x-priority"] ["3"]
   {
      fileinto "Priority";
   }
   elsif address :count "gt" :comparator "i;ascii-numeric"
              ["to"] ["5"]
   {
      # everything with more than 5 recipients in the "to" field
      # is considered SPAM
      fileinto "SPAM";
   }
   elsif address :value "gt" :all :comparator "i;ascii-casemap"
              ["from"] ["M"]
   {
      fileinto "From N-Z";
   } else {
      fileinto "From A-M";
   }
   if allof ( address :count "eq" :comparator "i;ascii-numeric"
                      ["to", "cc"] ["1"] ,
              address :all :comparator "i;ascii-casemap"
                      ["to", "cc"] ["me@foo.example.com"] )
   {
      fileinto "Only me";
   }
`
	s, err := sieve.Parse(crlf(extended))
	if err != nil {
		t.Fatalf("Parse §7 example: %v", err)
	}
	me := []sieve.Address{{Address: "me@foo.example.com"}}
	six := []sieve.Address{{Address: "a@x.example"}, {Address: "b@x.example"}, {Address: "c@x.example"},
		{Address: "d@x.example"}, {Address: "e@x.example"}, {Address: "f@x.example"}}
	five := six[:5]
	runs := []struct {
		name string
		h    sieve.Headers
		want []string
	}{
		{"priority 1", sieve.Headers{Raw: map[string][]string{"X-Priority": {"1"}}, From: []sieve.Address{{Address: "zed@example.com"}}, To: me},
			[]string{"fileinto:Priority", "fileinto:Only me"}},
		{"priority 3 is not below 3", sieve.Headers{Raw: map[string][]string{"X-Priority": {"3"}}, From: []sieve.Address{{Address: "zed@example.com"}}, To: me},
			[]string{"fileinto:From N-Z", "fileinto:Only me"}},
		{"six recipients", sieve.Headers{From: []sieve.Address{{Address: "amy@example.com"}}, To: six},
			[]string{"fileinto:SPAM"}},
		{"five recipients", sieve.Headers{From: []sieve.Address{{Address: "amy@example.com"}}, To: five},
			[]string{"fileinto:From A-M"}},
		{"from N-Z", sieve.Headers{From: []sieve.Address{{Address: "Nora@example.com"}}, To: me},
			[]string{"fileinto:From N-Z", "fileinto:Only me"}},
		{"from M exactly sorts below M...", sieve.Headers{From: []sieve.Address{{Address: "m"}}, To: me},
			[]string{"fileinto:From A-M", "fileinto:Only me"}},
		{"me and a cc", sieve.Headers{From: []sieve.Address{{Address: "amy@example.com"}}, To: me, Cc: []sieve.Address{{Address: "x@example.com"}}},
			[]string{"fileinto:From A-M"}},
	}
	for _, r := range runs {
		ex := newRecExec()
		s.Evaluate(&sieve.Message{Headers: r.h}, ex)
		if fmt.Sprint(ex.applied) != fmt.Sprint(r.want) {
			t.Errorf("%s: actions %v, want %v", r.name, ex.applied, r.want)
		}
	}
}
