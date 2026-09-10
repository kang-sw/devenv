---
title: "Route ticket workers by high risk and reserve xlarge for escalation"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ee3ce6c079fe8b1b
sage-review-completeness-reviewed: ee3ce6c079fe8b1b
---

# Route ticket workers by high risk and reserve xlarge for escalation

## Background

Today the spawned ticket-worker's model tier is set by one free-text line in
`agents-plugin/rsrc/lead-run/lead-run.md` (Spawn step 3, ~L55-57): "Spawn one
worker of at least current-mainstream or previous-generation flagship class".
This is a fixed, ticket-independent floor. Two problems:

1. **Cost.** Every ticket's code authoring runs at flagship (>= large) tier
   regardless of phase difficulty, so the route cannot assign a cheaper worker
   when the reviewed ticket facts identify no high risk.
2. **Pattern divergence.** `ticket-worker.md:5` already declares `tier: large`
   frontmatter, which `playbook.render` surfaces as a `recommended-tier` and
   resolves to a concrete model (`playbook_tools.go:717-719, 283-310`). But
   `lead-run` ignores that recommended-tier for the worker spawn and uses its
   own flagship prose instead. Every other delegate spawn follows the
   render-recommends pattern (`lead-ticket.md:62`, `lead-workflow-manual.md:100-101`);
   only the worker spawn diverges.

The inputs for a risk-based route already exist and are already trusted: the
ticket's `## Route Facts` table carries `risk.correctness` / `risk.fit` /
`risk.test` / `risk.security_or_contract`, authored by `ticket-fact-populator`,
vetted by sage design+completeness review, and pinned by the sage-stamp digest.
The resolver already reads them and applies the deterministic predicate
`materialRisk(value) == (value == "moderate" || value == "high")`
(`implement_resolver.go:719-722, 776-799`) to decide review breadth. Worker
tiering needs a separate threshold: the user reports an observed model bias
toward `moderate`, so using the review predicate for author tier would still
route routine work to the large tier.

## Decisions

- **This ticket amends refoundation Decision 4.** The fixed
  current-mainstream/flagship minimum in
  `260909-epic-ws-worker-interpreter-refoundation` is replaced by the
  risk-based initial tier and xlarge escalation policy below.
- **Keep the three risk grades and separate author tier from review breadth.**
  `low|moderate|high` remains the fact vocabulary. Review allocation continues
  to treat `moderate|high` as material through `materialRisk`; worker tiering
  uses its own high-only threshold. This absorbs the observed `moderate` bias
  in authoring cost without weakening independent review.
- **Only an explicit `high` raises the initial ticket worker.** If any of the
  four `risk.*` facts is `high`, render the large worker; otherwise render the
  medium worker. This makes `low`, `moderate`, and `unknown` medium for worker
  tiering while leaving their existing review meaning unchanged.
- **Read ticket risk through the existing projection.** The lead point-resolves
  the selected stem with `tickets.query`, whose response already includes the
  Route Facts projection. It does not read or summarize the ticket body, and
  this ticket adds no MCP helper or public contract.
- **Ad-hoc contracts use a binary lead judgment.** Because an ad-hoc run has no
  ticket Route Facts, the lead classifies the user's contract as routine or
  difficult: routine renders the medium worker and difficult renders the large
  worker. No synthetic risk table or scoring step is introduced.
- **Use explicit playbook tiers, including xlarge escalation.** Follow the
  existing `implementer` / `implementer-elevated` precedent with medium and
  large worker playbooks, plus an xlarge escalation variant. Stop (e) raises a
  medium worker to large and a large worker to xlarge; a second stop (e) still
  escalates to the user under the existing protocol.

## Constraints

- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ticket-worker/ticket-worker.md, their agents-plugin-wsflow/rsrc/ mirrors, and new large and xlarge ticket-worker variants |
| scope.surface | public-interface | agents-plugin/rsrc/lead-run/lead-run.md#L30-L76 is a shipped lead entry playbook whose worker-spawn tier changes for every caller |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | tickets.query already returns Route Facts and playbook.render already returns resolved tier metadata |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2587-L2603 directly covers ticket-worker render tier; agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L80 covers rsrc mirror drift |
| complexity.reuse_points | confirmed | tickets.query Route Facts projection; implementer/implementer-elevated tiered-playbook precedent; existing small/medium/large/xlarge alias resolution in internal/wsconfig/config.go |
| complexity.side_effect_risk | moderate | changes the tier every future lead-run worker spawn resolves to, including the stop-(e) escalation re-spawn (lead-run.md:97-98) |
| risk.correctness | moderate | the lead must apply separate ticket, ad-hoc, and stop-(e) tier rules without conflating the high-only author threshold with materialRisk review routing |
| risk.fit | low | follows two already-verified precedents: the render-recommends pattern (lead-ticket.md:62, lead-workflow-manual.md:100-101) and the implementer/implementer-elevated two-tier split |
| risk.test | moderate | no existing test exercises risk-to-tier routing; the three playbook tiers and lead-run selection instructions need new or extended coverage in the test files named above |
| risk.security_or_contract | low | no MCP schema, key capability, or configuration vocabulary changes; the route uses existing generic Route Facts and render metadata |

## Phases

### Phase 1: Split the worker playbook and risk-route the spawn

Split `ticket-worker` into three rendered names: the base at `tier: medium`, an
elevated variant at `tier: large`, and an escalation variant at `tier: xlarge`.
Keep their bodies identical apart from tier frontmatter. For ticket targets,
have `lead-run` point-resolve the selected stem through `tickets.query` and
render large only when any projected `risk.*` value is `high`; render medium
otherwise. For ad-hoc contracts, have the lead classify the contract as routine
or difficult and render medium or large respectively. Replace the fixed
flagship prose with the render-recommends spawn pattern already used by other
delegates, and mirror all changes to `agents-plugin-wsflow`.

Update stop (e) to render the next explicit tier: medium to large, or large to
xlarge. Preserve the existing second-stop escalation to the user. Verify all
three rendered names return their declared tiers, ticket selection distinguishes
`high` from every other allowed risk value without changing `materialRisk`,
ad-hoc selection covers both classifications, stop (e) advances each initial
tier, and ws/wsflow copies remain byte-identical.

## Sage Review Round 1 (2026-09-10)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Escalation cannot raise an already-large worker | important | missing |
| 2 | Ad-hoc dispatch has no tier policy | important | missing |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Unresolved tier-routing mechanism | important |
