---
title: "Install kra"
status: implemented
---

# Install kra

Supported platforms (release artifacts):

- macOS: `amd64`, `arm64`
- Linux: `amd64`, `arm64`

For compatibility policy, see `docs/spec/core/COMPATIBILITY.md`.

## Install via GitHub Releases (manual)

1. Download a release archive for your OS/arch from GitHub Releases.
2. Extract and place `kra` on your `PATH`.
3. Verify:
   - `kra version`
   - `kra --version`

## Install via Homebrew

Homebrew uses GitHub Releases as the source of truth (stable tags only).

```sh
brew tap tasuku43/kra
brew install kra
```

Notes:

- Homebrew is intended for the latest stable release.
- Pre-release tags (example: `v0.1.0-rc.1`) are not published to Homebrew.

## Build from source

Requirements:

- Go 1.24+
- Git

```sh
go build -o kra ./cmd/kra
./kra version
```
