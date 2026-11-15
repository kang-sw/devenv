---
title: "Add researching as a whiteboard topic work state"
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: "793: rendering phase waits for the asset split"
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: "Existing state, run, query and notification contracts"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5a87a5b89f65d3b3
sage-review-completeness-reviewed: 5a87a5b89f65d3b3
completed: 2026-11-15
---

# Add researching as a whiteboard topic work state

## Background

Research in progress is neither unresolved discussion nor implementation. Give
leads a truthful declared work state without manufacturing an open discussion
ask. This ticket captures owner decision 947, explicitly settled by MAIN at
05:50:31 on 2026-11-15. It is now in `ready/`, with Phase 1 recorded below;
this handoff populates Phase 2 facts only (this ticket's path and Phase 1 Result).
Implementation dispatch remains lead-owned.

## Decisions

- Add the true fifth state `researching`, Korean label `연구 중`, as a peer of
  `discussing`, `waiting`, `implementing` and `awaiting-confirmation`. Declare
  it explicitly through `wb:state`, for example:
  ```xml
  <wb:topic name="investigation"><wb:state value="researching"/></wb:topic>
  ```
  As with existing declarations, the topic must already be known and open.
  This is active investigation; the normal exit is `implementing` when research
  resolves into executable work. It is not an automatic transition or an
  exclusive transition graph. New topics still default to `discussing`.
- `owner` and `eta_minutes` are optional for researching. Do not impose the
  implementing state's mandatory executor/estimate requirement on research.
  Both attributes are informational, not a timed research run (MAIN decision
  963). `wb:eta`, run identity and extensions stay implementing-only; entering
  implementing starts a fresh implementing clock rather than carrying research
  time. Reject timed research runs and research ETA extensions.
- Preserve hourly-notice activation for implementing/waiting only. The hourly
  body continues to list every open topic, including researching. Researching
  gets no ETA-overdue notice, even with an informational estimate. General
  stale behavior remains state-independent (MAIN decision 963).
- The `discussing_without_open_items` warning does not apply to researching.
  Keep the general stale-topic nudge state-independent; do not exempt research,
  change thresholds, or reinterpret open-item/activity criteria. Researching
  does not substitute for an implementing topic in the existing
  `waiting_without_implementing` warning.
- Reject a research marker or badge layered on discussing: alternative 954 was
  withdrawn. The state itself is authoritative; its eventual badge represents
  that state, not an independent research flag.
- **Compatibility: T0, explicitly accepted for the true enum addition.** In the
  supported same-minor deployment, the newest daemon wins (`keepRunning` and
  `CompareVersions`); old plugin text calls that newer daemon, and old playbooks
  never emit researching. Store reads/writes belong to the daemon rather than
  separate old client executables. Dashboard/whiteboard embedded assets ship
  with that same binary (`pages.go` and `render.go`), not an independently
  versioned store reader. Cross-minor `/mcp` hello is rejected (`proxy.go`).
  Authoritative whiteboards are repository-local at
  `<repo>/ai-docs/whiteboards/<ReplyID>/`, not cache-local; only legacy
  `whiteboard.json` lives in the cache/session layout as a rollback snapshot.
  `renderWhiteboard` calls `registerWhiteboardExclude` in
  `internal/mcp/whiteboard_tools.go`, automatically registering
  `/ai-docs/whiteboards/` in the common git directory's local `info/exclude`.
  This shipped behavior covers worktrees but cannot untrack committed boards.
  In this clone that exclusion exists and
  `git ls-files -- ai-docs/whiteboards` lists zero files; neither fact guarantees
  downstream boards are untracked.
- **Accepted cross-machine residual — MAIN 05:53:36.** Downstream projects can
  commit `ai-docs/whiteboards/` and sync machines running different daemon
  versions. An older daemon then reads researching as `malformed`; an older
  writer can declare a later state over it. The owner explicitly accepts this
  visible degradation, not other data loss, and retains T0. An unsupported
  manual downgrade likewise remains malformed until upgrade. Neither residual
  blocks this draft or reopens the withdrawn marker alternative. No architecture
  move or guaranteed-untracked policy is authorized. No version, protocol,
  record-version or store-schema bump is required. Surface any different
  supported-authority contradiction or data-loss behavior to the lead.
- **Shipped documentation audit flag.** The bounded audit found no explicit
  recommendation to Git-track whiteboards. `executor-wrapup.md`'s Doc Commit
  Gate (`agents-plugin/rsrc/executor-wrapup.md#L17-L25`; wsflow/Pi copies also at lines 17–25)
  says to commit all `ai-docs/` changes: that implicitly includes already-tracked
  or unignored boards, not force-adding excluded files. The whiteboard playbook's
  takeover recipe uses “committed folder” for a durable store transaction, not
  Git. Record this residual interaction; changing wrapup conventions is outside
  Phase 1 and requires separate authority.
- Deliver in two sequential slices: Phase 1 changes unfrozen Go, playbook,
  mirror and all 11 locale tables; Phase 2 completes rendering after 793.
  Phase 1 is a partial delivery with a render gap, not completed UI support.
  Embedded assets belonging to one daemon does not eliminate this deliberately
  staged source-level gap. No additional rollout gate or fallback mechanism is
  required; MAIN owns deployment. The split landed at develop `81382b093` and
  its move maps are in
  `ai-docs/tickets/assets/261114-refactor-ws-dashboard-whiteboard-asset-split/`.
- **Phase 2 filter expansion — MAIN 06:29:45, owner request.** Put researching
  immediately after All, before the other state filters. Selecting any state
  filter, including Closed, hides framework card roots as well as nonmatching
  topics. Add an exclusive hammer radio after Closed, labeled `Cards` / `카드`,
  that shows only framework cards and hides topics and closed-history paging.
  Framework scope is preview, terminal, web and explorer card roots; panels,
  logs and history inside a topic follow that topic and are not separate cards.
  The Cards count includes framework cards across normal and pinned regions;
  All retains its existing topic-only count. Preserve the other topic counts,
  deduplication, state/closed matching and unloaded closed remainder semantics.
- **Phase 2 compatibility: T0, client-side view filtering.** Reuse existing
  local per-session filter selection storage and view-only visibility. Older
  clients fall back to All for an unknown filter value; no shared store, API,
  history event, card lifecycle or ownership change is required.
- UI-fast verification has no approved screenshot reference and no screenshot
  or viewport/theme matrix requirement. The owner explicitly permits this radio
  in the existing row without a mockup. Use focused tests plus wsi18n; do not
  import the asset-split ticket's screenshot acceptance into this feature.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Apply `ai-docs/manuals/shipped-surface-boundary.md` to all shipped changes:
  the eventual playbook explains generic behavior, not this repository's owner
  numbers, tickets or scheduling rules.
- Apply `ai-docs/manuals/release-compatibility.md` to Go/store-facing work,
  using the explicitly settled T0 deployment assumptions above.
- Apply `ai-docs/manuals/skill-authoring.md` to the whiteboard playbook and
  `ai-docs/manuals/wsflow-mirroring.md` to its generated mirror and manifests.
- Phase 1 leaves rendering assets and scripts untouched, including the
  post-split `dashboard-*.css/js`, `page-*.css`, `ago.js`, and
  `scripts/board-headless/*.mjs` responsibilities. Renderer and `page.tmpl`
  badge changes belong to Phase 2. The asset freeze itself was lifted when
  the split landed; this remains the ticket's phase boundary.
- No new research provider, research execution machinery, automatic state
  inference or changes to general stale policy are included.
- MAIN owns develop integration and runtime activation. Implementation uses
  offline, scoped checks only: no provider calls, downloads, credentials,
  shared-daemon changes, plugin install/reload or push. Phase 1 is recorded
  below; the next invocation executes Phase 2 only. Lead reads diffs,
  not repeated raw logs; keep worker reports short and in plain English.

## Current-source grounding

Phase 2 pointers below describe the current checkout at `310581f7c` (including
asset split `81382b093` and Phase 1), not later develop or runtime acceptance.
Backend-only rows retain capture context; Phase 1's plan and Result are frozen.
All checks in this population were static reads; no tests, builds, browser,
network probes or runtime operations were run.

| Surface | Current evidence and relevance |
|---|---|
| `internal/wswhiteboard/topic_state.go` | Phase 1 already supplies five states, including `StateResearching`, and top-level informational `TopicState.Owner`/`ETAMinutes` distinct from `Run`; Phase 2 consumes this model rather than adding another enum or clock (`agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L48`). |
| `internal/wswhiteboard/topic_eta.go` | `validExtension` and `applyTopicETA` accept only implementing runs, with cumulative budget and strict extension history. Research attributes are informational and must not reuse implementing runs or extensions. |
| `internal/wswhiteboard/query.go` | Topic metadata's `State *TopicState` carries value, entry and optional run. Query tests pin state/diagnostic output; extend the existing projection rather than inventing a separate marker. |
| `internal/wswhiteboard/board_notice.go` | `eligibleState` admits implementing/waiting for hourly epochs; `planNotice` computes overdue only for implementing, but its hourly body lists every open topic, including each declared state. Trigger eligibility and body inclusion are distinct (`agents-plugin-tool/internal/wswhiteboard/board_notice.go#L37-L39`, `#L141-L161`, `#L197-L209`). |
| `internal/wswhiteboard/nudge.go` | `staleNudge` uses open-topic count, open-item absence and activity age, not declared state. Its local `waiting` map means open asks, not `StateWaiting`. Preserve that distinction. |
| `internal/wswhiteboard/update_summaries.go` | New-topic default producer writes `StateDiscussing`; `summaryState` and net-change summaries project state. Preserve default behavior and cover explicit researching in the same creation batch. |
| `agents-plugin/rsrc/whiteboard/whiteboard.md` | Phase 1 already documents researching and independently optional informational owner/ETA without a run/deadline; no new playbook or mirror change is requested in Phase 2 (`agents-plugin/rsrc/whiteboard/whiteboard.md#L129-L140`, `#L395-L398`). |
| `internal/wsi18n/tables/*.json` | All 11 tables: `en`, `ko`, `ja`, `zh-Hans`, `zh-Hant`, `de`, `es`, `fr`, `it`, `pt-BR`, `ru`. Existing state keys use `js.wb.state.*`, filter labels use `js.board.filter.*`. Supply research state/filter translations in Phase 1; Korean state label is fixed above. |
| `internal/wswhiteboard/assets/page.tmpl`, `ago.js`, `page-items.css` | `topicstate` still renders four states; topic attributes expose owner/ETA only through `.Run`, not researching's top-level metadata. `ago.js` recognizes the same four and requires implementing/run for owner and clock. Badge styling moved to `page-items.css`, not the base `page.css` (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L42-L50`, `agents-plugin-tool/internal/wswhiteboard/assets/ago.js#L31-L75`, `agents-plugin-tool/internal/wswhiteboard/assets/page-items.css#L7-L23`). |
| `internal/wsdashboard/assets/cards.js` and rendering assembly | `filterValues` still enumerates four states plus all/closed. `buildStateFilter` generates both radio groups and keyboard order; unknown stored values fall back to All, and `selectFilter` saves through `opt.key + ".state-filter"` after Overview's callback. `countTopics` currently filters only topics/paging, deduplicates topics across board/pins, and adds unloaded closed remainder; it does not count or filter framework cards (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L75-L78`, `#L183-L260`, `#L288-L330`). Dashboard styling is in `agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.css`; assembly remains `agents-plugin-tool/internal/wsdashboard/pages.go` and `agents-plugin-tool/internal/wswhiteboard/render.go`. |
| Daemon compatibility | `internal/wsdashboard/ensure.go:keepRunning`, `version.go:CompareVersions`/`SameMinor`, `proxy.go` hello validation, and the embed declarations in `pages.go`/`render.go` ground the supported T0 assumptions. `internal/mcp/whiteboard_store.go:whiteboardFolder` locates the repository-local authoritative folder store; legacy cache parts are rollback snapshots. `internal/mcp/whiteboard_tools.go:renderWhiteboard`/`registerWhiteboardExclude` auto-register the local exclusion, which does not untrack boards. This clone has zero tracked whiteboard files; downstream git usage can differ. |

All `internal/` paths above are under `agents-plugin-tool/`.

Phase 2 framework/filter reuse seams (current checkout):

| Claim | Evidence |
|---|---|
| Framework roots have stable card identity and kind; normal/pinned placement shares one registry. Panels/logs/history within topics are not framework roots. | `agents-plugin-tool/internal/wsdashboard/assets/cards.js#L1-L10`, `#L39-L65`, `#L680-L697`; preview, terminal and web register in `assets/preview.js`, `assets/terminal.js`, `assets/webcard.js`; explorer registers in `agents-plugin-tool/internal/wsdashboard/assets/files.js#L1556-L1581`. |
| Existing visibility is an attribute, not DOM destruction or the HTML hidden attribute. | `filtered` writes `data-state-filtered` (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L288-L290`); `[data-state-filtered] { display: none !important; }` is in `agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.css#L195-L196`. |
| Opening/rearranging reaches recounting, but closing an unpinned card currently does not. Phase 2 needs that close seam. | `agents-plugin-tool/internal/wsdashboard/assets/cards.js#L524-L546`, `#L804-L813`, `#L1425-L1433`; close only calls `arrange` when pinned. |
| Hammer is the existing dashboard sprite symbol, not a Lucide name accepted by `licon`. | `agents-plugin-tool/internal/wsdashboard/assets/sprite.svg#L14`; `licon` prefixes `#l-` (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L80`); scoped search `rg -n hammer agents-plugin-tool/internal/wsdashboard/assets/lucide` returned no matches. |
| Existing focused tests cover the actual extension seams, without requiring a new test file or a browser launch during population. | `TestBoardStateFilter` in `agents-plugin-tool/internal/wsdashboard/board_state_filter_test.go#L8-L83`; `TestBoardToolbarPlacementAndCounts` in `agents-plugin-tool/internal/wsdashboard/board_toolbar_test.go#L32-L75`; placement/pin/close fixtures in `agents-plugin-tool/internal/wsdashboard/cards_test.go`; `TestOverviewFilterContracts` and `TestOverviewStateFilterLifecycle` in `agents-plugin-tool/internal/wsdashboard/overview_test.go#L53-L156`; `TestTopicStateBadgeClockAndReconciliation` in `agents-plugin-tool/internal/wswhiteboard/topic_state_badge_test.go`. |
| Overview already excludes filtered nodes and defers filter changes through its existing barrier; preserve that behavior. | `agents-plugin-tool/internal/wsdashboard/assets/overview.js#L38`, `agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L229`; `agents-plugin-tool/internal/wsdashboard/overview_test.go#L94-L156`. |
| All 11 research state/filter translations already landed, but real consumers and the Cards labels remain Phase 2 work. Remove only the two-key staging exemption. | `agents-plugin-tool/internal/wsi18n/tables/{en,ko,ja,zh-Hans,zh-Hant,de,es,fr,it,pt-BR,ru}.json`; English/Korean research keys at lines 375/382; `agents-plugin-tool/internal/wsi18n/keys_test.go#L103-L124`. |

## Prior Decisions

- 261115-feat-ws-whiteboard-topic-state-badge (2026-11-15, Decisions): "Preserve closed-topic lifecycle and existing toolbar filtering. A closed topic is not a new operational state, and closing/resetting must not retain a stale implementing owner/timer." — bearing: constrains
- 261115-feat-ws-whiteboard-toolbar-state-filter (2026-11-15, commit d95fde3d): "Filtering marks nodes without removing or moving them. Overview completes its existing restoration before a user changes the filter; its picker/motion snapshots exclude filtered topics." — bearing: constrains
- 3b775bca (2026-11-15, commit): "Mechanical T0 toolbar seam only: reorder existing values and label keys together, preserving state identities and handlers rather than changing filtering, badge behavior or storage." — bearing: constrains
- 3fadb536 (2026-11-15, commit): "Encode topic keys as hex fragment IDs and let the card owner clear a hiding state filter before scrolling, including asynchronous Overview restoration." — bearing: constrains
- f5b6b502 (2026-11-15, commit): "Reuse the existing wbAgo initializer/refresh/adoption hook and its document-owned timer rather than introduce another asset or mutate dashboard refresh interfaces." — bearing: constrains
- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-15, Decisions): "Explicit work state rather than inference alone." — bearing: supports
- 02a2e572 (2026-11-15, commit): "Trimmed acceptance per (899)=a and the owner UI-fast principle: scoped tests, wsi18n, one lite (clean); owner on-screen check is acceptance." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 2 only: agents-plugin-tool/internal/wswhiteboard/assets/{page.tmpl,ago.js,page-items.css}, internal/wsdashboard/assets/{cards.js,dashboard-board.css}, internal/wsi18n/{keys_test.go,tables/*.json}, affected existing tests. |
| scope.surface | cross-module | Whiteboard server/live badges and dashboard filters consume the landed state; no wb:state, store, API or lifecycle change (agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L42-L50; agents-plugin-tool/internal/wsdashboard/assets/cards.js#L183-L330). |
| scope.new_public_symbol | no | StateResearching already exists; Phase 2 adds presentation/filter values only, not exported commands or model symbols (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L48). |
| scope.new_type_contract | yes | Additive page-data projection of existing informational research owner/ETA is needed: page.tmpl currently emits these only from Run; no shared-store or API contract change (agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L50; topic_state.go#L41-L48). |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/topic_state_badge_test.go, topic_state_view_test.go; internal/wsdashboard/{board_state_filter_test.go,board_toolbar_test.go,cards_test.go,overview_test.go}; internal/wsi18n/keys_test.go. Read only, not executed. |
| complexity.reuse_points | confirmed | Read topicstate, ago.js badge, page-items.css, buildStateFilter/selectFilter/countTopics, Card roots/close, explorer registration and Overview filter fixtures; reuse data-state-filtered and existing per-session opt.load/save. |
| complexity.side_effect_risk | moderate | Visibility affects retained frame/card roots, pinned placement, keyboard targets and Overview geometry; local filter persistence changes but card lifecycle must not (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L288-L330, #L680-L697, #L804-L813). |
| risk.correctness | high | Preserve topic deduplication and unloaded closed remainder while counting framework identities separately; unpinned close currently bypasses arrange/recount (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L291-L311, #L804-L813). |
| risk.fit | moderate | Extend both generated radio groups and existing filter callback, not a second selector or root re-creation; distinguish framework roots from topic internals (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L183-L260, #L680-L697). |
| risk.test | high | Existing fixtures assume six filter options and four badges; extend identity/count/open-close/pin-refresh and server/live parity coverage. Tests have not run; browser/runtime and screenshots are not authorized for this invocation (agents-plugin-tool/internal/wsdashboard/board_state_filter_test.go#L8-L83; internal/wswhiteboard/topic_state_badge_test.go). |
| risk.security_or_contract | moderate | Informational owner text must remain escaped, never become a run/clock, and unknown stored filters must retain All fallback; T0 local selection only (agents-plugin-tool/internal/wswhiteboard/assets/ago.js#L50-L75; assets/page.tmpl#L42-L50; internal/wsdashboard/assets/cards.js#L188-L189). |

## Phases

### Phase 1: Add the declared state on unfrozen backend and text surfaces

Add researching to enum acceptance, persisted state parsing/folding/history, command
validation, query projections and net-change summaries. Keep explicit same-batch
creation overrides and the new-topic discussing default intact. Cover the
owner/ETA seams according to the confirmed contract, preserving implementing
start/extension/close semantics. Make warnings and timed notices reflect the
settled scopes without changing general stale behavior.

Update the canonical whiteboard playbook's work-state obligation, grammar,
query and notification explanations; synchronize the wsflow and Pi rsrc copies
(`agents-plugin-{wsflow,pi}/rsrc/whiteboard/whiteboard.md`) and their manifests
through the documented mirroring procedure. Add the state and filter
translations in all 11 locales. Keep `TestEveryEnglishKeyIsUsed` strict: a
Phase 1 staging allowance may name only the exact new research state/filter
keys whose consumers arrive in Phase 2, with a removal obligation in Phase 2.
Do not add dummy consumers or weaken the general unused-key guard. Honor the
frozen rendering boundary above;
report the partial delivery and remaining UI gap explicitly. Optional research
owner/ETA must survive replay, queries and net-change summaries without a run;
`wb:eta` remains invalid for researching, transitioning to implementing starts
a new clock, and hourly bodies include researching without new trigger or
overdue eligibility.

Verification boundary for the future worker (not run during ticket capture):

- Extend `topic_state_test.go`, `topic_state_query_test.go`,
  `topic_state_view_test.go` and `topic_state_history_test.go` for acceptance,
  optional attributes, atomic invalid batches, repeat declarations, transitions,
  diagnostics, close/reopen and raw history retention under the confirmed policy.
- Extend `topic_eta_test.go`, `board_notice_test.go`, `nudge_test.go` and
  `update_summaries_test.go` at their affected seams: retain implementing clocks,
  warning behavior, general stale thresholds and default discussing output.
- Run relevant Go package tests in `internal/wswhiteboard`, affected MCP
  whiteboard tests, `internal/wsi18n`, and `internal/wsrsrc`; include manifest
  drift, `TestWsflowRsrcMirrorUpToDate` and `TestPiMirrorUpToDate`. Verify the
  staged-key allowance is confined to the exact new research state/filter keys.
  Run
  `python3 -m unittest discover agents-plugin-wsflow/tests` for the mirror.
  Record exact commands/results when execution is authorized. Inspect the diff
  to prove frozen assets were not changed; tests reading old badges are not
  evidence of completed researching UI.

### Result (b17c08bb3) - 2026-11-15

- Phase 1 landed in `824f0ec1e` (text/locales) and `b17c08bb3`
  (backend/contracts), based on the handed-off `40a2f44e4` tree including
  asset split `81382b093`. This is partial backend/text delivery, not complete
  researching UI support. Phase 2 remains for the next handoff; no rendering
  assets, scripts or frozen render fixtures changed.
- Added `StateResearching` with independently optional top-level `owner` and
  `eta_minutes` in immutable records and derived state/query projections.
  These reuse existing text/numeric validation without creating a run or
  deadline. Identical declarations are no-ops; changed informational attributes
  append a state event, and omission clears them. Net summaries compare these
  fields, including metadata-only changes and net cancellation. This follows
  declarative replacement rather than inventing research execution machinery.
- Explicit same-batch research overrides preserve the discussing default
  without intermediate default history. Close/reopen and adoption preserve
  lifecycle/history semantics. Exiting implementing retains its completed run
  in history; explicitly entering implementing after research starts a fresh
  clock. Research ETA extensions and timed research records are rejected.
- Existing warning, stale and notice implementations already have the settled
  scopes; regression coverage pins researching without an open ask, not
  substituting for implementing, state-independent stale thresholds/activity,
  no hourly/overdue activation and inclusion in eligible hourly bodies.
- Updated the canonical playbook and generated wsflow/Pi copies/manifests.
  Added state/filter labels in all 11 locales, Korean `연구 중`. The strict
  unused-key test permits exactly `js.wb.state.researching` and
  `js.board.filter.researching` temporarily; Phase 2 must remove that allowance
  when real consumers land. The documented T0 skew residual and wrapup/Git
  interaction remain accepted and unchanged.
- Verification used offline Go environment `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local`. Worker checks also used isolated
  `WS_CACHE_HOME=/tmp/ws-researching-phase1-worker/cache` and
  `WS_CONFIG_HOME=/tmp/ws-researching-phase1-worker/config`:
  - `go test -p=1 ./internal/wswhiteboard -count=1 -timeout=240s -run
    '^(TestTopicState|TestTopicETA|TestTopicUpdate|TestBoardNotice|TestStaleNudge)'`
    passed (5.018s), including raw replay and actual historical-writer checks.
  - `go test -p=1 ./internal/mcp -count=1 -timeout=240s -run '^TestWhiteboard'`
    passed (62.240s).
  - Text leaf verified `go test ./internal/wsi18n -count=1`;
    `go test ./internal/wsrsrc -count=1 -run
    'Test(ShippedManifestUpToDate|WsflowRsrcMirrorUpToDate|PiMirrorUpToDate)$'`;
    `go test ./internal/mcp -count=1 -run
    '^TestWhiteboardPlaybookStreamGuidanceProductModes$'`; and
    `python3 -m unittest discover agents-plugin-wsflow/tests` (24 tests,
    2 skipped). All passed; generator diffs contained only expected resources.
  - `git diff --check` passed; the range's path list confirms the Phase 1
    rendering boundary. Full worker outputs were read in
    `/tmp/ws-researching-phase1-worker/{model-scoped,mcp-whiteboard,whiteboard}.log`.
- The full `go test -p=1 ./internal/wswhiteboard -count=1 -timeout=240s`
  run was **not green**: it had only the six historical reds documented by
  `e33f77521` and `2de77962f` on this captured baseline: five stale rendering
  fixtures (`TestServedBoardFramesEveryBodySeparately`,
  `TestDashboardTopicBodyUsesWholeCardFold`, `TestRenderPageTopicBodyPlacement`,
  `TestRenderPageContentOnlyTopic`, `TestRenderPageEmbedOmitsCurrentBody`) and
  `TestRenderPageEmbedSizeBudget`. Embed measured 247276 bytes versus the
  246415-byte threshold (historical 247192 plus 84 locale-key bytes). No added
  backend failures were observed. These are not claims about later develop
  `be9673139`; no baseline synchronization or render-fixture repair was done.
- Independent one-pass lite review was clean over
  `40a2f44e4..b17c08bb3`; no fixes or re-review were needed. Findings artifact:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2530e43b/review-paths/10c95de2-01-researching-phase1-lite.md`.
  No providers, downloads, credentials, browser/runtime/harness execution,
  shared-daemon changes, plugin installation/reload, push or merge occurred.

### Phase 2: Complete badge and dashboard rendering after 793

Depends on Phase 1's landed Result and
`261114-refactor-ws-dashboard-whiteboard-asset-split` landing with its move map.
This is a phase-specific prerequisite, not a whole-ticket `blocked-by` gate
that would incorrectly block the unfrozen Phase 1.

Extend the server `topicstate` template, live badge reconciliation (`ago.js`
responsibility), badge styling (`page.css` responsibility), dashboard cards,
state filter labels/selection and affected rendering/overview surfaces for
researching. Add the confirmed Cards radio and framework-card visibility/count
behavior in the same filter row, with researching immediately after All and
Cards after Closed. Reuse the existing filter generator, visibility attribute,
card registry and topic/pinned projection rather than inventing a second filter
or changing card lifecycle. Recount when cards close as well as when they open
or move. Supply the Cards label in all 11 locales. Use actual post-split
asset/assembly locations, including dashboard CSS/JS responsibilities only as
needed. Render `Researching` / `연구 중` as the
fifth peer state with the existing localized badge/filter pattern; owner/ETA
presentation follows the separately confirmed metadata contract. Remove the
Phase 1 unused-translation-key staging allowance when those real consumers
land, and verify `TestEveryEnglishKeyIsUsed` without that allowance. Do not add a
second research marker or infer researching from topic content.

Verification boundary: extend `topic_state_badge_test.go` and
`internal/wsdashboard/board_state_filter_test.go`, plus existing affected
render/view tests, to cover initial/server rendering and live state changes,
research filter inclusion, transition away, close/reset, localized labels and
owner/ETA projection. Extend the existing filter fixtures with actual framework
card roots in normal and pinned regions: state filters hide them, Cards hides
all topics and closed paging, All restores the current view without changing
its topic-only count. Cover card open/close, pin/unpin, refresh, retained card
identity and contents, inline/compact radio parity, keyboard navigation,
persisted selection and unknown-value fallback. Filtering must not destroy or
recreate cards or deduplicate them by path/content. Preserve Overview's existing
filter callback/deferred work behavior.

Run focused whiteboard/dashboard Go tests, full wsi18n and syntax checks for
changed post-split scripts. Verify the hammer references the existing
`#i-hammer` sprite, not a nonexistent `#l-hammer` icon. UI-fast requires no
mockup, screenshot or viewport/theme matrix. The existing browser filter test
and `scripts/board-headless.mjs` may ground a later bounded check only if
separately authorized; this invocation grants no browser, runtime or harness run.

### Result (32dec7255) - 2026-11-15

- Phase 2 landed in `94406db99` (badges/locales) and `32dec7255`
  (dashboard filters). Researching is a localized peer badge with escaped,
  independently optional owner and static estimate. The estimate uses
  `js.wb.state.estimate`, never an elapsed/deadline clock or overdue style;
  state transitions and close/reset remove stale metadata. Phase 1 plan and
  Result bytes remain unchanged.
- The shared filter row now orders All, Researching, Discussing, Waiting,
  Implementing, Awaiting confirmation, Closed, Cards. Both radio groups and
  the compact trigger use the existing `#i-hammer` for Cards. Registry-based
  card counting preserves identity across normal/pinned regions and counts
  equal-path cards separately; All retains its topic-only total and unloaded
  closed remainder. State filters hide framework roots, Cards hides topics
  and closed paging, and All restores view-only visibility without rebuilding
  frames. Unpinned closes recount directly; pin/open/refresh retain existing
  placement/lifecycle and Overview callback ownership.
- All 11 locales include Cards and the static estimate. Removed exactly the
  two Phase 1 research-key staging exceptions; full wsi18n passes with real
  consumers and incoming user-role key removals preserved.
- Lead-owned synchronization reached `dfa0fec19`, incorporating exact develop
  `667f0f64a`, the final right-side single-line answer-chip/full substantive
  disclosure refinement, and history-init batching. Interaction inspection
  and the combined focused checks below passed. `stream.js`, `overview.js`
  and `dashboard-board.js` are byte-identical to `667f0f64a`; P2 does not change
  fit/init/keep, add per-row layout reads, or alter timeline markup.
- Final verification at `dfa0fec19` used offline `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local` and isolated `WS_CACHE_HOME=/tmp/ws-research-p2/cache`,
  `WS_CONFIG_HOME=/tmp/ws-research-p2/config`, from `agents-plugin-tool/`:
  - `go test -p=1 ./internal/wswhiteboard -count=1 -timeout=240s -run
    '^(TestTopicState|TestHistory|TestViewKit|TestCheckRowsStageAnswersInTheReplyBar|TestPageSourceAssembly|TestRenderPageDocumentShape|TestRenderPageTopicBodyPlacement|TestRenderPageContentOnlyTopic|TestRenderPageEmbedOmitsCurrentBody|TestRenderPageFollowsViewLanguage|TestDashboardTopicBodyUsesWholeCardFold|TestServedBoardFramesEveryBodySeparately)'`
    passed (4.152s), including chip disclosures and batched history admission.
  - `go test -p=1 ./internal/wsdashboard -count=1 -timeout=240s -run
    '^(TestBoard(StateFilter|CardPlacement|Toolbar|Topic|PinnedClosedPagesCount|Sheet)|TestOverview(PureContracts|FilterContracts|StateFilterLifecycle)$|TestToolbar|TestPinnedColumnWidth|TestSharedCardActionSnapshots|TestFilesToolbarSheet|TestFullscreenPanelWidth|TestConversationPresentation)'`
    passed (12.310s), including research/Cards filtering and retained roots.
  - `go test -p=1 ./internal/wsi18n -count=1 -timeout=240s` passed (1.073s).
    `node --check` passed for `cards.js`, `ago.js` and incoming `stream.js`;
    `git diff --check` passed. Full command outputs were read.
- Earlier post-e8 dashboard checking encountered the unchanged
  `TestOverviewPageIntegration` source-string failure: it expects
  `maximize(c)` while both captured `c70f7f550` and current `cards.js` defer
  `maximize(c, floating)`. Exact baseline source/test inspection established
  attribution; no test was changed to bless it and it was not rerun.
  Full suites were not claimed green; other lead-reported baseline failures
  were outside the selected scope and were not executed.
- Independent one-pass lite review was clean over
  `e8f5500c0..56ed15c91`; findings artifact:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@34840370/review-paths/55163db1-01-researching-phase2-lite.md`.
  The lead accepted retaining this review after its serialized baseline merges;
  no repeat review was run. Browser geometry remains unverified: UI-fast
  authorizes focused tests, not browser/screenshots/matrix/runtime acceptance.
  No provider/download/credential, daemon, install/reload, push or worker-owned
  merge operation occurred; develop integration and activation remain lead-owned.
