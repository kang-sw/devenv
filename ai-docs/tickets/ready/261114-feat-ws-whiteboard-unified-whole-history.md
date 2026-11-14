---
title: "Unify rich whole-entry HISTORY with the centered header and unobstructed focus"
related:
  261114-bug-ws-dashboard-oq-wrap-top-class-collision: regression and collision fix owned here; idea remains unchanged
  261114-bug-ws-whiteboard-fold-reopens-on-panel-republish: EXEC3 supplied baseline cause evidence; no assumption that its unmerged ticket is present
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6ece1ddbc182036b
sage-review-completeness-reviewed: 6ece1ddbc182036b
---

# Unify rich whole-entry HISTORY with the centered header and unobstructed focus

## Background

TASK3 is one accepted implementation unit covering 650, 651, 633, 634, 683 and
691. MAIN recovered the approved 633/634 scope and relayed owner
`683,684=ok`; this supersedes the retained drafts' separate-placement and
missing-chronology-approval holds. Owner's whole-history instruction:
`기록 쪽 클릭해서 접는 기능 아예 없애고, 이전 더 보기 뱃지 위로 밀어올리는 기능`.

The approved TASK3 contract is recorded in
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/e81ff7f0-01-exec2-approved-tasks-3-4-675.md`.
Its latest parallel-worktree override permits independent workers; develop
merges remain serialized through MAIN. After lead fact population and required
lite design/completeness gates, implementation needs no further owner wait.

## Approved references

- [Centered floating header](../assets/261114-feat-ws-whiteboard-history-centered-header/approved-centered-header.fragment.html)
  and [original report](../assets/261114-feat-ws-whiteboard-history-centered-header/approved-centered-header-report.md).
- [Unified rich decision density](../assets/261114-feat-ws-whiteboard-history-unified-decisions/approved-density.fragment.html)
  and [original report](../assets/261114-feat-ws-whiteboard-history-unified-decisions/approved-density-report.md).

Preserve these four assets and both retained idea drafts byte-for-byte; the
assets are tracked here, but the two drafts remain in
`/Users/kang-sw/.ws-worktrees/devenv/reference-outcast-afford/ai-docs/tickets/idea/`
(not this checkout; `git ls-files '*history-centered-header*' '*history-unified-decisions*'`
returns only the four assets). The
header report's historical `650=no` and review-only wording precede approval;
MAIN's 683 and owner `683,684=ok` supersede that restriction, not its historical
record. Density approval was `651=ok`. Mockup IDs/answers are examples, not live
decisions; mockup-only checks are not production verification.

## Decisions

1. Move `이전 더 보기` into a centered floating rounded capsule in the compressed
   approximately 28px HISTORY header band, with history count left and quiet
   panel/log totals right. Keep geometric centering and visible totals without
   narrow overlap; exhaustion hides only the capsule, leaving the band stable.
   The rejected alternative is the right-aligned rectangular utility button.
2. Remove only the whole-HISTORY details/click-to-collapse behavior: HISTORY
   stays expanded when its containing view is shown. **Per-panel collapse must
   remain**, including panel versions; preserve independent topic, system,
   log-group and item disclosures and saved descendant state. Ignore only
   obsolete saved whole-history open/closed state.
3. Keep `stream.js` choose()/fit() selection policy: 3 -> 2 -> 1 panels within
   1000px; log-only selection retains 30 lines. Render selected entries WHOLE,
   including a single oversized entry: no clipping viewport, partial entry,
   fade/gradient (including the old 22px overlay), forced height, line clamp or
   ellipsis. Load older reveals additional whole entries, not pixel slices.
4. Closed topics use the SAME history component and appearance as open cards:
   time/author, folded log groups and expansion. Remove the bottom `결정 기록`
   section; put policy/announced/technical/check items and their attached
   answers in chronological HISTORY flow, not a separate catalog. Preserve
   recorded answers and times under confirmed Decisions 8 and 9: recorded
   event time/order only, visible unknown-time legacy items, retained legacy
   per-item creation logs, and future-only coalescing of actually known batches.
   These decisions supersede the earlier blanket archive-time ordering and
   creation-batch single-log wording; no timestamp fallback or migration is
   authorized.
5. Reuse the existing rich question renderer: class colors/labels, meaningful
   body/reasons, options, recommendation, answer value and owner's verbatim
   quote remain. Show identity/class/title/status once. Plain `ok` has no
   redundant answer/detail row; omit genuinely empty disclosures, not useful
   content that resembles a title. Keep assumed, withdrawn and superseded
   distinct, including interpretation, reasons and replacement references.
   Narrow layouts retain time and natural title wrapping.
6. Own 691 here: remove the `.oq-wrap.top`/header `.top` class collision AND
   actual `.oq-wrap`/`.oq-pill` obstruction of a Tab-focused history boundary
   control. Its center must be exposed, not merely focused or within history
   bounds. Prove both fixes rather than assuming the collision causes the
   obstruction. This is not general CSS cleanup or EXEC1's separate 690
   p1/`DIV.tbar` stale-offset repair. Leave the existing collision idea open
   until the lead decides its disposition.
7. Compatibility target: T0 dashboard presentation. Preserve MCP/API/protocol,
   store and answer semantics; no T2/minor bump is authorized. Any genuinely
   unconfirmed data/schema/chronology choice returns through MAIN with question,
   class, options, recommendation and why; do not invent a fallback or IDs.
8. **718 confirmed**: use ONLY recorded event times and order. Keep undated
   legacy items visible with an explicit unknown-time label and stable existing
   order. Never call settlement time archive time or infer a historical
   timestamp from `ChangedAt`, settle counters, IDs or the current clock.
   MAIN relayed the owner's verbatim `718,719=a` at 2026-11-14T12:07:54;
   this settles the missing-time disposition without changing stored history.
9. **719 confirmed**: retain legacy per-item creation logs. Coalesce ONLY
   future updates whose batch membership is actually known; matching timestamps
   alone do not establish a historical batch. The same owner answer
   `718,719=a` settles this boundary. No invented batch ID, schema field,
   historical regrouping or migration is authorized. Decisions 8 and 9 require
   no repeated owner approval; design/readiness gates remain with MAIN.
10. **754 confirmed, relayed by MAIN at 2026-11-14T13:10:09**: a folded
    latest panel follows its stable **pN** identity across a republish to a
    new eN and stays folded. Its header shows a prominent, noticeable
    **updated** badge until the panel is actually opened. Owner's exact
    answer: `754=a^네, 다만 갱신된 건 좀 시끄러운 UI로 ... 아, 근데 지금 메시지 보드 재디자인 티켓 돌고 있지 않던가요?`.
    This explicitly reverses the prior preserve-new-version-default-open
    instruction for a panel the reader folded; an unrelated new pN retains
    its default. Keep historical panel-version disclosures independent.
    Use T0 presentation/browser state only, with legacy entry-key browser
    state migration and reload/frame-graft coverage. A transient graft
    `open` attribute/toggle must not acknowledge an unread update or clear
    its badge. Do not change durable entry IDs, history or store schema.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` for implementation under
  `agents-plugin-tool/internal/`; `internal/mcp/` also requires
  `ai-docs/manuals/ws-mcp.md`. Any shared prompt/skill work requires
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Preserve paging, finalized boundary IDs, first-height caches, expanded steps,
  deep links, keyboard reveal, live-update identity, reader scroll compensation,
  meaningful content and native controls except the two approved removals.
  Keep the actual `.history-older` inside its `.wb-history` owner; preserve the
  separate embedded recap branch and existing open-question answer controls.
