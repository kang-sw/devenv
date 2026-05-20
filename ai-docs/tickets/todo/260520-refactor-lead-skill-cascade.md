---
title: lead skill cascade pruning from skill-authoring doctrine
related:
  260517-bug-lead-proceed-overbroad-slice: shares lead-proceed surface; phase resolution stays domain complexity, sequence after Phase 3 lands
  260519-feat-implement-branch-squash-gate: shares lead-implement surface; sequence Phase 2 (R6) before or coordinate with squash-gate Phase 1
related-mental-model:
  - workflow-skills
---

# lead skill cascade pruning from skill-authoring doctrine

## Background

Lead skills (`ws:lead-proceed`, `ws:lead-implement`, `ws:lead-write-ticket`,
`ws:lead-discuss`, etc.) feel procedurally heavy at execution time. The
heaviness is not in sub-agent execution (already offloaded to
`agents-plugin-tool/internal/wsprompt/`) but in the orchestrator skill text
itself. Treating `ws:lead-skill-authoring` as the cascade source point lets a
small number of doctrine and layout rules prune downstream lead skills in one
coordinated pass.

The work is scoped to lead skill text and the `workflow-skills` mental model
anchors that codify the rules being removed. Sub-agent prompts under
`agents-plugin-tool/internal/wsprompt/prompts/` and `infra/` are out of scope.

## Decisions

- Lead skill text is orchestrator/router contract; routing logic stays
  mechanical and auditable. Full paradigm shift to short, intent-only skills is
  rejected — compaction survival, cross-model floor, and carried-context schema
  trust would all degrade without measurement to justify the loss.
- Skill verbosity is procedural sophistication. Part is intrinsic domain
  complexity (ticket × actionable × discussion × needs-ticket matrix); part is
  accidental complexity (carried context prose, cross-skill judges, unevidenced
  negations). Only accidental complexity is pruned here.
- "Carried context" between same-actor lead skill calls is a translation
  artifact from the older Claude-style skills-with-arguments standard. Same
  conversation, same model, same working memory — no carry needed. Actor
  boundaries (sub-agent invocations) still need explicit briefs at the call
  site, but that pattern already lives in `prompts/`.
