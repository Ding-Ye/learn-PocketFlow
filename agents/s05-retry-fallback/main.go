// Package main implements s05 of learn-PocketFlow: retry + fallback.
//
// s01-s04 used "best-effort" exec that returns whatever it returns. Real
// agents call LLMs and HTTP endpoints that fail. PocketFlow's `Node` (an
// upgrade over `BaseNode`) wraps exec in a retry loop with optional
// per-attempt sleep, and falls back to a user-supplied ExecFallback on the
// final failure.
//
// Upstream reference: pocketflow/__init__.py#L26-L34 + tests/test_fall_back.py.
//
// Key Go vs Python divergence: upstream stores `cur_retry` on `self` (the
// node instance). We push it into the retry loop's local scope, removing
// the need for the `copy.copy(node)` step upstream uses for per-run
// isolation. See docs/en/s05-retry-fallback.md for the reasoning.
package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

type SharedStore map[string]any

// Exec now returns (any, error). BaseNode keeps the no-error shape for
// compatibility with s01-s04 demos; RetryNode introduces the error-aware
// variant `TryExec(prepRes) (any, error)`.
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

// ----- RetryNode: the s05 contribution -----

// RetryableNode is what RetryNode-derived nodes implement. Note the
// error-aware signatures: TryExec MAY fail; ExecFallback decides what to
// return when all retries are exhausted.
type RetryableNode interface {
	TryExec(prepRes any) (any, error)
	ExecFallback(prepRes any, err error) any
}

// RetryNode embeds BaseNode and adds retry config. Concrete nodes embed
// *RetryNode and implement TryExec + ExecFallback.
type RetryNode struct {
	*BaseNode
	MaxRetries int           // total attempts; default 1 (no retry)
	Wait       time.Duration // sleep between retries; default 0
}

func NewRetryNode(maxRetries int, wait time.Duration) *RetryNode {
	if maxRetries < 1 {
		maxRetries = 1
	}
	return &RetryNode{BaseNode: NewBaseNode(), MaxRetries: maxRetries, Wait: wait}
}

// RunWithRetry orchestrates prep → retry-loop(exec) → post for a RetryableNode.
// This replaces the simpler runLifecycle from s03-s04 when the node is retryable.
func RunWithRetry(n Node, shared SharedStore) string {
	prepRes := n.Prep(shared)

	var execRes any
	if r, ok := n.(RetryableNode); ok {
		execRes = retryExec(n.(interface{ GetMaxRetries() int }).GetMaxRetries(),
			n.(interface{ GetWait() time.Duration }).GetWait(),
			prepRes, r.TryExec, r.ExecFallback)
	} else {
		// Plain BaseNode path — no retry, just Exec.
		execRes = n.Exec(prepRes)
	}

	return n.Post(shared, prepRes, execRes)
}

// retryExec is the actual loop. Crucially, cur_retry lives here on the stack,
// NOT on the node, so a node can be safely reused across flow runs.
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
	// Unreachable when maxRetries > 0, but Go's compiler can't prove it.
	return fallback(prepRes, lastErr)
}

// Accessors so the interface assertion in RunWithRetry doesn't need direct field access.
func (r *RetryNode) GetMaxRetries() int        { return r.MaxRetries }
func (r *RetryNode) GetWait() time.Duration    { return r.Wait }

// ----- Demo: a node that fails the first 2 attempts, succeeds on the 3rd -----

// FlakyHTTPNode pretends to fetch a URL; first calls fail with a stub error,
// final attempt succeeds. Demonstrates retry without exhausting.
type FlakyHTTPNode struct {
	*RetryNode
	calls int // counter to simulate flake
}

func NewFlakyHTTPNode() *FlakyHTTPNode {
	return &FlakyHTTPNode{RetryNode: NewRetryNode(3, 10*time.Millisecond)}
}

func (n *FlakyHTTPNode) TryExec(prepRes any) (any, error) {
	n.calls++
	url, _ := prepRes.(string)
	if n.calls < 3 {
		return nil, fmt.Errorf("transient HTTP error (attempt %d) on %s", n.calls, url)
	}
	return fmt.Sprintf("OK %d-byte response from %s", 42*n.calls, url), nil
}

func (n *FlakyHTTPNode) ExecFallback(prepRes any, err error) any {
	return fmt.Sprintf("FALLBACK: %v", err)
}

func (n *FlakyHTTPNode) Prep(shared SharedStore) any { v, _ := shared["url"].(string); return v }
func (n *FlakyHTTPNode) Post(shared SharedStore, prepRes, execRes any) string {
	shared["body"] = execRes
	return "default"
}

// HardFailingNode always fails — ExecFallback returns a sentinel.
type HardFailingNode struct {
	*RetryNode
	calls int
}

func NewHardFailingNode() *HardFailingNode {
	return &HardFailingNode{RetryNode: NewRetryNode(3, 1*time.Millisecond)}
}

func (n *HardFailingNode) TryExec(prepRes any) (any, error) {
	n.calls++
	return nil, errors.New("permanent failure")
}
func (n *HardFailingNode) ExecFallback(prepRes any, err error) any {
	return "FALLBACK-VALUE"
}
func (n *HardFailingNode) Post(shared SharedStore, prepRes, execRes any) string {
	shared["result"] = execRes
	return "default"
}

func main() {
	fmt.Println(strings.Repeat("=", 60))

	// Demo 1: flake recovers after 2 retries
	flaky := NewFlakyHTTPNode()
	shared := SharedStore{"url": "https://example.com"}
	RunWithRetry(flaky, shared)
	fmt.Printf("flaky calls = %d (expected 3)\n", flaky.calls)
	fmt.Printf("body = %v\n", shared["body"])

	fmt.Println(strings.Repeat("-", 60))

	// Demo 2: hard-failing falls back
	hard := NewHardFailingNode()
	shared2 := SharedStore{}
	RunWithRetry(hard, shared2)
	fmt.Printf("hard calls  = %d (expected 3)\n", hard.calls)
	fmt.Printf("result = %v (fallback)\n", shared2["result"])

	fmt.Println(strings.Repeat("=", 60))
}
