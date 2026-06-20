---
title: tier vocabulary split is redundant — collapse to one capability vocabulary with direct capability→model config
related:
  260611-research-ws-per-role-delegation-tuning-config: collapse simplifies its 3-layer config model to 2 layers
spec:
  - 260620-tier-vocabulary-collapse-direct-model-map
related-mental-model:
  - named-agent-runtime
  - prompt-bundle
---

# tier vocabulary split is redundant — collapse to one capability vocabulary with direct capability→model config

## Background

Found while dogfooding the playbook-factory delegate-render path. The tier system
carries two vocabularies for what is effectively one axis:

- a first-class **capability tier** `small/medium/large/xlarge`, spoken by
  playbook frontmatter `tier:`, `playbook.render` (`recommended-tier`), and
  `ws.mercenary.register(tier)`;
- a **model alias** `light/core/deep`, the only vocabulary `config.agents_tier`
  accepts, where the per-harness concrete `(backend, model, effort)` is configured.

They are bridged by the hardcoded `firstClassTierToAlias`
(`agents-plugin-tool/internal/mcp/playbook_tools.go:223`):
`small→light, medium→core, large→deep, xlarge→deep`.

The original caller-visible symptom: a lead renders a delegate, reads
`recommended-tier: small`, wants to change which model backs it, and calls the
documented model-tuning tool `config.agents_tier(tier: small)` — which is
**rejected** (`normalizedTier` accepts only `light/core/deep` + `haiku/sonnet/opus`,
`config.go:428`; error "tier must be light, core, or deep", `config.go:108`).
Nothing at any surface connects the `recommended-tier` a lead just saw to the
alias they must configure. The render→register happy path works only because
`register` translates internally (`server.go:972`).

Initially captured as a documentation/coherence gap ("document the bridge").
Fan-out verification (5 read-only audits, 2026-06-20) reframed it: the split is
not just undocumented, it is **redundant** in the current structure, and the
sound resolution is to collapse it rather than document it.

## Verified Evidence

Five independent audits, cited to source. None of these conclusions rests on
"the spec decided so" — they are structural facts.

1. **Capability tier is inert beyond the bridge.** `firstClassTierToAlias`
   (`playbook_tools.go:223`) is the *sole* consumer of `small/medium/large/xlarge`.
   After translation, no code branches on the four-way distinction; `recommended-tier`
   is display-only metadata (`playbook_tools.go:243-248`, surfaced at `:632`).
   `xlarge` and `large` resolve to an **identical** `(backend, model, effort)` —
   both map to `deep` (test `mercenary_surface_test.go:555` comment
   `"xlarge": "deep" // no legacy alias`; `TestMercenaryTierRoutingResolvesCustomModel`
   `:611-646` does not distinguish them). So one of the two vocabularies is pure
   passthrough today.

2. **The alias layer is a near-identity key, not added machinery.** The genuine
   value in `wsconfig` — per-harness polymorphism (`light` → haiku on claude,
   gpt-5.4-mini on codex; `ModelAliases[alias][harness]`, `config.go:291-309`),
   effort isolation (`config.go:53-56`), and backend-affinity guarding
   (`useAliasMappingForBackend`, `config.go:396-410`) — is **orthogonal to the key
   name** and survives a rename of the keys to capability vocabulary unchanged.
   The second vocabulary adds zero machinery.

3. **No hard blocker to collapsing.** The three silent-failure risks are persisted
   config map keys (`config.go:32-35`, stored in config.json), the SQLite agent
   `tier` column (`wsstore/store.go:62,592-636`, metadata-only — call-time reads
   concrete backend/model/effort, not tier), and the hardcoded tool enum
   (`server.go:2209`). All three are absorbed by extending the **existing**
   `normalizedTier` read-compat path (`config.go:428`, already does
   `haiku/sonnet/opus → light/core/deep`) to also accept capability vocabulary.
   `schemaVersion=1` is never incremented and there is no migration machinery
   (`config.go:13`, additive backfill only) — and none is needed with read-compat.
   Cost is mechanical churn: ~132 code occurrences (mostly test literals —
   `config_test.go` ~46, `config.go` ~20, `agent_test.go` ~15, others), ~15 doc
   sections, plus the byte-identical `agents-plugin-wsflow/rsrc` mirror doubling.

