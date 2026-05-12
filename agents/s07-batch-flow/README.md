# s07 · BatchFlow (param iteration)

Orthogonal to s06's `BatchNode`. Where `BatchNode` iterates **data** items
inside one node, `BatchFlow` iterates **parameter sets** across the entire
inner flow. Each parameter dict from `PrepBatch` triggers a full flow run.

```bash
go run .
go test -v ./...
```

Demo: process the same image-processing flow with the Cartesian product of
N images × M filters.

Read [`docs/en/s07-batch-flow.md`](../../docs/en/s07-batch-flow.md).
Upstream: [`pocketflow/__init__.py#L53-L57`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L53-L57).
