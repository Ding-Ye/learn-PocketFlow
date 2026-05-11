---
title: "s03 · The Flow orchestrator"
chapter: 3
slug: s03-flow-orchestrator
est_read_min: 9
---

# s03 · The Flow orchestrator

> What this teaches: how `Flow` walks the directed graph built in s02. Introduces the `params` vs `shared` distinction — the two dictionaries every PocketFlow node sees.

---

## Problem

s02 lets us build a graph: `greet.Next(upper); upper.Next(count)`. But nobody runs it. If we call `RunOnce(greet, shared)` we get a warning ("you wanted a Flow") and only the first node executes.

We need an *orchestrator* — a thing that, given a start node, walks successors automatically, calling each node's prep/exec/post in order. That's `Flow`. Upstream defines it in 13 lines (`pocketflow/__init__.py#L39-L51`); we mirror it in ~30 Go lines because we have to spell out a few things Python's syntax hides.

## Solution

A `Flow` is a struct that holds a `Start` node. `Orchestrate(shared, params)` loops:

```
curr := f.Start
for curr != nil {
    curr.SetParams(merged_params)
    last = prep_exec_post(curr, shared)
    curr = curr.successors[last]   // (s04 will refine "default" fallback semantics)
}
```

Three design decisions:

1. **`Flow` is itself a Node** — it embeds `*BaseNode`, so flows can be nested inside flows. We don't exploit this in s03's demo (single-level flow), but the type shape is correct.
2. **`params` is per-orchestration, `shared` is per-walk** — both are `map[string]any`, but `Flow.Orchestrate` lets callers pass per-call params that override flow-level params, while `shared` is just threaded through unchanged.
3. **We do NOT shallow-copy nodes per step** — upstream calls `copy.copy(curr)` to isolate per-run state like `cur_retry`. We push that state into a local variable in s05 (retry-fallback) so copying becomes unnecessary. This is a *deliberate divergence*; we explain it in s05's docs.

## How It Works

```ascii-anim frames=2
┌─────────────────────────────────────────────────────────────────────────┐
│                          Flow.Orchestrate                                │
│                                                                          │
│   ┌──────────────┐                                                       │
│   │  Start node  │   ◀──── curr = f.Start                                │
│   │  (greet)     │                                                       │
│   └──────┬───────┘                                                       │
│          │  SetParams(merged)                                            │
│          │  Prep → Exec → Post → "default"                               │
│          │  curr = curr.Successors["default"]                            │
│          ▼                                                               │
│   ┌──────────────┐                                                       │
│   │  upper       │                                                       │
│   └──────┬───────┘                                                       │
│          │  ... same lifecycle                                           │
│          ▼                                                               │
│   ┌──────────────┐                                                       │
│   │  count       │ → Successors["default"] = nil → loop exits            │
│   └──────────────┘                                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

Core 30 lines (from [`agents/s03-flow-orchestrator/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s03-flow-orchestrator/main.go)):

```go
type Flow struct {
    *BaseNode
    Start Node
}

func (f *Flow) Orchestrate(shared SharedStore, params map[string]any) string {
    curr := f.Start
    p := mergeParams(f.Params, params)
    last := ""
    for curr != nil {
        curr.SetParams(p)
        last = runLifecycle(curr, shared)
        curr = getNextNode(curr, last)
    }
    return last
}

func getNextNode(curr Node, action string) Node {
    if action == "" { action = "default" }
    return curr.GetSuccessors()[action]
}

func mergeParams(base, overrides map[string]any) map[string]any {
    out := map[string]any{}
    for k, v := range base { out[k] = v }
    for k, v := range overrides { out[k] = v }
    return out
}
```

**4 non-obvious points**:

1. **Empty action string is normalized to "default"** — upstream uses `curr.successors.get(action or "default")`. The `or "default"` covers both `None` and `""`. Our Go `if action == ""` covers the same edge case.
2. **`mergeParams` builds a fresh map** — never mutates `base` or `overrides`. That matters because the Flow's `Params` field is shared with the caller; mutating it would surprise them on the next `Orchestrate` call.
3. **`Flow` embeds `*BaseNode`** — not `BaseNode` by value. The pointer form makes `Flow` itself participate in chains: `outerFlow.Next(otherNode)` works because the method set is preserved.
4. **The loop body is three lines and there's no recursion** — even though Flow walks a tree-shaped graph. The trick: each node has at most one successor *per action*, so the graph reduces to a single path under any specific sequence of actions. Recursion would be wasted complexity.

