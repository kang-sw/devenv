---
title: "Opt-in design review and per-phase code review, with a non-convergence elevation stop"
related:
  260909-research-ws-refoundation-evidence-audit: binding anchor (topics stop conditions, worker interpreter); A7 and the closed stop list are revised here
  260909-epic-ws-worker-interpreter-refoundation: origin of the closed stop list (Decision 5), lead-side elevation (Decisions 4 and 6), and two-round review (Decision 20)
  260824-epic-review-watermark-model: review-altitude epic whose sweep/gate is the integration net this ticket leans on
  260831-research-ai-phase-over-granularity-review-load: review load = per-unit weight x unit count; this ticket removes the per-phase unit by default
  260915-research-lead-run-tier-selection-qualitative: sibling cost lever (worker tier over-elevation); deliberately not addressed here
---

# Opt-in design review and per-phase code review, with a non-convergence elevation stop

## Background

ws/wsflow went into company-wide use and users with small usage quotas burn
tokens at a very high rate. The owner attributes most of it to three
mandatory steps: fact population, the design-review Sage stage at `ready/`
promotion, and the per-phase independent code review every worker runs
(risk-keyed `single` or `partitioned: correctness, fit, test`, two rounds,
once per phase).

For high-intervention, sprint-style development the owner observes:

- Per-phase code review is replaceable by the user reading the change and
  either hot-fixing narrowly through `lead-delegate` or asking the same
  worker for a follow-up. `lead-review` (range sweep) already exists as the
  pre-merge whole review, and the review-watermark checkpoint nudge already
  recommends it on every project, track-less or not
  (`wsreview.CheckpointNudge`).
- Design review is replaceable by the developer catching mis-steps during
  ticket discussion.

Both stay valuable when the user hands most work to agents, so they become
opt-in knobs for heavy users rather than being removed. Fact population and
the completeness review stay on by default (completeness is a single fresh
read of one document and is cheap).

Evidence that shaped the decisions:

- The binding anchor's A7 ("independent review catches blind spots", `true`)
  rests on `260828` -> `260831`, where abort rates rose because the relay
  budget hard-stopped an unresolved Critical; that was not evidence that
  per-phase review must exist. Epic `260824` already places per-phase review
  as the light layer and the sweep/gate as the integration net. Independent
  review therefore survives as `lead-review`; only the per-phase unit becomes
  opt-in.
- Today the only trigger of the lead-side elevation ladder (`lead-run`'s
  "Retry after stop (e)" table) is stop (e), a Critical still open after
  review round 2. With per-phase review off, (e) can never fire and the
  ladder is dead.
