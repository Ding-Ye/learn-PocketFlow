# s04 · Action-based routing

Promote `Post`'s return value to a routing signal. `getNextNode` now honors
arbitrary action strings and warns when an action has no matching successor.
Enables conditional branching AND loop-back patterns.

```bash
go run .
go test -v ./...
```

Read [`docs/en/s04-action-routing.md`](../../docs/en/s04-action-routing.md).
Upstream: [`pocketflow/__init__.py#L42-L48`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L42-L48) + [`tests/test_flow_basic.py`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/tests/test_flow_basic.py).
