package sieve

import (
	"reflect"
	"testing"
)

// These tests check RFC 3894 (the "copy" extension) clause by clause; the
// implicit-keep cases themselves are in copy_test.go.

// TestCopy_RequiresCapability checks that :copy is allowed only once the script
// declares the "copy" capability (RFC 3894 §2; RFC 5228 §3.2).
//
// checklist: COPY-01
func TestCopy_RequiresCapability(t *testing.T) {
	good := []string{
		`require "copy"; redirect :copy "a@example.com";`,
		`require ["copy", "fileinto"]; fileinto :copy "Archive";`,
		`require ["fileinto", "copy"]; fileinto :copy "Archive";`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`redirect :copy "a@example.com";`,
		`require "fileinto"; fileinto :copy "Archive";`,
		`require "fileinto"; if true { fileinto :copy "Archive"; }`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("%q: accepted without require \"copy\", want an error", s)
		}
	}
}

// TestCopy_Syntax checks the grammar of RFC 3894 §3: :copy is an optional tag
// of fileinto and redirect, given at most once, and of no other command.
//
// checklist: COPY-02
func TestCopy_Syntax(t *testing.T) {
	good := []string{
		`require "copy"; redirect "a@example.com";`,
		`require "copy"; redirect :copy "a@example.com";`,
		`require ["copy", "fileinto"]; fileinto "Archive";`,
		`require ["copy", "fileinto"]; fileinto :copy "Archive";`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`require "copy"; redirect :copy :copy "a@example.com";`,
		`require ["copy", "fileinto"]; fileinto :copy :copy "Archive";`,
		`require "copy"; redirect :copy;`,
		`require ["copy", "fileinto"]; fileinto :copy;`,
		`require "copy"; keep :copy;`,
		`require "copy"; discard :copy;`,
		`require "copy"; stop :copy;`,
		`require "copy"; if header :copy "Subject" "x" { keep; }`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestCopy_InAdditionToOtherActions checks that a :copy action files or
// redirects a copy in addition to whatever else happens to the message (RFC
// 3894 §3): the actions around it still take effect, and the copy is not
// withdrawn by them.
//
// checklist: COPY-04
func TestCopy_InAdditionToOtherActions(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		applied      []string
		disposition  Disposition
		implicitKeep bool
	}{
		{
			name: "fileinto :copy then discard files the copy and drops the keep",
			script: `require ["copy", "fileinto"];
fileinto :copy "Archive";
discard;`,
			applied:      []string{"fileinto:Archive"},
			implicitKeep: false,
		},
		{
			name: "fileinto :copy then fileinto files both",
			script: `require ["copy", "fileinto"];
fileinto :copy "Archive";
fileinto "Work";`,
			applied:      []string{"fileinto:Archive", "fileinto:Work"},
			implicitKeep: false,
		},
		{
			name: "fileinto :copy and redirect :copy both act and keep the implicit keep",
			script: `require ["copy", "fileinto"];
fileinto :copy "Archive";
redirect :copy "a@example.com";`,
			applied:      []string{"fileinto:Archive", "redirect:a@example.com"},
			implicitKeep: true,
		},
		{
			name: "fileinto :copy then stop keeps the implicit keep",
			script: `require ["copy", "fileinto"];
fileinto :copy "Archive";
stop;
discard;`,
			applied:      []string{"fileinto:Archive"},
			implicitKeep: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Parse(tc.script)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			ex := newRecExec()
			out := s.Evaluate(sieveEmail(), ex)
			if out.Error != nil {
				t.Fatalf("Evaluate: unexpected error %v", out.Error)
			}
			if out.Disposition != tc.disposition {
				t.Errorf("disposition = %v, want %v", out.Disposition, tc.disposition)
			}
			if out.ImplicitKeep != tc.implicitKeep {
				t.Errorf("ImplicitKeep = %v, want %v", out.ImplicitKeep, tc.implicitKeep)
			}
			if !reflect.DeepEqual(ex.applied, tc.applied) {
				t.Errorf("applied = %v, want %v", ex.applied, tc.applied)
			}
		})
	}
}

// TestCopy_RFCExample runs the example of RFC 3894 §3, with a following test
// standing in for its "more processing follows" comment.
//
// checklist: COPY-05
func TestCopy_RFCExample(t *testing.T) {
	s, err := Parse(`require ["copy", "fileinto"];
fileinto :copy "incoming";
# ... more processing follows ...
if header :contains "Subject" "no such subject" { discard; }`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ex := newRecExec()
	out := s.Evaluate(sieveEmail(), ex)
	if out.Error != nil {
		t.Fatalf("Evaluate: unexpected error %v", out.Error)
	}
	if want := []string{"fileinto:incoming"}; !reflect.DeepEqual(ex.applied, want) {
		t.Errorf("applied = %v, want %v", ex.applied, want)
	}
	if !out.ImplicitKeep {
		t.Error("ImplicitKeep = false, want true: fileinto :copy does not cancel it")
	}
}
