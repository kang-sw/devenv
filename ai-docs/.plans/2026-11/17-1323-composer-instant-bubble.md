# Dashboard composer: show the sent message instantly (optimistic bubble)

## Context

Owner, 2026-11-17 13:09: after pressing Enter, the message takes a long time
even to appear in the conversation in its "not yet sent" (held) state.

A read-only trace of develop found the following; re-locate the lines.
- `sendMessage` is in `agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js:1228-1301`, and it posts with `post()` at :433-446.
- While one send is in flight, Enter is ignored (:1229 `if (sending) return`).
- The composer clears only when the POST returns OK (:1263).
- There is no optimistic bubble. The comment at :1297 says "the held bubble in
  the conversation is the confirmation".
- So the held bubble needs two things before it appears:
  - the POST round trip;
  - a server re-render pushed over the stream.

  On the owner's PC, with heavy file-syscall amplification, both are slow.
- A parallel worker is making the daemon POST return faster. This change is the
  client half.

## Outcome

1. **On Enter, at once:**
   - insert a local "sending" bubble into the conversation where the server's
     held bubble would appear;
   - clear the composer;
   - allow typing the next message.
2. **Replace on arrival.** When the server's held bubble for that message
   arrives in a render, replace the local bubble with it.
   - Match by the id the POST returns, if it returns one.
   - Otherwise, match by the order of pending local sends. Check the
     route's response in `inbox_routes.go` `handleInboxWrite`.
   - A render that does not yet contain the message must not drop the local
     bubble. Region and stream re-renders must keep it until it is matched.
3. **On POST failure:**
   - remove the local bubble;
   - restore the text, and the attachments if practical, into the composer;
   - show the existing error toast or banner.
4. **Sequential sends.** Pressing Enter twice quickly sends two messages in
   order. Two local bubbles are shown in order. If concurrent POSTs risk
   reordering, queue them client-side.
5. **Steer ("instant delivery") and answer sends** follow the same path where
   they use `sendMessage`. Do not change what is sent.

## Look

- The local bubble looks like the existing held bubble, with a subtle
  "sending" marker (for example, reduced opacity or a small spinner, using
  existing CSS tokens). It is not a new design.
- LOOK gate: produce before and after captures of the conversation right after
  Enter, plus the failure state. Compare them side by side with the existing
  held bubble.
  - Use the repo's existing dashboard browser-test or capture harness if it can
    run without ws/ws-mcp executables or Go on this PC.
  - Otherwise, use a static harness page that loads the real dashboard assets
    with a stubbed `post()` and stream. Put it under
    `D:/Temp/composer-instant-bubble/`.
- Report the capture paths.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Dashboard JS and CSS only. Do not change Go, the route or the payload (T0,
  embedded assets only).
- Simplest complete change. No new config knobs.
- NO local `go test`, Go test executables, ws/ws-mcp executables, the launcher,
  the daemon or Electron on this PC (owner AV rule).
  - Node and JS tests and Playwright against static files are fine.
  - If Go tests cover embedded assets (such as asset-hash or template tests),
    run them on w5090 only:
    `D:/sys_lock.sh remote w5090 run instant-bubble 15 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/composer-instant-bubble/`. Delete them
  afterwards, except the final captures, which the lead reviews.
- Commit with your own model's Co-Authored-By trailer. Do not push.

## Verification

- JS tests, or harness checks, for:
  - the bubble appears before the POST resolves;
  - it is replaced exactly once by the server bubble;
  - a render without the message keeps it;
  - a failure restores the text;
  - two quick sends keep their order.
- LOOK captures: before, after and failure.
- Report:
  - the commit;
  - the files;
  - the test results;
  - the capture paths.