- A worker that cannot converge ("a second failure with the same root cause
  as an earlier one in this run") currently reports stop (c), whose handler
  revises the ticket through `lead-ticket` and resumes the same-tier worker.
  Ticket revision is the wrong remedy for a capability shortfall; this is a
  latent defect that per-phase review currently masks.
- `sage_review` (`off|ask|auto`, builtin `auto`) already exists, but `off`
  skips design and completeness together, so it cannot express "completeness
  on, design off".
- Session-scope config does not reach a worker: `sessionStore.getOverride`
  reads only the given key's own record, while the review allocation is
  resolved by the worker under its child key. Session records already carry
  `Parent`.

## Decisions

1. **Two independent knobs, not one preset.** `sage_review_design`
   (`off|ask|auto`, builtin `off`) governs the design Sage stage;
   `sage_review` keeps its name and values and from now on governs only the
   completeness stage (builtin `auto`). `review_phase` (`off|on`, builtin
   `off`) governs the worker's per-phase independent review; `on` is
   today's risk-keyed allocation and two-round protocol unchanged.
   *Rejected: one preset knob (e.g. `workflow.autonomy: sprint|delegated`)
   toggling both* - the two are disabled for different reasons and must be
   tunable separately; a preset can be layered on later.
   *Rejected: a `lite` floor (single medium-tier reviewer, one round) as the
   default instead of `off`* - the owner judged per-phase review ceremony
   for high-intervention use given `lead-review` and the nudge.
2. **Defaults are opt-out.** Both new knobs default to `off` at builtin
   scope; heavy users opt in with `config.tune` at session, project, or
   global scope. This is a deliberate downstream-visible behavior change.
3. **Fact population stays mandatory** at `ready/` promotion, unchanged.
4. **New stop (f): non-convergence.** "A second failure with the same root
   cause as an earlier one in this run" moves from stop (c) to a new stop (f)
   in the closed stop list. Stop (c) keeps only "a ticket decision
   contradicted by code reality". `lead-run` handles (f) with the same retry
   ladder as (e) (medium -> `ticket-worker-elevated` large; large -> xlarge;
   xlarge -> user). The retry budget is shared: one retry across (e) and (f)
   combined, after which the next (e) or (f) goes to the user.
   *Rejected: fold non-convergence into (e)* - (e) is a review outcome and
   is unreachable when per-phase review is off.
   *Rejected: leave it in (c)* - (c)'s ticket-revision handler does not fix
   a capability shortfall.
   *Rejected: separate budgets for (e) and (f)* - allows two elevations in
   the worst case.
5. **Session overrides walk the parent chain.** For every session-scope
   knob, a key with no own override resolves through its `Parent` chain
   before falling to project scope; a key's own override always wins. This
   is a general config rule, not specific to `review_phase`, so a value the
   lead tunes for its session applies to the workers it spawns.
   *Rejected: lead-run passes the resolved `review_phase` to the worker
   explicitly* - a prose relay through the lead can be dropped.
6. **Stop (c)'s design-review gate is independent of `sage_review_design`.**
   A mid-run contract revision is small and corrects a ticket that already
   proved wrong, so it keeps its design review when the knob is `off`.

7. **No seeding from the old `sage_review` value.** An existing
   `sage_review=auto|ask` override does not imply design review after the
   split; a user who wants design review sets `sage_review_design`.
   *Rejected: fall back to an explicitly set `sage_review` when
   `sage_review_design` is unset at every scope* - it makes the design
   default depend on another knob's history and hides the split in
   `config.list`.
8. **An explicit `policy.review.override` of `single` or `partitioned`
   still dispatches review when `review_phase` is `off`.** The resolver
   already gives an explicit override precedence over derived allocation.
9. **No release-notes deliverable.** The default change is disclosed through
   the `config.list` knob descriptions and the merge commit body; the ship
   flow publishes an auto-generated GitHub release and this ticket does not
   add a release-notes surface.

## Constraints

- Shipped surfaces: read every manual the `AGENTS.md`
  `### Implementation Conventions` rows match before editing -
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`. Shipped text names no devenv-only path or
  ticket.
- `agents-plugin-wsflow/rsrc/` is generated byte-identical from
  `agents-plugin/rsrc/`; regenerate it and the manifests per
  `wsflow-mirroring.md` rather than hand-editing.
- `ticket-worker-elevated.md` carries the same review constraint and step as
  `ticket-worker.md`; both change together.
- `lead-review` (branch and range), the review-watermark nudge, the ledger,
  and the `lead-ship` release gate are unchanged.
- Out of scope: worker tier over-elevation (`260915`); the declared but
  unwired `ItemSageReviewDesignTier` / `ItemSageReviewCompletenessTier`
  constants; token telemetry.

## Phases

### Phase 1: Non-convergence stop (f) and session parent-walk

No default behavior changes in this phase.

- `worker-stop-protocol.md`: add (f) to the Stop List with the
  non-convergence wording; narrow (c); extend the Report block's `stop:`
  enum and the valid-pair rule to `a` through `f`.
- `ticket-worker.md` and `ticket-worker-elevated.md`: the same-root-cause
  sentence in Constraints now names stop (f).
- `lead-run.md`: the retry table header covers (e) and (f); add an (f)
  handler that reuses the (e) retry path, adding the report's failing
  verification line to the task block; one retry shared across (e) and (f).
- Config resolver: session-scope lookup walks `Parent` when the key has no
  own override (Decision 5). Guard the walk against a cycle or a missing
  record by stopping at the first unreadable link. Writes stay on the key
  that was given.
- Check whether any Go code validates stop letters and extend it if so.

Verification: Go tests in the config/session packages showing (i) a child
key with no own override resolves its parent's session override, (ii) a
child's own override beats the parent's, (iii) a grandchild resolves through
two links, (iv) a key without `Parent` behaves as before, (v) a parent cycle
terminates. Full `go test ./...` for `agents-plugin-tool`, plus the plugin
package tests (runtime-contract and skill-shim drift) after the wsflow
regeneration.

### Phase 2: Opt-in knobs and defaults

Depends on Phase 1: the default flip is safe only once (f) keeps the
elevation ladder reachable without review.

- `sage_review_design`: register in `wsconfig` and the `config.tune`
  registry with `off|ask|auto`, builtin `off`, the same scopes and wsflow
  visibility as `sage_review`. The Sage gate resolves the design stage's
  posture from `sage_review_design` and the completeness stage's from
  `sage_review`; the `ready-sage-posture` guardrail is unchanged (`skipped`
  is terminal). Update the per-ticket advisory text and the
  `config.list` descriptions so both knobs state what they govern.
- `review_phase`: register with `off|on`, builtin `off`, scopes
  session/project/global, wsflow-visible. `route.resolve_implement`
  resolves it under the caller's key (parent-walk applies). When `off`:
  `NeedReview: false`, review allocation `none`, and the installed todo
  carries no review step; an explicit `policy.review.override` of `single`
  or `partitioned` still dispatches review. When `on`: today's behavior.
  Rewrite the "always dispatched" invariant comments to match.
- `ticket-worker.md`, `ticket-worker-elevated.md`, `worker-stop-protocol.md`:
  independent review runs when the route verdict requires it; when it does
  not, the worker skips step 4 and states the skip in its Report
  `omitted:` field so the user knows to read the diff. The two-round
  protocol and stop (e) apply only when review ran.
- `lead-ticket.md`: describe design review as the `sage_review_design`
  stage; `lead-run.md` (c) handler states its design review runs regardless
  of that knob (Decision 6).
- Binding anchor `260909-research-ws-refoundation-evidence-audit`: append a
  dated addendum recording that A7's independent review now lives at the
  `lead-review` boundary by default with per-phase review opt-in, and that
  the closed stop list gained (f).

Verification: Go tests for (i) `review_phase` unset/`off` yields
`NeedReview: false` and no review todo, (ii) `on` reproduces the current
allocation table, (iii) explicit `policy.review.override` dispatches under
`off`, (iv) the Sage gate skips design and runs completeness at builtin
defaults, (v) `sage_review_design=auto` with `sage_review=off` runs design
only, (vi) `config.list` lists both new knobs with their domains. Full
`go test ./...` and plugin package tests after wsflow regeneration.
