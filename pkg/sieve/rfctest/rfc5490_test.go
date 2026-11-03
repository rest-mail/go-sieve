package rfctest

import (
	"errors"
	"reflect"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// RFC 5490 defines three independent capabilities. The mailbox capability is
// implemented: fileinto :create and the mailboxexists test, which the host
// answers through MailboxChecker. mboxmetadata and servermetadata are not.

// checklist: MAILBOX-01
func TestMailbox_RequiresCapability(t *testing.T) {
	good := []string{
		`require "mailbox"; keep;`,
		`require ["mailbox", "fileinto"]; fileinto :create "Archive";`,
		`require "fileinto"; require "mailbox"; fileinto :create "Archive";`,
	}
	for _, script := range good {
		if err := sieve.Validate(script); err != nil {
			t.Errorf("%q: %v", script, err)
		}
	}
	bad := []string{
		`fileinto :create "Archive";`,
		`require "fileinto"; fileinto :create "Archive";`,
		`require "mailbox"; fileinto :create "Archive";`,
		`require "fileinto"; if true { fileinto :create "Archive"; }`,
	}
	for _, script := range bad {
		if err := sieve.Validate(script); err == nil {
			t.Errorf("%q: accepted without both required capabilities", script)
		}
	}
}

// checklist: MAILBOX-02
func TestMailbox_CreateSyntax(t *testing.T) {
	const requires = `require ["fileinto", "mailbox", "copy", "imap4flags"]; `
	good := []string{
		`fileinto "Archive";`,
		`fileinto :create "Archive";`,
		`fileinto :create :copy "Archive";`,
		`fileinto :copy :create "Archive";`,
		`fileinto :flags ["seen"] :create "Archive";`,
		`fileinto :create :flags ["seen"] "Archive";`,
		`FILEINTO :CREATE "Archive";`,
	}
	for _, action := range good {
		if err := sieve.Validate(requires + action); err != nil {
			t.Errorf("%q: %v", action, err)
		}
	}
	bad := []string{
		`fileinto :create;`,
		`fileinto :create :create "Archive";`,
		`fileinto :create ["Archive"];`,
		`fileinto :create 1;`,
		`fileinto :create "Archive" "Other";`,
		`fileinto "Archive" :create;`,
		`keep :create;`,
		`redirect :create "user@example.com";`,
		`if exists :create "Subject" { keep; }`,
	}
	for _, action := range bad {
		if err := sieve.Validate(requires + action); err == nil {
			t.Errorf("%q: accepted, want a syntax error", action)
		}
	}
}

type mailboxCall struct {
	folder string
	create bool
}

type mailboxExec struct {
	*recExec
	files []mailboxCall
	fail  error
}

func (e *mailboxExec) FileInto(folder string, create bool) {
	e.files = append(e.files, mailboxCall{folder, create})
	if e.fail != nil {
		panic(e.fail)
	}
}

// checklist: MAILBOX-03 MAILBOX-04
func TestMailbox_CreateCallback(t *testing.T) {
	tests := []struct {
		name         string
		actions      string
		files        []mailboxCall
		implicitKeep bool
	}{
		{
			name:    "ordinary fileinto does not request creation",
			actions: `fileinto "Archive";`,
			files:   []mailboxCall{{"Archive", false}},
		},
		{
			name:    "create preserves the mailbox name",
			actions: `fileinto :create "Projects/Ärchive";`,
			files:   []mailboxCall{{"Projects/Ärchive", true}},
		},
		{
			name:    "create belongs only to its own action",
			actions: `fileinto :create "New"; fileinto "Existing";`,
			files:   []mailboxCall{{"New", true}, {"Existing", false}},
		},
		{
			name:         "create and copy leave the implicit keep",
			actions:      `fileinto :copy :create "Archive";`,
			files:        []mailboxCall{{"Archive", true}},
			implicitKeep: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := sieve.Parse(`require ["fileinto", "mailbox", "copy"]; ` + tc.actions)
			if err != nil {
				t.Fatal(err)
			}
			ex := &mailboxExec{recExec: newRecExec()}
			out := s.Evaluate(sieveEmail(), ex)
			if out.Error != nil || out.Disposition != sieve.Continue || out.ImplicitKeep != tc.implicitKeep {
				t.Fatalf("unexpected outcome: %+v", out)
			}
			if !reflect.DeepEqual(ex.files, tc.files) {
				t.Errorf("FileInto calls = %+v, want %+v", ex.files, tc.files)
			}
		})
	}
}

