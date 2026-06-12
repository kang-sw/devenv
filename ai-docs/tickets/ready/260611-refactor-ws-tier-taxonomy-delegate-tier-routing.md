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
- **wsflow rsrc provisioning + cross-package source policy (confirmed 2026-06-12).**
  The single source of truth (`agents-plugin/rsrc/`) is enforced *across packages*:
  the same prompt stem's body must NEVER diverge between ws and wsflow. Body
  divergence is assumed not to exist; if a wsflow-only variant is ever genuinely
  needed it is added as a SEPARATE rsrc file that only wsflow renders and ws does not
  reference — never a divergent body of a shared stem. Because wsflow is a separately
  distributed package that (unlike ws) ships no rsrc tree today, Phase 6's "rewire
  wsflow `prompt.render` onto rsrc" requires wsflow to carry an rsrc tree at runtime.
  That tree is a **generated/copied artifact** produced from canonical
  `agents-plugin/rsrc/` at build/package time, following the existing per-package
  real-file-copy precedent (`ws-mcp-launcher.py` is byte-identical and committed as a
  real file in both packages; git content-dedupes, so the on-disk per-package copy is
  the intended end state and storage cost is ~0). **Symlink is rejected:** Claude
  `install.sh` removes wsflow from the plugin cache (no co-located sibling to point
  at), independent distribution cannot assume the two packages are co-located, git
  symlinks are fragile on the Windows desktop target, and the repo has zero
  committed-symlink precedent. The pain being solved is **drift, not storage** — so a
  drift-guard test asserting the wsflow rsrc copy is render-equivalent to canonical
  (post `ws/`→`wsflow/` namespace substitution) is mandatory; its absence is exactly
  why the `wsprompt` embedded copies silently drifted from rsrc. **Doctrine
  carve-out:** the `wsflow-mirroring.md` "drift visibility over generated sameness"
  rule stays for *skills*, but the rsrc subtree is explicitly the one place where
  **generated sameness IS the contract**. **Resolved at implementation (2026-06-12,
  Phase 6 source survey):** (a) the generation step is **test-driven regen** — a Go
  test asserts byte-equality between `agents-plugin-wsflow/rsrc/` and canonical and
  regenerates the committed copy under `WS_REGEN_WSFLOW_RSRC=1` (mirrors the existing
  `WS_REGEN_MANIFEST` pattern); chosen over a release-build copy (no committed tree →
  no dev/test rsrc, drift caught only at release) and a launcher shared-tree pointer
  (rejected: co-location not guaranteed, Claude removes wsflow). (b) **on-disk
  committed copy** — the launcher's existing `apply_rsrc_root_env` already sets
  `WS_RSRC_ROOT` when a sibling `rsrc/` exists, so a committed `agents-plugin-wsflow/
  rsrc/` just works with no launcher change. (c) The stored wsflow rsrc files are
  **byte-identical** to canonical because `ws/`→`wsflow/` namespace substitution
  happens at render time in the `prompt.render` tool layer, not in stored files — so
  the drift guard is a tree/manifest **byte-equality** check (simpler than the
  earlier "render-equivalent post-substitution" framing). (d) **retain-or-retire
  verdict:** api-doc / infra-doc embeds migrate to rsrc; legacy `skeleton-populator`/
  `skeleton-reviewer`/`sprint-survey` are confirmed-dead-then-deleted (handled in
  Phase 6b).
