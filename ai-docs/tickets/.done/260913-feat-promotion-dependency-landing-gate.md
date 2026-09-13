---
title: "Promotion/dispatch dependency-landing gate with typed frontmatter edges"
related:
  260913-research-promotion-dependency-landing-gate: design-source — carries the settled Confirmed Decisions this ticket implements and the scoped-out proposal 5
  260909-epic-ws-worker-interpreter-refoundation: constraint — lead-run/selector are the shared worker-interpreter surface this touches
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ce737ee5354ac419
sage-review-completeness-reviewed: ce737ee5354ac419
completed: 2026-09-13
---

# Promotion/dispatch dependency-landing gate with typed frontmatter edges

## Background

A `ready/` ticket can be dispatched to a worker while a ticket it block-depends
on is still unlanded, and no reviewer catches it on the solo-promotion path: the
design reviewer treats `related:` as landed and only reasons about dependency
mistakes over an explicit batch, and the completeness reviewer does not evaluate
relations at all. The dependency's landed state is a point-in-time cross-ticket
scheduling fact, not a property of the reviewed ticket's text, so it cannot live
inside a content-hashed review without going silently stale. The root-cause
analysis, the decisive content-hash-staleness argument, and the confirmed
decisions this ticket implements live in the design source
`260913-research-promotion-dependency-landing-gate` (Outcome Ledger → Confirmed
Decisions).

This ticket implements the mechanically-decidable subset. It deliberately does
**not** implement the frontmatter-matches-prose consistency check (research
proposal 5): its owner — completeness reviewer vs. a promotion-time lint vs. the
gate — is contested because the completeness reviewer's charter deliberately
excludes `Relations:` evaluation, and that needs discussion first. That item
stays in the research ticket.

## Decisions

Confirmed with the user (2026-09-13); see the design source's Outcome Ledger.

