---
title: Whiteboard topic card as one interleaved timeline with windowed loading and owner-input headers
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: prerequisite; consume its landed move map before touching dashboard or whiteboard assets
  261115-feat-ws-whiteboard-viewed-panel-anchor: prerequisite; its per-scroller scroll owner (landed as b57dff0be, reverted in a1b1a5468 for a dashboard freeze, re-lands after EXEC2's perf fix) is the one writer this ticket's window insertion and navigation use
  261114-feat-whiteboard-center-reference-navigation: coordinate; reference navigation into gaps reuses this ticket's target materialization
  261115-feat-ws-dashboard-user-message-envelope: coordinate; shares the owner send-time source (inbox sentAt) used for answer times and input headers
  261115-feat-ws-dashboard-conversation-rendering-consolidation: coordinate; its board-target navigation rebases onto this ticket's always-present anchors
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0a88e882326c5b4a
sage-review-completeness-reviewed: 0a88e882326c5b4a
---

# Whiteboard topic card as one interleaved timeline with windowed loading and owner-input headers

## Background

Owner request (relayed by MAIN, 2026-11-15, board topic
`log-question-link`): an agent usually reports state in a log line and asks
questions in the same update, but the topic card shows logs in the stream and
questions in a separate list at the card bottom, so the reader has to
re-find which report a question came from.

- **(895)=d**, owner's words: "질문 시점하고 응답 시점을 각각 카드에
  기록하는 식으로, 그래서 질문 카드랑 로그, 패널 등 모든 항목들이
  ws-message를 1 iter로 interleave되어서 보이게 만드는 방향을 고민해보고
  싶어요." (Record ask and answer times on the cards and interleave every
  item - question cards, logs, panels - with one owner message exchange as
  one iteration.)
- **Windowing spec**, owner's words (topic note on `log-question-link`):

  > 그러면 지금 긴 메시지에서 자르기 로직이 문제가 될텐데, 반드시 '각 질문의
  > 바로 위' 로그는 보이게 하고, 그래서 아주 긴 로그 사이사이에 질문이 낀
  > 경우에는:
  >
  > ... N개 로그 항목 <위쪽 더 불러오기>
  > * 질문 뭉치 앞의 바로 앞 로그|패널
  > * 답변 안 된 질문 A, with rich panel
  > * 답변 된 질문 B [펼치기 버튼]: 원래 질문 (클릭하면 답변 위치로 이동)
  > ... K개 로그 항목 <위쪽 | 아래쪽 더 불러오기>
  > * 질문 뭉치 앞의 바로 앞 로그|패널
  > * 답변 안 된 질문 C, with rich panel
  > ... M개 로그 항목 <위쪽 | 아래쪽 더 불러오기>
  > * B에 대한 사용자 답변 [펼치기 버튼]: 원래 질문 + 사용자 답변 및 에이전트 코멘트
  >
  > 자르기 규칙도 단순화하자는 제안입니다. 그냥 무조건, 1000px 분량을 더
  > 불러오고, 1000px에 걸친 아이템까지 포함해서 (즉 항상 1000px이상)
  > 불러오는 식.

- **(897)=b**, owner's words: "네 좋은 아이디어라고 봅니다. 목업도
  승인합니다. 좋아 보여요." Option b: add the A-style owner-message header
  to W; when the agent updates the board it records the owner message it
  received just before (send time, excerpt, answer chips) in an optional
  field (additive only); free text shows as a header only in topics the
  agent updated from that message, answer chips only in their question's
  topic; the answer time becomes the real send time. Rejected: a (W alone,
  no headers), c (free text repeated in every topic updated in the batch).

**Approved visual reference** (owner, (897)): mockup W at
[`assets/261115-feat-ws-whiteboard-topic-interleaved-timeline/windowed.html`](../assets/261115-feat-ws-whiteboard-topic-interleaved-timeline/windowed.html)
(open locally; it loads `page.css` and `mock-common.css` beside it), with
captures `W-step0-dark.png` (initial view), `W-step1-dark.png` (after one
"load below" on a middle gap), `W-step2-dark.png`, `W-step0-expanded-dark.png`
(answered question expanded), `W-step0-light.png` (light theme), and
`A-final-dark.png` plus `mockup.html` for the owner-message header style.

