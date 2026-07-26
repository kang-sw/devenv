---
title: "enter.* verdict output as an executable scenario — decision trace and per-axis prose fragments"
sage-review-design: blocked
related:
  260627-feat-enter-implement-deterministic-verdict-engine: substrate; authored the current raw verdict format and the "no long explanatory prose in raw output" rule this ticket deliberately reverses
  260627-feat-enter-proceed-deterministic-verdict-engine: substrate; authored the proceed route vocabulary and its preservation constraint
  260625-feat-ws-session-state-machine: introduced the enter.* mode-switch, agenda, and todo substrate this output rides on
  260630-bug-enter-implement-explicit-direct-edit-schema-undocumented: precedent that a schema-vs-actual input gap is a bug; the legacy enter.implement input path is the same class
  260702-feat-enter-implement-policy-feedback: adjacent prior pass over enter.implement output
  260630-epic-skill-playbook-diet: adjacent direction (remove prose that does not earn its cost) but explicitly NOT a parent — that epic diets hand-written playbook prose, this ticket reshapes MCP-generated prose
---

# enter.* verdict output as an executable scenario — decision trace and per-axis prose fragments

## Background

`ws/enter.implement` and `ws/enter.proceed` are the two deterministic verdict
engines in the enter.* family. Both return a flat field dump that reads as a
tool log rather than as instructions the lead can execute.

Observed defects in `renderImplementRaw`
(`agents-plugin-tool/internal/mcp/implement_resolver.go:800`) and
`renderProceedRaw` (`proceed_resolver.go:534`), which are structurally the same
renderer:

1. Verdict lines are bare enum tokens — `Mode: delegated`, `Plan Depth: survey`,
   `Review Allocation: partitioned`, `Doc Mode: standard`. Nothing states what
   the token means for this run.
2. `Conditions:` renders ~19 normalized resolver facts (`span=unknown`,
   `surface=internal`, `fit-risk=low`, …). This is an input echo, not guidance.
3. `Agenda:` restates verdict fields already printed above. In
   `renderImplementRaw`, `delegation`, `branch_plan.action`, `plan_depth`,
   `review_alloc`, and `doc_mode` each appear twice.
4. `implementReason` (`implement_resolver.go:796`) produces
   `delegation=…; plan-depth=…; review=…; surface=…; span=…; side-effect-risk=…`.
   A field named `Reason` that contains no reasoning.

The root cause of (4) is not formatting. `deriveImplementDelegation`
(`implement_resolver.go:543`) knows which branch fired — explicit direct-edit
override, explicit delegation request, automatic eligibility, or default
fallthrough — and returns only the resulting string. `automaticDirectEditEligible`
(`:556`) is a five-term AND whose failing term is exactly what a narrated
decision needs, and it is discarded. The same shape holds for
`automaticLeadOnlyReviewEligible` (`:564`) and `currentBranchImplementEligible`
(`:572`). A decision whose cause was discarded cannot be narrated no matter how
it is rendered.

`enter.proceed` is ahead here: `selectProceedRoute` (`proceed_resolver.go:467`)
returns a structured `route` label in a `<family>.<specific>` taxonomy
(`terminal-artifact.non-actionable-inline`,
`anchor-discussion.migration-anchor-conflict`,
`implementation-dispatch.ready-actionable`, `scope-gate.<blocked>`,
`fallback.insufficient-route-facts`). That label *is* a decision trace. Its
`reason` text, however, is still predicate echo.

## Decisions

- Reshape enter.* verdict output so the default text reads as an executable
  scenario: situation, decision with its cause, ordered acts each carrying its
  own how-to, and stop conditions.
- Scope is the two verdict engines only: `enter.implement` and `enter.proceed`.
- Keep the `format: "json"` capability. It has standing value for CLI-side
  testing. Whether it stays visible in the public JSON Schema is not settled by
  this ticket and is explicitly not the same class of problem as
  `260630-bug-enter-implement-explicit-direct-edit-schema-undocumented`.
  Retaining the capability is separate from `lead-implement` continuing to
  request it — see Constraints; the playbook stops requesting it in Phase 3.
- Model prose per axis, not per verdict tuple. Each enum value owns one authored
  fragment; the assembler owns ordering and connective tissue. Authoring a
  paragraph per verdict combination is rejected — the implement verdict spans
  roughly six near-independent axes.
- Genuine cross-axis interactions are declared in one enumerable table rather
  than expressed as nested switches. Measured inventory over the ten current
  instruction builders: `delegation × plan_depth` (edit step),
  `doc_mode × merge_confirm` (final-action-gate step), and
  `branch=current × delegation=direct-edit` (edit step). Eight of the ten acts
  read one axis or none; only the edit and final-action-gate acts are
  multi-axis.
