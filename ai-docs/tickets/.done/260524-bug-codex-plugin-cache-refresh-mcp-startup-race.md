---
title: Codex plugin cache refresh can race ws MCP startup
related:
  260523-bug-ws-mcp-launcher-runtime-repair-race: adjacent launcher-side first-start repair race that was already hardened
related-mental-model:
  - plugin-runtime
  - mcp-runtime
completed: 2026-06-15
---

# Codex plugin cache refresh can race ws MCP startup

## Background

Local dogfooding showed ws MCP startup failing while Codex was refreshing the
installed plugin cache. The failure is distinct from the launcher runtime repair
race fixed in `260523-bug-ws-mcp-launcher-runtime-repair-race`: the MCP process
started from the expected installed cache path, but `runtime.json` and multiple
skill files were missing at the instant Codex tried to load them.

Observed log shape:

- Codex cleared skill caches and refreshed MCP servers.
- Codex reported `plugin is not installed` for
  `/Users/kang-sw/.codex/plugins/cache/kang-sw-devenv/ws`.
- A new session initialized MCP with `ws` enabled.
- The skill loader reported missing files under
  `.../ws/0.28.1/skills/...`.
- The launcher failed with
  `ws-mcp-launcher: missing runtime contract: .../ws/0.28.1/runtime.json`.
- Seconds later the cache path contained the complete `0.28.1` plugin tree, and
  `codex mcp list` reported the same `ws` entry enabled.

The current launcher handles concurrent runtime install/repair under an already
materialized plugin tree, but it cannot read `runtime.json` before the host has
finished materializing the plugin package. The likely boundary is Codex plugin
cache install/refresh atomicity or startup ordering.

## Spec Impact

Target spec area: `ai-docs/spec/plugin-runtime.md` launcher startup and runtime
compatibility contracts.

Expected caller-visible change: plugin-managed MCP startup during local Codex
plugin refresh should either wait briefly for the installed package tree to
contain the required contract files, or fail with diagnostics that make the
package-materialization boundary clear rather than looking like an ordinary
runtime repair failure.

Contract-first spec: no. The exact mitigation should be chosen after the
reproduction/classification pass distinguishes a host cache materialization race
from ws launcher runtime repair.

## Phases

### Phase 1: Reproduce and classify the cache materialization race

Create a focused repro around Codex local plugin refresh or reinstall while a
session starts or refreshes MCP. Capture whether Codex exposes a versioned
plugin directory before all files are copied, removes and recreates the same
version directory in place, or refreshes MCP before the installed plugin marker
is durable.

Verification boundary: logs or a small probe must distinguish this host cache
race from ws launcher runtime repair, missing release assets, Windows Python
availability, and ordinary MCP startup timeout.

### Result (3c1518d9) - 2026-06-15

The race was classified as a package-materialization window: the Python
launcher can be invoked from a versioned plugin cache after `bin/` is visible
but before `runtime.json` is present. That is distinct from runtime repair,
release asset lookup, Python availability, and ordinary MCP startup timeout
because runtime compatibility has not started yet.

The implementation added a focused launcher unit test that simulates
`runtime.json` appearing after the first wait interval, plus a failure-path test
that verifies the expired wait names plugin package materialization directly.

### Phase 2: Choose a mitigation boundary

Evaluate whether ws can mitigate this from the launcher side by briefly waiting
for `runtime.json` and a package-ready sentinel, or whether the proper fix
requires Codex plugin cache atomicity/retry behavior. Preserve stdout silence for
stdio MCP and keep diagnostics on stderr.

Verification boundary: a session started during plugin refresh either delays
until the package tree is complete or produces a retryable/actionable failure,
without regressing normal hot-path startup.

### Result (3c1518d9) - 2026-06-15

ws can mitigate the launcher-visible part of the cache race by waiting briefly
for `runtime.json` before reading the runtime contract. If the file appears,
startup continues into the existing compatibility path. If the wait expires,
the launcher fails on stderr with a package-materialization diagnostic instead
of the previous generic missing-contract repair shape.

This does not claim to fix host skill-bundle reads that happen before the MCP
launcher runs; those remain a Codex plugin-cache atomicity or refresh-order
boundary. The implemented mitigation keeps stdout silent for stdio MCP and does
not change the normal hot-path once `runtime.json` is already present.
