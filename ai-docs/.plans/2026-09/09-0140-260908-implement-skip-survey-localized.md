# Plan: 260908-feat-implement-skip-survey-for-localized-ticket-target — Phase 1: Four-fact skip condition, delegated stub plan, and spec

## Relevant Ticket Contract

- **Skip condition (Decision 1).** For a delegated **ticket** target,
  `deriveImplementPlanDepth` returns `plan_depth: none` when all four hold
  together: `change_points: clear`, `reuse_points: confirmed` or
  `not-applicable`, `strategy_shape: single-obvious`, `side_effect_risk: low`;
  any weaker value (including `unknown`) on any one keeps `survey`. The dead
  branch (`ChangePoints == "clear" && SideEffectRisk == "low"` — both arms
  already return `"none"`) is deleted, not revived. No new
  `route.resolve_implement` input field, enum value, or output field; this is
  a new value combination on existing fields only (Constraints).
- **Ticket targets only (Decision 3).** A delegated **inline** target always
  keeps `survey` — an inline contract has no file for a stub to point at.
  `deriveImplementPlanDepth` gains a target-kind parameter (`input.Target.Kind`
  is available at the call site) so this is enforced in the resolver, not the
  playbook.
- **Lead-written stub (Decision 2).** For delegated + `plan_depth: none`, Prep
  still allocates the plan path via `path.generate` and the lead writes a stub
  with all six headings (never an empty Implementation/Verification Plan):
  - `## Relevant Ticket Contract` — ticket path + selected phase heading only
    (same content the prerequisite ticket's Decision 2 already leaves there).
  - `## Out of Scope`, `## Implementation Plan`, `## Verification Plan` —
    each carries the line `Skipped: the ticket localizes the change (facts:
    change_points=clear, reuse_points=<value>, strategy_shape=single-obvious,
    side_effect_risk=low)`, where `<value>` is the actual `reuse_points` value
    (`confirmed` or `not-applicable`).
  - `## Escalations` — `None.`
  - `## Codebase Findings` — one line per binding constraint the Prep
    guardrails already had the lead read (mental-model lookup docs,
    `infra.read("impl-playbook")`, any declared `AGENTS.md` anchor doc), or
    `Skipped: no binding constraint from Prep reads` when none applies. Stated
    in terms of the generic Prep reads, not any one project's anchor
    (shipped-surface rule).
  - No planner dispatch for this case.
- **Go touch points named in Constraints:**
  - `deriveImplementPlanDepth` and its reason string.
  - `implementPrepInstruction`'s `none` case (currently worded only for
    direct-edit; needs a delegated variant naming the stub).
  - `implementEditInstruction`'s delegated `default` branch (already
    dispatches without a plan-readiness clause; must name `PlanPath`).
  - `parseLegacyImplementPlanDepth`'s `delegated` case (currently rejects
    `none` with "want survey"; must accept `none` too; `research` message
    unchanged).
  - Pinned tests: `TestResolveImplementDelegatedDefaultsToSurveyPlan`,
    `TestDeriveImplementTodoInstructionsDelegatedSurvey`,
    `TestDeriveImplementTodoInstructionsPrepGuardrails`.
- **No playbook changes.** `lead-implement` Prep names no plan depth; the
  stub step lives entirely in the Go instruction string.
- **Drift pin.** Add a Go test asserting the stub's section-heading set
  equals the `plan-populator-survey` template's heading set, read through the
  rsrc loader.
- **Spec/mental-model sentences to update** (Constraints, exact locations
  below): `ai-docs/spec/mcp-tools.md` `{#260625-session-state-tools}`;
  `ai-docs/spec/workflow-skills.md` `{#260505-implementation-workflow-skills}`
  (two sentences) and `{#260519-proceed-implementation-dispatch-precheck}`;
  `ai-docs/mental-model/workflow-skills.md` one line. `
  {#260505-proceed-routing-pipeline}`'s "decide delegated plan depth" clause
  stays true (verified below — no edit).
- **Verification boundary (ticket Phase 1):** `go test ./...` in
  `agents-plugin-tool/` with `-count=1`; a resolver test for the four
  strongest values -> `none`, weakening any one (incl. `unknown`) -> `survey`;
  a todo-derivation test for the delegated `none` Prep/Edit instructions; the
  existing survey-default test (facts left `unknown`) keeps passing. Manual
  dogfood run is a separate, non-code verification step.

