# Release-gate fix, unit C: whiteboard / wsagent / shipped text

The ws 0.48.13 release gate (lead-review of `11ef849000956f9e0282def355e3298b51f9faf9..889db46fb`) returned NEEDS FIX. User decisions (board, 2026-11-09): (146) fix locally; (147) three parallel worktree units, then re-review; (148) a - no new JS tests in this round beyond the dashboard's B1/B2 (so C6, pill.js/form.js tests, becomes a follow-up, not work here); (149) ok + (150) a - the Windows scrollbar restyle is included. This unit owns `internal/wswhiteboard`, `internal/wsagent`, the shipped playbooks under `agents-plugin*/`, and the wiki tool descriptions in `internal/mcp/server.go:4921-4972` (descriptions only; unit A edits other parts of `internal/mcp`, keep your hunk confined to those description strings). Unit A (wsledger/host/mcp) and unit B (wsdashboard) run in parallel on other branches; do not edit their files.

Full findings file (read-only reference): `/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/e20afafa-01-findings.md`, section "Unit C".

## Fixes

- **C1** `internal/wswhiteboard/view.go:558` + `store.go:731-736`: assumed items stored before this release have `Settled == 0`, so after upgrade they sort as the oldest settled rows and drop into the collapsed list. Order an assumed item with `Settled == 0` by `Changed` (or as newest). Test.
- **C2** `store.go:544-554` with applyAnswer `:721-724`: `wb:close` before a `wb:answer` for one of its open items in the same batch refuses the batch with "patch its status to open before answering it". The refusal names the earlier close in the same batch and says to put answers before the close; the whiteboard playbook's `wb:close` bullet says answers go before it. Test for the refusal text.
- **C3** `agents-plugin/rsrc/whiteboard/whiteboard.md` Re-ask bullet (and mirrors): it says the dashboard drops answers sent before the patch, but `sentMarks` only hides the sent mark; a held inbox entry with the old letter still drains to the lead. Fix the wording: the sent mark is dropped; an answer already held still arrives and refers to the old options.
- **C4** `internal/wsagent/claude.go:463-470` + tracker_show.go: a `tracker.show` called by a subagent of the support agent never draws a card (its tool_result carries `parent_tool_use_id`). The support instructions say only the agent's own `tracker.show` draws a card.
- **C5** `agents-plugin/rsrc/tracker/tracker.md:15-23` (mirrored in wsflow and pi) and the `wiki.query` / `wiki.update` descriptions (`internal/mcp/server.go:4921-4972`): the agent-facing docs never define the `wiki:<name>/<path>` link form, and the tracker playbook does not list the wiki tools. Add a short wiki paragraph in the playbook `## Calls` (tools, read-then-save-with-base, how `wiki:<name>/<path>` maps to them with the store from context, that bodies may use such links); mirror to wsflow and pi; refresh manifest hashes; one clause in the tool descriptions defining the link form.

## Scrollbar restyle (149 / 150)

On Windows the whiteboard page shows the OS default scrollbars: wide, opaque grey track. There is no scrollbar CSS in `internal/wswhiteboard/assets/page.css`.

- Thin scrollbar, transparent track, semi-transparent thumb derived from the page's theme tokens (light and dark), a little stronger on hover.
- Standard `scrollbar-width: thin` / `scrollbar-color` plus `::-webkit-scrollbar*` rules. Chromium 121+ ignores `::-webkit-scrollbar` on an element whose `scrollbar-width`/`scrollbar-color` is set, so pick a combination that gives the intended look on current Chrome/Edge on Windows and on Firefox, and does not regress macOS overlay scrollbars.
- Covers the dashboard-served page, the file page, and the embed snapshot if they share the stylesheet (check how page.css reaches each).
- Unit B applies the same spec to the dashboard and tracker; match it so the two read alike.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/skill-authoring.md`, and `ai-docs/manuals/wsflow-mirroring.md` before editing shipped text.
- Do not touch the user's dashboard daemons on ports 29417 / 45743; a headless check uses a dev daemon with isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`, or the file page. Never run `git stash`. Do not push. Do not edit `ai-docs/tickets/`.
- Minors on contact may be fixed (e.g. uneven wrapping in `whiteboard.md:22-27,133-139`); do not go looking for more.

## Verification

`cd agents-plugin-tool && go test ./internal/wswhiteboard/... ./internal/wsagent/... ./internal/mcp/...` plus the package drift tests for the mirrored playbooks (whatever covers manifest hashes and wsflow/pi mirrors), then full `go test ./...` once. Headless Chromium screenshot of a whiteboard page with a scrolled area, light and dark; screenshots to your scratchpad, paths in the report. One or more commits on branch `fix/gate-0.48.13-whiteboard`, this plan committed with the first.
