---
title: "s06 · BatchNode (map pattern)"
chapter: 6
slug: s06-batch-node
est_read_min: 7
---

# s06 · BatchNode (map pattern)

> What this teaches: turn `RetryNode` into a map operator. `Prep` returns a list; the runner applies the retry-with-fallback loop once per item; `Post` receives a slice of results.

---

## Problem

s05 handled "call this once, retry on failure". But agents constantly batch:
- "Translate this paragraph to ES / FR / DE / ZH"
- "Embed these 200 document chunks"
- "Summarise these 50 GitHub issues"

You could write a `for` loop inside `Exec`, but you'd lose per-item retry and fallback. If item #3 fails, naïve looping either crashes the whole batch or silently skips it.

PocketFlow's `BatchNode` (upstream `pocketflow/__init__.py#L36-L37`) is a **one-line** subclass: it overrides `_exec` to iterate, calling `super()._exec(item)` per element — which inherits the entire `Node`-level retry loop *per item*. Elegant.

## Solution

A `BatchItemNode` interface with `TryExecItem(item) (any, error)` + `ExecFallbackItem(item, err) any`. `BatchNode` embeds `RetryNode`. The runner `RunBatch(n, shared)` does:

1. Get the slice from `Prep`.
2. For each item: call `retryExec(MaxRetries, Wait, item, TryExecItem, ExecFallbackItem)` — same `retryExec` from s05.
3. Collect into `[]any`.
4. Pass the slice to `Post`.

Three design decisions:

1. **`MaxRetries` and `Wait` apply *per item*** — if one item is flaky, only it retries; others go through cleanly.
2. **Per-item fallback returns a value, not an error** — failures don't abort the batch. The fallback's return goes into the result slice, so post can still process all items uniformly.
3. **Prep returns `[]any`, not generic types** — Go generics would require type parameters on the interface, which complicates the Node contract. We trade type safety for interface simplicity; users type-assert inside `TryExecItem`.

## How It Works

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────────────┐
│                          RunBatch (s06)                              │
│                                                                     │
│   items := Prep(shared)        // returns []any                     │
│   results := []any{}                                                │
│   for _, item := range items {                                      │
│       r := retryExec(MaxRetries, Wait, item,                        │
│                      TryExecItem, ExecFallbackItem)                 │
│       results = append(results, r)                                  │
│   }                                                                 │
│   return Post(shared, items, results)                               │
└────────────────────────────────────────────────────────────────────┘
```

Core 18 lines (from [`agents/s06-batch-node/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s06-batch-node/main.go)):

```go
func RunBatch(n Node, shared SharedStore) string {
    prepRes := n.Prep(shared)
    items, _ := prepRes.([]any)

    results := make([]any, 0, len(items))
    if b, ok := n.(BatchItemNode); ok {
        maxRetries := n.(interface{ GetMaxRetries() int }).GetMaxRetries()
        wait := n.(interface{ GetWait() time.Duration }).GetWait()
        for _, item := range items {
            item := item
            r := retryExec(maxRetries, wait, item,
                func(p any) (any, error) { return b.TryExecItem(p) },
                func(p any, err error) any { return b.ExecFallbackItem(p, err) },
            )
            results = append(results, r)
        }
    }
    return n.Post(shared, prepRes, results)
}
```

**3 non-obvious points**:

1. **Per-item retry counter resets** — `retryExec` is called fresh per item, so each item has its own attempt counter. If item 1 needs 2 tries and item 2 needs 3, that's fine. Upstream gets this for free via Python's MRO (`BatchNode(Node)` calls `super()._exec` which starts a new for-loop for `cur_retry`).
2. **Adapter funcs are needed because `TryExecItem` is a method, not a closure** — Go can't pass a bound method directly as `func(any) (any, error)` without naming it explicitly. The two-line wrapper is unavoidable.
3. **`item := item` shadow** — Go's range loop captures by reference until Go 1.22+. Even though our `go.mod` says 1.22, we keep the shadow for clarity and to support readers on older Go.

## What Changed (vs. s05)

```diff
+type BatchItemNode interface {
+    TryExecItem(item any) (any, error)
+    ExecFallbackItem(item any, err error) any
+}
+
+type BatchNode struct {
+    *RetryNode
+}
+
+func NewBatchNode(maxRetries int, wait time.Duration) *BatchNode { ... }
+
+func RunBatch(n Node, shared SharedStore) string { ... }
```

`retryExec` is unchanged. `BatchNode` is genuinely thin — the heavy lifting all happened in s05.

## Try It

```bash
cd agents/s06-batch-node
go run .
go test -v ./...
```

Expected output:

```
============================================================
Translations:
  [ES] Hello, world!
  [FR] Hello, world!
  [DE] Hello, world!
  [ZH] Hello, world!
============================================================
```

Notice FR succeeds — the demo simulates one transient failure on French, which `retryExec` recovers from.

## Upstream Source Reading

```upstream:pocketflow/__init__.py#L36-L37
# Source: pocketflow/__init__.py#L36-L37
class BatchNode(Node):
    def _exec(self,items):
        return [super(BatchNode,self)._exec(i) for i in (items or [])]
```

**Reading notes**:

- **One line of meaningful code**: `[super(BatchNode,self)._exec(i) for i in (items or [])]`. That's it. The list comprehension applies `Node._exec` (which is the retry loop from s05) to each item.
- **`super(BatchNode,self)._exec(i)`**: explicitly skips `BatchNode._exec` to avoid infinite recursion (otherwise self.\_exec(i) would call self.\_exec → BatchNode.\_exec → ...). Our Go version uses `retryExec` as a free function, so no inheritance walking is needed.
- **`items or []`**: Python's defensive nil guard. If prep returns None, treat as empty. Our Go port does `items, _ := prepRes.([]any)` — `items` defaults to nil; ranging nil is safe.
- **No per-batch fallback** — only per-item. Upstream's `BatchNode` doesn't have its own `exec_fallback`; it inherits `Node`'s, which is invoked per item via `_exec`. Same in our Go version: `ExecFallbackItem` runs once per failed item, never for the batch as a whole.
- **Why is this design so elegant?** Because `BatchNode` doesn't reinvent retry; it composes on top of `Node`. The list-comprehension wrapping `super()._exec` means each item runs through the *same* `for self.cur_retry in range(self.max_retries)` loop from line 30. Reuse via inheritance, Pythonic-style.

**Read further**: `pocketflow/__init__.py#L53-L57` defines `BatchFlow` — orthogonal to `BatchNode`. Where `BatchNode` iterates *data*, `BatchFlow` iterates *params*. Same flavour, different axis. That's s07.

---

**Next**: s07 introduces `BatchFlow`. Same name family, opposite axis: iterate parameter sets instead of data items.