- Read-only grounding at develop `9854ff350fa1b0eccfb5c71649c5190c405925b9`
  plus isolated capture `1b54402f3` (this checkout's HEAD; the capture adds only
  this ticket and the four assets, `git diff 9854ff350..1b54402f3 --stat`):
  `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl` templates
  `stream`, `settled`, `srow`, `ctx`, `opts`; `render.go` methods `Expandable`,
  `OptionHTML`, `RationaleHTML`, `RecHTML`; `assets/stream.js` functions `choose`,
  `fit`, `capture`, `restore`, `updateControl`, `init`. These already supply rich
  rendering, owner-local paging and independent fold/state reuse points.
- At that develop revision, `wswhiteboard/assets/page.css` still has the
  max-height/hidden history clip and 22px gradient; `.oq-wrap.top` meets bare
  `.top { gap: 10px; margin-bottom: 10px; }` in dashboard CSS. `store.go` Item
  has `AnsweredAt`, `ChangedAt`, settle-order `Settled`, not an archive-time
  field; Decisions 8 and 9 now settle the presentation of these recorded and
  missing data without introducing a chronology field.
- Prior 691 evidence: `/tmp/compact-panel-649-integrated-preview/{plain,combined}-first-probe-diagnostic/{first-probe-diagnostic.json,board-first-probe-diagnostic.png}`.
  Both probes reach boundary/focus e320 but report `centerExposed=false` at
  (720,97.6484375), under the pill at x674.719..765.281/y86..114, sticky wrap
  z4 (VERIFIED: `plain-first-probe-diagnostic/first-probe-diagnostic.json`,
  `combined-first-probe-diagnostic/first-probe-diagnostic.json` under that root).
  VERIFIED: the later baseline log also proves pill-family occlusion of e320,
  p156 and e519 (`/tmp/compact-panel-649-integrated-preview/baseline-standalone-e519.log#L1-L11`).
  The 700 expected-exception IDs are regression targets to FIX under 691, not
  accepted implementation exceptions. The tracked harness already requires
  center exposure (`scripts/board-headless.mjs#L2639-L2643`,
  `scripts/board-headless.mjs#L2679-L2689`); temporary modified predicates in
  diagnostic logs are not production acceptance. This is baseline evidence,
  not proof that a selector change fixes it.
