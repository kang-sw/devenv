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

(See research 260611 for the full taxonomy model.)

- **First-class tier = `small/medium/large/xlarge`** — the only tier abstraction
  skills, users, and the reviewer-allocation default speak; plane-neutral.
- **First-class axis = capability level** (resolved 2026-06-11), NOT a
  subscription/plan bracket: the tier names task-intrinsic reasoning depth, so
  frontmatter stays host/plan-neutral and cost/plan mapping lives in the
  layer-2/3 user config. Locked alias mapping: `light↦small`, `core↦medium`,
  `deep↦large`; `xlarge` (fable-class) has no legacy alias.
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

- ~~Blocked on research 260611~~ **Unblocked (2026-06-11):** first-class axis
  (capability level) and the `light↦small`/`core↦medium`/`deep↦large` mapping are
  resolved in research 260611. Ready for promotion (pending spec-address +
  slice-boundary refinement at promotion).
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
(`agent.go` hardcodes `opts.Tier = "core"` when no tier flows in). **Per-spawn
override path (resolved at promotion): frontmatter-only first** — the
frontmatter `tier:` (mapped first-class→alias) is the sole tier source for this
phase; a per-render `tier` override arg on `playbook.render` is explicitly
deferred to the `(skill, role) → tier` role-config surface in research 260611,
not built here. Remove the `Manager.oneShot()` / `oneShotOptions` dead
code (test-only despite the 2c Result's "live caller" note). Verification: an
e2e test that customizes light & deep via `config.agents_tier`, routes a
mercenary to each, and asserts the subprocess resolves to the custom
backend/model (not core). Closes 260609 Edition `0c7c0f50` gap 3 + the oneShot
flag.

### Phase 3: first-class tier vocabulary adoption + migration

Re-author the `judge: review-allocation` reviewer-tier default (dropped
`e6aadfc9`) in first-class vocab — **correctness→large, fit/test→medium** (under
the locked `deep↦large`, `core↦medium` mapping) — in the
`lead-implement` playbook + wsflow mirror + manifest regen; migrate spec and
mental-model text that frames `light/core/deep` as "the tier abstraction" to the
alias-layer framing. Adds the `(skill, role) → first-class tier` config override
surface only if research 260611 promotes that surface into this ticket.

## Spec Impact

**Target spec areas + caller-visible change (per phase):**

- **Phase 1** (shipped delegate asset) — no new contract. Exercises behavior
  already specified by `260610-mercenary-delegation-surface` (render-minted child
  keys) and `260609-playbook-tools` (`playbook.render` variable substitution);
  the asset just makes that behavior reachable on the shipped surface.
- **Phase 2** (per-spawn tier routing) — extends
  `260610-mercenary-delegation-surface` in `ai-docs/spec/mcp-tools.md`:
  caller-visible change is that a mercenary's model now resolves from its
  frontmatter `tier:` (first-class→alias→`config.agents_tier`) instead of being
  pinned to `core`. Frontmatter-only tier source (no new `playbook.render` arg).
- **Phase 3** (first-class vocabulary) — touches the alias-config spec
  (`260508-model-alias-config-tools` / `260513-harness-local-agent-tier-config`
  in `ai-docs/spec/mcp-tools.md`) and the reviewer-allocation default in
  `ai-docs/spec/workflow-skills.md`. Caller-visible change: a new first-class
  tier vocabulary `small/medium/large/xlarge` (capability axis) sits above the
  existing `light/core/deep` alias layer; frontmatter declares `role:` + `tier:`
  in first-class vocab; locked mapping `light↦small`/`core↦medium`/`deep↦large`.

**Contract-first spec: no.** The full tier-vocabulary contract (first-class set,
capability axis, alias mapping, frontmatter `tier:`/`role:`) is already captured
in this ticket's `## Decisions` and research `260611`; the surfaces being
extended are already spec'd (`260610`, `260508`, `260513`). The spec entries are
best authored at closeout against the concrete implemented anchors (exact field
names, the `config.agents_tier` first-class indexing). The doc-pre-pass
`lead-update-spec` run reconciles `mcp-tools.md` + `workflow-skills.md` within
each phase's commit range.
