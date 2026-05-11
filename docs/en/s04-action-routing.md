---
title: "s04 · Action-based routing"
chapter: 4
slug: s04-action-routing
est_read_min: 8
---

# s04 · Action-based routing

> What this teaches: `Post`'s return string is a routing signal — it picks which successor runs next. Enables branches (yes/no) and loops (back-edges).

---

## Problem

s03's `Flow.Orchestrate` walks the graph but only follows the `"default"` successor. That's enough for linear pipelines, but real agents need to *choose*:

- A guard node returns `"valid"` → proceed; returns `"invalid"` → ask user.
- A reasoning node returns `"search"` → tool call; returns `"answer"` → final response.
- A judge node returns `"good"` → ship; returns `"retry"` → loop back to the writer.

We also need a way to signal "the graph stopped where the user expected, not because of a missing edge". Upstream uses two heuristics: `action or "default"` (empty falls through to default), and a warning that fires only when *successors exist* but *the chosen action isn't among them*.

## Solution

Refine `getNextNode(curr, action)` to:

1. Normalize empty / unset action to `"default"`.
2. Look up `succ[action]`.
3. If miss AND `len(succ) > 0` (i.e. the node was set up with branches and we missed one) → warn.
4. Return the resolved node (or `nil` to end the flow).

That's the whole change. The graph topology semantics (loops, joins, leaves) already work — they just needed the routing function to respect the action string.

Three design decisions:

1. **Warnings, not errors** — upstream emits `warnings.warn`; we use `log.Printf`. The intent is "your graph is probably wrong but the program shouldn't crash". A stricter version could promote to error at the Flow level.
2. **No max-iteration cap** — loops terminate when a node returns an action with no matching successor (typically the explicit "finished" action). PocketFlow trusts the graph designer, not a global counter. Demonstrated in the `FinishedNode` demo.
3. **Empty string also routes to "default"** — Post is allowed to be lazy and return `""` for "I have nothing interesting to say"; the Flow treats it as the default path.

## How It Works

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────────────────┐
│                          Flow.Orchestrate (s04)                         │
│                                                                         │
│   ┌─────────┐  Post returns "positive"     ┌─────────┐                  │
│   │ check   │ ──────────────────────────▶ │  add3   │                  │
│   │         │                              └────┬────┘                  │
│   │         │  Post returns "negative"     ┌────▼─────┐                 │
│   │         │ ──────────────────────────▶ │  sub3    │                 │
│   └─────────┘                              └────┬─────┘                 │
│        ▲                                       │                       │
│        │       "loop"                          │                       │
│        └──────────────────────── ┌─────────────▼─────┐                  │
│                                  │ finished (counts) │                  │
│                                  │ returns "loop" 3× │                  │
│                                  │ then "finished"   │ → no successor   │
│                                  └───────────────────┘    → flow exits  │
└────────────────────────────────────────────────────────────────────────┘
```

Core 18 lines (from [`agents/s04-action-routing/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s04-action-routing/main.go)):

```go
func getNextNode(curr Node, action string) Node {
    if action == "" {
        action = "default"
    }
    succ := curr.GetSuccessors()
    nxt := succ[action]
    if nxt == nil && len(succ) > 0 {
        keys := make([]string, 0, len(succ))
        for k := range succ {
            keys = append(keys, k)
        }
        log.Printf("[warn] Flow ends: action %q not found in %v", action, keys)
    }
    return nxt
}
```

**3 non-obvious points**:

1. **The warning is "you probably have a missing edge", not "the flow ended"** — leaves (nodes with no successors) terminate silently. The warning fires only when the graph was clearly set up to continue but didn't find a matching edge. This is the difference between `len(succ) > 0` and `len(succ) == 0`.
2. **Loop-back is just a graph edge** — `finished.NextOn("loop", check)` registers `check` as `finished`'s "loop" successor. When `finished.Post` returns `"loop"`, the Flow walks back to `check`. No special "loop" construct; it's just a back-edge.
3. **Termination via "unknown action" is a valid design choice** — the FinishedNode demo returns `"finished"` to exit, even though it could equally `nil`-return or wire an explicit terminator node. PocketFlow's idiom: leaves return whatever string is most descriptive; the Flow's behaviour (warn + exit) is the same.

