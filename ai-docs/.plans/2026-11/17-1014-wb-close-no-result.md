# Whiteboard: retire the `wb:close` result; close is `<wb:close/>`

## Context

Owner 2026-11-17 10:09-10:14: closing a topic currently requires
`<wb:close result="..."/>`. The owner retires the result field: the topic's
latest recap (newest agent log line or panel recap, already the topic-head
summary) is enough. Decisions:

- (1281): remove every mention of the result from the whiteboard playbook and
  change the syntax. This change MAY edit
  `agents-plugin/rsrc/whiteboard/whiteboard.md` (normally off-limits), but
  only the `wb:close`-related text.
- (1282)=a: the new syntax is `<wb:close/>`, with no attributes and no body.
  A lead that wants a closing line writes a `wb:log` in the same batch, before
  the close.
- Migration (owner, verbatim intent): for existing closed topics that carry a
  result, set the result to `""` and append its content as the topic's last
  agent log line (the recap).

## Outcome

1. Command:
   - `<wb:close/>` closes a topic. Everything else about close is unchanged:
     open items are withdrawn, assumed items stay, the declared state and run
     end, and later activity reopens the topic.
   - A legacy `result="..."` attribute is still accepted, but no document
     mentions it. Older plugin sessions keep calling with it.
   - When `result` is nonempty, it is stored as an agent log line placed just
     before the close event, and the close itself carries no result.
   - Compatibility tier: T0 for callers.
2. Store migration: when a board is loaded, every closed topic whose stored
   close result is nonempty is migrated once.
   - Append its text as an agent log entry timestamped at the close time, so
     it becomes the head summary unless something newer exists.
   - Set the stored result to `""`.
   - The migration is idempotent: re-opening the board must not duplicate the
     line, and a crash mid-way must not lose the text.
   - State the compatibility tier with source lines for which version writes
     and reads what. T1 is expected: an older daemon shows an empty result,
     and the text survives in the log.
3. Display:
   - The dashboard and board pages no longer render a close result. A closed
     topic shows its latest recap like any other topic.
   - The close system event text has no result.
   - Update the `whiteboard.update` success/refusal messages and the
     tool-facing strings that mention `result` for close.
4. Docs: remove the `wb:close` result from:
   - the whiteboard playbook (command table row, example, the `wb:close`
     bullet, and the one-off report sentence "the `wb:close` result is its
     one-line conclusion"; rephrase it so the conclusion is the last `wb:log`
     line or panel recap);
   - every other shipped playbook, skill or convention that writes
     `wb:close result` (grep `agents-plugin/`, `agents-plugin-wsflow/`,
     `agents-plugin-tool/`), with wsflow mirrors kept in sync per
     `ai-docs/manuals/wsflow-mirroring.md`.

## Constraints

- Before editing, read:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`;
  - `ai-docs/manuals/skill-authoring.md`;
  - `ai-docs/manuals/wsflow-mirroring.md`.
- A parallel worker changes the whiteboard nudge timers
  (branch impl/develop/board-nudge-quiet). Keep this change to close, its
  storage and rendering, and docs.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule).
  - Go tests run on remote w5090 only:
    `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard ./internal/mcp ./internal/wsdashboard'`.
  - Add any other package that owns close or rendering.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation (they need Node).
  - Plugin JS/Node tests may run locally.
- Simplest complete change; no new config.
- Temp files only under `D:/Temp/wb-close-no-result/`, deleted afterwards.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Go tests on w5090 covering:
  - `<wb:close/>` closes a topic;
  - legacy `result` becomes a log line before the close, and the head summary
    shows it;
  - migration of a stored closed topic with a result: the log line is added at
    the close time, the result becomes `""`, and a second load adds nothing;
  - reopen and close semantics are unchanged.
- Plugin doc tests and mirror drift tests pass.
- R1 review; a fixes-only R2 for substantive findings.
- Report:
  - files changed;
  - the compatibility tier with source lines;
  - test results;
  - the list of doc sites changed.
