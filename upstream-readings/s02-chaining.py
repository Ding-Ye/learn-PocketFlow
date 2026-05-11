# Annotated upstream source for s02 (node chaining).
#
# Source: pocketflow/__init__.py#L6-L24 @ 43ef382bb0c9dae8167528618bb40f5a3f9a28a5
# https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L6-L24
#
# The three pieces that make `parent >> child` and `parent - "action" >> child`
# work, all in one class plus a tiny helper. Strip the annotations to recover
# upstream verbatim.

def next(self,node,action="default"):                          # L6  Method that does the actual work.
    if action in self.successors:                              # L7  Detect overwrite.
        warnings.warn(f"Overwriting successor for action '{action}'")
    self.successors[action]=node                               # L8  Store.
    return node                                                #     Return appended → enables fluent chaining.

# ----- operator sugar (L17-L20) -----

def __rshift__(self,other): return self.next(other)            # L17 `parent >> child` desugars to `parent.next(child)`.

def __sub__(self,action):                                      # L18 `parent - "action"` returns a transient builder.
    if isinstance(action,str):
        return _ConditionalTransition(self,action)             #     The builder remembers (src, action) for the next step.
    raise TypeError("Action must be a string")                 # L20

# ----- the transient builder class (L22-L24) -----

class _ConditionalTransition:                                  # L22 Private helper. Lives only between `-` and `>>`.
    def __init__(self,src,action):
        self.src,self.action=src,action                        # L23 Remember which node + which action.

    def __rshift__(self,tgt):                                  # L24 When you do `... >> tgt`, finally call src.next(tgt, action).
        return self.src.next(tgt,self.action)


# How we map this to Go (in agents/s02-node-chaining/main.go):
#
#   parent >> child            → parent.Next(child)
#   parent - "yes" >> child    → parent.NextOn("yes", child)
#
# We drop _ConditionalTransition entirely. In Python it exists only as a
# bookkeeping object so `parent - "yes" >> child` can be written without
# parentheses. In Go we use a single method call with both arguments, so
# the helper class has no reason to exist.
#
# Semantics preserved 1:1: action key lookup, overwrite warning, fluent
# return-the-appended-node behaviour.
