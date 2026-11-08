# Plan: whiteboard topic header - pen note, updated-ago, close result, closed-topic cap

Branch `delegate/whiteboard-topic-header` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here (one commit per logical unit is fine); do not merge or
push. Do not run `git stash`.

Source: user decisions on the session whiteboard, 2026-11-08, items
(478)-(483). Topics today show only a bare name; closed topics pile up
without a cap; nobody records how a topic ended.

Read first: `ai-docs/manuals/shipped-surface-boundary.md`,
`ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/skill-authoring.md` and
`ai-docs/manuals/wsflow-mirroring.md` (the `whiteboard` playbook changes).
Code: `agents-plugin-tool/internal/wswhiteboard/` (store.go, view.go,
render.go, commands.go, assets/page.tmpl, form.js, live.js, page.css),
`internal/mcp/whiteboard_tools.go`, the dashboard daemon that serves the
session page (`internal/wsdashboard/`), UI strings in
`internal/wsi18n/tables/*.json`, playbook
`agents-plugin/rsrc/whiteboard/whiteboard.md` with its wsflow mirror and the
hand-copied Pi mirror (`agents-plugin-pi/rsrc/`; the stale-topic nudge commit
`1a7e75cf4` shows the mirroring and manifest regeneration steps).

## Settled behavior

1. **Topic pen note (478 a).** Each topic bar gets a pen button, like the
   item pen. It opens a note box under the topic bar; non-empty text adds a
   line `[<topic name>] <text>` to the reply string the copy-answers button
   builds (after the item segments). Update the playbook's `#### Answers`
   section with one rule: a `[topic] text` segment is the user's comment or
   question about that topic, answered in that topic (no item ID needed).
2. **Updated-ago (479).** The stale-topic nudge already added `ChangedAt`
   (RFC 3339) to items and topic bodies; reuse it. The topic bar shows the
   topic's latest change (max over its items and body) as relative time
   ("5 min ago"); the full page refreshes it client-side every minute. The
   embed is a frozen snapshot: show local `HH:MM` instead. A topic with no
   time (old store) shows nothing.
3. **Close result (480 c, 482).** `wb:close` takes a required `result`
   attribute, one sentence saying how the topic ended. A close without it is
   refused with the usual problem listing. A closed topic shows the result
   dimmed beside its name. When the topic reopens (new item or body change)
   the result is hidden but kept in the store; the next close replaces it.
   Topics closed before this change have no result and show none. Update the
   playbook command table, the `wb:close` bullet, and the example block.
4. **Closed-topic cap (483).** Order closed topics by close order (the
   store's closed-topic list order is already close order). The dashboard
   session page renders the 30 most recently closed topics; a "show more"
   control fetches the next 30 from the daemon (add the smallest endpoint or
   query parameter the existing page-serving path allows; keep it read-only
   and session-scoped like the page itself). The file page (no daemon) and
   the embed hard-cut at 30 and print one line "N more closed topics not
   shown". Open, quiet (not closed), and waiting topics are never capped.
   The store keeps everything.
5. New UI strings go through the existing i18n tables (all languages; a
   machine draft is acceptable, as with earlier dashboard strings).

## Tests (each fails before the change)

- Close without `result` refused; with it stored and rendered beside the
  name; hidden after reopen; replaced on re-close; old closed topic renders.
- Topic bar renders updated-ago data (page) and `HH:MM` (embed); none for a
  topic without times.
- Pen note: the reply builder output includes the `[topic] text` line (if no
  JS harness exists, cover the template output and verify the JS by a cheap
  headless Chromium check; cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`).
- Cap: 35 closed topics -> page shows 30 plus a more control; the endpoint
  returns the rest; file page/embed show 30 plus the "N more" line.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wswhiteboard/ ./internal/mcp/ ./internal/wsdashboard/ ./internal/wsi18n/ && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wsrsrc/ && go test -count=1 -run 'Whiteboard|Dashboard' ./internal/mcp/`

## Return

Commit hashes, changed files, test output (which new tests failed before),
screenshots path if a browser check ran, and anything left open.