Today (develop `ddf95a78e`, paths under `agents-plugin-tool/internal/`):
- `wswhiteboard/view.go` ~628-659 builds item groups separately from the
  stream (`streamView`); `assets/page.tmpl` ~46 orders a topic as checks,
  legacy body, stream, then the bottom `.tbody` where policy questions,
  acknowledgments and settled items render (~70-124).
- `wswhiteboard/stream_view.go` (~10-261): `HistoryWindow = 50`; the newest
  50 entries render, older pages ship as inert `<template>` elements in the
  page (not network paging); adjacent system events fold into chips; earlier
  panel versions nest under the latest panel; only the latest three panels
  start expanded.
- `wswhiteboard/assets/stream.js` ~28 `LIMIT = 1000, PANELS = 3, LINES = 30`:
  `choose()` (~136-165) admits rows backwards but stops before the next row
  would exceed any budget, except that the first row always joins
  (`agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L136-L159`);
  older pages are cloned from templates on "older" (~302-313, ~445-459) and
  intervening pages are materialized for known targets (~318-342).
- Events (`wswhiteboard/store.go`): "Item {{N}} added [class]: title" system entry with trusted
  `ItemID` (`agents-plugin-tool/internal/wswhiteboard/store.go#L895`); answering stores `Answer`, `AnswerQuote`, `AnsweredAt`
  (agent apply time) and emits an answered system entry (~939-949); a re-ask
  clears the answer and sets `ReaskedAt`, emitting a generic patched entry
  (~1093-1104). Entries carry board-wide `eN` IDs and `At`, no batch
  identity; `State.Seq` advances per accepted batch and on implicit renders
  (~560-610). `Entry` preserves unknown JSON members (`entry_json.go`).
- Commit: `mcp/whiteboard_tools.go` ~226-284 -> `FolderStore.UpdateAndPublishObserved`
  (`wswhiteboard/folders.go` ~149-176, append-only topic `log.jsonl`, folder
  schema 2). Inbox drain keeps the last 50 drained records under `drained/`
  (`wsinbox/inbox.go` ~95-97, ~912-1004) with id, `sentAt`, parts and chips
  (chip `ID`, `SentAt`, ~161-173).

## Decisions

- **(895)=d, (897)=b and the windowing spec** as quoted in Background, with
  the approved mockup W as the visual contract.
- **MAIN defaults (announced on the board, accepted):** answered question
  clusters also keep their preceding log always visible; an answered
  question stays at its ask position as a collapsed row; the load amount is
  a fixed 1000px regardless of card size; the "preceding" item skips system
  events and is the nearest agent log or panel; unanswered questions are
  never truncated however many; below the last anchor only the latest batch
  and the current-state line show; a re-asked question stays at its first
  ask position with a "다시 물음 HH:MM" chip.
- **MAIN contract (relayed 2026-11-15):** the single 1000px rule replaces
  `stream.js` `LIMIT`/`PANELS`/`LINES`; ask position and time come from the
  "Item added" event (`ItemID` + `At`), answer position from the answered
  event; `streamView` ships anchors outside the 50-entry paging plus gap
  descriptors (board-frame tests updated); (897)=b is an optional T0
  `inputs` stamp `[{inbox_id, sent_at, excerpt, chip_ids}]` plus optional
  `seq` on a batch's entries at commit, with the excerpt copied because the
  drained inbox keeps only 50 records; an owner-message header starts an
  iteration (a batch preceded by a delivery); answer time shown is the owner
  send time, falling back to `AnsweredAt` for legacy entries. Tier T0.
