---
title: "s02 · 节点串接与转换"
chapter: 2
slug: s02-node-chaining
est_read_min: 7
---

# s02 · 节点串接与转换

> 教什么：怎么把节点连成一个有向图。上游用 `>>` 和 `-"action">>` 算符重载；Go 没有算符重载 —— 我们改用 `Next(node)` 和 `NextOn(action, node)`。

---

## Problem / 问题

s01 给了我们单个 Node 和一个 `RunOnce`。但 agent 是图，不是孤立的回调："如果 LLM 返回 `tool_use`，路由到工具节点；如果 `end_turn`，结束"。没有连接节点的方式，每个 Flow 都得在 `Exec` 内塞一个巨大的 if-else。

PocketFlow 用两个算符重载解决：`parent >> child` 是默认转换，`parent - "action" >> child` 是命名转换。两个都最终调 `parent.next(child, action)`。Go 没有算符重载。我们要保留 **语义**，找一个干净的 Go **语法**。

## Solution / 解决方案

给 `BaseNode` 加两个方法：

```go
func (b *BaseNode) Next(n Node) Node                   // 默认 action
func (b *BaseNode) NextOn(action string, n Node) Node  // 命名 action
```

两者都把 `n` 加到 `Successors` map 里（用对应的 action 做 key），并返回 `n`，这样调用方可以流式串接。我们再给 `Node` 接口加一个方法 —— `GetSuccessors() map[string]Node` —— 这样 s03 的 Flow 可以问任何节点"你的出边是什么"，而不依赖具体的 `BaseNode` 类型。

三个关键决策：

1. **`NextOn` 覆盖时只警告不报错** —— 上游用 `warnings.warn`，我们用 `log.Printf`。意图都是"你大概率写错了，但程序不该 crash"。
2. **`Next` 是 `NextOn("default", n)` 的薄壳** —— 让两个方法保持一致。"default" 这个魔法字符串直接照搬上游。
3. **不用反射搞算符重载** —— Go 是小语法语言。`.Next()` 比 `>>` 啰嗦但 `grep` 友好。

## How It Works / 工作原理

```ascii-anim frames=2
┌──────────────────────────────────────────────────────────────┐
│             check.NextOn("empty", load)                       │
│             check.Next(load)              // == NextOn("default")
│             load.Next(caps)                                   │
│                                                               │
│   ┌──────────┐   "default" / "empty"    ┌──────────┐          │
│   │  check   │ ───────────────────────▶ │   load   │          │
│   │ (s02)    │                          │ (s02)    │          │
│   └──────────┘                          └────┬─────┘          │
│                                              │  "default"     │
│                                              ▼                │
│                                         ┌──────────┐          │
│                                         │   caps   │          │
│                                         │ (s02)    │          │
│                                         └──────────┘          │
└──────────────────────────────────────────────────────────────┘
```

