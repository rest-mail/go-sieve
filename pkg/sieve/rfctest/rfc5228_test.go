package rfctest

import (
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// matchedFolder runs a script that files into "hit" when its test matches, and
// reports whether it did.
func matchedFolder(t *testing.T, test string, msg *sieve.Message) bool {
	t.Helper()
	r := runSieve(t, `require ["fileinto", "envelope"]; if `+test+` { fileinto "hit"; }`, msg)
	return folderOf(r) == "hit"
}

// TestRFC5228_IdentifiersCaseInsensitive checks that command, test and tag
// names are case-insensitive (§2.1), though quoted strings are not.
//
// checklist: SIEVE-LEX-02
func TestRFC5228_IdentifiersCaseInsensitive(t *testing.T) {
	r := runSieve(t, `REQUIRE "fileinto"; IF HEADER :IS "Subject" "Test message" { FILEINTO "hit"; STOP; }`, sieveEmail())
	if folderOf(r) != "hit" {
		t.Errorf("an upper-case script did not file into hit: %+v", r)
	}
	if err := sieve.Validate(`if header :IS :COMPARATOR "I;OCTET" "a" "b" { keep; }`); err != nil {
		t.Errorf("tag and comparator names are case-insensitive, got %v", err)
	}
}

// TestRFC5228_SyntaxErrors checks that scripts breaking the grammar of §8.2 are
// rejected: Validate is how a host (ManageSieve) learns a script is invalid.
//
// checklist: SIEVE-ACT-01 SIEVE-ACT-03 SIEVE-CMD-02 SIEVE-LEX-01 SIEVE-LEX-05 SIEVE-LEX-07 SIEVE-LEX-09 SIEVE-LEX-10 SIEVE-LEX-11 SIEVE-MT-04 SIEVE-MT-06 SIEVE-MT-08 SIEVE-TST-03 SIEVE-TST-08 SIEVE-TST-11
func TestRFC5228_SyntaxErrors(t *testing.T) {
	bad := map[string]string{
		"missing semicolon":                "keep",
		"block without braces":             "if true keep;",
		"else without if":                  "else { keep; }",
		"elsif without if":                 "elsif true { keep; }",
		"else after else":                  "if true { keep; } else { keep; } else { keep; }",
		"elsif after else":                 "if true { keep; } else { keep; } elsif true { keep; }",
		"stray closing brace":              "if true { keep; } }",
		"unclosed block":                   "if true { keep;",
		"unterminated bracket comment":     "/* never closed keep;",
		"unterminated quoted string":       `if header :is "Subject" "x { keep; }`,
		"unterminated multi-line string":   "if header :is \"Subject\" text:\nabc\n { keep; }",
		"stop with an argument":            "stop true;",
		"discard with an argument":         `discard "x";`,
		"redirect without an address":      "redirect;",
		"redirect with two addresses":      `redirect "a@example.com" "b@example.com";`,
		"fileinto without a folder":        `require "fileinto"; fileinto;`,
		"empty allof":                      "if allof() { keep; }",
		"empty anyof":                      "if anyof( ) { keep; }",
		"not without a test":               "if not { keep; }",
		"exists without a header":          "if exists { keep; }",
		"header without a key":             `if header :is "Subject" { keep; }`,
		"size without :over or :under":     "if size 1 { keep; }",
		"size with a missing number":       "if size :over { keep; }",
		"trailing comma in a string list":  `if header :is ["a",] "x" { keep; }`,
		"unterminated string list":         `if header :is ["a", "b" "x" { keep; }`,
		"command in an expression":         "if keep { keep; }",
		"if without a test":                "if { keep; }",
		"test used as a command":           `header :is "a" "b";`,
		"two match types":                  `if header :is :contains "a" "b" { keep; }`,
		"two address parts":                `if address :all :domain :is "From" "x" { keep; }`,
		"two comparators":                  `if header :comparator "i;octet" :comparator "i;octet" :is "a" "b" { keep; }`,
		"comparator without a name":        `if header :comparator :is "a" "b" { keep; }`,
		"unknown comparator":               `if header :comparator "i;nonesuch" :is "a" "b" { keep; }`,
		"address part on header":           `if header :localpart :is "a" "b" { keep; }`,
		"match type without arguments":     `if header :is { keep; }`,
		"address without a header or key":  `if address :is "From" { keep; }`,
		"envelope without require":         `if envelope :is "from" "x" { keep; }`,
		"fileinto without require":         `fileinto "x";`,
		"quantifier on a non-number":       "if size :over K { keep; }",
		"number with an unknown suffix":    "if size :over 1X { keep; }",
		"string where a block is expected": `if true "x";`,
	}
	for name, script := range bad {
		if err := sieve.Validate(script); err == nil {
			t.Errorf("%s: %q was accepted", name, script)
		}
	}
	good := map[string]string{
		"comments only":            "# nothing\n/* here */",
		"empty script":             "",
		"empty block":              "if true { }",
		"empty-string folder":      `require "fileinto"; fileinto "";`,
		"default match type":       `if header "Subject" "x" { keep; }`,
		"elsif chain without else": "if true { keep; } elsif false { keep; }",
		"nested blocks":            "if true { if false { stop; } elsif true { keep; } else { discard; } }",
	}
	for name, script := range good {
		if err := sieve.Validate(script); err != nil {
			t.Errorf("%s: %q was rejected: %v", name, script, err)
		}
	}
}

// TestRFC5228_MultipleRequires checks that several require commands may open a
// script, and that Requires and Empty report on the parsed script.
//
// checklist: SIEVE-CMD-03
func TestRFC5228_MultipleRequires(t *testing.T) {
	s, err := sieve.Parse(`require "fileinto"; require ["envelope", "reject"]; keep;`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.Requires(), ","); got != "fileinto,envelope,reject" {
		t.Errorf("Requires() = %q", got)
	}
	if s.Empty() {
		t.Error("a script with a keep is not empty")
	}
	only, err := sieve.Parse("# nothing but\nrequire \"fileinto\";")
	if err != nil {
		t.Fatal(err)
	}
	if !only.Empty() {
		t.Error("a script of only a require is empty")
	}
	var nilScript *sieve.Script
	if !nilScript.Empty() {
		t.Error("a nil script is empty")
	}
	if err := sieve.Validate(`require "fileinto"; keep; require "envelope";`); err == nil {
		t.Error("a require after another command must be rejected")
	}
}

// TestRFC5228_MatchTypeIs checks the :is match type, which is the default (§2.7.1).
//
// checklist: SIEVE-MT-01
func TestRFC5228_MatchTypeIs(t *testing.T) {
	msg := sieveEmail()
	for test, want := range map[string]bool{
		`header :is "Subject" "Test message"`:         true,
		`header "Subject" "Test message"`:             true,
		`header :is "Subject" "Test"`:                 false,
		`header :is "Subject" "test MESSAGE"`:         true, // i;ascii-casemap by default
		`header :is "Subject" ["no", "Test message"]`: true,
		`header :is "Subject" ""`:                     false,
	} {
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
	empty := sieveEmail()
	empty.Headers.Raw = map[string][]string{"X-Empty": {""}}
	if !matchedFolder(t, `header :is "X-Empty" ""`, empty) {
		t.Error(`an empty header value must be :is ""`)
	}
}

// TestRFC5228_MatchTypeContains checks the :contains match type (§2.7.1).
//
// checklist: SIEVE-MT-02
func TestRFC5228_MatchTypeContains(t *testing.T) {
	msg := sieveEmail()
	for test, want := range map[string]bool{
		`header :contains "Subject" "st mes"`:                       true,
		`header :contains "Subject" "ST MES"`:                       true,
		`header :contains "Subject" ""`:                             true, // the empty string is in every string
		`header :contains "Subject" "xyz"`:                          false,
		`header :contains :comparator "i;octet" "Subject" "ST MES"`: false,
		`header :contains :comparator "i;octet" "Subject" "st mes"`: true,
	} {
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
}

// TestRFC5228_MatchesWildcards checks the :matches pattern language of §2.7.1:
// `*` for any run of characters, `?` for exactly one, `\` to escape either.
//
// checklist: SIEVE-MT-03
func TestRFC5228_MatchesWildcards(t *testing.T) {
	msg := sieveEmail()
	msg.Headers.Subject = "a*b?c"
	for pattern, want := range map[string]bool{
		`a\\*b\\?c`: true,
		`a\\*b\\?`:  false,
		`a*`:        true,
		`*c`:        true,
		`a?b?c`:     true,
		`a??b?c`:    false,
		`*`:         true,
		`**`:        true,
		`*b*`:       true,
		`*\\?*`:     true,
		`*\\*\\?*`:  false,
		`A*C`:       true,
		`a*b?c\\`:   false,
	} {
		test := `header :matches "Subject" "` + pattern + `"`
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
	empty := sieveEmail()
	empty.Headers.Raw = map[string][]string{"X-Empty": {""}}
	if !matchedFolder(t, `header :matches "X-Empty" "*"`, empty) {
		t.Error(`* must match the empty string`)
	}
	if matchedFolder(t, `header :matches "X-Empty" "?"`, empty) {
		t.Error(`? must not match the empty string`)
	}
}

// TestRFC5228_AbsentHeaderNeverMatches checks that a header the message does
// not have gives a key nothing to match, even the empty string (§5.7).
//
// checklist: SIEVE-TST-06 SIEVE-TST-08
func TestRFC5228_AbsentHeaderNeverMatches(t *testing.T) {
	msg := sieveEmail()
	for _, test := range []string{
		`header :is "X-Absent" ""`,
		`header :contains "X-Absent" ""`,
		`header :matches "X-Absent" "*"`,
		`exists "X-Absent"`,
	} {
		if matchedFolder(t, test, msg) {
			t.Errorf("%s: matched a header the message lacks", test)
		}
	}
	if !matchedFolder(t, `not exists "X-Absent"`, msg) {
		t.Error("`not exists` of an absent header must be true")
	}
}

// TestRFC5228_HeaderNamesCaseInsensitive checks that header field names match
// without regard to case (§5.7), and that a name list is tried name by name.
//
// checklist: SIEVE-TST-06 SIEVE-TST-08
func TestRFC5228_HeaderNamesCaseInsensitive(t *testing.T) {
	msg := sieveEmail()
	msg.Headers.Raw = map[string][]string{"X-Custom": {"Value"}}
	for test, want := range map[string]bool{
		`header :is "subject" "Test message"`:         true,
		`header :is "SUBJECT" "Test message"`:         true,
		`header :is "x-custom" "value"`:               true,
		`header :is ["X-Nope", "x-CUSTOM"] "value"`:   true,
		`header :is ["X-Nope", "X-Also-Not"] "value"`: false,
		`exists ["Subject", "x-custom"]`:              true,
		`exists ["Subject", "x-nope"]`:                false,
	} {
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
}

// TestRFC5228_SizeTest checks the size test (§5.9): :over is strictly greater
// and :under strictly less, so a message of exactly the limit is neither.
//
// checklist: SIEVE-LEX-08 SIEVE-TST-10
func TestRFC5228_SizeTest(t *testing.T) {
	msg := sieveEmail()
	msg.RawSize = 1000
	for test, want := range map[string]bool{
		`size :over 999`:   true,
		`size :over 1000`:  false,
		`size :under 1000`: false,
		`size :under 1001`: true,
		`size :over 0`:     true,
		`size :under 1K`:   true,
		`size :over 1K`:    false,
		`size :over 1k`:    false,
	} {
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
	msg.RawSize = 2 * 1024 * 1024
	if !matchedFolder(t, `size :over 1M`, msg) || matchedFolder(t, `size :over 2M`, msg) || !matchedFolder(t, `size :under 1G`, msg) {
		t.Error("the M and G quantifiers are powers of 1024")
	}
}

// TestRFC5228_EnvelopePartsCaseInsensitive checks the two supported envelope
// parts, including a list and a case-sensitive value comparator (§5.4).
//
// checklist: SIEVE-TST-05
func TestRFC5228_EnvelopePartsCaseInsensitive(t *testing.T) {
	msg := sieveEmail()
	for _, expr := range []string{
		`envelope :is "FROM" "sender@example.com"`,
		`envelope :is "To" "recipient@example.com"`,
		`envelope :comparator "i;octet" :is ["FrOm", "tO"] ["absent@example.com", "recipient@example.com"]`,
	} {
		if !matchedFolder(t, expr, msg) {
			t.Errorf("envelope part names are case-insensitive: %s", expr)
		}
	}
}

// checklist: SIEVE-CORE-24
func TestRFC5228_EnvelopeUnknownPartsRejected(t *testing.T) {
	for _, parts := range []string{
		`"x-nonesuch"`, `""`, `"sender"`, `" from"`, `"to "`,
		`["from", "x-nonesuch"]`, `["x-nonesuch", "to"]`,
		`"${hex:78}"`, `["from", "${unicode:78}"]`,
	} {
		for _, condition := range []string{
			`envelope :is ` + parts + ` ""`,
			`allof (false, envelope :is ` + parts + ` "")`,
			`anyof (true, envelope :is ` + parts + ` "")`,
		} {
			script := `require ["envelope", "encoded-character"]; if ` + condition + ` { keep; }`
			parsed, err := sieve.Parse(script)
			if err == nil || parsed != nil {
				t.Errorf("Parse accepted unknown envelope part: %s", script)
			} else if !strings.Contains(err.Error(), "unknown envelope part") {
				t.Errorf("unexpected error for %s: %v", parts, err)
			}
			if err := sieve.Validate(script); err == nil {
				t.Errorf("Validate accepted unknown envelope part: %s", script)
			}
		}
	}
}

// TestRFC5228_StopInNestedBlock checks that stop ends the script from inside
// nested blocks, and that commands after it do not run (§3.3).
//
// checklist: SIEVE-CMD-04
func TestRFC5228_StopInNestedBlock(t *testing.T) {
	r := runSieve(t, `require "fileinto";
if true { if true { fileinto "first"; stop; } }
fileinto "second";`, sieveEmail())
	if folderOf(r) != "first" {
		t.Errorf("stop in a nested block must end the script, folder = %q", folderOf(r))
	}
}

// TestRFC5228_AllofAnyofNested checks the lists of tests of §5.2 and
// §5.3: allof is true for all, anyof for any, and a test list may be nested.
//
// checklist: SIEVE-TST-03 SIEVE-TST-04 SIEVE-TST-09
func TestRFC5228_AllofAnyofNested(t *testing.T) {
	msg := sieveEmail()
	for test, want := range map[string]bool{
		`allof(true, true, true)`:                      true,
		`allof(true, false, true)`:                     false,
		`anyof(false, false, true)`:                    true,
		`anyof(false, false)`:                          false,
		`allof(anyof(false, true), not false)`:         true,
		`allof(anyof(false, false), true)`:             false,
		`not allof(true, false)`:                       true,
		`not not true`:                                 true,
		`anyof(allof(true, false), allof(true, true))`: true,
	} {
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
}
