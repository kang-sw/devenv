# Plan: keep the Claude lead's text from tool-calling steps in the dashboard conversation

Branch `delegate/claude-step-reply-text` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

## Problem (user report, 2026-11-07)

The dashboard conversation misses lead output on Claude: text the lead writes
in a step that also calls tools (for example a short progress line before a
tool call) shows while streaming, then disappears; only the last step's
answer stays.

Cause: `agents-plugin/hooks/claude-mod/dashboard.ts` `stepEnded` (around
L1069-1079) sends a final `reply` event only when the host's step result has
a non-empty `answer`. A tool-calling step has `answer: ''`, and the branch
`else if (reply?.step === step) finishReply()` discards the streamed text
without sending it. The Pi adapter (`agents-plugin-pi/src/dashboard-feed.ts`
`message_end`, ~L684) already records every assistant message with text, so
Pi is the reference behavior.

## Outcome

1. When a lead step ends with an empty `answer` but the step streamed
   non-empty text (`reply.blocks` for that step), send that text as a final
   reply (`{kind: 'reply', text, final: true}`, capped by `TEXT_SEND_CAP`) in
   the step's order, before later entries. Whitespace-only text sends
   nothing.
2. Keep the duplicate guard working: `turnCompleted` must not send a turn's
   answer twice when it equals the last final already sent (`lastFinal`).
   Decide whether streamed text should set `lastFinal`; the rule is no
   duplicate and no loss.
3. Subagent steps are unchanged (lead only).
4. Tests in `dashboard.test.ts` (and the engine test if it covers this path):
   a tool-calling step with streamed text produces one final reply; a step
   with only tool calls produces none; the final answer step is not
   duplicated; existing tests stay green.

## Verification

Run the claude-mod test suite the repo uses for
`agents-plugin/hooks/claude-mod/` (find the runner in the package or nearby
`package.json`/scripts). If claude-mod files are mirrored into
`agents-plugin-wsflow/` (check `ai-docs/manuals/wsflow-mirroring.md`),
update the mirror and run its drift test.

## Return

Changed files, the `lastFinal` decision, test output, and anything unchecked
(a live Claude session check is not expected).
