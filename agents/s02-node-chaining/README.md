# s02 · Node chaining + transitions

Extend `BaseNode` with `Next(node)` and `NextOn(action, node)` so callers can
build a directed graph of nodes. We don't run the graph yet — that's s03.

```bash
go run .
go test -v ./...
```

Replaces upstream's `>>` and `-"action">>` operator overloads with plain
methods (Go can't overload operators).

Read [`docs/en/s02-node-chaining.md`](../../docs/en/s02-node-chaining.md) for
the walkthrough. Upstream: [`pocketflow/__init__.py#L6-L24`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L6-L24).
