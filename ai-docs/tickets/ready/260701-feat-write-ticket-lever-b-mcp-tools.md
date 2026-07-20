---
title: "Lever B: checklist-as-todo and sage gate/record MCP tools"
parent: 260630-epic-skill-playbook-diet
sage-review-design: completed
sage-review-completeness: completed
---

# Lever B: checklist-as-todo and sage gate/record MCP tools

## Background

`agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` carries two blocks of
static prose that the model self-attests against with no external check —
skippable under attention pressure per the reader-model doctrine in
`lead-skill-authoring`. Both blocks are followups from the epic's Lever-A diet
pass on this skill (371 → 277 lines across two passes); this ticket scopes the
Lever-B (MCP-ification) work that pass deferred.

### 2026-07-20 re-scope: sage gate re-inflated, tool design changed

The Lever-A pass had dieted this skill to 277 lines. Commit `5c707ce9` then
split the Sage Review Gate by landing status (design@`todo/` /
completeness@`ready/`) and re-added the whole state machine as prose, taking the
file to **449 lines / ~10K tokens** — the sage block alone is now ~235 lines
(~60% of the file), up from the ~44 lines this ticket's Phase 2 originally
assumed. The ROI of Phase 2 is now ~4-5× what was scoped.

Because of that, Phase 2's tool boundary is re-drawn from the original
`sage_review_posture` + `sage_review_aggregate` (which left frontmatter writes,
Blocked-section rendering, and commits in the playbook) to `sage_gate` +
`sage_record`, so the two write-side artifacts (commit-title literals, the three
Blocked Section Templates, the "keep this table in sync with the Go side"
category×stage note) also leave the prose. See the revised Phase 2 below.

A golden-reference target body — the ~160-line / ~3K-token shape this skill
should reach once both phases land, with a per-block Diet Ledger and the new
tool contracts — is at
`ai-docs/.plans/2026-07/20-1610-write-ticket-diet-target.md`. It also records the
partial-ship fallback shapes if only one phase lands.

## Spec Impact

- Target spec area: `ai-docs/spec/mcp-tools.md` (ws MCP tool contracts).
- Caller-visible change: three new tools under the `tickets.*` namespace —
  `tickets.checklist(type, phase)` (Phase 1), `tickets.sage_gate(stem, landing)`
  and `tickets.sage_record(stem, stage, verdicts)` (Phase 2). Each returns
  data/action the `lead-write-ticket` playbook follows verbatim; the sole caller
  today is that playbook. Closeout also touches `workflow-skills.md` where the
  `lead-write-ticket` procedure surface is described, to reflect the collapsed
  handlers.
