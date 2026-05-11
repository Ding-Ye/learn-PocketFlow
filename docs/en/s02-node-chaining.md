---
title: "s02 · Node chaining + transitions"
chapter: 2
slug: s02-node-chaining
est_read_min: 7
---

# s02 · Node chaining + transitions

> What this teaches: how to wire nodes into a directed graph. Upstream uses `>>` and `-"action">>` operator overloads; Go can't overload — we ship `Next(node)` and `NextOn(action, node)` instead.

---

## Problem

s01 gave us a single Node and a `RunOnce` helper. But agents are graphs, not lone callbacks: "if the LLM returns `tool_use`, route to the tool node; if `end_turn`, stop". Without a way to connect nodes, every Flow has to be a giant if-else inside a single `Exec`.

PocketFlow solves this in Python with two operator overloads — `parent >> child` for the default transition and `parent - "action" >> child` for named transitions. Both desugar to `parent.next(child, action)`. Go has no operator overloading. We need to keep the *semantics* and find a clean Go *syntax*.

## Solution

Add two methods to `BaseNode`:

```go
func (b *BaseNode) Next(n Node) Node                   // default action
func (b *BaseNode) NextOn(action string, n Node) Node  // named action
```

Both append `n` to the `Successors` map under the chosen action key and return `n`, so callers can fluent-chain. We add one method on the `Node` interface — `GetSuccessors() map[string]Node` — so s03's Flow can ask any node for its outgoing edges without depending on the concrete `BaseNode` type.

Three design decisions:

1. **`NextOn` warns on overwrite, doesn't error** — upstream calls `warnings.warn`. We use `log.Printf`. The intent is "you probably made a mistake but the program shouldn't crash".
2. **`Next` is a thin wrapper over `NextOn("default", n)`** — keeps the two methods consistent. Action "default" is a magic string borrowed verbatim from upstream.
3. **No operator overloading even via reflection tricks** — Go is a small-syntax language. `.Next()` is more verbose than `>>` but more searchable in `grep`.

## How It Works

```ascii-anim frames=2
┌──────────────────────────────────────────────────────────────┐
│             check.NextOn("empty", load)                       │
│             check.Next(load)              // == NextOn("default")
│             load.Next(caps)                                   │
│                                                               │
│   ┌──────────┐   "default" / "empty"    ┌──────────┐          │
│   │  check   │ ───────────────────────▶ │   load   │          │
│   │ (s02)    │                          │ (s02)    │          │
│   └──────────┘                          └────┬─────┘          │
│                                              │  "default"     │
│                                              ▼                │
│                                         ┌──────────┐          │
│                                         │   caps   │          │
│                                         │ (s02)    │          │
│                                         └──────────┘          │
└──────────────────────────────────────────────────────────────┘
```

