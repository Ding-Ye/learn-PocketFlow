---
title: "s01 · 最小节点循环"
chapter: 1
slug: s01-minimum-node
est_read_min: 8
---

# s01 · 最小节点循环

> 教什么：PocketFlow 的原子单位 —— 拥有 `Prep → Exec → Post` 生命周期的 `Node`。后续所有章节（串接、编排、重试、批量、异步、RAG）都是在这三个方法之上的延伸。

---

## Problem / 问题

我们想通过 Go 一节一节重新实现 PocketFlow。但在把多个节点串起来、按条件路由、失败重试、扇出批量执行之前 —— 我们得先搞清楚 **单个节点** 长什么样。上游 Python 框架以 99 行闻名，第 3-20 行定义的 `BaseNode` 是一切的基础。这里写错，后续每一节都会继承同一个 bug。

那么：最小可能的 PocketFlow 节点是什么？最小可能的"运行一个节点"又是什么？还没有 Flow，还没有 successors，还没有 retry。只有一个节点、一个 shared 字典、三个方法。

## Solution / 解决方案

一个节点就是一个有三个方法的值：`Prep` 从共享字典读、`Exec` 干活、`Post` 把结果写回去并返回一个路由提示。Go 端的建模是：一个 `Node` interface、一个可嵌入用的 `BaseNode` struct、一个 `RunOnce(node, shared)` 辅助函数把三个方法按顺序调一遍。

三个关键决策：

1. **`SharedStore = map[string]any`** —— 完全对齐 Python dict。不用泛型，因为 shared-store 模式的精髓就是松耦合。
2. **存在 `BaseNode` 是为了让嵌入它的节点拿到零参数默认实现** —— Go 没有方法默认值，所以我们用 struct embedding 实现 Python 用类继承做的事情。需要的方法 override，不需要的继承。
3. **`RunOnce` 是自由函数而非方法** —— 它不该属于 `BaseNode`，因为 s03 的 Flow 会把它取代。自由函数让 s01 → s03 的过渡更干净。

## How It Works / 工作原理

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────┐
│                    SharedStore (map)                       │
│   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐       │
│   │ "question"   │ │ "answer"     │ │ ...          │       │
│   └──────┬───────┘ └──────▲───────┘ └──────────────┘       │
│          │ 读             │ 写                             │
│   ┌──────▼────────────────┴───────────────────────┐        │
│   │             AnswerNode                         │        │
│   │  Prep(shared) ─► Exec(prep) ─► Post(shared,…)  │        │
│   │      │              │                │         │        │
│   │      ▼              ▼                ▼         │        │
│   │   "question"     "Mock answer..."  action="default"     │
│   └──────────────────────────────────────────────────┘     │
└────────────────────────────────────────────────────────────┘
```

核心 30 行（节选自 [`agents/s01-minimum-node/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s01-minimum-node/main.go)）：

```go
type SharedStore map[string]any

type Node interface {
    Prep(shared SharedStore) any
    Exec(prepRes any) any
    Post(shared SharedStore, prepRes any, execRes any) string
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

func RunOnce(n Node, shared SharedStore) string {
    if b, ok := n.(interface{ HasSuccessors() bool }); ok && b.HasSuccessors() {
        log.Printf("[warn] Node has successors but RunOnce ignores them; use Flow (s03+).")
    }
    p := n.Prep(shared)
    e := n.Exec(p)
    return n.Post(shared, p, e)
}
```

**4 个非显然之处**：

1. **`Successors` 字段已经在 BaseNode 里了** —— 虽然 s01 不用串接，但我们提前留好这个字段，后续章节不需要回头改类型。这和上游 `BaseNode.__init__` 第 4 行同时初始化 `params` 和 `successors` 的做法对齐。
2. **`RunOnce` 是警告而不是报错** —— 如果你给节点设了 successors 还直接调 `RunOnce`，那你想要的其实是 Flow。上游用 `warnings.warn`，我们用 `log.Printf`（Go 没有 warnings 包）。两种做法都是"可恢复的提示"。
3. **action 字符串必须返回** —— `Post` 一定要返回点什么。返回 `""` 等价于让 Flow（s04）按 `"default"` 处理。我们让这一点显式：`BaseNode.Post` 直接返回 `"default"`。
4. **`*BaseNode` 是指针嵌入** —— 表明每个节点持有独立的字段而不是共享。即使值嵌入也能工作（因为每次 `NewAnswerNode()` 都会复制），指针形式让意图更清楚。

