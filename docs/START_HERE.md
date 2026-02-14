# Start Here

This document is the shortest entry path for both users and contributors.

## If You Want to Use kra

1. Read `README.md` first (installation + quick start).
2. Initialize a root with `kra init --root <path>`.
3. Create your first workspace with `kra ws create`.
4. Use `kra ws dashboard` to inspect operational state.

## If You Want to Contribute

1. Read `AGENTS.md` in the repository root.
2. Use `docs/backlog/README.md` as the single backlog entrypoint.
3. Follow linked specs under `docs/spec/**` before coding.
4. Run the minimum quality gate:

```sh
test -z "$(gofmt -l .)"
./scripts/lint-ui-color.sh
go vet ./...
go test ./...
```
