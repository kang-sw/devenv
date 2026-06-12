---
title: ws tier taxonomy (first-class small/…/xlarge) + delegate tier routing + delegate-prompt convergence (wsprompt retirement)
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

**Expanded scope — delegate-prompt convergence + wsprompt retirement (confirmed
2026-06-12).** Investigating the Phase 1 shipped delegate asset surfaced that M3
Phase 2c already converged the *mercenary* delegate prompt onto rsrc
`playbook.render` (it removed the `agents.register(prompts:[stems])` MCP schema —
`server.go:2334` now reads "the former prompts/tier/model registration fields are
removed; use a self-contained prompt from playbook.render"). But the migration is
incomplete and the two prompt trees still duplicate delegate content:

- The shipped `lead-implement` playbook (and peers) still instruct the lead to
  call `ws/agents.register(name: …, prompts: […])` (lines 61/64/91/102) — a now
  semantically-stale schema field; the self-contained prompt must instead come
  from `playbook.render`.
- `agents-plugin-tool/internal/wsprompt/prompts/` still holds the canonical
  delegate prompt bodies (`implementer.md`, `code-reviewer.md`,
  `reference-discovery.md`, `mental-model-updater.md`, `plan-populator-survey/
  research.md`), duplicated against the rsrc delegate playbooks this ticket adds.

The `explore` rsrc playbook is the existing precedent (skills already delegate it
via `playbook.render`). The confirmed direction completes that pattern for every
delegate, then retires the `wsprompt` (go:embed) loader entirely — including its
remaining non-delegate consumers (`api.ask` hard-coded stems, the wsflow
`prompt.render` `RenderSource` loader) — so `agents-plugin/rsrc/` becomes the
single prompt source of truth. Mental-model `prompt-bundle.md` line 27
("deliberately parallel, non-overlapping loaders") is 2c drift, not a binding
constraint, and is rewritten at closeout.

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
- **Delegate-prompt convergence (confirmed 2026-06-12).** Every delegate prompt
  (implementer, reviewer family, reference-discovery, mental-model-updater,
  plan-populator-survey/research) becomes a canonical rsrc playbook rendered via
  `playbook.render`; skills delegate by rendering the playbook (mint-injected
  child key + self-contained prompt) and spawning native/mercenary, never via
  `register(prompts:[stems])`. The shipped delegate asset content (incl. Phase 1
  implementer/reviewer) is a **port of the canonical `wsprompt/prompts/` body**,
  not invented text.
- **wsprompt retirement is the end state (confirmed 2026-06-12), full not
  partial.** The `wsprompt` go:embed loader is removed once all consumers move
  off it: delegate prompts → rsrc playbooks; `api.ask` hard-coded stems → rsrc;
  wsflow `prompt.render` `RenderSource` → rsrc. Temporary two-tree duplication
  during the migration is accepted; the embedded copy of a delegate prompt is
  deleted only after both the skill migration (Phase 5) and the loader retirement
  (Phase 6) remove its last consumer. Rejected: keep `wsprompt` for non-delegate
  internal callers (the user chose full retirement so rsrc is the single source).
- **Tier routing = render-returned recommended tier + register pass-through
  (confirmed 2026-06-12).** `playbook.render`/`print` is the single delegation
  entry point — called exactly once per delegation (no double-render) — and returns
  a structured `recommended-tier` (first-class) read from the playbook frontmatter
  alongside the rendered body. The lead reuses that one value for BOTH paths: a
  native subagent takes it as a host model-selection guide; a mercenary passes it
  to `agents.register` (re-introduced `tier` arg) which maps first-class→alias and
  sets `RegisterOptions.Tier`. The register `tier` arg is a *pass-through of the
  render-returned value* (origin = frontmatter), NOT a caller-chosen workload tier —
  so 2c's intent (declarative tier, caller does not hand-pick) is preserved while
  the previously-missing routing bridge is finally built. Why this over the
  alternatives: native delegation also needs the tier, so a render-returned value
  serves both paths through one entry point. Rejected: register internally
  re-rendering the playbook ("absorb" form — splits the native vs mercenary render
  path and risks a double mint/double render); scanning the spliced key/tier out of
  `system_prompt_text` (brittle prompt coupling). spec `260508`/`260610` reconcile
  to "register tier = render pass-through channel".
- **Tier is single-sourced in frontmatter; the body never hardcodes it (confirmed
  2026-06-12).** A delegate playbook declares its base tier ONLY in frontmatter
  `tier:` (one maintenance point). The body / spawn-guidance text must not restate a
  literal tier — the Phase 1 "Suggested capability tier: medium" body line is
  exactly this duplication and is removed. When guidance text must reference a tier,
  it pulls it from frontmatter via a variable (e.g. `<suggested-tier>`, or an
  "elevated to `{{.DeepTier}}`"-style phrase), never a hardcoded literal. Corollary:
  if a tier guide is duplicated between a lead playbook and a delegate frontmatter,
  the lead-playbook copy is the one that goes (tooling/frontmatter is the source) —
  see Phase 3.
- **multi-prompt combine respects the first (primary) playbook's tier (confirmed
  2026-06-12).** Delegate prompt families that combine multiple prompts (e.g.
  reviewer = code-reviewer + correctness/fit/test) combine at render time as
  separate rsrc playbooks — NOT merged into one playbook, which would duplicate the
  shared Process/Output sections heavily. When combined tiers differ, the first
  (primary-role) playbook's tier wins. Scoped to Phase 4; Phase 2 handles a single
  playbook's tier only.
