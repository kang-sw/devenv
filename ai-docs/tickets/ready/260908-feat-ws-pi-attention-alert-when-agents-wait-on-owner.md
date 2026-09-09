---
title: "Visually loud widget rows when a child or the lead is waiting on the owner"
spec:
  - pi-adapter-runtime
related:
  260908-epic-ws-pi-subagent-conversation-view: specifies prominent rendering for `idle-awaiting-owner` (owner-held children) only, styling left open; this ticket covers the other waits and pins the styling
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: introduces `lastWriter`/owner-held liveness; its `idle-awaiting-owner` rows should get the same treatment
  260905-feat-ws-pi-live-agent-widget: the widget whose `awaiting owner` / `awaiting approval` rows are the surface being changed
  260906-workset-ws-pi-dogfood-ux: polishing board
  260909-feat-ws-pi-agent-count-panel-header: coordinate the single count destination; independent implementation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1c4eae80bb6eece6
sage-review-completeness-reviewed: 1c4eae80bb6eece6
---

# Emphasize child waits that need the owner

## Background

The owner wants an intentionally conspicuous TUI cue for child questions and
approval waits. On 2026-09-09 the owner selected a 330ms bold/plain text toggle
and confirmed the display-title/command distinction and approval label.

## Decisions

- Only the owner's lead Pi animates; child Pi processes do not. Qualifying rows
  are open owner questions, pending approvals, and owner-held idle children once
  that state is supplied by the audit/steering ticket. Ordinary lead idle is out
  of scope. Do not restore the deprecated ws-ask or ws-resolve tool surface;
  fork-raised questions use the current question mechanism.
- Question cue: `/answer <title>` is the visible attention phrase, accompanied
  by the valid `/answer qN` command hint. The title is display text, never a new
  command lookup key. Preserve the ID hint when truncating a long title; sanitize
  control characters and use width-aware truncation. Fall back to the question
  ID when its title is unavailable.
- Approval cue: toggle the existing `awaiting approval` label, with no fabricated
  `/answer` target. An owner-held idle row without a question keeps its existing
  state label and inspection affordance, applying the same emphasis.
- Toggle only text weight between bold and ordinary every **330ms**. Keep text
  visible in both states. Preserve elapsed time and existing tool/body muting.
  Continue while qualifying waits remain; there is no timeout-to-static.
- Emphasize the existing single agent-count segment on the same cadence while
  any wait qualifies. The count-header ticket moves it above rows; do not restore
  or duplicate an old footer count when integrating either landing order.
- Provide one adapter configuration flag to disable animation, using static
  emphasis instead. Default is enabled. No sound, OS notification or color cycle.
- Own one animation timer per owner widget, only while enabled and needed.
  Stop it on final wait resolution, disable, session switch or teardown; avoid
  duplicate timers on re-render/reopen. No model calls or RPC polling per tick.

## Spec Impact

Update `pi-adapter-runtime` live-agent-widget coverage for the 330ms weight
animation, trigger/label rules, valid question command hint, disabled fallback
and timer lifecycle. The audit ticket's future owner-held state is an integration
point, not a blocker for existing question and approval rows.

## Phases

### Phase 1: Animate actionable owner waits

Implement the confirmed row/count styling and configuration over existing waits,
retaining sorting, protected-row visibility and question/approval actions. Apply
the same rendering to owner-held idle states when available without implementing
owner steering in this slice.

Verification: fake-clock tests for 330ms bold/plain transitions and persistent
visibility, multiple waits sharing one timer, last wait resolution, disabled
fallback, child-context exclusion, teardown/session switch, long/control-character
titles and valid qN hints, approval rows without answer targets, and count-header
integration. Owner live acceptance checks readability in a real terminal with
fork questions and approval waits; no ws-ask/ws-resolve test is required.
