# Contributing to kra

Thanks for contributing.

## Development Setup

Requirements:

- Go 1.24+
- Git

Common commands:

```sh
task build
task test
task check
task ci:full
```

## Workflow

1. Read `AGENTS.md` and `docs/backlog/README.md`.
2. Confirm or update the relevant spec in `docs/spec/**` first.
3. Implement in small, ticket-aligned slices.
4. Add tests (including non-happy paths where relevant).
5. Run quality checks before opening a PR.

## Required Quality Gate

```sh
test -z "$(gofmt -l .)"
./scripts/lint-ui-color.sh
go vet ./...
go test ./...
```

## Optional Security Checks

```sh
task vuln
task gosec
```

## Pull Requests

Please include:

- Problem statement and scope
- Spec/backlog links
- Test evidence (commands and key output)
- Any behavioral or contract changes

## Questions and Support

- Usage questions: `SUPPORT.md` (GitHub Discussions)
- Bugs/feature requests: GitHub Issues

## Release Model

- Releases are tag-driven (`v*`) via GitHub Actions.
- Distribution artifacts are published to GitHub Releases.
- Stable tags also update the Homebrew tap (`tasuku43/homebrew-kra`).
