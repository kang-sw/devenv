---
title: Pi tier resolution rejects ws's backend-keyed model slugs, so every delegated child inherits the parent model
related:
  260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit: owns the catalog accept rule and warning line this ticket extends; its "bare id is ambiguous" rationale is superseded by backend disambiguation
  260906-bug-ws-pi-model-warning-keeps-reappearing: warning cardinality; independent fix
  260905-feat-ws-pi-harness-config-layer: seeded the harness-keyed pi bucket and the resolve_agent read tool
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d512d05213735b2e
sage-review-completeness-reviewed: d512d05213735b2e
---

# Pi tier resolution rejects ws's backend-keyed model slugs, so every delegated child inherits the parent model

## Background

Owner dogfood, 2026-09-06: children requested at tier `small` and `medium`
ran on the lead's own model and provider usage was drained. Root cause,
confirmed from the persisted config, the adapter source, and the session
transcripts:

- ws stores `agents.tier` values as `{backend, model, effort}` where `model`
  is a backend-keyed slug (`gpt-5.6-luna`), the same shape as its own seed
  defaults and its `codex` bucket. The `pi` bucket in the owner's project
  config holds exactly that shape for all four tiers
  (`small: codex/gpt-5.6-luna high`, `medium: codex/gpt-5.6-terra high`,
  `large: codex/gpt-6-astra medium`, `xlarge: codex/gpt-6-astra xhigh`).
  `config.tune` and `SetAgentsTierForHarness` validate effort only; the
  model string is trimmed and stored verbatim for every harness.
- The adapter's `resolveModelForAliasViaWsMcp` (`agents-plugin-pi/src/spawner.ts`)
  accepts a `resolved_from: "pi"` answer only when `model` is an exact
  `provider/id` catalog entry (before `e5e09187`, when it contained a `/`).
  `gpt-5.6-luna` fails both rules, so every tier resolution degrades to
  inherit: `openai-codex/gpt-6-astra`, the lead's model, at Pi's default
  thinking level `medium` (inherit passes no effort; the RPC path never
  calls `setThinkingLevel`, the process path passes no `--thinking`).
- The cascade is two-level: the `full-worker` and `execute-worker` tool
  groups include `explore`, which resolves `small` and inherits the worker's
  model in turn; `ws-execute` defaults every non-complex call to `small`.
  Transcript tally for 2026-09-06: 36 child sessions under `ws-pi-agent-*`,
  564 assistant responses, all on `openai-codex/gpt-6-astra` at `medium`,
  roughly 83 USD by catalog prices against a 50x input-price gap to
  `gpt-5.6-luna`; the two lead sessions added about 34 USD.
- The warning commit `e5e09187` changed diagnostics only, not the outcome,
  and the three Pi sessions running that day started before it landed and
  never loaded it. Its suggestion heuristic would have proposed
  `openai-codex/gpt-5.6-luna` (exact id match).

The tier ticket's Decisions treat a bare id as ambiguous across providers
and require the owner to write `provider/id`. That reasoning ignored the
`backend` field ws returns beside the model: with `backend` present the
provider is determined, and the value ws seeds and displays by default
should resolve on Pi without a second, Pi-only spelling.

Immediate mitigation available to the owner without any code change:
re-tune each `pi` tier to the `provider/id` form
(`config.tune(key: "agents.tier", harness: "pi", value: {tier: "small",
model: "openai-codex/gpt-5.6-luna", effort: "high"})` and likewise for
`medium`/`large`/`xlarge`); the accepted shape works on the current adapter.

## Decisions

- **Derive the Pi provider from `backend` when the model has no `/`.** After
  `resolved_from === "pi"` and before the catalog check, a slash-less model
  is expanded to `<provider>/<model>` through a fixed adapter-owned map:
  `codex` -> `openai-codex`, `claude` -> `anthropic`. The expanded string
  then goes through the existing exact-membership and `hasConfiguredAuth`
  rules unchanged; on acceptance the tier's `effort` applies as a genuine
  hit, exactly as for a `provider/id` value. A model that already contains
  a `/` is used as written, never re-prefixed.
- **Unmapped backend keeps today's path.** A slash-less model whose
  `backend` is empty, `pi`, or anything outside the map is rejected with
  `why: "unknown"` and the suggestion heuristic, as today. A mapped but
  absent expansion (`codex` + `gpt-5.6-lunar`) is rejected with
  `why: "unknown"`, and the warning names the expanded string it tried.
