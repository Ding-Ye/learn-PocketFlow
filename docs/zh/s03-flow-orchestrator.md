---
title: "s03 · 图编排器"
chapter: 3
slug: s03-flow-orchestrator
est_read_min: 9
---

# s03 · 图编排器

> 教什么：`Flow` 怎么遍历 s02 搭好的有向图。引入 `params` 和 `shared` 的区别 —— 每个 PocketFlow 节点都会看到的两个字典。

---

## Problem / 问题

s02 让我们能搭出图：`greet.Next(upper); upper.Next(count)`。但没人去运行它。如果调 `RunOnce(greet, shared)` 会收到警告（"你想要的是 Flow"），而且只有第一个节点被执行。

我们需要一个 **编排器** —— 给定起始节点，自动遍历 successors，按顺序调用每个节点的 prep/exec/post。那就是 `Flow`。上游用 13 行（`pocketflow/__init__.py#L39-L51`）完成；我们 Go 大约 30 行，因为得把 Python 语法藏起来的东西拼出来。

## Solution / 解决方案

`Flow` 是个 struct，存 `Start` 节点。`Orchestrate(shared, params)` 循环：

```
curr := f.Start
for curr != nil {
    curr.SetParams(merged_params)
    last = prep_exec_post(curr, shared)
    curr = curr.successors[last]   // s04 会细化 "default" fallback 语义
}
```

三个关键决策：

1. **`Flow` 自己也是 Node** —— 嵌入 `*BaseNode`，所以 flow 可以套 flow。s03 demo 里不展示嵌套（单层 flow），但类型形态是对的。
2. **`params` 是 per-orchestration 的，`shared` 是 per-walk 的** —— 都是 `map[string]any`。`Flow.Orchestrate` 允许调用方传 per-call params 覆盖 flow 级别 params，而 `shared` 只是原样穿过。
3. **我们 **不** 对每个节点 shallow-copy** —— 上游用 `copy.copy(curr)` 隔离 `cur_retry` 之类的 per-run 状态。我们 s05 会把它推到局部变量里，所以不需要 copy。这是 **故意的偏离**，s05 文档里会解释。

## How It Works / 工作原理

```ascii-anim frames=2
┌─────────────────────────────────────────────────────────────────────────┐
│                          Flow.Orchestrate                                │
│                                                                          │
│   ┌──────────────┐                                                       │
│   │  Start node  │   ◀──── curr = f.Start                                │
│   │  (greet)     │                                                       │
│   └──────┬───────┘                                                       │
│          │  SetParams(merged)                                            │
│          │  Prep → Exec → Post → "default"                               │
│          │  curr = curr.Successors["default"]                            │
│          ▼                                                               │
│   ┌──────────────┐                                                       │
│   │  upper       │                                                       │
│   └──────┬───────┘                                                       │
│          │  ... 同样的生命周期                                           │
│          ▼                                                               │
│   ┌──────────────┐                                                       │
│   │  count       │ → Successors["default"] = nil → 循环退出              │
│   └──────────────┘                                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

核心 30 行（节选自 [`agents/s03-flow-orchestrator/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s03-flow-orchestrator/main.go)）：

```go
type Flow struct {
    *BaseNode
    Start Node
}

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

func getNextNode(curr Node, action string) Node {
    if action == "" { action = "default" }
    return curr.GetSuccessors()[action]
}

func mergeParams(base, overrides map[string]any) map[string]any {
    out := map[string]any{}
    for k, v := range base { out[k] = v }
    for k, v := range overrides { out[k] = v }
    return out
}
```

**4 个非显然之处**：

1. **空 action 归一化成 "default"** —— 上游 `curr.successors.get(action or "default")`，`or "default"` 同时处理 `None` 和 `""`。我们 Go 的 `if action == ""` 覆盖同样的边界。
2. **`mergeParams` 生成新 map** —— 从不修改 `base` 或 `overrides`。重要：Flow 的 `Params` 是和调用方共享的；如果改了它，下次 `Orchestrate` 就会有惊喜。
3. **`Flow` 嵌入 `*BaseNode`** —— 不是值嵌入。指针形式让 Flow 自己也能进链：`outerFlow.Next(otherNode)` 有效，因为方法集保留下来了。
4. **循环体只有 3 行，没有递归** —— 即使 Flow 遍历的是树形图。诀窍：每个节点对每个 action **最多一个 successor**，所以在任意特定 action 序列下，图就退化成一条单路径。递归是多余的复杂度。

