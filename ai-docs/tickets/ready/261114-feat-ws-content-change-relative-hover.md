---
title: Latest body update time in topic header and change gutter
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: fa3b36bdc68a3c08
sage-review-completeness-reviewed: fa3b36bdc68a3c08
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
  not permission to infer trust from existing ChangedAt. Lead technical
  settlement selects the portable TopicDoc-local proof contract below,
  auto-confirming the fact-populator's one technical gap without a new owner
  question or global ID. No schema/protocol bump, new MCP API, migration,
  sidecar, or transient authority is selected.
- Retain existing 617 recap/log inline-time behavior rather than implement it
  twice. Repair it only if current behavioral verification finds it missing:
  author mark and time (owner example `04:08 · 3시간 전`) lead the body inline,
  leaving multiline text the available width rather than a middle time column.

### Selected portable body-time proof

Add optional `TopicDoc.BodyTimeProof`, JSON `body_time_proof,omitempty`:

```json
{"v":1,"at":"2026-11-14T13:00:00Z","changed":123,"digest":"<sha256>"}
```

- Validate locally against the current TopicDoc: supported version 1, nonempty
  body, valid UTC RFC3339 `at` equal to current `ChangedAt`, positive `changed`
  equal to current body `Changed`, and matching digest. Missing, malformed,
  unsupported-version or mismatched evidence yields unknown, never a guessed
  age. Keep existing ChangedAt legacy-backfill semantics unchanged.
- Digest is lowercase SHA256 of deterministic JSON
  `["wb-body-time-v1", changed, at, exactBody, orderedMarkPairs]`; mark pairs
  are `[block, kind]` in stored order, with nil/empty marks normalized to `[]`.
  Do not normalize body bytes. Time alone permits same-second collisions;
  body/marks alone permits separate-batch edit-away/edit-back. Validate all
  components together, with Changed as the existing batch discriminator.
- Mint only after the final actual net body mutation and timestamp/marks of
  an accepted Apply batch are established. A backfill-only observation,
  same-body/no-op/empty/unrelated update must never mint proof. Those paths
  preserve original proof (including absence); same-batch return-to-base
  restores original proof with original body metadata. Clear removes proof;
  a real new nonempty-body mutation creates fresh proof, even without marks.
- Adoption/rename, relocation, and whole-board copy/restoration preserve the
  originating body's time/proof when the body is unchanged; they are not new
  edits. Exclude topic name, labels and board/root paths from the digest.
  Reject absolute-path/topic scope because it adds portability loss and
  needless board-context plumbing; reject sidecars because old writers would
  not erase their independent evidence with typed topic metadata.
- Adoption plus edit-return in one batch must compare with its logical
  batch-start body, not lose the origin on rename. Carry this comparison
  origin only within the batch; no new persisted identity, lease or API.
- Body-neutral mark changes must not mint age: their digest mismatch becomes
  unknown. Unsupported proof versions stay unknown on no-op/load/render;
  only a real new body mutation replaces proof with supported v1 evidence.
- Body and proof enter the SAME folder intent and durable metadata publication.
  Pending replay retains their exact committed bytes; callback success alone
  must not publish a trusted age for an uncommitted body.
- A TopicDoc-local validator produces ONE optional validated TopicView body
  timestamp for the header and both framed/unframed gutter paths. Thread no
  board identity/context for validation. Frames receive the same validated
  timestamp/unknown state with isolated formatter/i18n support, not parent
  access or sandbox expansion; do not expose proof as UI authority.
- This checks accidental stale evidence under supported writers, not
  cryptographic authenticity. Captured proof from a different batch fails
  Changed/digest checks; deliberately recomputed self-consistent forgery or
  full consistent rollback is not detectable or promised. Public digests
  are not signatures. No MAC or external identity service is authorized.

## Prior Art

VERIFIED at refreshed fact-population baseline: this branch `feat/content-change-hover-677`
points to `74170cf6e256e5db1a1c2bdefb3ffd1210cd032f`; local develop points to
`ec23205525e240bf3d3da2cf36ea3fd4642b2211`. Since the shared product baseline
`9854ff350fa1b0eccfb5c71649c5190c405925b9`, this branch changes only this ticket;
develop also changes dashboard selection-note code/tests and i18n tables, not
whiteboard source (`git diff --name-only 9854ff350 HEAD` and
`git diff --name-only 9854ff350 develop`). There is still no whiteboard source
diff between them (`git diff --name-only HEAD develop --
agents-plugin-tool/internal/wswhiteboard` returned nothing; unchanged markers:
`agents-plugin-tool/internal/wswhiteboard/assets/chg.js#L8-L49`,
`agents-plugin-tool/internal/wswhiteboard/assets/page.css#L553-L559`).

