---
title: "Releasing kra"
status: implemented
---

# Releasing kra

`kra` binaries are distributed via GitHub Releases.
Releases are triggered automatically by pushing a Git tag.

## Release pipeline

- Trigger: push tag `vX.Y.Z` or `vX.Y.Z-rc.N`
- Workflow: `.github/workflows/release.yml`
- Build/packaging: `.goreleaser.yaml`
- Release note links:
  - `docs/guides/INSTALL.md`
  - `docs/spec/core/COMPATIBILITY.md`
- Artifacts:
  - `kra_<tag>_<os>_<arch>.tar.gz`
  - `checksums.txt` (SHA256)
- Homebrew:
  - Stable tags only (no prerelease suffix) trigger a formula update PR to `tasuku43/homebrew-kra`.
  - Uses GitHub App secrets: `HOMEBREW_APP_ID`, `HOMEBREW_APP_KEY`.

## Build metadata

`kra version` prints build metadata embedded at compile time.

- `version`: tag (`vX.Y.Z`)
- `commit`: short commit hash
- `date`: build date (UTC)

In release builds, GoReleaser injects these via `-ldflags`.

## Operator checklist

1. Confirm CI is green on `main`.
2. Create a tag locally: `git tag v0.1.0`
3. Push the tag: `git push origin v0.1.0`
4. Confirm the GitHub Actions `Release` workflow succeeded.
5. Confirm the GitHub Release contains:
   - macOS/Linux archives for amd64/arm64
   - `checksums.txt`
6. Download one artifact and run `kra version` to verify the expected tag is shown.
7. For stable tags (without `-rc` or any prerelease suffix), confirm a PR was created in `tasuku43/homebrew-kra` updating `Formula/kra.rb`.