// checklist: MAILBOX-07
func TestMailbox_CreationFailure(t *testing.T) {
	s, err := sieve.Parse(`require ["fileinto", "mailbox"];
fileinto :create "Archive";
keep;
fileinto "Later";`)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("mailbox creation denied")
	ex := &mailboxExec{recExec: newRecExec(), fail: failure}
	out := s.Evaluate(sieveEmail(), ex)
	if !errors.Is(out.Error, failure) || !out.ImplicitKeep || out.Disposition != sieve.Continue {
		t.Fatalf("creation failure must report the error and fail safe to keep: %+v", out)
	}
	if want := []mailboxCall{{"Archive", true}}; !reflect.DeepEqual(ex.files, want) {
		t.Errorf("FileInto calls = %+v, want %+v", ex.files, want)
	}
	if len(ex.applied) != 0 {
		t.Errorf("actions after the creation failure ran: %v", ex.applied)
	}
}

// mailstore is a MailboxChecker for one user: writable holds the mailboxes the
// user may deliver into, readOnly those that exist without that right, and
// fail those whose lookup fails. calls records every query.
type mailstore struct {
	*recExec
	writable map[string]bool
	readOnly map[string]bool
	fail     map[string]error
	calls    []string
}

func (m *mailstore) MailboxExists(name string) (bool, error) {
	m.calls = append(m.calls, name)
	if err := m.fail[name]; err != nil {
		return false, err
	}
	if m.readOnly[name] {
		return false, nil // exists, but the user has no p or i right
	}
	return m.writable[name], nil
}

func newMailstore() *mailstore {
	return &mailstore{
		recExec:  newRecExec(),
		writable: map[string]bool{"INBOX": true, "Partners": true, "Lists/Go": true, "Ärchive": true},
		readOnly: map[string]bool{"Shared/Announcements": true},
		fail:     map[string]error{},
	}
}

// checklist: MAILBOX-08
func TestMailboxExists_Syntax(t *testing.T) {
	good := []string{
		`require "mailbox"; if mailboxexists "Partners" { keep; }`,
		`require "mailbox"; if mailboxexists ["Partners", "INBOX"] { keep; }`,
		`require "mailbox"; if not mailboxexists "Partners" { keep; }`,
		`require "mailbox"; if MAILBOXEXISTS "Partners" { keep; }`,
		`require ["fileinto", "reject", "mailbox"]; if mailboxexists "Partners" { fileinto "Partners"; } else { reject "This message was not accepted by the Mailstore"; }`,
	}
	for _, script := range good {
		if err := sieve.Validate(script); err != nil {
			t.Errorf("%q: %v", script, err)
		}
	}
	bad := []string{
		`if mailboxexists "Partners" { keep; }`,
		`require "fileinto"; if mailboxexists "Partners" { keep; }`,
		`require "mailbox"; if mailboxexists { keep; }`,
		`require "mailbox"; if mailboxexists [] { keep; }`,
		`require "mailbox"; if mailboxexists 1 { keep; }`,
		`require "mailbox"; if mailboxexists :create "Partners" { keep; }`,
		`require "mailbox"; if mailboxexists "Partners" "INBOX" { keep; }`,
		`require "mailbox"; mailboxexists "Partners";`,
	}
	for _, script := range bad {
		if err := sieve.Validate(script); err == nil {
			t.Errorf("%q: accepted, want an error", script)
		}
	}
}

