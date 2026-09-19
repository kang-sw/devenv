---
title: playbook.render tier override decoupled from frontmatter tier
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260919-research-mini-lead-intra-ticket-orchestration: motivating consumer — "xlarge = elevated body + xlarge model" needs this override
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a309921888669d29
sage-review-completeness-reviewed: a309921888669d29
completed: 2026-09-19
---

# playbook.render tier override decoupled from frontmatter tier

## Background

`playbook.render` binds a playbook's model and recommended tier solely to its
frontmatter `tier:` (`pb.Meta.Tier`). There is no way to render a body at a tier
other than the one its frontmatter declares:

- `renderPlaybookBody` reads `recommendedTier := pb.Meta.Tier`
  (`agents-plugin-tool/internal/mcp/playbook_tools.go:777`) and passes it to
  `buildPlaybookVars` (`:270`), which resolves the reserved `RoleModel` var from
  that tier via `resolveRoleModelVar` (`:111`) and **overwrites** any
  caller-supplied value (`:298-303`).
- `withRecommendedRenderBinding` (`:366`) / `withRecommendedTier` (`:351`)
  append `recommended-tier:` / `recommended-model:` / `recommended-reasoning-effort:`
  to the payload, all derived from that same frontmatter tier.
- The MCP tool handler (`server.go:1670-1728`) and its input schema
  (`server.go:4128-4139`, properties `{session_key, name, context, root_override}`,
  `required: ["name"]`) expose no tier/model override. `context` cannot reach
  `RoleModel` because it is a reserved var the render overwrites.

The config layer is already capable: `ResolveAgentForHarnessConfig`
(`agents-plugin-tool/internal/wsconfig/config.go:220-263`) short-circuits on an
explicit `model` and otherwise resolves per tier, and `ResolveAgentTierForHarness`
(`:273`) resolves a fixed tier and rejects unknown/empty tiers (`:274-276`). The
gap is purely that the render path never threads an override into these resolvers.

This blocks the "two playbook bodies, not three" decision in
`260919-research-mini-lead-intra-ticket-orchestration` (Confirmed Decision 5):
that design has one `elevated` body (frontmatter `tier: large`) serve both large
and xlarge dispatch, with the lead overriding the model to the xlarge tier at
render time. Without a render-time tier override, xlarge still needs a separate
body — the exact duplication that ticket retires. This is the feasibility gate
for that approach, split out as its own tooling primitive.

## Decisions

- **A tier override, not a raw model string.** The new arg names a tier
  (`small`/`medium`/`large`/`xlarge`), staying in tier vocabulary and letting the
  config resolver derive `{model, effort}`. This matches how every other consumer
  reasons about dispatch (the lead grades risk → tier), and keeps the recommended
  binding self-consistent (recommended-tier and recommended-model both reflect the
  override). A raw model override is not exposed here — the config layer supports
  it, but no consumer needs to bypass tier vocabulary.
- **Optional and additive.** The arg is optional; absent, render behaves exactly
  as today (frontmatter tier). `required` stays `["name"]`, so this is a
  backward-compatible MCP surface addition, not a semantics change to existing
  callers.
- **Unknown tier is rejected, not coerced.** Reuse `ResolveAgentTierForHarness`'s
  existing rejection of unknown/empty tiers (`config.go:274-276`) so a typo fails
  loudly at render rather than silently coercing to `medium`.
- **The override drives both `RoleModel` and the recommended binding.** The
  consumer (`lead-run` step 5) spawns "at the tier the render recommends", so the
  override must reach `recommended-tier`/`recommended-model` too — not only the
  in-body `RoleModel`. Overriding one without the other would spawn at the wrong
  model.