4. **The native model hint is preservable (hard user constraint).** Today playbooks
   hand-pick an alias-named var (`{{.LightModel}}`, e.g.
   `agents-plugin/rsrc/reference-discovery/reference-discovery.md` line 16) injected
   by the fixed-list `resolveModelVars` (`playbook_tools.go:85-103`). `pb.Meta.Tier`
   is already in scope at the injection site (`renderPlaybookBody`, read at `:632`,
   vars built at `:634`), so the hint can be derived from the playbook's own `tier:`
   into a single `{{.RoleModel}}` with a minimal change (add a `tier` arg to
   `buildPlaybookVars`, replace `resolveModelVars`). Concrete model string and
   `recommended-tier` both still ship. Native hint is not a blocker.

5. **The dual state is substantially accretion, and the future feature does not
   need two vocabularies.** The `light/core/deep` config layer shipped first
   (2026-05-08, spec `#260508-model-alias-config-tools`); the capability vocabulary
   was retrofitted 34 days later (2026-06-12, `#260612-first-class-tier-vocabulary`).
   Spec language is transitional, not a settled end-state: `config.agents_tier`
   "**remains** keyed by the `light`/`core`/`deep` alias" (`mcp-tools.md:219-220`)
   and a "**pending** `config.model_alias` rename" (`mcp-tools.md:271`).
   `260611-research-ws-per-role-delegation-tuning-config` proposes a 3-layer config
   `(skill,role)→tier`, `tier→alias/model`, `alias→concrete` — but these are three
   *maps*, not three *vocabularies*: layers all key off one capability word. The
   per-role override is `(role)→tier`; the model binding is `tier→model`. Collapsing
   the vocabularies merges the middle layer away and reduces 260611 from 3 layers to
   2. The future feature is *easier* after collapse, not blocked by it.

## Decision

**Collapse upward to a single capability vocabulary (`small/medium/large/xlarge`)
with a direct `capability → (backend, model, effort)` config map.** The
`capability → alias → model` indirection is redundant: the alias is a renamed
intermediate key that adds no configurability a direct map lacks, and actively
*subtracts* it (it forces `large` and `xlarge` to share `deep`, which a user
cannot split). A direct map removes the locked bridge, gives `xlarge` an
independent slot, and eliminates the discoverability footgun class entirely.

The composite `capability → model` is *already* user-controllable today, only
through alias names — e.g. "medium and large both to sonnet" is achievable by
setting `core=sonnet` and `deep=sonnet`. The resolve hook
(`ResolveAgentForHarnessConfig`, `config.go:175-218`) is vocabulary-agnostic;
keying it by capability directly is a normalizer + key-rename change, not a
structural one. So the room for direct tuning is already open and the alias step
buys nothing at this scale.

### Rejected alternatives

- **Keep both vocabularies / document the bridge only** (the original Direction):
  pays the full cost (two synonym sets, the footgun seam, doubled surface area)
  for benefit that is unrealized — capability tier is inert (Evidence 1) and the
  alias adds no machinery (Evidence 2).
- **Collapse downward to `light/core/deep`**: drags model-budget words up into
  authoring and loses the "authors speak capability, models stay hidden" boundary,
  and cannot name a 4th tier. Collapse *up* keeps that boundary and adds `xlarge`.
- **Pending `config.model_alias` rename (`#260612`/`260611` follow-up)**: only
  renames the tool while keeping `light/core/deep` keys — preserves the redundant
  layer. This decision supersedes that planned rename.

## Phases