- `agents-plugin-tool/internal/wswhiteboard/assets/chg.js`, search
  `wbChangeBars`: green `new`, blue `mod`, red `del` (including `delafter`)
  are generated from body comment markers into a zero-height absolute layer.
  `assets/page.css`, search `.wb-chg-bar`: 2px vertical bars, 8px by 2px
  deletion ticks, `pointer-events: none`; no hover behavior there.
- `changes.go`, search `type BlockMark`: persisted markers contain only block
  index `b` and kind `k`. `store.go`, search `setTopicBody` and `TopicDoc`:
  marks represent the latest body-edit batch, with body-specific `ChangedAt`;
  same-body/no-op and same-batch return-to-base edits preserve existing body
  marks and time; a missing legacy time can still be backfilled by the accepted
  update, including an empty command batch (`agents-plugin-tool/internal/wswhiteboard/store.go#L442-L466`,
  `agents-plugin-tool/internal/wswhiteboard/store.go#L580-L593`,
  `agents-plugin-tool/internal/wswhiteboard/nudge.go#L26-L46`). Unrelated
  entry/recap changes do not replace body marks or a nonempty body time. This
  is not a retained per-block history.
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

## Remaining readiness: design review and execution evidence

For a known actual mutation batch, existing `ChangedAt` is its actual edit
time and covers all its current marks; a separate timestamp per block is not
inherently required by that latest-batch model. However, `BlockMark` stores
no timestamp or provenance, and `ChangedAt` alone cannot distinguish actual
mutation from legacy backfill. The inspected representation does not prove
actual historical body-edit times. The folder-store trace found immutable
system entries for body operations, but no general history binding the current body batch to those times: entries
carry text and time, not a body hash or body Changed; intermediate edits emit
entries even when the batch restores its starting body and time
(`agents-plugin-tool/internal/wswhiteboard/entries.go#L25-L36`,
`agents-plugin-tool/internal/wswhiteboard/entries.go#L61-L71`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L450-L466`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L1078-L1151`). One value per topic
removes per-mark attribution, not this origin ambiguity. Owner answer 713=b
settles unknown display; 743=a requires the same trusted body-time value in
both the header and latest-delta gutter.

`agents-plugin-tool/internal/wswhiteboard/folders.go`, search `folderDecode`,
`folderJSON`, and `topic.Doc = &copy`: the schema-2 historical writers
`972766abc` and `e30578dd4`, and this baseline, decode typed TopicDoc values,
clone State through typed JSON, and reconstruct every topic's metadata on
publication; an unknown optional TopicDoc proof field is lost on a subsequent
published update, even an unrelated accepted Apply batch, not preserved stale
(`git show 972766abc:agents-plugin-tool/internal/wswhiteboard/folders.go`,
`git show e30578dd4:agents-plugin-tool/internal/wswhiteboard/folders.go`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L350-L360`,
`agents-plugin-tool/internal/wswhiteboard/folders.go#L202-L214`,
`agents-plugin-tool/internal/wswhiteboard/folders.go#L619-L664`). Read-only open
or an unchanged folder callback need not publish, while pending-intent replay
publishes its stored exact topic bytes rather than a new mutation
(`agents-plugin-tool/internal/wswhiteboard/folders.go#L133-L165`,
`agents-plugin-tool/internal/wswhiteboard/folders.go#L729-L747`). No inspected
historical writer preserves an unknown TopicDoc proof across a different body
batch; a sidecar or blind-preservation alternative would have a different
stale-proof threat. Additive syntax alone does not establish general T0
preservation; 724=a specifically permits safe loss of this UX proof. Existing
Seq/Generation values do not prove timestamp origin; Changed is nevertheless
an existing batch discriminator, unlike RFC3339 time, which has only second
precision (`agents-plugin-tool/internal/wswhiteboard/store.go#L450-L466`,
`agents-plugin-tool/internal/wswhiteboard/store.go#L580-L593`,
`agents-plugin-tool/internal/wswhiteboard/nudge.go#L31-L41`). No existing
persistent provenance path has been established, and process-only witnessing
is not a selected authority or retention model.

