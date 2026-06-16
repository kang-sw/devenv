---
title: restore or retire launcher runtime_install_forced test contract
related:
  260616-refactor-remove-agent-backed-api-tools: discovered while running package tests during API tool removal
---

# Restore or retire launcher runtime_install_forced test contract

## Finding

`python3 -m unittest discover agents-plugin/tests` currently fails because
`agents-plugin/tests/test_ws_mcp_launcher_capabilities.py` expects
`ws_mcp_launcher.runtime_install_forced`, but
`agents-plugin/bin/ws-mcp-launcher.py` no longer exposes that function.

The failure appears unrelated to agent-backed API tool removal: the current API
tool deletion diff does not edit the launcher module or its Python tests.

## Follow-Up

Restore `runtime_install_forced(plugin_dir, os_name)` as the launcher helper
contract expected by the Python launcher tests. The helper should return true
when a bootstrap runtime source is provided or a valid local-devenv runtime
contract is active for the current plugin directory and host OS.

## Decisions

- **Restore the helper instead of deleting tests.** Existing tests exercise a
  readable launcher contract used to distinguish forced install paths from
  ordinary release downloads. Removing the tests would hide the bootstrap/local
  repair policy.
- **Keep local contract validation centralized.** `runtime_install_forced`
  should delegate to `local_devenv_runtime_enabled`, which already handles
  plugin-cache/package detection, marker validation, and Windows exclusion.
- **Do not change runtime installation behavior.** This is a launcher/test
  contract drift fix, not a new runtime staging policy.

## Spec Impact

Target spec area: plugin runtime launcher compatibility.
Expected caller-visible change: none; restore an internal launcher helper used
by tests while preserving existing bootstrap/local repair behavior.
Contract-first spec: no.

## Phases

### Phase 1: restore launcher forced-install helper

Add back `runtime_install_forced(plugin_dir, os_name)` in
`agents-plugin/bin/ws-mcp-launcher.py`, wire `main()` through it where useful,
and preserve current runtime installation behavior.

Verification:

- `python3 -m unittest discover agents-plugin/tests` passes.
- Targeted launcher tests covering bootstrap, local-devenv marker, invalid
  marker fallback, and release download paths pass.
