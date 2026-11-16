---
title: "Whiteboard ask 1205 was absent from the board page despite existing in the store"
---

# Whiteboard ask 1205 was absent from the board page despite existing in the store

## Background

Dogfood #15, 2026-11-16: the owner could not see ask item **1205** on the
whiteboard page although the store held it. This is lead/owner-reported evidence;
the original capture did not inspect the live store or browser. The subsequent bounded pass read the stored records without mutation. The coordinator identified session `eject-alfalfa-unvaried`, board `renewaldevoteetranspirefencing`, topic `exe-spawn`, and page `http://127.0.0.1:29417/s/renewaldevoteetranspirefencing`. The served daemon reported hash `c34fbf9413a3a53e` and started at 00:43:35Z; that hash is not established as a Git revision. No incident DOM, browser, filter or fold capture exists. The cause is unknown: do not label it a stale stream, filter, rendering or storage defect without evidence.

## Evidence for diagnosis

The read-only incident record `ai-docs/whiteboards/renewaldevoteetranspirefencing/t12/log.jsonl#L10-L14` records policy item 1205 added at 00:56:41.8505109Z (e210, seq 112; its title contained `<b>python</b>`), topic state changed to implementing in e211 at the same time, and e212 was an agent clarification at 00:56:46.2192075Z. At 00:57:30.876551Z e214 records a policy patch open to open with `python` unmarked in the title and an owner input reporting the question was not visible; at 01:02:35.3706213Z e217 records answer `a` (seq 125). The coordinator describes e214 as title-only; the event records the resulting title and status, not the command fields. The current `ai-docs/whiteboards/renewaldevoteetranspirefencing/t12/meta.json` item is answered and records `asked: 117`, `reasked_at: 2026-11-16T00:57:30.876Z`, and `changed: 125`. Current source treats a changed title on an open item as a re-ask (`agents-plugin-tool/internal/wswhiteboard/store.go#L1101-L1110`, `#L1207-L1222`); this does not establish the incident daemon's code or the browser's state.

