package main

import (
	"testing"
)

// Test 1: Flow walks all three nodes in order; shared accumulates results.
func TestFlow_WalksAllNodes(t *testing.T) {
	greet := NewGreetNode()
	upper := NewUppercaseNode()
	count := NewCountNode()
	greet.Next(upper)
	upper.Next(count)

	flow := NewFlow(greet)
	shared := SharedStore{"name": "Go"}
	flow.Orchestrate(shared, map[string]any{"greeting": "Hi"})

	msg, _ := shared["msg"].(string)
	if msg != "HI, GO!" {
		t.Errorf("expected HI, GO!, got %q", msg)
	}
	if l := shared["len"]; l != 7 {
		t.Errorf("expected len=7, got %v", l)
	}
}

// Test 2: Single-node flow terminates after one step.
func TestFlow_SingleNode(t *testing.T) {
	flow := NewFlow(NewGreetNode())
	shared := SharedStore{"name": "x"}
	last := flow.Orchestrate(shared, map[string]any{"greeting": "Hey"})

	if msg, _ := shared["msg"].(string); msg != "Hey, x!" {
		t.Errorf("expected greeting written, got %q", msg)
	}
	if last != "default" {
		t.Errorf("expected last=default, got %q", last)
	}
}

// Test 3: mergeParams — overrides win, base preserved where override missing.
func TestMergeParams(t *testing.T) {
	base := map[string]any{"a": 1, "b": 2}
	over := map[string]any{"b": 99, "c": 3}
	out := mergeParams(base, over)

	if out["a"] != 1 {
		t.Errorf("expected a=1 preserved, got %v", out["a"])
	}
	if out["b"] != 99 {
		t.Errorf("expected b overridden to 99, got %v", out["b"])
	}
	if out["c"] != 3 {
		t.Errorf("expected c=3 added, got %v", out["c"])
	}
}

// Test 4: per-iteration params override flow.Params.
func TestFlow_ParamsOverride(t *testing.T) {
	greet := NewGreetNode()
	flow := NewFlow(greet)
	flow.Params = map[string]any{"greeting": "Hello"}

	shared := SharedStore{"name": "x"}
	flow.Orchestrate(shared, map[string]any{"greeting": "OVERRIDE"})

	if msg, _ := shared["msg"].(string); msg != "OVERRIDE, x!" {
		t.Errorf("expected per-iteration override, got %q", msg)
	}
}

// Test 5: Flow returns last action returned by the final Post.
func TestFlow_ReturnsLastAction(t *testing.T) {
	greet := NewGreetNode()
	greet.Next(NewCountNode())

	flow := NewFlow(greet)
	shared := SharedStore{"name": "x"}
	last := flow.Orchestrate(shared, nil)

	if last != "default" {
		t.Errorf("expected last=default, got %q", last)
	}
}

// Test 6 (bonus): Flow.Run is a thin wrapper that uses flow.Params as defaults.
func TestFlow_RunUsesFlowParams(t *testing.T) {
	greet := NewGreetNode()
	flow := NewFlow(greet)
	flow.Params = map[string]any{"greeting": "Yo"}

	shared := SharedStore{"name": "world"}
	flow.Run(shared)

	if msg, _ := shared["msg"].(string); msg != "Yo, world!" {
		t.Errorf("expected Run to use flow.Params, got %q", msg)
	}
}
