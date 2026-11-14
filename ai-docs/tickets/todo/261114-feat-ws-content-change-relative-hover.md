---
title: Latest body update time in topic header and change gutter
---

# Latest body update time in topic header and change gutter

## Background

Owner-approved TASK4 (MAIN 677): make the existing left-hand content-change
markers useful on hover, not merely report their status. Preserve this exact
owner request (05:43:39Z):

> 이 content 업데이트에 왼쪽 파란 줄/빨간 줄/ 초록줄 뜨는 업데이트 gizmo? 있잖아요? 이거 마우스 올리면 살짝 두꺼워지면서 (n분 전, 이건 무조건 상대 시간 표기~ m년 전 등) 시간 표시되게 하면 더 편하겠네요.

Later owner reminder, verbatim:

> content 좌측의 초록선 빨간선 파란선에 마우스 올리면 몇 분 전으로 시간 뜨는 기능도 만들어놓으라 그랬는데, 이것도 누락되어 있고요

At 12:28:12 on 2026-11-14, the owner reduced the requirement to one latest
body-content update time per topic, before implementation had started:

> 아 근데 이거 혹시 이미 구현 들어갔나요? 사실 모든 행에 N분 전이 뜨기보단, 그냥  topic content 자체의 최신 반영이 언제인지만 알면 되는데. 각 토픽당 숫자 하나로요. 제가 처음 말한 것보다 범위가 꽤 어려워 보여서요. 다만 이미 들어갔다면 그냥 그대로 진행하고요 ..

The owner then approved header placement and clarified that the latest-delta
gutter must show that same single time (MAIN relay at 12:53:04), verbatim:

> 743=a^그리고 추가로(원래 요청했던 거지만) 최신 반영 delta 나타내는 좌단의 gutter에 마우스 호버했을 때에도 살짝 선분 두꺼워지면서 시간 뜨게요. 이건 숫자 하나면 되잖아요? (어차피 최신 delta만 선으로 표시하니까)

These refinements supersede the earlier per-mark interpretation and temporary
713-only/all-unknown delivery wording. They retain the original body-time and
gutter intent; the ticket stem remains unchanged.

Authority: TASK4 in the EXEC2 approved queue contract
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/e81ff7f0-01-exec2-approved-tasks-3-4-675.md`.
Its latest coordination override allows parallel isolated worktrees; MAIN
serializes develop merges. This ticket captures design only, not implementation
or daemon activation authority in this authoring pass.

## Decisions

- One latest actual BODY-mutation time per topic is the source for both
  displays: a distinct header value beside overall activity time, using
  `본문 N분 전 갱신`, and the latest-delta gutter hover. This is not per-row,
  per-paragraph, per-mark history or overall topic activity.
- Green, red, and blue latest-change gutter bars thicken slightly on hover
  and show the SAME per-topic body-update time as the header. Known actual
  ages are always relative, from minutes through years; neither display may
  substitute a clock or calendar date. Exact thickness and tooltip mechanism
  are not prescribed.
- Keep marker colors, content layout, and anchor position; hovering must cause
  neither reflow nor scrolling. Topic-latest activity is not a substitute for
  the content edit's timestamp.
- Owner answer 713=b, relayed by MAIN at 12:00:19/12:01:14 on 2026-11-14,
  verbatim: `711,713=b`. For unknown or legacy-backfilled actual edit time,
  retain hover thickening and show the exact human-facing text
  `변경 시각 모름`. Do not invent a relative age.
- Owner answer 743=a confirms topic-header placement beside, and distinct
  from, overall activity time; its gutter refinement requires the same source
  and value in both places. Unknown/backfilled or otherwise unproven actual
  times use exact `변경 시각 모름` in both displays, retaining gutter thickening.
- Intended compatibility tier: T0 with the owner's narrow 724=a exception:
  old writers MAY discard UX provenance, causing the age to become unknown
  again, but NEVER a false actual age. This does not relax the general T0
  unknown-field preservation rule. Proof must bind the exact actual body
  mutation batch, with stale/mismatched proof failing closed. Proof loss is
  not permission to infer trust from existing ChangedAt. Exact proof
  representation remains an unselected technical design; no field/schema,
  API, migration, or transient authority is chosen by this ticket.
- Retain existing 617 recap/log inline-time behavior rather than implement it
  twice. Repair it only if current behavioral verification finds it missing:
  author mark and time (owner example `04:08 · 3시간 전`) lead the body inline,
  leaving multiline text the available width rather than a middle time column.

## Prior Art

VERIFIED at fact-population baseline: this branch `feat/content-change-hover-677`
points to `4b88660eec6f1ca045c192116ba7f301a077a4d2`; local develop points to
`4412b2c672e9e5df776d4573a58dc4e3bc4be783`. Their changes since the shared
product baseline `9854ff350fa1b0eccfb5c71649c5190c405925b9` are ticket-only;
there is no whiteboard source diff between them (`git diff --name-only
9854ff350 HEAD` and `git diff --name-only 9854ff350 develop`; unchanged marker
implementation: `agents-plugin-tool/internal/wswhiteboard/assets/chg.js#L8-L49`,
`agents-plugin-tool/internal/wswhiteboard/assets/page.css#L553-L559`).

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

