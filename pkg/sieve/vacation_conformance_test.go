package sieve

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// checklist: VACATION-01 VACATION-02 VACATION-03 VACATION-04 VACATION-05 VACATION-06 VACATION-07 VACATION-08 VACATION-09
func TestVacation_Syntax(t *testing.T) {
	good := []string{
		`vacation "";`,
		`vacation :days 1 :subject "away" :from "Away <me@example.com>, other@example.com" :addresses ["me@example.com", "alias@example.com"] :mime :handle "" "Content-Type: text/plain\r\n\r\nAway";`,
		`vacation :addresses "me@example.com" "away";`,
		`VaCaTiOn :SUBJECT "away" :DAYS 2 "reason";`,
	}
	for _, s := range good {
		if err := Validate(`require "vacation"; ` + s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	bad := []string{
		`vacation;`, `vacation :days 2;`, `vacation "one" "two";`,
		`vacation :days "2" "away";`, `vacation :days -1 "away";`,
		`vacation :subject 1 "away";`, `vacation :from 1 "away";`,
		`vacation :handle 1 "away";`, `vacation :handle;`,
		`vacation :addresses [] "away";`, `vacation :addresses [1] "away";`,
		`vacation :bogus "away";`, `vacation :mime "one" "two";`,
		`vacation :from "not an address" "away";`,
	}
	for _, tag := range []string{`:days 2`, `:subject "s"`, `:from "me@example.com"`, `:addresses "me@example.com"`, `:mime`, `:handle "h"`} {
		bad = append(bad, "vacation "+tag+" "+tag+` "away";`)
	}
	for _, s := range bad {
		if err := Validate(`require "vacation"; ` + s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	if err := Validate(`vacation "away";`); err == nil {
		t.Fatal("vacation without require accepted")
	}
}

// checklist: VACATION-03 VACATION-10 VACATION-12 VACATION-13 VACATION-14 VACATION-15
func TestVacation_DaysBounds(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		days int
	}{
		{"", 7}, {":days 0", 1}, {":days 1", 1}, {":days 31", 31},
		{":days 365", 365}, {":days 366", 365}, {":days 4294967295", 365},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			got := runSieve(t, `require "vacation"; vacation `+tc.arg+` "away";`, sieveEmail())
			if got.meta["vacation_seconds"] != fmt.Sprint(tc.days*86400) {
				t.Fatalf("interval = %s seconds, want %d days", got.meta["vacation_seconds"], tc.days)
			}
		})
	}
}

// checklist: VACATION-06 VACATION-30 VACATION-31
func TestVacation_PersonalRecipient(t *testing.T) {
	for _, header := range []string{"To", "Cc", "Bcc", "Resent-To", "Resent-Cc", "Resent-Bcc"} {
		for _, alias := range []bool{false, true} {
			t.Run(fmt.Sprint(header, alias), func(t *testing.T) {
				msg := sieveEmail()
				msg.Headers.To = nil
				address := "recipient@example.com"
				args := ""
				if alias {
					address = "alias@example.com"
					args = `:addresses "alias@example.com"`
				}
				msg.Headers.Raw = map[string][]string{strings.ToLower(header): {"Someone <" + strings.ToUpper(address) + ">"}}
				got := runSieve(t, `require "vacation"; vacation `+args+` "away";`, msg)
				if got.meta["vacation_reply_to"] == "" {
					t.Fatal("personal message suppressed")
				}
			})
		}
	}
	for _, value := range []string{"", "other@example.com", "not an address", "A <other@example.com>,"} {
		msg := sieveEmail()
		msg.Headers.To = nil
		msg.Headers.Raw = map[string][]string{"To": {value}}
		got := runSieve(t, `require "vacation"; vacation "away";`, msg)
		if got.meta["vacation_reply_to"] != "" {
			t.Errorf("non-personal message %q received reply", value)
		}
	}
	// :addresses adds identities; it must not replace the envelope recipient.
	got := runSieve(t, `require "vacation"; vacation :addresses "alias@example.com" "away";`, sieveEmail())
	if got.meta["vacation_reply_to"] == "" {
		t.Fatal(":addresses replaced envelope recipient")
	}
}

// checklist: VACATION-32 VACATION-33 VACATION-34 VACATION-35 VACATION-36
func TestVacation_Suppression(t *testing.T) {
	headers := []string{"List-Id", "List-Help", "List-Subscribe", "List-Unsubscribe", "List-Post", "List-Owner", "List-Archive", "Auto-Submitted", "Precedence"}
	for _, h := range headers {
		values := []string{"yes"}
		if h == "Auto-Submitted" {
			values = []string{"auto-generated", "auto-replied", "AUTO-REPLIED; owner=x"}
		}
		if h == "Precedence" {
			values = []string{"bulk", "list", "junk", " BULK "}
		}
		for _, value := range values {
			msg := sieveEmail()
			msg.Headers.Raw = map[string][]string{strings.ToLower(h): {value}}
			got := runSieve(t, `require "vacation"; vacation "away";`, msg)
			if got.meta["vacation_reply_to"] != "" {
				t.Errorf("%s: %s received reply", h, value)
			}
		}
	}
	for _, sender := range []string{"MAILER-DAEMON@example.com", "LISTSERV@example.com", "majordomo@example.com", "list-request@example.com", "owner-list@example.com"} {
		msg := sieveEmail()
		msg.Envelope.From = sender
		if got := runSieve(t, `require "vacation"; vacation "away";`, msg); got.meta["vacation_reply_to"] != "" {
			t.Errorf("system sender %s received reply", sender)
		}
	}
	for _, raw := range []map[string][]string{
		{"Auto-Submitted": {"no"}}, {"Auto-Submitted": {" NO; owner=x "}}, {"Precedence": {"normal"}},
	} {
		msg := sieveEmail()
		msg.Headers.Raw = raw
		msg.Envelope.From = "postmaster@example.com"
		if got := runSieve(t, `require "vacation"; vacation "away";`, msg); got.meta["vacation_reply_to"] == "" {
			t.Errorf("ordinary message %v suppressed", raw)
		}
	}
}

// checklist: VACATION-37 VACATION-38 VACATION-39
func TestVacation_ActionInteraction(t *testing.T) {
	for _, action := range []string{"", "keep;", "discard;", `fileinto "folder";`, `redirect "other@example.com";`} {
		script, err := Parse(`require ["vacation", "fileinto"]; vacation "away"; ` + action)
		if err != nil {
			t.Fatal(err)
		}
		exec := newRecExec()
		out := script.Evaluate(sieveEmail(), exec)
		if out.Error != nil || exec.meta["vacation_reply_to"] == "" {
			t.Fatalf("%s: out=%+v actions=%v", action, out, exec.applied)
		}
		if action == "" && !out.ImplicitKeep {
			t.Fatal("vacation canceled implicit keep")
		}
	}
	for _, tail := range []string{`vacation "second";`, `if true { vacation "second"; }`} {
		script, err := Parse(`require "vacation"; vacation "first"; ` + tail)
		if err != nil {
			t.Fatal(err)
		}
		for _, suppress := range []bool{false, true} {
			msg := sieveEmail()
			msg.Envelope.FromNull = suppress
			exec := newRecExec()
			out := script.Evaluate(msg, exec)
			if out.Error == nil || !out.ImplicitKeep {
				t.Fatalf("second executed vacation must fail safe: %+v", out)
			}
			if strings.Count(strings.Join(exec.applied, ","), "vacation:") > 1 {
				t.Fatal("second vacation callback escaped")
			}
		}
	}
	script, err := Parse(`require "vacation"; if false { vacation "unused"; } vacation "used";`)
	if err != nil {
		t.Fatal(err)
	}
	if out := script.Evaluate(sieveEmail(), newRecExec()); out.Error != nil {
		t.Fatal(out.Error)
	}
}

// checklist: VACATION-16
func TestVacation_EnvelopeDestination(t *testing.T) {
	msg := sieveEmail()
	msg.Envelope.From = "envelope@example.com"
	msg.Headers.Raw = map[string][]string{"Reply-To": {"reply@example.com"}}
	if got := runSieve(t, `require "vacation"; vacation "away";`, msg).meta["vacation_reply_to"]; got != msg.Envelope.From {
		t.Fatal(got)
	}
	msg.Envelope.From = ""
	if got := runSieve(t, `require "vacation"; vacation "away";`, msg).meta["vacation_reply_to"]; got != "" {
		t.Fatalf("absent envelope sender fell back to From: %s", got)
	}
}

type vacationRecorder struct {
	*recExec
	replies []Vacation
}

func (e *vacationRecorder) Vacation(v Vacation) { e.replies = append(e.replies, v) }

// checklist: VACATION-04 VACATION-07 VACATION-08 VACATION-26 VACATION-29 VACATION-40
func TestVacation_CallbackArguments(t *testing.T) {
	for _, tc := range []struct {
		args, body string
		want       Vacation
	}{
		{``, "Estoy fuera — 再见", Vacation{Interval: 7 * 24 * time.Hour, Body: "Estoy fuera — 再见", ReplyTo: "sender@example.com"}},
		{`:subject "" :from "Me <me@example.com>" :handle ""`, "away", Vacation{Interval: 7 * 24 * time.Hour, SubjectSet: true, From: "Me <me@example.com>", HandleSet: true, Body: "away", ReplyTo: "sender@example.com"}},
		{`:subject "À bientôt" :handle "trip" :mime`, "Content-Type: text/plain; charset=utf-8\r\n\r\nÀ bientôt", Vacation{Interval: 7 * 24 * time.Hour, Subject: "À bientôt", SubjectSet: true, Handle: "trip", HandleSet: true, MIME: true, Body: "Content-Type: text/plain; charset=utf-8\r\n\r\nÀ bientôt", ReplyTo: "sender@example.com"}},
	} {
		script, err := Parse(`require "vacation"; vacation ` + tc.args + ` "` + tc.body + `";`)
		if err != nil {
			t.Fatal(err)
		}
		exec := &vacationRecorder{recExec: newRecExec()}
		for i := 0; i < 2; i++ {
			out := script.Evaluate(sieveEmail(), exec)
			if out.Error != nil {
				t.Fatal(out.Error)
			}
		}
		if len(exec.replies) != 2 {
			t.Fatalf("callbacks = %d, want 2 (tracking belongs to host)", len(exec.replies))
		}
		for _, reply := range exec.replies {
			if reply != tc.want {
				t.Errorf("got %+v, want %+v", reply, tc.want)
			}
		}
	}
}
