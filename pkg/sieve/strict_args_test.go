package sieve

import "testing"

// TestKeep_RefusesArguments checks that keep takes no argument of its own (RFC
// 5228 §4.3, §8.2): only the :flags tagged argument of RFC 5232 §5 is accepted,
// and only once "imap4flags" is declared.
//
// checklist: SIEVE-ACT-10 COPY-02
func TestKeep_RefusesArguments(t *testing.T) {
	good := []string{
		`keep;`,
		`require "imap4flags"; keep :flags "\\Seen";`,
		`require "imap4flags"; keep :flags ["\\Seen", "\\Flagged"];`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`keep "x";`,
		`keep ["x", "y"];`,
		`keep 1;`,
		`keep :copy;`,
		`require "copy"; keep :copy;`,
		`keep :flags "\\Seen";`,
		`require "imap4flags"; keep :flags;`,
		`require "imap4flags"; keep :flags "\\Seen" "x";`,
		`require "imap4flags"; keep :flags "\\Seen" :bogus;`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestReject_RequiresReason checks that reject and ereject take exactly one
// reason string (RFC 5429 §2.1, §2.2: reject <reason: string>).
//
// checklist: REJECT-01 EREJECT-01
func TestReject_RequiresReason(t *testing.T) {
	good := []string{
		`require "reject"; reject "no";`,
		`require "ereject"; ereject "no";`,
		"require \"reject\"; reject text:\nline one\r\nline two\r\n.\r\n;",
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`require "reject"; reject;`,
		`require "ereject"; ereject;`,
		`require "reject"; reject 1;`,
		`require "reject"; reject ["a"];`,
		`require "reject"; reject :reason "a";`,
		`require "ereject"; ereject "a" "b";`,
		`reject "no";`,
		`ereject "no";`,
		`require "reject"; ereject "no";`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestReject_RefusesAndCancelsImplicitKeep checks that reject and ereject
// refuse the message with the reason, unchanged and UTF-8, and cancel the
// implicit keep (RFC 5429 §2.1, §2.2, §2.4).
//
// checklist: REJECT-02 EREJECT-02 REJECT-08
func TestReject_RefusesAndCancelsImplicitKeep(t *testing.T) {
	for _, cmd := range []string{"reject", "ereject"} {
		script := `require "` + cmd + `"; ` + cmd + ` "Nicht zugestellt: Grüße";`
		r := runSieve(t, script, sieveEmail())
		if r.disposition != Reject {
			t.Errorf("%s: disposition %v, want Reject", cmd, r.disposition)
		}
		if r.reject != "Nicht zugestellt: Grüße" {
			t.Errorf("%s: reason %q, want the reason unchanged", cmd, r.reject)
		}
		if r.implicitKeep {
			t.Errorf("%s: the implicit keep is still in effect", cmd)
		}
	}
}

// TestReject_OnlyOneRuns checks that a second reject never executes: the first
// one halts the script (RFC 5429 §2.4).
//
// checklist: REJECT-03
func TestReject_OnlyOneRuns(t *testing.T) {
	script := `require ["reject", "ereject"]; reject "first"; ereject "second"; reject "third";`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Reject || r.reject != "first" {
		t.Errorf("got %v %q, want Reject \"first\"", r.disposition, r.reject)
	}
}