## Out of Scope

- `260908-feat-survey-plan-is-route-not-contract` Phase 2 (reviewer/wrap-up
  ticket-diffing) — already landed and depended-on, not touched here.
- A lead-written inline stub — explicitly deferred by Decision 3 ("not
  decided here").
- Reviving the dead two-fact branch as-is — explicitly rejected by Decision 1.
- A new `ticket_localizes_change` fact — explicitly rejected by Decision 1.
- Any `agents-plugin/rsrc/*` playbook body edit — Constraints state "No
  playbook changes"; only the Go instruction strings change.
- The manual dogfood run named in the ticket's Phase 1 verification
  paragraph (requires a real ticket + live session; not part of the Go
  change/test loop this plan covers).

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L684-692` —
  `deriveImplementPlanDepth(n normalizedImplementFacts, delegation string) string`:
  current body is exactly the dead branch described in the ticket Background
  (`if delegation == "delegated" { return "survey" }`, then both remaining
  arms return `"none"`). Replace with a `targetKind string` third parameter
  and the four-fact condition gated on `targetKind == "ticket"`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L514` — sole call
  site: `planDepth := deriveImplementPlanDepth(n, delegation)` inside
  `resolveImplement`. `input.Target.Kind` is already in scope here (used at
  `#L506` for `target.Kind`) — pass it as the new third argument. No other
  call site exists (`grep -n "deriveImplementPlanDepth("` across
  `internal/mcp` returns only the definition and this call).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L647` —
  `currentBranchImplementEligible` already takes `targetKind string` as a
  parameter and compares it with plain `targetKind == "inline"` (no
  case-folding). This is the sibling pattern to mirror for the new
  `deriveImplementPlanDepth` parameter, i.e. compare `targetKind == "ticket"`
  directly rather than introducing a new normalization helper.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L957-977` —
  `implementConditions` already emits `"change-points="`, `"reuse-points="`,
  `"strategy-shape="`, and `"side-effect-risk="` into `result.Conditions`
  (an existing output field, populated from the same `normalizedImplementFacts`
  used by `deriveImplementPlanDepth`). This resolves the "reason string"
  plumbing question without any new output field: the lead already receives
  the concrete `reuse_points` value (and the other three facts) in the same
  tool response's `Conditions` list, so the delegated-`none` Prep instruction
  can simply tell the lead to read `reuse_points` off the verdict's
  `Conditions` when filling in the stub's "facts: ..." sentence — no plumbing
  through `implementTodoVerdict` is needed (contrast with `DocReason`, which
  *is* threaded through `implementAgenda`/`implementTodoVerdict` because the
  doc-mode instruction needs a value not otherwise exposed).
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L989-991` —
  `implementReason(n, delegation, planDepth, reviewAlloc)` builds
  `"delegation=%s; plan-depth=%s; ..."`; since `delegation` and `planDepth`
  are both already interpolated, `"delegation=delegated; plan-depth=none"`
  already discriminates the new skip case from `"delegation=direct-edit;
  plan-depth=none"` with no wording change needed to this function.
- `agents-plugin-tool/internal/mcp/session_state.go#L381-396` —
  `implementTodoVerdict` already carries both `TargetKind` and `Delegation`
  fields, both populated at the real call site
  (`agents-plugin-tool/internal/mcp/session_state.go#L1090-1101`,
  `TargetKind: result.Target.Kind`). `implementPrepInstruction` can branch on
  `verdict.Delegation` (not just `verdict.PlanDepth`) with no struct changes.
- `agents-plugin-tool/internal/mcp/session_state.go#L530-545` —
  `implementPrepInstruction`. Current `case "none", "":` (`#L536-537`) returns
  the direct-edit-only wording unconditionally. Needs to branch further on
  `verdict.Delegation`: `delegation != "delegated"` keeps the existing
  sentence; `delegation == "delegated"` returns the new stub instruction
  (allocate plan path via `path.generate`, write the six-section stub per
  Decision 2, explicitly "do not dispatch a planner"). The `guardrails` prefix
  (`#L534`) is unaffected and still prepended.
- `agents-plugin-tool/internal/mcp/session_state.go#L547-569` —
  `implementEditInstruction`. The delegated inner switch's `default:` case
  (`#L563-564`) currently reads `"Dispatch the delegated implementer with
  Delegate dispatch and the Implementer spawn prompt, using the resolved
  implementation context; capture the implemented commit range for review and
  relays."` — no mention of `PlanPath`, unlike the sibling `"survey"`/
  `"research"` cases (`#L559-562`, both say `render implementer with
  PlanPath`). This `default` case is genuinely reachable only once
  `deriveImplementPlanDepth` can return `"none"` for `delegation == "delegated"`
  (today delegated always resolves to `"survey"`, so this branch is presently
  dead for delegated targets, matching the ticket's "already dispatches
  without a plan-readiness clause" description). Reword to name `PlanPath`
  while dropping any plan-readiness clause (the stub has no escalation
  signal to wait on).
- `agents-plugin-tool/internal/mcp/session_state.go#L1150-1176` —
  `parseLegacyImplementPlanDepth`. `case "delegated":` (`#L1162-1172`):
  `"", "survey"` -> `"survey"`; `"research"` -> error (unchanged, per
  Constraints); `"none"` -> currently errors with `"invalid plan_depth %q for
  delegated: want survey"` (`#L1169`). Change so `"none"` also returns
  `("none", nil)` for delegated; leave the default/`"research"` branches as-is.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L211-253` —
  `TestResolveImplementDelegatedDefaultsToSurveyPlan` (pinned). Input sets
  `ChangePoints: clear`, `ReusePoints: confirmed`, `StrategyShape:
  single-obvious`, but `SideEffectRisk: moderate` — i.e. 3-of-4 strong,
  1 weak. This test's expected outcome (`PlanDepth == "survey"`) is
  unaffected by the new four-fact condition (side-effect-risk moderate ≠
  low), so this pinned test needs no assertion change; it continues to prove
  "survey by default whenever any fact is short of its strongest value."
  Confirmed passing today (baseline `go test` run, see Verification Plan).
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L459-499` —
  `TestResolveImplementSurveyEscalatesResearchFromSurveySignal` is the
  "existing survey-default test keeps passing with facts left `unknown`"
  case named in the ticket's Phase 1 last paragraph: `ChangePoints` is left
  unset (`unknown`), `ReusePoints: unconfirmed`, `StrategyShape:
  multiple-viable`, `SideEffectRisk: high`. Unaffected by this change; no
  edit needed.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L160-178` —
  `TestDeriveImplementTodoInstructionsDelegatedSurvey` (pinned) builds a
  verdict with `PlanDepth: "survey"` only; asserts the exact `prep`/`edit`
  strings for the survey case verbatim. Unaffected by adding a new
  delegated-`none` branch to `implementPrepInstruction`/
  `implementEditInstruction`, since this test never exercises `PlanDepth:
  "none"`. No edit needed — confirmed by running it (see Verification Plan).
- `agents-plugin-tool/internal/mcp/session_state_test.go#L180-239` —
  `TestDeriveImplementTodoInstructionsPrepGuardrails` (pinned) **will need an
  edit**. Its subtests share a single `Delegation: "delegated"` set at the
  loop body (`#L217`), while the `"none no declaration"` subtest
  (`#L191-196`, `depth: "none"`) asserts `wantTail: "Confirm the direct-edit
  facts are still accurate"` — the wording this ticket moves behind a
  `delegation != "delegated"` check. Once `implementPrepInstruction`
  branches on delegation for the `"none"` case, this subtest (as currently
  written) would instead receive the new delegated-stub wording and fail.
  The test case struct (`#L185-190`: `name, depth, anchorClause, wantTail,
  wantAnchor`) has no per-case delegation field today; fixing this requires
  either (a) adding a `delegation` field to the table and setting
  `"direct-edit"` for the `"none no declaration"` case specifically (to keep
  proving the *direct-edit* `none` wording still holds), or (b) splitting
  that one subtest out. Either way this is a required, ticket-authorized
  edit to a pinned test, not an ambiguity — the ticket's own Decision 2 states
  the `none` case "needs a delegated variant," which is exactly what breaks
  this subtest's current shared-delegation setup. A new subtest (or a
  parallel table) should be added alongside it asserting the delegated-`none`
  stub wording (headings, "Skipped:" lines, "do not dispatch a planner").
- `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md#L103-127`
  — both plan template variants ("survey is sufficient" / "research is
  needed") declare the identical six `##` headings, all indented 4 spaces
  inside the fenced fragments: `## Relevant Ticket Contract` (`#L105`,
  `#L130`), `## Out of Scope` (`#L110`, `#L135`), `## Codebase Findings`
  (`#L114`, `#L139`), `## Implementation Plan` (`#L117`, `#L142`),
  `## Verification Plan` (`#L120`, `#L145`), `## Escalations` (`#L123`,
  `#L148`). This is the "heading set of the `plan-populator-survey` template
  variants" the drift-pin test must read and compare against. Non-obvious
  parsing constraint: the template's *top-level* Process headings (`##
  Render Inputs`, `## Purpose`, `## Rules`, `## Process`, `## Doctrine`,
  unindented) are also literal `## ` lines in the same file — a heading
  scanner must select only the indented (4-space) `##` lines inside the two
  fenced code blocks under `### 3. Write`, or filter to the known six names,
  to avoid false drift-check failures.
- `agents-plugin-tool/internal/wsrsrc/loader.go#L92-151` — `wsrsrc.Load(root,
  name, harness string, vars map[string]string) (LoadedPlaybook, error)`
  is the rsrc loader; passing `vars == nil` (`#L134` guard) skips variable
  substitution entirely, so `wsrsrc.Load(rsrcRoot, "plan-populator-survey",
  "", nil).Body` returns the raw template text (with `{{.var}}` placeholders
  intact) — sufficient for a heading-set drift check with no need to supply
  the six declared render variables.
- `agents-plugin-tool/internal/mcp/prompt_override_test.go#L504` (and
  similarly `#L533`, `#L572`, `#L607`) and
  `agents-plugin-tool/internal/mcp/mercenary_surface_test.go#L495` — existing
  precedent for resolving the shipped rsrc root from a test in this package:
  `filepath.Join("..", "..", "..", "agents-plugin", "rsrc")` (repo-relative
  from `agents-plugin-tool/internal/mcp/`); `mercenary_surface_test.go#L495`
  wraps this in a small `shippedRsrcRootForTest()` helper. Reuse this pattern
  for the drift-pin test rather than `wsrsrc.ResolveRoot()` (which depends on
  `os.Executable()`/`WS_RSRC_ROOT` and is not test-friendly without an env
  override).
- `ai-docs/spec/mcp-tools.md#L319-322` — sentence to update: "The resolver
  derives `delegation`, `branch_plan`, `plan_depth`, `review_alloc`,
  `need_review`, and `doc_mode`, ... `plan_depth` is `none` for direct edit
  and `survey` for reachable delegated preparation" (the clause needing the
  four-fact exception is on `#L321-322`).