## What Changed / 与前一节的变化

这是第一节，没有 s00 可以对比。学习之旅的起点就是"空白"。s01 完成后，你拥有：

- `Node` interface
- `BaseNode` struct（含 `Params`、`Successors`）
- `RunOnce` 辅助函数
- 一个 demo：`AnswerNode`

## Try It / 动手试一试

```bash
cd agents/s01-minimum-node
go run .
go test -v ./...
```

期望输出：

```
============================================================
Question : What is PocketFlow?
Answer   : Mock answer to: "What is PocketFlow?" (length=19)
Action   : "default"
============================================================
```

测试套件：

```
=== RUN   TestAnswerNode_ReadsAndWrites
--- PASS: TestAnswerNode_ReadsAndWrites (0.00s)
=== RUN   TestAnswerNode_PostReturnsDefault
--- PASS: TestAnswerNode_PostReturnsDefault (0.00s)
=== RUN   TestRunOnce_LifecycleOrder
--- PASS: TestRunOnce_LifecycleOrder (0.00s)
=== RUN   TestSharedStore_MutationVisible
--- PASS: TestSharedStore_MutationVisible (0.00s)
=== RUN   TestRunOnce_WarnsOnSuccessors
--- PASS: TestRunOnce_WarnsOnSuccessors (0.00s)
PASS
```

## Upstream Source Reading / 上游源码阅读

上游的对应是 [`pocketflow/__init__.py`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L3-L20) 第 3-20 行的 `BaseNode`：

```python
# Source: pocketflow/__init__.py#L3-L20
class BaseNode:
    def __init__(self): self.params,self.successors={},{}
    def set_params(self,params): self.params=params
    def next(self,node,action="default"):
        if action in self.successors: warnings.warn(f"Overwriting successor for action '{action}'")
        self.successors[action]=node; return node
    def prep(self,shared): pass
    def exec(self,prep_res): pass
    def post(self,shared,prep_res,exec_res): pass
    def _exec(self,prep_res): return self.exec(prep_res)
    def _run(self,shared): p=self.prep(shared); e=self._exec(p); return self.post(shared,p,e)
    def run(self,shared):
        if self.successors: warnings.warn("Node won't run successors. Use Flow.")
        return self._run(shared)
    def __rshift__(self,other): return self.next(other)
    def __sub__(self,action):
        if isinstance(action,str): return _ConditionalTransition(self,action)
        raise TypeError("Action must be a string")
```

**对照阅读要点**：

- **`_exec` 是个间接包装**：上游同时有 `exec()`（用户 override 的目标）和 `_exec()`（调用方使用的入口）。在 `BaseNode` 里两者完全一致 —— 但 s05 的 `Node` 会用 `_exec` 包一层重试。我们 Go 版的 s01 暂时跳过这个接缝，到 s05 再以闭包形式重新引入。
- **`next()` 返回追加的那个节点**：让你可以流式串接 `parent.next(child).next(grandchild)`。s02 才会用到，s01 只保留 `Successors` map。
- **`>>` 和 `-` 算符重载**：第 17-20 行让 `nodeA >> nodeB` 和 `nodeA - "action" >> nodeB` 成为可能。Go 不能重载运算符，所以 s02 会用 `Next(node)` 和 `NextOn("action", node)` 方法代替。
- **`set_params` 在 L5 就有了**：BaseNode 层级就支持外部注入参数。我们 s03（Flow）才会用上 —— 但 `Params` 字段从 s01 开始就在 struct 里。
- **`run()` 先警告再调 `_run()`**：发现有 successors 时不报错而是警告 —— 这正是我们 `RunOnce` 的形态。给一个节点设了 successors 还直接调 `run()` 是个范畴错误（你想要的是 Flow），但可以恢复，所以警告更合适。

**想读更多**：从 `pocketflow/__init__.py#L3-L20` 入手，s02 会把 L6 的 `next()` 和 L22-24 的 `_ConditionalTransition` 拿过来 → s03 在 L39-51 的 `Flow` 之上构建图编排，遍历 successors。

---

**下一节预告**：s02 让节点可以串接。我们会给 `BaseNode` 加 `Next(other)` 和 `NextOn(action, other)` 方法，从而构建有向图（但还不会运行）。
