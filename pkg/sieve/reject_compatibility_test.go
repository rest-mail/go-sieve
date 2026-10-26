package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// TestReject_VacationIncompatible rejects both action orders before the host
// can receive any callback, including combinations across nested blocks.
//
// checklist: REJECT-07
func TestReject_VacationIncompatible(t *testing.T) {
	for _, action := range []string{"reject", "ereject"} {
		for _, body := range []string{
			`vacation "away"; %s "no";`,
			`%s "no"; vacation "away";`,
			`if true { vacation "away"; %s "no"; }`,
			`if true { %s "no"; vacation "away"; }`,
			`vacation "away"; if header :is "Subject" "spam" { %s "no"; }`,
			`if header :is "Subject" "spam" { %s "no"; } vacation "away";`,
			`if header :is "Subject" "spam" { vacation "away"; } %s "no";`,
			`%s "no"; if true { vacation "away"; }`,
			`if exists "From" { vacation "away"; } if exists "Subject" { %s "no"; }`,
		} {
			src := `require ["reject", "ereject", "vacation"]; ` + fmt.Sprintf(body, action)
			t.Run(action+"/"+body, func(t *testing.T) {
				if err := Validate(src); err == nil || !strings.Contains(err.Error(), "incompatible") {
					t.Errorf("Validate = %v, want incompatible actions error", err)
				}
				if script, err := Parse(src); err == nil || script != nil {
					t.Errorf("Parse = %v, %v; want nil script and error before evaluation", script, err)
				}
			})
		}
	}
}

// TestReject_VacationExclusiveBranches permits the alternatives of one if chain:
// a script can reject spam and send a vacation reply to accepted messages.
//
// checklist: REJECT-07
func TestReject_VacationExclusiveBranches(t *testing.T) {
	for _, action := range []string{"reject", "ereject"} {
		for _, tc := range []struct {
			body, spam, hello string
		}{
			{`if header :is "Subject" "spam" { %s "no"; } else { vacation "away"; }`, "reject", "vacation:sender@example.com"},
			{`if header :is "Subject" "spam" { vacation "away"; } elsif true { %s "no"; }`, "vacation:sender@example.com", "reject"},
			{`if header :is "Subject" "spam" { if true { %s "no"; } } else { vacation "away"; }`, "reject", "vacation:sender@example.com"},
			{`if header :is "Subject" "spam" { %s "no"; } else { keep; }`, "reject", "keep"},
		} {
			src := `require ["reject", "ereject", "vacation"]; ` + fmt.Sprintf(tc.body, action)
			if err := Validate(src); err != nil {
				t.Fatalf("Validate(%s): %v", src, err)
			}
			script, err := Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			for subject, want := range map[string]string{"spam": tc.spam, "hello": tc.hello} {
				exec := newRecExec()
				// Vacation replies only to mail addressed to the user (RFC 5230 §4.5).
				out := script.Evaluate(&Message{
					Headers:  Headers{Subject: subject, To: []Address{{Address: "recipient@example.com"}}},
					Envelope: Envelope{From: "sender@example.com", To: []string{"recipient@example.com"}},
				}, exec)
				if out.Error != nil {
					t.Fatalf("Evaluate: %v", out.Error)
				}
				if want == "reject" {
					if out.Disposition != Reject || out.ExtendedReject != (action == "ereject") || len(exec.applied) != 0 {
						t.Errorf("%s, subject %s: outcome %+v, actions %v; want only %s", src, subject, out, exec.applied, action)
					}
				} else if out.Disposition != Continue || len(exec.applied) != 1 || exec.applied[0] != want {
					t.Errorf("%s, subject %s: outcome %+v, actions %v; want only %s", src, subject, out, exec.applied, want)
				}
			}
		}
	}
}

// TestReject_OutcomeDistinguishesExtendedRefusal preserves the actual command,
// not merely the required capability or the last rejection in source order.
//
// checklist: REJECT-05
func TestReject_OutcomeDistinguishesExtendedRefusal(t *testing.T) {
	for _, tc := range []struct {
		body     string
		extended bool
	}{
		{`reject "first"; ereject "second";`, false},
		{`ereject "first"; reject "second";`, true},
		{`if false { ereject "skipped"; } reject "first";`, false},
		{`if false { reject "skipped"; } ereject "first";`, true},
	} {
		script, err := Parse(`require ["reject", "ereject"]; ` + tc.body)
		if err != nil {
			t.Fatal(err)
		}
		out := script.Evaluate(&Message{}, newRecExec())
		if out.Disposition != Reject || out.RejectReason != "first" || out.ExtendedReject != tc.extended || out.Error != nil || out.ImplicitKeep {
			t.Errorf("%s: got %+v, want Reject first, ExtendedReject=%v", tc.body, out, tc.extended)
		}
	}
	for _, body := range []string{`keep;`, `discard;`, `if false { ereject "skipped"; }`} {
		script, err := Parse(`require "ereject"; ` + body)
		if err != nil {
			t.Fatal(err)
		}
		if out := script.Evaluate(&Message{}, newRecExec()); out.ExtendedReject {
			t.Errorf("%s: ExtendedReject set without ereject: %+v", body, out)
		}
	}
}