- `ai-docs/spec/workflow-skills.md#L774-777` — delegated-mode sentence:
  "Delegated mode: the lead selects ticket authority ... generates a plan
  path, dispatches a planner to write or refine that single implementation
  plan, spawns an implementer agent with the plan, and captures the
  resulting commit range." — gains the lead-written-stub no-planner case per
  Constraints.
- `ai-docs/spec/workflow-skills.md#L906` — "Plan population defaults to the
  survey planner for delegated mode." — the exact sentence Constraints name
  for the four-fact skip exception. Both this and the `#L774-777` sentence
  sit inside `## Implementation Workflow Skills {#260505-implementation-workflow-skills}`
  (heading at `#L746`).
- `ai-docs/spec/workflow-skills.md#L1132-1136` — binding-constraints
  sentence: "`lead-implement` also loads the project's declared binding
  anchor before editing when the target touches its declared topics.
  Delegated implementation has a required plan artifact; when the binding
  anchor is read, binding implementation constraints from the anchor are
  copied into the plan and the anchor is listed as a `[Must]` reference
  before plan population or implementer dispatch." — gains the stub's
  `## Codebase Findings` as the landing section for that copy. This sentence
  sits before the `{#260519-proceed-implementation-dispatch-precheck}`
  anchor marker at `#L1170` (anchors in this file mark the *end* of the
  addressed span, confirmed against the neighboring
  `{#260612-reviewer-allocation-tier-default}` at `#L870` and
  `{#260513-proceed-ticket-freshness-gate}` at `#L1122` patterns).
