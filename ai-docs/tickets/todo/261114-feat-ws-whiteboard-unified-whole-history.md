---
title: "Unify rich whole-entry HISTORY with the centered header and unobstructed focus"
related:
  261114-feat-ws-whiteboard-history-centered-header: retained approval provenance; combined here
  261114-feat-ws-whiteboard-history-unified-decisions: retained approval provenance; combined here
  261114-bug-ws-dashboard-oq-wrap-top-class-collision: regression and collision fix owned here; idea remains unchanged
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

Preserve these four assets and both retained idea drafts byte-for-byte. The
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
   answers in chronological HISTORY flow, not a separate catalog. Preserve the
   creation-batch single-log and archive-time ordering contract and recorded
   answers/times; no timestamp fallback or migration is selected here.
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

## Constraints and prior art

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
  (not this checkout's older base): `wswhiteboard/assets/page.tmpl` templates
  `stream`, `settled`, `srow`, `ctx`, `opts`; `render.go` methods `Expandable`,
  `OptionHTML`, `RationaleHTML`, `RecHTML`; `assets/stream.js` functions `choose`,
  `fit`, `capture`, `restore`, `updateControl`, `init`. These already supply rich
  rendering, owner-local paging and independent fold/state reuse points.
- At that develop revision, `wswhiteboard/assets/page.css` still has the
  max-height/hidden history clip and 22px gradient; `.oq-wrap.top` meets bare
  `.top { gap: 10px; margin-bottom: 10px; }` in dashboard CSS. `store.go` Item
  has `AnsweredAt`, `ChangedAt`, settle-order `Settled`, not an archive-time
  field; these facts do not select a chronology mechanism.
- Prior 691 evidence: `/tmp/compact-panel-649-integrated-preview/{plain,combined}-first-probe-diagnostic/{first-probe-diagnostic.json,board-first-probe-diagnostic.png}`.
  Both probes reach boundary/focus e320 but report `centerExposed=false` at
  (720,97.6484375), under the pill at x674.719..765.281/y86..114, sticky wrap
  z4. This is baseline evidence, not proof that a selector change fixes it.
- Shared daemon activation is separate from source integration, announced
  before/after and coordinated after or batched with EXEC1's 652+670. No source,
  runtime, build, fixture or worktree changes belong to this ticket capture.

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

Exercise 3 -> 2 -> 1 and log-only 30 selection, oversized whole entries, repeated
load older through exhaustion, saved boundaries/first heights, reload/live
replacement/frame grafts and deep links. Assert no partial entry, clipping or
fade; per-panel and descendant folds retain independent behavior and state,
while saved whole-history `open:false` cannot hide HISTORY. Check centered
capsule/counts, no overlap/overflow, unchanged open-question controls and scroll
stability. Reproduce 691 with Tab and center hit-testing: the focused control
must remain exposed, with no `.oq-wrap`/`.oq-pill` coverage; separately confirm
the inherited class collision is gone. Send screenshots, comparison, Go output,
review and any unverified surface to MAIN before develop integration.
