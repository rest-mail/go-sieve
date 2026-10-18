package sieve

import (
	"errors"
	"testing"
)

// fullMessage is a message with every structured header set.
func fullMessage() *Message {
	m := sieveEmail()
	m.Headers.From = []Address{{Name: "Ann", Address: "ann@example.com"}, {Address: "bob@example.org"}, {Name: "Only Name"}}
	m.Headers.To = []Address{{Address: "to@example.com"}}
	m.Headers.Cc = []Address{{Address: "cc@example.com"}}
	m.Headers.Bcc = []Address{{Address: "bcc@example.com"}}
	m.Headers.InReplyTo = "<parent@example.com>"
	m.Headers.References = []string{"<r1@example.com>", "<r2@example.com>"}
	m.Headers.Raw = map[string][]string{"Reply-To": {"Zed <zed@example.net>", "not an address"}}
	return m
}

// TestHeaderTest_StructuredHeaders checks that the header and address tests see
// every header the Message holds in structured form, under any case of name.
//
// checklist: SIEVE-TST-01 SIEVE-TST-08
func TestHeaderTest_StructuredHeaders(t *testing.T) {
	msg := fullMessage()
	for test, want := range map[string]bool{
		`header :is "From" "Ann <ann@example.com>"`:                         true,
		`header :is "From" "bob@example.org"`:                               true,
		`header :is "From" "Only Name"`:                                     true,
		`header :is "To" "to@example.com"`:                                  true,
		`header :is "Cc" "cc@example.com"`:                                  true,
		`header :is "Bcc" "bcc@example.com"`:                                true,
		`header :is "Message-ID" "<abc123@example.com>"`:                    true,
		`header :is "In-Reply-To" "<parent@example.com>"`:                   true,
		`header :is "References" "<r2@example.com>"`:                        true,
		`header :contains "Date" "2026"`:                                    true,
		`header :contains "Reply-To" "zed"`:                                 true,
		`address :is :domain "To" "example.com"`:                            true,
		`address :is :domain "Bcc" "example.com"`:                           true,
		`address :is :domain "Cc" "example.com"`:                            true,
		`address :is :localpart "Reply-To" "zed"`:                           true,
		`address :is :localpart "reply-to" "not an address"`:                false,
		`address :is :all "From" ["nobody@example.com", "bob@example.org"]`: true,
		`address :is :all ["To", "Cc"] "cc@example.com"`:                    true,
	} {
		if got := matchedFolder(t, test, msg); got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
}

// TestMatchOne_ComparatorsAndRegex checks the :matches and :regex match types
// under the two comparators, and that a broken regular expression never matches.
//
// checklist: SIEVE-EXT-06 SIEVE-MT-03 SIEVE-MT-05
func TestMatchOne_ComparatorsAndRegex(t *testing.T) {
	msg := sieveEmail()
	for test, want := range map[string]bool{
		`header :matches :comparator "i;octet" "Subject" "Test*"`:         true,
		`header :matches :comparator "i;octet" "Subject" "test*"`:         false,
		`header :matches :comparator "i;ascii-casemap" "Subject" "test*"`: true,
		`header :regex "Subject" "^test"`:                                 true,
		`header :regex :comparator "i;octet" "Subject" "^test"`:           false,
		`header :regex :comparator "i;octet" "Subject" "^Test"`:           true,
		`header :regex "Subject" "("`:                                     false,
	} {
		s := `require ["regex", "fileinto"]; if ` + test + ` { fileinto "hit"; }`
		if got := folderOf(runSieve(t, s, msg)) == "hit"; got != want {
			t.Errorf("%s: matched = %v, want %v", test, got, want)
		}
	}
}

// TestEvaluate_PanicWithErrorValue checks that a host callback that panics with
// an error is reported as that error, and the message is kept.
//
// checklist: SIEVE-ACT-07
func TestEvaluate_PanicWithErrorValue(t *testing.T) {
	boom := errors.New("boom")
	s, err := Parse(`require "fileinto"; fileinto "x";`)
	if err != nil {
		t.Fatal(err)
	}
	out := s.Evaluate(sieveEmail(), errorPanicExec{newRecExec(), boom})
	if !errors.Is(out.Error, boom) || !out.ImplicitKeep {
		t.Errorf("Evaluate = %+v, want the panic's error and an implicit keep", out)
	}
}

type errorPanicExec struct {
	*recExec
	err error
}

func (e errorPanicExec) FileInto(string, bool) { panic(e.err) }

// TestVacation_ReplyTo checks which address a vacation reply goes to: the
// envelope sender, else the first From address, and none when there is neither.
//
// checklist: SIEVE-EXT-01
func TestVacation_ReplyTo(t *testing.T) {
	script := `require "vacation"; vacation "away";`
	msg := sieveEmail()
	msg.Envelope.From = ""
	if got := runSieve(t, script, msg).meta["vacation_reply_to"]; got != "sender@example.com" {
		t.Errorf("reply-to = %q, want the From header's address", got)
	}
	msg.Headers.From = nil
	if _, ok := runSieve(t, script, msg).meta["vacation_reply_to"]; ok {
		t.Error("a message with no sender must not get a vacation reply")
	}
}

// TestExtensionArguments_Parse checks that the tagged arguments of the
// extension commands are parsed, and that a missing value is an error.
//
// checklist: SIEVE-EXT-01 SIEVE-EXT-02 SIEVE-EXT-04 SIEVE-EXT-05 SIEVE-EXT-07
func TestExtensionArguments_Parse(t *testing.T) {
	good := []string{
		`require "vacation"; vacation :days 2 :subject "s" :from "a@b" :handle "h" :addresses ["a@b", "c@d"] :mime "body";`,
		`require "vacation"; vacation :seconds 30 "body";`,
		`require "vacation"; vacation;`,
		`require "notify"; notify :method "mailto:a@b" :message "m" :options ["x"] :from "a@b" :importance "1" :id "i";`,
		`require "notify"; notify "mailto:a@b";`,
		`require ["fileinto", "imap4flags", "copy", "mailbox"]; fileinto :create :copy :flags ["\\Seen"] "x";`,
		`require "imap4flags"; keep :flags ["\\Seen", "\\Flagged"];`,
		`require "imap4flags"; setflag "\\Seen";`,
		`require "reject"; reject "no";`,
		`require "body"; if body :text :contains "x" { keep; }`,
		`require "body"; if body :contains "x" { keep; }`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`require "vacation"; vacation :days;`,
		`require "vacation"; vacation :seconds;`,
		`require "vacation"; vacation :subject;`,
		`require "vacation"; vacation :from;`,
		`require "vacation"; vacation :addresses;`,
		`require "vacation"; vacation :days 2`,
		`require "notify"; notify :method;`,
		`require "notify"; notify :message;`,
		`require "notify"; notify :options;`,
		`require "notify"; notify :from;`,
		`require "notify"; notify "x" "y";`,
		`require "fileinto"; fileinto :create "x";`,
		`require "fileinto"; fileinto :copy "x";`,
		`require "fileinto"; fileinto :flags ["a"] "x";`,
		`require ["fileinto", "imap4flags"]; fileinto :flags "x";`,
		`require "fileinto"; fileinto :bogus;`,
		`redirect :copy "a@b";`,
		`require "reject"; reject "a" "b";`,
		`require "body"; if body { keep; }`,
		`if envelope { keep; }`,
		`require "envelope"; if envelope :is "from" { keep; }`,
		`require "envelope"; if envelope { keep; }`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}
}

// TestEncodedCharacter_SequenceErrors checks the malformed sequences of the
// encoded-character extension that stay literal and the ones that are errors.
//
// checklist: SIEVE-LEX-06
func TestEncodedCharacter_SequenceErrors(t *testing.T) {
	for in, want := range map[string]string{
		"${hex:zz}":              "${hex:zz}",
		"${hex:123}":             "${hex:123}",
		"${hex:}":                "${hex:}",
		"${unicode:}":            "${unicode:}",
		"${unicode:zz}":          "${unicode:zz}",
		"${hex:41":               "${hex:41",
		"${hex:41 4}":            "A\x04",
		"${HEX:41}${UNICODE:42}": "AB",
		"${other:41}":            "${other:41}",
		"${":                     "${",
		"$${hex:41}":             "$A",
	} {
		got, err := decodeEncodedCharacters(in)
		if err != nil || got != want {
			t.Errorf("decode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"${unicode:D800}", "${unicode:DFFF}", "${unicode:110000}", "${unicode:41 110000}", "${unicode:1234567}"} {
		if _, err := decodeEncodedCharacters(in); err == nil {
			t.Errorf("decode(%q) succeeded, want an error", in)
		}
	}
}

// TestLexer_Errors checks that strings and tokens the lexer cannot read are
// reported with the line they are on.
//
// checklist: SIEVE-LEX-01 SIEVE-LEX-03 SIEVE-LEX-05 SIEVE-LEX-08
func TestLexer_Errors(t *testing.T) {
	for _, s := range []string{
		"keep; @",
		"keep; \"abc\\",
		"if header :is \"a\" text: \nabc\n",
		"if header :is \"a\" text:abc\n.\n { keep; }",
		"if size :over 18446744073709551616 { keep; }",
		"if size :over 99999999999999999999G { keep; }",
		"if size :over 17179869184G { keep; }",
		":",
		"keep; /* ",
		"\x00",
	} {
		if err := Validate(s); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}
}
