---
title: "Visually loud widget rows when a child or the lead is waiting on the owner"
spec:
  - pi-adapter-runtime
  - 260910-pi-owner-wait-attention
plans:
  phase-1: ai-docs/.plans/2026-09/10-0120-260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner.md
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

### Result (719a18b3) - 2026-09-10

Implemented 330ms bold/plain question and approval attention in the existing
owner-lead widget, synchronized with its single count heading. Display titles
remain separate from valid qN command hints. The existing packaged adapter
configuration now accepts `agent_wait_animation`; literal false selects static
bold emphasis, and the next widget refresh observes changes. One attention
timer is shared by qualifying waits and remains separate from elapsed updates.
The formatter accepts supplied owner-held idle rows and inspection hints without
implementing ownership transitions or steering.

Final source checkpoint: `38657d6646948633ad60111120e8cc5fbb04b8c9`.
Source review range starts at plan commit `8b11343d`.

Verification:

- `cd agents-plugin-pi && node --test test/agent-widget.test.ts test/goal-loop.test.ts`:
  164 pass, zero failures at the final source checkpoint.
- `cd agents-plugin-pi && npm test`: 1478 total, 1348 pass, 130 fail. The
  failures retain the known baseline categories: 129 hardcoded Linux SDK-path
  fixtures and one stale `ws-ask` exposure assertion. The full suite is not
  clean; no unrelated fixture changes were made.
- Fake-clock assertions cover literal 330ms cadence, visible text in both
  phases, shared timer ownership, enabled final-wait cleanup, disabled static
  fallback, child exclusion, and controller teardown. Formatting regressions
  cover injected escape controls, delimiter/state-like names, exact ANSI span
  boundaries, supplied owner-held rows, and protected action hints. Widths
  0/1/8 additionally guard styling from reintroducing an omitted suffix.
- Controller tests and source-inspected host replacement/shutdown wiring are
  offline evidence; they do not claim a full live Pi session or real-terminal
  readability check.

Review dispositions:

- ATTN-COR-001 and ATTN-TST-003 [fixed]: relay commit `68313f71` uses structured
  cue/state boundaries and exact styling regressions; elapsed, telemetry,
  separators, and qN hints remain ordinary.
- ATTN-COR-002 and ATTN-TST-004 [fixed]: the same relay adds supplied
  `idle-awaiting-owner` presentation and preserves an explicit inspection hint.
- ATTN-TST-001 [fixed]: asserts literal timing, both visible phases, and final
  resolution while animation remains enabled.
- ATTN-TST-002 [fixed]: checks that the injected control sequence is absent
  without stripping it out of the inspected output first.
- ATTN-FIT-DOC-001 [fixed]: initially deferred to the root-owned documentation
  pass; spec commit `1c49d551` extends the existing widget contract with its
  subordinate attention entry. No parallel widget/count surface was added.
- ATTN-LEAD-001 [fixed]: final documentation/source checking exposed a new
  narrow-width regression introduced by the relay. Commit `38657d66` tracks
  the suffix actually appended and adds width 0/1/8 regressions. This required
  width-verification follow-up is not another independent Important review.
- All first-round findings were Important and relayed together once; there
  were no Critical or Minor findings and no independent re-review. Source/test
  fixes above are implementer self-reports with verification evidence.

Closeout: the runtime spec captures configuration and timer invariants, so no
duplicate mental-model or project-orientation change is needed. Preserve the
separate spec/ticket commits and corrective history; documentation-tip
compaction is skipped. Source inputs are unchanged after final suite evidence;
documentation checks cover the subsequent closeout. The unrelated untracked
`06-1203` plan remains untouched.

## Blocked (2026-09-10)

Owner-live acceptance remains outstanding: in a real lead TUI, observe a fork
question and an approval wait, confirm readable 330ms bold/plain emphasis and
valid qN hints, resolve waits, then disable/reload and confirm static emphasis
without duplicate animation. Keep this ticket in `ready/` and skip it during
unattended queue selection until that evidence arrives. No deprecated
ws-ask/ws-resolve flow is needed for this check.
