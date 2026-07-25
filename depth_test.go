package sieve

import (
	"strings"
	"testing"
)

// nestedNotScript builds `not not ... true` with depth "not" wrappers around a
// single true test, all inside an `if ... { keep; }` so it parses as a script.
func nestedNotScript(depth int) string {
	var b strings.Builder
	b.WriteString("if ")
	for i := 0; i < depth; i++ {
		b.WriteString("not ")
	}
	b.WriteString("true { keep; }")
	return b.String()
}

// nestedAnyofScript builds anyof(anyof(...(true)...)) nested depth deep.
func nestedAnyofScript(depth int) string {
	var b strings.Builder
	b.WriteString("if ")
	for i := 0; i < depth; i++ {
		b.WriteString("anyof(")
	}
	b.WriteString("true")
	for i := 0; i < depth; i++ {
		b.WriteString(")")
	}
	b.WriteString(" { keep; }")
	return b.String()
}

// nestedBlockScript builds `if true { if true { ... keep; ... } }` nested depth
// deep.
func nestedBlockScript(depth int) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteString("if true { ")
	}
	b.WriteString("keep;")
	for i := 0; i < depth; i++ {
		b.WriteString(" }")
	}
	return b.String()
}

// TestParse_RejectsDeeplyNestedTests proves that a test expression nested far
// beyond DefaultMaxDepth is rejected at parse time rather than parsed into an
// unbounded AST (the DoS in issue #6).
func TestParse_RejectsDeeplyNestedTests(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"not chain", nestedNotScript(DefaultMaxDepth + 50)},
		{"anyof chain", nestedAnyofScript(DefaultMaxDepth + 50)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.src)
			if err == nil {
				t.Fatalf("expected a depth-limit error, got nil")
			}
			if !strings.Contains(err.Error(), "nesting too deep") {
				t.Fatalf("expected a nesting error, got: %v", err)
			}
		})
	}
}

// TestParse_RejectsDeeplyNestedBlocks proves nested control blocks are bounded.
func TestParse_RejectsDeeplyNestedBlocks(t *testing.T) {
	_, err := Parse(nestedBlockScript(DefaultMaxDepth + 50))
	if err == nil {
		t.Fatalf("expected a depth-limit error, got nil")
	}
	if !strings.Contains(err.Error(), "nesting too deep") {
		t.Fatalf("expected a nesting error, got: %v", err)
	}
}

// TestParse_AcceptsNestingUpToLimit confirms the cap does not reject scripts at
// or below the limit (no false positives for legitimately deep scripts).
func TestParse_AcceptsNestingUpToLimit(t *testing.T) {
	// DefaultMaxDepth-1 "not" wrappers plus the inner "true" == DefaultMaxDepth
	// test-parse levels, which is exactly at the limit.
	if _, err := Parse(nestedNotScript(DefaultMaxDepth - 1)); err != nil {
		t.Fatalf("script at the depth limit was rejected: %v", err)
	}
	if _, err := Parse(nestedBlockScript(DefaultMaxDepth)); err != nil {
		t.Fatalf("blocks at the depth limit were rejected: %v", err)
	}
}

// TestParse_WithMaxDepth checks the limit is configurable.
func TestParse_WithMaxDepth(t *testing.T) {
	src := nestedNotScript(20)
	if _, err := Parse(src, WithMaxDepth(5)); err == nil {
		t.Fatalf("expected rejection with a tight limit, got nil")
	}
	if _, err := Parse(src, WithMaxDepth(100)); err != nil {
		t.Fatalf("expected acceptance with a generous limit, got: %v", err)
	}
	// A non-positive value falls back to the default.
	if _, err := Parse(nestedNotScript(DefaultMaxDepth-1), WithMaxDepth(0)); err != nil {
		t.Fatalf("WithMaxDepth(0) should fall back to the default: %v", err)
	}
}

// TestParse_DeeplyNestedDoesNotCrash is the DoS regression test: a script with
// millions of nested tests must return an error, not overflow the goroutine
// stack and crash the process. On the unfixed parser this recurses without
// bound and aborts the test binary with a fatal stack-overflow.
func TestParse_DeeplyNestedDoesNotCrash(t *testing.T) {
	const huge = 2_000_000
	if _, err := Parse(nestedNotScript(huge)); err == nil {
		t.Fatalf("expected a depth-limit error for a %d-deep script, got nil", huge)
	}
	if _, err := Parse(nestedBlockScript(huge)); err == nil {
		t.Fatalf("expected a depth-limit error for %d-deep blocks, got nil", huge)
	}
}