Core 20 lines (from [`agents/s02-node-chaining/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s02-node-chaining/main.go)):

```go
func (b *BaseNode) Next(n Node) Node { return b.NextOn("default", n) }

func (b *BaseNode) NextOn(action string, n Node) Node {
    if b.Successors == nil { b.Successors = map[string]Node{} }
    if _, ok := b.Successors[action]; ok {
        log.Printf("[warn] overwriting successor for action %q", action)
    }
    b.Successors[action] = n
    return n
}

// Node interface gains:
type Node interface {
    Prep(shared SharedStore) any
    Exec(prepRes any) any
    Post(shared SharedStore, prepRes any, execRes any) string
    GetSuccessors() map[string]Node  // NEW
}
```

**3 non-obvious points**:

1. **`GetSuccessors()` is on the interface, not just `BaseNode`** — s03's Flow asks any node "what are your outgoing edges?" without caring about the concrete type. If we left this method on `*BaseNode` only, Flow would have to type-assert, leaking implementation details.
2. **`Next` and `NextOn` return the *new* successor, not `self`** — this matches upstream `def next(self,node,action="default"): ...; return node`. It lets you fluent-chain `a.Next(b).Next(c)`. If we returned `self`, that pattern would build a star graph instead of a line.
3. **Nil-map guard on every call** — Go's zero-value map is nil and panics on write. The constructor `NewBaseNode()` already initializes the map, but the guard makes the method safe if someone embeds `BaseNode` without calling the constructor.

## What Changed (vs. s01)

```diff
 type Node interface {
     Prep(shared SharedStore) any
     Exec(prepRes any) any
     Post(shared SharedStore, prepRes any, execRes any) string
+    GetSuccessors() map[string]Node
 }

+func (b *BaseNode) Next(n Node) Node { return b.NextOn("default", n) }
+func (b *BaseNode) NextOn(action string, n Node) Node {
+    if b.Successors == nil { b.Successors = map[string]Node{} }
+    if _, ok := b.Successors[action]; ok {
+        log.Printf("[warn] overwriting successor for action %q", action)
+    }
+    b.Successors[action] = n
+    return n
+}
+func (b *BaseNode) GetSuccessors() map[string]Node { return b.Successors }
```

`RunOnce` is unchanged but switches from interface-type-assertion to the direct `n.GetSuccessors()` call now that it's part of the contract.

## Try It

```bash
cd agents/s02-node-chaining
go run .
go test -v ./...
```

Expected output:

```
Graph built (no walking — that's s03):
  check[default] -> *main.LoadNode
  check[empty  ] -> *main.LoadNode
  load [default] -> *main.CapsNode
  caps successors: 0 (terminal)

Trying RunOnce on a chained node (expect a warning):
2026/05/12 00:21:33 [warn] Node has successors but RunOnce ignores them; use Flow (s03+).
```

## Upstream Source Reading

Upstream's chaining lives in three pieces: the `next()` method on `BaseNode`, the `__rshift__` and `__sub__` operator overloads, and a tiny helper class `_ConditionalTransition` that wires them together.

```upstream:pocketflow/__init__.py#L6-L24
# Source: pocketflow/__init__.py#L6-L24
def next(self,node,action="default"):
    if action in self.successors:
        warnings.warn(f"Overwriting successor for action '{action}'")
    self.successors[action]=node; return node

# (...prep/exec/post elided — see s01...)

def __rshift__(self,other): return self.next(other)
def __sub__(self,action):
    if isinstance(action,str): return _ConditionalTransition(self,action)
    raise TypeError("Action must be a string")

class _ConditionalTransition:
    def __init__(self,src,action): self.src,self.action=src,action
    def __rshift__(self,tgt): return self.src.next(tgt,self.action)
```

**Reading notes**:

- **`>>` desugars to `.next()`**: `__rshift__` just calls `next` with the default action. That's why "default" is the fallback action key — it's the literal that the operator hard-codes.
- **`-"action" >>` is a two-step trick**: `parent - "action"` returns a transient `_ConditionalTransition`; that object's own `__rshift__` then calls `parent.next(target, "action")`. You can't write that in one operator because Python's `-` always returns an intermediate value, not a side effect.
- **Warning semantics**: upstream uses `warnings.warn` which can be filtered with `warnings.filterwarnings("error")` to upgrade to exceptions. Go's `log.Printf` is one-way. If you want strict mode, wrap `NextOn` and check the map before calling.
- **`next()` returns the appended node**: this is what enables `a.next(b).next(c)`. Both upstream and our Go port preserve this; without it, building a linear pipeline becomes much more verbose.
- **Deliberately omitted from our Go port**: we don't ship a helper struct like `_ConditionalTransition`. In Python it exists *only* to make the `-"action">>` two-token operator sequence work; without operator overloading, our `NextOn(action, node)` does the job in one method call. Less code, less indirection.

**Read further**: from `pocketflow/__init__.py#L6-L24`, the chain continues into `Flow._orch` at lines 46-49 — that's the loop that *walks* the graph we just built. We tackle it in s03.

---

**Next**: s03 builds the `Flow` orchestrator that actually walks the successors we registered here.