- **Realizes a deferred item from `260611`, by a different mechanism.**
  `260611-refactor-ws-tier-taxonomy-delegate-tier-routing` (Phase 2) explicitly
  deferred a per-render tier override — "a per-render `tier` override arg on
  `playbook.render` is explicitly deferred to the `(skill, role) → tier`
  role-config surface" — rather than rejecting it. This builds it as an MCP arg
  instead of that (never-built) `config.role_tier` lookup, because a role-keyed
  config maps each role to a fixed tier and cannot express the per-ticket dispatch
  grade (the same `elevated` body dispatched large vs xlarge by risk). The override
  stays inside 260611's declarative principle: it invents no workload tier, it
  selects within the fixed taxonomy, and the lead supplies it at dispatch — the
  authority `260915-refactor-lead-run-dispatch-time-tier-judgment` already
  established. Absent an override, 260611's frontmatter pass-through is unchanged.

## Constraints

- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/) —
  read before editing the MCP tool surface.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- This is a Go/MCP-tool change only; it edits no shipped `rsrc/` playbook prose,
  so the skill-authoring and wsflow-mirroring obligations do not apply to this
  ticket. shipped-surface-boundary.md *does* apply, though: its scope is not
  limited to `rsrc/` prose — it covers "every string `agents-plugin-tool/`
  emits to an agent (todo instructions, advisories, banners, doctor checks,
  tool descriptions)" (`ai-docs/manuals/shipped-surface-boundary.md:9-13`), and
  every existing `playbook.render` schema property (`session_key`, `name`,
  `context`, `root_override`) carries a description via `stringProperty`
  (`server.go:4625`); the new `tier_override` property will need one too. The
  consuming prose changes (elevated body, `lead-run` dispatch table) belong to
  the mini-lead research ticket, not here.

## Prior Decisions

