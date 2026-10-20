package sieve

import (
	"errors"
	"reflect"
	"testing"
)

// RFC 5490 defines three independent capabilities. Only fileinto :create from
// the mailbox capability is implemented; the checklist records the missing
// mailboxexists test separately from the host's storage responsibilities.

// checklist: MAILBOX-01
func TestMailbox_RequiresCapability(t *testing.T) {
	good := []string{
		`require "mailbox"; keep;`,
		`require ["mailbox", "fileinto"]; fileinto :create "Archive";`,
		`require "fileinto"; require "mailbox"; fileinto :create "Archive";`,
	}
	for _, script := range good {
		if err := Validate(script); err != nil {
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
		if err := Validate(script); err == nil {
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
		if err := Validate(requires + action); err != nil {
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
		if err := Validate(requires + action); err == nil {
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
			s, err := Parse(`require ["fileinto", "mailbox", "copy"]; ` + tc.actions)
			if err != nil {
				t.Fatal(err)
			}
			ex := &mailboxExec{recExec: newRecExec()}
			out := s.Evaluate(sieveEmail(), ex)
			if out.Error != nil || out.Disposition != Continue || out.ImplicitKeep != tc.implicitKeep {
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
	s, err := Parse(`require ["fileinto", "mailbox"];
fileinto :create "Archive";
keep;
fileinto "Later";`)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("mailbox creation denied")
	ex := &mailboxExec{recExec: newRecExec(), fail: failure}
	out := s.Evaluate(sieveEmail(), ex)
	if !errors.Is(out.Error, failure) || !out.ImplicitKeep || out.Disposition != Continue {
		t.Fatalf("creation failure must report the error and fail safe to keep: %+v", out)
	}
	if want := []mailboxCall{{"Archive", true}}; !reflect.DeepEqual(ex.files, want) {
		t.Errorf("FileInto calls = %+v, want %+v", ex.files, want)
	}
	if len(ex.applied) != 0 {
		t.Errorf("actions after the creation failure ran: %v", ex.applied)
	}
}
