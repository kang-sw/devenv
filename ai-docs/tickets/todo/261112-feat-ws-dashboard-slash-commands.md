---
title: "Dashboard composer runs the harness's slash commands"
parent: 261107-epic-ws-dashboard-messenger-round2
---

# Dashboard composer runs the harness's slash commands

## Background

The messenger-round2 epic first listed slash commands as non-scope; the user
brought them back on 2026-11-12. Today, by host (read-only investigation
2026-11-12):

- Terminal Claude (claude-mod): the watcher drains dashboard messages, joins
  them, and submits them under the whiteboard label line
  (`agents-plugin/hooks/claude-mod/register.ts` ~926-962,
  `watcher.ts` ~536-555, `dashboard.ts` `whiteboardLabel` ~120), so `/compact`
  is the second line of a user message and never runs. The mod API offers
  `$.command.list()` (name, description, source builtin/plugin/user/mcp),
  `$.command.run({command, args})` (as if typed; queued until idle; result
  `{text?}`), and a `command.run` hook; the mod already calls
  `$.command.run` for `compact` and `mcp reconnect` (register.ts ~1058,
  ~1125). API reference: the mods types file
  (`~/.claude/plugins/marketplaces/claude-code-plugins/mods/types/claude-code.d.ts`).
- Hosted Claude (wsagent, `claude -p`): text goes in bare
  (`wsagent/claude.go` ~341), so a lone `/compact` already runs (Decision 9 of
  `261110-feat-ws-dashboard-headless-lead-sessions`; fixture
  `testdata/claude-lead-local-commands.jsonl`), but it breaks when the inbox
  merges it into a held entry (`wsinbox/inbox.go` ~649,
  `wsdashboard/inbox_routes.go` ~316), when the drain joins entries
  (`wsagent/runner.go` ~1396), when chip/mention blocks are prepended
  (`inbox.go` ~276), or when it is steered. `init.slash_commands` arrives on
  every turn but is not captured (`claude.go` `claudeInit` ~229).
- Hosted Pi: a lead's `/compact [instructions]` maps to RPC `compact`
  (`wsagent/pi.go` ~418-440); other text goes as an RPC prompt.
- Terminal Pi: labelled steer input with no slash expansion; out of scope.
- Composer: no special handling of `/`; the `@` file-mention picker
  (`wsdashboard/assets/mention.js`, wired in `dashboard.js` ~1394-1471) is a
  reusable popup whose trigger and item shape are hard-coded.

## Decisions

- (434) Hosts in scope: terminal Claude, hosted Claude, hosted Pi.
  Rejected: adding terminal Pi now (it needs its own execution path);
  terminal Claude only.
- (432) The composer sends a command as an explicit field (`{command,
  args}`), not as text the server parses; the inbox keeps it as its own
  entry that is never merged with, joined to, or prefixed by other entries,
  never steered, and never carries the whiteboard label. Each adapter runs
  it natively: terminal Claude through `$.command.run`, hosted Claude as a
  bare single user message, hosted Pi through its RPC mapping (`compact`) or
  as a bare prompt. Rejected: detecting a leading `/` in message text, which
  breaks as soon as merging, chips, or mention blocks touch the text.
- (433) Output: terminal Claude pushes a new feed event for the command echo
  and its result text, announced through intake's `accepts` list like the
  `compact` kind; hosted sessions keep showing the output as today (a ring
  reply). A command with no text output (a terminal-only panel) shows a
  short "opens in the terminal" note. Rejected: no output in the dashboard.
- (435) The picker lists every command the session reports (terminal Claude
  `$.command.list()`, hosted Claude `init.slash_commands`, hosted Pi its
  command list), with terminal-only panel commands shown with the
  "opens in the terminal" note. Where the host's listing tells user-only
  commands from agent-only ones (for example skills a user cannot invoke or
  the model cannot invoke), the picker groups them by that layer; where it
  cannot tell, one list (user note on (435)). Rejected: a fixed allowlist.
- (436) A command sent while a turn runs waits in its own entry until the
  turn ends, then runs, as when typed in the terminal. Rejected: refusing
  commands while a turn runs.
- (430) Child of `261107-epic-ws-dashboard-messenger-round2`, whose
  non-scope line for slash commands is removed.
- (431) The hosted (wsagent) part is coordinated with the peer lead who owns
  wsagent / wsholder / wslive before merging.

## Phases

### Phase 1: Slash commands from the composer

Implement per `## Decisions`. Verification: inbox tests (a command entry is
never merged, joined, prefixed or steered); claude-mod engine tests (a drained
command calls `$.command.run`, a held one waits for turn end, the result text
reaches the feed under the new kind only when intake accepts it); wsagent tests
(hosted Claude sends the command bare as its own message; hosted Pi maps
`/compact`; `init.slash_commands` captured into the session's command list);
node tests for the `/` picker; a headless check of the picker and a command's
output row.
