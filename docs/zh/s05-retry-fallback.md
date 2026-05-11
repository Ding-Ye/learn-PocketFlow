---
title: "s05 · 重试与降级"
chapter: 5
slug: s05-retry-fallback
est_read_min: 8
---

# s05 · 重试与降级

> 教什么：生产级 exec。把工作包在 `MaxRetries` 重试循环里，可选 `Wait`，终态失败时调 `ExecFallback`。

---

## Problem / 问题

s01-s04 用 `Exec(prepRes) any` —— 签名没有 error。算数 OK，调 LLM 和 HTTP 必败。上游的解决方案是 `Node` 类（`pocketflow/__init__.py` L26-34），在 `BaseNode` 之上加了三个状态 —— `max_retries`、`wait`、`cur_retry` —— 和一个可 override 的 `exec_fallback` 方法决定全失败时返回什么。

Go 要做同样的事情，还有一个变种：上游把 `cur_retry` 存在 `self` 上，然后每次 flow run 前 `copy.copy(node)` 隔离计数器。**这** 就是上游每次浅拷贝的唯一理由。如果我们把计数器放到局部变量，整个拷贝步骤可以从 `Flow._orch` 里删掉。

## Solution / 解决方案

加一个 `RetryNode` struct 嵌入 `*BaseNode`，再加一个 `RetryableNode` 接口两个方法：

```go
type RetryableNode interface {
    TryExec(prepRes any) (any, error)
    ExecFallback(prepRes any, err error) any
}
```

`RunWithRetry(node, shared)` 通过 type assertion 检查节点是否实现 `RetryableNode`，分流到重试循环或 s01-s04 的 `Exec` 路径。向后兼容。

三个关键决策：

1. **`cur_retry` 放在 `retryExec` 的局部作用域** —— 故意偏离上游。语义相同，不需要 per-run 拷贝。
2. **`TryExec` 返回 `(any, error)`，`Exec` 不返回 error** —— 让 s01-s04 的 demo 继续能编译；你只通过嵌入 `RetryNode` + 实现 `TryExec` 来"申请"使用 error。
3. **`ExecFallback` 必须实现** —— 没有默认。上游默认 `raise exc`。我们让嵌入者必须思考这一点；最常见的实现是返回 nil + err 或者 sentinel。

## How It Works / 工作原理

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────────┐
│                       RunWithRetry(node, shared)                │
│                                                                 │
│   prepRes = node.Prep(shared)                                   │
│   if node 实现 RetryableNode:                                   │
│       for attempt = 0 .. MaxRetries-1:                          │
│           out, err = node.TryExec(prepRes)                      │
│           if err == nil: return Post(shared, prepRes, out)      │
│           if attempt 是最后一次:                                │
│               return Post(shared, prepRes, ExecFallback(...))   │
│           if Wait > 0: time.Sleep(Wait)                         │
│   else:                                                         │
│       out = node.Exec(prepRes)            // s01-s04 路径        │
│       return node.Post(shared, prepRes, out)                    │
└────────────────────────────────────────────────────────────────┘
```

核心 22 行（节选自 [`agents/s05-retry-fallback/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s05-retry-fallback/main.go)）：

```go
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
        if wait > 0 { time.Sleep(wait) }
    }
    return fallback(prepRes, lastErr)
}
```

**3 个非显然之处**：

1. **`cur_retry` 是栈上变量 `attempt`** —— 没有 `node.CurRetry` 字段。这是 **故意偏离** 上游。结果：同一个 `RetryNode` 可以跨 flow 运行复用，没有状态泄漏风险；Flow 不再需要 `copy.copy` 节点。
2. **`MaxRetries=1` 是"只试一次"** —— 对齐上游默认。循环 `range(max_retries)`，`max_retries=1` 跑一次然后直接进"最后一次？" 分支（是）→ 失败 fallback，或者成功直接返回。
3. **`time.Sleep` 发生在下一次尝试 **之前**，不在最后一次之后** —— `if attempt == maxRetries-1` 先检查；只有"还有下一次"才睡。省掉最后多余的睡眠。

