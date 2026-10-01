---
title: "review_phase lite tier as the default: one-round correctness and test-integrity review"
related:
  261001-feat-opt-in-design-and-phase-review: prerequisite (landed on develop, unreleased); this ticket revises its review_phase default and value domain
  260909-research-ws-refoundation-evidence-audit: binding anchor (topics worker interpreter, stop conditions); its 261001 addendum is updated here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0c8bc66eee1f37a4
sage-review-completeness-reviewed: 0c8bc66eee1f37a4
completed: 2026-10-01
---

# review_phase lite tier as the default: one-round correctness and test-integrity review

## Background

`261001-feat-opt-in-design-and-phase-review` added `review_phase` (`on|off`,
builtin `off`). `on` keeps the original risk-keyed allocation
(`single` or `partitioned: correctness, fit, test`) and the two-round
protocol. Re-examining the default right after that merge, the owner and the
lead concluded that `off` leans too hard on two things: the user reading the
diff, and the advisory `lead-review` sweep. Two reasons:

- **What a sprint-style human review replaces.** A human skimming a diff
  catches fit problems (conventions, structure, style) quickly. It rarely
  checks what test assertions actually prove, and it misses subtle
  correctness defects. Those are the implementer's structural blind spots:
  the same author writes and verifies, so tests confirm what was built
  (implementation-mirroring assertions, always-passing checks, mocks that
  bypass the real path). The `code-review-test` checklist (tautological
  assertions, unreachable asserts, mock integrity) targets exactly this.
- **This session's own evidence (n=1 each).** The design review caught a
  dead-by-default epic path that the discussion missed. The worker's
  round-1 code review caught an Important test gap the implementer missed.

The `lead-review` sweep is a weaker net for low-quota users: the
watermark nudge is advisory, low-quota users are the likeliest to skip
sweeps, and a range review reads a large diff at lower per-line density,
finding defects later when fixes cost more.

`review_phase` is not in any release yet (no tag contains the merge
`02f402d0d`; latest tag `v0.46.24`), so its value domain can change without
a compatibility alias.

## Decisions

1. **Value domain `off | lite | full`, builtin default `lite`.** `full` is
   exactly today's `on` behavior (risk-keyed allocation, two rounds,
   stop (e) after round 2). `off` is today's `off`. This reverses the
   prerequisite's rejected alternative "a `lite` floor ... as the default
   instead of `off`", by the owner's explicit re-decision (see Background).
   *Rejected: keep builtin `off`* - see Background.
   *Rejected: default `full`* - its cost (up to three reviewers, two rounds,
   per phase) is the cost problem `261001-feat-opt-in-design-and-phase-review`
   was opened for.
2. **`lite` is one fresh reviewer, one round, medium tier, covering the
   correctness and test-integrity checklists; fit is excluded.** Fit is the
   part a human diff skim replaces. Medium is enough because test deception
   is close to pattern recognition, and the goal is low-quota cost.
   *Rejected: large tier for lite* - better on subtle correctness, but
   against the cost goal; heavy users who want it pick `full`.
3. **`lite` ignores the risk partition.** Under `lite` the allocation is
   always the single lite reviewer, whatever the route facts say. An
   explicit `policy.review.override` of `single` or `partitioned` still wins
   over the knob (Decision 8 of the prerequisite, unchanged).
4. **No compatibility alias for `on`.** The knob is unreleased. A
   previously stored `on` is rejected at `config.tune` write time; at
   resolve time the existing rule stands (an unknown stored value
   normalizes to the builtin, now `lite`).
5. **Naming.** The new wrapper is `code-review-lite` and the allocation
   value is `lite`, following the existing `code-review-*` wrapper and
   allocation naming.
6. **Lite fix pass and stops.** Under `lite` the worker fixes Critical and
   Important findings in one pass with no re-review. A Critical the worker
   cannot fix is stop (e), so the elevation ladder stays reachable; an
   Important it does not fix goes to `unresolved:` with a reason. The worker
   judging "I could not fix it" is not grading its own fix as correct, so
   this does not reintroduce self-review.
   *Rejected: an unfixable Critical goes to `unresolved:` with no stop* - it
   would bypass the ladder that exists for exactly this case.
   The worker sees only the allocation, so the round protocol keys on the
   allocation, not the knob: allocation `lite` is one pass; `single` and
   `partitioned` keep two rounds even when an override applies under knob
   `lite`. The override enum stays `auto|single|partitioned`, so `lite`
   is not requestable through an override.
7. **The binding-anchor addendum is amended in place.** The prerequisite's
   dated addendum in the research anchor is not a frozen Result; it is
   updated to name `lite` as the default.

## Constraints

- Read every manual the `AGENTS.md` `### Implementation Conventions` rows
  match before editing (`shipped-surface-boundary.md`, `skill-authoring.md`,
  `wsflow-mirroring.md`, `ws-mcp.md`). Shipped text names no devenv-only
  path or ticket.
