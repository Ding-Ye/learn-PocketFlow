package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// Test 1: positive branch.
func TestRouting_PositiveBranch(t *testing.T) {
	check := NewCheckPositiveNode()
	add3 := NewAdd3Node()
	sub3 := NewSubtract3Node()
	check.NextOn("positive", add3)
	check.NextOn("negative", sub3)

	flow := NewFlow(check)
	shared := SharedStore{"current": 5}
	flow.Run(shared)

	if got := shared["current"]; got != 5+3 {
		t.Errorf("expected 8 after add3 branch, got %v", got)
	}
}

// Test 2: negative branch.
func TestRouting_NegativeBranch(t *testing.T) {
	check := NewCheckPositiveNode()
	add3 := NewAdd3Node()
	sub3 := NewSubtract3Node()
	check.NextOn("positive", add3)
	check.NextOn("negative", sub3)

	flow := NewFlow(check)
	shared := SharedStore{"current": -2}
	flow.Run(shared)

	if got := shared["current"]; got != -2-3 {
		t.Errorf("expected -5 after subtract3 branch, got %v", got)
	}
}

// Test 3: unknown action with non-empty successors logs a warning AND terminates.
func TestRouting_UnknownActionWarns(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	check := NewCheckPositiveNode()
	check.NextOn("positive", NewAdd3Node())
	// "negative" not registered — Post will return "negative" but lookup fails.

	flow := NewFlow(check)
	shared := SharedStore{"current": -1}
	last := flow.Run(shared)

	out := buf.String()
	if !strings.Contains(out, "Flow ends:") {
		t.Errorf("expected unknown-action warning, got %q", out)
	}
	if last != "negative" {
		t.Errorf("expected last=negative even when terminating, got %q", last)
	}
}

// Test 4: leaf node (no successors) terminates silently — no warning.
func TestRouting_LeafTerminatesSilently(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	flow := NewFlow(NewAdd3Node()) // single node, no successors

	shared := SharedStore{"current": 0}
	flow.Run(shared)

	if strings.Contains(buf.String(), "[warn]") {
		t.Errorf("expected no warning on leaf termination, got %q", buf.String())
	}
}

// Test 5: loop-back works — FinishedNode loops 3 times then exits via "finished".
func TestRouting_LoopBackTerminates(t *testing.T) {
	check := NewCheckPositiveNode()
	add3 := NewAdd3Node()
	sub3 := NewSubtract3Node()
	finished := NewFinishedNode()

	check.NextOn("positive", add3)
	check.NextOn("negative", sub3)
	add3.NextOn("default", finished)
	sub3.NextOn("default", finished)
	finished.NextOn("loop", check)
	// finished returns "finished" on the 3rd iteration → no successor → exits.

	flow := NewFlow(check)
	shared := SharedStore{"current": 0}
	last := flow.Run(shared)

	if finished.count != 3 {
		t.Errorf("expected 3 loop iterations, got %d", finished.count)
	}
	if last != "finished" {
		t.Errorf("expected last=finished, got %q", last)
	}
	if got := shared["current"]; got != 9 { // 0 → 3 → 6 → 9 (positive branch all three times)
		t.Errorf("expected current=9, got %v", got)
	}
}

// Test 6: empty action string still routes via "default".
func TestRouting_EmptyActionFallsBack(t *testing.T) {
	type emptyActionNode struct{ *BaseNode }
	n := &emptyActionNode{BaseNode: NewBaseNode()}
	// override Post to return ""
	_ = n
	// Use stock BaseNode whose Post returns "default" — equivalent for this test.
	parent := NewBaseNode()
	child := NewAdd3Node()
	parent.NextOn("default", child)

	// Simulate the lookup directly:
	got := getNextNode(struct {
		*BaseNode
	}{parent}, "")
	if got != Node(child) {
		t.Errorf("expected empty action to resolve to default successor")
	}
}