Single-vocabulary collapse, transition-safe. Sequential: each phase is one
reviewable, verifiable behavior; Phase 2 depends on Phase 1, Phase 3 on both.
Skill/doc edits run under `lead-skill-authoring`. This is observable API/protocol
surface change (Always-ask territory). Spec addressed by the `🚧` planned entry
`#260620-tier-vocabulary-collapse-direct-model-map`.

### Phase 1: Capability-keyed model config + read-compat

Re-home the model-config surface so it accepts, stores, and resolves the
capability vocabulary directly, retiring the `firstClassTierToAlias` bridge for
the config path.

- Re-key `Agents.Tiers` / `Agents.ModelAliases` semantics to capability
  vocabulary (`config.go:32-35`); give `xlarge` its own resolvable entry instead
  of folding onto `deep`.
- Extend `normalizedTier` (`config.go:428`) to fold `light→small`/`core→medium`/
  `deep→large` and keep `haiku/sonnet/opus`; apply at config load
  (`config.go:75-85`) and SQLite tier read (`wsstore/store.go:592-636`).
- Keep the per-harness map shape, effort field (`config.go:53-56`), and
  backend-affinity guard (`config.go:396-410`) unchanged — only the key
  vocabulary changes (Evidence 2).
- Update the `config.agents_tier` tool enum + help (`server.go:2209`,
  `cmd/ws-mcp/main.go`) to capability vocabulary; route `ws.mercenary.register`
  tier directly (drop the `firstClassTierToAlias` call at `server.go:972`).
- Update the full `runtime.json` enum if it pins one; `wsflow` omits
  `config.agents_tier`.

Verification: existing `light/core/deep` config.json and stored agents still
resolve unchanged; `config.agents_tier(tier: small)` succeeds; `xlarge`
configures independently of `large`; `config_test`/`agent_test`/
`mercenary_surface_test` pass with capability literals. Deferred: template var
(Phase 2), doc/skill prose (Phase 3).

### Result (ea545a51) - 2026-06-20

Implemented. `wsconfig` is now keyed by the capability vocabulary
(`small`/`medium`/`large`/`xlarge`); `config.agents_tier` accepts capability
tiers (schema enum updated) with `light`/`core`/`deep` kept as read-compat
synonyms. `firstClassTierToAlias` is retired (`server.go:972` passes the tier
through directly). `xlarge` is independently configurable, seeded to the `large`
default. Reviewed partitioned (correctness/fit/test) — clean; test-coverage
polish landed in `cb46727e`.

Read-compat mechanism: a load-time map-key migration (`normalizeLegacyTierKeys`)
remaps persisted `light`/`core`/`deep` keys to capability keys in-memory before
default backfill (capability key wins on collision; no file rewrite,
`schemaVersion` stays 1). `normalizedTier` folds `light/core/deep` +
`haiku/sonnet/opus` to capability words; `ModelAlias` recognizes alias *names*
only and deliberately excludes `haiku/sonnet/opus` (concrete models — verified
across callers `config.go:179,320`, `wsagent/agent.go:525`).

Plan corrections (code-grounded deviations from the phase bullets):
- **No `wsstore/store.go` change.** The SQLite `tier` column is vestigial
  metadata (agents resolve concrete `(backend, model, effort)` at registration);
  normalizing it would require importing `wsconfig` into `wsstore`, a forbidden
  reverse-import cycle. Old stored `light` labels are cosmetic.
- **No `runtime.json` change.** It pins no tier enum for `config.agents_tier`,
  only a version range.
- Capability-vocabulary spots updated beyond the listed bullets:
  `formatConfigView` aliases list (`server.go`) and the `agent.go` empty-tier
  fallback (`core`→`medium`).

> Forward (Phase 3): also update the `named-agent-runtime` and `mcp-runtime`
> mental models — they still describe the config layer as `light/core/deep`-keyed.
> Deferred from Phase 1 on purpose: the vocabulary is only fully settled after
> Phase 3, so updating mid-migration would describe a transient state.