- Regenerate `agents-plugin-wsflow/rsrc/` and the manifests per
  `wsflow-mirroring.md`; do not hand-edit mirrors. Also resync the
  byte-identical `agents-plugin-pi/rsrc/` mirror and its `manifest.json`
  (guarded by `TestPiMirrorUpToDate`), as the prerequisite's commits did.
- `ticket-worker.md` and `ticket-worker-elevated.md` change together.
- The shared reviewer contract (`code-reviewer.md`), the existing
  `reviewer` and `code-review-*` wrappers, `lead-review`, and the design
  and completeness knobs are unchanged.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261001-feat-opt-in-design-and-phase-review (2026-10-01, Decisions): "Rejected: a `lite` floor (single medium-tier reviewer, one round) as the default instead of `off` - the owner judged per-phase review ceremony for high-intervention use given `lead-review` and the nudge." — bearing: contradiction-candidate
- 261001-feat-opt-in-design-and-phase-review (2026-10-01, Decisions): "`review_phase` (`off|on`, builtin `off`) governs the worker's per-phase independent review; `on` is today's risk-keyed allocation and two-round protocol unchanged." — bearing: constrains
- 261001-feat-opt-in-design-and-phase-review (2026-10-01, Decisions): "An explicit `policy.review.override` of `single` or `partitioned` still dispatches review when `review_phase` is `off`." — bearing: constrains
- 261001-feat-opt-in-design-and-phase-review (2026-10-01, commit 2585c658): "route.resolve_implement resolves review_phase under the caller's key (repo root included, parent-walk applies): off yields allocation none, NeedReview: false, and no review todo." — bearing: supports
- 260612-reviewer-allocation-tier-default (2026-06-12, commit 76568a41): "Reviewer-allocation tier default (workflow-skills.md #260612-reviewer-allocation-tier-default) documents correctness=large, fit/test=medium with recommended-tier single-source precedence." — bearing: supports
- 260611-refactor-ws-tier-taxonomy-delegate-tier-routing (2026-06-12, ticket Result 5023562c): "Single-reviewer path target resolved: single review -> the reviewer playbook; partitioned -> code-review-correctness/fit/test (each includes the code-reviewer base)." — bearing: supports
- 260828-refactor-per-slice-review-relay (2026-08-30, commit d819ab65): "session_state.go's generated review instruction landed the one-relay model (review #1 -> one repair relay -> closeout; Critical-only branch adds a Critical-scoped review #2 -> hard-stop)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/config_registry.go, agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin-tool/internal/mcp/session_state.go, agents-plugin-tool/internal/wsconfig/scope.go, agents-plugin/rsrc/ticket-worker/ticket-worker.md, agents-plugin/rsrc/ticket-worker-elevated/ticket-worker-elevated.md, agents-plugin/rsrc/worker-stop-protocol.md, agents-plugin/rsrc/lead-tune/lead-tune.md, agents-plugin/rsrc/code-review-lite/ (new), agents-plugin-wsflow/rsrc/ (regenerated mirror), agents-plugin-pi/rsrc/ (resynced mirror), ai-docs/tickets/idea/260909-research-ws-refoundation-evidence-audit.md |
| scope.surface | public-interface | review_phase's config.tune/config.list value domain and route.resolve_implement's allocation field gain a new lite value (agents-plugin-tool/internal/mcp/config_registry.go#L160-174, implement_resolver.go#L794-831) |
| scope.new_public_symbol | no | reuses the existing wsconfig.ItemReviewPhase key (agents-plugin-tool/internal/wsconfig/scope.go#L55); only its enum and resolved allocation strings change |
| scope.new_type_contract | no | deriveImplementReviewAlloc keeps its existing signature (agents-plugin-tool/internal/mcp/implement_resolver.go#L814) |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go, agents-plugin-tool/internal/mcp/session_state_test.go, agents-plugin-tool/internal/mcp/prompt_override_test.go, agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin-tool/internal/wsconfig/scope_test.go |
| complexity.reuse_points | confirmed | deriveImplementReviewAlloc's existing override/allocation branching (agents-plugin-tool/internal/mcp/implement_resolver.go#L794-831) and the code-review-correctness/code-review-test wrapper pattern (agents-plugin/rsrc/code-review-correctness/code-review-correctness.md, agents-plugin/rsrc/code-review-test/code-review-test.md) are both reused |
| complexity.side_effect_risk | moderate | flips the builtin default consumed on every route.resolve_implement review dispatch and the worker-installed review todo text |
| risk.correctness | moderate | lite must bypass the existing risk-keyed partition branch unconditionally (Decision 3) while leaving the override path and the full-tier table unchanged |
| risk.fit | low | extends the existing on/off knob shape and the established code-review-* wrapper/allocation naming (Decision 5) |
| risk.test | moderate | golden-string tests pin exact worker-stop-protocol/lead-tune/review-verdict prose this ticket edits, across the wsconfig, mcp, and rsrc surfaces, plus wsflow regeneration |
| risk.security_or_contract | moderate | Decision 4 drops the `on` value with no compatibility alias, so an existing stored review_phase=on config becomes an unrecognized value (Verification vi) |

## Phases

### Phase 1: lite tier and default

- `wsconfig` / `config.tune` registry: `review_phase` domain becomes
  `off|lite|full`, builtin `lite`; update the `config.list` description and
  `lead-tune.md`'s catalog line (it names `on`/`off` today).
- Resolver (`implement_resolver.go`): normalize to `off|lite|full`; `full`
  is today's `on` path; `lite` allocates a new allocation value for the lite
  reviewer (Decision 3). Update the review-phase verdict condition, the
  review-off and review-on instruction strings, and their golden tests.
  Existing tests that enable review switch from `on` to `full`.
- Installed review todo (`session_state.go`: `implementReviewTitle`,
  `implementReviewRoundsClause`, `implementReviewInstruction`): add a `lite`
  branch with a "Review (lite)" title and a one-pass / unfixable-Critical
  stop (e) clause; today an unknown allocation falls into the two-round
  default. Update `session_state_test.go`.
- New render wrapper `code-review-lite` (`kind: render`, `delegates: true`,
  `tier: medium`), including the shared `code-reviewer.md` contract. Its
  partition scope paragraph names both correctness and test integrity, and
  its checklist is the union of the `code-review-correctness` and
  `code-review-test` checklist items (copied, not re-derived).
- `ticket-worker.md`, `ticket-worker-elevated.md`: map the lite allocation
  to `code-review-lite`, and carve `lite` out of step 4's "Two rounds ...
  A Critical still open after round 2 is stop (e)" sentence (one pass, no
  re-review; an unfixable Critical is stop (e)).
- `worker-stop-protocol.md`: Review Rounds describes the lite round, and
  the stop (e) wording covers both the `full` case (Critical open after
  round 2) and the `lite` case (a Critical the worker could not fix)
  (Decision 6).
- Binding anchor `260909-research-ws-refoundation-evidence-audit`: amend
  the 261001 addendum so it says the default per-phase review is `lite`.

Verification: Go tests showing (i) builtin resolves `lite` and allocates
the lite reviewer regardless of route-fact risk, (ii) `full` reproduces the
previous `on` allocation table, (iii) `off` yields no review, (iv) an
explicit `policy.review.override` wins under each value, (v) `config.list`
shows `off|lite|full` with default `lite`, (vi) `on` is rejected as an
unknown value, (vii) the lite verdict's installed review todo states the
one-pass rule and the unfixable-Critical stop, (viii) an override of
`single`/`partitioned` under knob `lite` installs the two-round todo.
Full `go test ./...` for `agents-plugin-tool` and the plugin package tests
after wsflow and pi mirror regeneration. Doc check: the anchor's 261001
addendum names `lite` as the builtin per-phase review and no longer says
builtin `off`.

### Result (b3aefa1ef) - 2026-10-01

Landed as planned. `review_phase` is `off|lite|full` with builtin `lite`
(`config_registry.go` `reviewPhaseEnum`, `builtinConfigDefaults`, catalog
description, `wsconfig.ItemReviewPhase` doc). `normalizeReviewPhase` maps
unset/unknown (including the retired `on`) to `lite`;
`deriveImplementReviewAlloc` returns allocation `lite` before reading risk
facts, `full` runs the unchanged risk-keyed table, and an explicit
`single`/`partitioned` override still wins first. The installed review todo
has a `Review (lite)` title and a `code-review-lite` instruction with the
one-pass / unfixable-Critical stop / unfixed-Important `unresolved:` clause;
`implementNextAfterBranch` names "the one-pass lite review". New render
wrapper `agents-plugin/rsrc/code-review-lite/` (medium, includes
`code-reviewer`, correctness + test checklist items copied). Worker playbooks
(both) map `lite` and carve it out of the two-round sentence;
`worker-stop-protocol.md` describes the lite round and both stop (e) cases;
`lead-tune.md` names `off`/`lite`/`full`. Binding anchor 261001 addendum now
names `lite` as builtin. wsflow and pi rsrc mirrors and manifests regenerated.

Verification: `go test ./... -count=1` in `agents-plugin-tool` (all packages
ok); `python3 -m unittest discover agents-plugin-wsflow/tests` (14 ok);
`python3 -m unittest discover agents-plugin/tests` (76 ok);
`npm test` in `agents-plugin-pi` (1929 pass, 0 fail). Ticket checks (i)-(iv),
(viii): `TestResolveImplementReviewPhaseGatesAllocation`; (ii) also
`TestDeriveImplementReviewAllocProportionalPartitions` under `full`; (v)
`TestConfigTuningCatalogProjectsPromptAndSchemaKnobs`; (vi)
`TestEnterImplementReviewPhaseResolvesThroughParentSession`; (vii)
`TestDeriveImplementTodoInstructionsLiteReview`; wrapper scope
`TestRenderGoldenShippedReviewLiteCoversCorrectnessAndTest`. Independent
review (partitioned: correctness, test): both clean in round 1.

Decisions: correctness checklist item 6's trailing pointer to the Test
partition was dropped in the lite copy, since test integrity is in the same
scope; `session_auth_test` parent-walk fixtures switched their sample value
from `on` to `full` for vocabulary only.
