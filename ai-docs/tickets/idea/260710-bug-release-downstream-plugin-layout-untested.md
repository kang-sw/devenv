---
title: release shipping does not verify downstream plugin installation layout
---

# release shipping does not verify downstream plugin installation layout

## Background

After shipping `v0.33.7`, a downstream installation reported that the
`.runtime/skills/` path is absent. The tagged plugin source contains skills at
`agents-plugin/skills/`; it does not contain `.runtime/`. The launcher reserves
`.runtime/<os>-<arch>/` for a repaired runtime binary and sets `WS_RSRC_ROOT`
to the plugin-root `rsrc/` tree.

The release workflow validates the source tree, runtime assets, and release
metadata, but it does not install the GitHub marketplace plugin into a fresh
downstream cache and exercise skill discovery plus MCP startup. The ship
configuration explicitly left that check as user-performed, so the release was
not end-to-end verified.

## Phases

### Phase 1: Reproduce and classify the downstream layout failure

Capture the exact downstream installer, host version, plugin cache path, and
full error text. Determine whether it incorrectly expects `.runtime/skills/`,
whether the marketplace entry resolves the wrong package root, or whether the
launcher/runtime bootstrap creates an invalid layout.

### Result (94b3503) - 2026-07-10

Confirmed that maximum-delegation workflow-manual loading resolves
`lead-prefer-subagent` through `WS_SKILLS_ROOT`. The launcher exported only
`WS_RSRC_ROOT`, so a runtime installed below `.runtime/<platform>/` fell back
to the nonexistent `.runtime/skills` tree instead of the packaged
plugin-root `skills/` tree.

### Phase 2: Fix launcher skills-root handoff

Export the plugin-root `skills/` tree through `WS_SKILLS_ROOT` in both the ws
and wsflow launchers without overriding caller configuration. Cover both the
helper contract and the environment passed by the launcher `main()` handoff.

### Result (94b3503) - 2026-07-10

Both launchers now export `WS_SKILLS_ROOT` alongside `WS_RSRC_ROOT` before
runtime handoff. Launcher helper and main-path regression coverage, the full
launcher test suite, wsflow package tests, and relevant Go tests pass.

### Phase 3: Add release-gated fresh-install acceptance

Add an automatable fresh-install acceptance path, or a documented and enforced
manual release gate when host UI constraints prevent automation. Verify
installed skill discovery, maximum-delegation workflow-manual loading, and MCP
startup from the downstream cache layout before a release is considered fully
accepted.