- **Technical calls (acknowledged by MAIN 2026-11-15):**
  - **Gap model.** Gaps keep today's server-embedded model: `streamView`
    emits every gap's entries as inert content with a descriptor (count,
    first/last time, direction set), and the client materializes whole items
    from the edge nearest the clicked control until at least 1000px is added
    including the straddling item; loaded items stay (no eviction). Rejected:
    a network-paged gap endpoint - it needs a new HTTP route and breaks
    saved-history (static) pages.
  - **Order.** Timeline order is append order (entry sequence); the answer row
    sits at its answered system event and only the displayed answer time uses
    the owner send time. Rejected: ordering by owner send time - it reorders
    agent-authored history and breaks stable anchors.
  - **Cluster anchor.** A question cluster is a maximal run of "Item added"
    entries with no agent log or panel between them; its anchor is the
    cluster plus the nearest preceding agent log or panel (system events
    skipped); a cluster at the very start of a topic has no preceding anchor.
    Rejected: per-question anchors - the owner's spec groups questions as a
    뭉치 under one preceding log.
  - **Re-ask and answer rows.** A re-asked item stays at its first ask position
    with the "다시 물음 HH:MM" chip from `ReaskedAt`; every answered event gets
    its own "사용자 답변" row at its position, an earlier row rendering from
    its own system-event text (value/quote) and the latest from the item's
    current fields. The collapsed "원래 질문" row renders the item's current
    title and options; no historical question snapshots are stored, and title
    or option patches stay visible as their own system chips. Rejected: one
    answer row that moves (changes meaning of what was already seen);
    persisted question snapshots per ask (new storage for a display detail).
  - **Items without timeline events.** Items that have no "Item added" entry
    (folder conversion deliberately synthesizes none:
    `wswhiteboard/folders.go` ~326-343, kept by `TestFolderConversionAndRestart`)
    render at the head of the topic timeline, before the first entry, in item
    number order, with their full controls and stable item anchors and no ask
    time; their answered state shows on the item row itself unless an
    answered system entry exists, which then gets its answer row as usual. No
    historical timestamp or event is invented. Rejected: dropping them with
    the bottom list (loses retained questions); synthesizing events at
    conversion (fabricates history).
  - **Panels.** Panels keep today's version nesting at the latest version's
    position, and the "latest three panels start expanded" rule is unchanged
    except that an unanswered question's rich panel is always expanded.
    Rejected: inlining every panel version chronologically - duplicate panels
    and a changed panel contract other tickets rely on.
  - **Inputs attribution.** At commit the server computes `inputs` from this
    session's `drained/` records. A candidate is a drained record not already
    referenced by any `inputs` stamp on this session's board (an id set, so
    steering's rewritten ids and steer-before-follow-up drain order cannot
    double-credit a record) and whose `inbox_id` sorts after the board's
    `inputs_floor`. The floor is established once: the first batch with
    entries committed on a board that carries no stamp yet credits nothing
    and is stamped `inputs: []` plus `inputs_floor: <newest inbox_id present
    in drained/ at that commit>` (absent when `drained/` is empty, meaning no
    floor); it is persisted in the log, so it survives restarts, and no batch
    commit time is needed. A record queued with an id older than the floor
    and drained later is not credited (a one-time miss at activation,
    accepted). Records whose only content is the answer-check nudge are
    excluded, recognized by a predicate kept beside the nudge text generator
    (`ack_check.go`) so the two cannot drift. A batch without entries is not
    stamped; records it consumed are credited to the next batch with entries,
    which is the agent's visible response. A batch with entries but no
    candidate gets no `inputs` member and continues the current iteration.
    Drain is the delivery proxy for hook, Pi and hosted delivery alike; a
    hosted record held behind a slash-command barrier or a failed send is
    credited to the first commit after its drain (a display-only early
    credit, accepted). A record aged out of the 50-record retention before
    commit is not credited: no header beats a wrong header. Rejected: a
    newest-`inbox_id` high-water cursor alone (skips or misorders under
    steering); a first-stamp floor from the previous batch's commit time
    (eventless batches advance `Seq` without entries, so no durable commit
    time exists; folder metadata records generations, not times); a
    per-session cursor field in `State` or an agent-supplied tool argument
    (new store field or new argument; the agent could misattribute); a
    delivery-ack ledger in `wsagent` (a new cross-module store for a header).
  - **Stamp fields.** `excerpt` is the record's free text cut to 240 runes,
    extracted from `Parts` by stripping serialized selection, mention and
    comment groups and image markers with the existing `wsinbox` parsers
    (`selections.go`); malformed groups and literal markup stay as text and
    are escaped at render (template auto-escape, never raw HTML). `chip_ids`
    lists the record's answer chip item ids; `sent_at` is the record's
    `sentAt`; `seq` is one number shared by all entries of a batch, taken
    from the commit's `State.Seq`. Rejected: joining `Parts` verbatim
    (leaks serialized markup); a new counter (existing monotonic value
    suffices).
  - **Answer send time.** The owner send time of an answer is stamped as
    optional `answer_sent_at` on the answered system entry, not on `Item`,
    taken from the newest-`SentAt` chip for that item among this commit's
    candidate records; with no matching chip there, no member is written and
    the display falls back to `AnsweredAt`. Rejected: an `Item` field (needs
    auditing every `Item` writer; `Entry` already round-trips unknown
    members); oldest-chip or first-match selection (the applied answer is the
    latest one sent).
  - **Header placement.** In each topic, an owner-message header row precedes
    the first entry of a stamped batch; it shows the excerpt only if that
    batch updated this topic with a log, panel or item change, and only the
    chips whose item lives in this topic; a topic with neither gets no
    header; unstamped (legacy) batches get no header. Rejected: a header per
    entry.
  - **Current-state line.** The line below the last anchor is the latest agent
    log entry `streamView` marks `EntryView.Current` (rendered as
    `.wb-entry.agent.current` with the `wb.stream.current` tag); "latest
    batch" is the entries sharing the newest `seq` (the newest entry for
    legacy). Rejected: a new summary field.
  - **Navigation into gaps.** A board reference, a conversation link or a
    "원래 질문" jump into a gap materializes the minimum whole-item window
    containing the target through the existing target path and the landed
    per-scroller scroll owner of 261115-feat-ws-whiteboard-viewed-panel-anchor
    (prerequisite; must be re-landed first); a gap click or jump is owner input and applies
    immediately under that ticket's deferral rule, while live updates keep its
    deferred visible-rendering behavior. Rejected: a separate scroll path.
  - **Tier T0.** Optional `inputs`, `inputs_floor`, `seq` and `answer_sent_at` members on
    `Entry` only, no folder schema bump, no MCP argument change; render
    changes ship as dashboard assets with the daemon. The current `Entry`
    codec round-trips unknown members (`wswhiteboard/entry_json.go`);
    historical schema-2 folder writers preserve committed log bytes and
    opaque pending append bytes, while an old typed legacy-state conversion
    can drop unknown `Entry` metadata (`topic_state_history_test.go`) - a
    dropped stamp only loses a header or falls back to `AnsweredAt`.
    Rejected: a store struct change.
  - **Ordering.** Asset split first (prerequisite, consume its move map);
    viewed-panel anchor (prerequisite; reverted in `a1b1a5468`, re-lands after its perf fix); envelope,
    center-reference navigation and conversation consolidation are coordinate
    edges over the shared files (`stream.js`, `streamView`, `page.tmpl`).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- No change to how items, logs or panels are written by the agent beyond the
  server-side stamp; no client-side store writes.
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417 (isolated daemons with their own
  `WS_CACHE_HOME` on free ports); end with an owned process scan.

