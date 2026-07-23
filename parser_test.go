package sieve

import "testing"

func TestParseSieveScript_RequiresAndCommands(t *testing.T) {
	script := `require ["fileinto", "envelope"];
if true { keep; }
discard;`
	s, err := parseSieveScript(script)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(s.requires) != 2 {
		t.Errorf("expected 2 requires, got %d (%v)", len(s.requires), s.requires)
	}
	if len(s.commands) != 2 {
		t.Errorf("expected 2 top-level commands, got %d", len(s.commands))
	}
}

func TestValidateSieve_Errors(t *testing.T) {
	bad := []string{
		`if true { keep;`,                      // unterminated block
		`if header :is "Subject" "x { keep; }`, // unterminated string
		`keep keep;`,                           // missing semicolon
		`if size 100 { keep; }`,                // size without :over/:under
		`else { keep; }`,                       // else without if
		`if header :is "Subject" { keep; }`,    // header missing key list
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("expected Validate to reject:\n%s", s)
		}
	}
}

func TestValidateSieve_AcceptsNewConstructs(t *testing.T) {
	good := []string{
		`if allof (exists "Subject", size :over 1K) { discard; }`,
		`if anyof (true, false) { keep; }`,
		`require "imap4flags";
if header :matches "Subject" "*urgent*" { addflag "\\Flagged"; }`,
		`if header :is "Subject" "x" { fileinto "A"; } elsif true { fileinto "B"; } else { keep; }`,
		`if address :localpart :contains "From" "admin" { redirect "ops@example.com"; }`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("Validate rejected a valid script: %v\n%s", err, s)
		}
	}
}

func TestValidateSieve_Valid(t *testing.T) {
	scripts := []string{
		`require "fileinto";
if header :contains "Subject" "test" {
  fileinto "Test";
}`,
		`require "vacation";
vacation :days 7 :subject "OOO" "Away.";`,
		`require "notify";
notify :method "mailto:a@b.com" :message "msg";`,
		`require ["body", "regex"];
if body :regex "test.*" {
  keep;
}`,
		`require "envelope";
if envelope :is "from" "a@b.com" {
  discard;
}`,
	}
	for _, s := range scripts {
		if err := Validate(s); err != nil {
			t.Errorf("Validate should accept valid script, got error: %v\nScript: %s", err, s)
		}
	}
}
