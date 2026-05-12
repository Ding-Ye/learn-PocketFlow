package main

import (
	"testing"
)

// Test 1: 3 param dicts → 3 full flow runs.
func TestBatchFlow_RunsOncePerDict(t *testing.T) {
	flow := NewImageProcessFlow()
	shared := SharedStore{
		"images":  []string{"a", "b", "c"},
		"filters": []string{"sepia"},
		"log":     []string{},
	}
	RunBatchFlow(flow.BatchFlow, flow, shared)

	log := shared["log"].([]string)
	if len(log) != 3 {
		t.Errorf("expected 3 flow runs, got %d", len(log))
	}
}

// Test 2: shared accumulates across iterations.
func TestBatchFlow_SharedAccumulates(t *testing.T) {
	flow := NewImageProcessFlow()
	shared := SharedStore{
		"images":  []string{"x", "y"},
		"filters": []string{"f1", "f2"},
		"log":     []string{},
	}
	RunBatchFlow(flow.BatchFlow, flow, shared)

	log := shared["log"].([]string)
	if len(log) != 4 { // 2 images × 2 filters
		t.Errorf("expected 4 entries, got %d: %v", len(log), log)
	}
}

// Test 3: per-iteration params override flow.Params.
func TestBatchFlow_ParamsOverrideFlowParams(t *testing.T) {
	flow := NewImageProcessFlow()
	flow.Params = map[string]any{"filter": "BASE-FILTER"}

	shared := SharedStore{
		"images":  []string{"img1"},
		"filters": []string{"OVERRIDE"},
		"log":     []string{},
	}
	RunBatchFlow(flow.BatchFlow, flow, shared)

	log := shared["log"].([]string)
	if len(log) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(log))
	}
	if !contains(log[0], "OVERRIDE") {
		t.Errorf("expected per-iteration OVERRIDE to win, got %q", log[0])
	}
}

// Test 4: empty params list → no iterations, no log entries.
func TestBatchFlow_EmptyPrepNoIterations(t *testing.T) {
	flow := NewImageProcessFlow()
	shared := SharedStore{
		"images":  []string{},
		"filters": []string{},
		"log":     []string{},
	}
	RunBatchFlow(flow.BatchFlow, flow, shared)

	if got := shared["batches_run"]; got != 0 {
		t.Errorf("expected 0 batches, got %v", got)
	}
}

// Test 5: BatchFlow.Post is called once after all iterations.
func TestBatchFlow_PostCalledOnce(t *testing.T) {
	flow := NewImageProcessFlow()
	shared := SharedStore{
		"images":  []string{"a", "b"},
		"filters": []string{"f"},
		"log":     []string{},
	}
	RunBatchFlow(flow.BatchFlow, flow, shared)

	if got := shared["batches_run"]; got != 2 {
		t.Errorf("expected batches_run=2 (set by Post), got %v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
