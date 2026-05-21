---
title: lead skill cascade pruning from skill-authoring doctrine
related:
  260517-bug-lead-proceed-overbroad-slice: shares lead-proceed surface; phase resolution stays domain complexity, sequence after Phase 3 lands; re-diagnose 260517 surface in Phase 3 Result since R2 removes lead-proceed steps 13-14
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
  re-pause") stays implicit at the conversation level. Each user-approval
  gate in a downstream skill is rewritten to fire only when the skill was not
  entered from another skill's flow; chained flows pass through without
  re-prompting.
- `ws:lead-edit` and `ws:lead-write-code` together produce a triple-execution
  shape where `lead-implement` routes to `lead-edit` and the lead simply starts
  editing without delegation. `lead-edit` is the lead wearing two hats, not a
  real actor boundary. Absorb both into `lead-implement` as a unified spine:
  `judge: needs-delegation` gates direct-edit vs delegated within each stage.
- `judge: plan-depth` is a unified 4-level cumulative spectrum
  (none / brief / survey / research) that fires independently of delegation
  path. Brief is a self-anchoring scope record usable on any path; survey
  and research build on brief by adding plan-populator output.
- Active-conversation freshness is lead-owned. `ws/subquery` agents do not
  inherit the lead's conversation state, so they may verify self-contained
  artifact or codebase evidence but must not infer hidden discussion context.
- Mechanical rules belong in skill text; the doctrine paragraph stays the
  generator. The new doctrine clause: lead skills are router contracts with
  auditable announce + same-actor conversation carry, with actor-boundary
  briefs at the call site.
- Fresh-reader audit is a post-edit verification gate for skill authoring:
  after local reread, a context-light pass flags wording that only makes sense
  from the current discussion, plus contradictions, duplication, orphan
  references, and missing end-state or output instructions.
- Fresh-reader audit output names each quote, issue, and suggested rewrite or
  delete; when clean, it states that no material issue remains.
- Downstream consistency sweep follows doctrine, terminology, route, layout,
  or audit-gate edits: a conservative finding-only pass scans affected skill
  surfaces, then the lead classifies findings as fix, intentional difference,
  or out of scope before editing.
- `lead-check-blockers` and `lead-verify-discussion` are intentional compact
  fast-path checkpoints. Treat missing full workflow ceremony there as an
  intentional difference unless the actual output or end state is unclear.

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

- New skills added after this cascade must follow the four-block
  `On: invoke` structure (Gather State / Decide / Announce / Execute), must
  not declare same-actor carry blocks, and must gate user-approval steps with
  the not-chained condition.
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

Goal: remove same-actor carry blocks from skill text and encode the
autonomous-grant assumption as a not-chained condition at each downstream
user-approval gate.

Scope:
- Delete `Carried Context` / `Context To Carry` sections from
  `ws:lead-proceed`, `ws:lead-discuss`, `ws:lead-write-ticket`, and any other
  skill that declares same-actor carry blocks.
- Replace inline "carry context: …" references in handler steps and the
  `Announce` template with bare `Continue through <skill>` phrasing.
- Sweep each downstream skill for user-approval gates (pauses, confirms,
  prompts that wait for user signal) and append a not-chained condition:
  "If this skill was not entered from another skill's flow, …". Direct
  invocation still hits the gate; chained flows pass through.
