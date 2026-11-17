# Fix: the optimistic outbox bubble has no cancel; give it the held bubble's cancel

## Context

The owner asked on 2026-11-17 at 17:16 whether a message in the pending
(outlined) bubble state can be cancelled. It currently cannot, until the
server renders the held bubble. Investigation (file:line on develop):

- **Server-side cancel already exists and is race-safe.**
  - UI: `.hcancel` `data-cancel="{{.ID}}"` in `wsdashboard/assets/dashboard.tmpl:310`.
  - Click wiring: `dashboard-actions.js:43-44`, `#r-convlive [data-cancel]` →
    `cancelHeld` (`dashboard-composer.js:1607-1615`).
  - `cancelHeld` POSTs `cancel`, then `restoreHeld` puts the text back in the
    composer and shows the toast `js.cancel.ok`.
  - Route: `inbox_routes.go:639-656` → `wsinbox.Cancel` (`inbox.go:903-923`).
    Its claim rename leaves exactly one winner against drain
    (`TestDrainAndCancelRaceHasOneWinner`). It answers 409 when drain wins.
- **The client outbox** (`dashboard-composer.js:1290-1463`) has two states:
  - Unacked: a spinner row (`outboxRow`, 1318-1327).
  - Acked: a locally built bubble (`outboxBubble`, 1348-1385). It carries
    `data-entry=<id>` but no cancel or steer buttons. The comment at
    1346-1347 says they "come with the server's render".
  - Nothing calls `outboxDrop` from the cancel path, so after a cancel
    `outboxSync` (1449-1463) would re-add the local bubble within
    `OUTBOX_LINGER` (30 s).

## Outcome

1. **The acked local outbox bubble shows the same cancel button as the
   server-rendered held bubble.** Use the same markup, class, i18n key
   `held.cancel` and `data-cancel=<id>`, so the existing click wiring and
   `cancelHeld` handle it.
   - On cancel success, drop the matching outbox item (`outboxDrop` by id) so
     it does not come back.
   - On 409 (already delivered), keep today's failure handling.
2. **The unacked spinner row gets a cancel.**
   - If the post has not been sent yet, splice it from the outbox and return
     its text to the composer.
   - If the POST is already in flight, tombstone the item. When the ack
     arrives, `outboxAcked` immediately calls the server cancel with the
     returned id, and the text is restored to the composer.
   - If the post fails, today's `outboxFail` path already returns it.
   - Keep this minimal. If (2) turns out materially more complex than (1),
     ship (1) and report (2) as a Forward.
3. **The steer button** may be added to the local bubble the same way if it is
   trivial (same markup, `data-steer`, shown only when not steered).
   Otherwise leave it.

## Constraints

- **Scope and tier.** Client-side JS (and the template only if needed) under
  `agents-plugin-tool/internal/wsdashboard/assets/`. No new route or store
  operation. Tier T0.
- **Layout.** Keep the look consistent with the server-rendered held bubble:
  same button and position. No new styling unless needed.
- **Recently merged code.** The composer bubble dedupe fix just merged
  (`606b7249d`): `lastUserEntry` also matches `.user-batch[data-entry]`, and
  the test lives in `composer_outbox_test.go`. Build on it.
  - Extend that test, or add one: cancel from the local bubble posts `cancel`
    with the right id, drops the outbox item and restores the text, and a
    tombstoned in-flight send cancels on ack.
  - The node part of that test runs locally. Node is fine on this PC and is
    not in the Go test ban.
- **Do not touch:**
  - the CV CSS `dashboard-board.css:93-96`;
  - `dashboard-session-view.js` (switch-click worker);
  - the tracker view code;
  - image-serving code.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run outbox-cancel 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/tassel-sneak-enjoyably' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test -count=1 ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/outbox-cancel/`.
  - Wait for the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/outbox-cancel` with your own model's
  Co-Authored-By trailer. Do not push.
- **Temp files** go only under `D:/Temp/outbox-cancel/`.

## Report

- what was added (1, 2, 3) and where;
- the tests;
- the commit and files;
- the w5090 result;
- anything left alone.