The lead has settled the formerly open representation as the portable
TopicDoc-local proof above. Remaining work is design/completeness review and
implementation fixture evidence, not an unselected mechanism or owner policy
gap. An old writer dropping proof may produce unknown under 724=a; captured
stale proof must not authenticate a different body batch. Neither observation
time, topic-latest time, timestamp coincidence, nor invented historical values
are edit evidence. Static historical-writer traces are not execution evidence;
old/new-writer fixtures and the adoption-origin integration still need to run.
No implementation or known-age delivery is claimed by this settlement.

## Constraints

- Future tooling edits follow `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md` per AGENTS.md path coverage.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Header and gutter must share one body-specific source and value; unrelated
  item/log/recap activity must not renew body age. No per-row or per-mark
  history, and no existing ChangedAt value is trusted solely for being present.
- Preserve portable proof on unchanged-body adoption/copy/relocation; do not
  add absolute-path/topic scope, board-aware validation handoffs or transient
  trust. Same-batch origin tracking for adoption is internal and batch-local.
- Proof/body publication must be atomic under the existing folder intent;
  validate once into TopicView and retain opaque-origin frame isolation.
- Preserve current framed-body and unframed-body behavior and marker geometry;
  do not replace source provenance with topic recency, filesystem mtime, or
  first-observation time labeled as an edit time.
- No source/runtime changes, builds, fixtures, merges, pushes, releases, or
  new worktrees in this authoring pass. Lead owns design/ready review and IDs.

## Prior Decisions

