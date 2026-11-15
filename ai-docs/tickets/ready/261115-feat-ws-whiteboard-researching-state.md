---
title: "Add researching as a whiteboard topic work state"
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: "793: rendering phase waits for the asset split"
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: "Existing state, run, query and notification contracts"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5a87a5b89f65d3b3
sage-review-completeness-reviewed: 5a87a5b89f65d3b3
---

# Add researching as a whiteboard topic work state

## Background

Research in progress is neither unresolved discussion nor implementation. Give
leads a truthful declared work state without manufacturing an open discussion
ask. This ticket captures owner decision 947, explicitly settled by MAIN at
05:50:31 on 2026-11-15. It remains accepted backlog in `todo/`; fact population,
design/completeness review, promotion and implementation dispatch remain
lead-owned.

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
- UI-fast verification has no approved screenshot reference and no screenshot
  or viewport/theme matrix requirement. Do not import the asset-split ticket's
  screenshot acceptance into this feature.

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
  shared-daemon changes, plugin install/reload or push. Execute Phase 1 only
  in this invocation; retain Phase 2 for the next handoff. Lead reads diffs,
  not repeated raw logs; keep worker reports short and in plain English.

## Current-source grounding

Pointers below describe the capture baseline, not implementation results or
ready-route facts. Search by the named symbols after 793 moves assets.

| Surface | Current evidence and relevance |
|---|---|
| `internal/wswhiteboard/topic_state.go` | `topicStateValues` contains four states; `applyTopicState` requires owner/ETA for implementing and rejects them otherwise. `parseTopicState`, `validRun`, `foldStateEvent` and close/reset history assume only implementing has a run; unknown enum values are malformed. `structuralWarnings` scopes the no-open-items warning to discussing. |
| `internal/wswhiteboard/topic_eta.go` | `validExtension` and `applyTopicETA` accept only implementing runs, with cumulative budget and strict extension history. Research attributes are informational and must not reuse implementing runs or extensions. |
| `internal/wswhiteboard/query.go` | Topic metadata's `State *TopicState` carries value, entry and optional run. Query tests pin state/diagnostic output; extend the existing projection rather than inventing a separate marker. |
| `internal/wswhiteboard/board_notice.go` | `eligibleState` admits implementing/waiting for hourly epochs; `planNotice` computes overdue only for implementing, but its hourly body lists every open topic, including each declared state. Trigger eligibility and body inclusion are distinct (`agents-plugin-tool/internal/wswhiteboard/board_notice.go#L37-L39`, `#L141-L161`, `#L197-L209`). |
| `internal/wswhiteboard/nudge.go` | `staleNudge` uses open-topic count, open-item absence and activity age, not declared state. Its local `waiting` map means open asks, not `StateWaiting`. Preserve that distinction. |
| `internal/wswhiteboard/update_summaries.go` | New-topic default producer writes `StateDiscussing`; `summaryState` and net-change summaries project state. Preserve default behavior and cover explicit researching in the same creation batch. |
| `agents-plugin/rsrc/whiteboard/whiteboard.md` | Work-state obligations, `wb:state`/`wb:eta` grammar, query metadata and timed-nudge sections currently describe four states and implementing-only runs. Update together to describe informational research attributes and unchanged implementing-only clocks. Mirror is `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`; both rsrc manifests participate. |
| `internal/wsi18n/tables/*.json` | All 11 tables: `en`, `ko`, `ja`, `zh-Hans`, `zh-Hant`, `de`, `es`, `fr`, `it`, `pt-BR`, `ru`. Existing state keys use `js.wb.state.*`, filter labels use `js.board.filter.*`. Supply research state/filter translations in Phase 1; Korean state label is fixed above. |
| `internal/wswhiteboard/assets/page.tmpl`, `ago.js`, `page.css` | `topicstate` renders a four-way badge; `ago.js` maps those states and currently shows owner only for implementing with run identity. Extend server rendering and live reconciliation consistently in Phase 2; use post-split stylesheet/script locations. |
| `internal/wsdashboard/assets/cards.js` and rendering assembly | `filterValues` enumerates four states plus all/closed. Phase 2 includes research filtering, card state propagation, overview/render consumers and any required dashboard styling. `internal/wsdashboard/pages.go`, `internal/wswhiteboard/render.go` embed/assemble the assets; follow the 793 move map rather than assuming old paths survive. |
| Daemon compatibility | `internal/wsdashboard/ensure.go:keepRunning`, `version.go:CompareVersions`/`SameMinor`, `proxy.go` hello validation, and the embed declarations in `pages.go`/`render.go` ground the supported T0 assumptions. `internal/mcp/whiteboard_store.go:whiteboardFolder` locates the repository-local authoritative folder store; legacy cache parts are rollback snapshots. `internal/mcp/whiteboard_tools.go:renderWhiteboard`/`registerWhiteboardExclude` auto-register the local exclusion, which does not untrack boards. This clone has zero tracked whiteboard files; downstream git usage can differ. |

All `internal/` paths above are under `agents-plugin-tool/`.

## Prior Decisions

