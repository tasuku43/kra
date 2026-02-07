#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT_DIR}"

fail=0

check_forbidden() {
  local label="$1"
  local pattern="$2"
  local mode="${3:-regex}"
  shift 2
  if [[ "${mode}" == "fixed" ]]; then
    shift 1
  fi

  local output
  if [[ "${mode}" == "fixed" ]]; then
    output="$(rg -n --fixed-strings --color never --glob '*.go' "${pattern}" "$@" || true)"
  else
    output="$(rg -n --color never --glob '*.go' "${pattern}" "$@" || true)"
  fi
  if [[ -n "${output}" ]]; then
    echo "[lint-ui-color] ${label}"
    echo "${output}"
    echo
    fail=1
  fi
}

# 1) Raw ANSI color literals are only allowed in ws_ui_common.go.
for code in \
  '\x1b[30m' '\x1b[31m' '\x1b[32m' '\x1b[33m' '\x1b[34m' '\x1b[35m' '\x1b[36m' '\x1b[37m' \
  '\x1b[90m' '\x1b[91m' '\x1b[92m' '\x1b[93m' '\x1b[94m' '\x1b[95m' '\x1b[96m' '\x1b[97m'
do
  check_forbidden \
    "raw ANSI color literal (${code}) is forbidden outside ws_ui_common.go" \
    "${code}" \
    fixed \
    internal/cli \
    --glob '!internal/cli/ws_ui_common.go'
done

# 2) Direct lipgloss.Color(...) usage is forbidden in CLI output code.
# Use semantic token helpers in ws_ui_common.go instead.
check_forbidden \
  "lipgloss.Color(...) is forbidden; use semantic token helpers" \
  'lipgloss\.Color\(' \
  internal/cli

# 3) Direct Foreground/Background with concrete colors are forbidden outside selector renderer.
check_forbidden \
  "direct lipgloss foreground/background color assignment is forbidden outside ws_selector.go" \
  '(Foreground|Background)\(lipgloss\.(Color|AdaptiveColor)' \
  internal/cli \
  --glob '!internal/cli/ws_selector.go'

if [[ "${fail}" -ne 0 ]]; then
  echo "[lint-ui-color] FAILED"
  exit 1
fi

echo "[lint-ui-color] OK"