// TestMailboxExists_RFCExample runs the RFC 5490 §3.1 example: a deliverable
// "Partners" is filed into, otherwise the message is rejected.
//
// checklist: MAILBOX-08 MAILBOX-09
func TestMailboxExists_RFCExample(t *testing.T) {
	const script = `require ["fileinto", "reject", "mailbox"];
if mailboxexists "Partners" {
   fileinto "Partners";
} else {
   reject "This message was not accepted by the Mailstore";
}`
	s, err := sieve.Parse(script)
	if err != nil {
		t.Fatal(err)
	}
	ex := newMailstore()
	out := s.Evaluate(sieveEmail(), ex)
	if out.Error != nil || out.Disposition != sieve.Continue || folderOf(&sieveResult{meta: ex.meta}) != "Partners" {
		t.Errorf("Partners deliverable: outcome %+v, applied %v", out, ex.applied)
	}
	ex = newMailstore()
	delete(ex.writable, "Partners")
	out = s.Evaluate(sieveEmail(), ex)
	if out.Error != nil || out.Disposition != sieve.Reject || out.RejectReason != "This message was not accepted by the Mailstore" {
		t.Errorf("Partners missing: outcome %+v", out)
	}
}

// TestMailboxExists_StringList checks the RFC 5490 §3.1 semantics: the test
// is true only if every named mailbox exists and accepts delivery by the
// script's user; a missing or read-only mailbox makes it false.
//
// checklist: MAILBOX-08 MAILBOX-09
func TestMailboxExists_StringList(t *testing.T) {
	tests := []struct {
		name  string
		names string
		want  bool
		calls []string
	}{
		{"one deliverable mailbox", `"Partners"`, true, []string{"Partners"}},
		{"every mailbox deliverable", `["INBOX", "Lists/Go", "Ärchive"]`, true, []string{"INBOX", "Lists/Go", "Ärchive"}},
		{"one missing mailbox", `"Nowhere"`, false, []string{"Nowhere"}},
		{"one of several missing", `["INBOX", "Nowhere", "Partners"]`, false, []string{"INBOX", "Nowhere"}},
		{"exists but not writable", `"Shared/Announcements"`, false, []string{"Shared/Announcements"}},
		{"writable and read-only", `["Partners", "Shared/Announcements"]`, false, []string{"Partners", "Shared/Announcements"}},
		{"name passed unchanged", `"lists/go"`, false, []string{"lists/go"}},
		{"repeated name", `["INBOX", "INBOX"]`, true, []string{"INBOX", "INBOX"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := sieve.Parse(`require "mailbox"; if mailboxexists ` + tc.names + ` { discard; }`)
			if err != nil {
				t.Fatal(err)
			}
			ex := newMailstore()
			out := s.Evaluate(sieveEmail(), ex)
			if out.Error != nil {
				t.Fatalf("runtime error: %v", out.Error)
			}
			if got := out.Disposition == sieve.Discard; got != tc.want {
				t.Errorf("mailboxexists %s = %v, want %v", tc.names, got, tc.want)
			}
			if !reflect.DeepEqual(ex.calls, tc.calls) {
				t.Errorf("queries = %v, want %v", ex.calls, tc.calls)
			}
		})
	}
}

