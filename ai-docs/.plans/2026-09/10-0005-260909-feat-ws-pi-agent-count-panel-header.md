# Plan: Move the agent count above the agent list — Phase 1: Render the count as the agent-panel heading

## Relevant Ticket Contract
- Move the existing `ws: N agents` count, with its pending-question suffix, to the first line of the existing `belowEditor` agent widget; clear only the old `ws-agents-status` footer segment.
- Preserve the deduplicated uncapped count and pending-question count. The heading is not a row, does not consume the five-row cap, and must retain the `+N more` running-row summary plus always-visible waiting rows.
- Hide the panel and count only when both the rows and pending questions are absent. Keep other footer keys, including the independent goal-loop status, the 40/80/120-column bounds, and refresh behavior intact.
- The attention ticket owns styling and animation. If its work is present, move that existing count styling with the count; do not add animation or a configuration switch in this phase.

## Out of Scope
- Row telemetry and the future owner-held state from `260909-feat-ws-pi-agent-row-model-and-usage`.
- Implementing the 330ms attention animation, its configuration, or its owner-held-row integration from `260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner`.
- Changing registry inclusion, ordering, cap semantics, status keys owned by other extensions, or goal-loop status behavior.

## Codebase Findings
- `agents-plugin-pi/src/agent-widget.ts#L270-L303` — `buildWidgetLines` performs the five-row protected-awaiting cap, while `buildStatusSegment` builds the uncapped deduplicated count and pending-question suffix; these must become one heading renderer without changing the row builder.
- `agents-plugin-pi/src/agent-widget.ts#L376-L423` — the controller reads both registries fresh, renders the `belowEditor` widget through a real-width component factory, writes only `AGENT_STATUS_KEY`, and arms its refresh timer only while rows exist. Pending-only input needs a heading-only widget and the timer/empty predicate must follow the combined display condition.
- `agents-plugin-pi/src/agent-widget.ts#L44-L54` — `AGENT_STATUS_KEY` is explicitly distinct from the goal-loop key, and `DEFAULT_AGENT_WIDGET_WIDTH` documents the width fallback; clear this key only, and route the heading through the existing width-aware truncation mechanism.
- `agents-plugin-pi/src/index.ts#L558-L572` — `index.ts` creates and refreshes one controller only for TUI lead/fork sessions; no wiring change is needed for live refresh on spawn, settle, close, or reload.
- `agents-plugin-pi/test/agent-widget.test.ts#L242-L274` — existing pure coverage already proves the 40/80/120 bounds, footer count singular/plural behavior, and pending-question-only footer behavior; extend this focused suite for heading placement, cleared agent footer, pending-only panel, and cap/wait regressions.
- `ai-docs/spec/pi-adapter-runtime.md#L1468-L1518` — the published contract still calls the count a footer segment and separately documents the goal-loop status, so the implementation must update this section to name the panel heading without changing its numeric meaning.

## Implementation Plan
1. In `agents-plugin-pi/src/agent-widget.ts`, replace the footer-specific count helper with a heading-line helper that retains its existing uncapped deduplicated count and `question(s)` suffix, applies the existing width truncation, and returns a heading when rows or pending questions exist. Prepend that heading to `buildWidgetLines` output while leaving row sorting, waiting-row inclusion, the five-row cap, and `+N more` logic untouched.
2. Update `createAgentWidgetController` in `agents-plugin-pi/src/agent-widget.ts` to compute `pendingCount` once per refresh, render the `belowEditor` component for either rows or pending questions, clear `AGENT_STATUS_KEY` on each refresh and shutdown, and use the same combined visibility condition for timer arming. Keep the component factory so render-time widths remain authoritative; preserve any pre-existing attention styling by applying it to the moved heading rather than adding an animation path.
3. Extend `agents-plugin-pi/test/agent-widget.test.ts` with pure renderer/controller-facing cases for empty, one, capped-many, pending-only, and mixed rows: assert heading-first placement, uncapped count, row cap isolation, waiting-row visibility, cleared old footer status, unchanged unrelated status ownership, and 40/80/120 display bounds. Update `ai-docs/spec/pi-adapter-runtime.md`’s live-agent-widget count-location description to match the moved heading and preserved goal-loop separation.

## Verification Plan
- Run `cd agents-plugin-pi && node --test test/agent-widget.test.ts`.
- Run `cd agents-plugin-pi && node --test test/*.test.ts` under Node 25; compare any failures with the known baseline of 130 failures (129 Linux SDK-path fixtures and one stale `ws-ask` expectation).
- Owner live check: with another usage widget installed, confirm the count appears above the list and refreshes across spawn, settle, and close without changing other footer segments or the goal-loop status.

## Escalations
- None.