- **`ws.mercenary.*` surface migration (confirmed 2026-06-12; future phase).**
  The delegation spawn/lifecycle tools migrate from the generic `agents.*`
  namespace to a dedicated `ws.mercenary.*` surface. Rationale: tool naming
  materially affects LLM behavioral clarity — `ws.mercenary.spawn` makes the
  delegation intent legible where `agents.register` is generic. Parked as Phase 7
  (after the Phase 5 skill migration) to avoid churning the contract twice:
  Phase 2 builds the tier-aware `agents.register` (render-returned tier
  pass-through) on the current name; Phase 7 renames the converged surface +
  migrates skills/spec/wsflow.

## Constraints

- ~~Blocked on research 260611~~ **Unblocked (2026-06-11):** first-class axis
  (capability level) and the `light↦small`/`core↦medium`/`deep↦large` mapping are
  resolved in research 260611. Ready for promotion (pending spec-address +
  slice-boundary refinement at promotion).
- Shipped-rsrc edits require manifest regen (see
  `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`).
- wsflow mirror: any shipped `lead-implement` / delegate playbook text change
  must mirror into `agents-plugin-wsflow` with no ws-only references.
- Convergence phase order is a hard dependency chain: Phase 4 (port delegate
  bodies to rsrc) → Phase 5 (migrate skill call sites off `register(prompts)`) →
  Phase 6 (retire the loader). An embedded delegate prompt is deleted only in
  Phase 6, after Phase 5 removed its last skill consumer; deleting earlier breaks
  live delegation. Phases 1-3 (tier surface) are independent of 4-6 and can land
  first.
- Phase 6 must not strand `api.ask` or wsflow `prompt.render` callers: their
  prompt source moves to rsrc in the same phase that removes `wsprompt`, and the
  launcher fast-path/fallback bundle-hash validation must stay self-consistent
  after the embedded-bundle metadata is collapsed.
- Phase 5 ↔ Phase 7 coordination: Phase 5 migrates skills onto render+spawn under
  the current `agents.*` names; Phase 7 renames that converged surface to
  `ws.mercenary.*`. Phase 7 runs after Phase 5 so the rename touches one
  already-converged call shape, not the legacy `register(prompts)` sites.

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

**Asset content = canonical port (per the convergence decision):** the
implementer/reviewer playbook bodies port the canonical
`wsprompt/prompts/implementer.md` and `code-reviewer.md` content (adapted to
self-contained `playbook.render` form + the child-key/model-var frontmatter), not
minimal invented stubs. This makes the Phase 1 assets the canonical delegate
source the later phases migrate skills onto. (The exploratory invented stubs from
the paused WIP are discarded.) The `Tier` parse field + shipped-manifest guard
test from the paused WIP are retained.

### Result (3019ade9) - 2026-06-12

Landed on `implement/ws-tier-taxonomy-phase1` (base `f7d14671`). The render-minted
child-key splice and per-harness tier model vars are now reachable on the shipped
surface (closes 260609 Edition `379ff5e5` gaps 1+2).