- In `ws:lead-skill-authoring`, replace the carry-block guidance
  ("Skill-to-skill transitions are context handoffs…", "`Continue through
  <skill>; carry context: …`") with bare-handoff phrasing, and add one
  short principle: user-approval gates are written with the not-chained
  condition so chained flows pass through.
- Regroup `ws:lead-skill-authoring`'s Principles section into five named
  subsections (Reader model / Layout / Content rules / Iteration / Skill
  semantics); semantics-preserving reorganization so downstream prune
  patterns can mirror the layout.
- Update mental-model anchor `{#260514-skill-authoring-carried-context}` to
  reflect: carry blocks removed; chained-flow allowance encoded at each gate
  locally.
- Sync the matching `ai-docs/spec/workflow-skills.md` paragraph carrying the
  same anchor `{#260514-skill-authoring-carried-context}` so spec prose
  describes the bare-handoff and chain-passthrough behavior canonically.

Suggested approach:
- Land the `ws:lead-skill-authoring` edit first (carry-guidance replacement
  plus the gate-condition principle, with the mental-model anchor update in
  the same commit) so downstream pruning has a stable doctrine to point at.
- Then prune one downstream skill at a time; each downstream prune is its
  own commit so a regression in one skill is independently revertible.
- Validate after each downstream prune by re-running the affected chain
  (`lead-discuss → lead-proceed → lead-write-ticket → lead-proceed → …`) on
  a trivial follow-up task; confirm gates still fire on direct invocation and
  pass through on chained invocation.

Rejected alternatives carried from discussion:
- Carry-as-schema (formalize the five prose blocks into one structured carry).
  Rejected once the same-actor realization made the carry itself the artifact
  to remove, not the format.

Verification:
- After each prune, the `Announce` block of the affected skill still names
  the next stage clearly enough for audit.
- Direct invocation of a pruned skill still hits its user-approval gates.
- Chained invocation (e.g., lead-discuss → lead-write-ticket, lead-proceed →
  lead-write-spec) passes through gates without re-prompting.

### Result (c5c1349b) - 2026-05-21

Commits: `fcea37a7..c5c1349b` (7 commits).

- `ws:lead-skill-authoring`: replaced carry-block guidance with gate-local
  chain-condition principle; regrouped Principles into 5 named subsections
  (Reader model / Layout / Content rules / Iteration / Skill semantics).
- Removed carry blocks from `ws:lead-discuss`, `ws:lead-write-ticket`,
  `ws:lead-proceed`, `ws:lead-write-skeleton`.
- Replaced inline "carry context: …" with bare `Continue through <skill>`.
- Synced spec anchor `{#260514-skill-authoring-carried-context}` to
  bare-handoff and chain-passthrough behavior.

Deviations:
- Not-chained gate annotation: encoded as doctrine principle in
  `ws:lead-skill-authoring` rather than per-gate text in each downstream
  skill. Downstream gates already behaved correctly once carry blocks were
  absent; per-gate annotation would be redundant prose.

### Phase 2: R6 Unify implement spine — absorb lead-edit and lead-write-code review

Goal: restructure `lead-implement` into a single unified spine where all
stages (Route → Prep → Edit → Review → Doc → Final → Merge) run as one path.
Direct-edit and delegated-edit are modes within the Edit stage; review is a
single stage with `judge: review-allocation` deciding reviewer depth and
partitions.

Pre-step (load-bearing check, verdict: preserve):
- Investigation 2026-05-21 found the "Review once" step's essence is `judge:
  review-scope` (lead-only for mechanical edits, 1-reviewer/2-cycle cap).
  This essence maps to `judge: review-allocation` Tier 1 lead-only and
  single-reviewer rows in the unified design.
- Re-confirm via `git log --oneline --grep "lead-edit"` at implementation
  time if new commits landed against `lead-edit` after the verdict date.

Scope:
- [done `009b1685`] Add invariant to `ws:lead-skill-authoring`:
  lead-not-own-executor.
- Restructure `lead-implement` into unified spine:
  - `On: invoke` stages: Route → Prep → Edit → Review → Doc → Final → Merge.
  - Replace `judge: execution-mode` with `judge: needs-delegation` (direct
    vs delegate to implementer agent).
  - Unify review into single `judge: review-allocation` with tiered decision:
    Tier 1 picks depth (lead-only / single-reviewer / partitioned); Tier 2
    picks partitions (correctness / fit / test) when partitioned.
  - Inline direct-edit flow (load context, edit, verify) as the direct-edit
    branch within the Edit stage.
  - Inline delegated flow: register and call implementer agent directly from
    the Edit stage (absorb `lead-write-code`'s implementer ceremony).
  - Single Review stage handles 0/1/N reviewers; relay cap 2 cycles for
    single reviewer, 3 cycles for partitioned.
  - Flatten labeled structural sections into judge-gated linear steps;
    each stage is a single numbered sequence, not forked by path.
  - Merge `judge: needs-brief` into `judge: plan-depth` as 4-level cumulative
    spectrum (none / brief / survey / research); brief fires on all paths
    when complexity warrants self-anchoring.
- Delete `ws:lead-edit` skill directory.
- Delete `ws:lead-write-code` skill directory; implementer agent prompt
  stays under `agents-plugin-tool/internal/wsprompt/prompts/`.
- Update `ws:lead-sprint` routing table: replace `ws:lead-edit` and
  `ws:lead-write-code` references with `ws:lead-implement` mode routing.
- Update `ws:lead-write-skeleton` next-route references.
- Update `ai-docs/spec/workflow-skills.md` — remove `lead-edit`, update
  `lead-write-code` entry to reflect absorption into `lead-implement`.
- Update mental-model anchors: drop `lead-edit` and `lead-write-code` from
  the code-and-review parity statement; update documentation pipeline
  ownership wording.

Sequencing:
- `260519-feat-implement-branch-squash-gate` touches `lead-implement`'s task
  list. Land Phase 2 first so squash gate is added to the unified spine.

Rejected alternatives:
- Keep `lead-edit` as a "mode marker" without execution semantics. Rejected:
  naming artifact without actor boundary.
- Route all implementation through delegated path. Rejected: round-trip cost
  for trivial single-file edits.
- Keep review bifurcated (`review-scope` in direct path, `partition-allocation`
  in delegated path). Rejected: produces two-code-in-one-skill shape instead
  of unified spine.
- Keep `lead-write-code` as thin implementer-wrapper skill. Rejected:
  implementer ceremony is short enough to inline; a wrapper skill with no
  review stage is just indirection.

Verification:
- Static review: zero orphan references to `ws:lead-edit` and
  `ws:lead-write-code` across skills, spec, mental-model, and `_index.md`.
- Confirm `judge: review-allocation` tiered table covers all prior review
  scenarios (lead-only, single-reviewer, 2-partition, 3-partition).
- Confirm `lead-sprint` routing table routes through `ws:lead-implement`.

### Result (3d48c1d9) - 2026-05-21

Commits: `009b1685..3d48c1d9` (7 implementation commits).

- Unified `lead-implement` spine: Route → Prep → Edit → Review → Doc
  Pre-Pass → Doc Commit Gate → Final Action Gate → Merge.
- Three judges gate all stages: `judge: needs-delegation` (direct-edit vs
  delegated), `judge: plan-depth` (4-level: none / brief / survey /
  research), `judge: review-allocation` (Tier 1 depth × Tier 2 partitions).
- Each stage is a single judge-gated linear sequence; no labeled structural
  sections or path forks.
- Deleted `ws:lead-edit` and `ws:lead-write-code` skill directories.
- Updated cascade: `ws:lead-sprint` routing table, `ws:lead-write-skeleton`
  next-route, `ws:lead-proceed` Announce template.
- Updated `ai-docs/spec/workflow-skills.md` and
  `ai-docs/mental-model/workflow-skills.md`; cleared 3 orphan
  `lead-write-code` references.

Deviations:
- `judge: plan-depth` merged planned `judge: needs-brief` into a 4-level
  cumulative spectrum; original scope only listed `needs-delegation` and
  `review-allocation` judges.
- `judge: branch-mode` was preserved from the original skill; scope did not
  list it but it was already present and needed no redesign.

### Phase 3: Batch — R1 invoke handler structure, R2 cross-skill judges ban, R4 negative-invariant evidence rule, R5 lead-owned freshness boundary

Goal: enforce four skill-authoring rules in `ws:lead-skill-authoring` in one
edit, then sweep affected downstream skills.

Scope (each item is its own commit inside the phase):

Skill-authoring wording cleanup:
- Rewrite local-debate phrasing in `ws:lead-skill-authoring` into standalone
  doctrine language: avoid negation-for-negation's-sake, hidden discussion
  shorthand, and unexplained internal terms.
- Add `fresh-reader audit` as the post-edit skill audit gate.
- Add `downstream consistency sweep` as the cross-surface follow-up gate for
  doctrine, terminology, route, layout, and audit-gate edits.
- Keep `lead-check-blockers` and `lead-verify-discussion` as compact fast-path
  checkpoints; classify full-ceremony rewrites for those skills as intentional
  differences unless output or end-state clarity is actually missing.

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
  `judge: needs-delegation` and `judge: branch-mode`).
- Remove `implementation-verdict-context` and implementation-verdict announce
  fields; verify no straggler references remain in full ws or wsflow proceed.
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

R5 — Lead-owned freshness and self-contained subquery boundary:
- Add to `ws:lead-skill-authoring`: active-conversation judgments stay
  lead-owned. Use `ws/subquery` only when the prompt is self-contained and the
  answer can be derived from explicit artifacts, source, specs, tickets, or
  other file-backed evidence. Do not delegate classification that depends on
  hidden conversation state.
- Remove `warmth` classification from `ws:lead-proceed`; it is an imprecise
  route variable. Replace `judge: ticket-freshness` with a lead-owned check:
  if the active conversation has settled decisions, constraints, rejected
  alternatives, or scope boundaries that are absent from the ticket, route
  through `ws:lead-write-ticket`; if none are missing, continue.
- If the lead cannot determine whether a decision is settled or missing from
  the ticket, stop through `ws:lead-discuss` or `ws:lead-check-blockers`
  instead of asking `ws/subquery` to infer unseen conversation context.
- Allow `ws/subquery` only for explicit evidence questions inside this flow,
  such as whether a ticket mentions a named contract, which spec anchor covers
  a behavior, or whether source/spec evidence makes a behavior caller-visible.
- Update mental-model anchor
  `{#260513-proceed-ticket-freshness-gate}` to reflect the lead-owned
  freshness check and the self-contained subquery boundary.

Verification:
- After all four sub-edits land, dogfood a discuss → proceed → write-ticket →
  proceed → implement chain end-to-end and confirm announces, audit trail,
  and ticket capture still work.
- Record line-count delta for `lead-proceed`, `lead-implement`,
  `lead-write-ticket`, and `lead-skill-authoring`; target is meaningful
  reduction in the first three without growth blowing up
  `lead-skill-authoring`.
- Confirm `ai-docs/_index.md` queue and inventory remain consistent.
