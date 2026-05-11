# s01 · Minimum node loop

The atomic PocketFlow unit in Go: a `Node` interface with `Prep`/`Exec`/`Post`,
a `BaseNode` you can embed for defaults, and a `RunOnce` helper that executes
the lifecycle.

```bash
go run .
go test -v ./...
```

Expected output:

```
============================================================
Question : What is PocketFlow?
Answer   : Mock answer to: "What is PocketFlow?" (length=19)
Action   : "default"
============================================================
```

Read [`docs/en/s01-minimum-node.md`](../../docs/en/s01-minimum-node.md) for the
walkthrough. Upstream reference: [`pocketflow/__init__.py#L3-L20`](https://github.com/The-Pocket/PocketFlow/blob/43ef382bb0c9dae8167528618bb40f5a3f9a28a5/pocketflow/__init__.py#L3-L20).