- 261115-feat-ws-whiteboard-topic-default-and-update-summaries (2026-11-15, commit 218176cc): "The playbook describes authoritative creation defaults, same-batch explicit override without intermediate default, no legacy/reopen backfill" — bearing: constrains
- 261115-feat-ws-whiteboard-topic-state-badge (2026-11-15, Decisions): "Preserve closed-topic lifecycle and existing toolbar filtering. A closed topic is not a new operational state, and closing/resetting must not retain a stale implementing owner/timer." — bearing: constrains
- 261115-feat-ws-whiteboard-toolbar-state-filter (2026-11-15, Result (01a121f70)): "Counts deduplicate board/pin topics and retain loaded closed history plus data-closed-more." — bearing: constrains
- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-15, Result (0203cff95)): "Scoped cumulative wb:eta preserves original start, run identity, owner and initial estimate" — bearing: constrains
- 733c7522 (2026-11-15, commit): "Fold all retained histories in numeric board-global order; recognize only the inspected non-imported unattributed historical system close producer, not authored text." — bearing: constrains
- dd03b1d0 (2026-11-15, commit): "Phase 3 uses immutable existing-kind system metadata for cumulative ETA revisions and reply-id-scoped notice receipts; retain original start/initial estimate/owner and every reason." — bearing: constrains
- 77229e91 (2026-11-15, commit): "Keep render-only calls on a cheap summary path; do not clone or compare the board again when no topic was touched." — bearing: constrains
- 02a2e572 (2026-11-15, commit): "Trimmed acceptance per (899)=a and the owner UI-fast principle: scoped tests, wsi18n, one lite (clean); owner on-screen check is acceptance." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/{topic_state.go,topic_eta.go,query.go,board_notice.go,update_summaries.go}, internal/wsi18n/tables/*.json, shared playbook/mirrors; Phase 2 rendering/assets and dashboard consumers. |
| scope.surface | public-interface | wb:state accepts a new value; derived query state and immutable history gain researching semantics (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L44, #L177-L259; query.go#L41-L43). |
| scope.new_public_symbol | yes | New public wb:state value researching; existing exported state constants are in agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L25. Exact Go constant name remains implementation-owned. |
| scope.new_type_contract | yes | Fifth enum value plus optional informational research owner/eta_minutes; MAIN963 settles no research run, implementing-only wb:eta and a fresh implementing clock. Existing TopicState/TopicRun carry only implementing run metadata (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L27-L44). |
| scope.test_surface | existing | Named topic_state*_test.go, topic_eta_test.go, board_notice_test.go, nudge_test.go, update_summaries_test.go exist under agents-plugin-tool/internal/wswhiteboard/; internal/wsdashboard/board_state_filter_test.go and internal/wsrsrc/wsflow_mirror_test.go exist. No tests executed during population. |
| complexity.reuse_points | confirmed | Read TopicStates/parseTopicState/applyTopicState, applyTopicETA, summaryState, eligibleState/planNotice, staleNudge, query projection, topicstate, ago.js badge and cards.js filterValues. Canonical playbook and wsflow/Pi mirror guards are existing reuse seams. |
| complexity.side_effect_risk | high | Persisted enum and metadata affect replay, summaries, warnings and notice epochs; phased rendering deliberately leaves a backend/UI gap. agents-plugin-tool/internal/wswhiteboard/topic_state.go#L104-L132; board_notice.go#L37-L67. |
| risk.correctness | high | Optional research metadata must survive folding/queries without becoming an implementing run; preserve repeat/transition/close/default semantics and distinguish hourly trigger from all-open-topic body (agents-plugin-tool/internal/wswhiteboard/board_notice.go#L141-L209). |
| risk.fit | moderate | Reuse immutable Entry authority and existing state consumers; current non-implementing fold drops Run and summaryState compares only Value/Run/Diagnostic, so informational metadata needs coordinated projection rather than a second flag (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L127-L132; update_summaries.go#L160-L172). Pi resource byte identity also applies (internal/wsrsrc/pi_mirror_test.go#L20-L80). |
| risk.test | high | Existing four-state coverage must expand across optional metadata, raw-history retention, clocks/notices, locale key-use guards and server/live/filter parity; Phase 2 follows the move map landed at develop 81382b093, under ai-docs/tickets/assets/261114-refactor-ws-dashboard-whiteboard-asset-split/. No screenshot/matrix requirement is authorized. |
| risk.security_or_contract | moderate | Public enum/metadata and T0 accepted skew residual must preserve raw history and escaped owner rendering; no wider data loss is accepted. Current parser rejects unknown values as malformed and permits a later valid baseline (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L242-L259, #L286-L300); server/client owner rendering is text (assets/page.tmpl#L42-L50; assets/ago.js#L50-L64). |

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

### Phase 2: Complete badge and dashboard rendering after 793

Depends on Phase 1's landed Result and
`261114-refactor-ws-dashboard-whiteboard-asset-split` landing with its move map.
This is a phase-specific prerequisite, not a whole-ticket `blocked-by` gate
that would incorrectly block the unfrozen Phase 1.

Extend the server `topicstate` template, live badge reconciliation (`ago.js`
responsibility), badge styling (`page.css` responsibility), dashboard cards,
state filter labels/selection and affected rendering/overview surfaces for
researching. Use actual post-split asset/assembly locations, including dashboard
CSS/JS responsibilities only as needed. Render `Researching` / `연구 중` as the
fifth peer state with the existing localized badge/filter pattern; owner/ETA
presentation follows the separately confirmed metadata contract. Remove the
Phase 1 unused-translation-key staging allowance when those real consumers
land, and verify `TestEveryEnglishKeyIsUsed` without that allowance. Do not add a
second research marker or infer researching from topic content.

Verification boundary: extend `topic_state_badge_test.go` and
`internal/wsdashboard/board_state_filter_test.go`, plus existing affected
render/view tests, to cover initial/server rendering and live state changes,
research filter inclusion, transition away, close/reset, localized labels and
owner/ETA projection. Run the focused whiteboard/dashboard Go tests and syntax
checks for changed scripts using the post-split paths. UI-fast has no approved
screenshot and requires no screenshot/matrix. The existing browser filter test
and `scripts/board-headless.mjs` may ground a later bounded check only if
separately authorized; this capture grants no browser, runtime or harness run.
