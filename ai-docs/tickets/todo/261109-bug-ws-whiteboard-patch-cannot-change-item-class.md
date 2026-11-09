---
title: "whiteboard: a re-asked item keeps its old class and its sent mark, so it looks settled"
sage-review-design: required
sage-review-completeness: required
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
  Rejected: an `info:` warning alone (the lead still cannot reclass, so it
  ends in the supersede workaround); documenting the supersede-and-re-add
  workaround in the playbook.
- **Sent mark follows the reopen** (121 a note, user 2026-11-09: "화면의
  sent도 엮어서 같이 우아하게 처리되게 해야겠네요"): a reopened item is
  shown as waiting on the user again, not as sent. The open-question pill
  and the tab-title count that `pill.js` sets include it.
- **Sent mark ignores answers sent before the reopen** (122, acknowledged by the
  user 2026-11-09): when `wb:patch` sets `status="open"` on a settled item, the
  store records the reopen time on the item. The daemon's `sentMarks`
  (`internal/wsdashboard/pageview_inbox.go`) drops a chip for that item whose
  record's `Entry.SentAt` is older than the reopen time, so no stale mark
  reaches `.sentdata` and `applySent()` needs no change for it. Both times
  are wall-clock times written on the same machine (the daemon writes
  `SentAt`, the MCP store writes the reopen time).
  Auto-confirmed (fact-populator gap, 2026-11-09): which time and where to
  compare. `.sentdata` carries no time today. Filtering server-side, by the
  inbox entry's `SentAt`, was picked over adding a time attribute that
  `applySent()` would compare in JS.
  Rejected: deleting or rewriting the inbox records. They are the session's
  message history and other code reads them.
- **`class` only on open items** (123, acknowledged 2026-11-09): `wb:patch`
  accepts `class` only on an item that is open, or that the same patch sets
  `status="open"`; anywhere else it is refused. This follows the playbook rule
  "Options change only on an open item": a stored answer means something only
  under the class and options it was given.
- **Playbook text** (124, acknowledged 2026-11-09): the `whiteboard` playbook
  (`agents-plugin/rsrc/whiteboard/whiteboard.md`, mirrored in
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md` and
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`, all three byte-identical
  and pinned by `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go` and
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`) documents `class` on
  `wb:patch`, the reopen behavior and the new `info:` warning.

## Open Decision Queue

Last ID: 127

(127) [open] [policy] What counts as a re-ask that clears earlier sent marks (design review missing issue; amends (122)).
- Context: options change only on an open item, so the lead's patch of (112)/(113) that added options proves the items were still open in the store: the user's `ok` was sent from the dashboard but not yet recorded with `wb:answer`, which is exactly when sent marks show. (122) records a reopen time only on settled -> open, so the reported case stays unfixed.
- a: a patch that changes `class` or options, or that sets a settled item back to `status="open"`, records the re-ask time; title/rationale-only edits do not.
- b: any patch carrying `status="open"`, including on an already-open item; the lead must send it explicitly.
- c: keep (122): settled -> open only; the reported case stays unfixed.
> a: no signal for the lead to remember; changed options change what a sent letter means, so dropping that mark is correct.

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/skill-authoring.md`
  and `ai-docs/manuals/wsflow-mirroring.md` apply to the playbook edit and to
  every string the tool emits.
- An item that was never reopened keeps today's sent behavior.
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
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, commands.go, assets/pill.js, agents-plugin-tool/internal/wsdashboard/assets/dashboard.js and dashboard.tmpl, three whiteboard.md playbook copies |
| scope.surface | public-interface | wb:patch command attributes in whiteboard.update, documented in the shipped whiteboard playbook; also crosses wswhiteboard and wsdashboard |
| scope.new_public_symbol | yes | class attribute on wb:patch, commands.go#L28 currently allows id status clear only |
| scope.new_type_contract | yes | persisted reopen time on the store Item, store.go#L116 area, plus a reopen-time attribute on the board region and a time on sentdata records, dashboard.tmpl sentdata carries no time today |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/store_test.go, commands_test.go, internal/wsdashboard tests, internal/wsrsrc mirror tests; no JS harness for applySent per 97c3ad72 |
| complexity.reuse_points | confirmed | class EnumAttr with classValues at store.go#L599 and the options-only-on-open refusal in applyPatch store.go#L750-L755 |
| complexity.side_effect_risk | moderate | adds a persisted store field and changes applySent, which marks every sent item on the dashboard |
| risk.correctness | moderate | reopen time must be compared against inbox record times that the sentdata markup does not carry yet |
| risk.fit | low | class-on-open rule mirrors the existing options-on-open refusal in applyPatch |
| risk.test | moderate | applySent and pill.js have no JS test harness; browser check is optional in the ticket |
| risk.security_or_contract | moderate | changes the shipped wb:patch contract and the persisted whiteboard.json item shape |

## Phases

### Phase 1: reclass via wb:patch and reopen-aware sent marks

- `internal/wswhiteboard`: `wb:patch` takes `class` (rules above), and the
  batch result's `info:` warns on options put on a non-policy item. Record the
  reopen time for the sent-mark decision.
- `internal/wsdashboard`: `sentMarks` drops chips sent before the item's
  reopen, so the page's sent mark, `pill.js`'s open-question pill and its
  tab-title count (`internal/wswhiteboard/assets/pill.js`) treat the item as
  open again.
- Playbook text in both packages (three byte-identical copies: agents-plugin,
  agents-plugin-wsflow and agents-plugin-pi, pinned by
  `internal/wsrsrc/wsflow_mirror_test.go` and `internal/wsrsrc/pi_mirror_test.go`).

#### Verification

From `agents-plugin-tool/`: `gofmt -l ./internal ./cmd`, `go vet
./internal/...`, `go test ./internal/wswhiteboard/ ./internal/wsdashboard/
./internal/wsi18n/ ./internal/wsdoc/... -count=1` plus any package test that
pins the playbook files, and `node --check` on changed JS. Tests pin these
behaviors:
- A patch reclasses an open item and is refused on a settled one without
  `status="open"`.
- The options-on-technical warning appears.
- A sent record older than the reopen does not mark the item, and one sent
  after it does.

A headless browser check of a reopened item on the dashboard is welcome. If it
is not run, say so.
