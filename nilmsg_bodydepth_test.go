package sieve

import "testing"

// ── issue #23: nil message and unbounded body traversal ──────────────

// TestEvaluate_NilMessage_DoesNotPanic proves the entrypoint guard for a nil
// *Message. On the unfixed evaluator the first test evaluated reads msg.Headers
// on a nil pointer and panics (nil dereference); the guard must turn that into
// a clean, defined result — the implicit keep (Continue) — instead.
func TestEvaluate_NilMessage_DoesNotPanic(t *testing.T) {
	// A script that actually evaluates a test, so a missing nil guard is
	// exercised rather than skipped.
	script := `require "fileinto";
if header :contains "Subject" "test" { fileinto "X"; }`
	s, err := Parse(script)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Evaluate(nil message) panicked instead of returning cleanly: %v", r)
		}
	}()

	ex := newRecExec()
	out := s.Evaluate(nil, ex)

	if out.Disposition != Continue {
		t.Fatalf("nil message should evaluate to the implicit keep (Continue), got %v", out.Disposition)
	}
	// With no message to test against, no action should have fired.
	if len(ex.applied) != 0 {
		t.Fatalf("nil message should select no actions, got %v", ex.applied)
	}
}

// TestEvaluate_NilMessage_NilScriptStillGuarded keeps the existing nil-Script
// guarantee intact alongside the new nil-message guard.
func TestEvaluate_NilMessage_NilScriptStillGuarded(t *testing.T) {
	var s *Script
	out := s.Evaluate(nil, newRecExec())
	if out.Disposition != Continue {
		t.Fatalf("nil script + nil message should be Continue, got %v", out.Disposition)
	}
}

// deepBodyChain builds a single-child multipart chain `depth` levels deep whose
// only text/plain content — leaf — sits at the very bottom. Descending the whole
// chain is required to reach the leaf.
func deepBodyChain(depth int, leaf string) Body {
	cur := Body{ContentType: "text/plain", Content: leaf}
	for i := 0; i < depth; i++ {
		cur = Body{ContentType: "multipart/mixed", Parts: []Body{cur}}
	}
	return cur
}

// TestEvaluate_BodyTest_DepthBounded proves the body test's descent into nested
// MIME structure is capped. The matching content is buried well past
// maxBodyDepth: an unbounded walk (the old code) recurses all the way down and
// matches; a depth-bounded walk stops first and, treating the deeper structure
// as absent, does not match.
func TestEvaluate_BodyTest_DepthBounded(t *testing.T) {
	const marker = "deeplyburiedneedle"
	email := sieveEmail()
	email.Body = deepBodyChain(maxBodyDepth+50, marker)

	script := `require ["body", "fileinto"];
if body :contains "` + marker + `" { fileinto "TooDeep"; }`
	r := runSieve(t, script, email)

	if folderOf(r) == "TooDeep" {
		t.Fatalf("body test reached content buried %d levels deep (past the %d-level cap); traversal is unbounded",
			maxBodyDepth+50, maxBodyDepth)
	}
}

// TestEvaluate_BodyTest_DepthWithinCapStillMatches guards against the cap being
// so tight that ordinary, modestly nested messages stop matching.
func TestEvaluate_BodyTest_DepthWithinCapStillMatches(t *testing.T) {
	const marker = "reachableneedle"
	email := sieveEmail()
	email.Body = deepBodyChain(maxBodyDepth-1, marker)

	script := `require ["body", "fileinto"];
if body :contains "` + marker + `" { fileinto "Reachable"; }`
	r := runSieve(t, script, email)

	if folderOf(r) != "Reachable" {
		t.Fatalf("body content nested within the depth cap should still match, got folder=%q", folderOf(r))
	}
}

// TestEvaluate_BodyTest_PartCountBounded proves the total-parts budget. The only
// matching part sits past maxBodyParts siblings in a single (shallow) multipart:
// an unbounded walk visits every sibling and matches; the budgeted walk stops
// once the cap is spent and does not.
func TestEvaluate_BodyTest_PartCountBounded(t *testing.T) {
	const marker = "beyondbudgetneedle"
	email := sieveEmail()
	email.Body.ContentType = "multipart/mixed"
	email.Body.Content = ""
	parts := make([]Body, 0, maxBodyParts+1)
	for i := 0; i < maxBodyParts; i++ {
		// Non-matching filler parts that consume the budget.
		parts = append(parts, Body{ContentType: "application/octet-stream", Content: "x"})
	}
	parts = append(parts, Body{ContentType: "text/plain", Content: marker})
	email.Body.Parts = parts

	script := `require ["body", "fileinto"];
if body :contains "` + marker + `" { fileinto "TooMany"; }`
	r := runSieve(t, script, email)

	if folderOf(r) == "TooMany" {
		t.Fatalf("body test reached a part past the %d-part budget; traversal is unbounded", maxBodyParts)
	}
}

// TestEvaluate_SizeTest_BodyDepthBounded proves the size test's body-octet
// reconstruction (bodySize) is likewise depth-bounded. A large payload buried
// past the cap is counted by the old unbounded sum (so :over fires) but not by
// the bounded walk (so it does not).
func TestEvaluate_SizeTest_BodyDepthBounded(t *testing.T) {
	payload := make([]byte, 4000)
	for i := range payload {
		payload[i] = 'a'
	}
	email := sieveEmail() // RawSize unset -> size is reconstructed
	email.Body = deepBodyChain(maxBodyDepth+50, string(payload))

	// The reconstructed header block is well under 1000 octets, so :over 1000
	// can only fire if the 4000-octet leaf beyond the cap is counted.
	script := `require ["fileinto"];
if size :over 1000 { fileinto "Big"; }`
	r := runSieve(t, script, email)

	if folderOf(r) == "Big" {
		t.Fatalf("size test summed body content buried past the depth cap; bodySize traversal is unbounded")
	}
}
