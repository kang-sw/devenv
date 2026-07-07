# Plan: 260707-research-sage-review-staged-design-completeness-split — whole target

## Relevant Ticket Contract

- Split the single-pass Sage Review Gate into two sequential, non-looping
  stage gates keyed to ticket lifecycle: design-sketch review at `todo`
  (tolerant of missing detail, catches wrong direction), completeness review
  at `ready` promotion (checks implementation-readiness, undecided
  user-policy points, capture gaps).
- Reuse `lead-check-blockers`'s blocking/non-blocking framing as the
  completeness reviewer's rubric — no new reviewer role.
- Completeness reviewer's rubric needs an explicit scope-boundary check:
  distinguish a genuine completeness gap (`resolution: autonomous`-eligible,
  fill it) from a design-shaped gap in disguise (new public interface,
  cross-module change, architecture reshaping — must be `resolution: missing`
  and left unfilled).
- Frontmatter splits per stage: single `sage-review:` field replaced by two
  stage-scoped fields.
- Category exemptions: `epic` requires design-review stage only (exempt from
  completeness); `research`/`workset` stay exempt from both stages.
- Hard invariant: design review is never skippable regardless of entry path —
  a ticket that reaches `ready` without ever passing `todo` must still pass
  design review before or as part of completeness review; completeness
  review must refuse to run, or trigger design review first, if
  `sage-review-design:` has never completed.
- When a ticket's design content is edited after design review already
  passed, an agent should judge whether the edit is substantial enough to
  require re-running design review — judgment mechanism unspecified by the
  ticket (see Out of Scope).
- Non-goal: no stateful reset/re-trigger loop; no new dedicated "readiness
  reviewer" role; no second frontmatter track beyond the two stage fields;
  no shared re-review judge conflating human-edit vs. reviewer-fix contexts.

## Out of Scope

