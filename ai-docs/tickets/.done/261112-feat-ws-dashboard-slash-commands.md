---
title: "Dashboard composer runs the harness's slash commands"
parent: 261107-epic-ws-dashboard-messenger-round2
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 727e4d4022ad3974
sage-review-completeness-reviewed: 727e4d4022ad3974
completed: 2026-11-12
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
  command list). Where the host's listing tells user-only
  commands from agent-only ones (for example skills a user cannot invoke or
  the model cannot invoke), the picker groups them by that layer; where it
  cannot tell, one list (user note on (435)). Rejected: a fixed allowlist.
- (441) Amends (435): no listing marks a command as a terminal-only panel
  (`$.command.list()` has no panel or type field, hosted
  `init.slash_commands` holds names only, Pi `get_commands` omits
  built-ins), so the picker marks nothing in advance; the "opens in the
  terminal" note appears only after a run returns no text, the (433) path.
  Rejected: a hard-coded table of known terminal Claude panel built-ins
  marking them in the picker (it drifts with every Claude Code release,
  close to the allowlist (435) rejected).
- (436) A command sent while a turn runs waits in its own entry until the
  turn ends, then runs, as when typed in the terminal. Rejected: refusing
  commands while a turn runs.
- Compatibility tier T0 (fact population, auto-confirmed): the command
  field on the inbox entry and the send route body is optional and additive,
  and the new feed event kind is announced through intake's `accepts` list,
  so an older reader never sees it. The dashboard offers the picker and
  sends a command entry only to a session whose adapter reports a command
  list, so an older adapter never drains a command entry it would read as
  empty text. Terminal Claude's claude-mod reports `$.command.list()` to
  the daemon over intake (a new event kind, or an optional field on an
  existing one; the implementer picks), and only while intake's `accepts`
  line names the new kinds: `accepts` is ws-mcp telling the adapter what it
  takes, so an adapter that does not see them stays silent and gets no
  picker. The optional `command` field on `dashboard.inbox_drain` output is
  part of the same T0 change (review, auto-confirmed).
  Rejected: T1 with a version bump (nothing is renamed or removed).
- Hosted Pi's command list comes from its RPC `get_commands`
  (`RpcSlashCommand {name, description?, source}`), which `wsagent/pi.go`
  does not call yet; `/compact` keeps its existing RPC mapping. Pi's
  `source` (`extension`, `prompt`, `skill`) names a command's origin, not
  whether only the user or only the agent may invoke it, so Pi commands
  show as one list under (435), each with its source as a muted label
  (fact population, auto-confirmed). Rejected: grouping Pi commands by
  `source` as if it were the user/agent layer.
- Drain order is preserved and a command splits the batch (review,
  auto-confirmed): for drained text A, command C, text B, the adapter
  submits A, runs C once the session is idle (terminal Claude's
  `$.command.run` queues until idle), then sends B, holding B adapter-side
  although the inbox already shows it drained. This applies to every drain
  path that joins entries today: claude-mod's `board.drain` submit, the
  hosted runner's join with mail, and `dashboard.inbox_drain`.
- Inbox handling of a command entry (review, auto-confirmed): `MergeTarget`
  skips command entries and a command send never merges into a text entry;
  `Steer` refuses a command entry as it refuses an Interrupt entry, and the
  page hides the steer action on command bubbles. A held command entry can
  be cancelled like any held entry; an interrupt returns it to the composer
  as a command selection, not as plain text, so resending keeps it a command
  under (432).
- Hosted Claude's list is `init.slash_commands`; `terminal_slash_commands`
  in the same message stays out of the picker, since a hosted session has no
  terminal behind it. The list arrives with `init`, so a hosted session
  that has not run a turn yet has no picker; accepted (review,
  auto-confirmed).
- Hosted Pi: `get_commands` omits built-in commands, so the adapter adds
  `/compact` (its existing RPC mapping) to the list. A held Pi command goes
  out as RPC `prompt` at the end of the turn, never as `steer` or
  `follow_up`, which refuse extension commands (review, auto-confirmed).
- The hosted (wsagent) part merges only after mail to the peer lead that
  owns wsagent / wsholder / wslive (431); the claude-mod and dashboard parts
  may merge first (review, auto-confirmed).
- (430) Child of `261107-epic-ws-dashboard-messenger-round2`, whose
  non-scope line for slash commands is removed.
