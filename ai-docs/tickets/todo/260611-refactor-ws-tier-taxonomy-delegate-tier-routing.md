---
title: ws tier taxonomy (first-class small/…/xlarge) + delegate tier routing
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: re-homes its two Phase 2c Editions (delegate role/tier asset + per-spawn tier routing); all 260609 phases merged to epic
  260611-research-ws-per-role-delegation-tuning-config: owns the tier-taxonomy model + the open first-class axis this ticket depends on
  260513-harness-local-agent-tier-config: existing config.agents_tier (tier × harness → backend/model/effort) = the mercenary concretion layer this builds on
  260610-refactor-ws-wordchain-id-generalization: sibling per-call-concept generalization follow-up
related-mental-model:
  - prompt-bundle
  - named-agent-runtime
  - mcp-runtime
---

# ws tier taxonomy (first-class small/…/xlarge) + delegate tier routing

## Background

Re-homed fill scope. Ticket 260609 (M3) shipped the mercenary runtime and the
`playbook.render` child-key / model-var / `role:` mechanism, but three
delegate-surface behaviors are unreachable on the shipped surface — captured as
260609 Phase 2c Editions `379ff5e5` (child-key splice never fires + tier model
vars never surfaced) and `0c7c0f50` (per-spawn/per-role tier routing lost as
collateral of the register-schema removal; plus `Manager.oneShot()` dead code).
The reviewer-tier skill default (dropped commit
`e6aadfc9e42297ba97af6b78d98f46d2fa411d4b`) was deferred so its vocabulary lands
in the first-class taxonomy once rather than as soon-revised alias text.

All 260609 numbered phases are complete and merged to epic 260605 in the
combined `merge(m3)` commit `be8c39e6`; 260609 is closed and its remaining fill
scope lives here.

This ticket also realizes the tier-taxonomy redesign decided 2026-06-11 (full
model in research `260611-research-ws-per-role-delegation-tuning-config`): the
abstract delegation tier becomes first-class `small/medium/large/xlarge`, and
`light/core/deep` is demoted to a conventional alias at the concrete-model layer
(alongside `haiku`/`sonnet`/`opus`). Frontmatter declares the first-class tier;
mercenary delegation is opt-in.

## Decisions

(See research 260611 for the full taxonomy model and the open axis question.)

- **First-class tier = `small/medium/large/xlarge`** — the only tier abstraction
  skills, users, and the reviewer-allocation default speak; plane-neutral.
- **`light/core/deep` = conventional aliases** at the concrete-model layer, not
  the abstraction; a `light ↦ small` style mapping connects an alias to a
  first-class tier.
- **Frontmatter `role:` + `tier:`** declares the first-class tier only; never
  `backend`/`model`/`effort` (that is user config, not a per-playbook concern).
- **Mercenary is opt-in; native is default.** Native: the host harness owns
  model selection (ws injects only tier guidance + `{{.*Model}}` text).
  Mercenary: the first-class tier indexes `config.agents_tier`
  (`tier × harness → {backend, model, effort}`) → subprocess `--model`.
- The existing `config.agents_tier` surface is unchanged by the vocabulary
  split (it stays the mercenary concretion layer).

## Constraints

- **Blocked on research 260611** resolving the first-class axis (subscription
  plan vs capability level) and the first-class→alias mapping cardinality before
  ready promotion.
- Shipped-rsrc edits require manifest regen (see
  `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`).
- wsflow mirror: any shipped `lead-implement` / delegate playbook text change
  must mirror into `agents-plugin-wsflow` with no ws-only references.

## Phases

> Phase sketch (todo backlog); refine slice boundaries at ready promotion.

### Phase 1: shipped delegate playbook asset (child-key splice + model vars)

Add the missing shipped delegate playbook asset(s) under `agents-plugin/rsrc/`
with `role:` (implementer/reviewer) + first-class `tier:` frontmatter that also
declare and use the `{{.LightModel}}`/`{{.CoreModel}}`/`{{.DeepModel}}`
(alias-layer) model vars in their guidance text; regenerate the rsrc manifest;
add a shipped-asset e2e test asserting (a) the render-minted child-key
credential block fires on a real shipped playbook and (b) the model vars resolve
to the expected per-harness model strings. Existing unit tests pass on in-memory
fixtures and do not catch a missing shipped asset, so the shipped-asset e2e is
the key new coverage. Closes 260609 Edition `379ff5e5` gaps 1+2.

### Phase 2: MCP-reachable per-spawn/per-role tier routing (mercenary plumbing)

Thread a first-class tier from frontmatter (default) into the render-minted
child's `RegisterOptions.Tier` so a mercenary spawn resolves against the user's
custom `config.agents_tier` entry instead of being pinned to core
(`agent.go` hardcodes `opts.Tier = "core"` when no tier flows in). Decide the
per-spawn override path at ready promotion (a per-render `tier` arg vs
frontmatter-only first). Remove the `Manager.oneShot()` / `oneShotOptions` dead
code (test-only despite the 2c Result's "live caller" note). Verification: an
e2e test that customizes light & deep via `config.agents_tier`, routes a
mercenary to each, and asserts the subprocess resolves to the custom
backend/model (not core). Closes 260609 Edition `0c7c0f50` gap 3 + the oneShot
flag.

### Phase 3: first-class tier vocabulary adoption + migration

Re-author the `judge: review-allocation` reviewer-tier default (dropped
`e6aadfc9`) in first-class vocab (mapping pending the axis decision: e.g.
correctness→large, fit/test→medium under `deep↦large`, `core↦medium`) in the
`lead-implement` playbook + wsflow mirror + manifest regen; migrate spec and
mental-model text that frames `light/core/deep` as "the tier abstraction" to the
alias-layer framing. Adds the `(skill, role) → first-class tier` config override
surface only if research 260611 promotes that surface into this ticket.

## Spec Impact

Deferred to ready promotion. Per-spawn tier routing + frontmatter `tier:` are
caller-visible MCP/playbook behavior; the likely targets are the existing
`260610-mercenary-delegation-surface` and `playbook.render` (`260609-playbook-tools`)
stems plus a vocabulary-migration pass. Contract-first spec: TBD at promotion.