// TestMailboxExists_QueryFailure checks that a failed lookup is a runtime
// error (RFC 5228 §2.10.6), not a missing mailbox: evaluation stops, the
// message is kept and the host gets the error, even under not.
//
// checklist: MAILBOX-08
func TestMailboxExists_QueryFailure(t *testing.T) {
	failure := errors.New("mailstore temporarily unavailable")
	scripts := []string{
		`require ["mailbox", "fileinto", "reject"]; fileinto "Before"; if mailboxexists ["INBOX", "Partners"] { discard; } fileinto "After";`,
		`require ["mailbox", "fileinto", "reject"]; fileinto "Before"; if not mailboxexists ["INBOX", "Partners"] { reject "no"; } fileinto "After";`,
	}
	for _, script := range scripts {
		s, err := sieve.Parse(script)
		if err != nil {
			t.Fatal(err)
		}
		ex := newMailstore()
		ex.fail["Partners"] = failure
		out := s.Evaluate(sieveEmail(), ex)
		if !errors.Is(out.Error, failure) || !out.ImplicitKeep || out.Disposition != sieve.Continue {
			t.Errorf("%q: want the query error and the fail-safe keep, got %+v", script, out)
		}
		if want := []string{"fileinto:Before"}; !reflect.DeepEqual(ex.applied, want) {
			t.Errorf("%q: applied %v, want %v", script, ex.applied, want)
		}
	}
	// A false answer before the failing name ends the test without asking.
	s, err := sieve.Parse(`require "mailbox"; if mailboxexists ["Nowhere", "Partners"] { discard; }`)
	if err != nil {
		t.Fatal(err)
	}
	ex := newMailstore()
	ex.fail["Partners"] = failure
	if out := s.Evaluate(sieveEmail(), ex); out.Error != nil || out.Disposition != sieve.Continue {
		t.Errorf("missing first mailbox: %+v", out)
	}
}

// TestMailboxExists_NoQuery checks the defined behaviour for a host whose
// Executor does not implement MailboxChecker: reaching mailboxexists is a
// runtime error with the fail-safe keep, so the evaluator never guesses.
// A script that does not reach the test is unaffected.
//
// checklist: MAILBOX-08
func TestMailboxExists_NoQuery(t *testing.T) {
	s, err := sieve.Parse(`require ["mailbox", "fileinto"]; if mailboxexists "Partners" { fileinto "Partners"; } else { discard; }`)
	if err != nil {
		t.Fatal(err)
	}
	ex := newRecExec()
	out := s.Evaluate(sieveEmail(), ex)
	if !errors.Is(out.Error, sieve.ErrMailboxQueryUnavailable) || !out.ImplicitKeep || out.Disposition != sieve.Continue {
		t.Errorf("want ErrMailboxQueryUnavailable and the fail-safe keep, got %+v", out)
	}
	if len(ex.applied) != 0 {
		t.Errorf("actions ran: %v", ex.applied)
	}
	s, err = sieve.Parse(`require "mailbox"; if anyof (true, mailboxexists "Partners") { discard; }`)
	if err != nil {
		t.Fatal(err)
	}
	if out := s.Evaluate(sieveEmail(), newRecExec()); out.Error != nil || out.Disposition != sieve.Discard {
		t.Errorf("unreached mailboxexists: %+v", out)
	}
}

// TestMailboxExists_NoDeliveryPromise checks that a true mailboxexists does
// not stand in for delivery: the fileinto still goes to the host, whose
// failure (for example over quota) is a runtime error with the fail-safe keep.
//
// checklist: MAILBOX-10
func TestMailboxExists_NoDeliveryPromise(t *testing.T) {
	s, err := sieve.Parse(`require ["mailbox", "fileinto"]; if mailboxexists "Partners" { fileinto "Partners"; }`)
	if err != nil {
		t.Fatal(err)
	}
	quota := errors.New("over quota")
	ex := &quotaStore{mailstore: newMailstore(), fail: quota}
	out := s.Evaluate(sieveEmail(), ex)
	if !errors.Is(out.Error, quota) || !out.ImplicitKeep {
		t.Errorf("want the delivery failure and the fail-safe keep, got %+v", out)
	}
	if want := []string{"Partners"}; !reflect.DeepEqual(ex.calls, want) {
		t.Errorf("queries = %v, want %v", ex.calls, want)
	}
}

// quotaStore answers mailboxexists but fails every fileinto.
type quotaStore struct {
	*mailstore
	fail error
}

func (q *quotaStore) FileInto(string, bool) { panic(q.fail) }