Delivered:
- Shipped delegate playbooks `agents-plugin/rsrc/implementer/implementer.md`
  (`role: implementer`, `tier: medium`, uses `{{.CoreModel}}`) and
  `reviewer/reviewer.md` (`role: reviewer`, `tier: large`, uses `{{.DeepModel}}`).
  Bodies are canonical ports of `wsprompt/prompts/{implementer,code-reviewer}.md`;
  the implementer port drops the stale `ws/subquery` reference (removed in M3 2b)
  and the legacy skeleton-amendment constraint. Manifest regenerated (+2 entries).
- `PlaybookMeta.Tier` parse-only field (`wsrsrc.go`) + `case "tier":`
  (`loader.go`, mirrors `role:`) + `TestLoaderParsesRoleAndTier` (parses + no Extra
  leak). Honoring the tier for mercenary model routing is Phase 2.
- `TestShippedManifestUpToDate` guard + env-gated `TestRegenerateShippedManifest`
  (`manifest_shipped_test.go`) — catches edited-shipped-asset-without-regen
  (`260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`).
- Shipped-asset e2e (`mercenary_surface_test.go`): renders the REAL shipped
  implementer/reviewer with a lead mint root → credential block fires + child key
  is delegate-scoped (gap 1); per-harness render → tier model var resolves to the
  built-in default (claude sonnet/opus, codex gpt-5.5) and diverges per harness
  (gap 2). Non-tautological (hardcoded model literals).

Verification: `go build/vet` clean; `go test ./internal/mcp ./internal/wsrsrc
-count=1` green (mcp 16s incl. new e2e + manifest guard; wsrsrc parse test).

Deviations / notes:
- Review was a single native subagent, not a ws mercenary reviewer: the post-2c ws
  path (`playbook.render` + `agents.call`) needs a configured subprocess backend
  that has been crash-prone (M3 2c evidence), and the `register(prompts:[stems])`
  path is the removed surface Phase 5 migrates; for a low-risk content+test diff a
  native review was proportionate. Recorded as a deviation from "named-agent
  delegation first." Reviewer returned `[clean]` first pass (no Critical/Important).
- rsrc-only change: prompt bundle hash decoupled (prompt-bundle.md line 47) →
  `runtime.json` unchanged. wsflow has no rsrc tree (agentless) → no Phase 1 mirror.

> Forward (Phase 2): honor the parsed first-class `tier:` by threading it
> (first-class→alias) into the render-minted child's `RegisterOptions.Tier` so a
> mercenary spawn resolves the user's custom `config.agents_tier` entry instead of
> the hardcoded `core`; remove `Manager.oneShot()` dead code.

### Phase 2: MCP-reachable per-spawn/per-role tier routing (mercenary plumbing)