- Apply 718/719 without rewriting historical evidence or the four approved
  assets. Earlier reference wording about archive-time placement or one log
  per creation batch is historical where it conflicts with Decisions 8 and 9.
  Keep unknown-time legacy items visible in their stable existing order;
  retain their per-item creation logs. Coalescing is future-only and requires
  actual batch knowledge, not a timestamp heuristic or invented batch schema.
- 754 is a bounded owner-approved refinement of current panel identity and
  header presentation, not a wider redesign. EXEC3's deployed-9854 trace
  proves the old entry-key/new-version default-open mechanism, not the
  reported incident or idle jitter. Verify stable-entry folds under frequent
  unrelated log/panel updates separately from same-pN republish and new-pN
  defaults; do not claim the owner incident or transient jitter resolved.
- Shared daemon activation is separate from source integration, announced
  before/after and coordinated after or batched with EXEC1's 652+670. No source,
  runtime, build, fixture or worktree changes belong to this ticket capture.

## Verified chronology and renderer evidence

All source paths below are under `agents-plugin-tool/internal/` unless absolute.
These observations ground the approved direction; they do not reopen 633/634,
650/651/683 or either approved removal.

- VERIFIED: `wswhiteboard/entries.go#L25-L38` stores immutable entries with
  `ID`, `Kind`, `ItemID`, `At`, text and panel metadata, but no batch identifier
  or item snapshot. `appendEntry` allocates eN and stamps the supplied update
  time in UTC RFC3339Nano, appending in command order
  (`wswhiteboard/entries.go#L62-L76`, `wswhiteboard/store.go#L619-L643`).
  `streamView` retains entry order, formats only parseable recorded `At`, and
  groups adjacent system events; it does not timestamp-sort or reconstruct
  missing clocks (`wswhiteboard/stream_view.go#L216-L292`). Equal-time entries
  therefore retain stream order, not an invented historical time.
- VERIFIED: creation currently emits one attributed `Item ... added` event per
  item, not one persisted creation-batch event
  (`wswhiteboard/store.go#L858-L866`). One `Apply` supplies the same `env.Now`
  to its commands and advances render sequence once
  (`wswhiteboard/store.go#L583-L643`). This was the evidence behind the former
  creation-batch gap, now resolved by owner 719: retain legacy per-item logs
  and coalesce only future updates with actually known batch membership.
  INFERRED: grouping solely by identical `At` cannot prove historical batch
  identity; entries do not retain the batch sequence. No new batch ID or schema
  is selected or needed as historical evidence.
- VERIFIED: answers and assumptions stamp `AnsweredAt` at seconds precision,
  increment `Settled`, and append an attributed event at the update's
  RFC3339Nano time; identical repeated answers are no-ops
  (`wswhiteboard/store.go#L900-L921`). Topic-close withdrawals also settle and
  append events (`wswhiteboard/store.go#L700-L719`). Status patches settle
  on transition and emit a patch event; reopening clears the current answer
  fields but leaves the immutable stream (`wswhiteboard/store.go#L1053-L1075`).
  These are actual recorded settlement/answer events, not an explicit archive
  event or a complete historical item-content snapshot.
