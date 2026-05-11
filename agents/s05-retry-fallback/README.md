# s05 · Retry + fallback

Add `RetryNode` — embeds `BaseNode`, wraps `TryExec` in a retry loop with
optional `Wait` between attempts, calls `ExecFallback` on terminal failure.

```bash
go run .
go test -v ./...
```

Critical design choice: `cur_retry` lives on the stack inside `retryExec`,
NOT on the node — eliminates the need for upstream's `copy.copy(node)`
per-run isolation.

Read [`docs/en/s05-retry-fallback.md`](../../docs/en/s05-retry-fallback.md).
Upstream: [`pocketflow/__init__.py#L26-L34`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L26-L34).
