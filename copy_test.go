package sieve

import (
	"reflect"
	"testing"
)

// These tests pin the RFC 3894 §3 :copy semantics (issue #49): a fileinto or
// redirect tagged :copy does not cancel the implicit keep, it never restores a
// keep another action cancelled, and the Executor learns each redirect's own
// :copy flag rather than inferring it from a script-wide require "copy".

func TestSieve_CopyImplicitKeep(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		redirects    []redirectCall
		applied      []string
		disposition  Disposition
		implicitKeep bool
	}{
		{
			name:         "bare redirect cancels the implicit keep",
			script:       `redirect "a@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com"}},
			applied:      []string{"redirect:a@example.com"},
			implicitKeep: false,
		},
		{
			// A script-wide require "copy" is not a per-action signal: this redirect
			// has no :copy, so it still consumes the message.
			name: "bare redirect cancels the implicit keep under require copy",
			script: `require "copy";
redirect "a@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com"}},
			applied:      []string{"redirect:a@example.com"},
			implicitKeep: false,
		},
		{
			name: "redirect :copy keeps the implicit keep",
			script: `require "copy";
redirect :copy "a@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}},
			applied:      []string{"redirect:a@example.com"},
			implicitKeep: true,
		},
		{
			name: "redirect :copy then bare redirect cancels the implicit keep",
			script: `require "copy";
redirect :copy "a@example.com";
redirect "b@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}, {addr: "b@example.com"}},
			applied:      []string{"redirect:a@example.com", "redirect:b@example.com"},
			implicitKeep: false,
		},
		{
			// A later :copy must not restore the keep the bare redirect cancelled.
			name: "bare redirect then redirect :copy cancels the implicit keep",
			script: `require "copy";
redirect "a@example.com";
redirect :copy "b@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com"}, {addr: "b@example.com", copy: true}},
			applied:      []string{"redirect:a@example.com", "redirect:b@example.com"},
			implicitKeep: false,
		},
		{
			name: "two redirect :copy actions keep the implicit keep",
			script: `require "copy";
redirect :copy "a@example.com";
redirect :copy "b@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}, {addr: "b@example.com", copy: true}},
			applied:      []string{"redirect:a@example.com", "redirect:b@example.com"},
			implicitKeep: true,
		},
		{
			// The explicit keep cancels the implicit one and delivers itself.
			name: "redirect :copy with explicit keep delivers through keep",
			script: `require "copy";
redirect :copy "a@example.com";
keep;`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}},
			applied:      []string{"redirect:a@example.com", "keep"},
			implicitKeep: false,
		},
		{
			// discard cancels the implicit keep; the forwarded copy still goes out,
			// so the message is not dropped (RFC 5228 §4.4).
			name: "discard then redirect :copy forwards without a keep",
			script: `require "copy";
discard;
redirect :copy "a@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}},
			applied:      []string{"redirect:a@example.com"},
			implicitKeep: false,
		},
		{
			name: "redirect :copy then discard forwards without a keep",
			script: `require "copy";
redirect :copy "a@example.com";
discard;`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}},
			applied:      []string{"redirect:a@example.com"},
			implicitKeep: false,
		},
		{
			name: "fileinto :copy keeps the implicit keep",
			script: `require ["fileinto", "copy"];
fileinto :copy "Archive";`,
			applied:      []string{"fileinto:Archive"},
			implicitKeep: true,
		},
		{
			name: "bare fileinto cancels the implicit keep under require copy",
			script: `require ["fileinto", "copy"];
fileinto "Archive";`,
			applied:      []string{"fileinto:Archive"},
			implicitKeep: false,
		},
		{
			name: "fileinto :copy then bare redirect cancels the implicit keep",
			script: `require ["fileinto", "copy"];
fileinto :copy "Archive";
redirect "a@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com"}},
			applied:      []string{"fileinto:Archive", "redirect:a@example.com"},
			implicitKeep: false,
		},
		{
			name: "bare fileinto then redirect :copy cancels the implicit keep",
			script: `require ["fileinto", "copy"];
fileinto "Archive";
redirect :copy "a@example.com";`,
			redirects:    []redirectCall{{addr: "a@example.com", copy: true}},
			applied:      []string{"fileinto:Archive", "redirect:a@example.com"},
			implicitKeep: false,
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
			if !reflect.DeepEqual(ex.redirects, tc.redirects) {
				t.Errorf("Redirect calls = %+v, want %+v", ex.redirects, tc.redirects)
			}
			if !reflect.DeepEqual(ex.applied, tc.applied) {
				t.Errorf("applied = %v, want %v", ex.applied, tc.applied)
			}
		})
	}
}
