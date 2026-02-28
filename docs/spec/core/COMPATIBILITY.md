---
title: "Compatibility policy"
status: implemented
---

# Compatibility policy

## Versioning

kra uses SemVer-style tags (`vX.Y.Z`).

During `v0.x`:

- Breaking changes may happen.
- Breaking behavior should be documented in release notes.

Starting at `v1.0.0`:

- Breaking changes are limited to major version bumps.

## Supported install methods

Supported:

- GitHub Releases (binaries)
- Homebrew (stable releases via `tasuku43/homebrew-kra`)
- Build from source (for contributors/operators)

Not supported as an end-user distribution contract:

- `go install ...@vX.Y.Z`

## Supported platforms (release artifacts)

- macOS: `amd64`, `arm64`
- Linux: `amd64`, `arm64`

Windows is not part of the current distribution plan.
