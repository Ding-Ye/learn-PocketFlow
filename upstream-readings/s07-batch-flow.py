# Annotated upstream source for s07 (BatchFlow — iterate parameters).
#
# Source: pocketflow/__init__.py#L53-L57 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5

class BatchFlow(Flow):                                         # L53 BatchFlow IS a Flow (Flow.start_node etc. work).
    def _run(self,shared):                                     # L54 Override _run only.
        pr = self.prep(shared) or []                           # L55 prep returns the LIST of per-iteration param dicts.
                                                                #     `or []` defends against None.
        for bp in pr:                                          # L56 One iteration per dict.
            self._orch(                                        #     Call _orch (the graph walker), NOT _run again.
                shared,
                {**self.params, **bp})                         #     Merge flow.params with per-iteration overrides.

        return self.post(shared, pr, None)                     # L57 Post once after all iterations, with the param-dict list
                                                                #     as prep_res; no exec_res because there's no single one.


# How we map this to Go (in agents/s07-batch-flow/main.go):
#
# Upstream                          | learn-PocketFlow Go (s07)
# ----------------------------------|----------------------------------------------
# class BatchFlow(Flow):            | type BatchFlow struct { *Flow }
# self.prep(shared) returns []dict  | (BatchFlowPrep).PrepBatch(shared) []map[string]any
# for bp in pr: ...                 | for _, bp := range prepBatches { ... }
# self._orch(shared, merged_params) | bf.Orchestrate(shared, bp)
# {**self.params, **bp}             | (Orchestrate internally calls mergeParams)
# self.post(shared, pr, None)       | sharedNode.Post(shared, prepBatches, nil)
#
# Key gotcha: in Python, BatchFlow.prep COULD return something other than a
# list, since Python is dynamically typed. We force the issue with a typed
# interface (BatchFlowPrep). If a node doesn't implement it, prepBatches
# stays nil → zero iterations → still calls Post once with nil. Behavior
# matches upstream when prep returns None.
#
# Also: upstream's Flow.post (line 51) defaults to `return exec_res` —
# which for BatchFlow is None. If you want non-None post in your BatchFlow,
# override it. Same in our Go port — concrete BatchFlow types override Post
# to compute aggregates.
