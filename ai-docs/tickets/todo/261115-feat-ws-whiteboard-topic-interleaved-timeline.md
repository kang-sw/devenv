---
title: Whiteboard topic card as one interleaved timeline with windowed loading and owner-input headers
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: prerequisite; consume its landed move map before touching dashboard or whiteboard assets
  261115-feat-ws-whiteboard-viewed-panel-anchor: prerequisite; already in implementation (EXEC2), owns the single scroll writer this ticket's window insertion must use
  261114-feat-whiteboard-center-reference-navigation: coordinate; reference navigation into gaps reuses this ticket's target materialization
  261115-feat-ws-dashboard-user-message-envelope: coordinate; shares the owner send-time source (inbox sentAt) used for answer times and input headers
  261115-feat-ws-dashboard-conversation-rendering-consolidation: coordinate; its board-target navigation rebases onto this ticket's always-present anchors
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
  `choose()` (~136-165) admits rows backwards until a budget is exceeded;
  older pages are cloned from templates on "older" (~302-313, ~445-459) and
  intervening pages are materialized for known targets (~318-342).
- Events (`wswhiteboard/store.go`): "Item N added" system entry with trusted
  `ItemID` (~895); answering stores `Answer`, `AnswerQuote`, `AnsweredAt`
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

## Open Decision Queue
Last ID: none minted (items below await MAIN acknowledgement)

(t1) [open] [technical] Gaps keep today's server-embedded model: `streamView` emits every gap's entries as inert content with a descriptor (count, first/last time, direction set), and the client materializes whole items from the edge nearest the clicked control until at least 1000px is added including the straddling item; loaded items stay (no eviction), over a new network-paged gap endpoint - no new HTTP route, saved-history (static) pages keep working, and the page size is what it is today.
(t2) [open] [technical] Timeline order is append order (entry sequence), the answer row sits at its answered system event, and only the displayed answer time uses the owner send time, over ordering by owner send time - send-time ordering would reorder agent-authored history and break the stable anchors.
(t3) [open] [technical] A question cluster is a maximal run of "Item added" entries with no agent log or panel between them; its anchor is the cluster plus the nearest preceding agent log or panel (system events skipped); a cluster at the very start of the topic has no preceding anchor, over per-question anchors - the owner's spec groups questions as a 뭉치 under one preceding log.
(t4) [open] [technical] Re-ask: the item stays at its first ask position with the "다시 물음 HH:MM" chip from `ReaskedAt`; every answered event gets its own "사용자 답변" row at its position, an earlier answer row rendering from its own system-event text and the latest from the item's current fields, over one answer row that moves - nothing already seen in the timeline changes meaning.
(t5) [open] [technical] Panels keep today's version nesting at the latest version's position, and the "latest three panels start expanded" rule is unchanged except that an unanswered question's rich panel is always expanded, over inlining every panel version chronologically - avoids duplicate panels and keeps the panel contract other tickets rely on.
(t6) [open] [technical] `inputs` are the inbox records drained for this session after the newest already-stamped `inbox_id` in this session's board and up to the commit (inbox ids sort by time), computed server-side at commit from `drained/`; a batch with no new records gets no stamp and continues the current iteration; the drain is the delivery proxy for hook, Pi and hosted delivery alike, over a new per-session cursor in `State` or an agent-supplied argument - no new store struct field, no new tool argument, and the agent cannot misattribute.
(t7) [open] [technical] `excerpt` is the record's free text (parts only, no answer, mention or attachment markup) cut to 240 runes; `chip_ids` lists its answer chip item ids; `sent_at` is the record's `sentAt`; `seq` is one number shared by all entries of a batch, taken from the commit's `State.Seq`, over a new counter - existing monotonic value, no schema change.
(t8) [open] [technical] The owner send time of an answer is stamped as optional `answer_sent_at` on the answered system entry (from the matching chip's `SentAt`), not on `Item`, over an `Item` field - `Entry` already round-trips unknown members, so the T0 claim holds without auditing every `Item` writer.
(t9) [open] [technical] Header placement: in each topic, an owner-message header row precedes the first entry of a stamped batch; it shows the excerpt only if that batch updated this topic with a log, panel or item change, and only the chips whose item lives in this topic; a topic with neither gets no header; unstamped (legacy) batches get no header, over repeating headers per entry.
(t10) [open] [technical] The current-state line below the last anchor is the topic's existing current-content summary line, and "latest batch" is the entries sharing the newest `seq` (or the newest entry when legacy), over a new summary field - reuses existing data.
(t11) [open] [technical] Navigation into a gap (a board reference, a conversation link, a "원래 질문" jump to an answer row that is itself an anchor) materializes the minimum whole-item window containing the target through the existing target path and the viewed-panel ticket's single scroll writer, over a separate scroll path - one scroll owner, as that ticket requires.
(t12) [open] [technical] Compatibility tier T0: optional `inputs`, `seq` and `answer_sent_at` members on `Entry` only, no folder schema bump, no MCP argument change; render-contract changes are dashboard assets shipped with the daemon, over a store struct change - `Entry` preserves unknown members across older writers (`entry_json.go`, historical-writer test `topic_state_history_test.go`).
(t13) [open] [announced] Ordering: asset split first (prerequisite); viewed-panel anchor first (prerequisite, in implementation); envelope, center-reference navigation and conversation consolidation are coordinate edges - each named ticket's own Decisions and the shared files (`stream.js`, `streamView`, `page.tmpl`) give the order.

## Constraints

- No change to how items, logs or panels are written by the agent beyond the
  server-side stamp; no client-side store writes.
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417 (isolated daemons with their own
  `WS_CACHE_HOME` on free ports); end with an owned process scan.

## Phases

### Phase 1: Interleaved timeline, windowed loading and owner-input headers

Implement the Decisions above once the queue settles and the prerequisites
land. Scope: `streamView` anchors and gap descriptors; the bottom question
list folded into the timeline; `stream.js` single 1000px rule; answered and
re-asked rows; server-side `inputs`/`seq`/`answer_sent_at` stamping at
commit; header rows; i18n keys in every `wsi18n` table.

Verification (to be finalized at promotion):
- `wswhiteboard` tests: anchors (cluster + preceding log/panel, system
  events skipped) render outside paging; gap descriptors carry counts and
  ranges; answered rows at both positions; re-ask chip; stamping of inputs,
  seq and answer_sent_at; legacy entries without stamps render with the
  `AnsweredAt` fallback and no header.
- `stream_script_test.go`: each click adds whole items until >=1000px
  including the straddling item; top gap loads up only, middle gaps both
  ways; the old budgets are gone.
- Board-frame tests updated for the new render contract.
- `wsi18n` completeness tests.
- Isolated Chromium capture of a long topic with three question clusters,
  compared side by side with `W-step0-dark.png`, `W-step1-dark.png`,
  `W-step0-expanded-dark.png` and `W-step0-light.png`; the header compared
  with `A-final-dark.png`.
