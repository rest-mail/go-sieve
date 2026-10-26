package rfctest

import (
	"bufio"
	"context"
	"fmt"
	"net/textproto"
	"slices"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"

	foxsieve "github.com/foxcpp/go-sieve"
	"github.com/foxcpp/go-sieve/interp"
)

// Differential tests: the same scripts run on the same message through this
// library and through github.com/foxcpp/go-sieve, an independent RFC 5228
// implementation (MIT). Both must accept or refuse each script alike, and an
// accepted script must deliver the message the same way. The reference is a
// test-only dependency; the library itself imports only the standard library.

// diffEmail is the message both implementations evaluate. diffMessage gives
// this library the same message as structured fields.
const diffEmail = "From: Wile E. Coyote <coyote@desert.example.org>\r\n" +
	"To: Road Runner <roadrunner@acme.example.com>, beep@acme.example.com\r\n" +
	"Cc: Acme Sales <sales@ACME.example.com>\r\n" +
	"Subject: I have a present for you\r\n" +
	"Date: Tue, 1 Apr 1997 09:06:31 -0800\r\n" +
	"Message-ID: <anvil-1@desert.example.org>\r\n" +
	"X-Spam-Score: 12\r\n" +
	"X-Mailer: Acme Mailer 3000\r\n" +
	"\r\n" +
	"Look, I'm sorry about the whole anvil thing.\r\n"

const (
	diffEnvelopeFrom = "coyote@desert.example.org"
	diffEnvelopeTo   = "roadrunner+lists@acme.example.com"
)

func diffMessage() *sieve.Message {
	return &sieve.Message{
		Envelope: sieve.Envelope{From: diffEnvelopeFrom, To: []string{diffEnvelopeTo}},
		Headers: sieve.Headers{
			From:      []sieve.Address{{Name: "Wile E. Coyote", Address: "coyote@desert.example.org"}},
			To:        []sieve.Address{{Name: "Road Runner", Address: "roadrunner@acme.example.com"}, {Address: "beep@acme.example.com"}},
			Cc:        []sieve.Address{{Name: "Acme Sales", Address: "sales@ACME.example.com"}},
			Subject:   "I have a present for you",
			Date:      "Tue, 1 Apr 1997 09:06:31 -0800",
			MessageID: "<anvil-1@desert.example.org>",
			Raw: map[string][]string{
				"X-Spam-Score": {"12"},
				"X-Mailer":     {"Acme Mailer 3000"},
			},
		},
		Body:    sieve.Body{ContentType: "text/plain", Content: "Look, I'm sorry about the whole anvil thing.\r\n"},
		RawSize: int64(len(diffEmail)),
	}
}

// diffExec records the delivery actions this library reports, in order.
type diffExec struct{ actions []string }

func (e *diffExec) Keep() { e.actions = append(e.actions, "keep") }
func (e *diffExec) FileInto(folder string, _ bool) {
	e.actions = append(e.actions, "fileinto:"+folder)
}
func (e *diffExec) Redirect(addr string, cp bool) {
	e.actions = append(e.actions, fmt.Sprintf("redirect:%s copy=%t", addr, cp))
}
func (e *diffExec) Flag(string, []string)   {}
func (e *diffExec) Vacation(sieve.Vacation) {}
func (e *diffExec) Notify(string, string)   {}

// deliveries sorts actions and drops repeats: delivering twice to one place is
// one delivery (RFC 5228 §2.10.3).
func deliveries(actions []string) []string {
	slices.Sort(actions)
	return slices.Compact(actions)
}

// ours runs a script through this library and returns where the message is
// delivered, the implicit keep included.
func ours(script string) ([]string, error) {
	s, err := sieve.Parse(script)
	if err != nil {
		return nil, err
	}
	ex := &diffExec{}
	out := s.Evaluate(diffMessage(), ex)
	if out.Error != nil {
		return nil, out.Error
	}
	if out.ImplicitKeep {
		ex.actions = append(ex.actions, "keep")
	}
	return deliveries(ex.actions), nil
}

