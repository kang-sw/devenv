---
title: Wrap every whiteboard-sent user message in a timestamped ws:user envelope
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: prerequisite; consume its landed move map before touching dashboard assets
  261115-feat-ws-dashboard-conversation-rendering-consolidation: coordinate; both change the user-bubble renderer, whichever lands second carries the envelope split into the other's renderer
  261115-feat-ws-whiteboard-topic-close-request: sibling; its close requests are delivered inside this envelope
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 462fc0e0aed0b002
sage-review-completeness-reviewed: 462fc0e0aed0b002
completed: 2026-11-15
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
  drained inbox text with reports and mail using blank lines, without that
  adapter label (`agents-plugin-tool/internal/wsagent/runner.go#L1837-L1844`,
  `agents-plugin-tool/internal/wsagent/runner.go#L1880-L1884`,
  `agents-plugin-tool/internal/wsagent/runner.go#L1918-L1933`).
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
- The shipped whiteboard playbook says a dashboard message "opens with the user's board answers as a"
  "`<wb:answers>` block" (`agents-plugin/rsrc/whiteboard/whiteboard.md#L402-L404`;
  byte-identical in `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` and
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`).

## Decisions

- **Owner request, as relayed:** one `<ws:user at="...">` wrapper per
  whiteboard-sent item, also inside bundles; the conversation view shows each
  as its own block; timezone-aware; no steer or follow-up marker.
- **MAIN technical calls (announced, accepted):** `at` is ISO 8601 at minute
  precision with the UTC offset (`2026-11-15T11:42+09:00`); the offset is the
  sending browser's, not a fixed `+09:00`; one wrapper per item; the scope is
  answers, freeform text and attachments, with `<wb:answers>` nested inside
  unchanged; the conversation view parses the blocks; no steer marker.
- **(889)=c, owner's words:** "60분까지는 n분 전으로, 그 외에는 a로 가죠."
  (Up to 60 minutes, "n minutes ago"; beyond that, format a.) Each block's
  time shows a relative "n분 전" (localized) while it is under 60 minutes
  old, refreshed live each minute; from 60 minutes on it shows format a:
  time only for today ("11:29"), date and time on other days
  ("11/14 23:50"). Hover always shows the full date, time and offset (MAIN
  reading of c's hover, announced). Rejected: a alone, b (always full date
  and time), c with no cut-over.
- **Envelope built once in `wsinbox.Entry.Text()`** (technical, acked by
  MAIN): Claude, Pi, the hosted runner and the MCP drain all deliver the
  same envelope. Rejected: per-adapter wrapping (double-wrap risk, divergent
  paths).
- **The adapter label stays outside the envelopes** (technical, acked): the
  leading `[user message from ... whiteboard]` label precedes the envelopes,
  one per delivered batch, because Claude and Pi provenance detection matches
  that exact leading label (commit `2fb9b65f`). Rejected: a label inside each
  envelope.
- **Commands bare, steering wrapped, empty entries absent** (technical,
  acked; the empty-entry clause auto-confirmed from fact population): a
  slash-command entry stays its bare typed line (commit `34c00c56`, hosts run
  it as is); a steered message is wrapped like any other item, with no marker;
  an entry whose text is empty (a textless interrupt control record) yields
  no envelope. Rejected: wrapping commands.
- **Daemon instant, browser offset** (technical, acked): the instant is the
  daemon's existing `SentAt`; the browser adds a new optional request field,
  the offset in minutes east of UTC from `-new Date().getTimezoneOffset()`,
  validated to the real-world range (-720..+840) and stored as an optional
  inbox-entry field. Rejected: a browser wall-clock time (skew could reorder
  or backdate messages).
- **Fallback zone** (technical, acked): an entry without a stored offset
  renders `at` in the daemon host's local zone. The hover's full value shows
  the offset actually used, so a legacy entry is visibly in the host zone
  (MAIN note). Rejected: UTC; omitting `at`.
- **Reversible delimiter escaping** (technical, acked; reversibility
  auto-confirmed from fact population): only the closing-delimiter sequence
  is escaped, by nested entity levels. In the envelope body, a literal
  `</ws:user` becomes `&lt;/ws:user`, and any existing `&(amp;)*lt;/ws:user`
  run gains one more `amp;` right after its `&` (`&lt;/ws:user` becomes
  `&amp;lt;/ws:user`). The view's unescape drops exactly one level per match
  (`&lt;/ws:user` back to `</ws:user`, otherwise one `amp;` removed), which
  restores the exact text, including literal entity input. The rest of the
  body is byte-identical to today's text. Corrected 2026-11-15 by the lead
  after the worker's stop (c): the earlier wording prefixed a whole `&amp;`
  to an existing run, so literal `&amp;</ws:user` and `&lt;/ws:user` both
  encoded to `&amp;&lt;/ws:user` and did not round-trip; intent unchanged. Rejected: leaving it raw (forged block
  boundaries); escaping every `<` (changes all delivered text).
- **Envelope-first block split, nothing dropped** (technical, acked;
  mixed-batch clause auto-confirmed from fact population): the conversation
  view unwraps envelopes first, makes one block per envelope, then runs the
  existing answers, mentions, picks and image parsers per block. Text outside
  any envelope (the adapter label is stripped as today; hosted-runner report
  or mail text joined into the same turn; legacy unwrapped history) renders
  as its own plain block in its original order. A malformed or unterminated
  envelope falls back to the whole turn as plain text, as the existing
  parsers already do (commit `562bfeb9`). Rejected: a strict parser that
  drops malformed input.
- **Tolerant playbook wording** (technical, acked): the ws, wsflow and Pi
  copies of the whiteboard playbook say each dashboard message is a
  `<ws:user at=...>` block whose content opens with `<wb:answers>` when
  present, and that older messages may arrive unwrapped. Rejected: editing
  only the opening sentence.
- **Compatibility tier T0** (technical, acked): optional additive request
  and inbox fields, an unchanged drain `text` shape, and dashboard assets that
  ship with their daemon. `Normalize` and every inbox rewrite (steer, held
  merge, cancel, drain) copy the new field; the known minor-mismatch caveat
  that an older binary merging into a held entry drops a new optional field
  is accepted as for mentions (commit `e0964b73`). Rejected: T2.
- **Asset edits wait for the asset-split move map** (announced, acked):
  `261114-refactor-ws-dashboard-whiteboard-asset-split` is a prerequisite;
  the conversation-rendering ticket is a coordinate edge. Implementation
  dispatch waits for a MAIN signal.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417 (isolated daemons with their own
  `WS_CACHE_HOME` on free ports); end with an owned process scan.

## Prior Decisions

- 261115-feat-ws-whiteboard-topic-close-request (2026-11-15, Decisions): "delivered as `<wb:close-request topic="<stem>">reason</wb:close-request>` inside the `<ws:user>` envelope" — bearing: constrains
- fdc0392d (2026-11-15, commit): "Saved-history mode has entries and board regions, no composer/live inbox/activity regions, and cannot overwrite saved composer drafts." — bearing: constrains
- 66d13443 (2026-11-14, commit): "One ParseWebElements (picks then consoles) rather than two parsers: the grammar fixes the order and a caller always wants both plus the rest." — bearing: constrains
- ed6510b9 (2026-11-14, commit): "Capsules reuse achip/mchip; only the selector clipping and a warn-colored edge for console blocks are new CSS." — bearing: supports
- 562bfeb9 (2026-11-12, commit): "Any malformed line, a missing close, or text glued to the close falls back to the whole text as before." — bearing: constrains
- e0964b73 (2026-11-12, commit): "Caveat: during a minor mismatch an older binary that merges into a held entry drops the new optional field." — bearing: constrains
- 34c00c56 (2026-11-12, commit): "Entry.Text() of a command entry is its typed line, so a host that runs a bare \"/name args\" message (hosted Claude) takes it as is." — bearing: constrains
- 2fb9b65f (2026-11-06, commit): "Origin: user message text opening with WHITEBOARD_MESSAGE_LABEL (\"[user message from the ws whiteboard]\") is tagged whiteboard with the label stripped" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsinbox/inbox.go; wsdashboard/inbox_routes.go, pageview_inbox.go and assets/dashboard.js, dashboard.tmpl under the same internal root; agents-plugin/rsrc/whiteboard/whiteboard.md and wsflow/Pi mirrors; wsi18n tables. |
| scope.surface | public-interface | Phase 1 changes model-facing delivered text via Entry.Text() and the dashboard message request; agents-plugin-tool/internal/mcp/dashboard_inbox.go#L93-L119 exposes that text to adapters. |
| scope.new_public_symbol | yes | New model-facing ws:user token and at attribute; no new MCP tool is proposed. |
| scope.new_type_contract | yes | Proposed optional browser offset request/inbox field plus one minute-precision, offset-bearing envelope per item; field spelling is the implementer's; grammar per the Decisions (offset field, reversible escaping, envelope-first split, T0). |
| scope.test_surface | existing | agents-plugin-tool/internal/wsinbox/inbox_test.go, command_test.go, answers_test.go; internal/mcp/dashboard_inbox_test.go; internal/wsdashboard/inbox_routes_test.go and conversation_markdown_test.go; internal/wsi18n/keys_test.go; internal/wsrsrc/pi_mirror_test.go and wsflow_mirror_test.go. No tests run for fact population. |
| complexity.reuse_points | confirmed | Read Entry.Text/Normalize/Send/Steer in agents-plugin-tool/internal/wsinbox/inbox.go#L331-L382 and #L688-L751; leading parsers in pageview_inbox.go#L101-L118; existing fmtTime/tickTimes in assets/dashboard.js#L35-L124 under internal/wsdashboard; mirror guards read. |
| complexity.side_effect_risk | high | One shared serializer feeds Claude, Pi, MCP and hosted delivery; hosted batches may also contain unenveloped reports/mail (agents-plugin-tool/internal/wsagent/runner.go#L1880-L1884). |
| risk.correctness | high | Splitting a single stable conversation entry into blocks must preserve answer/image ownership, legacy text and batch order; current split mutates one ConvEntry in place (agents-plugin-tool/internal/wsdashboard/pageview_inbox.go#L106-L118). |
| risk.fit | moderate | Reuses existing capsule/template/time mechanisms, but the asset-split move map is not landed and conversation-rendering consolidation is ready but unimplemented; consume the split map before asset work. |
| risk.test | high | Existing assertions pin bare delivered text and one user bubble; envelope, escaping, mixed bundles, offset fallback and live 59/60-minute rollover require new cases. Scoped search for ws:user in wsinbox/wsdashboard and adapter-label/mailbox-waiter returned no matches. |
| risk.security_or_contract | high | Untrusted delimiter/entity text must round-trip without forged blocks or lost content; Normalize reconstructs known Entry fields (agents-plugin-tool/internal/wsinbox/inbox.go#L377-L382), so optional-field preservation and T0 compatibility are not established by an additive field alone. |

## Phases

### Phase 1: Envelope, offset capture, block parsing and playbook

Implement the Decisions above. Scope: browser offset field in
`sendMessage()`; optional inbox field; envelope in `Entry.Text()`;
envelope-first split in `pageview_inbox.go` and the `entry` template;
playbook wording in the three copies; per-block time display per (889)
with localized relative-time strings in every `wsi18n` table and a
minute refresh.

Verification (to be finalized at promotion):
- `wsinbox` unit tests: one envelope per entry with the browser offset,
  minute precision; fallback zone without an offset; command entries bare;
  `</ws:user` escaping round-trip; steering rewrite preserves the offset.
- Drain tests: a bundle of three entries yields three envelopes in order.
- Conversation view tests: a bundled turn renders three blocks, each with
  its answer capsules intact; an unwrapped legacy turn renders as today; a
  malformed envelope renders as plain text.
- Time display tests per (889): 59 minutes old shows the relative form,
  60 minutes shows the clock; today shows time only, an older day adds the
  date; hover carries the full timestamp with offset; the relative label
  advances on the minute refresh. `wsi18n` completeness tests pass.
- Playbook mirror tests pass for the ws, wsflow and Pi copies.

### Result (697c95084) - 2026-11-15

- VERIFIED: `Entry.Text()` emits one minute-precision, offset-bearing envelope
  per nonempty item; commands remain bare and interrupt controls remain empty.
  The optional `utcOffsetMinutes` request/inbox field distinguishes missing
  offsets from UTC, validates -720..840, and survives normalize, steer, cancel,
  resend and drain. The daemon still owns the send instant.
- VERIFIED: the corrected nested-entity grammar round-trips literal delimiters,
  existing entity levels, the reported collision pair and 512 mixed-token
  combinations. Other body bytes and the nested capsule grammar are preserved.
- VERIFIED: envelope-first conversation projection keeps one stable ring/stream
  key while rendering each item as an independent existing-style user bubble.
  Answers, mentions, picks, consoles and image numbering stay item-local;
  outside report/mail text remains ordered plain text, legacy content retains
  its parsers, and malformed envelopes remain wholly plain. Nested attachment
  availability participates in cache invalidation.
- VERIFIED: each envelope's clock uses localized minutes below 60, then its
  sender-zone clock (with month/day outside today); hover retains the full
  timestamp and offset. The existing page tick refreshes the minute label.
  All 11 locale tables and the ws/wsflow/Pi guidance are updated; resource
  manifests are regenerated and all three resource trees are byte-identical.
- Decisions: use `UserBlocks` under the existing entry key rather than expand
  the ring slice, preserving stream/history identity and avoiding edits to
  `pages.go` or mail/wake presentation. Edit the landed owning asset fragments;
  keep the two-queue inbox and both TypeScript adapters unchanged. T0 follows
  the accepted optional-field policy and existing minor-mismatch caveat: no
  route, MCP argument, drain JSON member or store schema version was added.
- Verification: full `wsinbox`, `wsi18n` and `wsrsrc` passed; 15 dashboard inbox
  MCP tests passed, including three-item ordered delivery. The named dashboard
  selection passed 96/99 initially; its stale composer request expectation
  was corrected for the optional field and passed on its scoped rerun. The
  two remaining, untouched tests
  `TestWhiteboardEffectsRoutesReadSavedHistoryWithoutAnAdapter` and
  `TestWhiteboardEffectsOlderThanLiveCapRemainReachableOffline` also fail on
  archived base `ce22992e6` at the same assertions; they are baseline findings,
  not fixed here. All new envelope/offset/batch/cache/time tests and the CSS
  collision test passed. Wsflow package checks passed (24 tests, two skips),
  resource-tree comparisons and diff whitespace checks passed.
- Omitted by the caller: child agents and independent review (one later lite
  review belongs to the lead), browsers/screenshots, daemon/runtime actions,
  the full dashboard suite, and ticket closure. The ticket stays in `ready/`.
