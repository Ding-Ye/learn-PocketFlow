# Annotated upstream source for s04 (action-based routing).
#
# Source 1: pocketflow/__init__.py#L42-L48 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5
# Source 2: tests/test_flow_basic.py#L104-L157
# https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L42-L48

# ----- routing function (the entire mechanism) -----

def get_next_node(self,curr,action):                           # L42 Called by _orch after every Post.
    nxt = curr.successors.get(                                 # L43 Dict lookup. Python's `or` does:
        action or "default")                                   #     None/""/0/[] → fall through to "default".
                                                                #     We Go-port use only "" → "default".

    if not nxt and curr.successors:                            # L44 The crucial AND:
                                                                #     - Without successors at all: silent leaf exit
                                                                #     - With successors but no match: probable user error
        warnings.warn(                                         # L45 Helpful warning that names the key set
            f"Flow ends: '{action}' "
            f"not found in {list(curr.successors)}")

    return nxt                                                 # L46 None ends the flow; otherwise the orchestrator
                                                                #     follows the edge in _orch's while loop.


# ----- canonical test fixture showing branching + loop -----

# Source: tests/test_flow_basic.py#L104-L157

class CheckPositiveNode(Node):
    def post(self, shared_storage, prep_result, proc_result):  # post returns the action string.
        if shared_storage['current'] >= 0:
            return 'positive'                                  # match check.successors['positive']
        return 'negative'                                      # match check.successors['negative']


# Building the graph: branching with loop-back
def test_branching_loop():
    n1 = NumberNode(5)
    check = CheckPositiveNode()
    add_if_positive = AddNode(10)
    sub_if_negative = AddNode(-20)

    # Default chain n1 -> check
    n1 >> check

    # Branches off check
    check - "positive" >> add_if_positive
    check - "negative" >> sub_if_negative

    # CRITICAL: back-edges form the loop
    add_if_positive >> check                                   # back to check after add
    sub_if_negative >> check                                   # back to check after sub

    flow = Flow(start=n1)
    flow.run({'current': None})                                # eventually returns when an action has no matching edge


# Notes for the Go reader:
#
# Our s04 keeps the routing logic in a free function, getNextNode(curr, action).
# The exact decision tree:
#
#   action == ""           → action = "default"
#   succ[action] exists    → return it
#   succ[action] missing
#       AND len(succ) > 0  → log warning + return nil
#       AND len(succ) == 0 → silent return nil (leaf)
#
# That tree is 5 lines of Go, perfectly mirroring upstream's 5 lines of Python.
# The only semantic shift: Python's truthiness for "fall through to default"
# is wider (None, "", 0, []) than ours (only ""). For PocketFlow that doesn't
# matter — post() only ever returns str or None in practice.