// reference runs a script through foxcpp/go-sieve, in the same form as ours.
func reference(t *testing.T, script string) ([]string, error) {
	t.Helper()
	loaded, err := foxsieve.Load(strings.NewReader(script), foxsieve.DefaultOptions())
	if err != nil {
		return nil, err
	}
	hdr, err := textproto.NewReader(bufio.NewReader(strings.NewReader(diffEmail))).ReadMIMEHeader()
	if err != nil {
		t.Fatalf("reading the message header: %v", err)
	}
	env := interp.EnvelopeStatic{From: diffEnvelopeFrom, To: diffEnvelopeTo}
	msg := interp.MessageStatic{Size: len(diffEmail), Header: hdr, RawMessage: []byte(diffEmail)}
	data := interp.NewRuntimeData(loaded, interp.DummyPolicy{}, env, msg)
	if err := loaded.Execute(context.Background(), data); err != nil {
		return nil, err
	}
	// The reference appends a keep for the implicit keep, so both sides
	// compare deliveries: "keep" covers an explicit and an implicit keep.
	var actions []string
	for _, a := range data.AppliedActions {
		switch a := a.(type) {
		case interp.ActionKeep:
			actions = append(actions, "keep")
		case interp.ActionFileInto:
			actions = append(actions, "fileinto:"+a.Mailbox)
		case interp.ActionRedirect:
			actions = append(actions, fmt.Sprintf("redirect:%s copy=%t", a.Address, a.Copy))
		case interp.ActionDiscard:
			// Discard only cancels the implicit keep, which is compared below.
		default:
			t.Fatalf("reference reported an action the differential does not compare: %#v", a)
		}
	}
	return deliveries(actions), nil
}

// TestDifferentialEvaluate checks that both implementations deliver the
// message the same way for scripts covering RFC 5228's tests, match types,
// comparators, address parts and control flow, and RFC 3894's :copy.
//
// checklist: SIEVE-ACT-01 SIEVE-ACT-02 SIEVE-ACT-03 SIEVE-ACT-05 SIEVE-ACT-06 SIEVE-CMD-02 SIEVE-CMD-04 SIEVE-MT-01 SIEVE-MT-02 SIEVE-MT-03 SIEVE-MT-05 SIEVE-MT-07 SIEVE-MT-08 SIEVE-TST-01 SIEVE-TST-03 SIEVE-TST-04 SIEVE-TST-05 SIEVE-TST-06 SIEVE-TST-07 SIEVE-TST-08 SIEVE-TST-09 SIEVE-TST-10 SIEVE-EXT-04
func TestDifferentialEvaluate(t *testing.T) {
	const req = `require ["fileinto", "envelope", "copy", "comparator-i;ascii-numeric"]; `
	scripts := []string{
		// Control flow and actions (§3, §4).
		`keep;`,
		`discard;`,
		`require "fileinto"; fileinto "a"; keep;`,
		`redirect "x@example.net";`,
		`if false { keep; } elsif true { discard; } else { keep; }`,
		`require "fileinto"; if true { fileinto "a"; stop; } fileinto "never";`,
		req + `fileinto :copy "copy";`,
		req + `redirect :copy "x@example.net";`,
		req + `fileinto "a"; fileinto "b"; redirect "x@example.net";`,
		// header (§5.7) with each match type (§2.7.1) and comparator (§2.7.3).
		req + `if header :is "Subject" "I have a present for you" { fileinto "hit"; }`,
		req + `if header :is "subject" "I HAVE A PRESENT FOR YOU" { fileinto "hit"; }`,
		req + `if header :is :comparator "i;octet" "Subject" "I HAVE A PRESENT FOR YOU" { fileinto "hit"; }`,
		req + `if header :contains "Subject" "present" { fileinto "hit"; }`,
		req + `if header :contains ["X-Mailer", "Subject"] ["nothing", "3000"] { fileinto "hit"; }`,
		req + `if header :matches "Subject" "I * a pres?nt*" { fileinto "hit"; }`,
		req + `if header :matches "Subject" "present*" { fileinto "hit"; }`,
		req + `if header :matches :comparator "i;octet" "Subject" "i *" { fileinto "hit"; }`,
		req + `if header :is :comparator "i;ascii-numeric" "X-Spam-Score" "012" { fileinto "hit"; }`,
		req + `if header :contains "X-Missing" "" { fileinto "hit"; }`,
		// address (§5.1) and its parts (§2.7.4).
		req + `if address :is "From" "coyote@desert.example.org" { fileinto "hit"; }`,
		req + `if address :all :is "to" "beep@acme.example.com" { fileinto "hit"; }`,
		req + `if address :localpart :is "From" "coyote" { fileinto "hit"; }`,
		req + `if address :domain :is "Cc" "acme.example.com" { fileinto "hit"; }`,
		req + `if address :domain :is :comparator "i;octet" "Cc" "acme.example.com" { fileinto "hit"; }`,
		req + `if address :matches ["To", "Cc"] "*@acme.*" { fileinto "hit"; }`,
		// envelope (§5.4).
		req + `if envelope :is "from" "coyote@desert.example.org" { fileinto "hit"; }`,
		req + `if envelope :domain :is "to" "acme.example.com" { fileinto "hit"; }`,
		req + `if envelope :localpart :is "to" "roadrunner+lists" { fileinto "hit"; }`,
		// exists (§5.5), size (§5.9) and the logical tests (§5.2, §5.3, §5.8).
		req + `if exists ["From", "X-Spam-Score"] { fileinto "hit"; }`,
		req + `if exists ["From", "X-Missing"] { fileinto "hit"; }`,
		req + `if size :over 100 { fileinto "big"; } elsif size :under 100 { fileinto "small"; }`,
		req + `if size :over 1M { fileinto "huge"; }`,
		req + `if allof (true, header :contains "Subject" "present") { fileinto "hit"; }`,
		req + `if allof (true, false) { fileinto "hit"; }`,
		req + `if anyof (false, exists "Date") { fileinto "hit"; }`,
		req + `if not address :domain :is "From" "desert.example.org" { discard; }`,
		// RFC 5228 §9's example script, against this message.
		`require "fileinto";
		if header :contains "from" "coyote" {
			discard;
		} elsif header :contains ["subject"] ["$$$"] {
			discard;
		} else {
			fileinto "INBOX";
		}`,
	}
	for _, script := range scripts {
		want, refErr := reference(t, script)
		got, err := ours(script)
		if refErr != nil {
			t.Errorf("the reference refused a script this test expects it to run: %v\n%s", refErr, script)
			continue
		}
		if err != nil {
			t.Errorf("this library refused a script the reference runs: %v\n%s", err, script)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("actions differ\nscript: %s\nours:      %q\nreference: %q", script, got, want)
		}
	}
}

