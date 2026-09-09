---
title: "Move the agent count above the agent list"
sage-review-design: completed
related:
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only
  260905-feat-ws-pi-live-agent-widget: existing count and list semantics
  260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner: coordinate the count display destination without duplicating animation ownership
spec:
  - pi-adapter-runtime
  - 260905-pi-live-agent-widget
plans:
  phase-1: ai-docs/.plans/2026-09/10-0005-260909-feat-ws-pi-agent-count-panel-header.md
sage-review-completeness: completed
sage-review-design-reviewed: 4cc99468a585c8d2
sage-review-completeness-reviewed: 4cc99468a585c8d2
---

# Move the agent count above the agent list

## Background

The owner wants the agent count visually grouped above the list instead of
appearing after another extension's footer status. The current implementation
uses separate below-editor widget rows and a footer setStatus count, so their
relative placement is not list ordering. Ready promotion was authorized on
2026-09-09 through the dogfood collection.

## Decisions

- Move the existing `ws: N agents` count, including its pending-question suffix,
  to the first line inside the existing agent widget, immediately above rows.
  Clear the old agent-count footer segment so there is one count presentation.
- Preserve the existing deduplicated, uncapped count semantics and question
  count. The heading is not an agent row and does not consume the row cap.
  Preserve hidden-running-row summaries and always-visible waiting rows.
- Hide the panel/count when there are no agents or pending questions. Preserve
  other extensions' widgets/footer segments and the separate goal-loop status.
- Keep 40/80/120-column bounds and live count refresh. Attention styling is
  owned by its existing ticket and follows the count to this destination;
  this ticket does not add a separate animation or config switch.

## Spec Impact

Update the live-agent-widget section of `pi-adapter-runtime`: count location
moves from the footer into the list heading; its numeric meaning is unchanged.

## Phases

### Phase 1: Render the count as the agent-panel heading

Reuse count construction and render it before the existing rows; remove only
the old agent-count status segment. No dependency on row telemetry or the
attention feature; coordinate the shared display seam if both land together.

Verification: empty, one, many/capped, pending-question-only and mixed rows;
count matches uncapped deduplicated rows; old footer segment cleared; unrelated
footer keys untouched; widths and refresh on spawn/settle/close. Owner checks
the count appears above the list with another usage widget installed.

### Result (cbcc14cc) - 2026-09-10

Implementation checkpoint through `c251b320`; owner visual acceptance remains
pending. The count now renders first in the existing widget, using the same
render-time width handling as the rows. The controller clears only the retired
agent footer key and uses the combined rows-or-pending predicate for panel
visibility and timer lifetime. No attention animation was added.

Review: Correctness was clean. Test T1 [fixed], implementer report after its
single relay: a controller/factory fixture combines a matched thread-bound
agent and pending thread with six running agents. It renders
`ws: 7 agents · 1 question`, retains the waiting row, and preserves the capped
body with `+2 more`, proving the uncapped deduplicated heading at the production
controller boundary. Fit M1 [fixed]: the pending-count helper's stale comment
now names the panel heading. No Critical re-review or additional Important
re-review was needed.

Verification: 34 focused widget tests passed, including actual factory renders
at 40/80/120 columns, pending-only state, unrelated goal status preservation,
and visible-to-empty timer cleanup. Full adapter suite: 1,431 tests, 1,301
passed, 130 pre-existing failures (129 Linux-specific SDK path fixtures on
macOS and one stale `ws-ask` exposure expectation). No new suite failures;
`ws-ask` remains hidden. The plan, implementation, and spec describe the same
bounded relocation. No actual TUI session with another usage widget was run.

## Blocked (2026-09-10)

Awaiting the required owner visual check with another usage widget installed:
confirm the count appears immediately above the agent list and refreshes across
spawn, settle, and close while other footer segments and goal status remain
unchanged. Automated factory/controller coverage does not establish placement
in that live extension combination. Record the result before closing this
ticket; autonomous ready-queue selection should skip it while this acceptance
is outstanding.