- **Phase 6 re-slice (confirmed 2026-06-12).** A Phase 6 source survey found
  `wsprompt` has more consumers than the original plan text enumerated: besides the
  delegate prompts / `api.ask` stems / wsflow `prompt.render`, `wsprompt.ReadInfra`
  backs the `infra.read` tool (8 infra docs, most embed-only) and `wsprompt.Bundle`/
  `ContentSHA256` back runtime metadata + the launcher hash-validation chain. "Retire
  entirely" therefore requires migrating those too. To keep slices reviewable, full
  retirement is split: **Phase 6** lands the convergence headline (wsflow rsrc
  provisioning + `prompt.render`→rsrc + `api.ask`→rsrc + doctrine carve-out, leaving
  `wsprompt` in place for `infra.read`/runtime metadata); **Phase 6b** finishes
  retirement (`infra.read`→rsrc, dead-stem disposition, runtime/bundle-hash collapse,
  package deletion, `prompt-bundle.md` rewrite). This is a clean intermediate, not a
  stranded caller: after Phase 6 every render/api consumer is on rsrc and `wsprompt`
  only backs the still-embedded `infra.read`/`Bundle` paths.
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
- **Tier vocabulary naming rule + config-surface renames (confirmed 2026-06-12).**
  "tier" is reserved for the first-class abstraction layer; the concrete-model layer
  uses "alias"/"model". Consequently: (a) `config.agents_tier` is renamed
  `config.model_alias` — it is keyed by the `light/core/deep` alias and is read by
  BOTH the mercenary register path (`agent.go`) and the native-facing `{{.*Model}}`
  render-var substitution (`playbook_tools.go`), so it is a concrete-model-layer
  surface, NOT mercenary-only (the rejected `mercenary_alias` name would
  misattribute a mechanism-layer surface to one consumer). (b) The deferred
  `(skill, role)→first-class-tier` override surface (research `260611`) is named
  `config.role_tier` (role is the primary key; skill is a secondary key). (c)
  `firstClassTierToAlias` is the intended plane-translation boundary, kept by design;
  `config.agents_tier`/`config.model_alias` stays alias-keyed — this supersedes the
  Phase 2 Forward note's unconfirmed "teach wsconfig first-class" hint, which
  contradicted the "config.agents_tier unchanged" decision. Execution is
  caller-visible churn (MCP tool name + CLI mirror + config-file key + spec
  `260513`/`260508` + user-config compat read) deferred to the research-`260611`
  config-surface slice or coordinated with Phase 7, NOT Phase 3.

## Constraints

- ~~Blocked on research 260611~~ **Unblocked (2026-06-11):** first-class axis
  (capability level) and the `light↦small`/`core↦medium`/`deep↦large` mapping are
  resolved in research 260611. Ready for promotion (pending spec-address +
  slice-boundary refinement at promotion).
- Shipped-rsrc edits require manifest regen (see
  `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`).
- wsflow mirror: shipped skill (`lead-*`) text changes mirror into
  `agents-plugin-wsflow` with no ws-only references. For the **rsrc delegate
  playbooks** specifically, Phase 4 found wsflow ships no rsrc tree today, so there
  is no manual mirror target; Phase 6 resolves this by provisioning wsflow's rsrc as
  a generated copy of canonical (see the wsflow-rsrc-provisioning decision) — mirror
  becomes regenerate, not hand-copy.
- Convergence phase order is a hard dependency chain: Phase 4 (port delegate
  bodies to rsrc) → Phase 5 (migrate skill call sites off `register(prompts)`) →
  Phase 6 (wsflow provisioning + render/api convergence) → Phase 6b (finish
  retirement + delete the loader). An embedded delegate prompt is deleted only in
  Phase 6b, after Phase 5 removed its last skill consumer and Phase 6 moved the
  render/api consumers; deleting earlier breaks live delegation. Phases 1-3 (tier
  surface) are independent of 4-6b and can land first.
- Phase 6 must not strand `api.ask` or wsflow `prompt.render` callers: their prompt
  source moves to rsrc in Phase 6 while `wsprompt` stays in place for the still-
  embedded `infra.read`/`Bundle` paths (a clean intermediate). Phase 6b collapses the
  launcher fast-path/fallback bundle-hash validation; that collapse must stay
  self-consistent after the embedded-bundle metadata is removed.
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

#### Edition (45f32b80) - 2026-06-12

Resolved the Phase 3 Result's `wsconfig` adoption scope flag: it is **not** an open
routing decision. An Explore audit of the tier-taxonomy history (research `260611`)
found the "teach `wsconfig` the first-class vocabulary / retire the
`firstClassTierToAlias` bridge" intent originated as assistant-authored
implementation narrative (Phase 2 source comment `54e53d70`, propagated into the
Phase 2 Forward note + `_index` "Next"), and that it directly contradicts the
user-confirmed `## Decisions` bullet "the existing `config.agents_tier` surface is
unchanged by the vocabulary split". Per the finalized taxonomy, `config.agents_tier`
is the concrete-model layer (alias-keyed) and `firstClassTierToAlias` is the intended
plane-translation boundary, correct-by-design — not debt. The source comment was
reframed (`45f32b80`); no `wsconfig` change is owed. Recorded as a second instance of
the consent-gate pattern in `260611-chore-lead-discussion-gap-discipline` Phase 3.

