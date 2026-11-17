# Hotfix: dashboard renderer crash (STATUS_BREAKPOINT) on board ask click

## Context

The owner views the dashboard as an Edge 157 PWA. Clicking the body/label of an
open TECHNICAL ask row on a session board crashes the whole renderer with
`STATUS_BREAKPOINT`. The crash is intermittent, and it now happens so often that
the dashboard is unusable. The shell and all its `?frame=1` session iframes share
one renderer, so one crash kills everything.

Evidence is in `D:/Temp/pwa-crash/`: two dumps from 15:31 and 15:32. Both show
`ptype=renderer` and exception `0x80000003` in `msedge.dll`, with no OOM
annotation and no stack overflow, so this is a hard Blink CHECK.

Most likely trigger:

- The `content-visibility: auto` + `contain: layout style paint` rules on board
  topic parts, re-added in `de4b8a083`.
- Where they live:
  - `wsdashboard/assets/dashboard-board.css` ~93-95, including the
    `.bcard .bc-body` variant;
  - the `data-wbcv` marker in `cvWatch`/`cvRO`, `dashboard-frames.js` ~123-142;
  - the `contentvisibilityautostatechange` double-rAF `wbUnskip` handler,
    ~146-153.
- How the click triggers it: a label click moves focus into the topic. That
  flips `section.topic:not(:focus-within)`, which un-skips every part at once,
  together with `:has()` invalidation.
- An Electron white-window symptom earlier today was also suspected on
  `de4b8a083` + `5e5c7835f`.

Owner order, 2026-11-17 15:41: hotfix the frontend crash and restart the daemon
right away.

## Outcome

1. Remove board `content-visibility` skipping entirely, going back to the
   pre-`de4b8a083` posture:
   - the `content-visibility: auto` rules and the `contain` rule that exists
     only to support them;
   - the `data-wbcv` marking (`cvWatch`/`cvRO`) and the
     `contentvisibilityautostatechange` handler.
   - Keep anything else those commits added that is not skipping, such as
     unrelated layout fixes, unless it exists only to serve skipping.
   - Check `5e5c7835f` too. If it adds skipping or containment on the same
     path (topic parts, panel frames, bcard bodies), remove that part as well.
   - Re-locate every line; do not trust the line numbers above.
2. Make sure nothing else in the shipped assets still applies
   `content-visibility` to board topic parts, cards or panel frames. Grep under
   `agents-plugin-tool/internal`. If any remains, report it, and remove it if
   it is on the topic/ask/panel path.
3. Keep the change minimal. No new behavior, no new config.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md`. Expected tier: T0, since this is
  served CSS/JS behavior only. State it in the commit.
- Do not run, locally on this PC, `go test`, Go test exes, ws/ws-mcp exes, the
  launcher, the daemon, Electron, or a browser against the live daemon. This is
  the owner's antivirus rule. Node against static files is fine.
- Run Go tests on w5090 only, because tests may pin CSS/JS asset content:
  `D:/sys_lock.sh remote w5090 run pwa-crash-hotfix 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./internal/wsdashboard ./internal/wswhiteboard ./internal/mcp && go test ./internal/wsdashboard ./internal/wswhiteboard ./internal/mcp'`
  - Update any test that pins the removed rules.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files go only under `D:/Temp/pwa-crash-hotfix/`.
- Commit on `impl/develop/pwa-crash-hotfix` with your own model's
  Co-Authored-By trailer. Do not push.
- Speed matters: the owner cannot use the dashboard. Aim for one commit and
  one passing remote run.

## Report

- the commit;
- the removed rules and handlers with file:line;
- anything left that still uses `content-visibility`;
- the w5090 result;
- the tier.