核心 20 行（节选自 [`agents/s02-node-chaining/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s02-node-chaining/main.go)）：

```go
func (b *BaseNode) Next(n Node) Node { return b.NextOn("default", n) }

func (b *BaseNode) NextOn(action string, n Node) Node {
    if b.Successors == nil { b.Successors = map[string]Node{} }
    if _, ok := b.Successors[action]; ok {
        log.Printf("[warn] overwriting successor for action %q", action)
    }
    b.Successors[action] = n
    return n
}

type Node interface {
    Prep(shared SharedStore) any
    Exec(prepRes any) any
    Post(shared SharedStore, prepRes any, execRes any) string
    GetSuccessors() map[string]Node  // s02 新增
}
```

**3 个非显然之处**：

1. **`GetSuccessors()` 放在 interface 而不是只挂在 `BaseNode`** —— s03 的 Flow 问任何节点"你的出边"时，不用关心具体类型。如果只挂在 `*BaseNode` 上，Flow 就得 type-assert，泄露实现细节。
2. **`Next` 和 `NextOn` 返回 **新追加的** 节点，不是 self** —— 对齐上游 `def next(self,node,action="default"): ...; return node`。这才能 `a.Next(b).Next(c)` 链式调用。返回 self 会建出星形图，不是线性图。
3. **每次调用都 nil-map 守卫** —— Go 的零值 map 是 nil，写入会 panic。构造器 `NewBaseNode()` 已经初始化好了，但守卫让方法在"有人嵌入 BaseNode 但没用构造器"的场景下也安全。

## What Changed / 与 s01 的变化

```diff
 type Node interface {
     Prep(shared SharedStore) any
     Exec(prepRes any) any
     Post(shared SharedStore, prepRes any, execRes any) string
+    GetSuccessors() map[string]Node
 }

+func (b *BaseNode) Next(n Node) Node { return b.NextOn("default", n) }
+func (b *BaseNode) NextOn(action string, n Node) Node {
+    if b.Successors == nil { b.Successors = map[string]Node{} }
+    if _, ok := b.Successors[action]; ok {
+        log.Printf("[warn] overwriting successor for action %q", action)
+    }
+    b.Successors[action] = n
+    return n
+}
+func (b *BaseNode) GetSuccessors() map[string]Node { return b.Successors }
```

`RunOnce` 没改语义，只是从"接口断言"切换成直接调 `n.GetSuccessors()`，因为它现在是接口契约的一部分。

## Try It / 动手试一试

```bash
cd agents/s02-node-chaining
go run .
go test -v ./...
```

期望输出：

```
Graph built (no walking — that's s03):
  check[default] -> *main.LoadNode
  check[empty  ] -> *main.LoadNode
  load [default] -> *main.CapsNode
  caps successors: 0 (terminal)

Trying RunOnce on a chained node (expect a warning):
2026/05/12 00:21:33 [warn] Node has successors but RunOnce ignores them; use Flow (s03+).
```

## Upstream Source Reading / 上游源码阅读

上游的串接由三块组成：`BaseNode.next()` 方法、`__rshift__` / `__sub__` 算符重载、一个微型辅助类 `_ConditionalTransition`。

```upstream:pocketflow/__init__.py#L6-L24
# Source: pocketflow/__init__.py#L6-L24
def next(self,node,action="default"):
    if action in self.successors:
        warnings.warn(f"Overwriting successor for action '{action}'")
    self.successors[action]=node; return node

# (...prep/exec/post 见 s01...)

def __rshift__(self,other): return self.next(other)
def __sub__(self,action):
    if isinstance(action,str): return _ConditionalTransition(self,action)
    raise TypeError("Action must be a string")

class _ConditionalTransition:
    def __init__(self,src,action): self.src,self.action=src,action
    def __rshift__(self,tgt): return self.src.next(tgt,self.action)
```

**对照阅读要点**：

- **`>>` 脱糖成 `.next()`**：`__rshift__` 仅是 `next` 的默认 action 版本。所以 "default" 是这个 fallback key —— 它就是算符硬编码的字面量。
- **`-"action" >>` 是两步小技巧**：`parent - "action"` 返回临时对象 `_ConditionalTransition`；那个对象自己的 `__rshift__` 才真正调 `parent.next(target, "action")`。一个算符做不到，因为 Python 的 `-` 永远返回中间值，不是副作用。
- **警告语义**：上游用 `warnings.warn`，可用 `warnings.filterwarnings("error")` 升级成异常。Go 的 `log.Printf` 是单向的。想要严格模式自己在 `NextOn` 外面包一层先检查 map。
- **`next()` 返回追加的节点**：这是 `a.next(b).next(c)` 成立的原因。上游和我们的 Go 版都保留了这个返回。
- **我们 Go 版故意省略的**：我们没有 `_ConditionalTransition` 那样的辅助 struct。Python 里它存在 **只是** 为了让 `-"action">>` 两个 token 的算符序列工作；没有算符重载我们用 `NextOn(action, node)` 一个方法就够。少代码少间接层。

**想读更多**：从 `pocketflow/__init__.py#L6-L24` 继续到 L46-49 的 `Flow._orch` —— 那是真正 **遍历** 我们刚搭好的图的循环。下一节 s03 攻它。

---

**下一节预告**：s03 构建真正会遍历 successors 的 `Flow` 编排器。