Naming decisions confirmed 2026-06-12 (see `## Decisions`; execution deferred to the
research-`260611` config-surface slice / Phase 7 coordination, NOT this phase): "tier"
is reserved for the abstraction layer, `config.agents_tier` is renamed
`config.model_alias`, and the deferred `(skill, role)→tier` override surface is named
`config.role_tier`.

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

### Result (5a26b1d6) - 2026-06-12

Ported the remaining delegate prompts to canonical rsrc playbooks. Source commit
`5a26b1d6` (rsrc assets + manifest regen + shipped-render tests); mental-model
`9f7e311b`. Manifest grew 26→34 files.

Created assets (`agents-plugin/rsrc/`):

| Playbook | role | tier | model var | notes |
|----------|------|------|-----------|-------|
| `code-reviewer.md` (flat dep) | — | — | — | shared reviewer base, no frontmatter, var-free |
| `code-review-correctness/` | reviewer | large | DeepModel | `includes: [code-reviewer]` |
| `code-review-fit/` | reviewer | medium | CoreModel | `includes: [code-reviewer]` |
| `code-review-test/` | reviewer | medium | CoreModel | `includes: [code-reviewer]` |
| `reference-discovery/` | delegate | small | LightModel | `delegates: false` |
| `mental-model-updater/` | delegate | medium | CoreModel | `delegates: false` |
| `plan-populator-survey/` | delegate | medium | CoreModel | `delegates: false` |
| `plan-populator-research/` | delegate | large | DeepModel | `delegates: false` |

Tiers follow the locked alias mapping (`light↦small`/`core↦medium`/`deep↦large`)
applied to each source `model:`; the three review partitions take the Phase 3
reviewer-allocation default (correctness `large`, fit/test `medium`).

**Composition decision.** The three partition reviewers reuse a single flat
shared base `code-reviewer.md` via `includes:` rather than inlining the base
three times. A subdir playbook cannot be an include target (includes resolve
flat to `<root>/<name>.md`), so the base lives as a root-level flat dep and is
the canonical "code-reviewer" rsrc port. The base is var-free so partitions with
different tiers do not collide on an undeclared model var. Each partition still
renders to one self-contained prompt (the 2c "single self-contained prompt"
shape), since includes are appended at render time. `role:`/`delegates:` are
orthogonal: `role` gates render-minted child-key eligibility, `delegates` gates
the mercenary tip — the four auxiliary workers are `role: delegate` (child-key
eligible) but `delegates: false` (read-only / no-spawn).

**Verification.** Authoritative verification is the Go render layer
(`internal/mcp/mercenary_surface_test.go`): `TestRenderGoldenShippedPhase4Delegates`
asserts child-key splice (scope `roleDelegate`) + full var substitution for all
seven renderable playbooks; `TestRenderGoldenShippedReviewPartitionIncludesBase`
asserts the partitions resolve the shared base. Manifest guard + full module
suite green. Fit review clean (content fidelity vs source, frontmatter, tier/var
consistency, manifest hashes all confirmed).

**Deviations.**
- *No wsflow mirror.* The plan-text "mirror into `agents-plugin-wsflow`" has no
  target: wsflow is agentless with no rsrc tree (Phase 1 finding, `3019ade9`).
  Delegate-asset convergence for wsflow stays tracked by
  `260610-chore-wsflow-explore-playbook-mirroring` (idea).
- *Lead-driven direct edit + native review.* The ws delegation surface
  (`agents.register(prompts:[stems])`) is mid-migration and depends on these very
  assets, so this continues the documented M3 Phase 1/2 lead-driven deviation
  rather than ws named-agent delegation; review ran via a native subagent.
- *Live MCP server could not verify renders.* The running `0.30.0-dev` server
  re-reads `manifest.json` per call but caches its rsrc file SET in-process, so
  newly added files surface as `manifest-listed file missing` until restart even
  with `WS_RSRC_ROOT` on the live tree. Captured as a dogfood idea ticket
  (`260612-bug-ws-rsrc-dev-server-new-file-staleness`).

> Forward (Phase 5/6): the base reviewer text is intentionally duplicated between
> `code-reviewer.md` (flat dep) and `reviewer/reviewer.md` (Phase 1 subdir
> playbook) because a subdir playbook cannot be an include target. Phase 5/6
> should reconcile this when it migrates call sites — decide the single-reviewer
> path target (`reviewer` vs a `code-reviewer` playbook) and whether to dedupe.

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

