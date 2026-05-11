---
title: "s04 · 基于动作的路由"
chapter: 4
slug: s04-action-routing
est_read_min: 8
---

# s04 · 基于动作的路由

> 教什么：`Post` 返回的字符串是路由信号 —— 决定下一个 successor。让分支（yes/no）和循环（back-edge）成为可能。

---

## Problem / 问题

s03 的 `Flow.Orchestrate` 遍历图，但只走 `"default"` successor。线性管道够用，但真实 agent 要 **选择**：

- 守卫节点返回 `"valid"` → 继续；返回 `"invalid"` → 问用户。
- 推理节点返回 `"search"` → 调工具；返回 `"answer"` → 终响应。
- 评判节点返回 `"good"` → 出货；返回 `"retry"` → 回到 writer 重写。

我们还需要区分"图按用户期望停在了某个叶子"和"图因为缺了一条边而意外终止"。上游用两个启发式：`action or "default"`（空字符串走 default）+ 仅当 **有 successors 但 action 不在里面** 时警告。

## Solution / 解决方案

把 `getNextNode(curr, action)` 改成：

1. 空 action 归一化为 `"default"`。
2. 查 `succ[action]`。
3. 如果查不到 **且** `len(succ) > 0`（节点有分支但都没匹配）→ 警告。
4. 返回查到的节点（或 `nil` 结束 flow）。

变化就这些。图拓扑语义（循环、汇合、叶子）s03 就有了 —— 只是路由函数没尊重 action 字符串。

三个关键决策：

1. **警告而非报错** —— 上游 `warnings.warn`，我们 `log.Printf`。意图："你的图大概率写错了，但程序不该 crash"。严格模式可以在 Flow 层升级成 error。
2. **没有最大迭代次数限制** —— 循环靠某个节点返回"无对应 successor 的 action"（通常是显式的 "finished"）来结束。PocketFlow 信任图作者，不靠全局计数。`FinishedNode` demo 演示这点。
3. **空字符串也走 "default"** —— Post 可以"懒得返回有意义的 action"直接 `""`；Flow 当作默认路径。

## How It Works / 工作原理

```ascii-anim frames=2
┌────────────────────────────────────────────────────────────────────────┐
│                          Flow.Orchestrate (s04)                         │
│                                                                         │
│   ┌─────────┐  Post 返回 "positive"        ┌─────────┐                  │
│   │ check   │ ──────────────────────────▶ │  add3   │                  │
│   │         │                              └────┬────┘                  │
│   │         │  Post 返回 "negative"        ┌────▼─────┐                 │
│   │         │ ──────────────────────────▶ │  sub3    │                 │
│   └─────────┘                              └────┬─────┘                 │
│        ▲                                       │                       │
│        │       "loop"                          │                       │
│        └──────────────────────── ┌─────────────▼─────┐                  │
│                                  │ finished (计数)   │                  │
│                                  │ 3 次返回 "loop"   │                  │
│                                  │ 之后 "finished"   │ → 无 successor   │
│                                  └───────────────────┘    → flow 退出   │
└────────────────────────────────────────────────────────────────────────┘
```

