---
title: playbook.render tier override decoupled from frontmatter tier
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260919-research-mini-lead-intra-ticket-orchestration: motivating consumer — "xlarge = elevated body + xlarge model" needs this override
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

## Constraints

- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/) —
  read before editing the MCP tool surface.
- This is a Go/MCP-tool change only; it edits no shipped `rsrc/` playbook prose,
  so the skill-authoring / wsflow-mirroring / shipped-surface-boundary obligations
  do not apply to this ticket. The consuming prose changes (elevated body,
  `lead-run` dispatch table) belong to the mini-lead research ticket, not here.

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
