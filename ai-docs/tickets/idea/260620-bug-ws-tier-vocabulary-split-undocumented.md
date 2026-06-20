---
title: capability-tier vs model-alias vocabulary split is invisible at caller surfaces
related-mental-model:
  - named-agent-runtime
  - prompt-bundle
---

# capability-tier vs model-alias vocabulary split is invisible at caller surfaces

## Background

Found while dogfooding the playbook-factory delegate-render path. The tier system
has two intentional planes (per the `firstClassTierToAlias` comment in
`agents-plugin-tool/internal/mcp/playbook_tools.go`): a first-class capability
tier `small/medium/large/xlarge` (abstraction layer, spoken by playbook
frontmatter / render / register) and a model-alias layer `light/core/deep`
(concrete-model layer, where per-harness model config lives). The translation is
by design: `small→light`, `medium→core`, `large→deep`, `xlarge→deep`.

The split is not surfaced to callers, and the three tools that touch tiers accept
*disjoint* synonym sets:

| Surface | tier vocabulary accepted / emitted |
|---|---|
| `playbook.render` return | emits `recommended-tier: small` (the frontmatter capability tier) |
| `ws.mercenary.register(tier)` | `small/medium/large/xlarge` **and** `light/core/deep` (translates internally) |
| `config.agents_tier(tier)` | `light/core/deep` **and** `haiku/sonnet/opus` only — **rejects** `small/...` (`normalizedTier` → "tier must be light, core, or deep") |

So the only vocabulary every tier surface accepts is `light/core/deep`, yet
`render` emits `small` and `register`'s schema advertises
`small/medium/large/xlarge`. The `small↔light` / `medium↔core` /
`large,xlarge↔deep` bridge lives only in an internal Go comment — not in the
`config.agents_tier` param description, the `register` param description, or the
`lead-tune` "tune model tier" handler (which speaks only `light/core/deep`).

Caller-visible failure: a lead renders a delegate, reads `recommended-tier:
small`, wants to change which model backs that delegate, and invokes the
documented model-tuning tool `config.agents_tier` with `tier: small` — which is
rejected with no hint that `small` means the `light` alias. Nothing at the
surface connects the recommended-tier a lead just saw to the alias they must
configure.

This is a coherence/discoverability gap, not a code bug: the render→register
happy path works because register translates. It bites only at the tuning seam.

## Direction

Make the two planes legible where callers meet them. Options, roughly in
increasing scope:

- **Document the bridge** at every caller-facing tier surface: the
  `config.agents_tier` and `ws.mercenary.register` tier param descriptions, and
  the `lead-tune` "tune model tier" handler, should state the
  `small↔light / medium↔core / large,xlarge↔deep` mapping so a lead who saw a
  `recommended-tier` knows which alias to configure.
- **Accept both vocabularies symmetrically** at `config.agents_tier`: route its
  `tier` through `firstClassTierToAlias` too, so `tier: small` configures the
  `light` alias instead of erroring. Then every tier surface accepts the same
  capability vocabulary, and `light/core/deep` remain the canonical alias keys.
- Decide whether `xlarge` collapsing to `deep` (two capability tiers → one
  alias) should be visible to the lead, since configuring `deep` then silently
  also backs `xlarge` delegations.

Skill/doc edits run under `lead-skill-authoring`; any docstring change to a
shipped tool surface also touches both `runtime.json` contracts and the rsrc
manifest. Narrower than `260611-research-ws-per-role-delegation-tuning-config`:
this is about reconciling the *existing* two tier planes, not adding per-role
axes.