## Prior Decisions

- 184e0755 (2026-11-15, commit): "Remove fixed/minimum recap height but retain the shared four-line ellipsis clamp and matching maximum height in both sticky states." — bearing: constrains
- a9e2f3d8 (2026-11-15, commit): "the dashboard routes in-board fragment links through its own bridge, and the chip needs no new navigation path." — bearing: constrains
- c801da2b (2026-11-15, commit): "Ordering is a stable partition (queueRecords) over stamp order, not a new on-disk field." — bearing: constrains
- 24e88c1e (2026-11-15, commit): "Owner decision (833)=b: one unified chunk rule replaces the separate panel-count path and the log-only thirty-line path." — bearing: supports
- 33364d39 (2026-11-15, commit): "joinBoundary reduces to merging a system chip split by a page boundary, which is exactly what the old non-run branch did." — bearing: constrains
- 05096e96 (2026-11-15, commit): "Real archived historical schema-2 source remains unchanged rather than substituting current model types." — bearing: constrains
- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-14, Decisions): "One owner-facing ask. Anything unfinished and not being worked keeps an open ask, at most one per topic. Close only on the owner's final answer." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/{view.go,stream_view.go,store.go,entry_json.go,folders.go,assets/page.tmpl,assets/stream.js}; internal/mcp/whiteboard_tools.go, internal/wsinbox/inbox.go and internal/wsi18n/tables under the same package root. Approved references exist under ai-docs/tickets/assets/261115-feat-ws-whiteboard-topic-interleaved-timeline/. |
| scope.surface | cross-module | Server-side inbox-to-board commit stamps cross wsinbox, mcp and wswhiteboard; shared stream markup serves standalone and dashboard consumers. No new MCP argument or HTTP route is proposed. |
| scope.new_public_symbol | no | No new exported symbol, tool, argument or route is named; proposed Entry member names are covered by the new type contract below. |
| scope.new_type_contract | yes | Optional persisted Entry inputs [{inbox_id,sent_at,excerpt,chip_ids}], inputs_floor, seq and answer_sent_at; gap descriptors and always-present anchors change the renderer/client contract. |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/stream_test.go, stream_script_test.go and topic_state_history_test.go; internal/wsdashboard/board_frame_reuse_test.go and whiteboard_bridge_test.go; internal/wsi18n/keys_test.go. Static inspection only; no tests or browsers run. |
| complexity.reuse_points | confirmed | Inert full-history shipment exists: agents-plugin-tool/internal/wswhiteboard/stream_view.go#L243-L251 and assets/page.tmpl#L57-L57; stream_test.go#L102-L134 pins all pages. Existing loadOne/materialize/loadTarget in assets/stream.js#L302-L361 validate and admit known targets. New gap descriptors/window selection remain implementation work; the viewed-panel scroll owner is reverted (a1b1a5468) pending its re-land. |
| complexity.side_effect_risk | high | Commit-time input inference reads a separately drained, 50-record retained inbox; steering and hosted queued input can make filename high-water attribution skip or prematurely credit messages (agents-plugin-tool/internal/wsinbox/inbox.go#L834-L846, agents-plugin-tool/internal/wsagent/runner.go#L1841-L1919). |
| risk.correctness | high | Anchors must survive paging, re-asks and panel version nesting while mutable Item fields cannot reconstruct all historical question versions; answer events are EntrySystem with trusted ItemID (agents-plugin-tool/internal/wswhiteboard/entries.go#L66-L81, store.go#L939-L949). |
| risk.fit | high | t6's cursor/delivery premise needs settlement and t10 refers to the latest agent stream row, not body/state metadata; asset split is ready but unlanded; the viewed-panel scroll owner was reverted in a1b1a5468 and re-lands after a perf fix. |
| risk.test | high | Fake-DOM height budgets and frame identity tests pin the current shape; >=1000px straddling admission, multiple gaps, late frame heights, input attribution and approved-reference comparisons need new cases, not just updated source strings. |
| risk.security_or_contract | high | Owner-authored headers/answer times must not misattribute machine input or execute excerpt markup. Current Entry JSON preserves unknown fields, and answered events are Entries (agents-plugin-tool/internal/wswhiteboard/entry_json.go#L10-L44, entries.go#L66-L81, store.go#L939-L949); old folder-byte preservation does not establish old typed-legacy decoding compatibility (topic_state_history_test.go#L303-L329 in that directory). |

## Phases

### Phase 1: Interleaved timeline, windowed loading and owner-input headers

Implement the Decisions above once the asset split and the viewed-panel anchor land. Scope: `streamView`
anchors and gap descriptors; the bottom question list folded into the
timeline; `stream.js` single 1000px rule replacing `LIMIT`/`PANELS`/`LINES`;
answered and re-asked rows; server-side `inputs`/`seq`/`answer_sent_at`
stamping at commit; header rows; i18n keys in every `wsi18n` table.

Verification:
- `wswhiteboard` tests: anchors (cluster + preceding log/panel, system
  events skipped) render outside paging; gap descriptors carry counts and
  ranges; answered rows at both positions; re-ask chip; stamping of inputs
  (id-set exclusion, steer-before-follow-up order, first-stamp `inputs: []` +
  `inputs_floor` and its survival across a restart, eventless batch credited to
  the next batch, nudge exclusion, aged-out record), seq and answer_sent_at (newest chip, no-match
  fallback); excerpt extraction strips selection/mention/image markup and
  escapes literal markup; legacy entries without stamps render with the
  `AnsweredAt` fallback and no header; converted items without "Item added"
  entries render at the timeline head with controls and anchors intact
  (conversion + render regression).
- `stream_script_test.go`: each click adds whole items until >=1000px
  including the straddling item; top gap loads up only, middle gaps both
  ways; the old budgets are gone; a gap jump goes through the scroll owner.
- Board-frame tests updated for the new render contract (`-run` only).
- `wsi18n` completeness tests.
- Acceptance (owner UI principle, relayed by MAIN 2026-11-15): the owner's
  own on-screen check after merge and daemon swap, against mockup W as the
  reference (`windowed.html`, `W-step0-dark.png`, `W-step1-dark.png`,
  `W-step0-expanded-dark.png`, `W-step0-light.png`; header style
  `A-final-dark.png`). W is the reference for the look, not a pixel gate; no
  screenshot matrix or capture-comparison round is required of the worker.

### Result (15e44e235) - 2026-11-15

- VERIFIED: Phase 1 now renders a single event-ordered timeline with permanent
  question/answer anchors, preceding reports, re-ask clocks, converted-item
  controls, current state and latest-batch visibility. Other entries remain in
  inert gaps; directional loading includes the item crossing 1000px and never
  evicts admitted rows. Panel versions and the dependency clock remain intact.
- VERIFIED: Commit-time attribution writes only optional Entry metadata (T0),
  including activation/floor persistence, id-set exclusion, parser-cleaned
  excerpts and newest matching answer-chip times. Existing folder schema and
  MCP/HTTP surfaces are unchanged; archived schema-2 append and pending-recovery
  regressions retain the added metadata.
- Decisions: retain the existing complete EntryPage projection for panel/frame
  preparation and derive the anchor/gap projection over all its chunks. A gap
  target loads forward from itself, then backward from its nearest prefix if
  needed to fill the window. Check-pill folding/lifting retains the real controls
  at individual timeline positions with spacers. All insertion/navigation uses
  the landed per-scroller owner; its implementation remains unchanged.
- VERIFIED verification: full `go test ./internal/wswhiteboard -count=1` has
  exactly the six pre-edit baseline failures and no additional failing tests.
  Focused `TestTimeline.*` and `TestHistoryScript.*` pass. Full `wsinbox` and
  `wsi18n` packages pass. Named MCP input/stream/publication/conversion tests and
  dashboard board-frame/history/bridge/nudge-predicate tests pass. The named CSS
  collision test retains its four existing collisions. JavaScript syntax checks
  pass; the Node asset-split suite passes four tests and skips the opt-in pin.
- VERIFIED: populated embed size is 256,914 bytes versus 255,787 before the
  change (+1,127 bytes); its existing budget failure remains unresolved.
- Unresolved: the baseline failures/collisions remain outside this feature;
  independent lite review is lead-owned, and visual fit is INFERRED pending the
  owner's post-merge W/A reference check. The informational lease acquisition was
  refused because the existing develop track holds it; no override was used.
- Omitted by owner override: child agents, worker independent review, browsers,
  screenshots, daemon launches, full-dashboard testing, merge and push. Work
  remains on `exec4/timeline-impl`; the ticket is deliberately left in `ready/`
  without closure or movement.

#### Edition (e875aa671) - 2026-11-15

Owner amendment (owner-decided, relayed by the main lead; no open decision):
CHECK items interleave in the timeline too, rendered the same way as
questions. A check item sits at its own event position as a permanent anchor
row in the question-row form (ID, title, status chip; for example the owner's
"(964) held · ok" row), not only as a list lifted under the sticky check pill.
The pill's fold/lift of the real controls stays. Scope and contracts are
otherwise unchanged: no schema, MCP, or HTTP change.

#### Edition (3f4844582) - 2026-11-15

Result: check-interleave amendment implemented.

- VERIFIED: the prior implementation already anchored check add events
  (`timeline_view.go:80-81,141-179`) but rendered only check lists
  (`assets/page.tmpl:65` in `15e44e235`). Checks now reuse the question
  `srow`/`stext` summary and `pick` chip, with existing check verdict/reading
  and status tokens; no new CSS or i18n keys. Embeds are unchanged.
- VERIFIED: permanent `item-N` anchors stay at the add position while the pill
  folds/lifts the real `check-N` controls. Assumed notes remain owned by those
  controls; dashboard selection retains item identity even while lifted.
- VERIFIED: offline full whiteboard tests retain exactly the same six baseline
  failures, with no new failure. Focused timeline/check tests, full i18n, named
  dashboard selection/check/bridge/history tests and JavaScript syntax pass.
  Embed size remains 256,914 bytes (delta 0).
- Unresolved: the six baseline failures, including the embed budget, remain;
  visual acceptance is INFERRED pending the owner's post-merge check. No child
  agent, browser, daemon, merge, push or ticket closure was performed.

#### Edition (04cbdde8d) - 2026-11-15

Lead amendment at the develop sync (technical; landed behavior wins): items
without timeline events render at the head in the topic's existing stored
item order, not item-number order. TASK3's unified whole-history panel
(46af75a71) landed that order with a regression (`[9, 2]` stays `[9, 2]`).
The timeline adopts it instead of reordering a landed, owner-checked view.
The rest of the "Items without timeline events" decision is unchanged.
Rejected: item-number order (would break the landed TASK3 regression and
reorder what owners already see).

#### Edition (e36b9f5b9) - 2026-11-15

Develop sync: `63dd7e46a` integrates TASK3's whole-history frame; `e36b9f5b9`
syncs the later develop tip `aa07206c0` (including its folder cache).

- VERIFIED: permanent ask/check anchors and inert directional gaps now layer
  over TASK3's complete transition projection, single-line answer chips,
  complete disclosures, retirement evidence and unknown-time rows. Coalesced
  creation events stay unchanged on disk; their members retain their ask
  positions even after class patches. Eventless items keep stored order,
  including the landed `[9, 2]` regression.
- VERIFIED: open/assumed checks keep one real control/correction owner at their
  permanent ask positions for pill folding/lifting; completed checks retain
  the history visibility toggle. Ask and settlement disclosures have distinct
  keys. Initial layout shares one capture per scroller, and all navigation
  retains the landed keep/jump/place owner and pN panel-fold/update policy.
  Owner headers survive panel republishing; answer-chip clocks and hover
  metadata use owner send time. The T0 stamp contract is unchanged.
- VERIFIED: refreshed develop baseline and merged full whiteboard runs fail
  only `TestRenderPageEmbedSizeBudget`: 256,534 -> 257,941 bytes (+1,407).
  No additional failure remains from the branch's six-failure baseline. Full
  i18n, named board-frame/whiteboard-bridge/wbmention/timeline-bridge and MCP
  input-attribution tests, four Node asset-split tests (one opt-in skip), and
  assembled/standalone JavaScript syntax checks pass.
- Unresolved: the existing embed-budget failure; visual fit remains INFERRED
  pending the owner's check. No browser, daemon, child agent, push, merge into
  develop, ticket-plan edit, or ticket closure was performed.

#### Edition (1b8e95cae) - 2026-11-15

- Restored board search over inert timeline gaps while retaining legacy
  history-page indexing and the existing history-owner reveal/jump path.
- Secondary coalesced and eventless question articles now have distinct,
  stable, namespaced render keys, so Viewed deferral compares the exposed
  question. The first question retains its event navigation anchor. T0:
  display-only changes; stored data is unchanged.
- VERIFIED: regression tests reproduce both issues and pass after the fixes.
  Focused dashboard search, Viewed, frame, history and timeline tests and
  whiteboard timeline/gap tests pass. Full whiteboard testing retains only
  the known embed-budget failure; embed size is unchanged. No browser,
  daemon, child agent, merge or push was used.