- The post-pass-edit re-review judgment mechanism ("should this edit
  re-trigger design review?"). The ticket explicitly leaves this
  unspecified and it is a distinct judgment procedure, not a gating
  mechanics change. Do not add a `judge:` for this in this phase; leave the
  existing behavior (no automatic re-trigger on ticket edit) unchanged. Flag
  it in the plan's own `## Deferred to Implementation` follow-up note left
  in the ticket, not solved here.
- Any change to the two reviewer *roles* themselves beyond wiring the
  scope-boundary check into `ticket-reviewer-completeness`.
- `260626-bug-sage-review-config-setter-missing` (no lead-facing
  `sage_review` config setter) — unrelated, not referenced by this phase.
- Any change to `config.show`/`sage_review` config resolution semantics
  (`off`/`ask`/`auto`) — only which stage(s) each resolved posture is
  stamped onto changes, not the vocabulary or resolution rule itself.
- Bulk-editing existing ticket files' frontmatter as a repo-wide migration
  pass. Migration is handled lazily at read time (see Codebase Findings and
  Implementation Plan) — no scripted rewrite of existing ticket files.

## Codebase Findings

### Current single-field mechanics (baseline being replaced)

- `ai-docs/spec/mcp-tools.md#260624-sage-review-gate` (lines 772-785) —
  caller-facing contract for the gate: single `sage-review:` field with
  values `skipped|recommended|required|completed|blocked`; gate runs
  identically at `todo/` and `ready/` landings; `idea/` bypasses.
  `ai-docs/spec/mcp-tools.md#260620-ticket-move-tool` (lines ~740-748)
  documents the Go-side promotion validation this ticket must also update.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`:
  - `ResolvedSageReviewPosture(sageReview string) string` (line 220) — maps
    config value (`ask`/`auto`/other) to posture (`recommended`/`required`/
    `skipped`). Pure function, category-agnostic today. **Reused as-is** for
    resolving each of the two new fields from the same config value; no
    change needed to this function.
  - `prepareSageReviewForUpwardMove(ticketAbsPath, sageReview, to string) (string, error)`
    (line 231) — single-field resolve-then-validate: stamps a resolved
    posture if missing/`pending`, blocks on `blocked`, and for `to == "ready"`
    requires a terminal posture (`completed`/`skipped`), erroring with a
    posture-specific message otherwise. This is the function that must
    become two-field aware.
  - `currentSageReviewPosture` (line 262) and `sageReviewPostureTip` (line
    272) — read/format helpers for the tip message appended to `tickets.move`
    results; both are single-field and need per-stage counterparts or
    parameterization.
  - `ticketCategoryRE` (line 158, `^\d{6}-([a-z]+)-`) and
    `exemptReadyGateCategories` (line 162, `{epic, research, workset}`) —
    **existing, directly reusable** category-detection mechanism, already
    used by `readyGateWarning` for the analogous spec-address-gate exemption.
    Reuse this same regex/category-extraction approach for sage-review-stage
    exemption instead of inventing new category detection.
  - `TicketsMove` (line 92) calls `prepareSageReviewForUpwardMove` only when
    `isUpwardMove` is true (line 116) — the sole call site needing the
    two-field branch logic.
  - `writeFrontmatterField` (line 308) is a generic multi-field setter
    (`map[string]string`) — already supports writing both new fields in one
    call; no change needed to this helper.
- `agents-plugin-tool/internal/wsdoc/ticket_create.go`:
  - `TicketCreate` (line 23) stamps a single `sage-review: <resolved>` line
    for `state == "todo" || state == "ready"` (lines 55-58). Needs to
    become category-aware and stamp `sage-review-design:` (and, only when
    the category requires it, `sage-review-completeness:` — see decision
    below on whether `tickets.create` at `ready` state also stamps
    completeness eagerly).
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`, `## On: Sage
  Review Gate` (lines 131-174) — single dispatch point, invoked from step 6
  of `## On: invoke` (lines 66-68) for both landing statuses. Steps 2-8
  resolve one posture; step 9 unconditionally spawns both reviewers in
  parallel; steps 10-11 aggregate; steps 12-13 write one field back. This
  entire section is rewritten to branch on landing status.
- `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md` (84
  lines, tier: large) and
  `agents-plugin/rsrc/ticket-reviewer-completeness/ticket-reviewer-completeness.md`
  (76 lines, tier: medium) — existing reviewer prompts, reused unchanged in
  role; only `ticket-reviewer-completeness.md`'s Checklist gets the new
  scope-boundary check item.
- `agents-plugin/rsrc/lead-check-blockers/lead-check-blockers.md` (9 lines,
  `kind: print`) — the framing to reuse. It is a short conversational
  checkpoint procedure (separate blocking design questions from autonomous
  hygiene/capture gaps), not a reusable module with a callable interface;
  inline its distinction (blocking-question vs. autonomous-hygiene-gap) as
  prose into the completeness reviewer's Checklist rather than having the
  reviewer prompt "call" it — the reviewer already emits `resolution:
  autonomous|missing` per issue, which is the same axis. No shared-module
  extraction needed.
- `agents-plugin/rsrc/manifest.json` (lines 46-47) — registers both reviewer
  playbook stems keyed by content hash. Editing
  `ticket-reviewer-completeness.md`'s body changes its hash and requires
  manifest regeneration even though no new file is added (`agents-plugin-tool`
  regen recipe: `ai-docs/mental-model/workflow-skills.md` "Add a delegate
  prompt to a workflow", `WS_REGEN_MANIFEST=1` then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run <regen-test>`
  — `-count=1` is mandatory).
- `agents-plugin-tool/internal/wsdoc/ticket_create_test.go` and
  `tickets_mutate_test.go` — existing Go test files covering
  `TicketCreate`/`TicketsMove`/`prepareSageReviewForUpwardMove` today; the
  natural home for new two-field test cases (single-field posture tests will
  need updating in place since the field they assert on is being replaced).

### Decisions this research settles (deferred by the ticket to implementation)

The ticket's own "Deferred to Implementation" section frames these as
implementation-time decisions, not open user-policy questions — each is
resolved below by direct precedent already in the codebase, not invented
from scratch.

1. **Frontmatter field names and value vocabulary.** Use
   `sage-review-design:` and `sage-review-completeness:`, each carrying the
   exact same five-value vocabulary as today's single field:
   `skipped|recommended|required|completed|blocked`. Rationale: this is a
   pure rename-and-duplicate of an already-specified, already-implemented
   vocabulary (`ResolvedSageReviewPosture` output range); inventing a
   different vocabulary per stage would add unjustified surface area with no
   ticket-stated need. Both fields resolve independently from the same
   single `sage_review` config value (`off`/`ask`/`auto`) via the same
   `ResolvedSageReviewPosture` function — the config axis is not being
   split, only which stage(s) a given ticket category stamps.

2. **Category exemption mechanism.** Reuse the existing
   `ticketCategoryRE` + category-map pattern (`exemptReadyGateCategories`)
   already used for the spec-address-gate warning. Add two new maps (or a
   small stage-requirement function) expressing:
   - `research`, `workset`: exempt from both `sage-review-design` and
     `sage-review-completeness` — neither field is stamped at any landing,
     mirroring today's blanket exemption these categories already get from
     the spec-address gate.
   - `epic`: `sage-review-design` is stamped/resolved at `todo` and required
     resolved before `ready`; `sage-review-completeness` is never stamped
     and never checked (epics don't reach `lead-implement`).
   - `feat`/`bug`/`refactor`/`chore` (the default/actionable categories):
     both fields apply as specified in the ticket contract.

3. **Never-skippable design-review enforcement mechanism.** Hard-block at
   the Go layer, mirroring how `prepareSageReviewForUpwardMove` already
   hard-blocks `recommended`/`required`/unresolved postures today (it does
   not silently auto-run review — it returns an actionable error and lets
   the caller decide). Concretely: `prepareSageReviewForUpwardMove`'s
   `to == "ready"` branch, for any category requiring completeness,
   first checks `sage-review-design`; if it is not a terminal value
   (`completed`/`skipped`), return an error directing the caller to run
   design review before promoting — do not auto-run anything from Go (Go
   has no dispatch capability). The playbook layer is the one that "runs it
   first": `lead-write-ticket`'s `## On: Sage Review Gate`, when landing at
   `ready`, checks `sage-review-design` before dispatching the completeness
   reviewer; if unresolved, it renders and runs `ticket-reviewer-design`
   inline as part of the same gate invocation (same mechanism as today's
   step 9a, just moved earlier/conditionally), writes the resulting
   `sage-review-design` posture, and only then proceeds to the completeness
   stage. This covers every entry path (`idea`→`ready` direct, or a ticket
   authored directly at `ready`) because both the Go validation and the
   playbook check the same field, not the traversal history.
   Rejected alternative: a Go-side implicit "treat missing design posture as
   auto-passed on first ready-promotion" shortcut — rejected because it
   would let `idea`→`ready` bypass design review silently, directly
   contradicting the ticket's stated hard invariant.

4. **Whether `lead-write-ticket` needs any change.** Yes, both layers need
   changes: Go (`tickets_mutate.go`, `ticket_create.go`) owns field
   resolution, category exemption, and hard blocking; the playbook
   (`lead-write-ticket.md` `## On: Sage Review Gate`) owns which reviewer(s)
   to dispatch at which landing status and writing back the per-stage
   result. Neither layer alone can satisfy the contract: Go cannot spawn
   reviewer subagents, and the playbook has no enforcement teeth against a
   direct `tickets.move` call that bypasses the playbook (today's
   `readyGateWarning` pattern for the spec-address gate shows this project's
   convention is exactly this split: soft/hard primitive-layer check +
   playbook-layer orchestration).

5. **Legacy `sage-review:` migration policy.** Lazy, read-time migration —
   no bulk rewrite. When any code path reads sage-review state for a ticket
   that has the old single `sage-review:` field but neither new field, treat
   the old value as authoritative for both new fields:
   - old `completed` → both `sage-review-design: completed` and
     `sage-review-completeness: completed` (a ticket that already passed
     sage review under the old model has, by definition, had both existing
     reviewer roles run against it — see ticket Background: both reviewer
     roles already exist and already ran together).
   - old `skipped` → both fields `skipped`.
   - old `blocked` → both fields `blocked` (must still be addressed).
   - old `recommended`/`required`/missing/`pending` → treat as absent for
     both fields and resolve each fresh via `ResolvedSageReviewPosture`,
     same as new-ticket stamping.
   This migration write happens the first time `prepareSageReviewForUpwardMove`
   or the playbook's field-read step encounters a ticket in this state, and
   both new fields are written (replacing the read-time inference with a
   persisted value) so the migration is self-healing without a separate
   script. The old `sage-review:` field is left in place on migrated tickets
   (not deleted) — removing it is a pure-cleanup, zero-behavior-change
   follow-up outside this phase's verification boundary. This ticket's own
   frontmatter (`sage-review: completed`) is one instance of this case and
   needs no manual edit; it will lazily migrate to
   `sage-review-design: completed` / `sage-review-completeness: completed`
   the next time it is moved or gated.

### Rejected shortcut paths

- Rewriting `mcp-tools.md`'s `sage-review:` value vocabulary to reuse a
  single field with a compound value (e.g. `design-completed`) instead of
  two fields — rejected: the ticket explicitly settles on "two stage-scoped
  fields," and a compound-value single field would still need the same
  branch logic while losing independent readability of each stage's state.
- Auto-passing design review on ready-promotion when the field is merely
  absent (treating silence as success) — rejected, contradicts the
  ticket's explicit hard invariant (see decision 3 above).
- A repo-wide scripted rewrite of every existing ticket's frontmatter as
  part of this phase — rejected as unnecessary scope; lazy read-time
  migration satisfies the same need without a one-off migration script that
  has no home in the workflow-repo's usual change shape.

## Implementation Plan

### 1. Spec contract update — `ai-docs/spec/mcp-tools.md`

- Replace the `{#260624-sage-review-gate}` section's single-field contract
  with the two-field contract: name both fields, their shared vocabulary,
  which categories get which field(s) (default categories: both;
  `epic`: design only; `research`/`workset`: neither), the never-skippable
  design-before-completeness invariant on `ready` promotion, and the lazy
  legacy-migration behavior for tickets carrying only the old
  `sage-review:` field.
- Update the `{#260620-ticket-move-tool}` section's promotion-validation
  paragraph (currently describes single-field terminal-posture requirement
  for `ready/`) to describe the per-field check: `ready/` requires
  `sage-review-design` resolved for categories requiring design, and
  `sage-review-completeness` resolved for categories requiring completeness.
- Update the `tickets.create` paragraph (`{#260622-create-ticket-tool}`) to
  state it stamps `sage-review-design` at `todo`/`ready` creation for
  categories requiring design, and does not stamp `sage-review-completeness`
  at creation time (completeness is only evaluated at `ready`-promotion
  time via `tickets.move`, not at direct `tickets.create(status: "ready")`
  — see step 3 below for the corresponding Go behavior this spec text must
  match).

### 2. Go: category-to-stage-requirement helper — `tickets_mutate.go`

- Add a small helper (near `exemptReadyGateCategories`, line 162) exposing,
  per category token, whether design and/or completeness apply — e.g. two
  boolean-returning functions or a small struct/lookup keyed the same way
  `exemptReadyGateCategories` is keyed (`ticketCategoryRE` match group).
  Default (unmatched category, or `feat`/`bug`/`refactor`/`chore`): both
  apply. `epic`: design only. `research`/`workset`: neither.
- Add a legacy-migration helper that, given a ticket's frontmatter map,
  returns the effective `(design, completeness)` posture pair — reading the
  new fields if present, else deriving from the old `sage-review:` value per
  the mapping in Codebase Findings decision 5.

### 3. Go: `prepareSageReviewForUpwardMove` two-field rewrite — `tickets_mutate.go` (line 231)

- Contract change: the function now resolves and validates up to two
  frontmatter fields (`sage-review-design`, `sage-review-completeness`)
  instead of one, gated by the category-requirement helper from step 2.
- For each required field: if effective posture (post-migration-read) is
  empty/`pending`, write the resolved posture (`ResolvedSageReviewPosture`)
  via `writeFrontmatterField`; if `blocked`, return an error naming which
  stage is blocked.
- For `to == "ready"`: if completeness is required for this category, first
  require `sage-review-design` to be terminal (`completed`/`skipped`) —
  return an error directing the caller to run/resolve design review first
  if not (this is the hard invariant enforcement point, decision 3). Then
  require whichever field(s) this category needs to be terminal, same
  per-posture error messages as today (`recommended`/`required`/other →
  stage-specific "run review" / "run or skip" errors).
- For `epic` promoted to `ready`: only `sage-review-design` is checked
  (no completeness requirement).
- For `research`/`workset`: function is a no-op (neither field touched,
  no validation) — same behavior as today's implicit exemption pattern in
  `readyGateWarning`.
- Return signature: needs to report both resolved postures (not one string)
  for `TicketsMove`'s tip-building call site (line 129) to describe both
  stages' state — extend the return type accordingly (e.g. a small struct,
  or two return values) rather than overloading the existing single string.

### 4. Go: `currentSageReviewPosture` / `sageReviewPostureTip` — `tickets_mutate.go` (lines 262-285)

- Extend to per-field variants (or parameterize by field name) so
  `TicketsMove`'s post-move tip (line 129-131) can report both stages'
  resulting posture, applying the migration-read helper from step 2 so a
  legacy-only ticket's tip still reflects the correct effective values.

### 5. Go: `ticket_create.go` `TicketCreate` — stamping at creation

- Contract: `TicketCreate` stamps `sage-review-design` (not the old
  `sage-review` field) at `state == "todo" || state == "ready"`, gated by
  whether the target category requires design (skip stamping entirely for
  `research`/`workset` — matching their full exemption). It does not stamp
  `sage-review-completeness` at creation, even for `state == "ready"`
  (completeness is a promotion-time concern owned by `tickets.move`'s
  upward-move path, and direct `tickets.create(status: "ready")` has no
  "from" state to have run a prior gate against — the same invariant-check
  path used by `tickets.move` should be reused here too, rather than
  skipped, so a ticket created directly at `ready` cannot bypass the
  never-skippable invariant: **call the same design-required-before-ready
  validation from step 3 inside `TicketCreate` for `state == "ready"`** and
  surface its error instead of silently succeeding).
- `TicketCreateOptions.Stem` already carries the category token
  (`YYMMDD-<category>-<slug>` is assembled by the caller before this point
  — confirm category is extractable the same way `ticketCategoryRE` extracts
  it from `stem` today) so the same category-requirement helper from step 2
  applies unchanged.
- Update the returned `Tip` text to describe the design-stage posture (and,
  where relevant, the ready-promotion invariant outcome) instead of the old
  single-field tip.

### 6. Playbook: `lead-write-ticket.md` `## On: Sage Review Gate` (lines 131-174) — full rewrite

- Contract: branch primarily on **landing status**, secondarily on
  **category** (design-required / completeness-required per step 2's
  mapping, mirrored in playbook prose since the playbook cannot call Go
  helpers directly — restate the same three-tier category table here as the
  spec text from step 1, so the playbook and the Go validation stay in
  sync; do not let the two drift into different category lists).
- `idea/` landing: skip entirely (unchanged).
- `todo/` landing, category requires design: resolve/read
  `sage-review-design` posture (steps 2-8 of today's procedure, same
  posture-handling rules — `skipped` bypasses, `recommended` asks,
  `required` runs, missing/`pending` resolves via `config.show()`), but
  dispatch **only** `ticket-reviewer-design` (drop the parallel
  `ticket-reviewer-completeness` dispatch for this landing). Write result
  to `sage-review-design:` only (`completed`/`blocked`), using the same
  `## Blocked` section template scoped to the design reviewer's table only.
- `todo/` landing, category does not require design (`research`/`workset`):
  skip the gate entirely, same as `idea/`.
- `ready/` landing (including a requested `todo/`→`ready/` promotion),
  category requires completeness (default categories): first check
  `sage-review-design` posture. If not terminal
  (`completed`/`skipped`/absent-and-then-resolved-and-run), run the
  design-review sub-procedure from the `todo/` branch above inline, in
  place, before continuing — this is the "trigger design review first"
  behavior satisfying the hard invariant for any ticket that reaches
  `ready` without a prior `todo` design pass. Then resolve/dispatch
  `ticket-reviewer-completeness` only (drop design from this stage's
  parallel dispatch — it already ran, either previously or just now).
  Write result to `sage-review-completeness:` only.
- `ready/` landing, category is `epic` (design-only): same design-check as
  above, dispatch only the design reviewer if not already resolved; no
  completeness stage at all for epics.
- `ready/` landing, category is `research`/`workset`: skip entirely (matches
  today's behavior — worksets are already blocked from `ready` entirely by
  the separate **On: Move** rule at line 78-79; `research` reaching `ready`
  is exempt from both stages the same as at `todo`).
- Aggregation (today's steps 10-11) still applies **per dispatched
  reviewer's own verdict** — since each landing now dispatches at most one
  reviewer (except the inline "run design then completeness" ready-path
  case, which is two sequential single-reviewer dispatches, not one
  parallel pair), the existing `block`/`concern`/`pass` aggregation rule
  collapses to: if a stage dispatches only one reviewer, that reviewer's
  own verdict directly becomes that stage's result (no cross-reviewer
  aggregation needed at all for single-dispatch landings). Keep the
  existing pairwise aggregation logic only for the specific
  ready-promotion-with-inline-design-then-completeness case, applied across
  the two sequential results.
- Legacy field note: the playbook, like Go, must read effective posture via
  the migration mapping (decision 5) rather than assuming the new fields
  always exist — same table as step 2, restated as playbook prose (frontmatter
  read step) since the playbook cannot call the Go helper directly.

### 7. `ticket-reviewer-completeness.md` — scope-boundary check

- Add one new Checklist item (after item 5, "Verification expectations"):
  a scope-boundary check distinguishing a genuine completeness/readiness gap
  (fill it, `resolution: autonomous`) from a design-shaped gap in disguise
  — new public interface, cross-module interaction change, or architecture
  reshaping — which must be raised as `resolution: missing` and left
  unfilled rather than patched under cover of a completeness fix. Frame this
  using the same blocking-vs-autonomous distinction
  `lead-check-blockers.md` already uses (a design question that needs user
  input, vs. hygiene/capture the reviewer/implementer can resolve alone) —
  do not add a cross-reference call to `lead-check-blockers` as a dispatched
  procedure; the completeness reviewer already only reads its own ticket
  file (per its Constraints, line 22) and has no mechanism to invoke another
  playbook mid-review.
- This is a Checklist/Doctrine wording addition, not a structural rewrite of
  the reviewer's Process or Output sections — those stay unchanged (still
  reads only the ticket file, still emits the same `verdict`/`issues`
  structure).