### Phase 2: Tier-derived native model hint

Replace the fixed alias-named template-var set with a single tier-derived
`{{.RoleModel}}` (Evidence 4; native hint preserved).

- Add a `tier` arg to `buildPlaybookVars`; replace `resolveModelVars`
  (`playbook_tools.go:85-103`) with a `pb.Meta.Tier`-derived resolver; update
  `reservedToolVarNames` (`:51-66`).
- Migrate delegate playbooks' `variables:` frontmatter and bodies from
  `{{.LightModel}}`/`{{.CoreModel}}`/`{{.DeepModel}}` to `{{.RoleModel}}`
  (`reference-discovery` and any other model-var playbooks).
- Regen the `agents-plugin-wsflow/rsrc` mirror (`WS_REGEN_WSFLOW_RSRC=1`).

Verification: `playbook.render(reference-discovery)` substitutes the concrete
model resolved from `tier:`; `recommended-tier` still emitted; `playbook_tools_test`
and the wsflow mirror drift guard pass. Depends on Phase 1.

### Phase 3: Single-vocabulary docs, skills, and spec finalization

Align caller-facing prose to one vocabulary and mark the spec implemented.

- Rewrite `lead-tune` "tune model tier" handler + Storage notes to capability
  vocabulary; remove bridge prose; update `lead-workflow-manual` register/render
  notes.
- Clean the superseded "pending `config.model_alias` rename" references
  (`mcp-tools.md:256,271`).
- Convert the `🚧` planned callout
  `#260620-tier-vocabulary-collapse-direct-model-map` to implemented (remove `🚧`,
  fold into body) and reconcile `#260612`/`#260508` body vocabulary; the commit
  `## Spec` references the stem.
- Regen the wsflow rsrc mirror for skill-text changes.

Verification: docs/skills speak only capability vocabulary (`light/core/deep`
appears only as documented read-compat synonyms); `spec_index.verify` clean; no
`{{.LightModel}}`-family vars or alias-keyed tuning instructions remain. Depends
on Phases 1-2.

## Documentation Conflicts (must reconcile)

This direction conflicts with shipped documentation and requires conscious
re-opening, not silent contradiction:

- **`#260612-first-class-tier-vocabulary`** (`mcp-tools.md:211-220`) states
  `config.agents_tier` "remains keyed by `light/core/deep`" and frames the
  locked `small↦light…xlarge↦deep` mapping as intended. Collapse-up reverses the
  "remains keyed" decision; `#260612` must be re-opened/amended, not bypassed.
- **The pending `config.model_alias` rename** (`mcp-tools.md:271`) is superseded:
  that rename kept `light/core/deep`; this collapses them.
- **`lead-tune`** "tune model tier" handler and Storage section speak only
  `light/core/deep` and must be rewritten to capability vocabulary.
- **`260611-research-ws-per-role-delegation-tuning-config`**: collapse simplifies
  its 3-layer model to 2; coordinate so the per-role surface is designed against
  the single vocabulary.

## Resolution: direct map, even at scale

The reusable-palette scenario (the alias as a named model-bundle that multiple
capabilities reference, with a user-configurable `capability → slot` assignment)
was the one design that could have justified two vocabularies. Decided against
**even at larger scale and richer capability names** (e.g. `quick`,
`heavy-thinker`): named capabilities are self-documenting, so the alias adds no
clarity, and the palette's only real benefit (define-once / edit-the-group-once)
is dominated by the cost of the indirection plus the per-capability freedom it
removes. Direct `capability → (backend, model, effort)` stands.

Direct mapping also keeps more room open, not less. A capability-keyed entry is
the natural place to later carry an **execution-dispatch hint**
(`capability → [native subagent | mercenary]`, alongside backend/model/effort):
native consumes an advisory model hint while mercenary binds the model, so the
dispatch choice belongs next to the model choice. An alias indirection would add
a seam over where that hint lives. This is forward room to preserve, not a
planned requirement.
