---
title: Wrap every whiteboard-sent user message in a timestamped ws:user envelope
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: prerequisite; consume its landed move map before touching dashboard assets
  261115-feat-ws-dashboard-conversation-rendering-consolidation: coordinate; both change the user-bubble renderer, whichever lands second carries the envelope split into the other's renderer
  261115-feat-ws-whiteboard-topic-close-request: sibling; its close requests are delivered inside this envelope
---

# Wrap every whiteboard-sent user message in a timestamped ws:user envelope

## Background

Owner request (relayed by MAIN, 2026-11-15): every message the whiteboard
(dashboard composer) sends to the agent is wrapped as
`<ws:user at="YYYY-MM-DDThh:mm+09:00">...</ws:user>`, one wrapper per queued
item even when several items are delivered to the agent in one bundle; the
dashboard conversation view parses each wrapper into its own block; the
timestamp is timezone-aware. No steer or follow-up marker.

Today (develop `883762d19`, paths under `agents-plugin-tool/internal/`):
- The composer posts one structured request per send (`text`, `chips`,
  `topics`, `mentions`, picks, consoles, images) to `/s/<reply-id>/message`
  with no browser time or offset (`wsdashboard/assets/dashboard.js`
  `sendMessage()`, ~2518-2536). The daemon stamps `SentAt` with its own
  `time.Now()` (`wsdashboard/inbox_routes.go` ~222, `wsinbox/inbox.go` ~692)
  and stores epoch milliseconds, so the sender's offset is not recoverable.
- Each send is one inbox entry; sends never merge in the browser. Bundling
  happens at delivery: Claude (`agents-plugin/hooks/claude-mod/dashboard.ts`
  ~171-193, `register.ts` ~1067-1115) and Pi
  (`agents-plugin-pi/src/mailbox-waiter.ts` ~1224, `adapter-label.ts` ~49-68)
  drain JSON, join the entries' `.text` with blank lines under one leading
  `[user message from ... whiteboard]` label; the daemon-hosted runner joins
  drained text the same way (`wsagent/runner.go` ~1831-1933).
- The one text assembly point every delivery path shares is
  `wsinbox.Entry.Text()` (`wsinbox/inbox.go` ~325-367): answers block,
  mentions, picks, consoles, text paragraphs, image lines; a slash command
  returns its bare command line. `dashboard.inbox_drain` returns it as
  `text` (`mcp/dashboard_inbox.go` ~115-140).
- The conversation view splits a user turn with leading-anchored parsers
  (`wsinbox/answers.go` `ParseAnswerBlock` ~165-191, `wsinbox/mentions.go`
  ~66-93, `wsinbox/webpicks.go` ~74-109) in
  `wsdashboard/pageview_inbox.go` `splitUserEntries()` (~84-153) and renders
  one bubble with one clock (`wsdashboard/assets/dashboard.tmpl` `entry`,
  ~296). An outer wrapper would defeat every anchored parser unless the view
  unwraps first.
- The shipped whiteboard playbook says a message "opens with `<wb:answers>`"
  (`agents-plugin/rsrc/whiteboard/whiteboard.md` ~389, ~402-438; mirrored in
  `agents-plugin-wsflow/` and `agents-plugin-pi/`).

## Decisions

- **Owner request, as relayed:** one `<ws:user at="...">` wrapper per
  whiteboard-sent item, also inside bundles; the conversation view shows each
  as its own block; timezone-aware; no steer or follow-up marker.
- **MAIN technical calls (announced, accepted):** `at` is ISO 8601 at minute
  precision with the UTC offset (`2026-11-15T11:42+09:00`); the offset is the
  sending browser's, not a fixed `+09:00`; one wrapper per item; the scope is
  answers, freeform text and attachments, with `<wb:answers>` nested inside
  unchanged; the conversation view parses the blocks; no steer marker.

