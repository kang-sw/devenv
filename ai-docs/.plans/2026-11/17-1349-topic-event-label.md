# Conversation topic-update bubble: show the topic label, not its stem

## Context

Owner, 2026-11-17 13:49, with a screenshot
(`C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\d0d85054e935adad-1.png`):
in the dashboard conversation view, the bubble that announces a whiteboard
topic card update shows the topic's stem (for example `send-latency`) where
it should show the topic's title. The title is the label (for example
"채팅 전송 → 처리 시작 지연").

Topics may have a stem and a label (`wb:topic name="<stem>" label="<label>"`).
Stemless topics are keyed by their name, which is already the title.

## Outcome

- The conversation's topic-update bubble shows the topic's label when one
  exists. Otherwise it shows the topic's name, as today for stemless topics.
- If the stem is used as an identifier (a link target, data attribute or
  anchor), keep it there. Only the visible text changes.
- Apply the same fix to any sibling bubble kind that renders a topic
  reference the same way, such as a topic closed or state change event. Fix
  only those that share the code path. Do not broaden the scope.
- Locate where the bubble text is produced. It is either Go server-side
  rendering in `agents-plugin-tool/internal/wsdashboard/` or
  `internal/wswhiteboard/`, or the dashboard JS assets. Fix it at the source
  that has the label available. If the label is not in the data the bubble
  receives, add it to the render data, as a T0 additive field if it crosses a
  stored shape. Do not change any on-disk shape.

## Look

- LOOK gate: before and after captures of a conversation containing a
  topic-update bubble for a stemmed topic. Use a static harness, or the repo's
  existing test or capture harness, that runs without ws/ws-mcp executables or
  Go on this PC. Put captures under `D:/Temp/topic-event-label/`.
- Compare the after capture side by side with the owner's screenshot. Only the
  name text should differ.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing. If you
  touch `internal/`, also read `ai-docs/manuals/release-compatibility.md`.
- Simplest complete change. No new config knobs.
- NO local `go test`, Go test executables, ws/ws-mcp executables, the launcher,
  the daemon or Electron on this PC (owner AV rule).
  - Node and Playwright against static files are fine.
  - Go tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run topic-label 15 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard ./internal/wswhiteboard'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/topic-event-label/`. Delete them afterwards,
  except the final captures.
- Commit on `impl/develop/topic-event-label` with your own model's
  Co-Authored-By trailer. Do not push.

## Verification

- A test asserts that a stemmed topic's update bubble renders the label and a
  stemless one renders its name.
- The w5090 run of the packages above, if Go changed.
- The LOOK captures.
- Report:
  - the commit;
  - the files;
  - the test results;
  - the capture paths;
  - the tier.
