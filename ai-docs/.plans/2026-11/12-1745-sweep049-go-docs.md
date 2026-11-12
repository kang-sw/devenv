# 0.49 sweep repairs, batch A: dashboard Go, MCP descriptions, shipped text, manuals

## Outcome

Fix the Important findings A1-A6 of the 0.49 pre-ship review sweep
(range 62304312b..32b702129). The findings file is
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/05aaa35c-01-findings.md`;
read its batch A section first.

1. **A1, LAN slash commands (user decision, whiteboard (481) a):** in
   `agents-plugin-tool/internal/wsdashboard/inbox_routes.go` `sendCommand`
   (~271-289), a command entry is accepted only from a viewer the daemon
   treats as this machine (`localViewer` in `access.go`, the gate terminals
   use) AND only when `req.Command` is one of the names the session reported
   (`snap.Commands`). A LAN viewer's command is refused with a clear status;
   an unknown name is refused. The page should not offer the picker to a LAN
   viewer (hide or omit the cmds region for non-local viewers, whichever is
   simplest and consistent with how terminals are hidden). Add tests: a
   non-loopback request is refused, an unlisted name is refused, a local
   listed name is held.
2. **A2:** `agents-plugin-tool/internal/mcp/dashboard_page_test.go:90` is a
   stale expectation since f203fd289 (bulk count chips are buttons only on
   the served page). Normalize the bulk chips before the Contains check, as
   `wswhiteboard/regions_test.go` `bulkChip` does; prefer one shared test
   helper (for example an exported helper in a wswhiteboard test-support
   file both packages can use, or the simplest equivalent) so the two copies
   cannot drift. Test-only change.
3. **A3:** `internal/mcp/server.go` ~5189 `dashboard.intake` description:
   the accepts line names every optional kind (`wslive.OptionalKinds`),
   including `commands` and `command`; ~5206 `dashboard.inbox_drain`: add
   `mentions`, the optional `command` field, and the `command <id>` block
   head of the text format.
4. **A4:** `ai-docs/manuals/ws-mcp.md` ~382-383: match 69d028fea (a local
   viewer's outside route serves git-ignored siblings below a granted folder;
   dot paths and always-denied folders stay refused) and describe the in-root
   local/LAN preview split from 803cd0e38 (local: dot-only refusal; LAN: dot
   or git-ignored refused). Read the code to state it exactly.
5. **A5:** `ws-mcp.md` ~1177 and `ai-docs/manuals/wsflow-mirroring.md`
   ~333-336: wsflow's `claude.dashboard_enabled` is now on (47a38163e);
   correct both, and make the ws list in the same sentence complete.
6. **A6:** the `claude.dashboard_enabled` description in
   `agents-plugin/config-manifest.json` and `agents-plugin-wsflow/config-manifest.json`
   (~line 50, parity-pinned: same text) and the descriptions in
   `agents-plugin/hooks/hooks.json` and `agents-plugin-wsflow/hooks/hooks.json`
   (line 2): mention that the key also gates running slash commands the
   dashboard sends, reporting the session's command list, and holding
   dashboard messages during a compaction; keep the closing "when false"
   sentence true.
7. Also fix the docs-only minor in the same manual while there: `ws-mcp.md`
   ~906 ring size (lead holders 32 MiB `leadRingBytes`, terminals 4 MiB).

## Boundaries

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`
  (for `internal/mcp/`), `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/skill-authoring.md` (shipped description text).
- Shipped text must not reference this repository (no ticket stems, board
  numbers, ai-docs paths).
- No refactors beyond what each item needs. If A1 needs a design choice not
  covered here, stop and report.
- Never run go build/test/vet locally.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 20 -- test ./internal/mcp ./internal/wsdashboard ./internal/wswhiteboard -count=1`
  from the worktree root: all green, including `TestDashboardReadPageServesTheBoard`.
- Any parity or manifest test covering config-manifest/hooks.json (search
  the agents-plugin*/tests and agents-plugin-tool tests for the manifest
  parity check) passes.

## Return

Commit(s) on `impl/develop/sweep049-go-docs` in
`/home/swkang/.ws-worktrees/devenv/aim-hydration-granite`, this plan file in
the first commit. Do not merge or push. Report commits, what changed per
item, and test output.
