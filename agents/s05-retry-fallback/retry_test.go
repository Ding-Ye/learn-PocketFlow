package main

import (
	"errors"
	"testing"
	"time"
)

// Test 1: success on first try → no retry, no fallback.
func TestRetry_SuccessOnFirstTry(t *testing.T) {
	calls := 0
	out := retryExec(3, 0, "in",
		func(p any) (any, error) {
			calls++
			return "ok", nil
		},
		func(p any, err error) any { t.Fatal("fallback should not run"); return nil },
	)
	if out != "ok" || calls != 1 {
		t.Errorf("expected ok + 1 call, got %v + %d", out, calls)
	}
}

// Test 2: success on second try → 2 calls, no fallback.
func TestRetry_SuccessOnSecondTry(t *testing.T) {
	calls := 0
	out := retryExec(3, 0, "in",
		func(p any) (any, error) {
			calls++
			if calls < 2 {
				return nil, errors.New("transient")
			}
			return "ok", nil
		},
		func(p any, err error) any { t.Fatal("fallback should not run"); return nil },
	)
	if out != "ok" || calls != 2 {
		t.Errorf("expected ok + 2 calls, got %v + %d", out, calls)
	}
}

// Test 3: all retries fail → fallback called with the LAST error.
func TestRetry_FallbackOnAllFailure(t *testing.T) {
	calls := 0
	var fallbackErr error
	out := retryExec(3, 0, "in",
		func(p any) (any, error) {
			calls++
			return nil, errors.New("err")
		},
		func(p any, err error) any { fallbackErr = err; return "fallback" },
	)
	if out != "fallback" {
		t.Errorf("expected fallback, got %v", out)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
	if fallbackErr == nil || fallbackErr.Error() != "err" {
		t.Errorf("expected fallback to receive 'err', got %v", fallbackErr)
	}
}

// Test 4: Wait > 0 actually sleeps between attempts.
func TestRetry_WaitBetweenAttempts(t *testing.T) {
	start := time.Now()
	calls := 0
	retryExec(3, 50*time.Millisecond, "in",
		func(p any) (any, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("e")
			}
			return "ok", nil
		},
		func(p any, err error) any { return "f" },
	)
	elapsed := time.Since(start)
	// 2 retries × 50ms sleep = 100ms minimum.
	if elapsed < 90*time.Millisecond {
		t.Errorf("expected ≥90ms elapsed (2 waits), got %v", elapsed)
	}
}

// Test 5: cur_retry doesn't leak across runs — same node, two RunWithRetry calls.
func TestRetry_NoStateLeakAcrossRuns(t *testing.T) {
	flaky := NewFlakyHTTPNode()
	shared := SharedStore{"url": "u1"}
	RunWithRetry(flaky, shared)
	c1 := flaky.calls // should be 3
	RunWithRetry(flaky, shared)
	c2 := flaky.calls // total across both runs: 3 + 1 (succeeds on first try now)
	if c1 != 3 {
		t.Errorf("expected first run = 3 calls, got %d", c1)
	}
	// The second run starts with the same node so calls counter continues from 3 — but the
	// 3rd attempt-or-later branch hits OK immediately. We expect total 3 + 1 = 4.
	if c2 != 4 {
		t.Errorf("expected total calls after second run = 4, got %d", c2)
	}
}

// Test 6: hard-failing node returns ExecFallback's sentinel via Post.
func TestRetry_HardFailingFallback(t *testing.T) {
	hard := NewHardFailingNode()
	shared := SharedStore{}
	RunWithRetry(hard, shared)
	if shared["result"] != "FALLBACK-VALUE" {
		t.Errorf("expected fallback sentinel, got %v", shared["result"])
	}
	if hard.calls != 3 {
		t.Errorf("expected 3 attempts, got %d", hard.calls)
	}
}
