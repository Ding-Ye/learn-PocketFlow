# Annotated upstream source for s05 (Node = BaseNode + retry/fallback).
#
# Source 1: pocketflow/__init__.py#L26-L34 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5
# Source 2: tests/test_fall_back.py#L9-L33

class Node(BaseNode):                                          # L26 Promote BaseNode to retry-aware.
    def __init__(self,max_retries=1,wait=0):                   # L27 Defaults: try once, no wait.
        super().__init__()
        self.max_retries,self.wait=max_retries,wait

    def exec_fallback(self,prep_res,exc):                      # L28 Default fallback: re-raise.
        raise exc                                              #     Override to return a sentinel value
                                                                #     when failure is acceptable.

    def _exec(self,prep_res):                                  # L29 Overrides BaseNode._exec (which was identity).
        for self.cur_retry in range(self.max_retries):         # L30 cur_retry is INSTANCE state — leaks across runs
                                                                #     unless Flow shallow-copies the node.
            try:
                return self.exec(prep_res)                     # L31 User's actual work, called as a normal method.
            except Exception as e:                             # L32
                if self.cur_retry==self.max_retries-1:         # L33 Final attempt? hand off to fallback.
                    return self.exec_fallback(prep_res,e)
                if self.wait>0: time.sleep(self.wait)          # L34 Otherwise sleep and retry.


# ----- Canonical retry test fixture -----
#
# Source: tests/test_fall_back.py#L9-L33

class FallbackNode(Node):
    def __init__(self, should_fail=True, max_retries=1):
        super().__init__(max_retries=max_retries)
        self.should_fail = should_fail
        self.attempt_count = 0

    def prep(self, shared_storage):
        shared_storage.setdefault("results", [])
        return None

    def exec(self, prep_result):
        self.attempt_count += 1
        if self.should_fail:
            raise ValueError("Intentional failure")
        return "success"

    def exec_fallback(self, prep_result, exc):                 # User overrides to return a sentinel.
        return "fallback"

    def post(self, shared_storage, prep_result, exec_result):
        shared_storage["results"].append({
            "attempts": self.attempt_count,
            "result": exec_result
        })


# How we map this to Go (in agents/s05-retry-fallback/):
#
# Upstream                              | learn-PocketFlow Go (s05)
# --------------------------------------|------------------------------------------------
# class Node(BaseNode):                 | type RetryNode struct { *BaseNode; MaxRetries int; Wait time.Duration }
# self.max_retries / self.wait          | r.MaxRetries / r.Wait
# self.cur_retry (INSTANCE STATE)       | local var `attempt` inside retryExec   ← key divergence
# self.exec_fallback(prep_res, exc)     | (RetryableNode).ExecFallback(prepRes, err) any
# self.exec(prep_res)                   | (RetryableNode).TryExec(prepRes) (any, error)
# Exception catch in for-loop           | err != nil branch
# time.sleep(self.wait)                 | time.Sleep(r.Wait)
#
# The "key divergence" is the whole reason our Flow doesn't need to copy.copy
# nodes between runs. In Go, hoist mutable per-run state out of the struct
# and into the function scope; the struct stays reusable.
#
# Pattern to remember: in Python, instance state + shallow-copy is the
# default; in Go, function-local state + value semantics is the default.
# Both achieve the same goal — fresh state per run — but with opposite
# defaults.