### 8. Manifest regeneration

- After editing `ticket-reviewer-completeness.md`, regenerate
  `agents-plugin/rsrc/manifest.json` and the wsflow rsrc mirror per
  `ai-docs/mental-model/workflow-skills.md`'s "Add a delegate prompt to a
  workflow" recipe: `WS_REGEN_MANIFEST=1` then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run <regen-test>`
  (`-count=1` mandatory). `ticket-reviewer-design.md` is unchanged in this
  phase and needs no regeneration unless step 7's wording changes are found
  to also warrant a design-reviewer-side note (not planned).

### 9. Version bump

- Per `AGENTS.md`'s dev-merge rule, this change bumps the plugin patch
  version via `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>` on
  merge into an integration branch or `main` — do not hand-edit version
  fields.

## Verification Plan

- Go unit tests (extend `ticket_create_test.go`, `tickets_mutate_test.go`):
  - `TicketCreate` at `todo`/`ready` for a default category stamps
    `sage-review-design` only (not `sage-review-completeness`, not the old
    `sage-review` field).
  - `TicketCreate` at `ready` for a default category with design
    unresolved/blocked returns an error (never-skippable invariant enforced
    at direct-to-ready creation).
  - `TicketCreate` for `research`/`workset` stamps neither field.
  - `TicketsMove` upward to `ready` for a default category: blocks when
    `sage-review-design` is not terminal, even if `sage-review-completeness`
    is terminal or absent; succeeds only when both required fields are
    terminal.
  - `TicketsMove` upward to `ready` for `epic`: only checks
    `sage-review-design`; ignores completeness entirely.
  - `TicketsMove` upward to `ready`/`todo` for `research`/`workset`: no
    field touched, no error from sage-review logic.
  - Legacy migration: a ticket with only `sage-review: completed` (no new
    fields) moving to `ready` is treated as both stages terminal and
    succeeds, with both new fields written as a result of the move.
  - Legacy migration: a ticket with only `sage-review: blocked` still blocks
    the move.