- `ai-docs/spec/workflow-skills.md#L1127` — "`lead-proceed` does not
  rejudge general ticket quality, mutate ticket structure, decide delegated
  plan depth, or invoke implementation primitives before `lead-implement`" —
  the `{#260505-proceed-routing-pipeline}` clause the Constraints say "stays
  true." Confirmed: this is still an accurate statement of `lead-proceed`'s
  role after this change (delegated plan depth is still decided entirely
  inside `lead-implement`/`route.resolve_implement`, never by
  `lead-proceed`) — no edit needed here.
- `ai-docs/mental-model/workflow-skills.md#L92` — "`lead-implement` defaults
  delegated preparation to `plan-populator-survey`; a survey
  `[escalate-to-research]` signal routes to `plan-populator-research`, ...
  while a survey or research `[escalate-to-lead]` signal is a scope-reduction
  decision surfaced directly to the lead rather than a research hop." —
  the line Constraints name; gains the four-fact skip as a stated exception.
- Shipped-surface constraint (repo `AGENTS.md` rule 4, applies to the new Go
  instruction strings, not to this plan file): the new
  `implementPrepInstruction` delegated-`none` wording must describe the
  Prep-guardrail reads generically ("the mental-model lookup documents,
  `infra.read("impl-playbook")`, and any declared `AGENTS.md` anchor
  document") and must not name any devenv-specific ticket stem, anchor
  topic, or file path — matching how `implementPrepInstruction`'s existing
  `guardrails` string (`#L534`) already phrases the anchor clause generically
  via `verdict.BindingAnchorClause`, itself produced by
  `wsreview.ReadAgentsBindingAnchor(...).PrepClause()` rather than any
  literal devenv text.

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/implement_resolver.go#L684-692`: change
   `deriveImplementPlanDepth` to `func deriveImplementPlanDepth(n
   normalizedImplementFacts, delegation, targetKind string) string`; keep
   direct-edit's behavior unchanged (`delegation != "delegated"` ->
   `"none"`); for `delegation == "delegated"`, return `"none"` when
   `targetKind == "ticket"` and all four facts hold at strongest
   (`ChangePoints == "clear"`, `ReusePoints == "confirmed" ||
   ReusePoints == "not-applicable"`, `StrategyShape == "single-obvious"`,
   `SideEffectRisk == "low"`), else `"survey"`. Delete the old dead
   two-branch body entirely.