Thread a first-class tier from frontmatter (default) into the render-minted
child's `RegisterOptions.Tier` so a mercenary spawn resolves against the user's
custom `config.agents_tier` entry instead of being pinned to core
(`agent.go` hardcodes `opts.Tier = "core"` when no tier flows in).
**Mechanism (confirmed 2026-06-12): render returns recommended-tier, register
passes it through** — `playbook.render`/`print` returns a structured first-class
`recommended-tier` (read from frontmatter) alongside the body, called once per
delegation. `agents.register` (renamed to `ws.mercenary.*` in Phase 7) regains a
`tier` arg that is a *pass-through* of that render-returned value; the handler
maps first-class→alias and sets `RegisterOptions.Tier`. The one returned value
serves both paths — native (host model-selection guide) and mercenary (register
tier). `system_prompt_text` stays for non-playbook internal callers (api_docs).
The playbook body must not hardcode a tier literal — frontmatter `tier:` is the
single source — so the Phase 1 "Suggested capability tier: medium" body line is
removed here (guidance text, if any, pulls the tier via a variable like
`<suggested-tier>`). **Per-spawn override path (resolved at promotion):
frontmatter-only first** — the
frontmatter `tier:` (mapped first-class→alias) is the sole tier source for this
phase; a per-render `tier` override arg on `playbook.render` is explicitly
deferred to the `(skill, role) → tier` role-config surface in research 260611,
not built here. Remove the `Manager.oneShot()` / `oneShotOptions` dead
code (test-only despite the 2c Result's "live caller" note). Verification: an
e2e test that customizes light & deep via `config.agents_tier`, routes a
mercenary to each, and asserts the subprocess resolves to the custom
backend/model (not core). Closes 260609 Edition `0c7c0f50` gap 3 + the oneShot
flag.

### Result (54e53d70) - 2026-06-12

Landed on `implement/ws-tier-taxonomy-phase2` (stacked on the unmerged Phase 1
tip `6876249f`). A mercenary now resolves its model from the playbook frontmatter
`tier:` via `config.agents_tier` instead of being pinned to `core` (closes 260609
Edition `0c7c0f50` gap 3 + the oneShot flag).

Mechanism is the user-confirmed render-returned-tier + register pass-through (NOT
the prematurely-recorded "render-param forwarding"):
- `renderPlaybookBody` now returns `(body, recommendedTier, err)`; the
  `playbook.print`/`playbook.render` handlers append a `recommended-tier:
  <first-class>` line via `withRecommendedTier` (omitted when no tier). One render
  call routes both paths — native model guide + mercenary register tier.
- `agents.register` regains a `tier` arg (schema + handler) that is a pass-through
  of that value, mapped first-class→alias by `firstClassTierToAlias`
  (`small↦light`, `medium↦core`, `large↦deep`, `xlarge↦deep`; aliases pass through;
  empty/unknown → `""` so Register keeps its default). `prompts`/`prompt_refs`/
  `model` stay removed.
- Removed `Manager.oneShot()`/`oneShotOptions` + its test (test-only dead code);
  `syncCall`/`syncCallOptions` retained (other tests use them).
- rsrc: dropped the hardcoded tier literal from the shipped implementer/reviewer
  bodies (frontmatter `tier:` is the single source); kept the `{{.CoreModel}}`/
  `{{.DeepModel}}` model var so Phase 1 gap-2 coverage holds. Manifest regenerated.
- Tests: `TestFirstClassTierToAlias`, `TestWithRecommendedTier`,
  `TestRenderReturnsFrontmatterRecommendedTier` (shipped playbooks surface
  medium/large), `TestMercenaryTierRoutingResolvesCustomModel` (custom light&deep
  via `config.agents_tier` resolve to the custom backend/model, not core — the
  ticket e2e); updated `TestRegisterSchemaDropsLegacyFields` (tier now kept).

Verification: `go build/vet ./...` clean; `go test ./... -count=1` green (all 13
packages; mcp 16.8s incl. new tier tests + manifest guard). Spec
`#260610-mercenary-delegation-surface` reconciled (`4564d682`); mental models
`prompt-bundle`/`named-agent-runtime` updated (`a7958683`).

Deviations / notes:
- Review was a single native subagent (returned `[clean]` first pass, 0
  Critical/Important), not a ws mercenary reviewer — the post-2c ws delegation
  path still needs the crash-prone subprocess backend and `register(prompts)` is
  the removed surface Phase 5 migrates; proportionate for a contained diff.
  Recorded deviation from "named-agent delegation first" (continues Phase 1).
- Process correction: the mechanism was first recorded prematurely (`379e7bda`)
  before user confirmation, then corrected (`37445d3a`); the consent-gate dogfood
  is captured in `260611-chore-lead-discussion-gap-discipline` Phase 3.
- rsrc-only prompt change → prompt bundle hash decoupled, `runtime.json`
  unchanged; wsflow is agentless (no rsrc tree, `agents.*` hidden) → no mirror.

> Forward (Phase 3): teach `wsconfig` the first-class vocabulary (retire the
> MCP-layer `firstClassTierToAlias` bridge), re-author the `review-allocation`
> reviewer-tier default in first-class vocab, and unify the lead-playbook tier
> guide onto the frontmatter/`recommended-tier` single source.

### Phase 3: first-class tier vocabulary adoption + migration

Re-author the `judge: review-allocation` reviewer-tier default (dropped
`e6aadfc9`) in first-class vocab — **correctness→large, fit/test→medium** (under
the locked `deep↦large`, `core↦medium` mapping) — in the
`lead-implement` playbook + wsflow mirror + manifest regen; migrate spec and
mental-model text that frames `light/core/deep` as "the tier abstraction" to the
alias-layer framing. Adds the `(skill, role) → first-class tier` config override
surface only if research 260611 promotes that surface into this ticket.

Also resolve the tier-guide single-source (per the frontmatter-single-source
decision): where a delegate's tier is stated in BOTH a lead playbook (e.g. the
`review-allocation` table) and the delegate's frontmatter, the lead-playbook copy
is reduced to a pointer/variable so the delegate frontmatter `tier:` stays the one
maintenance point. The `recommended-tier` that `playbook.render` returns (Phase 2)
is the runtime channel for that single source.

