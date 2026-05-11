# Annotated upstream source for s03 (Flow orchestrator).
#
# Source: pocketflow/__init__.py#L39-L51 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5
# https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L39-L51

class Flow(BaseNode):                                          # L39 Flow is itself a Node — flows nest in flows.
    def __init__(self,start=None):                             # L40 Default start=None lets you build the graph
        super().__init__()                                     #     before attaching it.
        self.start_node=start

    def start(self,start):                                     # L41 Imperative setter (rare; usually start is in __init__).
        self.start_node=start; return start

    # ----- next-node lookup (L42-L45) -----

    def get_next_node(self,curr,action):                       # L42
        nxt=curr.successors.get(action or "default")           # L43 None/'' both fall back to "default" key.
        if not nxt and curr.successors:                        # L44 The "branch ends unexpectedly" warning fires only when
            warnings.warn(                                     #     successors EXIST but the chosen action isn't among them
                f"Flow ends: '{action}' not found in "         #     — otherwise we genuinely hit a leaf node.
                f"{list(curr.successors)}")
        return nxt                                             # L45

    # ----- the orchestration loop (L46-L49) -----

    def _orch(self,shared,params=None):                        # L46 Walk the graph.
        curr,p,last_action = (                                 # L47 Three things in one line:
            copy.copy(self.start_node),                        #     1. shallow-copy of start node (per-run isolation)
            (params or {**self.params}),                       #     2. params override flow.params via dict-spread
            None)                                              #     3. last_action seeded as None

        while curr:                                            # L48 Loop until no next node.
            curr.set_params(p)                                 #     Inject per-iteration params into the current node.
            last_action=curr._run(shared)                      #     Execute the node (prep → _exec → post).
            curr=copy.copy(                                    #     Move forward; copy isolates the *next* node too.
                self.get_next_node(curr,last_action))

        return last_action                                     # L49 Whoever called _orch gets the final action.

    # ----- wiring Flow as a Node (L50-L51) -----

    def _run(self,shared):                                     # L50 _run is what BaseNode._run looks like for a regular node:
        p=self.prep(shared)                                    #     prep / exec / post. Here exec = _orch.
        o=self._orch(shared)
        return self.post(shared,p,o)

    def post(self,shared,prep_res,exec_res):                   # L51 Default Flow.post returns the inner _orch result as
        return exec_res                                        #     this Flow's own "action", enabling Flow → Flow chaining.


# How we map this to Go:
#
# Upstream                         | learn-PocketFlow Go (s03)
# ---------------------------------|------------------------------------------
# `class Flow(BaseNode)`           | type Flow struct { *BaseNode; Start Node }
# `self.start_node`                | f.Start
# `_orch(shared, params)`          | (*Flow).Orchestrate(shared, params)
# `copy.copy(curr)`                | (NOT copied) — retry state lives in a
#                                  | local var in s05, so no copy needed
# `{**self.params, **bp}`          | mergeParams(base, overrides)
# `curr.successors.get(action or "default")` | getNextNode(curr, action)
# `warnings.warn(...)`             | log.Printf("[warn] ...") in s04 (skipped in s03)
# `_run` calls prep/_orch/post     | (*Flow).Run(shared)
#
# The "unknown action" warning on L44 is intentionally absent from our s03;
# s04 adds it back when we introduce branching properly. In s03 we only ever
# return "default", so the warning would never fire anyway.
