---
title: "Route the lead-delegate executor model through mandatory config.resolve_agent tier resolution"
related:
  260910-feat-lead-run-worktree-parallel-route: sibling; both tighten how the lead dispatches executors — this one governs delegate model routing, that one governs parallel worktree fan-out
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: fc58427c4dcde683
sage-review-design-reviewed: fc58427c4dcde683
completed: 2026-09-11
---

# Route the lead-delegate executor model through config.resolve_agent tier resolution

## Background

`lead-delegate` is the only lead dispatch path whose executor model is chosen
freehand. Its intro line tells the lead to "choose the executor's
prompt, model, tools, and permissions" (agents-plugin/rsrc/lead-delegate/lead-delegate.md#L7-L9)
with no resolution mechanism, so the
model is picked by raw name outside the `config.resolve_agent` fallback chain
(harness bucket -> default -> codex) that `playbook.render` and `agents.tier`
use for every other worker. `lead-run`/`ticket-worker` receive a config-resolved
`recommended-model` from `playbook.render`; delegate does not. The gap means
delegate model choice is neither config-tunable per harness nor consistent with
the rest of the workflow, and it is directly cost-relevant since the model tier
drives spend.

## Decisions

- **Tier resolution is mandatory, not advisory (D1).** The delegate must resolve
  its executor model through `config.resolve_agent(tier)` rather than naming a
  model directly. This is the cost-control chokepoint and the consistency fix
  that brings delegate onto the same fallback chain every other worker uses.
  Rejected: an advisory "prefer config" hint — it leaves the freehand escape
  open and closes neither the cost nor the consistency gap.
- **The tier judgment stays a one-line qualitative call (D2).** The lead picks
  small / medium / large / xlarge by feel from the assignment's difficulty, with
  no scoring rubric or criteria table. The added guidance is a single sentence —
  judge the difficulty tier, resolve it via `config.resolve_agent` — not a
  decision procedure. Rejected: a difficulty rubric, which is the prose bloat the
  user explicitly wants kept out.
- **Prompt, tools, and permissions stay freehand (D3).** Only the model-selection
  path is standardized; the delegate's intentional flexibility everywhere else
  ("there is no fixed executor role or read-only default") is preserved.
- **The resolution is an inline `config.resolve_agent(tier)` call, not a render
  step (D4).** The delegate has no rendered worker playbook today, so an inline
  tool call is the minimal addition and keeps the prose to one sentence. Rejected:
  giving delegate a light render step to obtain a `recommended-model` — it adds a
  new render surface and more prose for no observable benefit over the direct
  call.
- **`config.resolve_agent` is a canonical ws MCP tool, so this stays within the
  shipped-surface boundary (D5).** The render path already resolves tiers through
  the same chain; referencing the tool in shipped delegate text depends on
  nothing devenv-only and leaks no repository-local rule.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- No shipped playbook may enforce a repository-only rule; the change references only the canonical `config.resolve_agent` MCP tool, not any devenv-local fact.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-delegate/lead-delegate.md, agents-plugin-wsflow/rsrc/lead-delegate/lead-delegate.md |
| scope.surface | internal | no exported symbol changes; shipped playbook prose only |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | TestWsflowRsrcMirrorUpToDate (internal/wsrsrc); python3 -m unittest discover agents-plugin-wsflow/tests |
| complexity.reuse_points | confirmed | config.resolve_agent (agents-plugin-tool/internal/mcp/server.go#L875-L897, already shipped) |
| complexity.side_effect_risk | low | one added sentence to a print-kind playbook plus a byte-identical mirror regen, no runtime code path changed |
| risk.correctness | low | prose-only change with a fixed one-sentence scope (D2) and an existing, already-shipped resolver tool |
| risk.fit | low | reuses the same fallback chain and tool already used by playbook.render/agents.tier |
| risk.test | low | mirror byte-equality and wsflow package tests are existing drift guards, not new test surface |
| risk.security_or_contract | low | no schema, permission, or tool-contract change; D3 keeps prompt/tools/permissions freehand |

## Phases

### Phase 1: Route the delegate executor model through inline tier resolution and mirror

In `agents-plugin/rsrc/lead-delegate/lead-delegate.md`, route the executor model
through `config.resolve_agent(tier)` instead of a freehand pick. The model is
named only in the intro line ("choose the executor's prompt, model, tools, and
permissions", L7-9); `## Assignment` currently says just "Choose these for the
assignment" without naming the model. Add the tier-resolution instruction in
`## Assignment`, where the lead is told how to make the assignment choices — the
lead judges one difficulty tier (small / medium / large / xlarge) by feel and
resolves it via an inline `config.resolve_agent` call, a single added sentence
with no scoring rubric (D1, D2, D4) — and reconcile the intro line so it no
longer implies a freehand model pick. Leave the prompt / tools / permissions
freehand guidance unchanged (D3). Mirror the edit
byte-identically to `agents-plugin-wsflow/rsrc/lead-delegate/lead-delegate.md`
and regenerate the affected manifest(s) through the wsflow regen path rather than
hand-editing, per wsflow-mirroring.md. Apply the skill-authoring invariant
checklist to every changed line. Verify the ws/wsflow rsrc mirror is
byte-identical and the skill/mirror drift tests pass, and that the rendered
`lead-delegate` reads as one added tier-resolution sentence with the rest of
`## Assignment` intact.

### Result (90f71fa6) - 2026-09-11

Landed in `agents-plugin/rsrc/lead-delegate/lead-delegate.md` and its
byte-identical wsflow mirror:

- Intro line reconciled — dropped "model" from the freehand enumeration
  ("choose the executor's prompt, tools, and permissions"), so it no longer
  implies a freehand model pick (D1).
- `## Assignment` gained one sentence: "Resolve the executor's model by judging
  the assignment's difficulty tier (small / medium / large / xlarge) and passing
  it to `{{.McpNamespace}}/config.resolve_agent(tier)`, rather than naming a
  model directly." No scoring rubric; inline tool call, not a render step
  (D1, D2, D4).
- Prompt / tools / permissions freehand guidance left unchanged (D3).
- Used the `{{.McpNamespace}}` template token (renders `ws`/`wsflow` at render
  time) rather than a literal `ws/`, keeping the shared rsrc source
  byte-identical and satisfying the wsflow forbidden-reference guard;
  `config.resolve_agent` is a canonical ws MCP tool, staying within the
  shipped-surface boundary (D5).

Mirror and manifest regenerated via the documented wsflow regen path
(`WSRSRC_REGEN` + `WS_REGEN_WSFLOW_RSRC`), not hand-edited.

Verification:
- `diff` ws vs wsflow rsrc copy: IDENTICAL.
- `go test ./internal/wsrsrc -run TestWsflowRsrcMirrorUpToDate|TestGenerateRealManifest`: ok.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: OK (10 tests).
- Independent review (single, `reviewer`): clean — no Critical/Important/Minor
  findings.

Decisions: none beyond the ticket; the `{{.McpNamespace}}` token choice is the
established sibling-playbook convention, not a deviation.

## Release gate

This ticket is the release gate for the current cycle: the `epic/refound ->
develop` merge and the subsequent release proceed once it lands (user decision,
2026-09-11). It is the last planned change before the epic's work is shipped.