### Result (fc1cdc5f) - 2026-06-12

Landed on `implement/ws-tier-taxonomy-phase3` (stacked on the unmerged Phase 2 tip
`a184b6df`). The reviewer-tier default is re-authored in first-class vocabulary and
the first-class capability vocabulary is now established in spec + mental-model.

Delivered:
- Reviewer-tier default re-authored in the `lead-implement` `judge:
  review-allocation` Tier 2 table — **correctness `large`, fit/test `medium`** (the
  dropped `e6aadfc9` intent, now in first-class vocab under `deep↦large`/`core↦medium`)
  — in both the rsrc playbook and the wsflow mirror, manifest regenerated (only the
  lead-implement hash changed). Source commit `fc1cdc5f`.
- Tier-guide single-source (frontmatter-single-source decision) resolved at the
  note/precedence level: the ws note declares that when a delegate playbook sets
  its own `tier:`, the `recommended-tier` from `ws/playbook.render` is authoritative
  for that delegate and the table is the allocation default. No literal tier is
  duplicated between this playbook and an existing delegate frontmatter today
  (partition playbooks arrive in Phase 4). wsflow note stays host-neutral (no `ws/`
  reference; keeps the alias→model mapping hint).
- Spec (`10be8183`): `#260612-first-class-tier-vocabulary` (mcp-tools.md) establishes
  `small/medium/large/xlarge` as the capability-axis abstraction with `light/core/deep`
  as the concrete-model alias layer (locked `light↦small`/`core↦medium`/`deep↦large`;
  `xlarge` no legacy alias); `#260612-reviewer-allocation-tier-default`
  (workflow-skills.md) documents the per-partition default + recommended-tier
  precedence. The pre-existing `260508` alias entries already framed `light/core/deep`
  as *model aliases* (not "the tier abstraction"), so they needed no rewrite.
- Mental model (`94eafada`): disambiguated the legacy `tier` model-selection input
  (compatibility-only) from the current first-class `tier:` frontmatter in
  `workflow-skills.md`; `prompt-bundle`/`named-agent-runtime` already carried the
  first-class vocab from Phase 2; `mcp-runtime` correctly describes `wsconfig` as
  alias-keyed and was left unchanged.

Verification: `go build/vet ./...` clean; `go test ./... -count=1` green (all 13
packages incl. the manifest guard; the previously-flaky exec timing test passed);
`ws/spec_index.verify` ok; review `[clean]` first pass (single fit-focused native
subagent, 0 findings on all six binding checks + skill-authoring lens).

Deviations / open items:
- Review was a single native subagent and the mental-model reconciliation was
  lead-driven, both because the ws delegate path (`agents.register(prompts:[stems])`
  → mental-model-updater / reviewer) is the schema surface removed in M3 2c that
  Phase 5 migrates; proportionate for a contained doc/skill-text diff. Continues the
  recorded Phase 1/2 deviation from "named-agent delegation first."
- **Scope flag — `wsconfig` first-class adoption NOT done.** The Phase 2 `> Forward
  (Phase 3)` note and the `_index` focus "Next" line both anticipated "teach
  `wsconfig` the first-class vocabulary (retire the MCP-layer `firstClassTierToAlias`
  bridge)" in Phase 3, but the Phase 3 plan text above does not list a `wsconfig`
  change among its deliverables. Honoring the plan-text scope as the hard boundary,
  the MCP-layer `firstClassTierToAlias` bridge stays in place and `wsconfig` remains
  alias-keyed (the spec/mental-model framing reflects this current state). Routing
  the `wsconfig` adoption (Phase 3 Edition vs new phase vs defer) is left to the user.
