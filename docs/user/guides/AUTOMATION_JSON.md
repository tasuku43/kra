---
title: "Automation JSON guide"
status: implemented
---

# Automation JSON guide

This guide describes how to use `kra` commands in scripts with JSON output.

## Envelope shape

Commands that support JSON output return a shared envelope:

- `ok`
- `action`
- `workspace_id` (when applicable)
- `result`
- `error`

## Basic example

```sh
kra ws create --format json --id TASK-1234 --title "API retry hardening"
```

Typical success response:

```json
{
  "ok": true,
  "action": "ws.create",
  "workspace_id": "TASK-1234",
  "result": {
    "created": 1,
    "path": "<KRA_ROOT>/workspaces/TASK-1234"
  },
  "error": null
}
```

## Script usage pattern

Use `ok` for success/failure branching and `error.code` for machine decisions.

Example with `jq`:

```sh
out="$(kra ws create --format json --id TASK-1234)"
if [ "$(printf '%s' "$out" | jq -r '.ok')" = "true" ]; then
  printf 'created: %s\n' "$(printf '%s' "$out" | jq -r '.result.path')"
else
  printf 'error[%s]: %s\n' \
    "$(printf '%s' "$out" | jq -r '.error.code')" \
    "$(printf '%s' "$out" | jq -r '.error.message')" >&2
  exit 1
fi
```

## Notes

- Not all commands support `--format json`.
- JSON mode is non-interactive for commands that define JSON contracts.
- Keep script logic stable by depending on `action` and `error.code`, not human text.

## Related docs

- Command overview: `docs/user/guides/COMMANDS.md`
- Output contract: `docs/dev/spec/concepts/output-contract.md`