- (431) The hosted (wsagent) part is coordinated with the peer lead who owns
  wsagent / wsholder / wslive before merging.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`; intake `accepts` lives in `internal/mcp/live_snapshot.go`)

## Prior Decisions

- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-10, Decisions 9): "Slash commands pass through as text (technical, (177)). No shim; a `/compact` typed for a Pi session maps to the RPC `compact` command. Phase 1 has no command palette; `init.slash_commands` stays available for a later one." — bearing: supports
- 72c9d851 (2026-11-11, commit, 261110-feat-ws-dashboard-headless-lead-sessions Phase 1 spine): "An idle after a result is that turn's own and is ignored (mapper `open`), so it cannot close the next turn a flushed send already began." — bearing: constrains
- 261107-epic-ws-dashboard-messenger-round2 (2026-11-07, Cross-Child Decisions): "No host adapter change in this round, except steer promotion: everything reaches the session through the existing inbox drain as text." (8d484d01 records slash commands as a fourth exception) — bearing: constrains
- 00860d04 (2026-11-12, commit, 261112-bug-ws-claude-mod-dashboard-message-during-compaction-submits-as-follow-up): "User decision (board item 378 = a): hold exactly as during a turn, submit once the compaction settles, after any compaction continuation." — bearing: constrains
- d34ef5c2 (2026-11-07, commit): "Steer copies the whole entry before setting the flag, so fields added later ... carry over without a code change." and "MergeTarget returns the target record, not only its name" — bearing: constrains
- 57ba56f1 (2026-11-09, commit, 261109-bug-ws-whiteboard-patch-cannot-change-item-class): "Chip.SentAt is overwritten in Send (client-supplied values ignored) and stripped from the cancel route's page copy, so the browser wire shape is unchanged." — bearing: constrains
- c779de58 (2026-11-08, commit): "The item body editor and new-item input never had a mention picker, so per plan item 5 the popup is composer-only." — bearing: supports
- 8d484d01 (2026-11-12, commit): "User 2026-11-12: re-add harness slash commands to the dashboard; board (430)-(433) ok, (434)=a, (435)=a with note (group user-only vs agent-only commands where the listing tells), (436)=a." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, watcher.ts, dashboard.ts; agents-plugin-tool/internal/wsagent/claude.go, pi.go, runner.go; wsinbox/inbox.go; wsdashboard/inbox_routes.go, assets/mention.js, assets/dashboard.js; wsflow hooks mirror; intake accepts list in internal/mcp/live_snapshot.go and wslive/state.go |
| scope.surface | cross-module | inbox entry shape, dashboard send route, intake accepts list and live feed event kind, claude-mod and wsagent adapters, composer assets |
| scope.new_public_symbol | yes | a command field on wsinbox.Entry and the send route body, a new live feed event kind announced on intake accepts; names not yet chosen |
| scope.new_type_contract | yes | {command, args} inbox entry and route payload; session command list captured from init.slash_commands, $.command.list and Pi get_commands |
| scope.test_surface | existing | claude-mod dashboard-engine.test.ts and watcher.test.ts, wsagent lead_claude_test.go with testdata/claude-lead-local-commands.jsonl, wsinbox and wsdashboard inbox_routes_test.go, node tests for assets |
| complexity.reuse_points | confirmed | $.command.run already used at register.ts#L1058 and #L1125; mention.js attach popup; piCompactCommand in pi.go#L431-L441; intake accepts parsing in dashboard.ts#L586-L590 |
| complexity.side_effect_risk | high | changes inbox merge, join, steer and drain paths shared by every dashboard message on three hosts |
| risk.correctness | high | a command entry must stay unmerged, unjoined, unprefixed and unsteered across inbox Send, MergeTarget, Entry.Text, runner drain join and claude-mod submit, and must wait for turn end including compaction holds |
| risk.fit | moderate | decisions 430-436 settle behavior, but the hosted part needs coordination with the wsagent peer lead per 431; the Pi command list comes from RPC get_commands, not yet called |
| risk.test | moderate | verification spans Go, claude-mod engine and node tests plus a headless picker check; real-host $.command.run result text is assumed from the types file |
| risk.security_or_contract | moderate | new inbox entry field, route payload and live event kind are cross-version surfaces, recorded T0 in Decisions (picker offered only to adapters that report commands) |

## Phases

### Phase 1: Slash commands from the composer

Implement per `## Decisions`. Verification:

- Inbox tests: a command entry is never merged, joined, prefixed or steered;
  `MergeTarget` skips it; a held command entry cancels; an interrupt returns
  it as a command; `dashboard.inbox_drain` carries the `command` field.
- claude-mod engine tests: a drained command calls `$.command.run`; a held
  one waits for turn end, including a compaction hold; text before and
  after a command in one drain keeps its order around it; the command list
  and the result text reach the daemon under the new kinds only when intake
  accepts them; a run with no `text` shows the "opens in the terminal" note.