## What Changed / 与 s04 的变化

```diff
+type RetryableNode interface {
+    TryExec(prepRes any) (any, error)
+    ExecFallback(prepRes any, err error) any
+}
+
+type RetryNode struct {
+    *BaseNode
+    MaxRetries int
+    Wait       time.Duration
+}
+
+func NewRetryNode(maxRetries int, wait time.Duration) *RetryNode { ... }
+
+func RunWithRetry(n Node, shared SharedStore) string { ... }
+func retryExec(maxRetries int, wait time.Duration, prepRes any,
+    try func(any) (any, error),
+    fallback func(any, error) any) any { ... }
```

Node interface 和 `Flow.Orchestrate` 都没改。一个节点只有在嵌入 `*RetryNode` **且** 具体类型满足 `RetryableNode`（提供 `TryExec` + `ExecFallback`）时才走重试路径。否则走 s01-s04 的 `Exec` 路径。

## Try It / 动手试一试

```bash
cd agents/s05-retry-fallback
go run .
go test -v ./...
```

期望输出：

```
============================================================
flaky calls = 3 (expected 3)
body = OK 126-byte response from https://example.com
------------------------------------------------------------
hard calls  = 3 (expected 3)
result = FALLBACK-VALUE (fallback)
============================================================
```

## Upstream Source Reading / 上游源码阅读

```upstream:pocketflow/__init__.py#L26-L34
# Source: pocketflow/__init__.py#L26-L34
class Node(BaseNode):
    def __init__(self,max_retries=1,wait=0):
        super().__init__()
        self.max_retries,self.wait=max_retries,wait
    def exec_fallback(self,prep_res,exc): raise exc
    def _exec(self,prep_res):
        for self.cur_retry in range(self.max_retries):
            try: return self.exec(prep_res)
            except Exception as e:
                if self.cur_retry==self.max_retries-1:
                    return self.exec_fallback(prep_res,e)
                if self.wait>0: time.sleep(self.wait)
```

**对照阅读要点**：

- **`self.cur_retry = ...` 在 for 循环里**：上游把循环变量写到实例上。所以你在 `exec` 里 `print(node.cur_retry)` 能看到当前是第几次尝试。我们 Go 不暴露这个；需要的话通过 `prepRes` 传或者拓展接口。
- **`def exec_fallback(self,prep_res,exc): raise exc`**：上游默认是重抛。期望用户 override。我们 Go 版不给默认 —— `RetryableNode` 不继承自抽象基类，嵌入者必须显式实现 `ExecFallback`。更啰嗦但更显眼。
- **`time.sleep(self.wait)` → `time.Sleep(self.Wait)`**：Go 的 `time.Sleep` 接 `time.Duration`；上游接浮点秒。我们选 `time.Duration` 求类型安全；`NewRetryNode(3, 100*time.Millisecond)` 比 `NewRetryNode(3, 0.1)` 清楚。
- **重试循环就是 BaseNode 的全部升级**：L28-34 是全部 diff。其他（`next()`、successors、prep/post）都从 `BaseNode` 继承。
- **为啥 `Flow._orch` 里有 `copy.copy(node)`？** 它存在的 **唯一** 目的就是在 flow 运行之间重置 `cur_retry`。如果你把 `cur_retry` 从实例上拿掉（我们就这么做了），就可以删除拷贝。上游保留是为了对齐 cookbook 里偶尔事后读 `cur_retry` 的写法。

**想读更多**：L36-37 是 s06。`BatchNode` 一行 override `_exec` 迭代 list —— 免费继承整个重试循环，每个 item 独立享受。组合优雅。

---

**下一节预告**：s06 把 `RetryNode` 变成 `BatchNode` —— override `TryExec` 来迭代 list，每个 item 独立享受 retry + fallback。
