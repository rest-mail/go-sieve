package sieve

import (
	"strings"
	"testing"
)

// With variables, an address test on a host's AddressLists expands its field
// names and keys and sets the match variables from the same address the built
// lists would, while still holding one address at a time and stopping at the
// first match.
//
// checklist: VARIABLES-07 VARIABLES-11 VARIABLES-19
func TestAddressLists_Variables(t *testing.T) {
	fields := map[string]string{
		"from": "Ann <ann@one.example>",
		"to":   "bob@two.example, Carol <carol@three.example>",
		"cc":   "dave@four.example",
	}
	raw := map[string][]string{"Reply-To": {"erin@five.example, frank@six.example"}}
	const req = `require ["fileinto", "variables"]; set "h" "to"; set "tld" "example"; `
	for _, test := range []string{
		`address :matches "${h}" "*@*.${tld}"`,
		`address :matches ["from", "${h}"] "*@t*"`,
		`address :domain :matches ["cc", "${h}", "reply-to"] "*e.*"`,
		`address :localpart :matches ["reply-to", "${h}"] "?r*"`,
		`address :is "${h}" "Carol@Three.Example"`,
		`address :matches "${h}" "nobody*"`,
	} {
		script, err := Parse(req + `if ` + test + ` { fileinto "hit=${0}|${1}|${2}"; } else { fileinto "miss=${1}"; }`)
		if err != nil {
			t.Fatalf("%s: %v", test, err)
		}
		built, lazy := builtAndLazy(fields, raw)
		want, got := newRecExec(), newRecExec()
		script.Evaluate(built, want)
		script.Evaluate(lazy, got)
		if got.meta["deliver_to_folder"] != want.meta["deliver_to_folder"] {
			t.Errorf("%s: lists give %q, built lists %q", test, got.meta["deliver_to_folder"], want.meta["deliver_to_folder"])
		}
		if !strings.HasPrefix(want.meta["deliver_to_folder"], "hit=") && !strings.HasSuffix(test, `"nobody*"`) {
			t.Errorf("%s: did not match: %q", test, want.meta["deliver_to_folder"])
		}
	}

	// With relational and subaddress too: an operator built from a variable,
	// :detail and :user feeding the match variables, and :value and :count
	// leaving the match variables of the last :matches as they were.
	subFields := map[string]string{"from": "ann+news@one.example", "to": "bob@two.example, carol+x+y@three.example"}
	const subReq = `require ["fileinto", "variables", "relational", "subaddress", "comparator-i;ascii-numeric"]; set "h" "to"; set "op" "GT"; set "num" "I;ASCII-Numeric"; `
	for _, test := range []string{
		`address :detail :matches ["from", "${h}"] "*"`,
		`address :user :matches "${h}" "c*"`,
		`address :detail :matches "${h}" "*+*"`,
		`address :value "${op}" :localpart "${h}" "bob"`,
		`allof (address :matches "from" "*+*", address :value "${op}" :domain "${h}" "a")`,
		`allof (address :matches "from" "*+*", address :count "eq" :comparator "i;ascii-numeric" ["from", "${h}"] "3")`,
		`address :count "eq" :comparator "${num}" ["from", "${h}"] "03"`,
		`address :value "gt" :comparator "${num}" :localpart "${h}" "99"`,
	} {
		script, err := Parse(subReq + `if ` + test + ` { fileinto "hit=${0}|${1}|${2}"; } else { fileinto "miss=${1}"; }`)
		if err != nil {
			t.Fatalf("%s: %v", test, err)
		}
		built, lazy := builtAndLazy(subFields, nil)
		want, got := newRecExec(), newRecExec()
		script.Evaluate(built, want)
		script.Evaluate(lazy, got)
		if got.meta["deliver_to_folder"] != want.meta["deliver_to_folder"] {
			t.Errorf("%s: lists give %q, built lists %q", test, got.meta["deliver_to_folder"], want.meta["deliver_to_folder"])
		}
		if !strings.HasPrefix(want.meta["deliver_to_folder"], "hit=") {
			t.Errorf("%s: did not match: %q", test, want.meta["deliver_to_folder"])
		}
	}
	for test, folder := range map[string]string{
		`address :detail :matches ["from", "${h}"] "*"`:                                    "hit=news|news|",
		`address :user :matches "${h}" "c*"`:                                               "hit=carol|arol|",
		`address :detail :matches "${h}" "*+*"`:                                            "hit=x+y|x|y",
		`address :value "${op}" :localpart "${h}" "bob"`:                                   "hit=||",
		`allof (address :matches "from" "*+*", address :value "${op}" :domain "${h}" "a")`: "hit=ann+news@one.example|ann|news@one.example",
	} {
		script, err := Parse(subReq + `if ` + test + ` { fileinto "hit=${0}|${1}|${2}"; }`)
		if err != nil {
			t.Fatal(err)
		}
		_, lazy := builtAndLazy(subFields, nil)
		ex := newRecExec()
		script.Evaluate(lazy, ex)
		if ex.meta["deliver_to_folder"] != folder {
			t.Errorf("%s: folder %q, want %q", test, ex.meta["deliver_to_folder"], folder)
		}
	}
	// An operator that expands to no operator is a runtime error with the
	// fail-safe keep, on the lists as on built ones.
	script, err := Parse(subReq + `set "op" "bogus"; if address :value "${op}" "${h}" "x" { fileinto "hit"; }`)
	if err != nil {
		t.Fatal(err)
	}
	_, lazy := builtAndLazy(subFields, nil)
	if out := script.Evaluate(lazy, newRecExec()); out.Error == nil || !out.ImplicitKeep {
		t.Errorf("bogus operator: %+v, want a runtime error and the implicit keep", out)
	}

	// A comparator name built from a variable is expanded and checked when the
	// test runs (#100): i;octet tells case apart where the default does not,
	// and a name that is not declared is a runtime error with the fail-safe
	// keep, on the lists as on built ones.
	for comparator, folder := range map[string]string{"i;ascii-casemap": "hit", "i;octet": ""} {
		script, err := Parse(subReq + `set "c" "` + comparator + `"; if address :is :comparator "${c}" "${h}" "BOB@two.example" { fileinto "hit"; }`)
		if err != nil {
			t.Fatal(err)
		}
		built, lazy := builtAndLazy(subFields, nil)
		for _, msg := range []*Message{built, lazy} {
			ex := newRecExec()
			if out := script.Evaluate(msg, ex); out.Error != nil || ex.meta["deliver_to_folder"] != folder {
				t.Errorf("comparator %s: folder %q, error %v, want %q", comparator, ex.meta["deliver_to_folder"], out.Error, folder)
			}
		}
	}
	script, err = Parse(subReq + `set "c" "i;bogus"; if address :is :comparator "${c}" "${h}" "x" { fileinto "hit"; }`)
	if err != nil {
		t.Fatal(err)
	}
	built, lazy := builtAndLazy(subFields, nil)
	for _, msg := range []*Message{built, lazy} {
		if out := script.Evaluate(msg, newRecExec()); out.Error == nil || !out.ImplicitKeep || !strings.Contains(out.Error.Error(), `"i;bogus"`) {
			t.Errorf("bogus comparator: %+v, want a runtime error naming it and the implicit keep", out)
		}
	}

	// Still one address at a time, stopping at the first match.
	long := strings.TrimSuffix(strings.Repeat("a@b.example,", 1000), ",")
	lists := &parsedLists{fields: map[string]string{"from": "first@example.com," + long}}
	script, err = Parse(crlf(`require ["fileinto", "variables"]; set "f" "From";
if address :matches "${f}" "*@*" { fileinto "${1}"; }`))
	if err != nil {
		t.Fatal(err)
	}
	ex := newRecExec()
	script.Evaluate(&Message{Headers: Headers{Lists: lists}}, ex)
	if ex.meta["deliver_to_folder"] != "first" || lists.all != 0 || lists.seen != 1 {
		t.Errorf("folder %q, built %d lists, visited %d addresses; want first, 0, 1", ex.meta["deliver_to_folder"], lists.all, lists.seen)
	}

	// A field name that expands to an invalid one, here the empty string of an
	// unset variable, is skipped, and an empty addr-spec from the host is never
	// the matching address, so "*" captures the first real one.
	script, err = Parse(crlf(`require ["fileinto", "variables"];
if address :matches ["${unset}", "From"] "*" { fileinto "=${0}"; }`))
	if err != nil {
		t.Fatal(err)
	}
	ex = newRecExec()
	script.Evaluate(&Message{Headers: Headers{Lists: emptyFirstLists{}}}, ex)
	if got := ex.meta["deliver_to_folder"]; got != "=first@example.com" {
		t.Errorf("folder %q, want =first@example.com", got)
	}
}

// emptyFirstLists hands out an empty addr-spec before each real one, as a
// host might for an address with no addr-spec.
type emptyFirstLists struct{}

func (emptyFirstLists) All(string) []Address { return []Address{{Address: "first@example.com"}} }
func (emptyFirstLists) Specs(_ string, visit func(string) bool) {
	if visit("") {
		visit("first@example.com")
	}
}
func (emptyFirstLists) RawSpecs(string, func(string) bool) {}
