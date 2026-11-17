# Perf: take the outgoing-frame view save off the session-switch click path

## Context

The owner's Edge PWA trace (`Trace-20261117T164302`, analysis in
`D:/Temp/session-switch-trace/`) shows one session switch. The shell's click
handler held the main thread for 770 ms before the new frame's navigation
started. 734 ms of that was layout: 116 forced layouts of the OUTGOING frame,
each a full layout of about 6,000 layout objects.

Call chain (bundle names mapped by function name):
- click → `go` → `shOpen` → outgoing frame's `wsSaveSessionView`
  (`dashboard-lifecycle.js:452`; the outgoing frame is made inert right after
  the save, `:452-456`)
- → `saveSessionView` → `sessionBoardAnchors`
  (`dashboard-session-view.js:9-25`; line 17 `boardLivePanels`/`boardScroller`
  filters, line 18 rect map)
- → `dashboard-board.js:991`, `:1031` → `wswhiteboard/assets/stream.js:830-839`
  `rendered()` → `checkVisibility`.
- Profile: `checkVisibility` 513 ms, `getBoundingClientRect` 248 ms.
- Each of about 39 panels does three reads (two `rendered()` checks, one rect
  read), and every read finds layout dirty again (10-18 dirty objects each
  time). The likely cause is content-visibility display-lock updates triggered
  by the reads themselves (`contentvisibilityautostatechange` events are
  present). Not proven.

Content-visibility skipping must stay (owner 2026-11-17 15:56: keep
visible-only rendering). The PWA crash ticket
`261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility` is changing
only the CSS selector at `dashboard-board.css:95`. Do not touch the CV CSS rules
or the `cvRO`/`wbUnskip` code in `dashboard-frames.js`.

## Outcome

Clicking a session in the shell starts the new frame's navigation without
waiting on hundreds of milliseconds of outgoing-frame layout. The saved
session view (scroll anchors) must still restore correctly when the user
returns to that session.

Approaches, ranked; pick the simplest that keeps restore correct:
- **B.** Rewrite `sessionBoardAnchors` so it reads in one batch with no
  interleaved writes: one rect pass and cheap DOM-only `rendered` checks
  instead of `checkVisibility`. The `boardSnap` / `wbHistory.snapshot()`
  pattern near `dashboard-board.js:980` already exists. Goal: at most one
  forced layout per save.
- **A.** Start the new frame's navigation first, then save the outgoing view in
  a later task. This is valid only if the outgoing frame is still laid out and
  visible at save time. If `shOpen` hides it (`display:none` / inert), its
  rects and `checkVisibility` become meaningless, so A alone would break
  restore. Verify this before choosing A.

B and A may be combined, but only if both are needed.

## Constraints

- Frontend assets only (`agents-plugin-tool/internal/wsdashboard/assets/`,
  `internal/wswhiteboard/assets/` if needed). Tier T0: served JS only.
- The restore semantics of the saved view stay unchanged. Update any test that
  pins the anchor shape only if the shape legitimately changes; prefer that it
  does not change.
- No local `go test`, Go test exes, ws/ws-mcp exes, launcher or daemon on this
  PC (antivirus rule). Go runs only on w5090:
  `D:/sys_lock.sh remote w5090 run switch-click 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test -count=1 ./internal/wsdashboard ./internal/wswhiteboard ./internal/mcp'`
  - Redirect the output to a file under `D:/Temp/switch-click/` and wait for
    the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Node is available locally for pure JS unit tests if the repo has them; no
  browser launch is required. If the code has a JS test seam for
  `sessionBoardAnchors`, add a test that counts layout-forcing reads
  (for example by stubbing `getBoundingClientRect`/`checkVisibility`).
- Simplest complete change. R1 review, then a fixes-only R2.
- Commit on `impl/develop/switch-click-layout` with your own model's
  Co-Authored-By trailer and state the tier. Do not push.
- Temp files only under `D:/Temp/switch-click/`.
- Urgency: the owner cannot use session switching comfortably; keep momentum.

## Report

- the approach chosen and why (including whether A was viable);
- forced reads per save before → after (counted or reasoned from code);
- the commit and files;
- the w5090 result;
- anything left alone.
