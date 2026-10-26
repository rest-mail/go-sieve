package sieve

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Pin the public capability surface so that a newly advertised extension
// cannot silently escape the requirement inventory.
// checklist: EXT-CAP-01
func TestExtensionCapabilityInventory(t *testing.T) {
	want := []string{
		"fileinto", "reject", "ereject", "envelope", "body", "imap4flags",
		"vacation", "vacation-seconds", "notify", "mailbox", "copy",
		"encoded-character", "regex", "comparator-i;ascii-numeric",
		"comparator-i;octet", "comparator-i;ascii-casemap",
	}
	if len(supportedCapabilities) != len(want) {
		t.Fatalf("capability count = %d, inventory has %d", len(supportedCapabilities), len(want))
	}
	for _, capability := range want {
		if err := Validate(fmt.Sprintf("require %q;", capability)); err != nil {
			t.Errorf("%s: %v", capability, err)
		}
	}
	for _, capability := range []string{"relational", "subaddress", "variables", "enotify"} {
		if err := Validate(fmt.Sprintf("require %q;", capability)); err == nil {
			t.Errorf("advertised unsupported %s", capability)
		}
	}
	// The numeric equality comparator does not imply relational match types;
	// ordinary address parts do not imply subaddress splitting.
	for _, script := range []string{
		`require "comparator-i;ascii-numeric"; if header :value "gt" "X-Priority" "3" { discard; }`,
		`if header :count "gt" "Received" "3" { discard; }`,
		`if address :user "to" "ken" { discard; }`,
		`require "envelope"; if envelope :detail "to" "list" { discard; }`,
		`set "name" "value";`,
		`if string "a" "a" { discard; }`,
	} {
		if err := Validate(script); err == nil {
			t.Errorf("accepted unsupported syntax: %s", script)
		}
	}
}

// checklist: SECONDS-01 SECONDS-02
func TestVacationSeconds_Capability(t *testing.T) {
	for _, declaration := range []string{
		`require "vacation-seconds";`,
		`require ["vacation", "vacation-seconds"];`,
		`require "vacation-seconds"; require "vacation";`,
	} {
		for _, action := range []string{`vacation "away";`, `vacation :days 2 "away";`, `vacation :seconds 30 "away";`} {
			if err := Validate(declaration + action); err != nil {
				t.Errorf("%s%s: %v", declaration, action, err)
			}
		}
	}
	s, err := Parse(`require "vacation-seconds"; vacation "away";`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Requires(), []string{"vacation-seconds"}) {
		t.Fatalf("Requires = %v, want only the explicit declaration", s.Requires())
	}
	for _, script := range []string{
		`vacation :seconds 30 "away";`,
		`require "vacation"; vacation :seconds 30 "away";`,
	} {
		if err := Validate(script); err == nil {
			t.Errorf("accepted undeclared seconds: %s", script)
		}
	}
}

// checklist: SECONDS-03
func TestVacationSeconds_ExclusiveTags(t *testing.T) {
	for _, args := range []string{
		`:days 1 :seconds 30`, `:seconds 30 :days 1`,
		`:seconds 0 :seconds 1`, `:seconds "30"`, `:seconds -1`, `:seconds`,
	} {
		if err := Validate(`require "vacation-seconds"; vacation ` + args + ` "away";`); err == nil {
			t.Errorf("accepted invalid arguments: %s", args)
		}
	}
}

