---
title: tickets.sage_stamp leaves a stale Blocked section on a ticket whose issues were all folded autonomously
related:
  260828-bug-sage-stamp-leaves-blocked-section: earlier capture of the same defect; absorbed here (this ticket answers both of its follow-up questions)
spec:
  - mcp-tools
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3c7cf6fe9b4fbf31
sage-review-completeness-reviewed: 3c7cf6fe9b4fbf31
---

# tickets.sage_stamp leaves a stale Blocked section on a ticket whose issues were all folded autonomously

## Background

Dogfood, 2026-09-06, on the Pi track. Two tickets in `ready/` carried a
`## Blocked (2026-09-06)` section rendered by `tickets.sage_stamp`
(`agents-plugin-tool/internal/wsdoc/tickets_sage.go`, `renderBlockedSection`
via `appendOrReplaceBlockedSection`) from a first review pass whose verdict
was `block`. Every issue in those tables carried `resolution: autonomous`,
the lead folded them all, a later stamp recorded the re-review as
`concern`/`pass` and set both `sage-review-*: completed`, and
`tickets.move` promoted the tickets to `ready/`. The Blocked section
survived all of that: `appendOrReplaceBlockedSection` runs only when the
new verdict is itself `block` (`sageRecordSingle` and `sageRecordCombined`
both guard on `verdict == "block"`), so a non-block stamp never excises the
prior section.

The section is not decoration. The `lead-drain-ready-queue` skill tells its
selector to skip any candidate carrying a recorded blocker note and names a
`## Blocked (...)` entry as the example, so a ready ticket with a stale
table is invisible to a goal-run drain even though nothing blocks it. In
the same session the owner's drain was expected to pick
`260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction`, which was
in exactly that state. The lead removed the stale tables by hand
(commit on the Pi track) so the drain could proceed.

Source confirmation, 2026-09-06: `sageRecordSingle` and `sageRecordCombined`
(`tickets_sage.go`) call `renderBlockedSection` and
`appendOrReplaceBlockedSection` only inside their `verdict == "block"`
branches; the non-block branches write the `completed` posture and never
touch the section. The existing excision inside
`appendOrReplaceBlockedSection` removes every `## Blocked (` block by heading
prefix, without distinguishing the Go-rendered table from an owner-written
prose note, so the "owner notes survive" rule below is a new discriminator,
not current behavior. The `tickets.verify` sage-posture checks run only for
`status == "ready"` today, so the new warning needs a condition that also
covers `todo/`.

## Proposed direction

Host-neutral ws-mcp change under `agents-plugin-tool/`, landing through
`develop`.

- **Render only on `block`.** The Blocked section is rendered only when the
  stamped (single or combined) verdict is `block`; that is today's render
  condition, kept. A `concern` stamp renders nothing, including a `concern`
  that carries an issue with `resolution: missing`: the combined path
  already escalates such an issue to `concern` and writes `completed`, and
  rendering a table beside a `completed` posture would recreate the
  completed-plus-blocked state this ticket removes. The issue text stays
  in the commit message and the reviewer output. (The earlier draft's
  "block or missing renders" rule is withdrawn for that reason.)
- **Excise on any non-block stamp.** A stamp whose final verdict is not
  `block` removes the previously rendered table from the ticket, so a
  resolved review leaves no blocker note behind. The excision is a
  shape-aware helper split out of `appendOrReplaceBlockedSection`: under a
  `## Blocked (` heading it removes only the Go-rendered subsections (each
  `### <Reviewer> — <verdict>` heading immediately followed by the fixed
  table header line `renderBlockedSection` emits, with or without the
  Resolution column) and leaves any other prose under that heading
  untouched; a `###` subheading without that table header is prose. When
  nothing but the heading remains, the heading is removed too. The excision
  is not stage-scoped: a non-block stamp clears every rendered subsection,
  which is safe because a `blocked` posture stops the gate before any
  single-stage stamp can land beside another stage's table. The block-render path uses the same helper
  before appending, so a second `block` stamp replaces its own table and no
  longer clobbers owner prose under the heading (a behavior change from the
  prefix-only excision, stated here on purpose).
- **Owner-written blocker notes survive.** A `## Blocked (...)` heading
  followed by prose only (such as the sign-off note on
  `260905-feat-ws-pi-harness-config-layer`, or a note written by
  `lead-drain-ready-queue`) contains no rendered subsection, so the helper
  leaves it whole.
- **`tickets.verify` warns on a stale rendered table.** For a ticket in
  `todo/` or `ready/` that carries a rendered subsection under a
  `## Blocked (` heading while no `sage-review-*` posture reads `blocked`,
  `tickets.verify` emits a `stale-blocked-section` warning naming the
  section as a probable stale blocker. The condition is per-stage on the
  postures present (a `todo/` ticket normally carries only the design
  posture), not "both completed", so it fires in both statuses. Prose-only
  notes never warn.
- **Existing stale tables.** The excision is stamp-triggered and a ticket
  whose gate already reads terminal is never stamped again, so tables
  already sitting on tickets are surfaced by the verify warning and removed
  by hand; that is the intended remedy for the current fleet.

## Spec Impact

`mcp-tools`: `{#260624-sage-review-gate}` (the sentence that a `block`
result appends a `## Blocked (YYYY-MM-DD)` section gains the non-block
excision rule), `{#260720-sage-gate-record-tools}` (`tickets.sage_stamp`
renders on `block` only and excises otherwise, shape-aware), and
`{#260723-tickets-verify-tool}` (the guardrail list gains the
`stale-blocked-section` warning and its per-stage condition).

## Constraints

- No change to the posture fields, to the missing-resolution escalation,
  or to `tickets.move`'s upward-move validation.
- The rendered table shape is unchanged so existing tickets that still
  carry one are recognized by the excision and the verify warning.

## Phases

### Phase 1: Excise on resolution, render only on block

Split the shape-aware excision out of `appendOrReplaceBlockedSection`, call
it from both record paths on every non-block stamp and from the block path
before rendering, keep the block-only render condition, and add the
per-stage `stale-blocked-section` warning to `tickets.verify` for `todo/`
and `ready/`. Tests (`tickets_sage_test.go`): a block stamp followed by a
concern stamp with autonomous issues leaves no Blocked section; a concern
stamp carrying a missing-resolution issue renders none and writes the
existing escalated posture; a second block stamp replaces its own table and
keeps prose under the same heading; an owner-written prose Blocked note
survives a resolving stamp; a mixed section keeps its prose and loses its
table. Tests (`tickets_verify_test.go`): a `ready/` ticket with completed
postures and a rendered table warns; a `todo/` ticket with a completed
design posture and a rendered table warns; a ticket whose design posture
reads `blocked` does not warn; a prose-only note never warns. Amend the
three spec passages under Spec Impact.
