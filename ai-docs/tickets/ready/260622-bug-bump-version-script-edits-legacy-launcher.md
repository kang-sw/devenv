---
title: bump-ws-version.sh edits the legacy shell launcher, not the live .py launcher
sage-review: skipped
related:
  260605-epic-ws-playbook-factory-pivot: surfaced during this epic's pre-shipping Windows-surface discussion
---

# bump-ws-version.sh edits the legacy shell launcher, not the live .py launcher

## Background

During the pre-shipping Windows-surface discussion (epic `260605`, 2026-06-22),
verifying the version-bump mechanism revealed a stale target in
`agents-plugin-tool/scripts/bump-ws-version.sh`.

The script (lines 79–86) rewrites a hardcoded compatibility glob
(`0.30.*) return 0 ;;`) in `agents-plugin/bin/ws-mcp-launcher` — the legacy POSIX
**shell** launcher (no extension, ~11.8 KB, last modified May). But
`.claude-plugin/plugin.json` runs `bin/ws-mcp-launcher.py` (the live launcher,
~30 KB, updated through this epic). The live `.py` launcher derives version
compatibility from the `runtime.json` contract (`version_compatible()` reading
`plugin_version` / `required_mcp`) and has **no** hardcoded glob.

Both plugin dirs (`agents-plugin/bin/`, `agents-plugin-wsflow/bin/`) ship both
launchers side by side.

## Impact

- **No version-bump correctness regression today.** The script also updates
  `runtime.json` (`plugin_version`, `required_mcp`, `release_tag`, tool/command
  ranges), which is exactly what the live `.py` launcher reads — so a bump takes
  effect correctly.
- The launcher-glob edit (lines 79–86) is **dead work** landing on an unused
  entrypoint, and the legacy shell launcher ships as dead weight in the plugin
  package.
- **Trap:** future maintenance of the script's launcher-glob block silently does
  nothing; a reader assumes the live launcher's compat is being maintained when
  it is not.

## Proposed follow-up

1. Confirm `bin/ws-mcp-launcher` (no extension) is truly unreferenced — no
   `.claude-plugin`/`.codex-plugin` manifest or runtime path points at it.
2. Remove the legacy shell launcher from both plugin dirs.
3. Drop the launcher-glob rewrite block (lines 79–86) from `bump-ws-version.sh`.
4. Consider a guard/test asserting the bump script only edits files that exist
   and are referenced by a shipped manifest.

Out of scope for the Windows shipping-hardening ticket; pure cleanup/debt.

## Spec Impact

Target spec area: none — dead-code removal with no caller-visible behavior change.
Contract-first spec: no

## Phases

### Phase 1: Remove legacy shell launchers; verify bump script is clean

Pre-implementation status: the bump script glob-rewrite block was already removed in a prior pass
(current bump script lines 79–86 now update `main.go`, not the shell launcher).
Remaining work: delete `agents-plugin/bin/ws-mcp-launcher` and
`agents-plugin-wsflow/bin/ws-mcp-launcher` (legacy POSIX shell wrappers, ~11.8 KB each,
no `.py` extension). Verify no manifest or script reference to the extensionless file
remains before deletion.

Completion boundary: both legacy shell launcher files deleted; no `.codex-plugin` /
`.claude-plugin` manifest or bump script references remain.
Deferred: guard/test asserting bump script only edits referenced files (noted in ticket as optional).
Verification: `grep -r "ws-mcp-launcher[^.]"` across plugin dirs and scripts returns no hits; `go build ./...` clean.
