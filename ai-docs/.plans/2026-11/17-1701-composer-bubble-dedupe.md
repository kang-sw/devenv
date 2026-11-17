# Fix: hosted-lead chat shows each sent message twice (instant bubble not reconciled)

## Context

The owner's screenshot (2026-11-17 17:00):
`C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\d187f343f127e62e-1.png`.
The hosted lead's conversation view on the dashboard shows two messages sent
from the whiteboard composer ("여전히 새 페이지 열 때마다 10페이지인데요." and
"아니, 10초 ..") twice:
- once as normal delivered user bubbles, with a "화이트보드" source chip and
  "0분 전" timestamps, above the "리드 생각 중" (lead thinking) row;
- again BELOW the thinking row, as outlined pending/optimistic bubbles with a
  "화이트보드" chip and no timestamp.

The owner considers this plainly broken. The likely origin is the F1 composer
instant bubble, merged as `13af88dca` (look at its diff and the `## AI Context`
of its commits). That change shows an optimistic bubble on send. The
optimistic bubble evidently is not removed or merged when the real delivered
message arrives in the transcript. Possible reasons: the key that ties them
together does not match (for example the `<ws:user>` envelope or whiteboard
source wrapping changes the text), the reconciliation never runs for
whiteboard-sourced sends, or the optimistic bubble is re-rendered after the
transcript update. Find the real cause from the code, and from the stored
conversation data if needed.

## Outcome

- Each sent message appears exactly once.
- Before delivery it shows as the pending bubble. Once the transcript holds
  the delivered message, the pending bubble is replaced by it (or removed).
- This holds for whiteboard-composer sends, regular composer sends, and
  batched sends (two messages queued in one delivery, as in the screenshot).
- The instant feedback (bubble shown immediately on send) stays.
- Failure handling stays as it is (a send that fails still shows its failed
  state).

## Constraints

- Find the root cause before changing code. State it in the commit's
  `## AI Context`.
- Simplest complete change; no new abstraction. Tier: state it (expected T0,
  served JS/Go rendering only, no stored shape change). If the fix needs a
  stored-shape or protocol change, stop and report instead.
- Add a regression test at the level where the bug lives (Go test for server
  rendering, or the existing JS test seam if there is one) covering a
  whiteboard-sourced, batched two-message send.
- No local `go test`, Go test exes, ws/ws-mcp exes, launcher or daemon on this
  PC (antivirus rule). Go runs only on w5090:
  `D:/sys_lock.sh remote w5090 run bubble-dedupe 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/groove-animosity-playroom' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test -count=1 ./internal/wsdashboard ./internal/wsagent ./internal/mcp'`
  (adjust the package list to what you touched; keep `go vet ./...`).
  - Redirect the output to a file under `D:/Temp/bubble-dedupe/` and wait for
    the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Do not touch the content-visibility CSS (`dashboard-board.css:93-96`) or
  `sessionBoardAnchors` / `dashboard-session-view.js` (another worker is
  changing those).
- R1 review, then a fixes-only R2.
- Commit on `impl/develop/composer-bubble-dedupe` with your own model's
  Co-Authored-By trailer. Do not push.
- Temp files only under `D:/Temp/bubble-dedupe/`.
- Urgency: the owner sees this on every send; keep momentum.

## Report

- the root cause (file:line);
- the fix and the test;
- the commit and files;
- the w5090 result;
- anything left alone.