## What Changed / 与 s02 的变化

```diff
 type Node interface {
     Prep(shared SharedStore) any
     Exec(prepRes any) any
     Post(shared SharedStore, prepRes any, execRes any) string
     GetSuccessors() map[string]Node
+    SetParams(p map[string]any)
 }

+type Flow struct {
+    *BaseNode
+    Start Node
+}
+
+func NewFlow(start Node) *Flow { ... }
+func (f *Flow) Orchestrate(shared SharedStore, params map[string]any) string { ... }
+func (f *Flow) Run(shared SharedStore) string { return f.Orchestrate(shared, nil) }
+
+func mergeParams(base, overrides map[string]any) map[string]any { ... }
+func getNextNode(curr Node, action string) Node { ... }
+func runLifecycle(n Node, shared SharedStore) string { ... }
```

s01-s02 的 `RunOnce` 消失了 —— `Orchestrate` 取代它。对 1 节点的图效果一样（`flow.Orchestrate(shared, nil)` 走一步退出），但对真正的图有正确的遍历语义。

## Try It / 动手试一试

```bash
cd agents/s03-flow-orchestrator
go run .
go test -v ./...
```

期望输出：

```
=== Flow run ===
msg : HEY, POCKETFLOW!
len : 16
last action: "default"
```

flow 流程：
1. `greet` 读 `shared["name"]` + `params["greeting"]`，写 `"Hey, PocketFlow!"` 到 `shared["msg"]`（per-call params 覆盖 flow 级别）
2. `upper` 原地大写
3. `count` 把长度写到 `shared["len"]`
4. `count` 无 successor → 循环退出，`last="default"`

## Upstream Source Reading / 上游源码阅读

```upstream:pocketflow/__init__.py#L39-L51
# Source: pocketflow/__init__.py#L39-L51
class Flow(BaseNode):
    def __init__(self,start=None): super().__init__(); self.start_node=start
    def start(self,start): self.start_node=start; return start
    def get_next_node(self,curr,action):
        nxt=curr.successors.get(action or "default")
        if not nxt and curr.successors: warnings.warn(f"Flow ends: '{action}' not found in {list(curr.successors)}")
        return nxt
    def _orch(self,shared,params=None):
        curr,p,last_action =copy.copy(self.start_node),(params or {**self.params}),None
        while curr: curr.set_params(p); last_action=curr._run(shared); curr=copy.copy(self.get_next_node(curr,last_action))
        return last_action
    def _run(self,shared): p=self.prep(shared); o=self._orch(shared); return self.post(shared,p,o)
    def post(self,shared,prep_res,exec_res): return exec_res
```

**对照阅读要点**：

- **L47 的 `copy.copy(self.start_node)`**：上游在调用每个节点前都浅拷贝一次，让原节点保持干净状态。动机：把 `Node` (s05) 加的 `cur_retry` 限制成 per-run 作用域。我们 Go 版把 retry 状态推到栈上变量，所以不需要拷贝。代价：如果用户在 `Exec` 里改 `node.Params`，上游的拷贝保留原值，我们不保留。我们接受 —— flow 中途改 params 是未定义行为。
- **`get_next_node` 对未知 action 警告**：L44 仅当 **有 successors 但选的 action 不在里面** 时警告。我们 s03 静默返回 nil；s04 处理真正的分支时把警告补回来。
- **`Flow._run` 调 `prep` → `_orch` → `post`**：让 Flow 能嵌套在另一个 Flow 里（post 把编排结果作为 action，外层 flow 据此分支）。我们 `Flow.Run` 形态对齐，但暂不演示 flow 组合。
- **L47 和 L56 的 `{**self.params, **bp}`**：Python dict 散开合并。我们 `mergeParams` 完全等价。
- **L51 的 `Flow.post` 默认透传**：返回 `exec_res`（即 `_orch` 的最后 action）。这就是 Flow 能作为内层 Node 被另一个 Flow 使用的原因 —— 它的 post 返回成了外层 flow 的路由 action。我们 `Flow.Run` 保留了。

**想读更多**：`pocketflow/__init__.py` L42-L45 的 action 查找是 s04 的基础。"missing successors → 警告" 是分支的种子 —— s04 在此之上构建完整的分支 + 循环故事。

---

**下一节预告**：s04 把 s03 的简单 "default" 路由进化成真正的分支 —— action 字符串、loop-back、"未知 action" 警告。
