---
title: Codex plugin cache can retain a runtime without its rsrc manifest
related:
  260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood: adjacent stale-cache symptom
  260703-bug-claude-plugin-cache-stuck-below-source-version-mcp-refuses-start: adjacent plugin cache/runtime skew
---

# Codex plugin cache can retain a runtime without its rsrc manifest

## Background

After fast-forwarding devenv to v0.43.5, the active Codex ws MCP process still
identified its installed path as `ws/0.43.4`. Both `playbook.render` and
`playbook.print` failed with `rsrc manifest missing at
/Users/kang-sw/.codex/plugins/cache/kang-sw-devenv/ws/0.43.4/rsrc/manifest.json`.
The newer `ws/0.43.5` cache directory contains the launcher and CLI fallback,
so the failure is a stale or incomplete active plugin cache rather than a
missing source manifest.

## Phases

### Phase 1: Reproduce and make cache/runtime skew recoverable

Determine how Codex selected an incomplete v0.43.4 runtime after a source
update, reproduce the state with an installed-cache test or diagnostic, and
ensure playbook calls fail with actionable cache-refresh guidance or select a
complete matching runtime. Preserve the existing user-performed cache refresh
boundary; do not silently mutate Codex plugin state from ws.

Verification: after a source update and cache refresh, `playbook.print` and
`playbook.render` both resolve a shipped rsrc playbook successfully.
