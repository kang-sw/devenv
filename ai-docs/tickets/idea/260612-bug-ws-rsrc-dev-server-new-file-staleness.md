---
title: Dev MCP server reports newly added rsrc files as "manifest-listed file missing" until restart
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: owns the rsrc loader / playbook.render surface
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: surfaced during Phase 4 delegate-prompt porting
---

# Dev MCP server reports newly added rsrc files as "manifest-listed file missing" until restart

## Background

Dogfood surprise (2026-06-12, during `260611` Phase 4). After adding new rsrc
playbook files under `agents-plugin/rsrc/` and regenerating `manifest.json`, the
running `0.30.0-dev` MCP server failed `playbook.render` for the new playbooks
with `rsrc manifest-listed file missing: "<name>/<name>.md"`, while existing
playbooks rendered fine and the regenerated manifest correctly listed the new
files.

The server appears to **re-read `manifest.json` per call** (so it sees the new
manifest entries) but **cache its rsrc file set / directory listing in-process at
startup** (so the new files are not discoverable until the server restarts). The
mismatch — manifest lists a file the cached file set lacks — produces the
`ErrFileMissing` path.

This contradicts the documented `WS_RSRC_ROOT` dev affordance
(`ai-docs/spec/mcp-tools.md` `#260609-rsrc-playbook-distribution`): "a development
checkout can edit playbook text and see it live without waiting on plugin cache
refresh." Text edits to existing files ARE seen live (manifest re-read + per-file
hash), but **adding a new file** is not — it needs a server restart.

It did not block Phase 4: render verification was done through the authoritative
Go test layer (`internal/mcp` shipped-render tests) instead.

## Open questions

- Is the file-set caching intentional (perf) or an oversight? If intentional,
  should new-file discovery invalidate on a manifest mtime/hash change?
- Should the dev affordance doc be narrowed to "edit existing playbook text live"
  vs "add playbooks live", or should the loader re-stat the tree when the manifest
  changes?
- Is the same staleness present in production plugin-cache mode, or dev-only?

## Notes

- Captured at `idea` (exploratory); triage to `todo`/`bug` after confirming the
  caching mechanism in the rsrc loader / MCP server path.
- Workaround today: restart the MCP server (or run a fresh session) after adding
  rsrc files; verify renders via the Go shipped-render tests meanwhile.
