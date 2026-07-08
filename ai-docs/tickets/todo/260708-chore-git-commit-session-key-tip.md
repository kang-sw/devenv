---
title: "Add a session-key preservation tip to git.commit responses"
related:
  260708-feat-lead-revive-hook-replacement: prerequisite
spec: 260626-post-compaction-session-restoration
sage-review-design: completed
---

# Add a session-key preservation tip to git.commit responses

## Background

Split out of `260708-feat-lead-revive-hook-replacement` (the lead-revive
hook-replacement ticket) as an independently shippable piece: that ticket's
Decisions section notes that the post-compaction hook design does not
*depend* on the compaction summary reliably preserving the session key (an
unrecoverable key already falls back to the `obsidian-latch` sentinel,
which `workflow_manual` accepts to bootstrap safely). But that fallback
mints a fresh key and discards the prior session's agenda/todo state rather
than truly restoring it, so reducing how often the key is actually lost is
still worth doing on its own, independent of whether/when the larger
hook-replacement work lands.

`workflow_manual`'s existing `injectSessionKeyLine`
(`agents-plugin-tool/internal/mcp/workflow_manual.go:137-144`) places a
"preserve verbatim" reminder once, near the top of a manual reload; that
single placement loses attention salience as the transcript grows past it.
Repeating the key on a high-frequency, lead-scoped tool response keeps it
recent in the transcript at the point compaction is likely to trigger,
improving the odds a compaction summary actually carries it forward.

## Phases

### Phase 1: Session-key tip on `git.commit` responses

- Add a small shared helper (e.g. `appendSessionKeyTip(text, sessionKey
  string) string`) in `agents-plugin-tool/internal/mcp`, mirroring
  `injectSessionKeyLine`'s phrasing, that appends a trailer line — e.g. `tip:
  preserve this session key: <key> during compaction` — to a tool response.
- Wire it into the `git.commit` handler only (`server.go`, the `"git.commit"`
  case around line 895-919, where `commitKey` is already extracted in scope
  for the existing TODO-summary trailer). `git.commit` is a high-frequency,
  lead-scoped call that tends to land near the natural end of a working
  turn, keeping the key recent in the transcript at the point compaction is
  likely to trigger.
- Do not wire this into other lead-scoped tools (`tickets.move`,
  `agenda.set`, etc.) in this phase; there is no existing shared
  post-processing hook across tool formatters (each has its own `format*`
  function), so broader adoption is a separate, explicitly deferred
  follow-up rather than an implicit expansion of this phase's scope.
- Verify: unit-test `appendSessionKeyTip`'s own behavior directly (tip
  appended for a non-empty key, no-op for an empty key), then a test
  asserting `git.commit`'s returned text contains the tip line with the
  correct session key. Note `session_key` is always present by the time
  `git.commit` produces commit text — `resolveToolRoot` already makes it
  mandatory earlier in the handler — so there is no reachable no-key path
  through `git.commit` itself to test against.
- Append the tip trailer after the existing TODO-summary trailer (key line
  last), so it lands closest to the end of the response for salience.
