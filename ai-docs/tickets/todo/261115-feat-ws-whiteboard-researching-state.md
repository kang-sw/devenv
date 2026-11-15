---
title: "Add researching as a whiteboard topic work state"
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: "793: rendering phase waits for the asset split"
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: "Existing state, run, query and notification contracts"
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
  Their run/ETA consequences are not yet specified; see the Open Decision Queue.
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
  Gate (`agents-plugin/rsrc/`, lines 17–25; wsflow/Pi copies, lines 21–24)
  says to commit all `ai-docs/` changes: that implicitly includes already-tracked
  or unignored boards, not force-adding excluded files. The whiteboard playbook's
  takeover recipe uses “committed folder” for a durable store transaction, not
  Git. Record this residual interaction; changing wrapup conventions is outside
  Phase 1 and requires separate authority.
- Deliver in two sequential slices: Phase 1 changes unfrozen Go, playbook,
  mirror and all 11 locale tables; Phase 2 completes rendering after 793.
  Phase 1 is a partial delivery with a render gap, not completed UI support.
  Embedded assets belonging to one daemon does not eliminate this deliberately
  staged source-level gap. Exact exposure/rollout behavior is unresolved below.
- UI-fast verification has no approved screenshot reference and no screenshot
  or viewport/theme matrix requirement. Do not import the asset-split ticket's
  screenshot acceptance into this feature.

## Constraints

- Apply `ai-docs/manuals/shipped-surface-boundary.md` to all shipped changes:
  the eventual playbook explains generic behavior, not this repository's owner
  numbers, tickets or scheduling rules.
- Apply `ai-docs/manuals/release-compatibility.md` to Go/store-facing work,
  using the explicitly settled T0 deployment assumptions above.
- Apply `ai-docs/manuals/skill-authoring.md` to the whiteboard playbook and
  `ai-docs/manuals/wsflow-mirroring.md` to its generated mirror and manifests.
- Phase 1 leaves `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`,
  `dashboard.js`, `agents-plugin-tool/internal/wswhiteboard/assets/page.css`,
  all whiteboard scripts (including `ago.js`), and `scripts/board-headless.mjs`
  untouched. Renderer work is Phase 2 after the asset split's landed result/move
  map. `page.tmpl` badge changes also belong to Phase 2.
- No new research provider, research execution machinery, automatic state
  inference or changes to general stale policy are included.

## Current-source grounding

Pointers below describe the capture baseline, not implementation results or
ready-route facts. Search by the named symbols after 793 moves assets.

| Surface | Current evidence and relevance |
|---|---|
| `internal/wswhiteboard/topic_state.go` | `topicStateValues` contains four states; `applyTopicState` requires owner/ETA for implementing and rejects them otherwise. `parseTopicState`, `validRun`, `foldStateEvent` and close/reset history assume only implementing has a run; unknown enum values are malformed. `structuralWarnings` scopes the no-open-items warning to discussing. |
| `internal/wswhiteboard/topic_eta.go` | `validExtension` and `applyTopicETA` accept only implementing runs, with cumulative budget and strict extension history. Optional research attributes cannot simply reuse this without a decision. |
| `internal/wswhiteboard/query.go` | Topic metadata's `State *TopicState` carries value, entry and optional run. Query tests pin state/diagnostic output; extend the existing projection rather than inventing a separate marker. |
| `internal/wswhiteboard/board_notice.go` | `eligibleState` admits implementing/waiting for hourly epochs; `planNotice` computes overdue only for implementing. Research timed eligibility is a decision gap, separate from general stale behavior. |
| `internal/wswhiteboard/nudge.go` | `staleNudge` uses open-topic count, open-item absence and activity age, not declared state. Its local `waiting` map means open asks, not `StateWaiting`. Preserve that distinction. |
| `internal/wswhiteboard/update_summaries.go` | New-topic default producer writes `StateDiscussing`; `summaryState` and net-change summaries project state. Preserve default behavior and cover explicit researching in the same creation batch. |
| `agents-plugin/rsrc/whiteboard/whiteboard.md` | Work-state obligations, `wb:state`/`wb:eta` grammar, query metadata and timed-nudge sections currently describe four states and implementing-only runs. Update together once missing policy is settled. Mirror is `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`; both rsrc manifests participate. |
| `internal/wsi18n/tables/*.json` | All 11 tables: `en`, `ko`, `ja`, `zh-Hans`, `zh-Hant`, `de`, `es`, `fr`, `it`, `pt-BR`, `ru`. Existing state keys use `js.wb.state.*`, filter labels use `js.board.filter.*`. Supply research state/filter translations in Phase 1; Korean state label is fixed above. |
| `internal/wswhiteboard/assets/page.tmpl`, `ago.js`, `page.css` | `topicstate` renders a four-way badge; `ago.js` maps those states and currently shows owner only for implementing with run identity. Extend server rendering and live reconciliation consistently in Phase 2; use post-split stylesheet/script locations. |
| `internal/wsdashboard/assets/cards.js` and rendering assembly | `filterValues` enumerates four states plus all/closed. Phase 2 includes research filtering, card state propagation, overview/render consumers and any required dashboard styling. `internal/wsdashboard/pages.go`, `internal/wswhiteboard/render.go` embed/assemble the assets; follow the 793 move map rather than assuming old paths survive. |
| Daemon compatibility | `internal/wsdashboard/ensure.go:keepRunning`, `version.go:CompareVersions`/`SameMinor`, `proxy.go` hello validation, and the embed declarations in `pages.go`/`render.go` ground the supported T0 assumptions. `internal/mcp/whiteboard_store.go:whiteboardFolder` locates the repository-local authoritative folder store; legacy cache parts are rollback snapshots. `internal/mcp/whiteboard_tools.go:renderWhiteboard`/`registerWhiteboardExclude` auto-register the local exclusion, which does not untrack boards. This clone has zero tracked whiteboard files; downstream git usage can differ. |