- The conditional `(skill, role) → first-class tier` config override surface was
  skipped: research `260611` has not promoted that surface into this ticket.

> Forward (Phase 4): port the remaining delegate prompt bodies (`code-reviewer` +
> partition prompts, `reference-discovery`, `mental-model-updater`,
> `plan-populator-survey/research`) from `wsprompt/prompts/` to canonical rsrc
> playbooks with `role:` + first-class `tier:` frontmatter; embedded copies stay
> until Phase 5 removes their last skill consumer.

### Phase 4: port remaining delegate prompts to canonical rsrc playbooks

Port the remaining delegate prompt bodies from `wsprompt/prompts/` to rsrc
playbooks (self-contained `playbook.render` form, `role:` + first-class `tier:`
frontmatter, model vars where the role carries model guidance): `code-reviewer`
+ the `code-review-correctness/fit/test` partition prompts, `reference-discovery`,
`mental-model-updater`, `plan-populator-survey`, `plan-populator-research`.
Reviewer is already added in Phase 1; this phase completes the delegate set so
every skill-spawned delegate has a canonical rsrc source. Regenerate the manifest;
mirror into `agents-plugin-wsflow`. The embedded copies are NOT deleted yet (still
referenced by skill text until Phase 5). Verification: each ported playbook
renders via `playbook.render` with the expected child-key/vars; manifest guard
green. Boundary: does not change skill delegation call sites (Phase 5) or remove
`wsprompt` (Phase 6).

### Phase 5: migrate skill delegation off register(prompts:[stems])

Rewrite the shipped skill playbooks (`lead-implement` and any peer that registers
a prompt-stem delegate: reference-discovery, implementer, reviewer family,
mental-model-updater, plan-populator) to delegate via `playbook.render` (mint
child key + self-contained prompt) + `agents.call`/native spawn, removing every
`register(prompts:[stems])` / `prompts:`-field call against the post-2c schema.
Mirror each change into `agents-plugin-wsflow` with no ws-only references; manifest
regen. Verification: no shipped skill text references the removed `prompts`
register field; a representative delegation (e.g. lead-implement → implementer)
renders + spawns end-to-end on both ws and wsflow. Boundary: leaves the now-unused
embedded delegate prompts in place for Phase 6 to delete.

### Phase 6: retire the wsprompt loader entirely

Move `wsprompt`'s remaining non-delegate consumers onto rsrc and remove the
go:embed loader: rewire `api.ask` hard-coded prompt stems to rsrc playbooks;
rewire the wsflow `prompt.render` MCP tool off `wsprompt.RenderSource` onto rsrc
loading (reconcile the `#260529-prompt-render-tool` contract + the five-stem
render-eligibility allowlist); delete the now-orphaned embedded delegate prompt
bodies and the `wsprompt` package; collapse the prompt-bundle-hash / `runtime.json`
bundle-metadata machinery that only served embedded prompts (verify launcher
fast-path/fallback validation still agrees). Rewrite mental-model `prompt-bundle.md`
line 27 (and related entry-point/coupling text) to the single-rsrc-source-of-truth
model. Verification: `wsprompt` package gone; `go build/vet/test ./...` green;
wsflow `prompt.render` still serves its allowlisted stems from rsrc; launcher
validation green; `api.ask` resolves its prompts from rsrc. Boundary: this is the
last convergence phase; it depends on Phases 4+5 having moved every delegate
consumer first.

### Phase 7: migrate the delegation surface to `ws.mercenary.*`

Rename the delegation spawn/lifecycle tools from the generic `agents.*` namespace
to a dedicated `ws.mercenary.*` surface (e.g. `ws.mercenary.spawn`/`register` for
the tier-aware register built in Phase 2, plus `call`/`status`/`result`/
`cancel`/diagnostics as the retained mercenary lifecycle dictates), updating the
MCP dispatch, the full `runtime.json`, skill/playbook text, and the wsflow mirror.
Rationale: tool naming materially drives LLM behavioral clarity (decision
2026-06-12). Keep `agents.*` only where a non-delegation internal consumer still
needs it (api_docs), or alias for one release if a compatibility window is wanted.
Depends on Phase 5 (skills already delegate via render+spawn) so the rename
touches a single, already-converged call shape. Verification: no shipped
skill/playbook references the old delegation tool names; a representative
delegation spawns end-to-end on both ws and wsflow under the new names; spec
`#260610-mercenary-delegation-surface` reconciled to the `ws.mercenary.*` names.