## Remaining technical gap: batch-bound provenance

For a known actual mutation batch, existing `ChangedAt` is its actual edit
time and covers all its current marks; a separate timestamp per block is not
inherently required by that latest-batch model. However, `BlockMark` stores
no timestamp or provenance, and `ChangedAt` alone cannot distinguish actual
mutation from legacy backfill. The inspected representation does not prove
actual historical body-edit times. The bounded folder-store trace found no
general body-mutation history establishing those times. One value per topic
removes per-mark attribution, not this origin ambiguity. Owner answer 713=b
settles unknown display; 743=a requires the same trusted body-time value in
both the header and latest-delta gutter.

`agents-plugin-tool/internal/wswhiteboard/folders.go`, search `folderDecode`,
`folderJSON`, and `topic.Doc = &copy`: typed old writers can discard unknown
optional fields. Additive syntax alone does not establish general T0
preservation; 724=a specifically permits safe loss of this UX proof. Existing
Seq/Generation values do not prove timestamp origin. No existing persistent
provenance path has been established, and process-only witnessing is not a
selected authority or retention model.

Design must establish actual mutation provenance for new valid body updates,
bind it to the exact current batch, and fail closed after proof loss or stale
proof. An old writer dropping proof may produce unknown; an old writer
preserving stale proof must never authenticate a different mutation. Neither
observation time, topic-latest time, timestamp coincidence, nor an invented
historical value is actual edit evidence. The precise proof representation
and old/new-writer behavior need fresh fact population and design review;
this technical gap does not defer or remove the confirmed known-age contract.

## Constraints

- Future tooling edits follow `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md` per AGENTS.md path coverage.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Header and gutter must share one body-specific source and value; unrelated
  item/log/recap activity must not renew body age. No per-row or per-mark
  history, and no existing ChangedAt value is trusted solely for being present.
- Preserve current framed-body and unframed-body behavior and marker geometry;
  do not replace source provenance with topic recency, filesystem mtime, or
  first-observation time labeled as an edit time.
- No source/runtime changes, builds, fixtures, merges, pushes, releases, or
  new worktrees in this authoring pass. Lead owns design/ready review and IDs.

## Prior Decisions

- 1ab3d5d7 (2026-11-14, commit): "Owner request (617): move avatar+time inline to body first line, drop dedicated middle column." — bearing: constrains
- 4a692a3f (2026-11-08, commit): "Stored the computed marks (block index + kind) rather than the previous body: no second copy of large bodies, and they ride the existing JSON state through restarts." — bearing: constrains
- 1a7e75cf (2026-11-08, commit): "Legacy stores: entries without ChangedAt are stamped with the first update that sees them, so an upgrade does not immediately flag every old topic stale on unknown age; they nudge an hour later." — bearing: constrains
- 261111-feat-ws-whiteboard-topic-recap (2026-11-12, commit): "Relative time: the recap's <time class="tago" data-at> was already picked up by ago.js (full/served page and the dashboard after each board swap)" — bearing: supports

## Route Facts

Fresh fact population is REQUIRED before design/ready review. The following
rows replace the superseded 713-only scope assessment; `needs-refresh` means
no new code-level grounding or mechanism classification is claimed here.
Prior baseline citations in Prior Art remain historical evidence, not proof
that the revised feature is implemented.