核心 18 行（节选自 [`agents/s04-action-routing/main.go`](https://github.com/Ding-Ye/learn-PocketFlow/blob/main/agents/s04-action-routing/main.go)）：

```go
func getNextNode(curr Node, action string) Node {
    if action == "" {
        action = "default"
    }
    succ := curr.GetSuccessors()
    nxt := succ[action]
    if nxt == nil && len(succ) > 0 {
        keys := make([]string, 0, len(succ))
        for k := range succ {
            keys = append(keys, k)
        }
        log.Printf("[warn] Flow ends: action %q not found in %v", action, keys)
    }
    return nxt
}
```

**3 个非显然之处**：

1. **警告是"你大概有条边漏了"，不是"flow 结束了"** —— 叶子节点（无 successors）静默退出。警告仅当图明显设了分支却没匹配中才触发。区别就是 `len(succ) > 0` vs `len(succ) == 0`。
2. **Loop-back 就是一条普通的图边** —— `finished.NextOn("loop", check)` 把 `check` 注册成 `finished` 的 "loop" successor。当 `finished.Post` 返回 `"loop"`，Flow 走回 `check`。没有特殊的"循环"构造，纯粹是 back-edge。
3. **靠"未知 action"终止是合法设计** —— FinishedNode demo 返回 `"finished"` 来退出，虽然也可以返回 nil 或接一个终止节点。PocketFlow 的惯用法：叶子返回最有描述性的字符串；Flow 的行为（警告 + 退出）相同。

## What Changed / 与 s03 的变化

```diff
 func getNextNode(curr Node, action string) Node {
     if action == "" { action = "default" }
-    return curr.GetSuccessors()[action]
+    succ := curr.GetSuccessors()
+    nxt := succ[action]
+    if nxt == nil && len(succ) > 0 {
+        keys := make([]string, 0, len(succ))
+        for k := range succ { keys = append(keys, k) }
+        log.Printf("[warn] Flow ends: action %q not found in %v", action, keys)
+    }
+    return nxt
 }
```

就这一处 diff。Flow、Node、BaseNode 都没动。这是整门课最外科手术式的一章 —— 路由本来就该这么工作，s03 还没把 action 字符串"提拔"上来而已。

## Try It / 动手试一试

```bash
cd agents/s04-action-routing
go run .
go test -v ./...
```

期望输出：

```
==================================================
final current : 9
last action   : "finished"  (should be "finished")
==================================================
```

demo 从 `current=1` 开始，连续走三次 "positive" 分支（1 → 4 → 7 → 10？不对：`current=1` → check 返回 "positive" → add3 → current=4 → finished count=1 返回 "loop" → ...）。

## Upstream Source Reading / 上游源码阅读

```upstream:pocketflow/__init__.py#L42-L48
# Source: pocketflow/__init__.py#L42-L48
def get_next_node(self,curr,action):
    nxt=curr.successors.get(action or "default")
    if not nxt and curr.successors:
        warnings.warn(f"Flow ends: '{action}' not found in {list(curr.successors)}")
    return nxt
```

分支 + 循环的标准测试：

```upstream:tests/test_flow_basic.py#L104-L157
# Source: tests/test_flow_basic.py#L104-L157
class CheckPositiveNode(Node):
    def post(self, shared_storage, prep_result, proc_result):
        if shared_storage['current'] >= 0:
            return 'positive'
        return 'negative'

# ... 测试里 ...
n1 = NumberNode(5)
check = CheckPositiveNode()
add_if_positive = AddNode(10)
sub_if_negative = AddNode(-20)

n1 >> check
check - "positive" >> add_if_positive
check - "negative" >> sub_if_negative
add_if_positive >> check    # 回环
sub_if_negative >> check    # 回环

flow = Flow(start=n1)
flow.run(shared_storage)
```

**对照阅读要点**：

- **`not nxt and curr.successors`**：警告仅当 **有 successors 但都没匹配** 时触发。叶子节点（`not curr.successors`）静默退出。我们用 `len(succ) > 0` 镜像这条规则。
- **`action or "default"`**：Python 真值性 —— `None`/`""`/`0`/`[]` 都 fallback 到 `"default"`。Go 更严格，我们只在 `""` 时 fallback。
- **Loop-back 就是 `add_if_positive >> check`**：没有 `loop` 关键字。图碰巧有 back-edge，编排器不管。
- **测试用 `Node`（带 retry）而非 `BaseNode`**：真实 flow 要韧性，上游测试 fixture 用 `Node`（s05 才覆盖）。我们 s04 demo 用 `BaseNode`，路由逻辑两者无差异。
- **我们 Go 版故意省略的**：没有最大迭代保护。上游也没有。写出死循环就死循环。教学上加 `MaxSteps` 会掩盖"循环就是图边"这一事实。

**想读更多**：`pocketflow/__init__.py` L26-34 是 `Node`（带 retry/fallback），s05 来移植。Retry 语义和路由正交 —— 它包 `exec`，不动 `post`，所以基于 action 的路由不受影响。

---

**下一节预告**：s05 在生命周期上叠加 retry + fallback。`RetryNode` 扩展 `BaseNode`，加上 `MaxRetries` 和 `ExecFallback`，取代生产代码里裸用 BaseNode 的写法。
