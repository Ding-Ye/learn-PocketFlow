package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// Test 1: Next stores the successor under the "default" action.
func TestNext_StoresDefault(t *testing.T) {
	parent := NewLoadNode()
	child := NewCapsNode()

	parent.Next(child)

	got, ok := parent.GetSuccessors()["default"]
	if !ok {
		t.Fatalf("expected 'default' key in successors, got: %v", parent.GetSuccessors())
	}
	if got != child {
		t.Errorf("expected child as default successor, got different node")
	}
}

// Test 2: NextOn stores the successor under the named action.
func TestNextOn_StoresAction(t *testing.T) {
	parent := NewCheckEmptyNode()
	child := NewLoadNode()

	parent.NextOn("empty", child)

	if got := parent.GetSuccessors()["empty"]; got != child {
		t.Errorf("expected child under 'empty' action, got %v", got)
	}
	if got := parent.GetSuccessors()["default"]; got != nil {
		t.Errorf("expected no 'default' successor, got %v", got)
	}
}

// Test 3: Overwriting a successor logs a warning.
func TestNextOn_WarnsOnOverwrite(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	parent := NewCheckEmptyNode()
	parent.NextOn("yes", NewLoadNode())
	parent.NextOn("yes", NewCapsNode()) // overwrite

	out := buf.String()
	if !strings.Contains(out, "overwriting successor") {
		t.Errorf("expected overwrite warning, got %q", out)
	}
}

// Test 4: Default state has empty successors map.
func TestBaseNode_EmptySuccessorsByDefault(t *testing.T) {
	n := NewLoadNode()
	if got := n.GetSuccessors(); len(got) != 0 {
		t.Errorf("expected empty successors, got %d entries", len(got))
	}
}

// Test 5: Chain returns the appended node so callers can fluent-chain.
func TestNext_ReturnsAppended_ForFluentChaining(t *testing.T) {
	a := NewLoadNode()
	b := NewCapsNode()
	c := NewLoadNode()

	got := a.Next(b).(*CapsNode).Next(c)

	if got != c {
		t.Errorf("expected returned node to be the last appended (c)")
	}
	if a.GetSuccessors()["default"] != b {
		t.Errorf("expected a->b on default")
	}
	if b.GetSuccessors()["default"] != c {
		t.Errorf("expected b->c on default")
	}
}

// Test 6 (bonus): RunOnce on a chained node warns but still runs prep/exec/post.
func TestRunOnce_WarnsButRuns(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	parent := NewLoadNode()
	parent.Next(NewCapsNode())

	shared := SharedStore{"raw": "abc"}
	RunOnce(parent, shared)

	if got := shared["tokens"]; got == nil {
		t.Errorf("expected shared[tokens] populated despite successors warning")
	}
	if !strings.Contains(buf.String(), "Node has successors") {
		t.Errorf("expected 'successors' warning, got %q", buf.String())
	}
}
