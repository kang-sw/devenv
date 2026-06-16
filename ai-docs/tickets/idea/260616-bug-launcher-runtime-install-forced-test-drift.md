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

Decide whether `runtime_install_forced` remains a launcher helper contract that
should be restored, or whether the tests should move to the current helper
surface. Then update the launcher/test pair and rerun `python3 -m unittest
discover agents-plugin/tests`.