At source revision `98ccd5e61`, the query defaults to open items and excludes closed topics unless targeted or explicitly requested (`agents-plugin-tool/internal/wswhiteboard/query.go#L145-L172` and `#L181-L265`); a query is not the board render. The served board builds a view from the session record and renders regions (`agents-plugin-tool/internal/mcp/dashboard_page.go#L263-L306`); view projection classifies items by status and class (`agents-plugin-tool/internal/wswhiteboard/view.go#L608-L704`), and the full-page timeline retains open questions as anchors while settled questions may fold (`agents-plugin-tool/internal/wswhiteboard/timeline_view.go#L60-L118`, `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L67-L73`). History gaps can hold rows in inert templates. The dashboard receives WebSocket events (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-stream.js#L145-L173`) and stages or defers board morphs (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L4-L30`, `#L58-L106`); topic-state filtering and closed-page controls are in `agents-plugin-tool/internal/wsdashboard/assets/cards.js#L189-L193` and `#L261-L322`. Current-source event production subscribes before the initial render, wakes or refreshes the shared page, and emits changed regions or opt-in topic deltas (`agents-plugin-tool/internal/wsdashboard/sse.go#L442-L460`, `#L480-L515`, `#L552-L600`, `#L639-L679`; `agents-plugin-tool/internal/wsdashboard/board_topics.go#L102-L118`); successful dashboard-dirtying tool calls separately queue a dashboard nudge for a served registered session (`agents-plugin-tool/internal/mcp/server.go#L809-L822`, `agents-plugin-tool/internal/mcp/dashboard_sessions.go#L500-L519`, `#L540-L566`); the daemon's nudge handler requests an indexed re-read (`agents-plugin-tool/internal/wsdashboard/daemon.go#L837-L856`, `agents-plugin-tool/internal/wsdashboard/index.go#L225-L246`). The whiteboard update's own change callback is a separate nudge-discovery path (`agents-plugin-tool/internal/mcp/whiteboard_tools.go#L313-L315`, `agents-plugin-tool/internal/mcp/whiteboard_nudges.go#L71-L90`). These are investigation paths, not evidence that an incident event was sent or received. No stored data and source trace alone establish a visibility defect or its cause.

Relevant existing coverage includes `TestQueryClosedAndOverrides`, `TestTimelineOpenQuestionIsExtractedWithGapsOnBothSides`, `TestBoardViewedTimelineQuestionIdentity`, and `TestBoardStateFilterBrowser` in `agents-plugin-tool/internal/wswhiteboard/query_test.go`, `agents-plugin-tool/internal/wswhiteboard/timeline_fold_test.go`, `agents-plugin-tool/internal/wsdashboard/board_viewed_test.go`, and `agents-plugin-tool/internal/wsdashboard/board_state_filter_browser_test.go`, respectively. If the symptom recurs, capture the affected page and daemon identities, item status and history, filter/fold and pending-update state, initial response, subsequent stream messages, and corresponding DOM before choosing a fix; compare a reload and intentional filter controls without altering the original item.

## Blocked - needs live capture

The coordinator-authorized single bounded diagnosis pass on 2026-11-16 recovered the stored lifecycle and inspected the render/stream paths, but did not demonstrate a cause. Keep this ticket in `todo/`; do not guess-fix or promote it without a reproducer or layer-specific evidence. No implementation, tests, builds, live probing or store mutation occurred in this pass. The incident daemon hash did not resolve as a commit in the inspected checkout, so current-source behavior cannot establish the served behavior.

Candidate paths, not findings: topic-state filtering after e211, pending/staged or hidden-page updates, history/DOM visibility, and delivery/render mismatch. No incident browser or filter capture exists and none is expected retrospectively. On recurrence, capture the page/daemon identities, item state, filter/fold and pending-update state, initial response, stream messages and corresponding DOM; compare reload and intentional-filter controls. These observations must identify the omission layer before selecting a fix.

## Decisions

Compatibility: **n/a** for a visibility/rendering correction preserving existing
item/store and wire shapes; reassess if diagnosis requires a cross-version change.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261115-feat-ws-dashboard-board-topic-transport (2026-11-16, commit 07544121): "D6 directional T0: treerows=1 is additive and per connection; old pages retain region events and new pages accept old daemon full regions." — bearing: constrains
- 261113-feat-ws-dashboard-frontend-performance-profile-and-optimize (2026-11-16, commit 211817aa): "Watch rows after init/restore and on wbhistoryload; swept board handles and their cached report flags are cleared so materialized nodes can subscribe again." — bearing: constrains
- 261109-bug-ws-whiteboard-patch-cannot-change-item-class (2026-11-09, commit 19edecfd): "a re-ask is a patch that changes class or options, or sets a settled item back to open." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | unknown | The fault and affected implementation files are not yet identified; the evidence paths span dashboard and whiteboard. |
| scope.surface | unknown | The failing layer and required interface are not yet known. |
| scope.new_public_symbol | unknown | No fix has been selected. |
| scope.new_type_contract | unknown | No fix has been selected. |
| scope.test_surface | existing | query_test.go, timeline_fold_test.go, board_viewed_test.go and board_state_filter_browser_test.go provide relevant existing coverage. |
| complexity.reuse_points | confirmed | BuildView, timeline projection, board staging and existing test fixtures are present; which one needs correction is not established. |
| complexity.side_effect_risk | moderate | Changes may affect topic visibility, staged updates or stored item history. |
| risk.correctness | high | The omission layer and item eligibility are unknown, and changes must retain legitimate visibility rules. |
| risk.fit | moderate | The fix must preserve the existing history, filters and reader-stability mechanisms. |
| risk.test | high | Stored lifecycle is recovered, but the reproducer, browser/filter state and served source revision remain unknown; initial and incremental paths need controls. |
| risk.security_or_contract | moderate | Session isolation and cross-version compatibility require reassessment if the diagnosis crosses boundaries. |

## Phases

### Phase 1: Trace ask 1205 from stored item to owner-visible board

Recover the incident context and exact stored item, compare the board query/view,
rendered response and visible browser DOM, and reproduce why this ask was absent.
Check topic/session routing, filters/folds, initial load and incremental delivery
without assuming which layer failed. Fix only the demonstrated cause; retain
legitimate visibility rules and the ask's identity/history. Verify that the
same eligible stored ask appears on initial load and after live updates, with
controls for intentionally filtered/closed items. Which layer omitted item
1205, and under what page state? The coordinator authorized a bounded diagnosis and a fix only if a concrete cause appeared. That pass found no demonstrated cause; the ticket remains `todo/`, needs live capture, with no completed implementation phase.
