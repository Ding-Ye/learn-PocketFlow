---
title: "s07 · BatchFlow (param iteration)"
chapter: 7
slug: s07-batch-flow
est_read_min: 7
---

# s07 · BatchFlow (param iteration)

> What this teaches: the *other* batch axis. Where `BatchNode` (s06) iterates data items inside one node, `BatchFlow` iterates parameter sets across an entire inner flow.

---

## Problem

s06 lets one node process N items. But sometimes the unit of variation is bigger than data — it's the *whole flow*:

- "Apply 3 different filters to the same image" — each filter is a different parameter set running the same load/transform/save flow.
- "Train the same model with 8 hyperparameter combinations" — each combo runs the whole training flow.
- "Translate the same paragraph into 5 languages where the lang is in params" — each language re-runs the prep/translate/format flow.

We need a flow that runs N times with N different `params` dicts but the *same* node graph. That's `BatchFlow`.

## Solution

Override `Prep` to return `[]map[string]any` — a slice of per-iteration param dicts. The runner:

```
for _, bp := range prepBatches {
    Orchestrate(shared, mergeParams(flow.Params, bp))   // full flow walk with merged params
}
Post(shared, prepBatches, nil)
```

Three design decisions:

1. **`shared` survives across iterations, `params` resets** — iteration 1's writes to `shared` are visible to iteration 2. The dicts in `prepBatches` are the only per-iteration variation.
2. **`Post` runs once after all iterations** — not per iteration. Use it to write aggregate stats (count, summary). Mirrors upstream's `self.post(shared, pr, None)` after the loop.
3. **Concrete BatchFlows implement `BatchFlowPrep`** — a typed interface returning `[]map[string]any`. Avoids ambiguity with the plain `Node.Prep` (which returns `any`).

## How It Works

```ascii-anim frames=2
┌─────────────────────────────────────────────────────────────────────┐
│                       RunBatchFlow                                   │
│                                                                      │
│   prepBatches := node.PrepBatch(shared)   // []map[string]any        │
│                                                                      │
│   for _, bp := range prepBatches {                                   │
│       merged := mergeParams(flow.Params, bp)                         │
│       Flow.Orchestrate(shared, merged)    // FULL flow walk!         │
│   }                                                                  │
│                                                                      │
│   node.Post(shared, prepBatches, nil)                                │
└─────────────────────────────────────────────────────────────────────┘
```

Core 13 lines (from [`agents/s07-batch-flow/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s07-batch-flow/main.go)):

```go
func RunBatchFlow(bf *BatchFlow, sharedNode Node, shared SharedStore) string {
    var prepBatches []map[string]any
    if b, ok := sharedNode.(BatchFlowPrep); ok {
        prepBatches = b.PrepBatch(shared)
    }
    for _, bp := range prepBatches {
        bf.Orchestrate(shared, bp)
    }
    return sharedNode.Post(shared, prepBatches, nil)
}
```

**3 non-obvious points**:

1. **Each `Orchestrate` call walks the *entire* inner flow** — not just one node. That's why it's called BatchFlow, not BatchNode. The cost: if the inner flow is 5 nodes deep and you batch over 10 dicts, that's 50 node executions.
2. **`shared` is the *only* cross-iteration channel** — `params` is reset each iteration via `mergeParams`. Use shared for cumulative log/output; rely on params for per-iteration tweaks.
3. **`Post(shared, prepBatches, nil)`** — `exec_res` is `nil` because there's no single "exec result" for a BatchFlow; the work happened in the inner flow runs. Upstream passes `None` for the same reason.

## What Changed (vs. s06)

```diff
+type BatchFlow struct {
+    *Flow
+}
+
+type BatchFlowPrep interface {
+    PrepBatch(shared SharedStore) []map[string]any
+}
+
+func NewBatchFlow(start Node) *BatchFlow { return &BatchFlow{Flow: NewFlow(start)} }
+
+func RunBatchFlow(bf *BatchFlow, sharedNode Node, shared SharedStore) string {
+    var prepBatches []map[string]any
+    if b, ok := sharedNode.(BatchFlowPrep); ok {
+        prepBatches = b.PrepBatch(shared)
+    }
+    for _, bp := range prepBatches {
+        bf.Orchestrate(shared, bp)
+    }
+    return sharedNode.Post(shared, prepBatches, nil)
+}
```

`Flow`, `BaseNode`, and the orchestration loop are unchanged. `BatchFlow` is a thin wrapper that just iterates `Orchestrate`.

## Try It

```bash
cd agents/s07-batch-flow
go run .
go test -v ./...
```

Expected output:

```
============================================================
Batches run: 4 (expected 4 = 2 imgs × 2 filters)
Log:
  cat → [sepia]█████
  cat → [grayscale]█████
  dog → [sepia]██████
  dog → [grayscale]██████
============================================================
```

## Upstream Source Reading

```upstream:pocketflow/__init__.py#L53-L57
# Source: pocketflow/__init__.py#L53-L57
class BatchFlow(Flow):
    def _run(self,shared):
        pr=self.prep(shared) or []
        for bp in pr: self._orch(shared,{**self.params,**bp})
        return self.post(shared,pr,None)
```

**Reading notes**:

- **`pr=self.prep(shared) or []`**: upstream allows `prep` to return None as "no batches". We replicate via the type-assertion guard — if the node doesn't implement `BatchFlowPrep`, `prepBatches` stays nil, the for-loop runs zero times.
- **`{**self.params, **bp}` is per-iteration merge**: same `mergeParams` from s03. Per-iteration `bp` overrides flow-level `self.params` on key collision. Standard.
- **`self._orch(shared, ...)` not `self.run(...)`**: upstream calls the internal `_orch`, skipping its own `prep`/`post`. Otherwise we'd recurse forever (BatchFlow's prep is the source of `pr`). Our Go version calls `Orchestrate` directly, same reasoning.
- **`return self.post(shared,pr,None)` after the loop**: post runs once, gets the full `pr` (param-dict list) as `prep_res`, and `None` as `exec_res`. Use the param-dict list for aggregation; the "exec result" doesn't exist.
- **What BatchFlow can't do**: parallel iteration. Iterations run sequentially. For parallelism, you want `AsyncParallelBatchFlow` (`pocketflow/__init__.py#L96-L100`) — covered as Appendix B exercise #4.

**Read further**: `pocketflow/__init__.py#L59-L74` introduces `AsyncNode`. s08 ports it. Async is a separate axis from batching; understanding both lets you combine them in s09.

---

**Next**: s08 introduces `AsyncNode`. We map Python's `async def` lifecycle to Go's `context.Context` + goroutine-friendly methods.