// checklist: SECONDS-04 SECONDS-05 SECONDS-06 SECONDS-07
func TestVacationSeconds_Intervals(t *testing.T) {
	for _, tc := range []struct {
		args string
		want time.Duration
	}{
		{"", 7 * 24 * time.Hour},
		{":days 2", 48 * time.Hour},
		{":seconds 0", 0},
		{":seconds 1", time.Second},
		{":seconds 1800", 1800 * time.Second},
		{":seconds 86400", 24 * time.Hour},
		{":seconds 2147483647", 2147483647 * time.Second},
		{":seconds 2147483648", 2147483647 * time.Second},
		{":seconds 9223372036854775807", 2147483647 * time.Second},
	} {
		t.Run(tc.args, func(t *testing.T) {
			s, err := Parse(`require "vacation-seconds"; vacation ` + tc.args + ` "away";`)
			if err != nil {
				t.Fatal(err)
			}
			exec := &vacationRecorder{recExec: newRecExec()}
			out := s.Evaluate(sieveEmail(), exec)
			if out.Error != nil || !out.ImplicitKeep || len(exec.replies) != 1 {
				t.Fatalf("outcome = %+v, replies = %+v", out, exec.replies)
			}
			if got := exec.replies[0].Interval; got != tc.want {
				t.Errorf("interval = %v, want %v", got, tc.want)
			}
		})
	}
}

// RFC 6131 section 3's meeting example, including the additional address.
// checklist: SECONDS-08
func TestVacationSeconds_RFCExample(t *testing.T) {
	s, err := Parse(`require ["vacation-seconds"];
vacation :addresses ["tjs@example.edu", "ts4z@landru.example.edu"]
         :seconds 1800
         "I am in a meeting, and do not have access to email.";`)
	if err != nil {
		t.Fatal(err)
	}
	msg := sieveEmail()
	msg.Headers.To = []Address{{Address: "ts4z@landru.example.edu"}}
	exec := &vacationRecorder{recExec: newRecExec()}
	out := s.Evaluate(msg, exec)
	if out.Error != nil || !out.ImplicitKeep || len(exec.replies) != 1 {
		t.Fatalf("outcome = %+v, replies = %+v", out, exec.replies)
	}
	if got := exec.replies[0]; got.Interval != 30*time.Minute || got.ReplyTo != msg.Envelope.From || got.Body != "I am in a meeting, and do not have access to email." {
		t.Fatalf("reply = %+v", got)
	}
}

// checklist: REGEX-01 REGEX-02 REGEX-03
func TestRegex_ExtensionContract(t *testing.T) {
	for _, tc := range []struct {
		pattern, comparator, subject string
		match                        bool
	}{
		{"voice", "i;octet", "invoice", true}, // searches, rather than full-string matches
		{"^voice$", "i;octet", "invoice", false},
		{"^INVOICE$", "i;octet", "invoice", false},
		{"^INVOICE$", "i;ascii-casemap", "invoice", true},
		{"^INVOICE$", "", "invoice", true}, // default comparator
		{"^k$", "", "K", true},             // Go's Unicode case folding
		{"^k$", "i;octet", "K", false},
		{"[", "i;octet", "[", false},       // invalid patterns do not abort evaluation
		{"(a)\\1", "i;octet", "aa", false}, // Go RE2 does not implement backreferences
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			// A quoted Sieve string requires doubled backslashes.
			pattern := strings.ReplaceAll(tc.pattern, `\`, `\\`)
			comparator := ""
			if tc.comparator != "" {
				comparator = fmt.Sprintf(":comparator %q", tc.comparator)
			}
			s, err := Parse(fmt.Sprintf(`require ["regex", "fileinto"]; if header :regex %s "Subject" "%s" { fileinto "matched"; } keep;`, comparator, pattern))
			if err != nil {
				t.Fatal(err)
			}
			exec := newRecExec()
			out := s.Evaluate(&Message{Headers: Headers{Subject: tc.subject}}, exec)
			wantActions := []string{"keep"}
			if tc.match {
				wantActions = []string{"fileinto:matched", "keep"}
			}
			if out.Error != nil || out.ImplicitKeep || !reflect.DeepEqual(exec.applied, wantActions) {
				t.Fatalf("outcome = %+v, actions = %v", out, exec.applied)
			}
			if got := exec.meta["deliver_to_folder"] == "matched"; got != tc.match {
				t.Errorf("matched = %v, want %v", got, tc.match)
			}
		})
	}
	if err := Validate(`if header :regex "Subject" "x" { discard; }`); err == nil {
		t.Error("accepted undeclared regex")
	}
}