- Treat `branch_plan.action == "stop"` as a global suppression mode applied once
  at assembly, not as an axis value. It currently appears as an `isBranchStop`
  guard in eight instruction builders (prep, edit, review, the three doc
  builders, final-action, merge), each with its own hand-written variant of the
  same sentence. Two acts are deliberately outside that suppression and must stay
  outside it: `implementRouteInstruction` handles stop as a `case "stop"` in its
  branch-action switch because the route act must *state* the blocker rather than
  be silenced by it, and `implementCompletionInstruction` has no stop handling
  because it is installed only for branch action `current`, which cannot be stop.
- Emit a decision trace from the resolver: which predicate branch fired, and for
  multi-term eligibility predicates, which term failed. This is a new output
  field, not a new decision rule.
- Judgment rules stay unchanged. No `derive*` or `*Eligible` predicate changes
  its verdict as a result of this ticket.
- Prose fragments live in Go string literals. Rejected: serving them from `rsrc`
  or exposing them as configurable prompt overrides — `rsrc` has no fragment-text
  serving capability today, and a per-fragment override surface would add roughly
  one management point per fragment with unpredictable maintenance cost.
- Because Go literals are the only review surface for this prose, fragments must
  be collected into a single declared block so the Go diff reads as a prose
  table. Scattering them back across switch bodies defeats the change.
- The scenario does not become a second execution list. The todo list stays the
  authoritative runbook per
  `260627-feat-enter-implement-deterministic-verdict-engine` Phase 2; the
  scenario is a briefing rendered from the same source so the two cannot drift.
- Demote `Conditions:` and the duplicated `Agenda:` block out of the scenario
  text into JSON only.

### Reversed prior decisions

`260627-feat-enter-implement-deterministic-verdict-engine` settled the current
raw format deliberately. This ticket overturns two of its rules; a future session
reading that ticket must not treat them as still binding:

- "Do not include long explanatory prose in raw output. […] MCP output should
  remain stable and easy to follow." Reversed: the scenario *is* explanatory
  prose, and the lead should not have to add its own explanation to act.
- "`Conditions` renders normalized facts used by the resolver." Reversed for the
  text surface only; the facts move to JSON.

Its other stability rules — first non-empty line, fixed verdict line ordering —
are broken deliberately in Phase 3 and preserved through Phases 1 and 2.

## Constraints

- `260627-feat-enter-proceed-deterministic-verdict-engine`: "The schema and
  rendered conditions must preserve the current `lead-proceed` route vocabulary
  wherever it is already load-bearing. Renaming or collapsing facts into broad
  buckets is not acceptable when the current playbook has a specific
  deterministic input or stop reason." Demoting `Conditions:` to JSON must
  preserve the vocabulary, not collapse it.
