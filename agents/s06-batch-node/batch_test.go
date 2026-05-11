package main

import (
	"testing"
)

// Test 1: empty prep slice → empty result slice.
func TestBatch_EmptyList(t *testing.T) {
	n := NewPermBadNode()
	shared := SharedStore{"items": []any{}}
	RunBatch(n, shared)
	got, _ := shared["out"].([]any)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

// Test 2: 3 items → 3 results, order preserved.
func TestBatch_PreservesOrder(t *testing.T) {
	n := NewPermBadNode()
	shared := SharedStore{"items": []any{"a", "b", "c"}}
	RunBatch(n, shared)
	got, _ := shared["out"].([]any)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(got), got)
	}
	for i, want := range []string{"a-ok", "b-ok", "c-ok"} {
		if got[i] != want {
			t.Errorf("item %d: want %q got %v", i, want, got[i])
		}
	}
}

// Test 3: per-item retry — French fails once, recovers; others pass first try.
func TestBatch_PerItemRetry(t *testing.T) {
	tr := NewTranslateNode()
	shared := SharedStore{
		"text":      "hi",
		"languages": []string{"es", "fr", "de"},
	}
	RunBatch(tr, shared)
	got, _ := shared["translations"].([]any)
	if len(got) != 3 {
		t.Fatalf("expected 3 translations, got %d", len(got))
	}
	for _, v := range got {
		s, _ := v.(string)
		if s == "" {
			t.Errorf("got empty translation: %v", v)
		}
		if len(s) > 9 && s[:9] == "FALLBACK:" {
			t.Errorf("expected all to succeed after retry, got fallback: %v", s)
		}
	}
}

// Test 4: per-item fallback — bad item gets ITEM-FALLBACK, others succeed.
func TestBatch_PerItemFallback(t *testing.T) {
	n := NewPermBadNode()
	shared := SharedStore{"items": []any{"en", "es", "fr"}}
	RunBatch(n, shared)
	got, _ := shared["out"].([]any)
	if got[0] != "en-ok" || got[1] != "ITEM-FALLBACK" || got[2] != "fr-ok" {
		t.Errorf("expected en-ok / ITEM-FALLBACK / fr-ok, got %v", got)
	}
}

// Test 5: Post receives the full slice of per-item results.
func TestBatch_PostReceivesSlice(t *testing.T) {
	tr := NewTranslateNode()
	shared := SharedStore{"text": "yo", "languages": []string{"es", "de"}}
	RunBatch(tr, shared)
	out, ok := shared["translations"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", shared["translations"])
	}
	if len(out) != 2 {
		t.Errorf("expected 2 results, got %d", len(out))
	}
}

// Test 6: nil prep result is safe (no panic).
func TestBatch_NilPrepIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("expected no panic on nil prep, got %v", r)
		}
	}()
	n := NewPermBadNode()
	shared := SharedStore{} // items missing
	RunBatch(n, shared)
}