- **The `config.resolve_agent` parse gains `backend`.** The adapter's parsed
  shape adds `backend?: string`. In the resolver's `rejected` detail,
  `model` is the string that was checked against the catalog (the expanded
  form when expansion applied), and a new `stored?: string` carries the raw
  stored value when it differs. Both warning lines derive the provider from
  `model`, so the `no-auth` line names the expanded provider rather than
  slicing a slash-less value; the `unknown` line names the stored value and
  the expansion it tried. The list-row marker owned by the cardinality
  ticket reads the same detail.
- **Map entries are confirmed against the catalog at implementation.**
  `codex` -> `openai-codex` is evidenced by the owner's transcripts;
  `claude` -> `anthropic` is the expected Pi provider id and must be
  checked against a real `ctx.modelRegistry` entry before the test encodes
  it. A wrong entry degrades to today's rejection, never to a broken spawn.
- **The tier ticket's deferred spec pass writes this rule.** The related
  tier ticket landed its Phase 1 code with its three spec passages
  deferred; whichever ticket reaches the spec first writes the
  backend-expansion accept rule, and the tier ticket's "exactly
  `provider/id`" sentence is never written into the spec. The tier ticket's
  frontmatter names this ticket so that pass cannot miss it.
- **The advisory reconciles automatically.** `computePiAliasTableReport`
  goes through the same resolver, so a backend-keyed table stops reporting
  every tier rejected once this lands.
- **Rejected: validating model shape in ws-mcp per harness.** The shape a
  host expects is host logic; ws-mcp stays harness-neutral and keeps
  storing what the caller wrote. The owner clause allows harness-keyed
  config surfaces, not host-specific value grammars.
- **Rejected: requiring the owner to spell every tier as `provider/id`.**
  That is the current state and it failed silently for a full day; ws's
  own seed and `config.list` output use the backend-keyed form, so the
  adapter must read it.
- **Rejected: forwarding the parent's thinking level on inherit.** Inherit
  keeps passing no effort; changing that is a separate cost decision.
- **Supersedes** the tier ticket's "a bare id is ambiguous across providers"
  rationale: it is ambiguous only without `backend`, which ws always
  returns for a stored tier.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-spawner-model-tier-inherit}`: the accept
rule becomes `resolved_from === "pi"` AND exact catalog membership of the
model after backend expansion (a slash-less model is prefixed from a fixed
`backend` -> Pi provider map: `codex` -> `openai-codex`, `claude` ->
`anthropic`; a slashed model is used as written); the "partial tune seeds a
codex-shaped default" sentence is rewritten to say such a value now
resolves through the expansion. `{#260903-pi-model-catalog-unset-advisory}`:
the per-tier report applies the same expansion.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change.
- The provider map is a small exported constant beside the resolver so the
  advisory and the tests share it; adding a backend later is a one-line
  change.
- `resolveModelForAliasViaWsMcp` stays IO-free with respect to the catalog
  and keeps its signature apart from the widened parsed shape.
- Existing tests for `provider/id` acceptance, `unknown`, `no-auth`,
  transport failure, and non-`pi` `resolved_from` keep passing unchanged.

## Phases

### Phase 1: Expand backend-keyed slugs before the catalog check

Add the backend -> provider map and the expansion step to
`resolveModelForAliasViaWsMcp`; widen the parsed `config.resolve_agent`
shape with `backend`; make the rejected detail's `model` the checked
(expanded) string with `stored` beside it, and name both in the `unknown`
line when they differ; confirm the `claude` map entry against the live
catalog; amend the two spec passages under Spec Impact, writing the
expansion accept rule whether or not the tier ticket's deferred spec pass
has run. Tests: `codex` +
`gpt-5.6-luna` with `openai-codex/gpt-5.6-luna` in the catalog is accepted
with the tier's effort; `claude` + `sonnet` expands to `anthropic/sonnet`;
`codex` + `gpt-5.6-lunar` is rejected `unknown` with the expanded string as
`model`, the raw value as `stored`, and the `openai-codex/gpt-5.6-luna`
suggestion; a slashed value is never re-prefixed; empty or `pi` backend
with a slash-less model stays `unknown`; an expanded hit whose provider
lacks auth is `no-auth` and its rendered line names `openai-codex` as the
provider; the advisory report shows no rejected tier for a fully
backend-keyed table.
Live check (owner-run): with the project config left exactly as it is
today, spawn a `small` worker and confirm `ws-agent-list` shows
`openai-codex/gpt-5.6-luna` and the child's transcript shows thinking
`high`; run a lead `explore` and confirm the same model; confirm the
`workflow_manual` advisory no longer lists rejected tiers.
