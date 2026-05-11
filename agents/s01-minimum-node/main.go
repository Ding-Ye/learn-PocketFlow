// Package main implements s01 of learn-PocketFlow: the minimal Node lifecycle.
//
// The whole point of this session: PocketFlow's atomic unit is a Node with
// three lifecycle methods — Prep, Exec, Post. Everything later (chaining,
// orchestration, retry, batching, async) is built on top of this trio.
//
// Upstream reference: pocketflow/__init__.py#L3-L20 (BaseNode class).
package main

import (
	"fmt"
	"log"
	"strings"
)

// SharedStore is the inter-node communication channel: a mutable map passed
// by reference to every node in a flow. Mirrors PocketFlow's `shared` dict.
type SharedStore map[string]any

// Node is the contract every PocketFlow node satisfies.
//
//   - Prep reads inputs from `shared` and returns a value handed to Exec.
//   - Exec does the actual work (later sessions wrap it with retry).
//   - Post writes results back to `shared` and returns an action string
//     that drives Flow routing (covered in s04).
type Node interface {
	Prep(shared SharedStore) any
	Exec(prepRes any) any
	Post(shared SharedStore, prepRes any, execRes any) string
}

// BaseNode is the embeddable default: empty params, empty successors, and
// no-op lifecycle methods. Real nodes embed *BaseNode and override what
// they need. Successors stays unused until s02; we keep the field here to
// preserve the upstream class shape.
type BaseNode struct {
	Params     map[string]any
	Successors map[string]Node
}

// NewBaseNode initializes the maps so embedding nodes don't have to.
func NewBaseNode() *BaseNode {
	return &BaseNode{
		Params:     map[string]any{},
		Successors: map[string]Node{},
	}
}

// Default lifecycle: no-ops. Overrides are method-shadowed by the embedding
// struct because Go's method set on a pointer receiver wins.
func (b *BaseNode) Prep(shared SharedStore) any                              { return nil }
func (b *BaseNode) Exec(prepRes any) any                                     { return nil }
func (b *BaseNode) Post(shared SharedStore, prepRes any, execRes any) string { return "default" }

// RunOnce executes a single node's full lifecycle and returns the action
// post() decided. It is the equivalent of upstream's BaseNode._run.
//
// If the node has successors registered, RunOnce warns — the caller
// should be using a Flow (s03) instead. PocketFlow does the same:
//
//	if self.successors: warnings.warn("Node won't run successors. Use Flow.")
func RunOnce(n Node, shared SharedStore) string {
	if b, ok := n.(interface{ HasSuccessors() bool }); ok && b.HasSuccessors() {
		log.Printf("[warn] Node has successors but RunOnce ignores them; use Flow (s03+).")
	}
	p := n.Prep(shared)
	e := n.Exec(p)
	return n.Post(shared, p, e)
}

// HasSuccessors lets RunOnce introspect a node without depending on the
// concrete BaseNode type — keeps the contract loose.
func (b *BaseNode) HasSuccessors() bool { return len(b.Successors) > 0 }

// ----- Demo: a hello-world AnswerNode mirroring upstream's example -----

// AnswerNode reads a question from shared, "answers" it (no LLM yet —
// s10 wires in a Provider), and writes the answer back.
//
// Upstream analogue: cookbook/pocketflow-hello-world/flow.py#L6-L18.
type AnswerNode struct {
	*BaseNode
}

func NewAnswerNode() *AnswerNode {
	return &AnswerNode{BaseNode: NewBaseNode()}
}

func (a *AnswerNode) Prep(shared SharedStore) any {
	q, _ := shared["question"].(string)
	return q
}

func (a *AnswerNode) Exec(prepRes any) any {
	question, _ := prepRes.(string)
	if question == "" {
		return "I cannot answer an empty question."
	}
	return fmt.Sprintf("Mock answer to: %q (length=%d)", question, len(question))
}

func (a *AnswerNode) Post(shared SharedStore, prepRes any, execRes any) string {
	if ans, ok := execRes.(string); ok {
		shared["answer"] = ans
	}
	return "default"
}

func main() {
	shared := SharedStore{
		"question": "What is PocketFlow?",
	}

	node := NewAnswerNode()
	action := RunOnce(node, shared)

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Question : %s\n", shared["question"])
	fmt.Printf("Answer   : %s\n", shared["answer"])
	fmt.Printf("Action   : %q\n", action)
	fmt.Println(strings.Repeat("=", 60))
}