### Result (5023562c) - 2026-06-12

Migrated shipped skill delegation off `register(prompts:[stems])` to
`ws/playbook.render` + native/mercenary spawn. Source `5023562c` (rsrc + manifest);
spec `d5f158c7`; mental-model `3032e469`. Landed on `implement/ws-tier-taxonomy-phase5`
(base = phase4 tip + the Phase 6 decision commit `024a4874`).

Migrated playbooks (the only ws rsrc files that registered prompt-stem delegates):
- `lead-implement` — added a centralized **Delegate dispatch** template (render →
  native default / mercenary `register(system_prompt_text)`+`call`; no caller
  `context` since these delegates declare only auto-injected model-alias vars; task
  input handed to the worker, not the render call). Converted reference-discovery
  (step 10), plan-populator (step 12), implementer (step 13 + Edit 4/5),
  single/partitioned reviewer (Review 2/3 + partition table column), and
  mental-model-updater (Doc Pre-Pass).
- `lead-sprint` — mental-model-updater dispatch.
- `lead-workflow-manual` — the Persistent-agents primitive guidance + usage pattern.

**Single-reviewer path target resolved** (Phase 4 Forward note): single review → the
`reviewer` playbook (general reviewer; its shared base covers
correctness/standards/contract/security); partitioned → `code-review-correctness`/
`fit`/`test` (each includes the `code-reviewer` base). The base-text SOURCE dedup
(`code-reviewer.md` flat dep vs `reviewer/reviewer.md` subdir) stays deferred to
Phase 6.

**Verification.** No residual `register(prompts)`/`prompts:`/`prompt_refs` in any
delegation site (only prose noting the fields are gone). Manifest regenerated (3 file
hashes). `go build/vet ./...` clean; full module suite green (13 packages, manifest
guard included). Single native fit + skill-authoring review `[clean]` (six checks:
removal completeness, idiom correctness, reference validity, ticket-decision
preservation, skill-authoring fit, scope discipline); two low/non-blocking notes
rejected with reasons (the mercenary framing is lead-side-correct; the Review-step-4
redundancy is pre-existing / out-of-diff). spec `workflow-skills.md`
`#260507`/`#260505` reconciled; mental-models `prompt-bundle`/`workflow-skills`
reconciled.

**Deviations / open items.**
- *`lead-verify-design` excluded.* Its `ws/agents.register(name, model: "deep")`
  drives an inline-prompt design-reviewer using the removed `model` field — not a
  `register(prompts)` site, so outside Phase 5's enumerated delegate scope. Open
  follow-up: how an inline-prompt reviewer sets its model/tier post-2c (it renders no
  playbook, so there is no `recommended-tier` to pass through).
- *Lead-driven edit + native review* (continues the M3 Phase 1-4 deviation): the ws
  delegate path is the surface being migrated and is crash-prone; native review
  proportionate for a skill-text diff.
- *Live MCP server staleness, broadened.* `playbook.print(lead-implement)` on the
  running `0.30.0-dev` server returned PRE-edit (Phase 4) content for an EXISTING
  edited file — broader than `260612`'s "existing-file edits are seen live" note; the
  server reads a cached/installed rsrc root, not the working tree. Authoritative
  verification was the Go layer (working-tree rsrc) + grep; the three edited playbooks
  are `kind:print` with no vars/includes, so render = verbatim body (no substitution
  risk). Extends dogfood ticket `260612`.

> Forward (Phase 6): the single-reviewer target is now `reviewer` and partitions are
> `code-review-*`; Phase 6's base-text dedup should reconcile `code-reviewer.md` (flat
> dep) vs `reviewer/reviewer.md` (subdir) against these call sites. Also wire the
> wsflow rsrc generated-copy provisioning + drift guard recorded in `## Decisions`.

### Phase 6: wsflow rsrc provisioning + render/api.ask convergence

> Re-sliced 2026-06-12 (see the Phase 6 re-slice decision). A source survey found
> `wsprompt` has more consumers than the original plan text enumerated, so full
> retirement + package deletion moved to Phase 6b; Phase 6 lands the convergence
> headline.

Provision wsflow's distributed package with a generated rsrc tree and move the two
render-time consumers onto it:

