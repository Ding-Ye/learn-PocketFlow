package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// Test 1: AnswerNode reads `question` from shared and writes `answer` back.
func TestAnswerNode_ReadsAndWrites(t *testing.T) {
	shared := SharedStore{"question": "Hello?"}
	node := NewAnswerNode()

	RunOnce(node, shared)

	got, ok := shared["answer"].(string)
	if !ok {
		t.Fatalf("shared[answer] not a string, got %T", shared["answer"])
	}
	if !strings.Contains(got, "Hello?") {
		t.Errorf("expected answer to mention the question, got %q", got)
	}
}

// Test 2: Post returns "default" as the action.
func TestAnswerNode_PostReturnsDefault(t *testing.T) {
	shared := SharedStore{"question": "x"}
	node := NewAnswerNode()

	action := RunOnce(node, shared)

	if action != "default" {
		t.Errorf("expected action %q, got %q", "default", action)
	}
}

// Test 3: prep → exec → post call order is preserved. We trace via a
// minimal recording node that doesn't depend on AnswerNode internals.
type recordingNode struct {
	*BaseNode
	calls []string
}

func newRecordingNode() *recordingNode { return &recordingNode{BaseNode: NewBaseNode()} }

func (r *recordingNode) Prep(shared SharedStore) any { r.calls = append(r.calls, "prep"); return "p" }
func (r *recordingNode) Exec(prepRes any) any        { r.calls = append(r.calls, "exec"); return "e" }
func (r *recordingNode) Post(shared SharedStore, prepRes, execRes any) string {
	r.calls = append(r.calls, "post")
	return "default"
}

func TestRunOnce_LifecycleOrder(t *testing.T) {
	rec := newRecordingNode()
	RunOnce(rec, SharedStore{})

	want := []string{"prep", "exec", "post"}
	if len(rec.calls) != 3 {
		t.Fatalf("expected 3 lifecycle calls, got %v", rec.calls)
	}
	for i, c := range rec.calls {
		if c != want[i] {
			t.Errorf("step %d: want %q got %q", i, want[i], c)
		}
	}
}

// Test 4: Shared mutation survives after RunOnce returns (pass-by-reference).
func TestSharedStore_MutationVisible(t *testing.T) {
	shared := SharedStore{"counter": 0}
	mutator := &mutatingNode{BaseNode: NewBaseNode()}
	RunOnce(mutator, shared)

	if got := shared["counter"]; got != 1 {
		t.Errorf("expected shared[counter]=1, got %v", got)
	}
}

type mutatingNode struct {
	*BaseNode
}

func (m *mutatingNode) Post(shared SharedStore, prepRes, execRes any) string {
	c, _ := shared["counter"].(int)
	shared["counter"] = c + 1
	return "default"
}

// Test 5: RunOnce emits a warning when the node has successors registered.
// Upstream emits warnings.warn("Node won't run successors. Use Flow."), so we
// log to a captured logger and assert the substring.
func TestRunOnce_WarnsOnSuccessors(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	parent := NewAnswerNode()
	child := NewAnswerNode()
	parent.Successors["default"] = child // raw map mutation; s02 adds Next() API

	shared := SharedStore{"question": "?"}
	RunOnce(parent, shared)

	out := buf.String()
	if !strings.Contains(out, "successors") {
		t.Errorf("expected warning containing 'successors', got %q", out)
	}
}