All `internal/` paths above are under `agents-plugin-tool/`.

## Open Decision Queue

These observable policy boundaries remain for the lead/MAIN, not objections to
researching or T0 or additional ready gates. Optional owner/ETA and unchanged
general stale behavior are settled and are not re-asked. No unconfirmed mechanism
or behavioral recommendation is implementation authority.

1. **Research ETA clock behavior.** Choose whether a supplied research estimate
   is informational only, or starts a timed research run with elapsed history and
   extension behavior. If timed, choose whether `wb:eta` applies and whether the
   transition to implementing starts a fresh clock or carries the research clock.
   Source: `topic_state.go` (`validRun`, `foldStateEvent`,
   `applyTopicState`) requires complete implementing runs and drops current runs
   for other states; `topic_eta.go` is implementing-only. Storage fields and
   parser/fold organization are technical implementation details once observable
   clock/projection behavior is settled; no new persisted mechanism is chosen here.
2. **Timed research notices.** Choose existing eligibility unchanged, research
   included in hourly notices only, or research included in hourly and overdue
   notices when it has an ETA. Source:
   `board_notice.go:eligibleState` and `planNotice` currently exclude research.
   This is separate from the already-settled state-independent general stale
   nudge in `nudge.go`.
3. **Partial-delivery exposure.** Choose whether the Phase 1 backend/text slice
   is user-exposed with the known missing badge/filter, or user-facing rollout
   waits for Phase 2. No gate, negotiation or renderer fallback is chosen.
   Source: `page.tmpl:topicstate`,
   `ago.js`'s state map and `cards.js:filterValues` recognize four states and
   remain frozen for Phase 1; the two-phase delivery is already authorized.
   Deployment operations themselves are not authorized by this ticket.

## Phases

### Phase 1: Add the declared state on unfrozen backend and text surfaces

Add researching to enum acceptance, persisted state parsing/folding/history, command
validation, query projections and net-change summaries. Keep explicit same-batch
creation overrides and the new-topic discussing default intact. Cover the
owner/ETA seams according to the confirmed contract, preserving implementing
start/extension/close semantics. Make warnings and timed notices reflect the
settled scopes without changing general stale behavior.

Update the canonical whiteboard playbook's work-state obligation, grammar,
query and notification explanations; regenerate the wsflow rsrc copy and both
manifests through the documented mirroring procedure. Add the state and filter
translations in all 11 locales. Honor the frozen rendering boundary above;
report the partial delivery and remaining UI gap explicitly.

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
  drift and `TestWsflowRsrcMirrorUpToDate`. Run
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
presentation follows the separately confirmed metadata contract. Do not add a
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