- VERIFIED: archive rotation is a render-time selection of recent settle
  counters, excluding closed topics and checks kept in their checklist
  (`wswhiteboard/view.go#L821-L855`); eviction drops excess settled items
  (`wswhiteboard/store.go#L551-L578`). Neither creates an archive timestamp.
  `Item` has no creation/archive timestamp (`wswhiteboard/store.go#L118-L154`).
  `ChangedAt` is mutable and even backfilled at the first update for old
  records (`wswhiteboard/nudge.go#L26-L47`), so it is not historical archive
  evidence. INFERRED: a matching settlement event can ground that transition's
  recorded time, but is not proof of a later render-rotation time. Owner 718
  resolves presentation using only recorded event time/order; settlement time
  remains labeled as settlement/event time, NEVER archive time.
- VERIFIED: legacy folder conversion intentionally imports only the provable
  recap with its original time, possibly absent, and manufactures no body,
  creation or answer events (`wswhiteboard/folders.go#L308-L324`). Missing
  legacy creation/archive events and exact historical batch membership cannot
  be recovered from current item times or counters. Historical fact-population
  gaps were resolved by the owner's `718,719=a`, relayed by MAIN at
  2026-11-14T12:07:54: unknown-time legacy items stay visible with an explicit
  unknown-time label and stable existing order; legacy per-item creation logs
  remain unchanged. Missing data remain missing, not a normative hold or a
  renewed owner question. No fabricated archive time, batch ID, schema or
  migration follows from this resolution.
- VERIFIED: full-page `stream` already serves open and closed topics, but its
  whole-history details starts closed on closed topics; embed recaps take a
  separate branch (`wswhiteboard/assets/page.tmpl#L54-L67`). Items currently
  live outside that stream: policy/ack/settled branches in `topic`, and checks
  in their own list (`wswhiteboard/assets/page.tmpl#L44-L44`,
  `wswhiteboard/view.go#L600-L627`). `srow` reuses `ctx` and static `opts`;
  `Expandable` currently checks only options/rationale, and `stext` renders
  title, picked answer, interpretation, quote and links, not a full class/time
  timeline row (`wswhiteboard/assets/page.tmpl#L120-L128`,
  `wswhiteboard/render.go#L789-L808`). Preserve sanitized rich-field rendering,
  not raw HTML injection or a replacement mock renderer
  (`wswhiteboard/markdown.go#L242-L247`).
- VERIFIED: selection/state reuse is in `wswhiteboard/assets/stream.js#L28-L143`
  and `#L354-L407`; owner-local control lookup and click delegation are in
  `#L146-L164` and `#L409-L460`. Dashboard restores keyed folds separately
  (`wsdashboard/assets/dashboard.js#L179-L208`, `#L607-L608`), so ignoring only
  history snapshot `open` is not the entire whole-fold-state audit. Existing
  clip/fade rules remain (`wswhiteboard/assets/page.css#L460-L462`); the pill
  modifier meets bare `.top` rules in BOTH whiteboard and dashboard CSS
  (`wswhiteboard/assets/page.css#L116-L119`, `#L668-L673`,
  `wsdashboard/assets/dashboard.css#L66-L66`).
- VERIFIED: all four linked assets compare byte-for-byte with the retained
  checkout on 2026-11-14. Their report checks describe mockups, not current
  production behavior. Retain both links and the literal side-by-side
  implementation verification below. No implementation/build/browser tests
  were run during this population.

## Prior Decisions