## Open Decision Queue
Last ID: none minted (items below await MAIN's board IDs)

(u1) [open] [technical] The envelope is built once in `wsinbox.Entry.Text()`, so Claude, Pi, the hosted runner and the drain all deliver it, over per-adapter wrapping - one shared formatter cannot double-wrap and keeps every delivery path identical.
(u2) [open] [technical] The adapters' leading `[user message from ... whiteboard]` label stays outside and before the envelopes, one per delivered batch, over a label inside each envelope - provenance detection in Claude and Pi matches that exact leading label (`dashboard.ts` ~135-149, `adapter-label.ts` ~49-59).
(u3) [open] [technical] A slash-command entry stays a bare command line, unwrapped, over wrapping it - the command must reach the host as its own line to run; steering/interrupt entries are wrapped like any other item (no marker, per the owner).
(u4) [open] [technical] The instant comes from the daemon's existing `SentAt` and only the offset comes from the browser (new optional request field, minutes east of UTC, from `-new Date().getTimezoneOffset()`, stored as an optional inbox-entry field) over a browser-supplied wall-clock time - the daemon clock stays the single ordering clock, and a skewed browser clock cannot reorder or backdate messages.
(u5) [open] [technical] An entry without a stored offset (written before this change, or a client that omits it) renders `at` in the daemon host's local zone, over UTC or omitting `at` - every delivered item keeps one well-formed attribute, and the host zone is the best available guess for a local user.
(u6) [open] [technical] A literal `</ws:user` inside user text is escaped as `&lt;/ws:user` in the envelope body and unescaped by the conversation view, over leaving it raw or escaping every `<` - only that sequence can break the envelope, and the rest of the body stays byte-identical to today's text.
(u7) [open] [technical] The conversation view unwraps envelopes first, splits one user turn into one block per envelope, then runs the existing answers/mentions/picks/images parsers per block; text with no envelope (history, older daemons) is one block as today, and a malformed or unterminated envelope falls back to showing the turn as plain text, over a strict parser that drops it - history keeps rendering and nothing user-sent disappears.
(u8) [open] [technical] The shipped whiteboard playbook (ws, wsflow, Pi copies, per `wsflow-mirroring.md`) says each whiteboard message is a `<ws:user at=...>` block whose content opens with `<wb:answers>` when present, and that older messages may be unwrapped, over only editing the opening sentence - the tolerant wording keeps a newer playbook correct against an older daemon.
(u9) [open] [technical] Compatibility tier T0: the new request and inbox fields are optional and additive, the drain `text` field keeps its shape, dashboard assets ship with the daemon that serves them, and (u5)/(u7)/(u8) cover the mixed-version text skew, over treating the delivered-text change as T2 - no MCP argument, store schema or event type changes non-additively. The worker audits that inbox rewrites (steering marshal, `wsinbox/inbox.go` ~626-744) preserve the new field.
(u10) [open] [announced] Dashboard asset edits wait for the asset-split move map (`261114-refactor-ws-dashboard-whiteboard-asset-split`, recorded as prerequisite) - that ticket asks every consumer to use its landed move map; the conversation-rendering ticket is a coordinate edge, not a blocker, since it already waits on the same split.
(889) [open] [policy] How each block shows its time in the conversation view.
- Owner item minted by MAIN, 2026-11-15; not pre-settled here.
- a: today shows time only, other days add the date, hover shows the full timestamp with the offset.
- b: always the full date and time.
- c: relative ("5 min ago"), absolute on hover.
> MAIN's recommendation: a.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (playbook and
  dashboard assets ship downstream), `ai-docs/manuals/skill-authoring.md`
  and `ai-docs/manuals/wsflow-mirroring.md` (playbook edit and mirrors),
  `ai-docs/manuals/release-compatibility.md` (inbox entry and request fields).
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417 (isolated daemons with their own
  `WS_CACHE_HOME` on free ports); end with an owned process scan.

## Phases

### Phase 1: Envelope, offset capture, block parsing and playbook

Pending the queue above. Expected scope: browser offset field in
`sendMessage()`; optional inbox field; envelope in `Entry.Text()`;
envelope-first split in `pageview_inbox.go` and the `entry` template;
playbook wording in the three copies; i18n only if the time display adds
strings.

Verification (to be finalized at promotion):
- `wsinbox` unit tests: one envelope per entry with the browser offset,
  minute precision; fallback zone without an offset; command entries bare;
  `</ws:user` escaping round-trip; steering rewrite preserves the offset.
- Drain tests: a bundle of three entries yields three envelopes in order.
- Conversation view tests: a bundled turn renders three blocks, each with
  its answer capsules intact; an unwrapped legacy turn renders as today; a
  malformed envelope renders as plain text.
- Playbook mirror tests pass for the ws, wsflow and Pi copies.
