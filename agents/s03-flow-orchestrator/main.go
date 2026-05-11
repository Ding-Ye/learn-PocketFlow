// Package main implements s03 of learn-PocketFlow: the Flow orchestrator.
//
// s01 gave us a Node. s02 let us chain nodes into a graph. s03 actually
// WALKS that graph: starting at Flow.Start, run each node's lifecycle, look
// up the next node by the action returned from Post, repeat until no next
// node exists.
//
// We also formalise the params/shared distinction:
//   - shared : mutable map passed by reference to every node in the flow
//   - params : per-flow-iteration map; each node receives the same params dict
//              for the duration of one orchestration walk
//
// Upstream reference: pocketflow/__init__.py#L39-L51 (Flow + _orch).
package main

import (
	"fmt"
	"log"
)

type SharedStore map[string]any

type Node interface {
	Prep(shared SharedStore) any
	Exec(prepRes any) any
	Post(shared SharedStore, prepRes any, execRes any) string
	GetSuccessors() map[string]Node

	// NEW IN s03 — Flow injects per-iteration params before running each node.
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

// ----- Flow orchestrator -----

// Flow walks a directed graph of nodes starting at Start. It is itself a
// Node (embeds BaseNode) so flows can compose — but in s03 we only show the
// top-level case.
type Flow struct {
	*BaseNode
	Start Node
}

// NewFlow constructs a Flow that begins at `start`.
func NewFlow(start Node) *Flow {
	return &Flow{BaseNode: NewBaseNode(), Start: start}
}

// Orchestrate walks the graph until no successor exists. Returns the last
// action returned by the final Post.
//
// This is upstream's Flow._orch:
//
//	def _orch(self,shared,params=None):
//	    curr,p,last_action =copy.copy(self.start_node),(params or {**self.params}),None
//	    while curr: curr.set_params(p); last_action=curr._run(shared); curr=copy.copy(self.get_next_node(curr,last_action))
//	    return last_action
//
// Differences from upstream:
//   - We don't shallow-copy nodes per step. Upstream uses `copy.copy` to
//     isolate `cur_retry` across runs; we instead move retry state into a
//     local variable in s05, removing the need for copying.
//   - Params merging is explicit via mergeParams (no `{**a, **b}` syntax).
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

// Run lets a Flow be used as a Node from outside (e.g. wrapped in another Flow).
// Mirrors upstream's Flow._run.
func (f *Flow) Run(shared SharedStore) string {
	return f.Orchestrate(shared, nil)
}

// runLifecycle does prep → exec → post on a single node.
func runLifecycle(n Node, shared SharedStore) string {
	p := n.Prep(shared)
	e := n.Exec(p)
	return n.Post(shared, p, e)
}

// getNextNode looks up the successor for the action returned by Post.
// Falls back to "default" if action is empty. Returns nil to end the flow.
// (s04 expands the warning behaviour for unknown actions.)
func getNextNode(curr Node, action string) Node {
	if action == "" {
		action = "default"
	}
	return curr.GetSuccessors()[action]
}

// mergeParams returns base ⊕ overrides; overrides win on key collision.
// Mirrors upstream's `{**self.params, **bp}` dict-spread syntax.
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

// ----- Demo: 3-node linear flow with a params-aware node -----

// GreetNode reads "name" from shared, "greeting" from params, writes the result.
type GreetNode struct{ *BaseNode }

func NewGreetNode() *GreetNode { return &GreetNode{BaseNode: NewBaseNode()} }

func (n *GreetNode) Prep(shared SharedStore) any {
	name, _ := shared["name"].(string)
	greet, _ := n.Params["greeting"].(string)
	if greet == "" {
		greet = "Hello"
	}
	return fmt.Sprintf("%s, %s!", greet, name)
}
func (n *GreetNode) Post(shared SharedStore, prepRes, execRes any) string {
	shared["msg"] = prepRes
	return "default"
}

// UppercaseNode transforms shared["msg"] in place.
type UppercaseNode struct{ *BaseNode }

func NewUppercaseNode() *UppercaseNode { return &UppercaseNode{BaseNode: NewBaseNode()} }
func (n *UppercaseNode) Post(shared SharedStore, prepRes, execRes any) string {
	if s, ok := shared["msg"].(string); ok {
		shared["msg"] = upper(s)
	}
	return "default"
}

// CountNode writes len of msg into shared["len"].
type CountNode struct{ *BaseNode }

func NewCountNode() *CountNode { return &CountNode{BaseNode: NewBaseNode()} }
func (n *CountNode) Post(shared SharedStore, prepRes, execRes any) string {
	if s, ok := shared["msg"].(string); ok {
		shared["len"] = len(s)
	}
	return "default"
}

func upper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 32
		}
	}
	return string(out)
}

func main() {
	greet := NewGreetNode()
	upper := NewUppercaseNode()
	count := NewCountNode()
	greet.Next(upper)
	upper.Next(count)

	flow := NewFlow(greet)
	flow.Params = map[string]any{"greeting": "Hi"}

	shared := SharedStore{"name": "PocketFlow"}
	last := flow.Orchestrate(shared, map[string]any{"greeting": "Hey"})

	fmt.Println("=== Flow run ===")
	fmt.Printf("msg : %v\n", shared["msg"])
	fmt.Printf("len : %v\n", shared["len"])
	fmt.Printf("last action: %q\n", last)
}
