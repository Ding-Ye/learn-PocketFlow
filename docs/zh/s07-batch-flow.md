---
title: "s07 · 批量流（参数迭代）"
chapter: 7
slug: s07-batch-flow
est_read_min: 7
---

# s07 · 批量流（参数迭代）

> 教什么：**另一个** batch 轴。s06 的 `BatchNode` 在一个节点里迭代数据 item；`BatchFlow` 用一整个内层 flow 迭代参数集。

---

## Problem / 问题

s06 让一个节点处理 N 个 item。但有时候变化单位比数据大 —— 是 **整个 flow**：

- "用 3 种 filter 处理同一张图" —— 每种 filter 是一个参数集，同一个 load/transform/save flow。
- "用 8 组超参数训同一个模型" —— 每组跑整个训练 flow。
- "把同一段翻译成 5 种语言，语言在 params 里" —— 每个语言重跑 prep/translate/format flow。

我们要的：N 次跑 N 个不同 params dict 的同一节点图。那就是 `BatchFlow`。

## Solution / 解决方案

override `Prep` 返回 `[]map[string]any` —— per-iteration 参数 dict 切片。runner：

```
for _, bp := range prepBatches {
    Orchestrate(shared, mergeParams(flow.Params, bp))   // 整个 flow 走一遍
}
Post(shared, prepBatches, nil)
```

三个关键决策：

1. **`shared` 跨迭代持续，`params` 每次重置** —— 第 1 次写到 shared 的对第 2 次可见。`prepBatches` 是唯一的 per-iteration 变化。
2. **`Post` 在所有迭代后调一次** —— 不是 per iteration。用它写汇总（计数、摘要）。对齐上游 `self.post(shared, pr, None)`。
3. **具体 BatchFlow 实现 `BatchFlowPrep`** —— 类型化接口返回 `[]map[string]any`。和 `Node.Prep`（返回 `any`）区分开。

## How It Works / 工作原理

```ascii-anim frames=2
┌─────────────────────────────────────────────────────────────────────┐
│                       RunBatchFlow                                   │
│                                                                      │
│   prepBatches := node.PrepBatch(shared)   // []map[string]any        │
│                                                                      │
│   for _, bp := range prepBatches {                                   │
│       merged := mergeParams(flow.Params, bp)                         │
│       Flow.Orchestrate(shared, merged)    // 整个 flow 走一遍！      │
│   }                                                                  │
│                                                                      │
│   node.Post(shared, prepBatches, nil)                                │
└─────────────────────────────────────────────────────────────────────┘
```

核心 13 行（节选自 [`agents/s07-batch-flow/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s07-batch-flow/main.go)）：

```go
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
```

**3 个非显然之处**：

1. **每次 `Orchestrate` 走整个内层 flow** —— 不是一个节点。所以叫 BatchFlow，不叫 BatchNode。代价：内层 5 节点，迭代 10 dict，共 50 次节点执行。
2. **`shared` 是 **唯一** 跨迭代通道** —— `params` 每次都用 `mergeParams` 重置。累计输出用 shared；per-iteration 调整用 params。
3. **`Post(shared, prepBatches, nil)`** —— `exec_res` 是 nil 因为 BatchFlow 没有单一的 "exec 结果"，工作发生在内层 flow 运行里。上游传 `None` 同理。

## What Changed / 与 s06 的变化

```diff
+type BatchFlow struct {
+    *Flow
+}
+
+type BatchFlowPrep interface {
+    PrepBatch(shared SharedStore) []map[string]any
+}
+
+func NewBatchFlow(start Node) *BatchFlow { return &BatchFlow{Flow: NewFlow(start)} }
+
+func RunBatchFlow(bf *BatchFlow, sharedNode Node, shared SharedStore) string {
+    var prepBatches []map[string]any
+    if b, ok := sharedNode.(BatchFlowPrep); ok {
+        prepBatches = b.PrepBatch(shared)
+    }
+    for _, bp := range prepBatches {
+        bf.Orchestrate(shared, bp)
+    }
+    return sharedNode.Post(shared, prepBatches, nil)
+}
```

`Flow`、`BaseNode`、编排循环都没改。`BatchFlow` 是薄壳，只是迭代 `Orchestrate`。

## Try It / 动手试一试

```bash
cd agents/s07-batch-flow
go run .
go test -v ./...
```

期望输出：

```
============================================================
Batches run: 4 (expected 4 = 2 imgs × 2 filters)
Log:
  cat → [sepia]█████
  cat → [grayscale]█████
  dog → [sepia]██████
  dog → [grayscale]██████
============================================================
```

## Upstream Source Reading / 上游源码阅读

```upstream:pocketflow/__init__.py#L53-L57
# Source: pocketflow/__init__.py#L53-L57
class BatchFlow(Flow):
    def _run(self,shared):
        pr=self.prep(shared) or []
        for bp in pr: self._orch(shared,{**self.params,**bp})
        return self.post(shared,pr,None)
```

**对照阅读要点**：

- **`pr=self.prep(shared) or []`**：上游允许 prep 返回 None 表示"没有 batch"。我们用 type-assertion 守卫复制 —— 节点不实现 `BatchFlowPrep` 时 `prepBatches` 留 nil，for 跑 0 次。
- **`{**self.params, **bp}` 是 per-iteration 合并**：和 s03 的 `mergeParams` 一样。per-iteration `bp` 在 key 冲突时覆盖 flow-level `self.params`。
- **调 `self._orch(...)` 不是 `self.run(...)`**：上游调内部 `_orch` 跳过自己的 prep/post。否则会无限递归（BatchFlow 的 prep 就是 `pr` 的来源）。我们 Go 端直接调 `Orchestrate`，同理。
- **循环后 `return self.post(shared,pr,None)`**：post 跑一次，拿到完整 `pr`（参数 dict 列表）作为 `prep_res`，`None` 作为 `exec_res`。用参数 dict 列表做汇总；"exec 结果"不存在。
- **BatchFlow 做不到的**：并行迭代。迭代是顺序的。要并行用 `AsyncParallelBatchFlow`（`pocketflow/__init__.py#L96-L100`）—— 留作附录 B 练习题 #4。

**想读更多**：`pocketflow/__init__.py#L59-L74` 是 `AsyncNode`。s08 移植它。Async 和 batching 正交，理解两者后 s09 组合它们。

---

**下一节预告**：s08 引入 `AsyncNode`。把 Python `async def` 生命周期对应到 Go 的 `context.Context` + goroutine 友好方法。
