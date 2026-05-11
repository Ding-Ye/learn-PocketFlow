---
title: "s05 · Retry & fallback"
chapter: 5
slug: s05-retry-fallback
est_read_min: 8
---

# s05 · Retry & fallback

> What this teaches: production-grade exec. Wrap the work in a retry loop with `MaxRetries`, optional `Wait` between attempts, and an `ExecFallback` for terminal failure.

---

## Problem

s01-s04 used `Exec(prepRes) any` — no error in the signature. That's fine for arithmetic; useless for LLM calls and HTTP fetches that fail transiently. Upstream's solution is the `Node` class (line 26-34 of `pocketflow/__init__.py`), which extends `BaseNode` with three pieces of state — `max_retries`, `wait`, `cur_retry` — and an overrideable `exec_fallback` method that decides what to return when all attempts fail.

We need the same in Go, with one twist: upstream stashes `cur_retry` on `self` (the node), then `copy.copy(node)`s the instance before each flow run so the counter doesn't leak. That's the *only* reason for upstream's per-run shallow copy. If we move the counter to a local variable, we delete the entire copying step from `Flow._orch`.

## Solution

Add a `RetryNode` struct embedding `*BaseNode`, plus a `RetryableNode` interface with two methods:

```go
type RetryableNode interface {
    TryExec(prepRes any) (any, error)
    ExecFallback(prepRes any, err error) any
}
```

`RunWithRetry(node, shared)` detects via type assertion whether the node implements `RetryableNode` and routes to the retry loop. Non-retryable nodes (plain `BaseNode` users from s01-s04) take the existing `Exec` path unchanged — backward-compatible by design.

Three design decisions:

1. **`cur_retry` lives in `retryExec`'s local scope** — explicit divergence from upstream. Same semantics, no per-run copy needed.
2. **`TryExec` returns `(any, error)`, `Exec` does not** — keeps the s01-s04 demos compilable; you only opt into errors by embedding `RetryNode` and implementing `TryExec`.
3. **`ExecFallback` is required** — it has no default. Upstream's default `exec_fallback` is `raise exc` (re-raise the last exception). We force the embedder to think about it; the most common implementation is `return nil, err` or a sentinel value.

## How It Works

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────────┐
│                       RunWithRetry(node, shared)                │
│                                                                 │
│   prepRes = node.Prep(shared)                                   │
│   if node is RetryableNode:                                     │
│       for attempt = 0 .. MaxRetries-1:                          │
│           out, err = node.TryExec(prepRes)                      │
│           if err == nil: return Post(shared, prepRes, out)      │
│           if attempt == last:                                   │
│               return Post(shared, prepRes, ExecFallback(prep,err))│
│           if Wait > 0: time.Sleep(Wait)                         │
│   else:                                                         │
│       out = node.Exec(prepRes)            // s01-s04 path        │
│       return node.Post(shared, prepRes, out)                    │
└────────────────────────────────────────────────────────────────┘
```

Core 22 lines (from [`agents/s05-retry-fallback/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s05-retry-fallback/main.go)):

```go
func retryExec(maxRetries int, wait time.Duration, prepRes any,
    try func(any) (any, error),
    fallback func(any, error) any,
) any {
    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        out, err := try(prepRes)
        if err == nil {
            return out
        }
        lastErr = err
        if attempt == maxRetries-1 {
            return fallback(prepRes, err)
        }
        if wait > 0 { time.Sleep(wait) }
    }
    return fallback(prepRes, lastErr)
}
```

**3 non-obvious points**:

1. **`cur_retry` is a stack variable (`attempt`)** — no `node.CurRetry` field. This is the *deliberate divergence* from upstream. It means the same `RetryNode` can be reused across flow runs without state-leak risk; consequently, the Flow doesn't need to `copy.copy` nodes anymore.
2. **`MaxRetries=1` is "try once, no retry"** — upstream's default. The loop is `range(max_retries)` so `max_retries=1` runs once and skips straight to the "is this the last attempt?" branch (yes) → fallback if it failed, or return the success.
3. **`time.Sleep` happens BEFORE the next attempt, not after the last one** — the `if attempt == maxRetries-1` check executes first; only if there's *another attempt remaining* do we sleep. Saves the wasted sleep at the end.

