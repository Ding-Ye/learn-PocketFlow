---
title: "s01 · Minimum node loop"
chapter: 1
slug: s01-minimum-node
est_read_min: 8
---

# s01 · Minimum node loop

> What this teaches: PocketFlow's atomic unit — a `Node` with a `Prep → Exec → Post` lifecycle. Everything later in the curriculum (chaining, orchestration, retry, batching, async, RAG) is built on top of these three methods.

---

## Problem

We want to learn PocketFlow by re-implementing it in Go, one mechanism at a time. But before we can wire nodes together, route on conditions, retry on failure, or fan out into batches — we need to know what a *single* node looks like. The upstream Python framework is famously 99 lines; line 3-20 defines `BaseNode`, the foundation everything else extends. If we get the lifecycle wrong here, every later session inherits the bug.

So: what is the smallest possible PocketFlow node, and what's the smallest possible thing that runs one? No flow yet. No successors yet. No retry. Just one node, one shared dict, three methods.

## Solution

A node is a value with three methods: `Prep` reads from a shared dict, `Exec` does the work, `Post` writes results back and returns a routing hint. We model this in Go with a `Node` interface, a `BaseNode` struct you embed when you want defaults, and a `RunOnce(node, shared)` helper that calls the trio in order.

Key design decisions:

1. **`SharedStore` is `map[string]any`** — exactly like Python's dict. We don't type-parameterize; loose coupling is the whole point of the shared-store pattern.
2. **`BaseNode` exists so embedding nodes get zero-arg defaults** — Go has no method defaults, so we use struct embedding the same way Python uses class inheritance. Override what you need; inherit the rest.
3. **`RunOnce` is a free function, not a method** — it doesn't belong on `BaseNode` because Flow (s03) will replace it. Keeping it free makes the s01 → s03 transition cleaner.

## How It Works

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────┐
│                    SharedStore (map)                       │
│   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│   │ "question"   │ │ "answer"     │ │ ...          │       │
│   └──────┬───────┘ └──────▲───────┘ └──────────────┘       │
│          │ read           │ write                          │
│   ┌──────▼────────────────┴───────────────────────┐        │
│   │             AnswerNode                         │        │
│   │  Prep(shared) ─► Exec(prep) ─► Post(shared,…)  │        │
│   │      │              │                │         │        │
│   │      ▼              ▼                ▼         │        │
│   │   "question"     "Mock answer..."  action="default"     │
│   └──────────────────────────────────────────────────┘     │
└────────────────────────────────────────────────────────────┘
```

The core 30 lines (from [`agents/s01-minimum-node/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s01-minimum-node/main.go)):

```go
type SharedStore map[string]any

type Node interface {
    Prep(shared SharedStore) any
    Exec(prepRes any) any
    Post(shared SharedStore, prepRes any, execRes any) string
}

type BaseNode struct {
    Params     map[string]any
    Successors map[string]Node
}

func NewBaseNode() *BaseNode {
    return &BaseNode{Params: map[string]any{}, Successors: map[string]Node{}}
}

func (b *BaseNode) Prep(shared SharedStore) any                              { return nil }
func (b *BaseNode) Exec(prepRes any) any                                     { return nil }
func (b *BaseNode) Post(shared SharedStore, prepRes any, execRes any) string { return "default" }

func RunOnce(n Node, shared SharedStore) string {
    if b, ok := n.(interface{ HasSuccessors() bool }); ok && b.HasSuccessors() {
        log.Printf("[warn] Node has successors but RunOnce ignores them; use Flow (s03+).")
    }
    p := n.Prep(shared)
    e := n.Exec(p)
    return n.Post(shared, p, e)
}
```

**4 non-obvious points**:

1. **`Successors` already exists** — even though s01 doesn't use chaining, we keep the field in `BaseNode` so future sessions add behavior without rewriting the type. This mirrors how upstream's `BaseNode.__init__` initializes both `params` and `successors` together, line 4.
2. **`RunOnce` warns instead of failing** — if you give a node successors but call `RunOnce` directly, you wanted a Flow. Upstream uses `warnings.warn`; we use `log.Printf` because Go has no warnings package. Either way, it's recoverable.
3. **The action string is mandatory** — `Post` *must* return something. Returning `""` would force the Flow (s04) to treat it as "default". We make that explicit by returning `"default"` from `BaseNode.Post`.
4. **`*BaseNode` is a pointer embedding** — required so multiple nodes don't share state by accident. If we embedded `BaseNode` by value, every `NewAnswerNode()` would carry an independent struct anyway, but the pointer form makes intent clear.