- Autonomous grant ("user has approved this chain — downstream stages do not
  re-pause") must remain explicit after carried-context removal. It is lifted
  to a continuous-grant invariant in `ws:lead-skill-authoring` rather than
  re-declared per transition.
- `ws:lead-edit` and `ws:lead-write-code` together produce a triple-execution
  shape where `lead-implement` routes to `lead-edit` and the lead simply starts
  editing without delegation. `lead-edit` is the lead wearing two hats, not a
  real actor boundary. Absorb `lead-edit` into `lead-implement`; split routing
  via `judge: needs-delegation` to `lead-write-code` only.
- Mechanical rules belong in skill text; the doctrine paragraph stays the
  generator. The new doctrine clause: lead skills are router contracts with
  auditable announce + same-actor conversation carry, with actor-boundary
  briefs at the call site.

## Constraints

- Audit trail at the `Announce` block in router skills must remain intact;
  routing must remain reproducible across model tiers (Opus / Sonnet / Haiku).
- Mental-model anchors that codify removed rules must be updated in the same
  logical phase as the skill change so spec and mental-model stay in sync.
  Anchors involved: `{#260514-skill-authoring-carried-context}`,
  `{#260519-proceed-implementation-dispatch-precheck}`,
  `{#260513-proceed-ticket-freshness-gate}`.
- Per-phase verification is dogfooded by re-running the lead skill chain on a
  real follow-up task; record line-count deltas for `lead-proceed`,
  `lead-implement`, and `lead-write-ticket` as a coarse signal.
- Reversibility: each phase commits in small, locally revertible units so an
  obvious regression in a single skill can be rolled back without unwinding
  the whole cascade.

## Rejected alternatives

- **Short, intent-only lead skills (full paradigm shift)**: rejected. Loses
  audit trail, carried-context schema trust, and cross-model floor in exchange
  for surface-level brevity. Not measured to be a net win.
- **Contract/Operations two-layer split**: rejected for now. Lead skills are
  already layered against `prompts/` and `infra/`; adding another layer inside
  the skill text duplicates the existing boundary.
- **Carried-context as schema (instead of removal)**: rejected after the
  realization that same-actor handoffs share conversation. Schema would
  formalize a translation artifact instead of removing it.

## Forward-compatibility

- New lead skills added after this cascade must follow the four-block
  `On: invoke` structure (Gather State / Decide / Announce / Execute) and must
  not declare same-actor carry blocks.
- Sub-agent invocations remain the only place where explicit context briefs are
  authored; that contract is unchanged.

## Verification expectations

- Each phase: dogfood-run the lead skill chain on at least one real task,
  confirm routing decisions still announce correctly and audit trail is
  preserved.
- Each phase: record line-count delta for affected lead skill files.
- Phase 3 (batch): verify mental-model anchor edits land alongside skill edits
  in the same commit unit; verify `ai-docs/_index.md` queue and inventory still
  reflect the pruned skills.

## Phases

### Phase 1: R3' Same-actor carried-context removal

Goal: remove same-actor carried-context blocks from lead skill text and lift
the autonomous-grant guarantee to a single continuous-grant invariant in
`ws:lead-skill-authoring`.

Scope:
- Delete `Carried Context` / `Context To Carry` sections from
  `ws:lead-proceed`, `ws:lead-discuss`, `ws:lead-write-ticket`, and any other
  lead skill that declares same-actor carry blocks.
- Replace inline "carry context: …" references in handler steps and the
  `Announce` template with bare `Continue through <skill>` phrasing.
- Add to `ws:lead-skill-authoring`:
  - Invariant: lead skill transitions share conversation; do not redeclare
    same-actor carry; sub-agent invocations need explicit briefs at the call
    site.
  - Invariant: when invoked via another lead skill rather than direct user
    request, treat the chain as a continuous grant unless a fresh
    user-blocking decision arises.
- Update mental-model anchor `{#260514-skill-authoring-carried-context}` to
  reflect the new rule.

Suggested approach:
- Land the `ws:lead-skill-authoring` invariant edit first (with the mental
  model anchor update in the same commit) so downstream pruning has a stable
  anchor to point at.
- Then prune one downstream skill at a time; each downstream prune is its own
  commit so a regression in one skill is independently revertible.
- Validate after each downstream prune by re-running the affected chain
  (`lead-discuss → lead-proceed → lead-write-ticket → lead-proceed → …`) on a
  trivial follow-up task.

Rejected alternatives carried from discussion:
- Carry-as-schema (formalize the five prose blocks into one structured carry).
  Rejected once the same-actor realization made the carry itself the artifact
  to remove, not the format.

Verification:
- After each prune, the `Announce` block of the affected skill still names the
  next stage clearly enough for audit.
- Continuous-grant invariant produces no observable change in downstream skill
  behavior for known chained cases (proceed → write-ticket, discuss →
  write-spec, etc.).

### Phase 2: R6 Lead two-hat ban — absorb lead-edit into lead-implement

Goal: collapse the `lead-implement` → `lead-edit` shape into a single
`lead-implement` that splits delegated work to `lead-write-code` and direct
work inline.

Pre-step (load-bearing check):
- Run `git log --oneline --grep "lead-edit"` and inspect recent merges for the
  "Review once" mode-transition step in `lead-edit`. If that step is load-
  bearing (it actually catches regressions a delegated reviewer would miss),
  capture its essence as an inline review step inside the new
  `lead-implement`. If it never fires in practice, drop it.

Scope:
- Add an invariant to `ws:lead-skill-authoring`: a lead skill cannot also be
  its own executor; routing to a sibling skill where the lead remains the
  acting agent is a naming artifact, not an actor boundary.
- Replace `lead-implement`'s `judge: execution-mode` with `judge:
  needs-delegation` (single decision: direct in-place edit by the lead, or
  delegate to `lead-write-code`).
- Move any preserved `lead-edit` review step inline into `lead-implement`'s
  `Execute` block.
- Delete `ws:lead-edit` skill directory after dogfooding the absorbed flow.
- Update mental-model anchor referencing `lead-edit` and
  `lead-write-code` parity (currently the line: "`lead-edit` and
  `lead-write-code` are code-and-review primitives; `lead-implement` and
  `lead-sprint` own documentation pipeline timing") so `lead-edit` is removed
  from the parity statement.

Sequencing:
- `260519-feat-implement-branch-squash-gate` touches `lead-implement`'s task
  list (squash step before merge gate). Coordinate: land Phase 2 first so the
  squash gate is added to the absorbed `lead-implement`, or rebase
  squash-gate work on the absorbed skill. Document which sequencing is taken
  in the phase Result.

Rejected alternatives:
- Keep `lead-edit` as a "mode marker" without execution semantics. Rejected:
  if it has no execution semantics, it should not be a separate skill.
- Route all implementation through `lead-write-code` regardless of size.
  Rejected: round-trip cost for trivial single-file edits is unjustified.

Verification:
- Dogfood at least one direct-edit task and one delegated task through the new
  `lead-implement`; confirm both paths announce correctly.
- Confirm `lead-edit` references are removed from `ai-docs/_index.md`,
  `agents-plugin/skills/`, and the workflow-skills mental model.

### Phase 3: Batch — R1 invoke handler structure, R2 cross-skill judges ban, R4 negative-invariant evidence rule, R5 domain-inference subquery handoff

Goal: enforce four skill-authoring rules in `ws:lead-skill-authoring` in one
edit, then sweep affected downstream skills.

Scope (each item is its own commit inside the phase):

R1 — `On: invoke` four-block structure:
- Add to `ws:lead-skill-authoring`: `On: invoke` must be split into named
  sub-blocks. Default split: `1. Gather State`, `2. Decide`, `3. Announce`,
  `4. Execute`. Flat numbered lists are allowed only when they fit in ≤ 4
  steps. Adapt block names to the skill's reading pattern but keep four
  responsibilities.
- Apply to `ws:lead-write-ticket` (currently 10-step flat list),
  `ws:lead-discuss`, and any other skill missing block separation.

R2 — Cross-skill judges banned:
- Add to `ws:lead-skill-authoring`: a lead skill applies only its own judges;
  it must not pre-apply sibling skill judges. `Announce` may name the
  delegated decision (e.g., "verdict: delegated to lead-implement"); it may
  not compute the verdict itself.
- Remove `lead-proceed` step 13–14 (which pre-applies `lead-implement`'s
  `judge: execution-mode` and `judge: branch-mode`).
- Remove `implementation-verdict-context` (already removed by Phase 1, but
  verify no straggler references).
- Update mental-model anchor
  `{#260519-proceed-implementation-dispatch-precheck}` to reflect that
  `lead-proceed` only flags routes as implementation-bound and stops short of
  contract dispatch.

R4 — Negative-invariant evidence rule:
- Add to `ws:lead-skill-authoring`'s invariant checklist: **Evidence-backed?**
  — When the invariant is a negation, name the failure mode it prevents. If
  the failure mode is not citable, the negation is a candidate for
  ablation.
- Sweep `ws:lead-proceed`, `ws:lead-implement`, `ws:lead-discuss`, and
  `ws:lead-write-ticket` for unevidenced negations (e.g., "do not implement
  here", "Source-free", "do not rejudge ticket quality") and either cite a
  failure or remove the line.

R5 — Domain-inference subquery handoff:
- Add to `ws:lead-skill-authoring`: free-prose judges that ask the model to
  classify a domain ("is this caller-visible?", "is this a freshness drift?"
  ) should be replaced by `ws/subquery` invocations when a single sub-agent
  pass can return a categorical answer. The lead skill keeps the routing
  table; the subquery owns the classification.
- Remove `judge: ticket-freshness` and `warmth` classification from
  `ws:lead-proceed`; replace with a single subquery call when freshness is
  actually in question, or drop the gate entirely if the subquery shows it
  rarely fires.
- Update mental-model anchor
  `{#260513-proceed-ticket-freshness-gate}` to reflect the subquery-backed
  replacement (or removal, if the gate is dropped).

Verification:
- After all four sub-edits land, dogfood a discuss → proceed → write-ticket →
  proceed → implement chain end-to-end and confirm announces, audit trail,
  and ticket capture still work.
- Record line-count delta for `lead-proceed`, `lead-implement`,
  `lead-write-ticket`, and `lead-skill-authoring`; target is meaningful
  reduction in the first three without growth blowing up
  `lead-skill-authoring`.
- Confirm `ai-docs/_index.md` queue and inventory remain consistent.
