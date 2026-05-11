# Annotated upstream source for s01.
#
# Source: pocketflow/__init__.py#L3-L20 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5
# https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L3-L20
#
# This file mirrors lines 3-20 of upstream's __init__.py with inline notes that
# tie each line to a decision we made (or deferred) in agents/s01-minimum-node/.
# It is NOT executed — it's a reading guide. Strip annotations to recover the
# original.

# import asyncio, warnings, copy, time  # L1 — stdlib only; zero deps.

class BaseNode:                                                  # L3  Foundation. Our Go: type BaseNode struct.
    def __init__(self): self.params,self.successors={},{}        # L4  Two maps. We init via NewBaseNode().
    def set_params(self,params): self.params=params              # L5  Setter for params. Used by Flow (s03).
                                                                  #     We delay using it until s03 but keep the field now.

    def next(self,node,action="default"):                        # L6  Chaining. s02 ports this as .Next() / .NextOn().
        if action in self.successors:                            # L7  Warn on overwrite — Go uses log.Printf in s02.
            warnings.warn(f"Overwriting successor for action '{action}'")
        self.successors[action]=node; return node                # L8  Return appended node enables fluent chaining.

    def prep(self,shared): pass                                  # L9  Lifecycle 1: read inputs from shared.
    def exec(self,prep_res): pass                                # L10 Lifecycle 2: do the work (retries wrap this in s05).
    def post(self,shared,prep_res,exec_res): pass                # L11 Lifecycle 3: write results + return action string.

    def _exec(self,prep_res): return self.exec(prep_res)         # L12 Indirect wrapper. Trivial in BaseNode.
                                                                  #     s05's Node overrides _exec to add a retry loop
                                                                  #     while leaving exec() as the user override point.

    def _run(self,shared):                                       # L13 The canonical 3-step run. Our RunOnce() does this.
        p=self.prep(shared)
        e=self._exec(p)
        return self.post(shared,p,e)

    def run(self,shared):                                        # L14 Public entry. Warns if successors exist (you wanted Flow).
        if self.successors:                                      # L15 Same warn-then-continue shape as our RunOnce.
            warnings.warn("Node won't run successors. Use Flow.")
        return self._run(shared)

    def __rshift__(self,other): return self.next(other)          # L17 >> operator. Go can't overload — we ship Next() in s02.
    def __sub__(self,action):                                    # L18 - operator. Builds _ConditionalTransition.
        if isinstance(action,str):
            return _ConditionalTransition(self,action)           # L19 The conditional transition helper, see lines 22-24.
        raise TypeError("Action must be a string")               # L20


# Notes for the Go reader:
#
# 1. Python initializes both `params` and `successors` in one terse line (L4).
#    Go can't do that cleanly with zero-value maps (`nil` maps panic on write),
#    so we ship NewBaseNode() that pre-populates both maps.
#
# 2. The `prep / _exec / post` triple appears twice — once as overridable user
#    hooks (L9-L11) and once as the orchestrator (L13). We don't duplicate it
#    in Go because in s01 the indirection adds nothing; we revisit in s05 when
#    retry needs a wrapper layer.
#
# 3. `run()` on L14-L16 calls _run() but warns first. That's exactly the
#    semantics of our RunOnce(): warn if successors exist (you wanted Flow),
#    then execute anyway. The "warn-but-don't-fail" choice is conscious — it
#    keeps quick experimentation forgiving.
#
# 4. Lines 17-20 are the operator-overloading sugar (>> and -) that gives
#    PocketFlow its declarative chaining DSL. Go has no operator overloading,
#    so we replace them with .Next() and .NextOn() methods in s02. The
#    semantics are 1:1 — only the syntax differs.
