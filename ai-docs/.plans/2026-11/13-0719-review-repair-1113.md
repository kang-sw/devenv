# Plan: repair the 2026-11-13 develop review sweep findings

## Intent

The review sweep over `e7ef18a913882f3fb02fdc94533026c57859931d..6faab3fc3`
returned NEEDS FIX. The user chose a local bounded repair (whiteboard
(632) a). A live run on an isolated daemon then confirmed I4 and found two
small defects, added below. The full findings, with file:line and failure
scenarios, are in `/tmp/review-findings-1113.md`. Read it first; it is
the contract for items I1-I8 and the Minor list.

## Change

1. **I1** dashboard.css sent-state selectors: restore `.arow` (all five
   rules, including `:focus-within`). Dim sent and held check rows the same
   way, under the class item 2 gives them.
2. **I2** rename the whiteboard check-row class away from `.crow` (for
   example `.ckrow`) in every place that uses it: page.css, page.tmpl,
   form.js, pill.js, dashboard.js, and the Go and node tests. Do not scope
   the composer rule instead: the board must not depend on dashboard
   scoping.
3. **I3** runner restart: settle the interrupted turn before the loop can
   kick a new turn.
4. **I4** `pi.wait` on a child turn whose report was already handed to the
   parent (steered in, or queued in Reports) must not hand it over again.
   Make the behavior match the shipped worker-facing description. Do not
   narrow the description.
5. **I5** after a daemon restart, `pi.wait` on a child whose last turn
   settled before the restart answers at once, seeded from persisted record
   state.
6. **I6** a held message cancelled before drain must not leave a child
   counted as open. `turnMark` covers only what the turn actually sent; a
   leading slash-command turn does not answer a wait for a later text
   message.
7. **I7** tests for the check-row client paths in dashboard.js and pill.js
   (`readItem`/`writeItem` round trip for `no`, `tlDrafted`/`TL_ASK`,
   `responded()`). Extract `TL_ASK` from dashboard.js rather than
   hand-copying it into `toast_test.go`.
8. **I8** a Go test that mounts the real wsagent token route
   (`/agents/by-token/*`) behind `httptest` and drives it with the real
   `pi_forward.go` forwarder: spawn, list, wait, send, and close, checking
   status codes and JSON shapes.
9. **User decision (633) a.** When the caller is a Pi conversation (a
   worker), a child's empty `cwd` defaults to the holder conversation's own
   working directory (`rec.Cwd`), and a relative `cwd` resolves against it.
   When the caller is a lead, keep today's rule (the lead's root). Update
   the worker-facing `pi.spawn` schema text and the Pi spawn tree section
   of `ai-docs/manuals/ws-mcp.md` to say so. Add a test.
   - This revises lead-path ticket Decision 3 ("default the caller's root")
     for conversation holders. That ticket is `.done` and frozen, so record
     the revision in the commit's `## AI Context`, citing (633).
10. **Live-run additions:**
    - `pi_forward.go` `fail` adds the tool name a second time
      (`pi.spawn: pi.spawn: ...`). Prefix it once.
    - `agents-plugin/tests/test_ws_mcp_launcher_proxy.py:288-293`: resolve
      the temp root. On macOS, `/var` resolves to `/private/var`.
11. **Minors from the findings file:**
    - ws-mcp.md: add the `WS_MCP_CONFIG_MANIFESTS` row entry, and add
      `xlarge` to the tier order.
    - Localize the code.js copy strings through `window.wsT`, with a
      fallback, adding the `js.wb.*` keys to every i18n table. The ko value
      keeps "복사" / "복사됨".
    - Pin the `chg-stale` class in a render test.
    - Bound the `dashboard.test.ts` wait loop in ws and in the wsflow mirror.

Leave the findings file's "Minor follow-ups (accepted, not fixed now)"
section alone.

## Verification

- Every new behavior and fix gets a test that fails before the fix and
  passes after, where that is practical. Say which tests were confirmed to
  fail first.
- `go test ./...` and `go vet ./...` in agents-plugin-tool. The only
  expected failure is `cmd/ws-mcp
  TestHostedConnectionSeeingALANChangeReplacesTheDaemon`, which expects
  master; this machine uses main.
- `claude plugin test agents-plugin` and `claude plugin test
  agents-plugin-wsflow`.
- Run the Python launcher test with `python3 -m unittest`.
- Run the drift and mirror guards.
- Embed budget: if the renames push the budget past 42%, report the number.
  Do not raise the cap on your own.
- Take headless Chromium screenshots of an open check row and a sent ack
  row on the dashboard page, before and after, light and dark. Use the
  cached Playwright under `~/.npm/_npx/*/node_modules/playwright`. Report
  the paths.
- Report the exact commands and results.

## Constraints

- Before editing, read these manuals:
  - `ai-docs/manuals/shipped-surface-boundary.md`
  - `ai-docs/manuals/release-compatibility.md`
  - `ai-docs/manuals/ws-mcp.md`
  - `ai-docs/manuals/wsflow-mirroring.md`
- The cwd default change and the schema text are T0. They change no
  argument or field and add no store shape.
- Never run `git stash`. Work only in this worktree, on the branch
  `impl/develop/review-repair-1113`.
- Make one commit per logical unit. Each commit carries `## AI Context`
  citing the finding ID, and (633) where it applies.
- Stop and report if a fix needs a new contract decision beyond this plan.
