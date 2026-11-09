---
title: "whiteboard: a re-asked item keeps its old class and its sent mark, so it looks settled"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b4ab1a9a51a5423c
sage-review-completeness-reviewed: b4ab1a9a51a5423c
---

# whiteboard: a re-asked item keeps its old class and its sent mark, so it looks settled

## Background

Dogfood surprise (2026-11-09). The lead-ticket playbook says an announced or
technical item that the user objects to, or that a later finding undercuts,
"becomes a policy question under its existing ID". On the whiteboard the lead
reopened items (112) and (113) with `wb:patch status="open"` plus options,
a recommendation and a new rationale, intending them as policy questions.

What happened:

- `wb:patch` has no `class` attribute, so the store kept `[technical]`.
  Nothing warned that options on a technical item make no sense.
- The user had already answered `ok` from the dashboard. The dashboard's
  `applySent()` (`internal/wsdashboard/assets/dashboard.js` ~:1066) marks
  every board item that the inbox records hold a sent answer for
  (`.sentdata i[data-id]`) as `sent-sent`. It dims the answer area and adds a
  "sent · ok" mark. Nothing ties that record to the item's later reopen, so
  the reopened item still looked answered. `pill.js`
  (`internal/wswhiteboard/assets/pill.js#L29-L34`) also counts a
  `sent-sent` item as no longer open. The user saw the three items as
  "already resolved technical" ("지금 제 화면에는 112 113 114가 이미 resolve된
  technical로 보입니다").

The lead worked around it by superseding (112)-(114) and adding (116)-(118).

## Decisions

- **Reclass on the board** (121 a, user 2026-11-09): `wb:patch` accepts
  `class`, so the playbook's "becomes a policy question under its existing ID"
  can be written on the board. The batch result's `info:` line also warns when
  options are put on a technical or announced item.
  Auto-confirmed (reviewers, 2026-11-09): the warning is judged against the
  item's class after the command, so it covers `wb:add` of a technical or
  announced item with options, a patch adding options to one, and a patch
  that reclasses an item holding options away from policy. Such a reclass
  keeps the options; the warning applies.
  Rejected: an `info:` warning alone (the lead still cannot reclass, so it
  ends in the supersede workaround); documenting the supersede-and-re-add
  workaround in the playbook.
- **Sent mark follows the re-ask** (121 a note, user 2026-11-09: "화면의
  sent도 엮어서 같이 우아하게 처리되게 해야겠네요"): a re-asked item is
  shown as waiting on the user again, not as sent. The open-question pill
  and the tab-title count that `pill.js` sets include it.
- **Sent mark ignores answers sent before the latest re-ask** (122, amended by
  127 a, user 2026-11-09): a re-ask is a `wb:patch` that changes the item's
  `class` or its options, or that sets a settled item (answered, assumed,
  withdrawn, superseded) back to `status="open"`. A patch that edits only the
  title, rationale, recommendation or links is not a re-ask, and neither is
  a patch that resends the item's current class or identical options (a real
  difference only). The store
  records the time of the item's latest re-ask; a later re-ask replaces it.
  The daemon's `sentMarks` (`internal/wsdashboard/pageview_inbox.go`) drops a
  chip for that item, drained or held, sent before that time, so no stale
  mark reaches `.sentdata` and `applySent()` needs no change for it. A chip
  sent at the same millisecond or later keeps its mark; a chip with no send
  time (records written before this change) keeps today's behavior. Both
  times are wall-clock times written on the same machine (the daemon writes
  the send time, the MCP store writes the re-ask time).
  Auto-confirmed (design review, 2026-11-09): the compared time is the
  chip's own send time, not the record's `Entry.SentAt`. `wsinbox.Send`
  merges a new dashboard send into the newest held record, and `Merge` keeps
  the first send's `SentAt`, so a fresh answer merged after a re-ask into a
  record held from before it would be dropped. Each `Chip` gains a send time
  set in `Send`/`Merge`. Rejected: starting a new held record across a
  re-ask (the inbox would need to know about the board); the record's
  last-write time (one late chip would revive the stale chips beside it).
  The reported case is the reason for the wider trigger: the lead's patch of
  (112)/(113) went through with options, so the items were still open in the
  store, with the user's `ok` sent from the dashboard but not yet recorded
  with `wb:answer`. A settled-to-open trigger alone would leave it unfixed.
  Rejected (127): only a patch carrying `status="open"`, including on an open
  item (the lead must remember to send it); settled-to-open only.
  Auto-confirmed (fact-populator gap, 2026-11-09): which time and where to
  compare. `.sentdata` carries no time today. Filtering server-side, by the
  inbox entry's `SentAt`, was picked over adding a time attribute that
  `applySent()` would compare in JS.
  Auto-confirmed (design review, 2026-11-09): plumbing. `sentMarks` sees only
  inbox records, and the board reaches the daemon pre-rendered in
  `PageContent.Board`, built by `ReadPageLang` in
  `internal/mcp/dashboard_page.go`. The per-item re-ask times travel as a new
  field on `PageContent` filled from the session's whiteboard record. Inbox
  send times are epoch milliseconds, store times are RFC 3339 strings;
  compare after converting.
  Rejected: deleting or rewriting the inbox records. They are the session's
  message history and other code reads them.
- **`class` only on open items** (123, acknowledged 2026-11-09): `wb:patch`
  accepts `class` only on an item that is open, or that the same patch sets
  `status="open"`; anywhere else it is refused. This follows the playbook rule
  "Options change only on an open item": a stored answer means something only
  under the class and options it was given.
  Auto-confirmed (design review, 2026-11-09): the same rule applies to
  options. `applyPatch` today checks the item's status before the patch's own
  status change, so a settled item cannot take `status="open"` plus options in
  one patch. The open check for both `class` and options is evaluated after
  the same patch's status change, so one patch can reopen, reclass and give
  new options.
- **Playbook text** (124, acknowledged 2026-11-09): the `whiteboard` playbook
  (`agents-plugin/rsrc/whiteboard/whiteboard.md`, mirrored in
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` and
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`, all three byte-identical
  and pinned by `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go` and
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`) documents `class` on
  `wb:patch`, the re-ask behavior and the new `info:` warning.

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/skill-authoring.md`
  and `ai-docs/manuals/wsflow-mirroring.md` apply to the playbook edit and to
  every string the tool emits.
- An item that was never re-asked keeps today's sent behavior.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 7fea6af0 (2026-11-09, commit): "Dogfood surprise captured per AGENTS.md: reclassifying board items (112)-(114) to policy via wb:patch left them technical and the user saw them as resolved; worked around by superseding and re-adding" — bearing: supports
- 548c3aac (2026-11-09, commit): "answered(el) = responded(el) || sent-sent || sent-held ... Recompute trigger: a MutationObserver on the tracked items' class attribute, not a dashboard hook" — bearing: constrains
- 82372413 (2026-11-09, commit): "assumed items ... Counted as answered rather than dropped from the pill, matching the sent/held handling in 548c3aac6" — bearing: constrains
- a12a3843 (2026-11-09, commit): "User decision (whiteboard (35) a): close withdraws open/assumed items ... reopen leaves them withdrawn. ... A patch to open does not reopen a topic, so a closed topic can still wait on the user." — bearing: constrains
- 97c3ad72 (2026-11-08, commit): "The class is recomputed from scratch on every applySent run, like the marks, so it clears by itself when the lead records the answer ... No Go page test covers applySent's client-side markup" — bearing: supports
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, 62055a21 commit): "Sent marks derive from disk: the newest chip per item across the drained records (newest 20) and then the held entries ... Rejected: browser-only localStorage marks" — bearing: constrains
- 261030-feat-session-whiteboard-mode (2026-10-30, Decisions): "patch?: [{id, title?, options?, recommendation?, rationale?, links?, status?}]" (original patch schema without class) — bearing: constrains
- 261106-feat-whiteboard-policy-answer-user-alternative (2026-11-06, 324be2a0 commit): "whiteboard.update refused a policy answer that was a new user-proposed alternative, forcing a patch-then-answer two-update workaround" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, commands.go; internal/mcp/dashboard_page.go (ReadPageLang); internal/wsdashboard/page.go (PageContent field), pageview_inbox.go (sentMarks); internal/wsinbox/inbox.go (per-chip send time in Send/Merge); three whiteboard.md playbook copies. dashboard.js, dashboard.tmpl and pill.js do not change |
| scope.surface | public-interface | wb:patch command attributes in whiteboard.update, documented in the shipped whiteboard playbook; also crosses wswhiteboard and wsdashboard |
| scope.new_public_symbol | yes | class attribute on wb:patch, commands.go#L28 currently allows id status clear only |
| scope.new_type_contract | yes | persisted re-ask time on the store Item (store.go#L116 area) and a per-item re-ask time field on wsdashboard PageContent; .sentdata markup unchanged |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/store_test.go, commands_test.go, internal/wsdashboard tests, internal/wsrsrc mirror tests; no JS harness for applySent per 97c3ad72 |
| complexity.reuse_points | confirmed | class EnumAttr with classValues at store.go#L599 and the options-only-on-open refusal in applyPatch store.go#L750-L755 |
| complexity.side_effect_risk | moderate | adds a persisted store field and filters sentMarks, which feeds every sent mark on the dashboard |
| risk.correctness | moderate | re-ask time (RFC 3339) must be compared server-side against inbox Entry.SentAt (epoch ms) |
| risk.fit | low | class-on-open rule mirrors the existing options-on-open refusal in applyPatch |
| risk.test | moderate | the filter is server-side and Go-testable; applySent and pill.js have no JS test harness and do not change |
| risk.security_or_contract | moderate | changes the shipped wb:patch contract and the persisted whiteboard.json item shape |

## Phases

### Phase 1: reclass via wb:patch and re-ask-aware sent marks

- `internal/wswhiteboard`: `wb:patch` takes `class` (rules above), and the
  batch result's `info:` warns on options put on a non-policy item. Record the
  re-ask time for the sent-mark decision.
- `internal/wsdashboard`: `sentMarks` drops chips sent before the item's
  latest re-ask, so the page's sent mark, `pill.js`'s open-question pill and its
  tab-title count (`internal/wswhiteboard/assets/pill.js`) treat the item as
  open again. Plumb the re-ask times through `ReadPageLang`
  (`internal/mcp/dashboard_page.go`) into a `PageContent` field.
- Playbook text in both packages (three byte-identical copies: agents-plugin,
  agents-plugin-wsflow and agents-plugin-pi, pinned by
  `internal/wsrsrc/wsflow_mirror_test.go` and `internal/wsrsrc/pi_mirror_test.go`).

#### Verification

From `agents-plugin-tool/`: `gofmt -l ./internal ./cmd`, `go vet
./internal/...`, `go test ./internal/wswhiteboard/ ./internal/wsdashboard/
./internal/wsi18n/ ./internal/wsdoc/... ./internal/wsinbox/ ./internal/mcp/ -count=1` plus any package test that
pins the playbook files, and `node --check` on changed JS. Tests pin these
behaviors:
- A patch reclasses an open item and is refused on a settled one without
  `status="open"`; one patch with `status="open"`, `class="policy"` and
  options on a settled technical item succeeds.
- The options-on-non-policy warning appears for `wb:add`, for a patch adding
  options, and for a reclass away from policy that keeps options.
- A sent record (drained and held) older than the latest re-ask does not mark
  the item, and one sent after it does: for a class change on an open item,
  an options change on an open item, and settled to open. A title-only patch
  keeps the mark, and so does a patch resending the current class or
  identical options. A second re-ask replaces the first time.
- A held record created before a re-ask, with an answer for the item merged
  into it after the re-ask, still marks the item.

A headless browser check of a re-asked item on the dashboard is welcome. If it
is not run, say so.

### Result (57ba56f1d) - 2026-11-09

Landed:
- `wb:patch` takes `class` (`commands.go`, `applyPatch` in `store.go`). The
  open check for class and options reads the status after the patch's own
  `status` change, so one patch reopens, reclasses and gives options; a
  settled item without `status="open"` (or one the patch withdraws or
  supersedes) is refused.
- `Item.ReaskedAt` (RFC 3339, millisecond precision) is stamped on a real
  class change, a real options change (`slices.Equal`), or settled/assumed ->
  open; a later re-ask replaces it. `State.ReaskTimes()` exposes epoch ms.
- The update `info:` line warns ("Options on a non-policy item: (nn) [class]
  ...") when an add or patch leaves an item holding options under a class
  other than policy.
- `wsinbox.Chip.SentAt`: `Send` sets every chip to its send's time
  (overriding client values), `Merge` keeps each chip's own time. The cancel
  route's page copy strips it, so the browser wire shape is unchanged.
- `PageContent.Reasked` filled in `ReadPageLang`; `sentMarks` drops a chip
  (drained or held) with a send time earlier than its item's re-ask. Chips
  without a send time keep their marks. `dashboard.js`, `dashboard.tmpl` and
  `pill.js` are unchanged.
- Playbook: `wb:patch` row gains `class?`, the open-after-patch rule, a
  Re-ask bullet, and the new `info:` note, in all three byte-identical
  `whiteboard.md` copies; rsrc `manifest.json` regenerated and mirrored.

Verification (from `agents-plugin-tool/`): `gofmt -l ./internal ./cmd`
clean; `go vet ./internal/...` clean; `go test ./internal/wswhiteboard/
./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wsdoc/...
./internal/wsinbox/ ./internal/wsrsrc/ ./internal/mcp/ -count=1` all ok. No JS
changed, so no `node --check`. New tests: `wswhiteboard/reask_test.go`,
`wsinbox/chip_sent_test.go`, `TestSentMarksDropAnswersSentBeforeTheItemsReask`,
`TestDashboardReadPageCarriesReaskTimes`. Lite review: clean. No headless
browser check was run.

Decisions:
- The non-policy options warning judges every add/patch command by the item
  after it, so a later patch of such an item (for example a `wb:why` edit)
  warns again; deduped per batch.
- Because open checks read the post-patch status, a patch that sets options
  and `status="withdrawn"` on an open item in one command is now refused
  (previously accepted).
- Chip send time is stripped from the cancel route's response rather than
  exposed to the page.