// TestDifferentialStop pins where this library departs from the reference.
// RFC 5228 §3.3: "If the implicit keep has not been cancelled, then it is
// taken" when stop ends the script. The reference drops it, so the expected
// delivery is written here instead of taken from the reference.
//
// checklist: SIEVE-CMD-04 SIEVE-ACT-06
func TestDifferentialStop(t *testing.T) {
	for _, script := range []string{`stop; keep;`, `require "fileinto"; if true { stop; } fileinto "never";`} {
		got, err := ours(script)
		if err != nil {
			t.Fatalf("%s: %v", script, err)
		}
		if !slices.Equal(got, []string{"keep"}) {
			t.Errorf("%s: deliveries %q, want the implicit keep", script, got)
		}
		if ref, err := reference(t, script); err != nil || len(ref) != 0 {
			t.Logf("the reference now keeps the message (%q, %v); fold this script into TestDifferentialEvaluate", ref, err)
		}
	}
}

// TestDifferentialParse checks that both implementations refuse the same
// malformed scripts.
//
// checklist: SIEVE-CMD-01 SIEVE-CMD-02 SIEVE-CMD-03 SIEVE-LEX-01 SIEVE-LEX-03 SIEVE-LEX-07 SIEVE-LEX-09 SIEVE-LEX-10 SIEVE-MT-04 SIEVE-MT-08 SIEVE-TST-11
func TestDifferentialParse(t *testing.T) {
	bad := []string{
		`keep`,
		`if true keep;`,
		`fileinto "a";`,
		`require "nonexistent-extension";`,
		`if header :is "Subject" { keep; }`,
		`if header :bogus "Subject" "x" { keep; }`,
		`if size 100 { keep; }`,
		`if anyof () { keep; }`,
		`frobnicate;`,
		`"unterminated`,
		`/* unterminated comment`,
	}
	for _, script := range bad {
		if _, err := reference(t, script); err == nil {
			t.Errorf("the reference accepted a script this test expects it to refuse:\n%s", script)
		}
		if _, err := ours(script); err == nil {
			t.Errorf("this library accepted a malformed script:\n%s", script)
		}
	}
	// The reference accepts these, against RFC 5228: a stray closing brace
	// (§8.2), an else with no if (§3.1), two address parts (§2.7.4) and a test
	// list without parentheses (§8.2). Only this library is checked on them.
	referenceAccepts := []string{
		`keep; }`,
		`else { keep; }`,
		`if address :all :localpart "From" "x" { keep; }`,
		`if anyof true { keep; }`,
	}
	for _, script := range referenceAccepts {
		if _, err := ours(script); err == nil {
			t.Errorf("this library accepted a malformed script:\n%s", script)
		}
	}
}