- **wsflow rsrc provisioning.** Commit `agents-plugin-wsflow/rsrc/` as a real tree
  that is a **byte-identical** copy of canonical `agents-plugin/rsrc/` (symlink
  rejected). Stored files are byte-identical because the `ws/`→`wsflow/` namespace
  substitution happens at render time in the `prompt.render` tool layer, not in
  stored files — so the drift guard is a tree/manifest **byte-equality** check.
  Mechanism: committed copy + **test-driven regen** — a Go test asserts byte-equality
  and regenerates the copy under `WS_REGEN_WSFLOW_RSRC=1` (mirrors `WS_REGEN_MANIFEST`).
  The committed tree means wsflow resolves rsrc uniformly in dev/test/release and the
  launcher's existing `apply_rsrc_root_env` (sets `WS_RSRC_ROOT` when a sibling
  `rsrc/` exists) needs no change.
- **wsflow `prompt.render` → rsrc.** Rewire the tool off `wsprompt.RenderSource` onto
  rsrc loading; reconcile the `#260529-prompt-render-tool` contract + the five-stem
  render-eligibility allowlist; render-time namespace substitution is preserved.
- **`api.ask` hard-coded stems → rsrc.** Move `api-doc-manager`, `pre-router`,
  `api-doc-cargo-brief` to rsrc playbooks rendered into the agent `system_prompt_text`
  (removing the internal `prompts:[stems]`→`wsprompt.Resolve` path for `api.ask`).
- **Doctrine carve-out.** Carve the rsrc subtree out of `wsflow-mirroring.md`'s
  "drift visibility over generated sameness" doctrine as the one generated-sameness
  exception.

Boundary: leaves `wsprompt` in place (still backing `infra.read`/`ReadInfra` and
runtime `Bundle`/`ContentSHA256`); does NOT delete the package or collapse the
bundle-hash metadata (Phase 6b). Verification: wsflow `prompt.render` serves its
allowlisted stems from the wsflow rsrc copy; drift guard green; `api.ask` resolves
its prompts from rsrc; `go build/vet/test ./...` green. Depends on Phases 4+5 having
moved every delegate consumer first.

### Result (6be3bb64) - 2026-06-12

Landed on `implement/ws-tier-taxonomy-phase6` (stacked on the unmerged Phase 5 tip
`5c6fb71a`; merge-target = epic `260605`). The render/api consumers are off the
embedded bundle and wsflow ships a generated rsrc tree; `wsprompt` stays in place
for `infra.read`/runtime metadata (Phase 6b).

Source survey (two Explore agents) drove the re-slice (`1e4d4a1f`): `wsprompt` had
more consumers than the plan text enumerated (`ReadInfra`→`infra.read`,
`Bundle`/`ContentSHA256`→runtime metadata + launcher hash validation), so full
retirement + package deletion moved to Phase 6b.

Delivered (4 source commits):
- **Flat-playbook fallback** (`42bbcd46`, `wsrsrc/loader.go`): `Load` falls back to
  a flat `<root>/<name>.md` when the subdir playbook is absent (subdir wins), so the
  var-free `code-reviewer` flat dep is loadable as a playbook. Chosen over flipping
  the wsflow allowlist stem to `reviewer` (smaller contract blast). +2 unit tests.
- **api.ask → rsrc** (`2698341c`): added `kind:print` rsrc ports `pre-router`,
  `api-doc-manager`, `api-doc-cargo-brief`; `wsagentAPIRuntime` renders each via
  `renderAPIPrompt` (nil-vars `Load`) into `SystemPromptText`; the cargo-brief
  `ConditionalPromptRef` became an inline `exec.LookPath("cargo-brief")` gate.
  Manifest regenerated (+3). `Model`/`SuppressOrientation` preserved.
- **wsflow `prompt.render` → rsrc** (`3bed684a`): `renderPrompt` became a `*Server`
  method routing through `renderPlaybookBody` (mintRoot="", nil vars); 5-stem
  allowlist + render-time `ws/`→`wsflow/` substitution preserved; context stays a
  free-text Render Context block. `wsprompt.RenderSource` is now orphaned.
- **wsflow rsrc provisioning** (`6be3bb64`): committed `agents-plugin-wsflow/rsrc/`
  as a byte-identical copy of canonical (38 files incl. manifest);
  `TestWsflowRsrcMirrorUpToDate` byte-equality drift guard + `WS_REGEN_WSFLOW_RSRC`
  regen; `wsflow-mirroring.md` doctrine carve-out (rsrc = the one generated-sameness
  exception).

