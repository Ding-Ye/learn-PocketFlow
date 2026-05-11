---
title: "s06 · 批量节点（map 模式）"
chapter: 6
slug: s06-batch-node
est_read_min: 7
---

# s06 · 批量节点（map 模式）

> 教什么：把 `RetryNode` 改造成 map 操作。`Prep` 返回 list；runner 对每个 item 跑一遍 retry+fallback 循环；`Post` 收到结果切片。

---

## Problem / 问题

s05 处理"调一次，失败重试"。但 agent 经常批处理：
- "把这段翻译成 ES / FR / DE / ZH"
- "embed 这 200 个文档块"
- "总结这 50 个 GitHub issues"

你可以在 `Exec` 里写 for 循环，但会失去 per-item 重试和降级。如果第 3 项失败，原始循环要么整个 crash，要么静默跳过。

PocketFlow 的 `BatchNode`（上游 `pocketflow/__init__.py#L36-L37`）是一个 **一行** 子类：override `_exec`，对每个 item 调 `super()._exec(item)` —— 自动继承 `Node` 那层 retry，**每个 item 各自享受**。优雅。

## Solution / 解决方案

新接口 `BatchItemNode` 有 `TryExecItem(item) (any, error)` + `ExecFallbackItem(item, err) any`。`BatchNode` 嵌入 `RetryNode`。runner `RunBatch(n, shared)` 做：

1. 从 `Prep` 拿 slice。
2. 对每个 item 调 `retryExec(MaxRetries, Wait, item, TryExecItem, ExecFallbackItem)`。
3. 收集到 `[]any`。
4. 把 slice 传给 `Post`。

三个关键决策：

1. **`MaxRetries` 和 `Wait` 是 per-item 的** —— 一个 item flaky 只它重试，其他正常通过。
2. **Per-item fallback 返回值而非 error** —— 失败不中止 batch。fallback 的返回值进结果 slice，post 可以统一处理所有 item。
3. **Prep 返回 `[]any`，不用泛型** —— Go 泛型让接口复杂。我们牺牲类型安全换接口简洁；用户在 `TryExecItem` 内 type-assert。

## How It Works / 工作原理

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────────────┐
│                          RunBatch (s06)                              │
│                                                                     │
│   items := Prep(shared)        // 返回 []any                         │
│   results := []any{}                                                │
│   for _, item := range items {                                      │
│       r := retryExec(MaxRetries, Wait, item,                        │
│                      TryExecItem, ExecFallbackItem)                 │
│       results = append(results, r)                                  │
│   }                                                                 │
│   return Post(shared, items, results)                               │
└────────────────────────────────────────────────────────────────────┘
```

核心 18 行（节选自 [`agents/s06-batch-node/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s06-batch-node/main.go)）：

```go
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
    }
    return n.Post(shared, prepRes, results)
}
```

**3 个非显然之处**：

1. **每个 item 的重试计数器独立** —— `retryExec` per item 调一次，每个 item 自己的 attempt 计数器。item 1 需要 2 次、item 2 需要 3 次都没问题。上游靠 Python MRO（`BatchNode(Node)` 调 `super()._exec` 启动新的 `cur_retry` for 循环）免费得到。
2. **adapter func 是必要的** —— Go 不能直接把绑定的方法当 `func(any) (any, error)` 传。两行包装不可避免。
3. **`item := item` 影子** —— Go 1.22 之前 range 捕获是引用。我们 `go.mod` 已经 1.22，但保留 shadow 表达清晰，也兼容老版本读者。

## What Changed / 与 s05 的变化

```diff
+type BatchItemNode interface {
+    TryExecItem(item any) (any, error)
+    ExecFallbackItem(item any, err error) any
+}
+
+type BatchNode struct {
+    *RetryNode
+}
+
+func NewBatchNode(maxRetries int, wait time.Duration) *BatchNode { ... }
+
+func RunBatch(n Node, shared SharedStore) string { ... }
```

`retryExec` 没改。`BatchNode` 真的很薄 —— 重活儿都在 s05。

## Try It / 动手试一试

```bash
cd agents/s06-batch-node
go run .
go test -v ./...
```

期望输出：

```
============================================================
Translations:
  [ES] Hello, world!
  [FR] Hello, world!
  [DE] Hello, world!
  [ZH] Hello, world!
============================================================
```

注意 FR 成功 —— demo 模拟了 French 的一次瞬时失败，`retryExec` 恢复了。

## Upstream Source Reading / 上游源码阅读

```upstream:pocketflow/__init__.py#L36-L37
# Source: pocketflow/__init__.py#L36-L37
class BatchNode(Node):
    def _exec(self,items):
        return [super(BatchNode,self)._exec(i) for i in (items or [])]
```

**对照阅读要点**：

- **一行有意义的代码**：`[super(BatchNode,self)._exec(i) for i in (items or [])]`。就这。list comprehension 对每个 item 调 `Node._exec`（s05 的 retry 循环）。
- **`super(BatchNode,self)._exec(i)`**：显式跳过 `BatchNode._exec` 防止无限递归。我们 Go 版用 `retryExec` 自由函数，不需要继承链跳跃。
- **`items or []`**：Python 防御式 nil 守卫。prep 返回 None 当空。我们 Go 端 `items, _ := prepRes.([]any)` —— items 默认 nil，range nil 安全。
- **没有 per-batch fallback** —— 只有 per-item。上游 `BatchNode` 没有自己的 `exec_fallback`；继承 `Node` 的，per item 调用。我们 Go 同样：`ExecFallbackItem` 每个失败 item 跑一次，从不为整个 batch 跑。
- **为啥这么优雅？** `BatchNode` 不重新发明 retry，而是 **组合** 在 `Node` 上。`super()._exec` 让每个 item 跑同一个 L30 的 `for self.cur_retry in range(self.max_retries)`。继承式复用，Python 风格。

**想读更多**：`pocketflow/__init__.py#L53-L57` 是 `BatchFlow` —— 和 `BatchNode` 正交。`BatchNode` 迭代 **数据**，`BatchFlow` 迭代 **参数**。同名族，不同轴。那是 s07。

---

**下一节预告**：s07 引入 `BatchFlow`。同名族，反向轴：迭代参数集而非数据 item。
