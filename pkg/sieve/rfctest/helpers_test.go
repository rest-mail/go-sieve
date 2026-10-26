package rfctest

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// ── Test harness ─────────────────────────────────────────────────────

// recExec is a reference Executor that records the actions a script selects
// into a metadata map, the way a mail host would apply them, so the evaluator's
// behaviour can be asserted directly.
type recExec struct {
	meta      map[string]string
	applied   []string
	redirects []redirectCall
}

// redirectCall records one Redirect callback with its per-action :copy flag.
type redirectCall struct {
	addr string
	copy bool
}

func newRecExec() *recExec { return &recExec{meta: map[string]string{}} }

func (e *recExec) Keep() { e.applied = append(e.applied, "keep") }

func (e *recExec) FileInto(folder string, create bool) {
	e.meta["deliver_to_folder"] = folder
	if create {
		e.meta["deliver_to_folder_create"] = "true"
	}
	e.applied = append(e.applied, "fileinto:"+folder)
}

func (e *recExec) Redirect(addr string, copy bool) {
	e.meta["redirect_to"] = addr
	e.applied = append(e.applied, "redirect:"+addr)
	e.redirects = append(e.redirects, redirectCall{addr: addr, copy: copy})
}

func (e *recExec) Flag(op string, flags []string) {
	applyFlags(e.meta, op, flags)
	e.applied = append(e.applied, op+":"+strings.Join(flags, " "))
}

func (e *recExec) Vacation(v sieve.Vacation) {
	e.meta["vacation_reply_to"] = v.ReplyTo
	e.meta["vacation_reply_subject"] = v.Subject
	e.meta["vacation_reply_body"] = v.Body
	if v.Interval > 0 {
		e.meta["vacation_days"] = strconv.Itoa(int(v.Interval / (24 * time.Hour)))
		e.meta["vacation_seconds"] = strconv.Itoa(int(v.Interval / time.Second))
	}
	e.applied = append(e.applied, "vacation:"+v.ReplyTo)
}

func (e *recExec) Notify(method, message string) {
	e.meta["notify_method"] = method
	e.meta["notify_message"] = message
	e.applied = append(e.applied, "notify:"+method)
}

// applyFlags updates the imap4flags flag set stored in the meta map, mirroring a
// host's imap4flags handling.
func applyFlags(meta map[string]string, op string, flags []string) {
	var current []string
	if existing := strings.TrimSpace(meta["imap_flags"]); existing != "" {
		current = strings.Fields(existing)
	}
	switch op {
	case "setflag":
		current = uniqueStrings(flags)
	case "addflag":
		current = uniqueStrings(append(current, flags...))
	case "removeflag":
		remove := make(map[string]struct{}, len(flags))
		for _, fl := range flags {
			remove[fl] = struct{}{}
		}
		kept := current[:0]
		for _, fl := range current {
			if _, drop := remove[fl]; !drop {
				kept = append(kept, fl)
			}
		}
		current = kept
	}
	meta["imap_flags"] = strings.Join(current, " ")
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// sieveResult bundles the recorded actions and terminal outcome of a run.
type sieveResult struct {
	meta         map[string]string
	disposition  sieve.Disposition
	reject       string
	implicitKeep bool
}

func runSieve(t *testing.T, script string, msg *sieve.Message) *sieveResult {
	t.Helper()
	s, err := sieve.Parse(script)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ex := newRecExec()
	out := s.Evaluate(msg, ex)
	return &sieveResult{
		meta:         ex.meta,
		disposition:  out.Disposition,
		reject:       out.RejectReason,
		implicitKeep: out.ImplicitKeep,
	}
}

// folderOf returns the fileinto target recorded on a result, or "".
func folderOf(r *sieveResult) string { return r.meta["deliver_to_folder"] }

func sieveEmail() *sieve.Message {
	return &sieve.Message{
		Envelope: sieve.Envelope{
			From: "sender@example.com",
			To:   []string{"recipient@example.com"},
		},
		Headers: sieve.Headers{
			From:      []sieve.Address{{Address: "sender@example.com"}},
			To:        []sieve.Address{{Address: "recipient@example.com"}},
			Date:      "Mon, 17 Feb 2026 10:00:00 +0000",
			MessageID: "<abc123@example.com>",
			Subject:   "Test message",
		},
		Body: sieve.Body{
			ContentType: "text/plain",
			Content:     "Hello, this is a test message body.",
		},
	}
}