- Carrying the scenario in `raw` is **not** sufficient for the implement caller.
  `agents-plugin/rsrc/lead-implement/lead-implement.md:45` passes
  `format: "json"`, so `handleEnterImplement` (`session_state.go:1036-1039`)
  returns `json.MarshalIndent(result)` — the scenario arrives as a single
  JSON-escaped string in the `raw` field, and gets *worse* the more prose-like it
  becomes. `lead-implement` must stop passing `format: "json"` for the scenario to
  reach the lead as rendered text. The `format: "json"` capability itself is
  retained for CLI-side testing; only the playbook's use of it is dropped. This
  is the change the original request asked for ("make the rendered prose prompt
  the default"), and it lands in Phase 3 with the scenario, not earlier.
- `enter.proceed` is the only tool with a live text-reading caller today
  (`lead-proceed.md:29` passes no `format`). Any claim about improving what the
  implement lead actually reads depends on the playbook change above.
- `resolveImplement` populates `result.Raw` at `implement_resolver.go:493`,
  before the handler derives todos at `session_state.go:1022`. The renderer
  cannot currently see the todo instructions it must narrate. Plumbing must
  change before any scenario render is possible.
- `handleEnterImplement` (`session_state.go:996`) has two input contracts. The
  legacy branch (`session_state.go:1043-1067`) accepts `delegation`,
  `plan_depth`, `review_alloc`, `need_review`, and `need_doc` directly, bypassing
  the resolver entirely. It is gated purely on `args["target"]` presence. The
  schema's `"required": ["session_key", "target"]` is advertised but not enforced
  at dispatch, and `agents-plugin/bin/ws-cli` is a six-line shim that execs
  `ws-mcp-launcher.py` with arguments forwarded unchanged and no input-schema
  validation. **The legacy branch is therefore reachable** from `ws-cli` and from
  any non-validating MCP host. It must be kept, and the fragment assembler must
  record that verdicts arriving through it carry no decision trace. Do not remove
  it under this ticket — functionality deletion is always-ask per AGENTS.md.
- `deriveImplementPlanDepth` (`implement_resolver.go:610`) returns `survey` for
  delegated and `none` for every other path; its `ChangePoints`/`SideEffectRisk`
  test is dead because both remaining branches return `none`. The resolver
  therefore never emits `research`, while `implementPrepTitle`,
  `implementPrepInstruction`, and `implementEditInstruction` all carry `research`
  cases, and `parseLegacyImplementPlanDepth` rejects `research` on both legacy
  paths. Since this ticket forbids judgment changes, those fragments carry into
  the table as resolver-unreachable entries and must be labelled as such so a
  later reader does not mistake them for live combinations.
- Both packages ship a wsflow mirror; rsrc manifest and wsflow mirror
  regeneration apply to any shared playbook edit.

## Prior Art

- `proceedNextInstruction` (`proceed_resolver.go:349`) is the target shape for a
  closed-set authored fragment: it switches on a four-value enum and returns a
  complete imperative paragraph per case, interpolating only the namespace.
- `implementNextInstruction` (`implement_resolver.go:732`) is the anti-pattern:
  it glues verdict values into a sentence template, naming values without
  instructing.
- `implementRouteInstruction`, `implementPrepInstruction`,
  `implementEditInstruction`, `implementReviewInstruction`, and the doc/final/
  merge builders (`session_state.go:504-660`) already hold per-case authored
  prose. These are the fragments to be extracted, not rewritten, in Phase 1.
- `deriveProceedTodos` (`session_state.go:678`) carries titles only, with no
  `Instruction` fields. Fragment authoring for proceed is greenfield, not
  extraction.

## Out of Scope

- `enter.sprint` and `enter.salvage`. Excluded at user direction (2026-07-26):
  both are rarely invoked and their retention is being evaluated separately. They
  have no resolver, no facts, no verdict, and no `format` parameter, so they
  share nothing with the verdict engines beyond the `handleEnter` mode-switch. Do
  not extend this ticket to them without a new decision.
- Changing any verdict rule, predicate, or route selection outcome.
- Adding a configurable override surface for fragment text.
- Deciding whether `format` stays visible in the public schema.

## Spec Impact

Target spec areas:

- `ai-docs/spec/mcp-tools.md` — `enter.implement` and `enter.proceed` output
  contract: scenario text shape, decision-trace fields, and what moves from text
  to JSON only.
- `ai-docs/spec/workflow-skills.md` — `lead-implement` consumption changes in
  Phase 3: it stops requesting `format: "json"` and reads the rendered scenario
  directly.

Expected caller-visible change: the default text return of both tools changes
from a field dump to an ordered scenario; JSON gains decision-trace fields and
retains the normalized condition vocabulary; `lead-implement` receives rendered
text instead of a JSON document.

Contract-first spec: no. Behavior is pinned by the phases below; specs are
updated at implementation closeout to match what shipped.

## Phases

Phase 1 and Phase 2 are independent of each other. Phase 3 depends on both:
it cannot narrate causes that Phase 2 has not yet emitted, and it is not
reviewable until Phase 1 has separated mechanism from wording.

### Phase 1: Fragment table and assembler, wording unchanged

Extract the existing authored instruction strings into a single declared
fragment table keyed by axis and enum value, add the assembler that composes
them, and move todo derivation ahead of the raw render so the renderer can see
the instructions.

Required behavior:

- Collect every current instruction string into one declared block; do not
  reword any of them.
- Collapse the nine `isBranchStop` guard variants into one suppression fragment
  applied at assembly.
- Declare the three measured cross-axis interactions in one enumerable table.
- Move `result.Raw` population so it happens after todo derivation, or move todo
  derivation into the resolver; the renderer must be able to read todo
  instructions.
- Keep the legacy `handleEnterImplement` input branch (it is reachable — see
  Constraints) and record in the assembler that verdicts arriving through it
  carry no decision trace, so Phase 2 does not assume single-contract input.
- Label resolver-unreachable fragments (`plan_depth=research`) as such in the
  table.

Acceptance:

- The derived todo instruction set is byte-identical to pre-change output for
  every covered verdict combination, with one declared exception: the
  consolidated branch-stop fragment replaces the eight per-builder variants and
  its wording is chosen once.
- Raw text output is byte-identical with no exception; Phase 1 does not touch
  `renderImplementRaw` or `renderProceedRaw` content.

Verification:

- Golden tests over the **derived todo instruction set** — the surface the
  extracted strings actually reach. The instruction strings live only in
  `todoItem.Instruction`; `renderImplementRaw` never prints them and the
  response body is `result.Raw` or JSON, so a raw-text golden would pass even if
  every extracted fragment were corrupted. Cover delegation, branch action, plan
  depth, review allocation, doc mode, and merge confirm combinations.
- Separate raw-text goldens asserting byte equality for both tools.
- If todo derivation moves into `resolveImplement` rather than deferring the
  `Raw` write, `implementResult` gains a todos field and JSON output changes
  shape. Declare which option was taken and cover the JSON shape explicitly;
  byte-equality applies to text, not to the JSON document.
- Existing resolver, session-state, and playbook golden tests pass unchanged.
- rsrc manifest and wsflow mirror regeneration if any shared text moved.

### Phase 2: Decision trace emission

Make the resolvers report why each verdict was reached.

Required behavior:

- `deriveImplementDelegation` reports which of its four branches fired.
- `automaticDirectEditEligible`, `automaticLeadOnlyReviewEligible`, and
  `currentBranchImplementEligible` report which term failed when they return
  false.
- Equivalent trace emission for plan depth, review allocation, doc mode, and
  branch plan derivation.
- Build a cause statement from the trace and expose it as a new result field.
  Do **not** repoint `implementReason`/`result.Reason` at it in this phase:
  `renderImplementRaw` prints `Reason: %s` (`implement_resolver.go:818`) and
  `renderProceedRaw` prints it at `proceed_resolver.go:546`, so swapping the
  value would change text output. The `Reason:` line adopts the cause statement
  in Phase 3, where text output changes anyway.
- For `enter.proceed`, reuse the existing `route` label as the trace and derive
  its cause statement from the same taxonomy.
- No verdict changes. Every existing resolver test asserting a verdict value
  passes unmodified.

Verification:

- Unit tests asserting the trace names the correct firing branch or failing term
  for each predicate, including the multi-term eligibility predicates.
- Full existing resolver test suite passes with no expected-verdict edits.
- JSON output carries the trace and the cause statement; text output is
  unchanged, including the existing `Reason:` line.

### Phase 3: Scenario rendering and fragment authoring

Rewrite the fragments and the assembly into scenario form.

Required behavior:

- Compose situation, decision with its Phase 2 cause, ordered acts carrying
  their how-to, and stop conditions. Repoint the `Reason:` line at the Phase 2
  cause statement.
- Author proceed's fragments from zero, keyed on the existing `route` taxonomy.
  These are **not** installed as todo instructions — `deriveProceedTodos`
  (`session_state.go:678`) keeps its two route-independent title-only items, so
  this phase introduces no proceed todo behavior change.
- Remove `Conditions:` and the duplicated `Agenda:` block from text output;
  preserve the normalized condition vocabulary in JSON per the proceed
  constraint above.
- Drop `format: "json"` from `lead-implement.md:45` so the lead reads the
  rendered scenario instead of a JSON document, and adjust the surrounding
  consumption text (`lead-implement.md:60` currently says to read `raw`,
  `next_instruction`, and warnings). Mirror into wsflow.
- Keep the todo list as the authoritative runbook for implement; the implement
  scenario must be derived from the same fragments the todos use.

Verification:

- Golden tests over the new scenario text for each covered verdict combination.
- Anti-drift test for **implement only**: fails if a derived todo instruction has
  no corresponding scenario fragment. Proceed is excluded by construction — its
  todos carry no instructions and its authoritative next step is
  `NextInstruction`, not the todo list.
- JSON retains every normalized condition value present before this phase.
- `ai-docs/spec/mcp-tools.md` updated to the shipped output contract;
  `ai-docs/spec/workflow-skills.md` updated for the `lead-implement`
  consumption change.
- Full test suite, rsrc manifest regeneration, wsflow mirror regeneration.

## Blocked (2026-07-26)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | lead-implement reads JSON, so the scenario ships as an escaped string inside the field dump it replaces | critical | missing |
| 2 | Phase 1 acceptance test does not cover the surface Phase 1 refactors | important | autonomous |
| 3 | Phase 2 contradicts itself on text-output stability | important | autonomous |
| 4 | Phase 3 anti-drift invariant is unsatisfiable for enter.proceed | important | autonomous |
| 5 | branch-stop guard count is off and route's shape differs | minor | autonomous |
| 6 | legacy input branch is reachable; the removal instruction authorizes an always-ask deletion | minor | autonomous |
| 7 | plan_depth axis has an unreachable enum value the fragment table will encode | minor | autonomous |