- Manual/scenario verification (post-impl, since this is agent-orchestration
  behavior the Go tests can't cover):
  - Author a `todo/` `feat` ticket through `lead-write-ticket`: confirm only
    `ticket-reviewer-design` is dispatched and only `sage-review-design` is
    written.
  - Promote that ticket to `ready/`: confirm only `ticket-reviewer-completeness`
    is dispatched (design already resolved) and `sage-review-completeness`
    is written; design field is untouched by this pass.
  - Author a ticket directly at `ready/` (or promote `idea/`→`ready/`
    directly) and confirm the gate runs design review first, then
    completeness, in the same invocation, and both fields end up written.
  - Exercise the completeness reviewer's new scope-boundary check against a
    ticket phase containing a disguised interface-shaped "completeness gap"
    and confirm it surfaces as `resolution: missing`, not silently
    autonomous-resolved.
- Spec verification: `specs.status`/manual read confirms
  `ai-docs/spec/mcp-tools.md`'s three touched anchors describe the two-field
  contract consistently with the Go behavior and the playbook behavior (no
  drift between the three).

## Escalations

- Confidence: medium-high on the mechanics above; the four vocabulary/
  mechanism/ownership/migration questions the ticket deferred are resolved
  above by direct precedent (existing category-exemption pattern, existing
  posture vocabulary, existing Go/playbook split-of-concerns) rather than by
  invented policy, and the ticket's own "Deferred to Implementation"
  section frames these as implementation-time decisions rather than
  open user-policy questions requiring a lead consult.
- One item is a genuine process/scope question for the lead, not a codebase
  fact: this phase spans a spec-contract rewrite, two Go source files
  (validation + creation), a full playbook-section rewrite, and a
  reviewer-prompt edit plus manifest regen — a materially larger single
  slice than the project's own sage-review precedent tickets
  (`260622-feat-sage-review-ticket-gate`, `260626-feat-surface-sage-review-posture`,
  `260703-chore-sage-review-builtin-default-on`), each of which shipped as
  one narrow concern. Recommend the lead decide, before execution starts,
  whether to keep this as one phase or split it (e.g. Go field-split +
  validation as Phase 1, playbook stage-branching as Phase 2, reviewer
  scope-boundary wording as Phase 3) — this plan is written as one coherent
  unit either way, since the steps above are naturally separable at Go vs.
  playbook vs. reviewer-prompt boundaries if the lead chooses to split them.
- The post-design-pass-edit re-review judgment mechanism remains explicitly
  out of scope per the ticket; no escalation needed there, it is a
  documented non-goal for this phase already reflected in Out of Scope.
