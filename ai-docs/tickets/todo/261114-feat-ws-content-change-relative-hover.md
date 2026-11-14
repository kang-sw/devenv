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
  the actual change's age when proven, always relative, from minutes through
  years. Never
  show a clock or calendar date in this marker hover. Exact thickness and
  tooltip mechanism are not prescribed.
- Keep marker colors, content layout, and anchor position; hovering must cause
  neither reflow nor scrolling. Topic-latest activity is not a substitute for
  the content edit's timestamp.
- Owner answer 713=b, relayed by MAIN at 12:00:19/12:01:14 on 2026-11-14,
  verbatim: `711,713=b`. For unknown or legacy-backfilled actual edit time,
  retain hover thickening and show the exact human-facing text
  `변경 시각 모름`. Do not invent a relative age.
- Intended compatibility tier: T0. MAIN permits prospective provenance only
  if its design survives old writers under T0. Otherwise deliver the confirmed
  713=b hover behavior alone and return prospective provenance as a
  compatibility question. No persistent T0 provenance path is evidenced yet,
  so the current delivery slice is 713=b only, with no new field or transient
  mapping. The accepted actual relative-age contract is deferred where proof
  requires that unresolved provenance design, not deleted or implemented.
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
- `store.go:672` calls `stampChanges(mark, env.Now)` after a successful
  mutation batch. For a body actually changed in that batch, `ChangedAt` is
  that batch's mutation time (UTC RFC3339), shared by its latest-edit marks,
  not the latest unrelated topic activity. `nudge.go:26–41` also stamps a
  missing legacy time on the first observing update, even without a body
  mutation. That value is observation time, not actual edit time. The stored
  field does not distinguish these two origins. See the provenance gap below.
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
The process executable carried that hash. Lead reports TASK1 independently
verified its build provenance with `go version -m`: base `9854ff350`,
`vcs.modified=true`. This identifies a modified build base, not content parity
with develop. A successful `GET /s/sweatradartakingoverwrite` served inline
recap CSS and the same noninteractive bar CSS at that point in time only.
A subsequent GET returned 404; neither drift nor a daemon swap is inferred.
Live lifecycle investigation belongs to EXEC3. INFERRED: the successful
response included inline recap styling, not proof that all recap entries
visually satisfy 617. No stable live recap geometry or hover interaction was
verified.

## Remaining gap: provenance compatibility

For a known actual mutation batch, existing `ChangedAt` is its actual edit
time and covers all its current marks; a separate timestamp per block is not
inherently required by that latest-batch model. However, `BlockMark` stores
no timestamp or provenance, and `ChangedAt` alone cannot distinguish actual
mutation from legacy backfill. The inspected representation does not prove
actual historical per-mark times. The bounded folder-store trace found no
general body-mutation history establishing those times. Owner answer 713=b
settles their display, so missing historical provenance is no longer a blocker
to the confirmed unknown-time hover behavior.

`agents-plugin-tool/internal/wswhiteboard/folders.go`, search `folderDecode`,
`folderJSON`, and `topic.Doc = &copy`: typed old writers can discard unknown
optional fields. Additive syntax alone therefore does not establish T0
preservation. Existing Seq/Generation values do not prove timestamp origin.
The successful mutation boundary could support new transient witnessing, but
no such trusted mapping exists today, and process-lifetime retention is an
unapproved product choice. Neither approach reconstructs unknown history.

Prospective provenance remains conditional on lead design review demonstrating
old-writer T0 compatibility; otherwise return that question separately and
proceed only with 713=b. No observation age, topic-latest age, or invented
historical value is authorized as actual change age. The known actual-age
contract remains required wherever actual batch provenance is established;
713=b does not authorize hiding proven ages.

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

After the lead completes the design/ready gate, implement the approved 713=b
slice: slight hover thickening and exact `변경 시각 모름` for unknown/backfilled
or otherwise unproven actual edit times. Include no new persistent field or
transient mapping. Return the unresolved prospective provenance option as a
compatibility gap, without another policy question or mechanism selection.
The original relative actual-age contract remains accepted but its
provenance-dependent delivery is deferred under MAIN's direction, not claimed
implemented by this slice. Preserve existing 617 inline author/time placement;
do not broaden this into a history redesign.

Verification boundary: before/after headless captures at 1280/800/360 widths,
including green/blue/red and end-deletion marks, multiline content, stale marks,
framed and unframed bodies, and inline recap/log placement. Exercise relative
ages from minutes through years when the deferred provenance-dependent work
is authorized; for the current 713=b slice, assert exact `변경 시각 모름` plus
thickening for unknown/backfilled or otherwise unproven actual times. Assert
that unrelated topic activity does not change marker age, that hover contains
no clock/date, and that colors, anchors, content geometry and scroll offsets
are unchanged. Run the project's relevant whiteboard Go tests and independent
lite review; send evidence to the lead before MAIN's serialized develop merge.
Live activation and a fresh live verification are separate from source landing.