## Spec Impact

**Target spec areas + caller-visible change (per phase):**

- **Phase 1** (shipped delegate asset) — no new contract. Exercises behavior
  already specified by `260610-mercenary-delegation-surface` (render-minted child
  keys) and `260609-playbook-tools` (`playbook.render` variable substitution);
  the asset just makes that behavior reachable on the shipped surface.
- **Phase 2** (per-spawn tier routing) — extends
  `260610-mercenary-delegation-surface` + the `agents.register` / `playbook.render`
  entries in `ai-docs/spec/mcp-tools.md`: `playbook.render`/`print` now returns a
  structured first-class `recommended-tier` (read from frontmatter), and
  `agents.register` regains a `tier` arg that is a *pass-through* of that value
  (origin = frontmatter, not caller-chosen), mapping
  first-class→alias→`config.agents_tier`. Caller-visible change: a mercenary's
  model resolves from its playbook frontmatter `tier:` instead of being pinned to
  `core`; this reconciles the 2c `260508` tier-removal to "register tier = render
  pass-through channel" (the value origin stays declarative).
- **Phase 3** (first-class vocabulary) — touches the alias-config spec
  (`260508-model-alias-config-tools` / `260513-harness-local-agent-tier-config`
  in `ai-docs/spec/mcp-tools.md`) and the reviewer-allocation default in
  `ai-docs/spec/workflow-skills.md`. Caller-visible change: a new first-class
  tier vocabulary `small/medium/large/xlarge` (capability axis) sits above the
  existing `light/core/deep` alias layer; frontmatter declares `role:` + `tier:`
  in first-class vocab; locked mapping `light↦small`/`core↦medium`/`deep↦large`.
- **Phase 4** (port delegate prompts to rsrc) — no caller-visible contract; adds
  rsrc playbook assets + manifest entries. Internal source convergence only.
- **Phase 5** (skill delegation migration) — touches `ai-docs/spec/workflow-skills.md`
  delegation/registration text: skills delegate via `playbook.render` + spawn,
  no longer via `register(prompts:[stems])` (the register field was already
  removed by `260508-agents-register-model-alias-field` spec-remove in M3; this
  reconciles the skill-side delegation contract to match).
- **Phase 6** (wsprompt retirement) — touches `#260529-prompt-render-tool` in
  `ai-docs/spec/mcp-tools.md`: the wsflow `prompt.render` tool's source changes
  from the embedded `wsprompt.RenderSource` bundle to rsrc loading (the
  render-eligibility allowlist + namespace-substitution contract are preserved;
  the backing loader is reconciled). `api.ask`'s prompt source moves to rsrc.
  Removes the embedded-prompt-bundle hash/`runtime.json` metadata surface from the
  launcher-validation contract.
- **Phase 7** (`ws.mercenary.*` rename) — touches
  `#260610-mercenary-delegation-surface` in `ai-docs/spec/mcp-tools.md`: the
  delegation spawn/lifecycle tool names change from `agents.*` to
  `ws.mercenary.*` (caller-visible tool-name change). Reconciled at closeout
  unless promotion elects contract-first for the rename.

**Contract-first spec: no.** The full tier-vocabulary contract (first-class set,
capability axis, alias mapping, frontmatter `tier:`/`role:`) is already captured
in this ticket's `## Decisions` and research `260611`; the surfaces being
extended are already spec'd (`260610`, `260508`, `260513`). The convergence
phases (4-6) preserve observable behavior — they migrate the *source* of delegate
prompts and the wsflow `prompt.render` backing loader without changing the
rendered output or the render-eligibility/namespace contract — so they reconcile
existing stems (`#260529-prompt-render-tool`, `workflow-skills.md` delegation
text) at closeout rather than fixing a new contract up front. The spec entries are
best authored at closeout against the concrete implemented anchors (exact field
names, the `config.agents_tier` first-class indexing, the post-retirement
`prompt.render` source). The doc-pre-pass `lead-update-spec` run reconciles
`mcp-tools.md` + `workflow-skills.md` within each phase's commit range.
