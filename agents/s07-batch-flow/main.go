// Package main implements s07 of learn-PocketFlow: BatchFlow (param iteration).
//
// BatchNode (s06) iterates DATA: prep returns a list, exec runs per item.
// BatchFlow iterates PARAMS: prep returns a list of param dicts, the inner
// flow runs once per dict with that dict merged into flow.Params.
//
// Use cases:
//   - Apply 3 different filters to the same image (3 param dicts, same flow)
//   - Translate the same paragraph into 5 languages where the language is
//     in params, not in data (the inner nodes read params["lang"])
//   - Sweep hyperparameters: run the same training flow 10 times with
//     different lr/batch_size combinations.
//
// Upstream reference: pocketflow/__init__.py#L53-L57 (5 lines!).
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
func (b *BaseNode) Next(n Node) Node                                          { return b.NextOn("default", n) }
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

func getNextNode(curr Node, action string) Node {
	if action == "" {
		action = "default"
	}
	succ := curr.GetSuccessors()
	return succ[action]
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

// ----- BatchFlow: s07's contribution -----

// BatchFlow embeds Flow. Its Prep returns []map[string]any — a slice of
// per-iteration param dicts. The runner calls Orchestrate(shared, dict)
// once per dict.
type BatchFlow struct {
	*Flow
}

// NewBatchFlow constructs a BatchFlow that wraps an inner-flow graph.
func NewBatchFlow(start Node) *BatchFlow {
	return &BatchFlow{Flow: NewFlow(start)}
}

// BatchFlowPrep is the contract concrete BatchFlows implement to produce
// per-iteration param dicts. It augments the Node Prep (which returns
// `any`) with a typed []map[string]any return.
type BatchFlowPrep interface {
	PrepBatch(shared SharedStore) []map[string]any
}

// RunBatchFlow does the equivalent of upstream's BatchFlow._run:
//
//	def _run(self,shared):
//	    pr=self.prep(shared) or []
//	    for bp in pr: self._orch(shared,{**self.params,**bp})
//	    return self.post(shared,pr,None)
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

// ----- Demo: process 3 images with 3 different filter configs -----
// (Mirrors upstream's cookbook/pocketflow-batch-flow/ structure.)

// LoadImageNode reads params["filename"] and writes a stub "image bytes"
// into shared["bytes"].
type LoadImageNode struct{ *BaseNode }

func NewLoadImageNode() *LoadImageNode { return &LoadImageNode{BaseNode: NewBaseNode()} }
func (n *LoadImageNode) Post(shared SharedStore, prep, exec any) string {
	fn, _ := n.Params["filename"].(string)
	shared["bytes_"+fn] = strings.Repeat("█", len(fn))
	return "default"
}

// ApplyFilterNode reads params["filter"] and params["filename"], writes
// the "filtered" bytes back.
type ApplyFilterNode struct{ *BaseNode }

func NewApplyFilterNode() *ApplyFilterNode { return &ApplyFilterNode{BaseNode: NewBaseNode()} }
func (n *ApplyFilterNode) Post(shared SharedStore, prep, exec any) string {
	fn, _ := n.Params["filename"].(string)
	filter, _ := n.Params["filter"].(string)
	raw, _ := shared["bytes_"+fn].(string)
	shared["bytes_"+fn] = "[" + filter + "]" + raw
	return "default"
}

// SaveNode appends shared[bytes_X] to a result log.
type SaveNode struct{ *BaseNode }

func NewSaveNode() *SaveNode { return &SaveNode{BaseNode: NewBaseNode()} }
func (n *SaveNode) Post(shared SharedStore, prep, exec any) string {
	fn, _ := n.Params["filename"].(string)
	out, _ := shared["bytes_"+fn].(string)
	log := shared["log"].([]string)
	log = append(log, fmt.Sprintf("%s → %s", fn, out))
	shared["log"] = log
	return "default"
}

// ImageProcessFlow is a BatchFlow whose PrepBatch yields one params dict
// per (filename, filter) combination.
type ImageProcessFlow struct {
	*BatchFlow
}

func NewImageProcessFlow() *ImageProcessFlow {
	load := NewLoadImageNode()
	filter := NewApplyFilterNode()
	save := NewSaveNode()
	load.Next(filter)
	filter.Next(save)
	return &ImageProcessFlow{BatchFlow: NewBatchFlow(load)}
}

func (f *ImageProcessFlow) PrepBatch(shared SharedStore) []map[string]any {
	imgs, _ := shared["images"].([]string)
	filters, _ := shared["filters"].([]string)
	out := make([]map[string]any, 0, len(imgs)*len(filters))
	for _, img := range imgs {
		for _, fl := range filters {
			out = append(out, map[string]any{"filename": img, "filter": fl})
		}
	}
	return out
}

func (f *ImageProcessFlow) Post(shared SharedStore, prep, exec any) string {
	dicts := prep.([]map[string]any)
	shared["batches_run"] = len(dicts)
	return "default"
}

func main() {
	flow := NewImageProcessFlow()
	shared := SharedStore{
		"images":  []string{"cat", "dog"},
		"filters": []string{"sepia", "grayscale"},
		"log":     []string{},
	}
	RunBatchFlow(flow.BatchFlow, flow, shared)

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Batches run: %d (expected 4 = 2 imgs × 2 filters)\n", shared["batches_run"])
	fmt.Println("Log:")
	for _, line := range shared["log"].([]string) {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println(strings.Repeat("=", 60))
}
