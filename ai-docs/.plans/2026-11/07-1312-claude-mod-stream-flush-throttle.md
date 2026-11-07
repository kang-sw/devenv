# Plan: throttle the Claude mod's dashboard intake while only reply text streams

## Outcome

While the lead streams a reply, the Claude mod's dashboard feed calls the
`dashboard_intake` MCP tool up to ten times a second
(`agents-plugin/hooks/claude-mod/dashboard.ts`, `FLUSH_INTERVAL_MS = 100`,
the call through `$.mcp.call` in `register.ts`). The user sees the terminal
flicker during streaming and suspects these calls (cause not yet confirmed).
User decision (whiteboard (33), "ok"): when the only pending change is
streamed reply text, flush at most once per second; discrete events (tool
begin/end, permission ask, agent spawn, user message, turn end, batch
changes) keep today's 100 ms cadence and, when one is pending, it flushes
the pending text with it.

## Settled constraints

- Time-based, not line or paragraph based: a long paragraph or code block
  must still update about once a second.
- A discrete event arriving while text is waiting on the slow timer
  reschedules to the fast cadence (it must not wait up to a second).
- The final text of a reply must not be delayed by the slow cadence: the
  reply's end (step end / turn end) is a discrete event.
- Name the new interval as an exported constant beside `FLUSH_INTERVAL_MS`
  with a doc comment saying why (terminal flicker during streaming).
- Update the existing coalescing test in `dashboard.test.ts` ("streamed reply
  text coalesces: one intake call per flush interval") and add cases: text
  alone flushes at the slow interval; a tool begin during waiting text
  flushes at the fast interval carrying the text; reply end flushes promptly.
- Mirror: `agents-plugin-wsflow/hooks/claude-mod/` carries a copy; read
  `ai-docs/manuals/wsflow-mirroring.md` and keep the mirror identical as that
  manual requires (and its drift test green).
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing.

## Boundary

Only the flush scheduling in the mod's dashboard feed, its tests, and the
wsflow mirror. Return to the lead if the change needs the intake tool, the
daemon, or the snapshot schema to change.

## Verification

The mod's test command for `agents-plugin/hooks/claude-mod` (find it in the
package's test setup; run the dashboard and engine tests), the wsflow mirror
drift test, and report outputs.
