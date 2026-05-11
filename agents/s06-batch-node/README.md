# s06 · BatchNode (map pattern)

Promote `RetryNode` to `BatchNode`: `TryExec` iterates over a list of items,
each getting full retry-with-fallback semantics independently.

```bash
go run .
go test -v ./...
```

Demo: translate "Hello, world!" into ES / FR / DE / ZH; French fails once
then recovers via per-item retry.

Read [`docs/en/s06-batch-node.md`](../../docs/en/s06-batch-node.md).
Upstream: [`pocketflow/__init__.py#L36-L37`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L36-L37).
