---
title: "Route the ticket-worker tier by ticket risk (medium/large) instead of a fixed flagship floor, reusing the materialRisk predicate"
---

# Route the ticket-worker tier by ticket risk (medium/large) instead of a fixed flagship floor, reusing the materialRisk predicate

## Background

Today the spawned ticket-worker's model tier is set by one free-text line in
`agents-plugin/rsrc/lead-run/lead-run.md` (Spawn step 3, ~L55-57): "Spawn one
worker of at least current-mainstream or previous-generation flagship class".
This is a fixed, ticket-independent floor. Two problems:

1. **Cost.** Every ticket's code authoring runs at flagship (>= large) tier
   regardless of phase difficulty. Work that a medium-tier worker handled
   before the refoundation is now uniformly bumped up, so low-risk tickets pay
   large-tier dollars for no benefit.
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
(`implement_resolver.go:719-722, 776-799`) to decide review breadth. Nothing
maps them to a worker tier yet.

## Decisions

- **Risk grades are qualitative labels judged once, not a quantitative MCP
  score.** The route reuses the existing `materialRisk` string predicate; the
  only judgment is the populator's `low|moderate|high` label, frozen in the
  sage-stamped ticket body. No new scoring, no per-run model decision in the
  tier path. This is deliberately unlike the `related:`-marker dependence
  removed in `260910-chore-design-review-ready-inventory-contradiction-anchor`:
  there the judgment sat in the review loop each run; here it sits at populate
  time, reviewed and pinned.
- **Reuse `materialRisk`, add no new risk-to-tier rule.** Tier = large when any
  of the four `risk.*` facts is `moderate|high` (the same predicate that
  already gates the `correctness` review partition, `implement_resolver.go:778`);
  medium otherwise. Review strength and author tier then split from one
  deterministic risk read.
- **Follow the existing two-playbook precedent.** `implementer.md` (`tier: medium`)
  and `implementer-elevated.md` (`tier: large`) already model one role at two
  tiers as two distinct rendered names rather than a dynamic per-invocation
  tier. Mirror that for the worker rather than inventing per-call tier
  parameters in `playbook.render`.

## Constraints

- Convention: ai-docs/manuals/skill-authoring.md (declared for
  agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/,
  agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/).
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for
  agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/).
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ticket-worker/ticket-worker.md (plus a new elevated variant), mirrored agents-plugin-wsflow/rsrc/ counterparts |
| scope.surface | cross-module | spans rsrc playbook markdown (agents-plugin, agents-plugin-wsflow) and, if the Phase 1 open point resolves to a helper, agents-plugin-tool/internal/mcp/ Go source |
| scope.new_public_symbol | unknown | Phase 1's "Open implementation point" leaves helper-vs-lead-read undecided until execution |
| scope.new_type_contract | unknown | same open point; an MCP helper's request/response shape is not specified |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_render_surface_test.go covers render-tier surfacing; agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py covers skill-mirror drift |
| complexity.reuse_points | confirmed | materialRisk predicate (implement_resolver.go:798-800) and the implementer/implementer-elevated two-tier precedent (implementer/implementer.md:5, implementer-elevated/implementer-elevated.md:5) |
| complexity.side_effect_risk | moderate | changes the tier every future lead-run worker spawn resolves to, including the stop-(e) escalation re-spawn (lead-run.md:97-98) |
| risk.correctness | moderate | pre-spawn tier decision must reach Route Facts without the lead reading the ticket body, and the fetch mechanism (helper vs. lead-read) is still an open point |
| risk.fit | low | follows two already-verified precedents: the render-recommends pattern (lead-ticket.md:62, lead-workflow-manual.md:100-101) and the implementer/implementer-elevated two-tier split |
| risk.test | moderate | no existing test exercises risk-to-tier routing; the split playbooks and any new helper need new/extended coverage in the test files named above |
| risk.security_or_contract | moderate | the undecided helper option would add a new MCP tool contract; shipped-surface-boundary requires it ride only generic Route Facts + render metadata |

## Phases

### Phase 1: Split the worker playbook and risk-route the spawn

Split `ticket-worker` into two rendered names following the
`implementer` / `implementer-elevated` precedent: the base at `tier: medium`
and an elevated variant at `tier: large` (identical body, differing only in the
tier frontmatter). Wire `lead-run`'s Spawn step to select which name to render
from the ticket's `risk.*` Route Facts using the `materialRisk` predicate (any
`moderate|high` -> elevated, else base), and replace the fixed flagship-class
prose (`lead-run.md:55-57`) with the render-recommends pattern already used at
`lead-ticket.md:62`. Mirror to `agents-plugin-wsflow`. Fold in the stop-(e)
escalation: the "higher-tier worker" re-spawn (`lead-run.md:97-98`), which today
re-renders the same single `ticket-worker` and so does not actually raise the
tier, now renders the elevated name, giving the escalation real teeth.

Open implementation point to resolve during execution: the tier decision must
happen pre-spawn, at the lead. The refoundation deliberately keeps the lead
from reading ticket bodies (it hands the worker a pointer, not a digest), so
prefer a tiny deterministic MCP helper (given a ticket stem, return the
recommended worker tier by reading the sage-stamped `## Route Facts` and
applying `materialRisk`) over having the lead parse the ticket body itself.
Decide helper-vs-lead-read during execution and record the choice in the
Result.
