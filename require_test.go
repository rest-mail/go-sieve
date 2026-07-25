package sieve

import (
	"strings"
	"testing"
)

// These tests pin down RFC 5228 "require" semantics (issue #10). Before the fix
// the parser recorded require names but never enforced them: an unknown required
// extension was accepted, an extension used without being required was accepted,
// require could appear after other commands or inside a block, and unknown
// commands/tests were silently skipped. Each case below must now be a parse
// error.

// TestRequire_RejectsUnsupportedExtension covers RFC 5228 §2.10.5: a script that
// requires a capability this implementation does not provide MUST NOT run.
func TestRequire_RejectsUnsupportedExtension(t *testing.T) {
	bad := []string{
		`require "nonexistent-extension"; keep;`,
		`require ["fileinto", "boguscap"];
if true { fileinto "X"; }`,
		`require "editheader"; keep;`,
		`require "variables"; keep;`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("expected require of an unsupported extension to be rejected:\n%s", s)
		}
	}
}

// TestRequire_RejectsUndeclaredExtensionUse covers RFC 5228 §2.10.5: an
// extension command/test/comparator/tagged-argument that was not declared with
// require MUST be treated as unavailable, i.e. rejected.
func TestRequire_RejectsUndeclaredExtensionUse(t *testing.T) {
	bad := []struct {
		name   string
		script string
	}{
		{"fileinto action", `if true { fileinto "X"; }`},
		{"fileinto :create needs mailbox", `require "fileinto";
fileinto :create "X";`},
		{"envelope test", `if envelope :is "from" "a@b.com" { keep; }`},
		{"body test", `if body :contains "x" { keep; }`},
		{"imap4flags setflag", `setflag "\\Seen";`},
		{"imap4flags addflag", `if true { addflag "\\Flagged"; }`},
		{"vacation action", `vacation "away";`},
		{"notify action", `notify :method "mailto:a@b.com" :message "m";`},
		{"reject action", `reject "no";`},
		{":regex match type", `require "fileinto";
if header :regex "Subject" ".*" { fileinto "X"; }`},
		{"i;ascii-numeric comparator", `require "fileinto";
if header :is :comparator "i;ascii-numeric" "X-Priority" "1" { fileinto "X"; }`},
		{":copy tag on redirect", `redirect :copy "a@b.com";`},
		{"envelope required but body used", `require "envelope";
if body :contains "x" { keep; }`},
	}
	for _, tc := range bad {
		if err := Validate(tc.script); err == nil {
			t.Errorf("%s: expected use of an undeclared extension to be rejected:\n%s", tc.name, tc.script)
		}
	}
}

// TestRequire_RejectsMisplacedRequire covers RFC 5228 §3.2: it is an error for a
// require to appear after any other command, or anywhere other than the start of
// the script (in particular, inside a block).
func TestRequire_RejectsMisplacedRequire(t *testing.T) {
	bad := []struct {
		name   string
		script string
	}{
		{"after an action", `keep;
require "fileinto";`},
		{"after a control command", `if true { keep; }
require "fileinto";`},
		{"inside a block", `require "fileinto";
if true { require "envelope"; }`},
	}
	for _, tc := range bad {
		if err := Validate(tc.script); err == nil {
			t.Errorf("%s: expected a misplaced require to be rejected:\n%s", tc.name, tc.script)
		}
	}
}

// TestRequire_RejectsUnknownCommandsAndTests covers the silent-skip bug: a typo
// or a command/test from an undeclared extension must be a parse error, not a
// silent no-op.
func TestRequire_RejectsUnknownCommandsAndTests(t *testing.T) {
	bad := []struct {
		name   string
		script string
	}{
		{"typo command", `kep;`},
		{"unknown command", `frobnicate "x" 42;`},
		{"unknown test", `if spamtest :value "gt" "5" { keep; }`},
		{"unknown test in anyof", `if anyof (true, spamtest :value "gt" "5") { keep; }`},
	}
	for _, tc := range bad {
		if err := Validate(tc.script); err == nil {
			t.Errorf("%s: expected an unknown command/test to be rejected:\n%s", tc.name, tc.script)
		}
	}
}

// TestRequire_AcceptsProperlyDeclaredScripts guards against over-rejection: once
// an extension is declared with require, using it must still be accepted.
func TestRequire_AcceptsProperlyDeclaredScripts(t *testing.T) {
	good := []string{
		`require "fileinto";
if true { fileinto "X"; }`,
		`require ["fileinto", "mailbox"];
fileinto :create "X";`,
		`require ["envelope", "fileinto"];
if envelope :is "from" "a@b.com" { fileinto "X"; }`,
		`require "body";
if body :contains "x" { keep; }`,
		`require "imap4flags";
if true { setflag "\\Seen"; }`,
		`require "vacation";
vacation "away";`,
		`require "notify";
notify :method "mailto:a@b.com" :message "m";`,
		`require "reject";
reject "no";`,
		`require ["fileinto", "regex"];
if header :regex "Subject" ".*" { fileinto "X"; }`,
		`require ["fileinto", "comparator-i;ascii-numeric"];
if header :is :comparator "i;ascii-numeric" "X-Priority" "1" { fileinto "X"; }`,
		`require "copy";
redirect :copy "a@b.com";`,
		// require may list extensions it does not end up using.
		`require ["fileinto", "envelope"];
if true { keep; }
discard;`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("Validate rejected a properly-declared script: %v\n%s", err, s)
		}
	}
}

// TestRequire_ErrorMentionsCapability is a light check that the enforcement
// error is actionable (names the offending capability), not just any error.
func TestRequire_ErrorMentionsCapability(t *testing.T) {
	err := Validate(`if true { fileinto "X"; }`)
	if err == nil {
		t.Fatal("expected an error for undeclared fileinto")
	}
	if !strings.Contains(err.Error(), "fileinto") {
		t.Errorf("expected the error to mention fileinto, got: %v", err)
	}
}
