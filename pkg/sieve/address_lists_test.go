package sieve

import (
	"fmt"
	"net/mail"
	"reflect"
	"strings"
	"testing"
)

// parsedLists reads address fields the way a host holding them lazily would,
// and counts what each test made it do.
type parsedLists struct {
	fields map[string]string
	all    int
	specs  int
	raw    int
	seen   int
}

func (l *parsedLists) All(name string) []Address {
	l.all++
	addrs, err := mail.ParseAddressList(l.fields[name])
	if err != nil {
		return nil
	}
	out := make([]Address, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, Address{Name: a.Name, Address: a.Address})
	}
	return out
}

func (l *parsedLists) Specs(name string, visit func(string) bool) {
	l.specs++
	addrs, _ := mail.ParseAddressList(l.fields[name])
	for _, a := range addrs {
		l.seen++
		if !visit(a.Address) {
			return
		}
	}
}

func (l *parsedLists) RawSpecs(raw string, visit func(string) bool) {
	l.raw++
	addrs, err := mail.ParseAddressList(raw)
	if err != nil {
		return
	}
	for _, a := range addrs {
		l.seen++
		if !visit(a.Address) {
			return
		}
	}
}

// builtAndLazy is one message in both forms: every list built up front, and
// the same lists read through AddressLists.
func builtAndLazy(fields map[string]string, raw map[string][]string) (built, lazy *Message) {
	lists := &parsedLists{fields: fields}
	built = &Message{Headers: Headers{
		From: lists.All("from"), To: lists.All("to"), Cc: lists.All("cc"), Bcc: lists.All("bcc"), Raw: raw,
	}}
	lazy = &Message{Headers: Headers{Lists: &parsedLists{fields: fields}, Raw: raw}}
	return built, lazy
}

// The lazy form answers every test the built form does: header, address and
// size tests, each address part, match type and comparator, on lists of every
// shape, including values that do not read as a list.
//
// checklist: SIEVE-TST-01 SIEVE-TST-02 SIEVE-TST-06 SIEVE-TST-08
func TestAddressLists_AnswerAsBuiltListsDo(t *testing.T) {
	values := []string{
		"", "ann@example.com", "Ann <ann@example.com>", "ANN@EXAMPLE.COM, bob@example.org",
		`"a, b" <quoted@example.net>`, "(note) c@example.com (more)", "team: d@example.com, e@example.com;",
		"=?UTF-8?Q?J=C3=B6rg?= <joerg@example.de>", "user+tag@example.com", "not an address", "a@b,c@d,e@f",
	}
	var fields []map[string]string
	for _, from := range values {
		fields = append(fields, map[string]string{"from": from, "to": "to@example.com", "cc": from})
	}
	raws := []map[string][]string{
		nil,
		{"Reply-To": {"Zed <zed@example.net>", "not an address"}, "SENDER": {"ann@example.com"}},
		{"Resent-To": {"x@example.com, ann@example.com"}, "Subject": {"ann@example.com"}},
	}
	var tests []string
	for _, field := range []string{`"From"`, `"to"`, `"CC"`, `"bcc"`, `"Reply-To"`, `"sender"`, `"Resent-To"`, `"Subject"`, `["from", "cc", "reply-to"]`} {
		for _, part := range []string{"", ":all", ":localpart", ":domain"} {
			for _, kind := range []string{":is", ":contains", ":matches"} {
				for _, comparator := range []string{`:comparator "i;octet"`, `:comparator "i;ascii-casemap"`} {
					for _, key := range []string{"ann@example.com", "ann", "example.com", "*@example.*", "ANN"} {
						tests = append(tests, fmt.Sprintf(`address %s %s %s %s "%s"`, part, kind, comparator, field, key))
					}
				}
			}
		}
		tests = append(tests, fmt.Sprintf(`header :contains %s "ann"`, field), fmt.Sprintf(`exists %s`, field))
	}
	tests = append(tests, `size :over 40`, `size :under 40`, `size :over 80`)

	matched := 0
	for _, test := range tests {
		script, err := Parse(`require ["fileinto"]; if ` + test + ` { fileinto "hit"; }`)
		if err != nil {
			t.Fatalf("%s: %v", test, err)
		}
		for _, field := range fields {
			for _, raw := range raws {
				built, lazy := builtAndLazy(field, raw)
				a, b := newRecExec(), newRecExec()
				script.Evaluate(built, a)
				script.Evaluate(lazy, b)
				if !reflect.DeepEqual(a.applied, b.applied) {
					t.Fatalf("%s on %v %v: built %v, lazy %v", test, field, raw, a.applied, b.applied)
				}
				if strings.Contains(strings.Join(a.applied, " "), "hit") {
					matched++
				}
			}
		}
	}
	if matched == 0 {
		t.Fatal("no test matched, so the comparison proved nothing")
	}
}

// An address test holds no list: it asks for addresses one at a time and stops
// at the first match. A script that tests no address field asks for nothing.
//
// checklist: SIEVE-TST-01
func TestAddressLists_ReadOnlyWhatATestAsksFor(t *testing.T) {
	long := strings.TrimSuffix(strings.Repeat("a@b.example,", 1000), ",")
	message := func() (*Message, *parsedLists) {
		lists := &parsedLists{fields: map[string]string{"from": "first@example.com," + long}}
		return &Message{Headers: Headers{Subject: "hello", Lists: lists}}, lists
	}
	run := func(t *testing.T, test string, msg *Message) {
		t.Helper()
		script, err := Parse(`if ` + test + ` { keep; }`)
		if err != nil {
			t.Fatal(err)
		}
		script.Evaluate(msg, newRecExec())
	}

	t.Run("no address test", func(t *testing.T) {
		msg, lists := message()
		run(t, `header :is "Subject" "hello"`, msg)
		if lists.all+lists.specs+lists.raw != 0 {
			t.Fatalf("read a list no test asked for: %+v", lists)
		}
	})
	t.Run("stops at the first match", func(t *testing.T) {
		msg, lists := message()
		run(t, `address :is "From" "first@example.com"`, msg)
		if lists.all != 0 {
			t.Fatalf("built the whole list %d times", lists.all)
		}
		if lists.seen != 1 {
			t.Fatalf("visited %d addresses, want 1", lists.seen)
		}
	})
	t.Run("a test elsewhere does not read the field", func(t *testing.T) {
		msg, lists := message()
		run(t, `address :is "To" "first@example.com"`, msg)
		if lists.all != 0 || lists.seen != 0 {
			t.Fatalf("read From for a test of To: %+v", lists)
		}
	})
}

// An address test on a field outside RFC 5228's address-bearing set matches
// nothing, however the lists are held.
//
// checklist: SIEVE-TST-01
func TestAddressLists_SubjectIsNotAnAddress(t *testing.T) {
	lists := &parsedLists{}
	msg := &Message{Headers: Headers{Lists: lists, Raw: map[string][]string{"Subject": {"ann@example.com"}}}}
	script, err := Parse(`require ["fileinto"]; if address :is "Subject" "ann@example.com" { fileinto "hit"; }`)
	if err != nil {
		t.Fatal(err)
	}
	exec := newRecExec()
	script.Evaluate(msg, exec)
	if strings.Contains(strings.Join(exec.applied, " "), "hit") {
		t.Fatal("matched a Subject as an address")
	}
	if lists.specs+lists.raw != 0 {
		t.Fatalf("read a list for a Subject: %+v", lists)
	}
}