- Contract-first spec: no. Sole consumer is one playbook and the exact
  signatures (the `sage_gate` action enum, `mode`/`reviewers` fields, and
  `sage_record`'s return shape) will firm up during implementation; the spec is
  authored at closeout documenting the built contract. Matches the project's
  established posture for additive `tickets.*`/`agenda_*` MCP tools.

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

### Phase 2: Sage gate-resolve and record MCP tools

The status-split Sage Review Gate (`On: Sage Review Gate` +
`On: Design Review Stage` + `On: Completeness Review Stage` +
`On: Ready-promotion Aggregation` + the three `Blocked Section Template`s, ~235
lines total) is one deterministic state machine plus its output artifacts. The
only lead-owned step in it is spawning the reviewer subagents (an MCP tool
cannot spawn subagents); everything else is a pure function of already-known
inputs and needs no model judgment:

1. **Gate resolution** — from `(landing, category, effective posture)` decide
   the action: `idea/` landing → skip; category exempt (`research`/`workset`)
   → skip; posture `skipped`/`completed` → skip; `blocked` → stop and report;
   `recommended` → ask the user; `required` → run; missing/`pending` → resolve
   via `ws/config.show()` (`off`→skipped, `ask`→recommended, `auto`→required)
   and persist. Includes the legacy single-field `sage-review:` → dual-field
   migration mapping and the category×stage matrix (which stages run for each
   category) plus the standalone/combined-mode selection.
2. **Verdict record** — aggregate the design and completeness verdicts into a
   final `block`/`concern`/`pass` (incl. the `resolution: missing` escalation),
   write the resolved frontmatter posture, render the Blocked section from a
   Go-owned template when blocked, and commit with the canonical title.

Goal — two tools, drawn so that **both the decision logic and its write-side
output artifacts** leave the playbook (a wider boundary than the original
posture/aggregate scoping, which left frontmatter writes, Blocked-section text,
and commits as prose):

- `ws/tickets.sage_gate(stem, landing)` → `{ action, ask_prompt?, reviewers?,
  mode? }` where `action ∈ skip | stop_blocked | ask | run`. Owns posture
  resolution, legacy-field migration, `config.show` fallback, the category×stage
  matrix, and mode selection. For `ask` it returns the exact question; the lead
  relays the answer via a follow-up call. For `run` it names the reviewer(s) to
  spawn. Does not spawn.
- `ws/tickets.sage_record(stem, stage, verdicts)` → aggregates verdicts, writes
  the frontmatter posture, renders any Blocked section, and commits; returns the
  applied posture + commit ref. Mirrors `enter.implement`'s
  `raw`/`next_instruction` verbatim-follow contract.

Reviewer spawning (render `ticket-reviewer-design` /
`ticket-reviewer-completeness`, dispatch native subagents, parse their
`verdict:` text) stays lead-owned, collapsed to a single parameterized
`On: Reviewer Spawn` block (the two Stage blocks' spawn procedures are
near-identical).

Effect: the ~235-line sage block collapses to `On: Sage Review Gate` (~2 steps)
+ `On: Reviewer Spawn` (~3 steps), and the three `Blocked Section Template`s and
the "keep this table in sync with the Go category detection" note delete
entirely. Target shape in
`ai-docs/.plans/2026-07/20-1610-write-ticket-diet-target.md`.

Verification: exercise sage review across all posture values (`idea`,
`skipped`, `completed`, `blocked`, `recommended` accept/decline, `required`,
missing/legacy with each `config.show()` mapping), the category×stage matrix
(`feat`/`bug`/`refactor`/`chore`, `epic`, `research`/`workset`), and both
aggregation paths (`block`, `concern` with/without `resolution: missing`,
`pass`) against the new tools; confirm frontmatter, Blocked-section text, and
commit output match today's behavior exactly before deleting the corresponding
prose.

## Constraints

- Scoping this ticket must not edit `lead-write-ticket.md`; the **implementation
  phases do** edit it. Each phase wires its new tool(s) into the playbook and
  deletes the superseded prose in the same file — Phase 1 the
  `On: Apply Ticket Content` / `On: Intent Review` capture/check prose, Phase 2
  the ~235-line sage block and the three `Blocked Section Template`s. The
  "in-flight Lever-A diet edit" this constraint originally guarded against has
  since landed (the file is now 449 lines), so there is no longer a pending-edit
  conflict to avoid; "done" for each phase includes the corresponding deletion.
- Preserve the doctrine text verbatim — neither phase touches it.
- The three `Blocked Section Template`s are relocated (not rewritten) into
  `tickets.sage_record`, which owns Blocked-section rendering. The tool's
  rendered output must stay byte-identical to today's templates; the "verbatim"
  requirement binds the rendered result, not the prose location.
- Phase 1 and Phase 2 are independent; no ordering dependency between them. The
  golden reference's ~160-line target assumes both land; it records the
  partial-ship shapes (~430 lines Phase-1-only, ~230 lines Phase-2-only).
- Both phases add new MCP tool contracts (`tickets.checklist`,
  `tickets.sage_gate`, `tickets.sage_record`) — externally consumed schemas.
  Spec addressing (likely contract-first, given other skills or future playbook
  edits could rely on these schemas) is deferred to promotion time; this ticket
  stays in `todo/` and does not attempt the ready spec-address gate.

