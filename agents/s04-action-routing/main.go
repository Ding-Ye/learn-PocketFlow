// Package main implements s04 of learn-PocketFlow: action-based routing.
//
// s03's Flow only walked the "default" successor. s04 promotes Post's return
// value to a routing signal: any string returned from Post becomes the key
// for the next-node lookup. This enables conditional branching ("yes" vs
// "no") AND loops (a node whose successor points back to an earlier node).
//
// We also reinstate the warning upstream emits when a node has successors
// but the chosen action isn't among them — that's the signal "your graph
// probably has a missing edge".
//
// Upstream reference: pocketflow/__init__.py#L42-L48 + tests/test_flow_basic.py#L104-L157.
package main

import (
	"fmt"
	"log"
	"strings"
)

type SharedStore map[string]any

type Node interface {
	Prep(shared SharedStore) any
	Exec(prepRes any) any
	Post(shared SharedStore, prepRes any, execRes any) string
	GetSuccessors() map[string]Node
	SetParams(p map[string]any)
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
func (b *BaseNode) GetSuccessors() map[string]Node                            { return b.Successors }
func (b *BaseNode) SetParams(p map[string]any)                                { b.Params = p }

func (b *BaseNode) Next(n Node) Node { return b.NextOn("default", n) }
func (b *BaseNode) NextOn(action string, n Node) Node {
	if b.Successors == nil {
		b.Successors = map[string]Node{}
	}
	if _, ok := b.Successors[action]; ok {
		log.Printf("[warn] overwriting successor for action %q", action)
	}
	b.Successors[action] = n
	return n
}

type Flow struct {
	*BaseNode
	Start Node
}

func NewFlow(start Node) *Flow { return &Flow{BaseNode: NewBaseNode(), Start: start} }

// Orchestrate walks the graph following action-string transitions.
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

func (f *Flow) Run(shared SharedStore) string { return f.Orchestrate(shared, nil) }

func runLifecycle(n Node, shared SharedStore) string {
	p := n.Prep(shared)
	e := n.Exec(p)
	return n.Post(shared, p, e)
}

// getNextNode is the s04 promotion: warn when an action has no matching
// successor AND the node has successors at all (i.e. it's not a leaf).
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

func mergeParams(base, overrides map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// ----- Demo: branching + loop-back (mirrors tests/test_flow_basic.py#L104-L157) -----

// CheckPositiveNode reads shared["current"], returns "positive" if >= 0
// else "negative". Drives branching.
type CheckPositiveNode struct{ *BaseNode }

func NewCheckPositiveNode() *CheckPositiveNode { return &CheckPositiveNode{BaseNode: NewBaseNode()} }
func (n *CheckPositiveNode) Post(shared SharedStore, prepRes, execRes any) string {
	cur, _ := shared["current"].(int)
	if cur >= 0 {
		return "positive"
	}
	return "negative"
}

// Add3Node adds 3 to shared["current"], returns "default".
type Add3Node struct{ *BaseNode }

func NewAdd3Node() *Add3Node { return &Add3Node{BaseNode: NewBaseNode()} }
func (n *Add3Node) Post(shared SharedStore, prepRes, execRes any) string {
	cur, _ := shared["current"].(int)
	shared["current"] = cur + 3
	return "default"
}

// Subtract3Node subtracts 3 from shared["current"], returns "default".
type Subtract3Node struct{ *BaseNode }

func NewSubtract3Node() *Subtract3Node { return &Subtract3Node{BaseNode: NewBaseNode()} }
func (n *Subtract3Node) Post(shared SharedStore, prepRes, execRes any) string {
	cur, _ := shared["current"].(int)
	shared["current"] = cur - 3
	return "default"
}

// FinishedNode is a leaf that returns "finished" — by leaving its
// successors empty AND returning "finished", the Flow exits cleanly.
type FinishedNode struct {
	*BaseNode
	count int
}

func NewFinishedNode() *FinishedNode { return &FinishedNode{BaseNode: NewBaseNode()} }
func (n *FinishedNode) Post(shared SharedStore, prepRes, execRes any) string {
	n.count++
	if n.count >= 3 {
		return "finished" // leaf — no successors registered for this action → flow ends
	}
	return "loop" // route back to start
}

func main() {
	// Graph:
	//
	//       ┌───────────────────────────┐
	//       ▼                           │
	//   check ─"positive"─► add3 ───────┤
	//      \                            │
	//       └"negative"─► subtract3 ────┘   (both loop back to check)
	//
	// We'll start at current=-3 → negative → subtract3 → current=-6 → ...
	// To bound the demo we cap iterations via FinishedNode after add3/subtract3.

	check := NewCheckPositiveNode()
	add3 := NewAdd3Node()
	sub3 := NewSubtract3Node()
	finished := NewFinishedNode()

	check.NextOn("positive", add3)
	check.NextOn("negative", sub3)
	add3.NextOn("default", finished)
	sub3.NextOn("default", finished)
	finished.NextOn("loop", check)
	// finished returns "finished" eventually → no successor → flow exits.

	flow := NewFlow(check)
	shared := SharedStore{"current": 1}
	last := flow.Run(shared)

	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("final current : %v\n", shared["current"])
	fmt.Printf("last action   : %q  (should be \"finished\")\n", last)
	fmt.Println(strings.Repeat("=", 50))
}
