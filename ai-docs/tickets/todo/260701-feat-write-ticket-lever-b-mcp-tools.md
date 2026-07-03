---
title: "Lever B: checklist-as-todo and sage-review posture/aggregate MCP tools"
parent: 260630-epic-skill-playbook-diet
sage-review: completed
---

# Lever B: checklist-as-todo and sage-review posture/aggregate MCP tools

## Background

`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` carries two blocks of
static prose that the model self-attests against with no external check —
skippable under attention pressure per the reader-model doctrine in
`lead-skill-authoring`. Both blocks are followups from the epic's Lever-A diet
pass on this skill (371 → 305 lines); this ticket scopes the Lever-B
(MCP-ification) work that pass deferred.

## Phases

### Phase 1: Checklist-as-todo for `On: Apply Ticket Content` and `On: Intent Review`

`On: Apply Ticket Content` (a categorized capture list, currently 2 lines
after a 2026-07-01 dedup pass) and `On: Intent Review` (a check list plus the
single generative "fresh implementer" test, currently 7 lines) are static
prose the model reads once and self-attests against, with nothing forcing
per-item verification. Exact line/item counts will keep drifting as dedup
passes land; the checklist-as-todo goal below applies to whatever the current
category lists are, not a fixed count.

Goal: a new MCP tool (e.g. `ws/tickets.checklist(type, phase: "content"|"intent")`)
returns each phase's item list as data. The playbook installs exactly **one**
todo per phase (2 total, not 27) via `todo.append`; each todo's `Instruction`
carries the full multi-item text for that phase, mirroring `enter.implement`'s
per-step `Instruction` granularity — not per-checklist-item granularity. The
todo is checked only after the whole phase is satisfied, not per item.

`On: Open Decision Queue` is explicitly out of scope for this phase — it is
already gated by `judge: needs-open-decision-queue` as an interactive stop, so
wrapping it in a todo would duplicate an existing mechanical guard.

Once the tool exists and the playbook calls it, the corresponding static prose
for `On: Apply Ticket Content` and `On: Intent Review` becomes deletable from
the playbook, per the epic's Lever B rule ("Move rule-based conditional
decisions from playbook prose into `enter.*` or other MCP tools ... the
playbook collapses to: gather inputs → call tool → follow output").

Rejected alternative: one todo per checklist category (as many todos as
categories in the current lists) — rejected as ceremony/overhead
disproportionate to the verification benefit; coarse per-handler todos with
rich `Instruction` text give the same external-check benefit at 2 tool-call
round-trips instead of one per category.

Verification: exercise `lead-write-ticket` end to end on a real ticket edit and
confirm both todos install, carry the full checklist text, and get checked only
on phase completion; confirm the deleted prose leaves no dangling references in
the playbook.

### Phase 2: Sage-review posture-resolve and verdict-aggregate MCP tools

`On: Sage Review Gate` (~44 lines) contains two blocks that are pure functions
of already-known inputs, needing no model judgment:

1. **Posture resolution** — `idea/` → skip; `skipped`/`completed` → skip;
   `blocked` → stop and report; `recommended` → ask the user, then branch;
   `required` → run without asking; missing/`pending` → resolve via
   `ws/config.show()` (`off`→skipped, `ask`→recommended, `auto`→required),
   write the resolved posture to frontmatter, then continue from the matching
   rule.
2. **Verdict aggregation** — combine the design-reviewer and
   completeness-reviewer verdicts into a final `block`/`concern`/`pass`,
   including the `resolution: missing` escalation rule, and derive the
   frontmatter value (`blocked`/`completed`) and commit title.

Goal: `ws/tickets.sage_review_posture(ticket)` and
`ws/tickets.sage_review_aggregate(design_verdict, completeness_verdict)`, each
returning an action/instruction the lead follows verbatim (mirroring
`enter.implement`'s `raw`/`next_instruction` contract). Reviewer spawning
(rendering `ticket-reviewer-design` / `ticket-reviewer-completeness`,
dispatching native subagents, capturing their verdict text) stays lead-owned —
an MCP tool cannot spawn subagents.

Once both tools exist and the playbook calls them, `On: Sage Review Gate`
collapses from ~44 lines to an estimated ~15-18 lines: call posture tool →
follow returned action → if `run`, spawn both reviewers → call aggregate tool
with their verdicts → follow returned instruction (frontmatter value, commit
title, Blocked Section Template trigger).

Verification: exercise sage review across all posture values (`idea`,
`skipped`, `completed`, `blocked`, `recommended` accept/decline, `required`,
missing/legacy with each `config.show()` mapping) and both aggregation paths
(`block`, `concern` with/without `resolution: missing`, `pass`) against the new
tools; confirm frontmatter and commit output match today's behavior exactly
before deleting the corresponding prose.

## Constraints

- Do not change `lead-write-ticket.md` as part of scoping this ticket; a
  separate, already-in-flight Lever-A diet edit is pending on that file.
- Preserve the `Blocked Section Template` and the doctrine text verbatim —
  neither phase touches them.
- Phase 1 and Phase 2 are independent; no ordering dependency between them.
- Both phases add new MCP tool contracts (`tickets.checklist`,
  `sage_review_posture`, `sage_review_aggregate`) — externally consumed
  schemas. Spec addressing (likely contract-first, given other skills or
  future playbook edits could rely on these schemas) is deferred to promotion
  time; this ticket stays in `todo/` and does not attempt the ready
  spec-address gate.