## What Changed (vs. s02)

```diff
 type Node interface {
     Prep(shared SharedStore) any
     Exec(prepRes any) any
     Post(shared SharedStore, prepRes any, execRes any) string
     GetSuccessors() map[string]Node
+    SetParams(p map[string]any)
 }

+type Flow struct {
+    *BaseNode
+    Start Node
+}
+
+func NewFlow(start Node) *Flow { ... }
+func (f *Flow) Orchestrate(shared SharedStore, params map[string]any) string { ... }
+func (f *Flow) Run(shared SharedStore) string { return f.Orchestrate(shared, nil) }
+
+func mergeParams(base, overrides map[string]any) map[string]any { ... }
+func getNextNode(curr Node, action string) Node { ... }
+func runLifecycle(n Node, shared SharedStore) string { ... }
```

`RunOnce` from s01-s02 is gone — `Orchestrate` replaces it. Same effect for a 1-node graph (`flow.Orchestrate(shared, nil)` walks once and exits), but with proper graph-walking semantics for everything else.

## Try It

```bash
cd agents/s03-flow-orchestrator
go run .
go test -v ./...
```

Expected output:

```
=== Flow run ===
msg : HEY, POCKETFLOW!
len : 16
last action: "default"
```

The flow:
1. `greet` reads `shared["name"]` + `params["greeting"]`, writes `"Hey, PocketFlow!"` to `shared["msg"]` (per-call params override flow-level)
2. `upper` uppercases it in place
3. `count` writes len to `shared["len"]`
4. `count` has no successor → loop exits with `last="default"`

## Upstream Source Reading

```upstream:pocketflow/__init__.py#L39-L51
# Source: pocketflow/__init__.py#L39-L51
class Flow(BaseNode):
    def __init__(self,start=None): super().__init__(); self.start_node=start
    def start(self,start): self.start_node=start; return start
    def get_next_node(self,curr,action):
        nxt=curr.successors.get(action or "default")
        if not nxt and curr.successors: warnings.warn(f"Flow ends: '{action}' not found in {list(curr.successors)}")
        return nxt
    def _orch(self,shared,params=None):
        curr,p,last_action =copy.copy(self.start_node),(params or {**self.params}),None
        while curr: curr.set_params(p); last_action=curr._run(shared); curr=copy.copy(self.get_next_node(curr,last_action))
        return last_action
    def _run(self,shared): p=self.prep(shared); o=self._orch(shared); return self.post(shared,p,o)
    def post(self,shared,prep_res,exec_res): return exec_res
```

**Reading notes**:

- **`copy.copy(self.start_node)` on line 47**: upstream shallow-copies every node before invoking it, so the original keeps a pristine state. Their motivation is to keep `cur_retry` (added by `Node`, s05) per-run-scoped. Our Go port pushes that retry state into a stack variable inside the retry loop, so we don't need to copy. The trade-off: if a user mutates `node.Params` from inside `Exec`, upstream's copy preserves the original; ours doesn't. We accept that — mutating params mid-flow is undefined behaviour.
- **`get_next_node` warns on unknown action**: line 44 warns *only when there ARE successors but the chosen action isn't among them*. Our s03 silently returns nil; s04 adds back the warning when we cover branching properly.
- **`Flow._run` calls `prep`, then `_orch`, then `post`**: this lets a Flow be wrapped in another Flow (post returns the orchestration result as an action that the outer flow can branch on). Our `Flow.Run` mirrors the structure — but we don't show flow composition until later.
- **`{**self.params, **bp}` on line 47 and again on line 56**: Python dict-spread merge. Our `mergeParams` does exactly this.
- **`Flow.post` is a passthrough by default (L51)**: returns `exec_res` (i.e. the last action from `_orch`). That's how a Flow can be used as a Node inside another Flow — its `post` return becomes that outer flow's routing action. We preserve this in `Flow.Run`.

**Read further**: line 42-45 of `pocketflow/__init__.py` is the action-lookup logic that s04 will lean on. The conditional warning on missing successors is the seed of "branching" — and s04 builds the full branch + loop story on top of it.

---

**Next**: s04 turns the simple "default" routing of s03 into real branching with action strings, complete with loop-back support and the "unknown action" warning.
