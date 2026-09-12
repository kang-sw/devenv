# Plan: 260911-chore-ws-pi-track-sync-to-refound — Phase 1: Re-point the marker and absorb the settled ws-mcp

## Relevant Ticket Contract
- Re-point the gitignored, worktree-local `agents-plugin-pi/.local-devenv-runtime` marker to the `develop` worktree currently resolved by `git worktree list` (`/Users/kang-sw/devenv`), preserving its absolute-path schema and `/opt/homebrew/bin/go`; it tracks that worktree's checkout and must not become a tracked portable default or a SHA/branch pin.
- Absorb the post-refound `develop` `agents-plugin/runtime.json` and complete `agents-plugin/rsrc/` surface, then copy the complete runtime contract and rsrc tree verbatim into `agents-plugin-pi/`. This includes the seven retired documentation-discovery tools removed, `git.merge` added, and all 23 retired command entries removed; do not hand-pick a tool-only subset.
- Reconcile Pi-authored runtime-contract references, including captured bridge tool/manual fixtures, so Pi neither registers nor documents retired entries. Verify a live Pi session runs the develop-root binary and exposes the settled surface.
- Preserve the Pi-track-only authorship boundary: shared ws-mcp/rsrc content is imported from `develop`, never authored on this track. Keep the `260906` byte-identical runtime/rsrc/launcher guard green and confirm the retired compat worktree is still absent.

## Out of Scope
- Any new or edited ws-mcp Go source, shared rsrc wording authored on the Pi track, or a replacement frozen compat worktree.
- Pi-specific rsrc overlay work owned by a separate upstream child.
- `ai-docs/.plans/2026-09/06-1203-260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches.md`, an unrelated untracked plan; leave it untouched and out of staging/commit scope.
- Later ticket phases and unrelated Pi adapter behavior changes.

## Codebase Findings
- `agents-plugin-pi/.local-devenv-runtime#L1-L6` — the ignored marker currently targets this Pi worktree; its `source_root` and `tool_dir` must instead point at `/Users/kang-sw/devenv` and `/Users/kang-sw/devenv/agents-plugin-tool` while retaining schema version and the local Go executable.
- `agents-plugin-pi/src/local-devenv.ts#L152-L200` — the Pi adapter reads the marker's `source_root` for the commit stamp and builds `./cmd/ws-mcp` with `cwd: marker.tool_dir`; changing both paths is necessary for an actual develop-root build, not merely diagnostic text.
- `/Users/kang-sw/devenv/agents-plugin/runtime.json#L1-L83` — the settled contract has 51 tools and 18 commands; compared with `agents-plugin/runtime.json#L34-L110`, it removes the seven specified tool entries and 23 commands and adds `git.merge`.
- `agents-plugin-pi/src/index.ts#L157-L173` and `agents-plugin-pi/test/version-check.test.ts#L49-L129` — Pi deliberately hand-copies `runtime.json`, `rsrc/`, and the launcher; the existing guard compares full file/tree bytes and rejects missing or extra mirror files, so copying both source and mirror is required.
- `agents-plugin-pi/test/bridge.test.ts#L43-L113` — the Pi bridge retains a 60-tool captured list and workflow-manual fixtures that name retired `mental_models.*`, `references.trace`, and spec tools. Its comments prescribe re-capturing via `spawnWsMcpClient`, `initialize`, `ferrule`, `playbook.read`, and `workflow_manual`, rather than hand-editing a synthetic fixture.
- `/Users/kang-sw/devenv/agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L135-L166` — the settled shared manual now describes lead-owned `git.merge`; importing the entire rsrc tree, then refreshing Pi's captured manual fixtures, prevents a partial contract sync.

## Implementation Plan
1. Re-read `git worktree list` immediately before editing, then update only the ignored `agents-plugin-pi/.local-devenv-runtime` local marker: set `source_root` to the live develop worktree and `tool_dir` to its `agents-plugin-tool/`; retain `schema_version: 1`, the absolute Go path, and the marker's ignored/local-only status. Do not add a ref, SHA, or tracked fallback.
2. Import only the settled shared artifacts from `/Users/kang-sw/devenv`: replace this track's `agents-plugin/runtime.json` and `agents-plugin/rsrc/` with the develop copies, then verbatim-copy that runtime file and full rsrc tree to `agents-plugin-pi/runtime.json` and `agents-plugin-pi/rsrc/`. Treat these as develop-originated absorption, not Pi-track authorship; leave the already-identical launcher unchanged unless a new develop comparison proves otherwise.
3. Refresh Pi-owned contract consumers in `agents-plugin-pi/test/bridge.test.ts` and `agents-plugin-pi/test/fixtures/workflow-manual-{static-body,response}.txt` from a live locally built develop-root ws-mcp session using the documented capture path. Derive the tool-name count/list from that session, include `git.merge`, remove all retired tool references, and preserve the fixture trimming/anchor-cut test semantics rather than weakening them.
4. Search Pi-authored `src/`, `test/`, and committed package guidance outside the newly mirrored rsrc tree for the seven removed tools and retired command vocabulary; update/delete only stale contract references. Confirm no Pi file reinterprets the shared runtime surface and that the compat worktree name remains absent from `git worktree list`.

## Verification Plan
- From `agents-plugin-pi/`, run `npm test`; it includes the `260906` full-tree/runtime/launcher byte-identity guard.
- Run `diff -rq agents-plugin/rsrc agents-plugin-pi/rsrc` and byte-compare both `runtime.json` files; both must be empty/equal. Compare the shared copies against `/Users/kang-sw/devenv/agents-plugin/` as well.
- Start the Pi extension with the local marker and inspect the develop-root runtime's tool list/capabilities. Compare it with the develop checkout at execution time (not a frozen SHA): `git.merge` is present; `specs.query`, `mental_models.list/query/status`, `references.trace`, `spec_index.verify`, and `spec_stem.generate` are absent; and the removed command entries are absent.
- Confirm the live session/build diagnostic resolves `/Users/kang-sw/devenv` and its current `git rev-parse --short HEAD`, then verify `git worktree list` has no `ws-mcp-compat-a937b8dc` entry. Check `git status --short` before commit so the unrelated `06-1203` plan remains unmodified and unstaged.

## Escalations
- None.
