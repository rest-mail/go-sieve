package sieve

import "testing"

// TestParse_NestedErrorsPropagate drives a syntax error into each place where a
// parse function hands on a failure from a helper (a nested test, block,
// string list or expected token). Each script is refused, never half-parsed
// into a script that would run.
//
// checklist: SIEVE-LEX-11
func TestParse_NestedErrorsPropagate(t *testing.T) {
	bad := map[string]string{
		// parseIf
		"elsif with a bad test":      `if true { keep; } elsif header { keep; }`,
		"elsif without a block":      `if true { keep; } elsif true keep;`,
		"else without a block":       `if true { keep; } else keep;`,
		"bad command inside a block": `if true { kep; }`,
		// parseTestList
		"allof without a paren":     `if allof true { keep; }`,
		"anyof with a bad test":     `if anyof(true, header) { keep; }`,
		"anyof without close paren": `if anyof(true, false { keep; }`,
		// parseSizeTest
		"size with a string limit": `if size :over "1K" { keep; }`,
		// parseFileinto
		"fileinto :flags without a list": `require ["fileinto", "imap4flags"]; fileinto :flags;`,
		"fileinto with a number folder":  `require "fileinto"; fileinto 1;`,
		// parseNotify
		"notify :method without a string":  `require "notify"; notify :method 1;`,
		"notify :message without a string": `require "notify"; notify :message 1;`,
		"notify :options without a list":   `require "notify"; notify :options 1;`,
		"notify :from without a string":    `require "notify"; notify :from 1;`,
		"notify with a bad encoded method": `require ["notify", "encoded-character"]; notify "${unicode:110000}";`,
		// parseBodyTest
		"body with a bad match option": `require "body"; if body :comparator 1 "x" { keep; }`,
		"body without keys":            `require "body"; if body :contains { keep; }`,
		// parseMatchOptions: a second match type after any other one, :regex
		// included, is refused whichever comes first.
		":regex after :is":        `require "regex"; if header :is :regex "a" "b" { keep; }`,
		":is after :regex":        `require "regex"; if header :regex :is "a" "b" { keep; }`,
		":regex after :matches":   `require "regex"; if header :matches :regex "a" "b" { keep; }`,
		"comparator without name": `if header :comparator { keep; }`,
		// parseRequire
		"require inside a block":    `require "fileinto"; if true { require "body"; }`,
		"require with a bad list":   `require ["fileinto",];`,
		"require without semicolon": `require "fileinto" keep;`,
	}
	for name, src := range bad {
		t.Run(name, func(t *testing.T) {
			if s, err := Parse(src); err == nil {
				t.Fatalf("Parse(%q) accepted the script: %+v", src, s)
			}
		})
	}
}
