package rfctest

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// policyExec is a recExec that also implements sieve.RedirectReporter: each
// redirect returns the result results holds for its address (nil when absent)
// and is recorded as "redirect:<addr>" or "redirect:copy:<addr>".
type policyExec struct {
	*recExec
	results map[string]error
	tried   []string
}

func (e *policyExec) TryRedirect(addr string, copy bool) error {
	if copy {
		e.tried = append(e.tried, "redirect:copy:"+addr)
	} else {
		e.tried = append(e.tried, "redirect:"+addr)
	}
	return e.results[addr]
}

// TestRFC5228RedirectPolicySkip checks that a redirect host policy silently
// ignored does not cancel the implicit keep (RFC 5228 §4.2), for consuming and
// :copy redirects (RFC 3894), and that a skip is told apart from a successful
// redirect and from a runtime failure, which fails safe to the implicit keep
// (RFC 5228 §2.10.6).
//
// checklist: SIEVE-CORE-20
func TestRFC5228RedirectPolicySkip(t *testing.T) {
	skip := sieve.ErrRedirectSkipped
	failure := errors.New("smtp relay down")
	tests := []struct {
		name         string
		script       string
		results      map[string]error
		tried        []string
		disposition  sieve.Disposition
		implicitKeep bool
		failed       bool
	}{
		{"consuming redirect done", `redirect "a@example.net";`, nil,
			[]string{"redirect:a@example.net"}, sieve.Continue, false, false},
		{"consuming redirect skipped keeps", `redirect "a@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:a@example.net"}, sieve.Continue, true, false},
		{"wrapped skip", `redirect "a@example.net";`, map[string]error{"a@example.net": fmt.Errorf("limit reached: %w", skip)},
			[]string{"redirect:a@example.net"}, sieve.Continue, true, false},
		{":copy redirect done keeps", `redirect :copy "a@example.net";`, nil,
			[]string{"redirect:copy:a@example.net"}, sieve.Continue, true, false},
		{":copy redirect skipped keeps", `redirect :copy "a@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:copy:a@example.net"}, sieve.Continue, true, false},
		{"one skipped, one done", `redirect "a@example.net"; redirect "b@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:a@example.net", "redirect:b@example.net"}, sieve.Continue, false, false},
		{"skipped twice is asked once", `redirect "a@example.net"; redirect "a@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:a@example.net"}, sieve.Continue, true, false},
		{"skipped redirect does not override discard", `discard; redirect "a@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:a@example.net"}, sieve.Discard, false, false},
		{"skipped :copy redirect does not override discard", `discard; redirect :copy "a@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:copy:a@example.net"}, sieve.Discard, false, false},
		{"done :copy redirect overrides discard", `discard; redirect :copy "a@example.net";`, nil,
			[]string{"redirect:copy:a@example.net"}, sieve.Continue, false, false},
		{"skip after keep", `keep; redirect "a@example.net";`, map[string]error{"a@example.net": skip},
			[]string{"redirect:a@example.net"}, sieve.Continue, false, false},
		{"runtime failure fails safe", `redirect "a@example.net"; redirect "b@example.net";`, map[string]error{"a@example.net": failure},
			[]string{"redirect:a@example.net"}, sieve.Continue, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := sieve.Parse(`require "copy"; ` + tt.script)
			if err != nil {
				t.Fatal(err)
			}
			ex := &policyExec{recExec: newRecExec(), results: tt.results}
			out := s.Evaluate(sieveEmail(), ex)
			if !reflect.DeepEqual(ex.tried, tt.tried) {
				t.Errorf("TryRedirect calls = %q, want %q", ex.tried, tt.tried)
			}
			if len(ex.redirects) != 0 {
				t.Errorf("Redirect called alongside TryRedirect: %v", ex.redirects)
			}
			if out.Disposition != tt.disposition || out.ImplicitKeep != tt.implicitKeep {
				t.Errorf("outcome = %v, implicit keep %v; want %v, %v", out.Disposition, out.ImplicitKeep, tt.disposition, tt.implicitKeep)
			}
			if tt.failed {
				if !errors.Is(out.Error, failure) || errors.Is(out.Error, skip) {
					t.Errorf("Error = %v, want the host failure", out.Error)
				}
			} else if out.Error != nil {
				t.Errorf("Error = %v, want nil", out.Error)
			}
		})
	}
}
