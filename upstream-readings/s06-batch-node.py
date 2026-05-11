# Annotated upstream source for s06 (BatchNode — map pattern).
#
# Source: pocketflow/__init__.py#L36-L37 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5

class BatchNode(Node):                                         # L36 Inherits from Node (which has retry/fallback from s05).
    def _exec(self,items):                                     # L37 Override _exec only. exec / prep / post / etc.
        return [                                               #     come from Node and BaseNode unchanged.
            super(BatchNode,self)._exec(i)                     #     Per item: call Node._exec, which runs the retry loop.
            for i in (items or [])                             #     `items or []` makes None == [] for safety.
        ]


# Notes for the Go reader:
#
# Upstream's two-line class is a Python comprehension feat. Translating to Go:
#
#   def _exec(self,items): return [super(BatchNode,self)._exec(i) for i in (items or [])]
#
# becomes (effectively):
#
#   func (b *BatchNode) ExecBatch(items []any) []any {
#       results := make([]any, 0, len(items))
#       for _, i := range items {
#           // retryExec(maxRetries, wait, i, b.TryExecItem, b.ExecFallbackItem)
#           // is what Node._exec(i) does in upstream
#           results = append(results, retryExec(b.MaxRetries, b.Wait, i,
#               b.TryExecItem, b.ExecFallbackItem))
#       }
#       return results
#   }
#
# Same semantics, more lines, more obvious that "each item gets its own retry".
#
# WHY use `super(BatchNode,self)._exec(i)` and not just `self._exec(i)`?
# Because `self._exec` IS `BatchNode._exec` — we'd infinitely recurse. The
# `super(BatchNode,self)._exec(i)` says "call _exec from the class BEFORE
# BatchNode in MRO" — i.e. Node._exec, which has the retry loop.
#
# In Go we avoid this MRO trap because retryExec is a free function, not a
# method bound to the class hierarchy.