- 8f24cec5 (2026-11-14, commit): "Saved entry IDs replace cumulative 640px snapping so later geometry can grow the clip without changing its membership." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, commit): "Distinguish historical disclosure keys so a repost folds a previously expanded older version without losing later manual history choices." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-history-load-more (2026-11-14, commit): "Retry also confirmed focus occlusion by elementFromPoint hit-testing and fixed it by offsetting navigation past the sticky head stack." — bearing: constrains
- 261110-feat-ws-whiteboard-item-field-html (2026-11-11, commit): "One sanitizer (D3): itemField reuses rawMarkdown + sanitizeReply with a salt param" — bearing: constrains
- 261109-feat-ws-whiteboard-settled-row-expand (2026-11-10, commit): "Static options carry no data-opt, aria-pressed, button, or role" — bearing: constrains
- 0325a7aa (2026-11-14, commit): "Ticketless UX fix: root cause was the in-memory-only openState; panel fold state now persists in localStorage ws.dashboard.panelshut" — bearing: constrains
- 702e2f9d (2026-11-13, commit): "Structure is built server-side (EntryPage.Segments) rather than by client JS wrapping, because the dashboard's frameGraft needs matching DOM shapes along a frame's path." — bearing: constrains
- 261114-feat-ws-dashboard-shared-card-actions (2026-11-14, Decisions): "Preserve independent inner history/panel disclosures." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl, assets/page.css, assets/stream.js, render.go, view.go; wsdashboard/assets/dashboard.css and dashboard.js; scripts/board-headless.mjs |
| scope.surface | cross-module | shared window.wbHistory and rendered DOM consumed by dashboard swaps, folds and navigation |
| scope.new_public_symbol | no | no new exported API authorized; existing rich renderer and history helpers reused |
| scope.new_type_contract | no | T0 presentation only; no new store/schema/protocol or answer contract selected |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/stream_script_test.go, stream_test.go, render_test.go; internal/wsdashboard/css_collision_test.go; scripts/board-headless.mjs |
| complexity.reuse_points | confirmed | read choose/fit/capture/restore/updateControl/init, stream/settled/srow/ctx/opts, Expandable and rich field methods |
| complexity.side_effect_risk | high | shared history membership, identity, keyed folds, late frame heights and reader compensation interact |
| risk.correctness | high | chronological item projection must preserve real event times and distinguish missing legacy data without fabricating times |
| risk.fit | high | approved combined presentation spans stream segmentation, item/check lists, dashboard state restore and two CSS namespaces |
| risk.test | high | real Tab center hit-testing and side-by-side rich-content captures on both hosts cannot be established by static or mockup tests |
| risk.security_or_contract | moderate | rich HTML must retain sanitizer/static-option trust boundaries and unchanged answer/store semantics |

## Phases

### Phase 1: Implement and verify the combined whole-entry history contract

Implement the decisions together on the current integration base, reusing the
rich renderer and existing history selection/state machinery. Verify with
relevant Go tests (`go test ./internal/wswhiteboard ./internal/wsdashboard
./internal/mcp` from `agents-plugin-tool/`) and independent lite review before
MAIN's serialized develop merge.

Require literal side-by-side comparison of actual before/after headless result
captures against BOTH tracked approved text visuals at **1280/800/360px**, on
standalone and dashboard surfaces. Include a long topic with **20+ log lines
and 3 panels**, rich answered decisions (table/code/HTML, options, recommendation
and substantive owner quote), plain `ok`, assumed/withdrawn/superseded states,
closed/open history parity and narrow time visibility. Mockup slot heights are
not real-content height targets.

Verify 718/719 against actual renderer output on both surfaces: recorded
creation/answer/settlement events retain their recorded times and order,
including equal-time stream order; settlement time is never labeled archive
time. Undated legacy items remain visible, explicitly marked unknown-time,
with stable existing order through paging, reload and live replacement. Legacy
per-item creation logs remain per-item. Coalesce a future update only when its
batch membership is actually known; separate updates with matching timestamps
must not be merged merely because their times match. Confirm no invented time,
batch ID, schema field or historical regrouping. These are implementation-phase
checks, not authorization to run tests during ticket capture.

Exercise 3 -> 2 -> 1 and log-only 30 selection, oversized whole entries, repeated
load older through exhaustion, saved boundaries/first heights, reload/live
replacement/frame grafts and deep links. Assert no partial entry, clipping or
fade; per-panel and descendant folds retain independent behavior and state,
while saved whole-history `open:false` cannot hide HISTORY. Check centered
capsule/counts, no overlap/overflow, unchanged open-question controls and scroll
stability. Reproduce 691 with Tab and center hit-testing: the focused control
must remain exposed, with no `.oq-wrap`/`.oq-pill` coverage; separately confirm
the inherited class collision is gone.

For 754, verify a user-folded pN remains folded through stable-eN unrelated
updates and through republish to a new eN. A conspicuous updated header badge
persists through reload and frame paths until an actual open; transient
attribute/toggle changes during a graft do not clear it. Check legacy browser
entry-key migration, a genuinely new pN's unchanged default, independent
historical-version folds, and repeated updates while the badge is pending.
Include its visual refinement in both-reference comparison without changing
either approved asset. Send screenshots, comparison, Go output, review and any
unverified surface to MAIN before develop integration.
