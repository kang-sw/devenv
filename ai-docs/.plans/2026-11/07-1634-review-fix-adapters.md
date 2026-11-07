# Plan: review-sweep repair, adapter side (9768d4607..f1c811464 NEEDS FIX)

## Outcome

The range review sweep `9768d4607..develop` returned NEEDS FIX. Fix the
following findings, recorded in full (file:line, scenario, suggested fix) in
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/57c9501f-01-findings.md`
under "Slice 4". Read that section first.

- I3: the Claude tool-boundary steer drain (`claude-mod/register.ts` tool.call
  -> `watcher.toolCompleted` -> `steerBoard` -> `dashboard_inbox_drain
  {steer_only:true}`) has no timeout. Bound it (and the `serverResolved`
  wait) with a short timeout; on timeout skip that boundary so the entries stay
  held and the turn end drains them. A hung drain must not block `boardChain`.
  Test with a never-resolving MCP call.
- I4 doc drift: `agents-plugin/config-manifest.json` and the wsflow copy
  (`claude.dashboard_enabled`), `agents-plugin-pi/config-manifest.json`, the
  `register.ts` header (~74-84), and `agents-plugin/hooks/hooks.json`
  description (+ wsflow copy) must state current behavior: idle messages
  submit at once, mid-turn messages are held to turn end unless promoted to
  steer, which injects at the next main-thread tool boundary (Claude) / via
  steer delivery (Pi); the knob also feeds the `status` usage band; drop the
  "observe-only, it changes no event" claim.
- m4b: when a board submit is dropped and the text falls back to
  `board.append`, record the dashboard `user` event as the steer path does
  (`whiteboardSteered` analogue), so the message stays on the page.
- m4c: a steer row appended at a tool call the person then refuses or
  interrupts must not stay silent until the next prompt: re-submit it at turn
  end the way the refused-row path does.
- m4d: Pi `mailbox-waiter.ts` `drainBoard` idle path re-checks lead idleness
  after the drain await (as `steerBoard` does at ~:379) and carries the
  claimed text instead of losing it when Pi would refuse `sendUserMessage`.

## Settled constraints

- Files: `agents-plugin/hooks/` (claude-mod + hooks.json),
  `agents-plugin-wsflow/hooks/` (mirror: blob-identical except hooks.json
  package text; read `ai-docs/manuals/wsflow-mirroring.md`),
  `agents-plugin-pi/src/` and tests, the three `config-manifest.json`. Do not
  touch `agents-plugin-tool/` or `dashboard-config-manifest.json` (another
  delegate is editing them in parallel).
- Accepted as follow-ups, do not fix: m4a, m4e.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).

## Boundary

Commit on branch `delegate/review-fix-adapters` only; do not merge or push.
Return to the lead if a fix needs a protocol change on the ws-mcp side.

## Verification

claude-mod tests (dashboard.test.ts, watcher.test.ts and the rest of the
hooks suite, with the runner the package uses); Pi tests for
mailbox-waiter (if root node_modules is missing in the worktree, say what
could not run and run what can); `diff` the wsflow hooks mirror against ws
(only hooks.json package text may differ); the repo's package tests that
check runtime-contract / manifest drift. Report what was not checked.
