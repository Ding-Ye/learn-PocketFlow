# s03 · The Flow orchestrator

Build the `Flow` type that walks the graph s02 lets us construct.
`Orchestrate(shared, params)` follows successors until none remain, calling
each node's prep/exec/post and routing on the action returned by Post.

```bash
go run .
go test -v ./...
```

Also introduces the `params` vs `shared` distinction — both are dicts the
flow passes to nodes, but with different scoping rules.

Read [`docs/en/s03-flow-orchestrator.md`](../../docs/en/s03-flow-orchestrator.md).
Upstream: [`pocketflow/__init__.py#L39-L51`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L39-L51).
