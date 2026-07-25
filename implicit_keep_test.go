package sieve

import "testing"

// These tests exercise the RFC 5228 §2.10.2 implicit-keep model and the
// §2.10.6 fail-safe keep on a runtime error (issues #5 and #7).

// panicExec is an Executor whose FileInto callback panics, simulating a host
// delivery failure (a runtime error mid-script). Every other method is inherited
// from the embedded recExec.
type panicExec struct{ *recExec }

func (p panicExec) FileInto(folder string, create bool) {
	panic("simulated delivery failure")
}

// (a) #5 mail-loss: `discard` followed by `fileinto` must still file the message.
// RFC 5228 §4.4: "If 'discard' is used with other actions, the other actions
// still happen ... 'fileinto'+'discard' is equivalent to 'fileinto'." Before the
// fix, discard halted the script, so the fileinto never ran and the message was
// lost.
func TestSieve_DiscardThenFileintoStillDelivers(t *testing.T) {
	script := `require "fileinto";
if header :is "Subject" "Test message" {
  discard;
  fileinto "Archive";
}`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Continue {
		t.Errorf("discard then fileinto: expected Continue (delivering action wins), got %v", r.disposition)
	}
	if folderOf(r) != "Archive" {
		t.Errorf("discard then fileinto: expected delivery to Archive, got folder=%q", folderOf(r))
	}
	if r.implicitKeep {
		t.Errorf("discard then fileinto: a delivering action ran, implicit keep must be off")
	}
}

// The reverse order (fileinto then discard) must also deliver: discard only
// cancels the implicit keep, it does not withdraw an already-recorded delivery.
func TestSieve_FileintoThenDiscardStillDelivers(t *testing.T) {
	script := `require "fileinto";
if true {
  fileinto "Archive";
  discard;
}`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Continue {
		t.Errorf("fileinto then discard: expected Continue, got %v", r.disposition)
	}
	if folderOf(r) != "Archive" {
		t.Errorf("fileinto then discard: expected delivery to Archive, got folder=%q", folderOf(r))
	}
}

// discard must no longer stop the script: an action after it still runs.
func TestSieve_DiscardDoesNotStopScript(t *testing.T) {
	script := `require "fileinto";
if true {
  discard;
  fileinto "Later";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Later" {
		t.Errorf("expected fileinto after discard to run, got folder=%q", got)
	}
}

// (b) A bare discard (nothing else) drops the message: the implicit keep is
// cancelled and no delivering action was taken.
func TestSieve_BareDiscardDropsMessage(t *testing.T) {
	script := `if true { discard; }`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Discard {
		t.Errorf("bare discard: expected Discard, got %v", r.disposition)
	}
	if r.implicitKeep {
		t.Errorf("bare discard: implicit keep must be cancelled")
	}
}

// (c) A script that takes no delivering action (empty, or all tests fall through)
// delivers via the implicit keep to the default mailbox.
func TestSieve_NoActionImpliesImplicitKeep(t *testing.T) {
	// A script whose only command's test does not match, so no action fires.
	script := `require "fileinto";
if header :is "Subject" "does-not-match" {
  fileinto "Never";
}`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Continue {
		t.Errorf("no-action script: expected Continue, got %v", r.disposition)
	}
	if !r.implicitKeep {
		t.Errorf("no-action script: expected implicit keep to be in effect")
	}
	if folderOf(r) != "" {
		t.Errorf("no-action script: expected no explicit delivery, got folder=%q", folderOf(r))
	}
}

// An entirely empty script also delivers via the implicit keep.
func TestSieve_EmptyScriptImpliesImplicitKeep(t *testing.T) {
	r := runSieve(t, ``, sieveEmail())
	if r.disposition != Continue {
		t.Errorf("empty script: expected Continue, got %v", r.disposition)
	}
	if !r.implicitKeep {
		t.Errorf("empty script: expected implicit keep to be in effect")
	}
}

// (d) `stop` before any action ends processing, but the implicit keep (not yet
// cancelled) still delivers to the default mailbox.
func TestSieve_StopBeforeActionKeepsImplicitly(t *testing.T) {
	script := `require "fileinto";
stop;
if true { fileinto "Unreached"; }`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Continue {
		t.Errorf("stop before action: expected Continue, got %v", r.disposition)
	}
	if !r.implicitKeep {
		t.Errorf("stop before action: expected implicit keep to still deliver")
	}
	if folderOf(r) != "" {
		t.Errorf("stop before action: expected no fileinto, got folder=%q", folderOf(r))
	}
}

// A `discard` before `stop` cancels the implicit keep; stop then ends processing
// with no delivering action, so the message is dropped (not implicitly kept).
func TestSieve_DiscardThenStopDrops(t *testing.T) {
	script := `require "fileinto";
if true {
  discard;
  stop;
}
if true { fileinto "Unreached"; }`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Discard {
		t.Errorf("discard then stop: expected Discard, got %v", r.disposition)
	}
}

// (e) A runtime error mid-script (an Executor callback panics) must not lose the
// message: RFC 5228 §2.10.6 requires a fail-safe implicit keep, with the error
// reported so the host can notify the user.
func TestSieve_RuntimeErrorKeepsMessage(t *testing.T) {
	s, err := Parse(`require "fileinto";
if true { fileinto "Boom"; }`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var out Outcome
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("evaluation panicked instead of failing safe to an implicit keep: %v", r)
			}
		}()
		out = s.Evaluate(sieveEmail(), panicExec{newRecExec()})
	}()
	if out.Disposition != Continue {
		t.Errorf("runtime error: expected Continue (fail-safe keep), got %v", out.Disposition)
	}
	if !out.ImplicitKeep {
		t.Errorf("runtime error: expected the fail-safe implicit keep to be in effect")
	}
	if out.Error == nil {
		t.Errorf("runtime error: expected Outcome.Error to report the failure")
	}
}
