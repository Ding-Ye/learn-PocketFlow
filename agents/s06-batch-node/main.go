// Package main implements s06 of learn-PocketFlow: BatchNode (map pattern).
//
// Take the s05 RetryNode and override TryExec to iterate over a list of
// items. Each item gets independent retry-with-fallback semantics. The
// node's Prep returns the list; Exec processes one item at a time
// underneath; Post receives the full slice of per-item results.
//
// Upstream reference: pocketflow/__init__.py#L36-L37 (one-liner!) and
// cookbook/pocketflow-batch/main.py (translate-text-to-N-languages demo).
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
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

type RetryNode struct {
	*BaseNode
	MaxRetries int
	Wait       time.Duration
}

func NewRetryNode(maxRetries int, wait time.Duration) *RetryNode {
	if maxRetries < 1 {
		maxRetries = 1
	}
	return &RetryNode{BaseNode: NewBaseNode(), MaxRetries: maxRetries, Wait: wait}
}

// retryExec stays unchanged from s05 — per-attempt retry with fallback.
func retryExec(maxRetries int, wait time.Duration, prepRes any,
	try func(any) (any, error),
	fallback func(any, error) any,
) any {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		out, err := try(prepRes)
		if err == nil {
			return out
		}
		lastErr = err
		if attempt == maxRetries-1 {
			return fallback(prepRes, err)
		}
		if wait > 0 {
			time.Sleep(wait)
		}
	}
	return fallback(prepRes, lastErr)
}

// ----- BatchNode: s06's contribution -----

// BatchItemNode is what BatchNode-derived nodes implement. The runner
// calls TryExecItem once per item; each item gets full retry+fallback.
type BatchItemNode interface {
	TryExecItem(item any) (any, error)
	ExecFallbackItem(item any, err error) any
}

// BatchNode embeds RetryNode. Its TryExec iterates Prep's output as a
// slice, applying retryExec to each element. The MaxRetries/Wait config
// from RetryNode is reused PER ITEM.
type BatchNode struct {
	*RetryNode
}

func NewBatchNode(maxRetries int, wait time.Duration) *BatchNode {
	return &BatchNode{RetryNode: NewRetryNode(maxRetries, wait)}
}

// RunBatch is the s06 equivalent of s05's RunWithRetry: it expects Prep to
// return []any (or anything convertible) and produces []any results.
func RunBatch(n Node, shared SharedStore) string {
	prepRes := n.Prep(shared)
	items, _ := prepRes.([]any)

	results := make([]any, 0, len(items))
	if b, ok := n.(BatchItemNode); ok {
		maxRetries := n.(interface{ GetMaxRetries() int }).GetMaxRetries()
		wait := n.(interface{ GetWait() time.Duration }).GetWait()
		for _, item := range items {
			item := item
			r := retryExec(maxRetries, wait, item,
				func(p any) (any, error) { return b.TryExecItem(p) },
				func(p any, err error) any { return b.ExecFallbackItem(p, err) },
			)
			results = append(results, r)
		}
	} else {
		// Non-batch node fallback (unlikely in s06 but keeps the runner robust).
		for _, item := range items {
			results = append(results, n.Exec(item))
		}
	}
	return n.Post(shared, prepRes, results)
}

func (r *RetryNode) GetMaxRetries() int     { return r.MaxRetries }
func (r *RetryNode) GetWait() time.Duration { return r.Wait }

// ----- Demo: translate one English string into N languages -----

// TranslateNode mocks an LLM translation call. Each (text, lang) tuple is
// one "API call"; the demo simulates one transient failure on French to
// show per-item retry working.
type TranslateNode struct {
	*BatchNode
	failOnceFor map[string]bool // language → whether to fail the first attempt
}

func NewTranslateNode() *TranslateNode {
	return &TranslateNode{
		BatchNode:   NewBatchNode(3, 5*time.Millisecond),
		failOnceFor: map[string]bool{"fr": true}, // mock transient failure on French
	}
}

func (n *TranslateNode) Prep(shared SharedStore) any {
	text, _ := shared["text"].(string)
	langs, _ := shared["languages"].([]string)
	items := make([]any, 0, len(langs))
	for _, l := range langs {
		items = append(items, [2]string{text, l})
	}
	return items
}

func (n *TranslateNode) TryExecItem(item any) (any, error) {
	pair, _ := item.([2]string)
	text, lang := pair[0], pair[1]
	if n.failOnceFor[lang] {
		n.failOnceFor[lang] = false // succeed on retry
		return nil, fmt.Errorf("transient API error for %s", lang)
	}
	return fmt.Sprintf("[%s] %s", strings.ToUpper(lang), text), nil
}

func (n *TranslateNode) ExecFallbackItem(item any, err error) any {
	return fmt.Sprintf("FALLBACK: %v", err)
}

func (n *TranslateNode) Post(shared SharedStore, prepRes, execRes any) string {
	shared["translations"] = execRes
	return "default"
}

// PermBadNode always fails for "es" — used by tests to show per-item fallback.
type PermBadNode struct{ *BatchNode }

func NewPermBadNode() *PermBadNode { return &PermBadNode{BatchNode: NewBatchNode(2, 0)} }
func (n *PermBadNode) TryExecItem(item any) (any, error) {
	s, _ := item.(string)
	if s == "es" {
		return nil, errors.New("es never works")
	}
	return s + "-ok", nil
}
func (n *PermBadNode) ExecFallbackItem(item any, err error) any { return "ITEM-FALLBACK" }
func (n *PermBadNode) Prep(shared SharedStore) any              { return shared["items"] }
func (n *PermBadNode) Post(shared SharedStore, prep, exec any) string {
	shared["out"] = exec
	return "default"
}

func main() {
	tr := NewTranslateNode()
	shared := SharedStore{
		"text":      "Hello, world!",
		"languages": []string{"es", "fr", "de", "zh"},
	}
	RunBatch(tr, shared)

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("Translations:")
	for _, v := range shared["translations"].([]any) {
		fmt.Printf("  %v\n", v)
	}
	fmt.Println(strings.Repeat("=", 60))
}