- **Typed, optional frontmatter edge `blocked-by:` (new key, additive).** A
  blocking prerequisite a phase consumes is declared in a new frontmatter key
  `blocked-by:`, distinct from soft `related:` so the machine signal needs no
  prose parsing. Value shape resolves the phase-targeting question:
  - `blocked-by: <stem>` — the whole prerequisite ticket must be in `.done/`.
  - `blocked-by: <stem>#<phaseN>` — the prerequisite's phase N must carry a
    `### Result` (or the whole ticket be `.done/`). The edge, not prose, names
    *which* producer phase the landed-predicate checks — this is how the
    phase-granular interleave is addressed.
  The key is optional and additive: existing tickets without it are never broken
  and never migrated. (Exact spelling/format of the `#<phaseN>` suffix is this
  ticket's implementation choice.)
- **Graceful degradation when `blocked-by:` is absent.** The hard gate
  (`tickets.query` dispatch, `tickets.move` closure) fires ONLY on an explicit
  `blocked-by:` edge; absent it, there is no hard block — exactly today's
  behavior — because a hard refusal driven by prose inference would risk false
  blocks. The ticket-selector's ordering is advisory (a preference, not a block),
  so it keeps its EXISTING best-effort inference (its current
  prerequisite-preference over prose `related:` hints) as the fallback path and
  can still infer an order for legacy artifacts carrying no `blocked-by:`.
  Precise when declared, best-effort inference when not.
- **Landed-predicate is code-level, evaluated live.** A declared prerequisite
  counts as landed by the code-level predicate above (`.done/`, or the named
  phase carries a `### Result`) — never merely "the ticket left `todo/`", and
  computed live at the scheduling moment, never read from a content-hashed review
  stamp.
- **Gate-owner layer = ws runtime, bound to real tool calls (confirmed
  mechanism).**
  - **Dispatch-time hard gate → `ws/tickets.query` single-stem point-resolve.**
    lead-run always point-resolves the selected/named stem via `tickets.query`
    before spawning a worker — the one chokepoint both the selector path and the
    directly-named-ticket path pass through. The runtime computes, live from
    directory state, a `dispatch_blocked: {blocking_stem, reason}` field in that
    point-resolve projection (populated only in single-stem projection mode, not
    on every query, to avoid noise); lead-run honors it by refusing to spawn and
    reporting the blocker.
  - **Promotion closure → split between the tool and the playbook.**
    `ws/tickets.move(to: "ready")` is single-stem and batch-unaware, so the
    machine gate there enforces only the tool-visible subset: refuse when a
    `blocked-by:` prerequisite is neither in `ready/` nor `.done/`. The "or the
    same batch" case stays owned by lead-ticket's existing playbook closure — it
    evaluates the whole batch and moves prerequisites first, so a same-batch
    producer is already in `ready/` when the consumer moves. Do NOT encode a
    same-batch check into `tickets.move`: it has no batch handle, so that branch
    could never populate.
  - **Never bound to `ws/tickets.sage_stamp`.** The stamp is content-hash based;
    a scheduling fact bound to it passes at review and stays green while the
    dependency moves underneath — the exact silent-staleness failure the gate
    exists to prevent (design source: "Why (B) must NOT live inside a
    content-hashed review").
  - **Promotion-time advisory warning (soft, text).** For the legitimate
    in-between case — prerequisite in `ready/` (closure passes) but not yet
    code-landed — warn without blocking, preserving the "stage the consumer now,
    promote the producer imminently" workflow.
- **Design-reviewer solo-path advisory.** A non-blocking ordering finding on the
  solo promotion path, mirroring the batch path's existing dependency-mistake
  reasoning. Non-blocking because "promote the partner in the next batch" is a
  legitimate plan and conflating design quality with scheduling state produces
  noise.
- **Division of labor by prerequisite status (confirmed).** A prerequisite in
  `idea/`/`todo/` is caught at the promotion layer (`tickets.move(to:ready)`
  refuses the consumer); a prerequisite in `ready/`-but-unexecuted is allowed to
  promote (with the advisory warning) and caught at the dispatch layer
  (`tickets.query` gate) by the phase-granular code-level predicate; a
  prerequisite in `.done/` or whose consumed phase carries a `### Result` passes
  both.

## Constraints

- Whichever surfaces this edits, the typed-edge schema and gate semantics are a
  generic workflow-ordering concern and must stay host-neutral and
  downstream-first: they must not encode any devenv-specific rule, and no shipped
  playbook may be asked to enforce a rule from this repository's `AGENTS.md`
  (Architecture Rules 3–5). The path-scoped manuals
  (`shipped-surface-boundary.md`, `skill-authoring.md`, `wsflow-mirroring.md`,
  `ws-mcp.md`) apply per the root Implementation Conventions table and are copied
  into `## Constraints` at fact population for whichever paths this ends up
  touching.
- The landed-predicate must be evaluated at the scheduling moment (promotion,
  dispatch), never baked into a content-hashed review stamp, or it reproduces the
  silent-staleness failure it exists to prevent (design source: "Why (B) must
  NOT live inside a content-hashed review").
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- Reuse points to confirm at fact population: the selector playbook
  (`ws/rsrc/ticket-selector/…`) and its dependency-preference logic; the design
  reviewer's existing batch-path "dependency mistakes" reasoning
  (`ws/rsrc/ticket-reviewer-design/…`); the dispatch path in `lead-run`
  (`ws/rsrc/lead-run/…`) and its `session.note` assignment record; and
  `ws/tickets.query`'s existing `related:` / status projection as the substrate
  a gate reads.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-selector/ticket-selector.md, agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md, agents-plugin/rsrc/lead-run/lead-run.md (plus their agents-plugin-wsflow/rsrc/ mirrors), agents-plugin-tool/internal/wsdoc/tickets.go and sibling files, agents-plugin-tool/internal/mcp/server.go |
| scope.surface | public-interface | new dispatch_blocked field on tickets.query's single-stem point-resolve projection, a new refusal behavior on tickets.move, and a new typed frontmatter key are all caller-visible tool/schema contracts |
| scope.new_public_symbol | yes | dispatch_blocked field (tickets.query point-resolve output) and the typed frontmatter edge key (candidate blocked-by:) |
| scope.new_type_contract | yes | the dispatch_blocked: {blocking_stem, reason} object shape on tickets.query's point-resolve response |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go, agents-plugin-tool/internal/wsdoc/tickets_route_facts_test.go, agents-plugin-tool/internal/mcp/tickets_scope_test.go, agents-plugin-tool/internal/mcp/ticket_review_design_test.go already cover tickets.move, route-facts projection, and the design reviewer |
| complexity.reuse_points | confirmed | ticket-selector.md#L25-27 prerequisite-preference; ticket-reviewer-design.md#L145-148 batch dependency-mistake reasoning; lead-run.md#L74 session.note assignment; tickets.go#L461 relatedEntries projection |
| complexity.side_effect_risk | high | the gate binds into tickets.query and tickets.move, the universal dispatch and promotion chokepoints every ticket in this repo and every downstream ws install passes through |
| risk.correctness | high | the phase-granular landed-predicate must correctly find a ### Result on the specific consumed phase (not just ticket status) across arbitrary phase text |
| risk.fit | moderate | the ticket's own Constraints section requires the gate stay host-neutral and downstream-first (AGENTS.md Architecture Rules 3-5); no devenv-specific rule may leak into a shipped surface |
| risk.test | moderate | new coverage is needed for the point-resolve-only dispatch_blocked computation, phase-granular ### Result detection, and the ready/.done/same-batch promotion-closure boundary, though existing test files for these tools give a base to extend |
| risk.security_or_contract | high | tickets.move(to:ready) gains a new refusal condition, a behavioral contract change for every existing caller of the promotion tool |

## Phases

### Phase 1: Typed dependency edge + dispatch gate + promotion closure + selector ordering

Introduce the new optional `blocked-by:` frontmatter edge (value shape per
Decisions: bare `<stem>` = prerequisite `.done/`; `<stem>#<phaseN>` = that phase
carries a `### Result`), and make the runtime enforce it at two bound tool calls
plus feed the selector:
(1) **dispatch-time hard gate on `ws/tickets.query` single-stem point-resolve** —
the projection gains a live-computed `dispatch_blocked: {blocking_stem, reason}`
when the ticket's earliest unfinished phase has a `blocked-by:` prerequisite not
landed by the code-level predicate; populated only in single-stem projection
mode; lead-run honors it (refuse spawn, report blocker);
(2) **promotion closure on `ws/tickets.move(to: "ready")`** — refuse to move a
consumer to `ready/` while a `blocked-by:` prerequisite is neither in `ready/`
nor `.done/` (tool-visible subset only; the same-batch case stays with
lead-ticket's playbook closure per Decisions);
(3) the **ticket-selector** orders `ready/` from the `blocked-by:` edge when
present, and falls back to its existing best-effort prose inference when absent.
Every predicate is computed live at each call, never read from a review stamp.

Verify: a consumer whose `blocked-by:` prerequisite is unlanded gets
`dispatch_blocked` at point-resolve and lead-run refuses to spawn with the
blocking stem named; `tickets.move(to:ready)` refuses a consumer whose
`blocked-by:` prerequisite is neither in `ready/` nor `.done/` (a status-only
check — phase-granular `### Result` targeting is the dispatch gate's job, not the
promotion closure's); a consumer whose `blocked-by: <stem>#<N>` target phase
carries a `### Result` (prerequisite not fully `.done/`) is allowed at dispatch
(phase-granular interleave); **a ticket carrying no `blocked-by:` is neither
gated nor refused, and the selector still infers an order for it from prose
`related:` hints (no regression for legacy artifacts)**; a soft `related:` edge
never triggers the hard gate; and no gate path reads or writes a sage stamp.

### Result (a03be36) - 2026-09-13

Landed the typed `blocked-by:` edge and both bound gates.

- **Typed edge + parsing.** New optional `blocked-by:` frontmatter key, parsed
  on every projection into `TicketInfo.BlockedBy` (`agents-plugin-tool/internal/wsdoc/tickets.go`).
  `agents-plugin-tool/internal/wsdoc/tickets_deps.go` holds `blockedByEntries`
  (normalises scalar/list/map shapes), `parseBlockedByEdge` (bare `<stem>` =
  Phase 0; `<stem>#<phaseN>` suffix, accepting `#2` and `#phase2`), and the
  code-level landed-predicate (`edgeLanded`, `phaseResultPresent`,
  `phaseNumberOf`).
- **Dispatch gate.** `DispatchBlockFor` scans the whole board live
  (`resolveFull`, incl. `.done/`/`.dropped/` and index-hidden entries) and
  returns `DispatchBlock{blocking_stem, reason}` for the first unlanded edge. It
  is wired into the `tickets.query` single-stem point-resolve branch only
  (`agents-plugin-tool/internal/mcp/server.go`), surfaced in both text
  (`formatTickets`) and JSON via the `dispatch_blocked` omitempty pointer, and
  fails open on a scan error (by design; the promotion closure backstops the
  `idea`/`todo` cases). lead-run honors it (refuse spawn, report blocker); the
  tool description documents the field.
- **Promotion closure.** `blockedByPromotionError` in
  `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`, fired only for
  `TicketsMove(to: "ready")` before any write, refuses when a `blocked-by:`
  prerequisite is neither in `ready/` nor `.done/` (status-only subset).
- **Selector + mirrors.** `ticket-selector` orders `ready/` from the typed edge
  with prose fallback; `lead-run` and `ticket-selector` playbook edits mirrored
  to `agents-plugin-wsflow/` via the documented regen; both `manifest.json`
  files updated. `blocked-by:` documented in the bundled ticket-conventions.

Verification: `go build ./...`, `go vet ./...`, `go test ./...` (all packages
pass, incl. new `internal/wsdoc/tickets_deps_test.go` and
`internal/mcp/ticket_dispatch_gate_test.go`); `python3 -m unittest discover
agents-plugin-wsflow/tests` (11 tests OK). Independent review: partitioned
correctness/fit/test, round 1 then round 2 fix-verification — final verdict
clean (2 round-1 Important test-coverage findings fixed: sage-stamp
independence and multi-edge iteration; 1 fit Minor fixed: convention-doc entry
for the new key).

Decisions: dispatch gate computed in the MCP handler (not `readTicketFromBytes`)
so the field never appears on discovery listings; a malformed/absent
prerequisite blocks rather than being silently ignored, so a typo fails loud;
the fail-open-on-scan-error, first-unlanded-edge-only reporting, and
gate-runs-on-any-status point-resolve behaviors are accepted by design (recorded
as Minor in review).

### Phase 2: Promotion-time advisory warning + design-reviewer solo-path advisory

Add the softer, non-blocking layer on top of Phase 1's typed edge and gate. The
promotion-time advisory warning fires for the in-between case Phase 1's closure
deliberately allows — a typed prerequisite in `ready/` (closure passes) but not
yet code-landed — telling the promoter the producer is not executed yet, without
blocking the promotion. The design-reviewer solo-path advisory surfaces a
non-blocking ordering finding mirroring the batch path's dependency-mistake
reasoning. Both consume Phase 1's typed edge; both are text/advisory, never a
block.

Verify: promoting a consumer whose typed prerequisite is in
`ready/`-but-unexecuted emits the advisory warning and still completes the
promotion; the design reviewer on a solo promotion surfaces the ordering finding
as advisory (not a block); neither surface fires for a `.done/` prerequisite, a
consumed-phase-`### Result` prerequisite, or a soft `related:` edge.

### Result (2735b63) - 2026-09-13

Landed the softer, non-blocking layer over Phase 1's typed edge and gate.

- **Promotion-time advisory warning.** `blockedByPromotionWarning`
  (`agents-plugin-tool/internal/wsdoc/tickets_deps.go`) computes, live at
  promotion, the in-between case the Phase 1 closure allows: a typed
  `blocked-by:` prerequisite in `ready/` but not code-landed (not `.done/`, and
  any named producer phase carries no `### Result`). Wired into
  `TicketsMove(to: "ready")`'s post-move `to == "ready"` block
  (`tickets_mutate.go`), appended to the result `Tip` via the existing
  `appendTip`/`formatTicketMutate` channel (no `internal/mcp` change needed).
  Non-blocking (the move always completes) and fails open on a scan error, since
  the dispatch gate backstops the hard cases. Reuses Phase 1's `edgeLanded`
  predicate; the shared board scan was extracted into `boardByStem`, now used by
  both the dispatch gate and this warning.
- **Design-reviewer solo-path advisory.** New checklist item 6 in
  `agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md`
  (mirrored byte-identical to `agents-plugin-wsflow/`): on the single-ticket
  path, an unlanded typed `blocked-by:` prerequisite yields one `minor`,
  `resolution: autonomous` ordering finding that never raises the verdict,
  mirroring the batch path's dependency-mistake reasoning. Kept host-neutral
  (names only the generic `blocked-by:` key and `tickets.query`); worded to
  avoid the literal `related:` token so the existing checklist-anchoring guard
  stays green.

Verification: `go build ./...`, `go vet ./...`, `go test ./...` (all packages
pass, incl. new `TestBlockedByPromotionWarning`,
`TestTicketsMovePromotionAdvisory`, and
`TestTicketDesignReviewDependencyLandingAdvisory`); `python3 -m unittest discover
agents-plugin-wsflow/tests` (11 tests OK); rsrc manifest + wsflow mirror
regenerated (`WSRSRC_REGEN`, `WS_REGEN_WSFLOW_RSRC`). Independent review:
partitioned correctness/fit/test, round 1 (correctness clean, fit clean, test 1
Important + 2 Minor) then round 2 fix-verification — final verdict clean. The
Important (missing test for the design-reviewer advisory) and both Minors
(multi-edge join, malformed-edge-silent) were fixed.

Decisions: the promotion warning fails open on a scan error and stays silent on a
malformed/absent edge (the closure and dispatch gate already own those cases), so
the soft layer never double-reports; the design-reviewer advisory is `minor`, not
a block, by ticket decision (conflating design quality with scheduling state
produces noise, and "promote the partner in the next batch" is legitimate).