- 260919-research-mini-lead-intra-ticket-orchestration (2026-09-19, Confirmed Decisions): "Two playbook bodies, not three... xlarge = elevated prose + xlarge model override via config.resolve_agent, not a third body." — bearing: supports
- 260909-chore-retire-mercenary-surface (2026-09-09, ticket Decisions): "Tier resolution survives the removal. wsconfig.ResolveAgentForHarnessConfig and config.resolve_agent back native model selection through playbook.render's recommended-tier" — bearing: supports
- 260909-refactor-route-resolve-implement-reads-ticket-facts (2026-09-09, commit 92389a0f): "The render already returns a recommended tier, so the lead defers to it instead of carrying a second, drifting answer." — bearing: constrains
- 260714-feat-playbook-tier-model-render-vars (2026-07-14, ticket Decisions): "Coordinate with 260622-feat-playbook-render-tier-label... Both must share one resolution mechanism, not two parallel implementations." — bearing: constrains
- 260622-feat-playbook-render-tier-label (2026-06-22, ticket Decisions): "Preserve the existing recommended-tier: <tier> output line verbatim and add separate additive lines when values resolve." — bearing: constrains
- 260611-refactor-ws-tier-taxonomy-delegate-tier-routing (2026-06-12, commit 54e53d70): "Tier flows declaratively: frontmatter tier: is the single source; playbook.render returns it as recommended-tier... never a caller-invented workload tier." — bearing: reconciled (this ticket realizes 260611's own deferred per-render override; see Decisions)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go (playbook.render handler + input schema), playbook_tools.go (renderPlaybookBody, buildPlaybookVars, resolveRoleModelVar, withRecommendedRenderBinding), playbook_render_surface_test.go |
| scope.surface | internal | no new exported Go symbol; agents-plugin-tool/internal/mcp/ is an unexported internal package, no wsflow mirror |
| scope.new_public_symbol | yes | one new optional MCP arg `tier_override` on playbook.render (input schema server.go:4128-4138); required stays ["name"] |
| scope.new_type_contract | yes | added parameter threads through renderPlaybookBody/buildPlaybookVars/withRecommendedRenderBinding call signatures (playbook_tools.go:270,366,765,777); no new Go type, reuses ResolveAgentForHarnessConfig/ResolveAgentTierForHarness |
| scope.test_surface | existing | playbook_render_surface_test.go:533-555 (TestRenderReturnsFrontmatterRecommendedTier), :458-528 (TestWithRecommendedTier/TestWithRecommendedRenderBinding, recommended-binding format); add override + unknown-tier cases |
| complexity.reuse_points | confirmed | config resolver already resolves decoupled from frontmatter tier (config.go:220-263, explicit-model short-circuit :230-238); unknown-tier rejection (config.go:274-276) |
| complexity.side_effect_risk | moderate | playbook.render is consumed by every dispatch path; the change is gated behind an absent-by-default arg, so existing renders are byte-unchanged |
| risk.correctness | moderate | must thread the override through several call layers and decouple recommendedTier/RoleModel from pb.Meta.Tier without altering the no-override path |
| risk.fit | low | additive optional arg on an existing tool; the config layer already supports the resolution |
| risk.test | low | render-surface tests are golden and pin the frontmatter-tier path; a regression case guards it while new cases cover the override |
| risk.security_or_contract | low | no capability/permission change; the arg only selects a tier for model resolution |

## Phases

### Phase 1: Thread an optional tier override through playbook.render

Add an optional `tier_override` argument to `playbook.render` and thread it so
that, when present, it replaces `pb.Meta.Tier` for both `RoleModel` resolution and
the recommended-tier/model/effort binding; when absent, behavior is unchanged.

Edit surface (verify line numbers at implementation — they are search anchors):

1. **MCP surface** — `agents-plugin-tool/internal/mcp/server.go`: add optional
   `tier_override` (string) to the `playbook.render` input schema (~`:4128-4139`),
   keeping `required: ["name"]`; read it in the handler (~`:1670-1728`) and pass it
   into `renderPlaybook`.
2. **Render core** — `agents-plugin-tool/internal/mcp/playbook_tools.go`: compute
   `effectiveTier := tier_override != "" ? tier_override : pb.Meta.Tier` in
   `renderPlaybookBody` (~`:765`/`:777`) and use it wherever `pb.Meta.Tier` feeds
   model/tier resolution — `buildPlaybookVars` (~`:270`) → `resolveRoleModelVar`
   (~`:111`), and the recommended binding `withRecommendedTier`/
   `withRecommendedRenderBinding` (~`:351`/`:366`). The reserved-var overwrite of
   `RoleModel` (~`:298-303`) now derives from `effectiveTier`.
3. **Validation** — reject an unknown/unresolvable `tier_override` via the existing
   `ResolveAgentTierForHarness` rejection path (`wsconfig/config.go:274-276`); do
   not coerce to `medium`. The config resolver itself needs no change — it already
   supports explicit resolution (`config.go:220-263`).
4. **Tests** — `agents-plugin-tool/internal/mcp/playbook_render_surface_test.go`:
   add a case that renders a body with `tier_override` and asserts both the
   `recommended-tier`/`recommended-model` payload and the in-body `RoleModel`
   reflect the override, not the frontmatter tier; add a regression case asserting
   that with no override the frontmatter tier still governs (guards
   `TestRenderReturnsFrontmatterRecommendedTier` semantics at `~:530-555`). An
   unknown-tier case asserts the render errors rather than coercing.

Out of scope (belongs to the mini-lead research ticket): the `elevated` body
prose, the `lead-run` dispatch/escalation table rewrite, and retiring the
`ticket-worker-escalated` body. This phase ships the primitive only.

### Result (776dbb34) - 2026-09-19

Landed across two commits (`d01c7164`, `776dbb34`) on
`impl/goal/develop/cedar-drift-lantern/agile-fruit-icon`:

- **MCP surface** (`agents-plugin-tool/internal/mcp/server.go`): added optional
  `tier_override` (string) to the `playbook.render` input schema, with a
  `stringProperty` description per shipped-surface-boundary.md; `required`
  stays `["name"]`. The handler reads `params.Arguments["tier_override"]` and
  threads it into `renderPlaybook`.
- **Render core** (`agents-plugin-tool/internal/mcp/playbook_tools.go`): new
  `resolveEffectiveTier(frontmatterTier, tierOverride, harness, configOpts)`
  replaces the direct `pb.Meta.Tier` read in `renderPlaybookBody`. Absent
  override, it returns `frontmatterTier` unchanged (byte-identical no-override
  path, verified by the frontmatter-tier regression test). Present, it
  validates through the existing `wsconfig.ResolveAgentTierForHarness`
  rejection path (loud error on unknown tier, no coercion to medium) and
  returns `wsconfig.NormalizedTier(trimmed)` — a new thin exported wrapper
  around the existing unexported `normalizedTier` in
  `agents-plugin-tool/internal/wsconfig/config.go` — so an accepted alias
  (`"opus"`, `"Large"`) still surfaces as the first-class tier on the
  `recommended-tier` return channel. `tierOverride` threads as a new trailing
  parameter on `renderPlaybookBody`/`renderPlaybook` (placed last so every
  existing call site — production and ~40 test call sites — needed only a
  trailing `""` argument). `printPlaybook`/`playbook.read` intentionally never
  supplies an override, per the ticket's Decisions.
- **Tests** (`agents-plugin-tool/internal/mcp/playbook_render_surface_test.go`):
  `TestRenderPlaybookBodyTierOverride` covers override-replaces-frontmatter
  (both `recommendedTier` and in-body `RoleModel`, plus the composed
  `recommended-tier`/`recommended-model` payload), the no-override regression
  case, unknown-tier rejection, and alias normalization.
  `TestPlaybookRenderToolTierOverride` drives the same override end to end
  through the actual `playbook.render` tool dispatch (`callToolOnce`), added
  in round-1 review to close a coverage gap (see below).

**Decisions taken (not requiring escalation):**
- Added one exported symbol (`wsconfig.NormalizedTier`) beyond the ticket's
  Route Facts claim of no new exported symbol/type — a pure pass-through
  wrapper around the existing `normalizedTier`, needed because
  `ResolveAgentTierForHarness` validates aliases but does not return the
  normalized form, and the `recommended-tier` channel must stay in
  first-class vocabulary. Still package-internal (`internal/wsconfig`), so no
  shipped/external surface widens. Cosmetic deviation from the route facts,
  not a structural one.
- `tierOverride` parameter placed last (after `overrideLookup`) rather than
  adjacent to `configOpts`/`mintRoot`, to minimize test-call-site churn to a
  single trailing argument per call rather than a mid-signature insertion.

**Review:** single full-scope review, two rounds. Round 1 (commit `d01c7164`
reviewed) raised 1 Important (no test exercised `tier_override` through the
actual `playbook.render` tool dispatch, only through `renderPlaybookBody`
directly) and 2 Minor (alias-unnormalized `recommended-tier` line; the
render-core functions now carry 10 positional string-heavy params). Fixed the
Important and the first Minor in commit `776dbb34`; the second Minor was
explicitly flagged non-blocking by the reviewer and left as a future cleanup
(an options struct "next time this signature grows"). Round 2 confirmed both
fixes and raised nothing new: clean.

**Verification:**
- `go build ./...`, `go vet ./...` — clean.
- `go test ./...` (all `agents-plugin-tool` packages) — pass.
- `go test -count=1 ./internal/mcp/... ./internal/wsconfig/...` — pass
  (re-run after the round-1 fix commit).
- `gofmt -l` on every touched file — clean (only pre-existing,
  untouched-by-this-ticket drift in `internal/wsconfig/global.go`).
- `agents-plugin-tool/scripts/smoke-ws-mcp.sh ..` — passes (Level 1 per
  `ai-docs/manuals/ws-mcp.md`).

**Omitted:** the `elevated` body prose, `lead-run` dispatch table rewrite, and
`ticket-worker-escalated` retirement remain out of scope per the ticket, for
`260919-research-mini-lead-intra-ticket-orchestration` to consume this
primitive.
