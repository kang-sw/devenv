# Dashboard: move the topic state icon to the left end of the topic header

## Context

Owner 2026-11-17 10:05, small UX request with a screenshot
(`C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\8867cf0197867c1c-1.png`):
in a whiteboard topic card header the topic work-state icon (the round
orange speech-bubble badge shown for `discussing`; other states have their
own icons) sits at the right end, next to the relative time ("7분 전") and
the pin. The owner wants that state icon moved to the LEFT end of the header,
before the topic title.

## Outcome

1. The topic state icon renders at the left end of the topic header row,
   before the title, for every state (discussing, researching, waiting,
   implementing, awaiting-confirmation) and wherever the same topic header
   is used (board page in the browser and the Electron dashboard panel; the
   compact/rail variant too if it shares the markup).
2. Its existing behaviour is unchanged: tooltip/popover, clock/deadline
   text, click handling, and accessibility labels.
3. The right side keeps the relative time and pin; nothing else moves.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- Smallest change: reorder markup or CSS (`order`) in the existing header;
  no new component. Another worker is about to change card rendering for
  performance (visible-only processing); keep this change local to the header.
- Update any test that asserts the header's child order.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule). Go tests only on remote w5090:
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync '<this worktree>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard ./internal/wsdashboard'`.
  Accepted reds: only TestTopicStateBadgeResearchingMetadata and
  TestTopicStateBadgeClockAndReconciliation (need Node, absent on w5090) -
  run those two locally with Node only if they are JS-driven tests you can
  invoke without Go; otherwise note them.
- LOOK: render a static fixture of the board page (generated HTML or the
  existing test fixture) in a local headless browser and capture before and
  after screenshots under `D:/Temp/topic-state-icon-left/`; compare against
  the owner's screenshot. Do not start the ws daemon or Electron.
- Temp only under `D:/Temp/topic-state-icon-left/`, delete after (keep the
  two PNGs until the lead has seen the report, then they may be deleted).
- Commit on this branch with `## AI Context`. Do not push develop or main.

## Verification

- Before/after screenshots showing the icon at the left end.
- Affected Go packages pass on w5090 (accepted reds above).
- Report: files changed, commit, screenshots paths, test results.
