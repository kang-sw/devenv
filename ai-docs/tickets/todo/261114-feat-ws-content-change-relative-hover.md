---
title: Relative change-time hover on content markers
---

# Relative change-time hover on content markers

## Background

Owner-approved TASK4 (MAIN 677): make the existing left-hand content-change
markers useful on hover, not merely report their status. Preserve this exact
owner request (05:43:39Z):

> 이 content 업데이트에 왼쪽 파란 줄/빨간 줄/ 초록줄 뜨는 업데이트 gizmo? 있잖아요? 이거 마우스 올리면 살짝 두꺼워지면서 (n분 전, 이건 무조건 상대 시간 표기~ m년 전 등) 시간 표시되게 하면 더 편하겠네요.

Later owner reminder, verbatim:

> content 좌측의 초록선 빨간선 파란선에 마우스 올리면 몇 분 전으로 시간 뜨는 기능도 만들어놓으라 그랬는데, 이것도 누락되어 있고요

Authority: TASK4 in the EXEC2 approved queue contract
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/e81ff7f0-01-exec2-approved-tasks-3-4-675.md`.
Its latest coordination override allows parallel isolated worktrees; MAIN
serializes develop merges. This ticket captures design only, not implementation
or daemon activation authority in this authoring pass.

## Decisions

- Green, red, and blue content-change bars thicken slightly on hover and show
  the actual change's age, always relative, from minutes through years. Never
  show a clock or calendar date in this marker hover. Exact thickness and
  tooltip mechanism are not prescribed.
- Keep marker colors, content layout, and anchor position; hovering must cause
  neither reflow nor scrolling. Topic-latest activity is not a substitute for
  the content edit's timestamp.
- Intended compatibility tier: T0. MAIN permits an additive optional timestamp
  field if needed, but no field shape, historical fallback, or migration is
  settled. Report missing provenance before choosing a schema.
- Retain existing 617 recap/log inline-time behavior rather than implement it
  twice. Repair it only if current behavioral verification finds it missing:
  author mark and time (owner example `04:08 · 3시간 전`) lead the body inline,
  leaving multiline text the available width rather than a middle time column.

## Prior Art

VERIFIED at authoring baseline: this branch `feat/content-change-hover-677`
and local develop both point to `9854ff350fa1b0eccfb5c71649c5190c405925b9`;
there is no whiteboard source diff between them.

- `agents-plugin-tool/internal/wswhiteboard/assets/chg.js`, search
  `wbChangeBars`: green `new`, blue `mod`, red `del` (including `delafter`)
  are generated from body comment markers into a zero-height absolute layer.
  `assets/page.css`, search `.wb-chg-bar`: 2px vertical bars, 8px by 2px
  deletion ticks, `pointer-events: none`; no hover behavior there.
- `changes.go`, search `type BlockMark`: persisted markers contain only block
  index `b` and kind `k`. `store.go`, search `setTopicBody` and `TopicDoc`:
  marks represent the latest body-edit batch, with body-specific `ChangedAt`;
  same-body/no-op edits preserve these, unrelated entry/recap changes do not
  replace them. This is not a retained per-block history.
- `nudge.go`, search `stampChanges`: current body-edit batches get a UTC
  RFC3339 timestamp, but missing legacy times also get the first observing
  update's time. A nonempty `ChangedAt` therefore does not alone prove an
  historical edit time. See the provenance gap below.
- `assets/ago.js`, search `wbAgoText`: existing relative formatter supports
  now/minutes/hours/days only. `assets/stream.js`, search `mouseover`: entry
  hover appends an absolute stamp. Neither is the complete marker contract;
  do not inherit absolute-time hover output for markers.
- 617 is already in this baseline: implementation `6bf4df29f`, merge
  `1ab3d5d7e`. Current `assets/page.tmpl`, search `define "recap"` and
  `define "entry"`, places time before text; `assets/page.css`, search
  `.trecap .rtext`, uses inline text and inline first paragraph. Historical
  line-number hints are not the evidence.

Live was inspected separately, read-only, on 2026-11-14: endpoint and
`GET /health` agreed on PID 94122, port 29417, version `0.50.0-dev`, start
`2026-11-14T10:50:02Z`, content hash
`e45b0b035191ee5ae6a1257294677b5f96412f14f5c058316ada0acbe2292eb6`.
The process executable carried that hash. Runtime metadata says source commit
`dev`, not a git SHA: equivalence to develop is unproven. A successful
`GET /s/sweatradartakingoverwrite` served inline recap CSS and the same
noninteractive bar CSS. A subsequent GET returned 404; no stable live recap
geometry or hover interaction was verified. INFERRED: live contained inline
recap styling, not proof that all recap entries visually satisfy 617.

## Blocked: historical timestamp provenance

The current body-edit timestamp supplies a candidate for all markers of a
known latest edit batch, but legacy missing/backfilled timestamps cannot be
asserted to be actual edit times. No new timestamp schema or unknown-age UX
is selected here. Lead must settle this evidence gap before ready promotion;
record the owner answer separately before adding an implementation decision.

## Constraints

- Future tooling edits follow `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md` per AGENTS.md path coverage.
- Preserve current framed-body and unframed-body behavior and marker geometry;
  do not replace source provenance with topic recency, filesystem mtime, or
  first-observation time labeled as an edit time.
- No source/runtime changes, builds, fixtures, merges, pushes, releases, or
  new worktrees in this authoring pass. Lead owns design/ready review and IDs.

## Phases

### Phase 1: Relative marker hover without layout movement

After timestamp provenance is settled and the lead completes the design/ready
gate, implement the approved marker hover contract. Preserve existing 617
inline author/time placement; do not broaden this into a history redesign.

Verification boundary: before/after headless captures at 1280/800/360 widths,
including green/blue/red and end-deletion marks, multiline content, stale marks,
framed and unframed bodies, and inline recap/log placement. Exercise relative
ages from minutes through years, and the settled missing-time policy. Assert
that unrelated topic activity does not change marker age, that hover contains
no clock/date, and that colors, anchors, content geometry and scroll offsets
are unchanged. Run the project's relevant whiteboard Go tests and independent
lite review; send evidence to the lead before MAIN's serialized develop merge.
Live activation and a fresh live verification are separate from source landing.