## What Changed (vs. s03)

```diff
 func getNextNode(curr Node, action string) Node {
     if action == "" { action = "default" }
-    return curr.GetSuccessors()[action]
+    succ := curr.GetSuccessors()
+    nxt := succ[action]
+    if nxt == nil && len(succ) > 0 {
+        keys := make([]string, 0, len(succ))
+        for k := range succ { keys = append(keys, k) }
+        log.Printf("[warn] Flow ends: action %q not found in %v", action, keys)
+    }
+    return nxt
 }
```

That's the entire delta. Everything else — Flow, Node, BaseNode — is unchanged from s03. This is the most surgical chapter of the whole curriculum: routing was always *intended* to work this way; we just hadn't promoted the action string yet.

## Try It

```bash
cd agents/s04-action-routing
go run .
go test -v ./...
```

Expected output:

```
==================================================
final current : 9
last action   : "finished"  (should be "finished")
==================================================
```

The demo starts at `current=1`, takes the "positive" branch three times (1 → 4 → 7 → 10? no — `current=1` → check returns "positive" → add3 → current=4 → finished count=1 returns "loop" → ...).

## Upstream Source Reading

```upstream:pocketflow/__init__.py#L42-L48
# Source: pocketflow/__init__.py#L42-L48
def get_next_node(self,curr,action):
    nxt=curr.successors.get(action or "default")
    if not nxt and curr.successors:
        warnings.warn(f"Flow ends: '{action}' not found in {list(curr.successors)}")
    return nxt
```

And the canonical test for branching + looping:

```upstream:tests/test_flow_basic.py#L104-L157
# Source: tests/test_flow_basic.py#L104-L157
class CheckPositiveNode(Node):
    def post(self, shared_storage, prep_result, proc_result):
        if shared_storage['current'] >= 0:
            return 'positive'
        return 'negative'

# ... in the test ...
n1 = NumberNode(5)       # n1.post returns None -> "default"
check = CheckPositiveNode()
add_if_positive = AddNode(10)
sub_if_negative = AddNode(-20)

n1 >> check                          # default chain n1 -> check
check - "positive" >> add_if_positive  # branch
check - "negative" >> sub_if_negative  # branch
add_if_positive >> check               # LOOP-BACK
sub_if_negative >> check               # LOOP-BACK

flow = Flow(start=n1)
flow.run(shared_storage)
```

**Reading notes**:

- **`not nxt and curr.successors`**: the warning fires only when there ARE successors but none match. A leaf node (`not curr.successors`) exits silently. We mirror this with `len(succ) > 0`.
- **`action or "default"`**: Python truthiness — `None`, `""`, `0`, `[]` all fall through to `"default"`. Go is stricter; we only fall through on `""`.
- **Loop-back is just `add_if_positive >> check`**: there's no `loop` keyword. The graph happens to have a back-edge; the orchestrator doesn't care.
- **Test uses `Node` (with retry) not `BaseNode`**: that's because real flows want resilience; upstream's test fixtures use `Node` (covered in our s05). Our s04 demo uses `BaseNode`-based nodes — the routing logic is identical either way.
- **Deliberately omitted in our Go port**: no max-iteration safety. Upstream has none either. If you write an infinite loop you get an infinite loop. The pedagogical alternative — adding a `MaxSteps` cap — would obscure the fact that loops are just graph edges.

**Read further**: line 26-34 of `pocketflow/__init__.py` introduces `Node` (with retry/fallback), which s05 ports. The retry semantics are orthogonal to routing — they wrap `exec`, not `post`, so action-based routing keeps working unchanged.

---

**Next**: s05 layers retry + fallback on top of the lifecycle. `RetryNode` extends `BaseNode` with `MaxRetries` and `ExecFallback`, replacing today's bare nodes for production-ready code.