- wsagent tests: hosted Claude sends the command bare as its own message;
  `init.slash_commands` is captured from a fixture with a filled list, and
  `terminal_slash_commands` is left out; hosted Pi maps `/compact`, captures
  `get_commands` with each command's source label, and sends a held command
  as `prompt` at turn end.
- Node tests for the `/` picker: grouping by user/agent layer where the
  listing tells, one list where it cannot; no picker for a session whose
  adapter reports no list; no steer action on a command bubble.
- A headless check of the picker and a command's output row.

### Result (49e0da567) - 2026-11-12

Landed in five commits on impl/develop/rebel-zen-sprig:

- Spine (34c00c561): `wsinbox.Entry.Command {name, args}` is a command entry
  whose `Text()` is its typed line. A command send is always a new entry.
  `MergeTarget` stops at a command entry: it does not skip past it, so text
  sent after a command never joins text sent before it. `Steer` refuses a
  command with `ErrCommand` (409). The message route takes
  `{command, args}` only when the session snapshot lists commands (else
  409). Cancel and interrupt return `command`. `dashboard.inbox_drain`
  carries `command` in order (text output heads the block `command`).
  `wslive` adds the optional kinds `commands` (the snapshot's `commands`
  list, cleared by reset) and `command` (a `command` ring line with the
  typed line and its output), both announced on intake `accepts`.
- claude-mod (c4e68fc3e): reports `$.command.list()` as `commands` at feed
  start, after every reset, when accepted, and at each turn end when the
  list changed. The drain split submits A, runs C through `$.command.run`
  (dispatched via `$.clock.after(0)`), and holds B (and anything drained
  while C runs) in a watcher backlog. The backlog obeys the existing
  turn-end and compaction holds. A `command` event carries the output text,
  or no text when the run returned none; a refused run sends its error as
  text. The wsflow mirror is resynced.
- wsagent (f460afb7b): hosted Claude captures `init.slash_commands` and
  leaves out `terminal_slash_commands`. Hosted Pi calls `get_commands` at
  lead start and at each settle (mapped in the event stream, so a holder
  replay rebuilds it), labels each command with its source, and adds
  `compact`. `kick` splits the drain: text plus mail goes as one turn, a
  command goes alone and bare (Claude) or as RPC `compact`/`prompt` (Pi),
  and mail never rides with a command. The not-yet-sent remainder is kept
  in the optional `Record.Queued` field, so it survives a daemon restart.
  An interrupt keeps it; close and hand-off drop it.
- Page (625708d53): the `/` picker reuses the generalized `mention.js`
  popup. Rows group by user/agent layer when any command has one, else one
  list. There is no picker without a list. Picking makes a command capsule
  and the box text becomes the args. While a command is selected, chips,
  notes and mentions stay in the composer and images block the send with a
  toast. The held command bubble has no steer control. The `command` ring
  row shows the echo and output, or the "opens in the terminal" note. i18n
  covers all 11 languages.
- Review fix (49e0da567): a steered entry keeps its stamp, so text steered
  but not taken mid-turn still drains ahead of a later command.

Verification:
- `ws-rgo -- test ./internal/wsinbox ./internal/wslive ./internal/wsagent
  ./internal/wsdashboard ./internal/wsmailbox ./internal/wsrsrc
  ./internal/mcp`: all ok except the known pre-existing
  `TestDashboardReadPageServesTheBoard`.
- `claude plugin test .` in agents-plugin and agents-plugin-wsflow: 773
  pass each.
- `claude plugin validate agents-plugin`: passed.
- A headless chrome check against an isolated probe daemon covered the
  grouped and flat picker, filtering, the capsule, the held bubble without
  steer, cancel restoring the selection, the command output row and the
  terminal note, and no picker without a list.
- Lite review: one Important finding, fixed in 49e0da567.

Decisions taken:
- `MergeTarget` stops at a command rather than skipping it, to keep drain
  order.
- Steer of a command gets its own error, not the misleading "already
  received".
- Hosted queued input is persisted, not held in memory.
- Pi `get_commands` is handled in the mapper rather than through
  `requestBody`, so it can be replayed.
- Only lead sessions report a command list.

Open:
- A terminal Claude command whose `$.command.run` never settles (a panel
  left open) holds later board messages until it does. No timeout was
  added.
- The agent HTTP `interrupt` action returns a held command as its typed
  line (`[]string`), so resending it through `send` makes it text. The page
  route keeps it a command.
- `startConv`'s queued re-send for a non-rejoining restart has no test.
- Per (431), the hosted part merges only after mail to the peer lead that
  owns wsagent / wsholder / wslive.