| fact | value | evidence |
|---|---|---|
| scope.span | needs-refresh | Revised 743=a includes topic header, latest-delta gutter, shared body-time provenance and relative formatting; prior 713-only asset scope is obsolete. |
| scope.surface | needs-refresh | Shared-store involvement depends on unselected proof design. Do not assert asset-only scope. |
| scope.new_public_symbol | needs-refresh | No API or public symbol selected; prior no-symbol assessment does not establish revised implementation facts. |
| scope.new_type_contract | needs-refresh | Batch-bound proof representation remains unselected; 724=a permits safe old-writer proof loss, not false trust. |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/changes_test.go#L79-L231 covers batch/reload/comment rendering; view_test.go#L257-L297 covers stale marks on both render paths. recap_test.go and stream_test.go cover retained 617-adjacent rendering. Hover geometry still needs the phase's browser evidence; no tests run in this pass. |
| complexity.reuse_points | confirmed | Shared chg.js draws bars and ResizeObserver redraws them (agents-plugin-tool/internal/wswhiteboard/assets/chg.js#L8-L57). Scoped wiring: render.go#L145-L166 bundles it into frame assets; render.go#L403-L427 wires unframed bodies; assets/doc-frame.js#L37 starts framed-body bars. assets/ago.js#L9-L13 supports only now/minutes/hours/days and assets/stream.js#L639-L644 appends an absolute stamp, so neither is a complete marker-time implementation. |
| complexity.side_effect_risk | moderate | Enabling pointer interaction on currently noninteractive absolute gutter bars can affect body interaction and frame geometry (agents-plugin-tool/internal/wswhiteboard/assets/page.css#L553-L559); the shared script reaches framed and unframed bodies. |
| risk.correctness | needs-refresh | Existing ChangedAt includes legacy observation backfill (agents-plugin-tool/internal/wswhiteboard/nudge.go#L26-L46). Revised scope needs actual-batch proof, same header/gutter value, unknown fallback, and stale-proof fail-closed behavior. |
| risk.fit | needs-refresh | Owner 12:28 scope reduction plus 743=a settles one latest BODY time in header and gutter; all-unknown scope-cut assessment is obsolete. Retain colors, anchors, stale dimming and 617; no per-mark history. |
| risk.test | moderate | Existing Go checks pin marks and render structure, not hover thickness, hit testing, no-reflow or scroll offsets at 1280/800/360; browser evidence is deferred under this pass's throttle. |
| risk.security_or_contract | needs-refresh | 713=b exact unknown text, 743=a shared actual body age, and 724=a narrow discardable-proof exception are current authority. Validate proof binding and old/new writers; no representation or global T0 rule change selected. |

## Phases

### Phase 1: One latest body time in header and gutter

After fresh fact population and the lead's design/ready gate, implement one
latest actual BODY-mutation time per topic, displayed distinctly beside header
activity time (`본문 N분 전 갱신`) and from the SAME source/value on latest-delta
gutter hover. Green/blue/red bars thicken slightly on hover without changing
colors, layout, anchors or scroll. Actual ages remain relative-only from
minutes through years; unknown/backfilled/unproven times show exact
`변경 시각 모름` in both displays. Establish prospective actual-batch provenance
with stale-proof fail-closed behavior under 724=a's narrow safe-proof-loss
exception. The proof representation is for technical design review, not
selected here. Do not add per-row/per-mark history or duplicate 617's existing
inline author/time behavior; no history redesign.

Verification boundary: before/after headless captures at 1280/800/360 widths,
including distinct header activity/body times, green/blue/red and end-deletion
marks, multiline content, stale marks, framed/unframed bodies, and retained
inline recap/log placement. Assert identical body age in header and gutter,
relative ages from minutes through years for valid new mutations, exact
unknown text plus hover thickening for missing/backfilled/lost proof, and no
clock/date. Exercise old/new-writer proof loss and stale-proof attempts: loss
may become unknown, never a false age. Check same-body/no-op/return-to-base
and unrelated item/log/recap updates do not renew body age. Preserve colors,
anchors, content geometry and scroll offsets on hover. Run relevant whiteboard
Go tests and independent lite review; send evidence to the lead before MAIN's
serialized develop merge. Live activation and fresh live verification remain
separate from source landing. No implementation or verification run is claimed
by this ticket-authoring update.