## What Changed

This is the first session, so there's no s00 to diff against. The starting state of the entire learn-PocketFlow journey is *empty*. After s01, you have:

- `Node` interface
- `BaseNode` struct (with `Params`, `Successors`)
- `RunOnce` helper
- One demo: `AnswerNode`

## Try It

```bash
cd agents/s01-minimum-node
go run .
go test -v ./...
```

Expected output:

```
============================================================
Question : What is PocketFlow?
Answer   : Mock answer to: "What is PocketFlow?" (length=19)
Action   : "default"
============================================================
```

And the test suite:

```
=== RUN   TestAnswerNode_ReadsAndWrites
--- PASS: TestAnswerNode_ReadsAndWrites (0.00s)
=== RUN   TestAnswerNode_PostReturnsDefault
--- PASS: TestAnswerNode_PostReturnsDefault (0.00s)
=== RUN   TestRunOnce_LifecycleOrder
--- PASS: TestRunOnce_LifecycleOrder (0.00s)
=== RUN   TestSharedStore_MutationVisible
--- PASS: TestSharedStore_MutationVisible (0.00s)
=== RUN   TestRunOnce_WarnsOnSuccessors
--- PASS: TestRunOnce_WarnsOnSuccessors (0.00s)
PASS
```

## Upstream Source Reading

The upstream equivalent is `BaseNode` at lines 3-20 of [`pocketflow/__init__.py`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L3-L20):

```python
# Source: pocketflow/__init__.py#L3-L20
class BaseNode:
    def __init__(self): self.params,self.successors={},{}
    def set_params(self,params): self.params=params
    def next(self,node,action="default"):
        if action in self.successors: warnings.warn(f"Overwriting successor for action '{action}'")
        self.successors[action]=node; return node
    def prep(self,shared): pass
    def exec(self,prep_res): pass
    def post(self,shared,prep_res,exec_res): pass
    def _exec(self,prep_res): return self.exec(prep_res)
    def _run(self,shared): p=self.prep(shared); e=self._exec(p); return self.post(shared,p,e)
    def run(self,shared):
        if self.successors: warnings.warn("Node won't run successors. Use Flow.")
        return self._run(shared)
    def __rshift__(self,other): return self.next(other)
    def __sub__(self,action):
        if isinstance(action,str): return _ConditionalTransition(self,action)
        raise TypeError("Action must be a string")
```

**Reading notes**:

- **Indirect `_exec` wrapper**: upstream has both `exec()` (user override target) and `_exec()` (caller). In `BaseNode` they're identical — but `Node` (s05) will override `_exec` to wrap retry around `exec`. Our Go version skips this seam in s01 and reintroduces it as a wrapper closure in s05.
- **`next()` returns the appended node**: this lets users fluent-chain `parent.next(child).next(grandchild)`. We hold off on this until s02; s01 just keeps the `Successors` map.
- **`>>` and `-` operator overloads**: lines 17-20 enable `nodeA >> nodeB` and `nodeA - "action" >> nodeB`. Go can't overload operators, so s02 will replace these with `Next(node)` and `NextOn("action", node)` methods.
- **`set_params` exists at L5 already**: even at the BaseNode level, params can be injected from outside. We skip this until s03 (Flow) — but the `Params` field is on our struct from day one to match.
- **`run()` warns on successors, then calls `_run()`**: the warn-then-still-run shape is exactly what our `RunOnce` does. Calling `run()` on a node with successors is a category error (you wanted a Flow) — but it's recoverable, so a warning is correct.

**Read further**: `pocketflow/__init__.py#L3-L20` → s02 will pick up `next()` at line 6 and the `_ConditionalTransition` helper at lines 22-24 → s03 then layers `Flow` at lines 39-51 on top, walking the successor graph.

---

**Next**: s02 makes nodes chainable. We'll add `Next(other)` and `NextOn(action, other)` to `BaseNode`, so we can build directed graphs without yet running them.