**Verification.** `go build/vet ./...` clean; full module suite green (13 packages,
manifest guard + drift guard included; one known-flaky exec-timing test passed on
re-run). Partitioned review (correctness + fit + test, independent native
reviewers) all `[clean]`. spec `9f6db8f5` (#260529 source + #260513 provisioning);
mental-model `9112a632` (prompt-bundle / api-documentation-cache / plugin-runtime).

**Deviations / open items.**
- *Lead-driven edit + native partitioned review* (continues the M3/260611 Phase 1-5
  deviation): the ws delegate path is crash-prone and the live MCP server reads a
  cached rsrc root, so authoritative verification was the Go layer + grep.
- *Pre-existing wsflow python test failure (not a Phase 6 regression).* `python3 -m
  unittest discover agents-plugin-wsflow/tests` fails on
  `lead-workflow-manual/SKILL.md: full ws dotted namespace` (`wsflow/ws.lead.login`,
  introduced in `9649a4bf`, present at the Phase 5 tip). Captured as idea ticket
  `260612-bug-wsflow-skill-ws-dotted-namespace-ref`; out of Phase 6 (rsrc) scope.
- *cargo-brief conditional not unit-tested* (test reviewer note, accepted): a 4-line
  `exec.LookPath` gate whose unit test would need a real binary; the rsrc-load layer
  is covered.

> Forward (Phase 6b): move `wsprompt.ReadInfra` (`infra.read`) onto rsrc (migrate the
> embed-only infra docs); confirm-and-delete the dead `skeleton-*`/`sprint-survey`
> stems; collapse `Bundle`/`ContentSHA256` + the `runtime.json` bundle-hash metadata
> + launcher fast-path/fallback validation; delete the embedded bodies + the
> `wsprompt` package + the CLI `register --prompts`/internal `Prompts`→`Resolve` path;
> rewrite `prompt-bundle.md` line 27 to the single-rsrc-source-of-truth model.

### Phase 6b: finish wsprompt retirement + package deletion

Move the remaining consumers off `wsprompt` and delete the go:embed loader:

- **`infra.read` → rsrc.** Rewire `wsprompt.ReadInfra` (via `wsdoc.ReadInfra`) onto
  rsrc; migrate the embed-only infra docs (`executor-wrapup`, `impl-playbook`,
  `subagent-rules`, `delegate-orientation`) to rsrc (`code-review-{correctness,fit,
  test}` already have rsrc twins).
- **Dead-stem disposition.** Confirm-and-delete the legacy embed-only stems with no
  live consumer (`skeleton-populator`, `skeleton-reviewer`, `sprint-survey`); migrate
  any that prove live.
- **Runtime metadata collapse.** Retire `wsprompt.Bundle`/`ContentSHA256`; collapse
  the prompt-bundle-hash / `runtime.json` bundle-metadata machinery that only served
  embedded prompts and the launcher fast-path/fallback hash validation (verify
  launcher validation stays self-consistent after the embedded-bundle metadata is
  removed).
- **Delete** the now-orphaned embedded prompt bodies and the `wsprompt` package.
- **Rewrite** mental-model `prompt-bundle.md` line 27 (and related entry-point/
  coupling text) to the single-rsrc-source-of-truth model.

Verification: `wsprompt` package gone; `go build/vet/test ./...` green; `infra.read`
serves its docs from rsrc; launcher validation green. Boundary: depends on Phase 6
(render/api consumers already moved); this is the last convergence phase.

### Result (6873b480) - 2026-06-12

Landed on `implement/ws-tier-taxonomy-phase6b` (stacked on the unmerged Phase 6 tip
`18953d0a`; merge-target = epic `260605`). The `wsprompt` go:embed loader is fully
retired — `agents-plugin/rsrc/` is now the single prompt source of truth.

Delivered (4 source commits):
- **Infra docs → rsrc** (`96e9c5b6`): ported `executor-wrapup`, `impl-playbook`,
  `subagent-rules`, `delegate-orientation` to flat rsrc files; manifest + wsflow
  rsrc mirror regenerated (both up-to-date guards green).
- **`infra.read` → rsrc** (`36aaeb7f`): `wsdoc.ReadInfra` loads from rsrc via
  `wsrsrc.Load`/`ResolveRoot`, preserving bare-stem/path-traversal rejection.
- **Register prompt path → rsrc** (`64699c0d`): `wsagent.Register` replaced
  `wsprompt.Resolve` with `loadDelegateOrientation` (rsrc) + `SystemPromptText`
  join; removed `RegisterOptions.Prompts/PromptRefs/ConditionalPromptRefs`, the
  `ConditionalPromptRef` type, `promptSpecs`/`resolveConditionalPromptRefs`, and
  the CLI `agents register --prompt`/`--prompt-ref` flags. Added package
  `TestMain` (WS_RSRC_ROOT default) to wsagent/mcp/cmd test packages.
- **Bundle collapse + package deletion** (`6873b480`): removed
  `wsprompt.Bundle`/`ContentSHA256`/`BundleInfo` from `runtime.info` and the CLI
  runtime info/capabilities; dropped `prompt_bundle` from both `runtime.json`;
  removed the launcher's `prompt_bundle_compatible` + the capabilities hash block
  (both ws + wsflow launchers, kept byte-identical); **deleted the entire
  `internal/wsprompt/` package** (loader + 11 embedded prompt bodies incl. the
  confirmed-dead `skeleton-populator`/`skeleton-reviewer`/`sprint-survey` stems +
  8 infra docs). The launcher's guards already no-op on an absent contract hash,
  so validation stays self-consistent.

**Verification.** `go build/vet ./...` clean; full module suite green (13
packages, manifest + drift guards included). Partitioned review (correctness +
fit + test, independent native reviewers) all `[clean]`. Launcher-capabilities
python suite green (20 OK); wsflow runtime-contract python suite green; `infra.read`
verified live. spec `024e109a` (plugin-runtime/mcp-tools/named-agent-runtime/
api-documentation-cache reconciled); mental-model `8b0b261a` (prompt-bundle.md
line-27 rewrite to single-source + 5 peer docs).

**Deviations / open items.**
- *Lead-driven edit + native partitioned review* (continues the M3/260611 Phase
  1-6 deviation): the ws delegate path is crash-prone and the live MCP server
  reads a cached rsrc root, so authoritative verification was the Go layer + grep.
- *AgentDefinition.PromptRefs persisted field kept* (now records only the
  `delegate-orientation` marker) to avoid an out-of-scope wsstore schema change;
  the input-side `RegisterOptions` prompt fields are gone.
- *Pre-existing, unrelated test failures (NOT Phase 6b):*
  `agents-plugin/tests/test_skill_dispatch_contracts.py` (3) asserts old inline
  SKILL.md text the Phase 5 skill→playbook migration replaced with thin stubs;
  the wsflow `lead-workflow-manual` ws-dotted-namespace failure
  (`260612-bug-wsflow-skill-ws-dotted-namespace-ref`) also persists. No Phase 6b
  commit touches `agents-plugin/skills/`.

> Forward (Phase 7): with every prompt/infra/delegate consumer on rsrc and the
> embedded bundle gone, the `agents.*`→`ws.mercenary.*` rename now touches a
> single already-converged call shape (render+spawn), not the legacy
> `register(prompts)` sites.

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
- **Phase 6** (wsflow provisioning + render/api convergence) — touches
  `#260529-prompt-render-tool` in `ai-docs/spec/mcp-tools.md`: the wsflow
  `prompt.render` tool's source changes from the embedded `wsprompt.RenderSource`
  bundle to rsrc loading (the render-eligibility allowlist + namespace-substitution
  contract are preserved; the backing loader is reconciled). `api.ask`'s prompt
  source moves to rsrc. Also touches `#260513-wsflow-agentless-plugin-package` in
  `ai-docs/spec/plugin-runtime.md`: wsflow now carries a **generated rsrc subtree**
  (it previously shipped none), so the agentless-package description gains a
  generated-rsrc provisioning note. Boundary: does NOT touch the bundle-hash metadata
  contract (Phase 6b).
- **Phase 6b** (finish retirement + package deletion) — `infra.read`'s doc source
  moves from the embedded bundle to rsrc. Removes the embedded-prompt-bundle
  hash/`runtime.json` metadata surface from the launcher-validation contract
  (`prompt_bundle.content_sha256` and the prompt list drop out of the
  `runtime.info`/`runtime.capabilities`/`runtime.json` contract). Reconciles any
  spec text that described the embedded prompt bundle as the prompt source of truth.
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