2. `agents-plugin-tool/internal/mcp/implement_resolver.go#L514`: update the
   call to `deriveImplementPlanDepth(n, delegation, input.Target.Kind)`.
3. `agents-plugin-tool/internal/mcp/session_state.go#L530-545`
   (`implementPrepInstruction`): split the `case "none", "":` arm on
   `strings.ToLower(strings.TrimSpace(verdict.Delegation))`. Keep the
   existing direct-edit sentence for `delegation != "delegated"`. Add the
   delegated branch: instruct the lead to call `path.generate(kind: "plan",
   stems: [target stem or scope])`, then write the stub with the six
   sections from Decision 2 (ticket path + selected phase heading in
   `## Relevant Ticket Contract`; the "Skipped: the ticket localizes the
   change (facts: change_points=clear, reuse_points=<value>,
   strategy_shape=single-obvious, side_effect_risk=low)" line — telling the
   lead to fill `<value>` from the verdict's own `reuse_points` condition —
   in `## Out of Scope`, `## Implementation Plan`, `## Verification Plan`;
   `None.` in `## Escalations`; the binding-constraint carryover (or
   "Skipped: no binding constraint from Prep reads") in
   `## Codebase Findings`); explicitly state no planner is dispatched.
   Keep the `guardrails` prefix unchanged and still prepended.