## What Changed (vs. s04)

```diff
+type RetryableNode interface {
+    TryExec(prepRes any) (any, error)
+    ExecFallback(prepRes any, err error) any
+}
+
+type RetryNode struct {
+    *BaseNode
+    MaxRetries int
+    Wait       time.Duration
+}
+
+func NewRetryNode(maxRetries int, wait time.Duration) *RetryNode { ... }
+
+func RunWithRetry(n Node, shared SharedStore) string { ... }
+func retryExec(maxRetries int, wait time.Duration, prepRes any,
+    try func(any) (any, error),
+    fallback func(any, error) any) any { ... }
```

The Node interface and `Flow.Orchestrate` are unchanged. A node only enters the retry path if it embeds `*RetryNode` AND its concrete type satisfies `RetryableNode` (i.e. it provides `TryExec` + `ExecFallback`). Otherwise it uses the s01-s04 `Exec` path.

## Try It

```bash
cd agents/s05-retry-fallback
go run .
go test -v ./...
```

Expected output:

```
============================================================
flaky calls = 3 (expected 3)
body = OK 126-byte response from https://example.com
------------------------------------------------------------
hard calls  = 3 (expected 3)
result = FALLBACK-VALUE (fallback)
============================================================
```

## Upstream Source Reading

```upstream:pocketflow/__init__.py#L26-L34
# Source: pocketflow/__init__.py#L26-L34
class Node(BaseNode):
    def __init__(self,max_retries=1,wait=0):
        super().__init__()
        self.max_retries,self.wait=max_retries,wait
    def exec_fallback(self,prep_res,exc): raise exc
    def _exec(self,prep_res):
        for self.cur_retry in range(self.max_retries):
            try: return self.exec(prep_res)
            except Exception as e:
                if self.cur_retry==self.max_retries-1:
                    return self.exec_fallback(prep_res,e)
                if self.wait>0: time.sleep(self.wait)
```

**Reading notes**:

- **`self.cur_retry = ...` inside the for-loop**: upstream writes the loop variable to the instance. So if you `print(node.cur_retry)` inside `exec`, you see the current attempt number. We don't expose this in Go; if you need it, pass it through `prepRes` or accept it as a `TryExec` arg in a richer interface.
- **`def exec_fallback(self,prep_res,exc): raise exc`**: upstream's default re-raises. The user is expected to override. Our Go port doesn't ship a default — `RetryableNode` doesn't extend an abstract `RetryNode` that auto-rethrows, so the embedder must implement `ExecFallback` explicitly. Strictly more verbose, more discoverable.
- **`time.sleep(self.wait)` is `time.Sleep(self.Wait)`**: Go's `time.Sleep` takes a `time.Duration`; upstream takes float seconds. We choose `time.Duration` for type safety; `NewRetryNode(3, 100*time.Millisecond)` is clearer than `NewRetryNode(3, 0.1)`.
- **The retry loop is the whole upgrade from BaseNode**: lines 28-34 are the entire delta. Everything else — `next()`, successors, prep/post — comes from `BaseNode` via inheritance.
- **Why `copy.copy(node)` in `Flow._orch`?** It exists *only* to reset `cur_retry` between flow runs. If you removed `cur_retry` from the instance (as we did), you could remove the copy. Upstream keeps both for symmetry with cookbook patterns that occasionally read `cur_retry` post-mortem.

**Read further**: line 36-37 ports as s06. `BatchNode` is a one-liner that overrides `_exec` to iterate over a list — inheriting the entire retry loop for free, applied per-item. The composition is elegant.

---

**Next**: s06 turns `RetryNode` into `BatchNode` by overriding `TryExec` to iterate a list — each item gets the full retry-with-fallback semantics independently.
