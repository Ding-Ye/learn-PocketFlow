// Package main implements s02 of learn-PocketFlow: node chaining.
//
// One node is not enough — real agents are graphs. We add Next() and
// NextOn(action) methods to BaseNode so callers can build a directed graph.
// We still don't *walk* it (that's s03 / Flow); we just construct it.
//
// Upstream reference:
//   - pocketflow/__init__.py#L6-L24 (BaseNode.next + operator overloads)
//   - The __rshift__ and __sub__ Python operator hooks become plain methods
//     in Go since we can't overload operators.
package main

import (
	"fmt"
	"log"
)

// SharedStore mirrors PocketFlow's `shared` dict (s01).
type SharedStore map[string]any

// Node is the same contract from s01.
type Node interface {
	Prep(shared SharedStore) any
	Exec(prepRes any) any
	Post(shared SharedStore, prepRes any, execRes any) string

	// NEW IN s02 — gives Flow (s03) a way to ask any node for its outgoing edges.
	GetSuccessors() map[string]Node
}

// BaseNode now exposes Next() and NextOn() for graph construction.
type BaseNode struct {
	Params     map[string]any
	Successors map[string]Node
}

func NewBaseNode() *BaseNode {
	return &BaseNode{
		Params:     map[string]any{},
		Successors: map[string]Node{},
	}
}

func (b *BaseNode) Prep(shared SharedStore) any                              { return nil }
func (b *BaseNode) Exec(prepRes any) any                                     { return nil }
func (b *BaseNode) Post(shared SharedStore, prepRes any, execRes any) string { return "default" }

func (b *BaseNode) GetSuccessors() map[string]Node { return b.Successors }

// Next registers a default-action successor. Mirrors PocketFlow's `>>`.
//
//	upstream:  parent >> child
//	ours:      parent.Next(child)
//
// Returns the appended node so callers can chain: parent.Next(child).Next(grand).
func (b *BaseNode) Next(n Node) Node { return b.NextOn("default", n) }

// NextOn registers a named-action successor. Mirrors PocketFlow's `parent - "action" >> child`.
//
//	upstream:  parent - "yes" >> child
//	ours:      parent.NextOn("yes", child)
//
// On overwrite, logs a warning (upstream uses warnings.warn).
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

// RunOnce kept from s01 for nodes-without-successors smoke testing.
func RunOnce(n Node, shared SharedStore) string {
	if len(n.GetSuccessors()) > 0 {
		log.Printf("[warn] Node has successors but RunOnce ignores them; use Flow (s03+).")
	}
	p := n.Prep(shared)
	e := n.Exec(p)
	return n.Post(shared, p, e)
}

// ----- Demo: a 3-node pipeline. We only CONSTRUCT it here; walking comes in s03. -----

// LoadNode reads "raw" from shared and stores "tokens".
type LoadNode struct{ *BaseNode }

func NewLoadNode() *LoadNode { return &LoadNode{BaseNode: NewBaseNode()} }
func (n *LoadNode) Prep(shared SharedStore) any { v, _ := shared["raw"].(string); return v }
func (n *LoadNode) Exec(prepRes any) any        { s, _ := prepRes.(string); return len(s) }
func (n *LoadNode) Post(shared SharedStore, prepRes, execRes any) string {
	shared["tokens"] = execRes
	return "default"
}

// CapsNode uppercases the raw text. No-op when "raw" missing.
type CapsNode struct{ *BaseNode }

func NewCapsNode() *CapsNode { return &CapsNode{BaseNode: NewBaseNode()} }

// CheckEmptyNode returns "empty" if raw is "", else "default".
type CheckEmptyNode struct{ *BaseNode }

func NewCheckEmptyNode() *CheckEmptyNode { return &CheckEmptyNode{BaseNode: NewBaseNode()} }

func main() {
	// Build the graph:
	//
	//     CheckEmpty -- "default" --> Load --> Caps
	//            \--- "empty" -----> Load (degenerate — same destination, just to demo NextOn)
	check := NewCheckEmptyNode()
	load := NewLoadNode()
	caps := NewCapsNode()

	check.Next(load)
	check.NextOn("empty", load)
	load.Next(caps)

	// Inspect the graph (no walking yet).
	fmt.Println("Graph built (no walking — that's s03):")
	for action, succ := range check.GetSuccessors() {
		fmt.Printf("  check[%-7s] -> %T\n", action, succ)
	}
	for action, succ := range load.GetSuccessors() {
		fmt.Printf("  load [%-7s] -> %T\n", action, succ)
	}
	fmt.Printf("  caps successors: %d (terminal)\n", len(caps.GetSuccessors()))

	// Show that RunOnce on a node with successors warns.
	fmt.Println("\nTrying RunOnce on a chained node (expect a warning):")
	RunOnce(check, SharedStore{"raw": "hello"})
}