4. `agents-plugin-tool/internal/mcp/session_state.go#L547-569`
   (`implementEditInstruction`): reword the delegated inner switch's
   `default:` case to render `implementer` with `PlanPath` and dispatch,
   mirroring the `"survey"`/`"research"` cases' `PlanPath` mention but
   without a plan-readiness/escalation-wait clause (e.g. "Render implementer
   with PlanPath and dispatch the delegated implementer; capture the
   implemented commit range for review and relays.").
5. `agents-plugin-tool/internal/mcp/session_state.go#L1150-1176`
   (`parseLegacyImplementPlanDepth`): in the `case "delegated":` block, add
   `"none"` alongside `"", "survey"` to return `("none"`/`"survey"` per the
   actual input, `nil`)` — i.e. `"", "survey"` -> `"survey"`, `"none"` ->
   `"none"`; leave the `"research"` error and the default branch unchanged.
6. `agents-plugin-tool/internal/mcp/session_state_test.go#L180-239`
   (`TestDeriveImplementTodoInstructionsPrepGuardrails`): give the
   `"none no declaration"` subtest an explicit `Delegation: "direct-edit"`
   (add a `delegation` field to the test table, defaulting to
   `"delegated"` for the other subtests to preserve their current
   assertions) so it keeps proving the direct-edit `none` wording. Add a new
   subtest (or sibling test) asserting the delegated `none` stub wording:
   headings named, "Skipped: the ticket localizes the change (facts: ...)"
   phrase present, "Skipped: no binding constraint from Prep reads" phrase
   present when no anchor/guardrail applies, and no planner-dispatch phrase
   (`plan-populator`) present.
7. `agents-plugin-tool/internal/mcp/implement_resolver_test.go` (near
   `TestResolveImplementDelegatedDefaultsToSurveyPlan`, `#L211-253`): add a
   new resolver test with a delegated ticket target and all four facts at
   strongest (`change_points=clear`, `reuse_points=confirmed` or
   `not-applicable`, `strategy_shape=single-obvious`, `side_effect_risk=low`)
   asserting `PlanDepth == "none"`; then, for each of the four facts in
   turn, weaken it (including to `unknown`) with the other three held at
   strongest and assert `PlanDepth == "survey"`. Also add an inline-target
   variant with all four facts at strongest asserting `PlanDepth == "survey"`
   (Decision 3: ticket targets only).
8. Add the drift-pin test (new test function, e.g. in
   `implement_resolver_test.go` or `session_state_test.go`): resolve the
   shipped rsrc root the way `mercenary_surface_test.go#L495`'s
   `shippedRsrcRootForTest()` does; call `wsrsrc.Load(rsrcRoot,
   "plan-populator-survey", "", nil)`; scan `pb.Body` for the six indented
   `## ` headings inside the two fenced "Write" blocks (or filter matched
   headings to the known six names to sidestep the top-level `##
   Render Inputs`/`## Purpose`/etc. headings); assert the resulting set
   equals the six literal heading strings the new delegated-`none`
   `implementPrepInstruction` branch names, so a future rename of any of the
   six headings in the template fails this test.
9. `ai-docs/spec/mcp-tools.md#L321-322`: reword "`plan_depth` is `none` for
   direct edit and `survey` for reachable delegated preparation" to state
   the delegated ticket-target four-fact skip as an explicit exception
   (`none` also for a delegated ticket target when all four complexity
   facts hold at their strongest value).
10. `ai-docs/spec/workflow-skills.md#L774-777`: extend the delegated-mode
    sentence so "generates a plan path, dispatches a planner to write or
    refine that single implementation plan, spawns an implementer agent
    with the plan" states the lead-written no-planner stub case for a
    delegated ticket target meeting the four-fact skip condition.
11. `ai-docs/spec/workflow-skills.md#L906`: extend "Plan population defaults
    to the survey planner for delegated mode." with the four-fact skip
    exception (mirroring the mental-model wording from finding above).
12. `ai-docs/spec/workflow-skills.md#L1132-1136`: extend the binding-anchor
    sentence so the copied-in binding constraints are stated to land in the
    lead-written stub's `## Codebase Findings` when the delegated target
    skips the survey planner.
13. `ai-docs/mental-model/workflow-skills.md#L92`: append the four-fact
    skip exception to the "`lead-implement` defaults delegated preparation
    to `plan-populator-survey`" line.
14. Run the verification commands below; fix any additional test breakage
    the four-fact/wording change surfaces beyond the two tests already
    identified.

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'TestResolveImplement|TestDeriveImplementTodoInstructions|TestParseLegacyImplementPlanDepth' -v -count=1`
- `cd agents-plugin-tool && go test ./... -count=1` (full suite, per the
  ticket's stated verification boundary)
- Manual (out of scope for this plan's code loop, named by the ticket as a
  separate verification step): dogfood `route.resolve_implement` on a ticket
  whose selected phase already enumerates its edit list, confirming `Plan
  Depth: none`, a stub plan is written, no planner is dispatched, and the
  implementer proceeds from the ticket on the "Skipped:" stub sections
  rather than bouncing the plan back as defective.

## Escalations

- None.