- 6bf4df29 (2026-11-14, commit): "Remove the dedicated log timestamp column, keeping role badges beside the first line and placing timestamps inline before body text." — bearing: constrains
- a18cc24a (2026-11-14, commit): "Keep the renderer as owner of DOM shape so incoming and retained frame-graft paths agree; a client-only unwrap would force mismatched element paths on every update." — bearing: constrains
- 261113-feat-ws-whiteboard-board-frontend-performance (2026-11-14, commit): "Only the dashboard's framed render (RenderRegions, RenderClosedTopics) uses framedDoc, so no disk page needs the files written beside it." — bearing: constrains
- 261113-feat-ws-dashboard-board-reorder-hold (2026-11-14, commit): "Nearest scroller = first ancestor with overflow-y auto/scroll/overlay whose content overflows, else document.scrollingElement" — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, commit): "Commit exact bytes and append offsets under one folder lock; replay interrupted writes idempotently before any consumer reads authority." — bearing: constrains
- 417596db (2026-11-12, commit): "Stale rule: any item of the topic with Item.Changed above TopicDoc.Changed; a recap change does not count. Time-based fading was rejected by the user." — bearing: constrains
- 4a692a3f (2026-11-08, commit): "Stored the computed marks (block index + kind) rather than the previous body: no second copy of large bodies, and they ride the existing JSON state through restarts." — bearing: constrains
- 1a7e75cf (2026-11-08, commit): "Legacy stores: entries without ChangedAt are stamped with the first update that sees them, so an upgrade does not immediately flag every old topic stale on unknown age; they nudge an hour later." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, nudge.go, folders.go, view.go, render.go, assets/page.tmpl, assets/chg.js, assets/page.css and assets/ago.js are the grounded store/view/header/gutter/formatter sites; selected proof is optional TopicDoc-local v1 metadata, with batch-local adoption-origin handling in topics.go/store.go. No implementation is claimed. |
| scope.surface | public-interface | Distinct topic-header body age and gutter hover are user-visible; selected optional persistent proof touches the shared schema-2 store (agents-plugin-tool/internal/wswhiteboard/folders.go#L27-L64). No new MCP command is selected. |
| scope.new_public_symbol | yes | Selected optional TopicDoc.BodyTimeProof and one validated TopicView body-time value extend exported internal-package structs; no new MCP command/API or board-context handoff. |
| scope.new_type_contract | yes | Phase 1 requires a new actual-batch proof/validation contract and one shared body-time view value; selected representation is portable body_time_proof {v,at,changed,digest}, locally validated; existing TopicDoc lacks provenance (agents-plugin-tool/internal/wswhiteboard/store.go#L208-L239). |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/changes_test.go#L79-L235 covers no-op/batch/reload/render marks; view_test.go#L257-L297 covers both stale render paths; recap_test.go and stream_test.go cover retained 617-adjacent rendering, not hover geometry or proof. No tests run. |
| complexity.reuse_points | confirmed | agents-plugin-tool/internal/wswhiteboard/store.go#L442-L466 already compares against batch-start body; folders.go#L133-L165 owns locked commit/publication; assets/chg.js#L8-L57 draws/redraws bars; render.go#L145-L166 and render.go#L403-L427 wire frame/unframed scripts; assets/doc-frame.js#L37 starts framed bars. assets/ago.js#L9-L13 needs longer units, not replacement with stream.js's absolute-time suffix. |
| complexity.side_effect_risk | moderate | Shared frame/unframed gutter interaction, a new header value and selected optional persisted proof affect hit testing, frame heights and store round-trips (agents-plugin-tool/internal/wswhiteboard/assets/page.css#L553-L562, render.go#L604-L628, folders.go#L619-L664). |
| risk.correctness | high | ChangedAt backfills observation time; same-second mutations defeat time-only binding, and separate-batch edit-away/edit-back defeats body-only binding (agents-plugin-tool/internal/wswhiteboard/nudge.go#L26-L46, store.go#L450-L466, store.go#L580-L593). Selected proof must survive no-op/return-to-base without authenticating another batch; adoption plus edit-return needs batch-local logical-origin comparison (topics.go#L201-L231, store.go#L431-L466). |
| risk.fit | moderate | Header currently shows overall activity via stampUpdated, including items/entries; new body time must remain distinct and not become card ordering's first activity time (agents-plugin-tool/internal/wswhiteboard/view.go#L779-L820, assets/page.tmpl#L25-L48; agents-plugin-tool/internal/wsdashboard/assets/cards.js#L151-L157). Preserve 617, stale dimming and anchors. |
| risk.test | high | Existing checks lack batch-proof old/new-writer fixtures and header/gutter equality, months/years, hover hit testing, no-reflow and scroll/focus measurements at 1280/800/360. Framed documents have their own i18n subset and no parent formatter access (agents-plugin-tool/internal/wswhiteboard/render.go#L610-L617). |
| risk.security_or_contract | high | 724=a allows only safe proof loss, not false age or a global T0 relaxation. Typed historical writers drop the optional proof on publication. Portable local validation detects captured stale proof, not recomputed self-consistent forgery/full rollback; path scopes, sidecars and MACs are rejected. Framed payloads must retain opaque-origin isolation (agents-plugin-tool/internal/wswhiteboard/folders.go#L202-L214, folders.go#L619-L664, render.go#L631-L641). |

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
exception, using the selected optional v1 body_time_proof contract. Place
local digest creation/validation beside TopicDoc/change logic; mint only at
the final actual-mutation stamp, restore proof on return-to-base and clear it
with body metadata. Preserve origin on adoption without board-context plumbing,
and publish proof/body in the same durable intent. Feed one validated TopicView
body-time value to the header and both gutter render paths, with frame-local
formatter/i18n and no parent access. Do not add per-row/per-mark history or duplicate 617's existing
inline author/time behavior; no history redesign.

Verification boundary: before/after headless captures at 1280/800/360 widths,
including distinct header activity/body times, green/blue/red and end-deletion
marks, multiline content, stale marks, framed/unframed bodies, and retained
inline recap/log placement. Assert identical body age in header and gutter,
relative ages from minutes through years for valid new mutations, exact
unknown text plus hover thickening for missing/backfilled/lost proof, and no
clock/date. Exercise old/new-writer proof loss and stale-proof attempts: loss
may become unknown, never a false age. Check same-body/no-op/return-to-base
and unrelated item/log/recap updates do not renew body age. Include empty
batches backfilling legacy time without proof creation, same-second distinct
mutations and separate-batch edit-away/edit-back, clear/recreate, body-neutral
mark changes, missing/malformed/unsupported/mismatched proof, unchanged-body
adoption/relocation/board copy, and adoption plus edit-return in one batch.
Assert proof/body atomic publication and replay, and rejection of captured
stale evidence; do not demand detection of recomputed self-consistent forgery
or full consistent rollback. Preserve colors,
anchors, content geometry and scroll offsets on hover. Run relevant whiteboard
Go tests and independent lite review; send evidence to the lead before MAIN's
serialized develop merge. Live activation and fresh live verification remain
separate from source landing. No implementation or verification run is claimed
by this ticket-authoring update.
