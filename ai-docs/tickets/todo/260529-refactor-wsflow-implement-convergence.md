---
title: Converge wsflow lead-implement onto the unified spine with a wsflow-only prompt dispatch tool
related:
  260521-refactor-wsflow-lead-implement-mirroring-gap: ends its deferred "for now" lead-edit divergence resolution
related-mental-model:
  - workflow-skills
---

# Converge wsflow lead-implement onto the unified spine with a wsflow-only prompt dispatch tool

## Background

The full ws lead skill cascade (`260520-refactor-lead-skill-cascade`, commit
`ad9475c9`) unified `ws:lead-edit` + `ws:lead-write-code` into a single
`ws:lead-implement` spine (Route → Verdict → Prep → Edit → Review → Doc → Final
→ Merge) with parameterized judges (`needs-delegation`, `plan-depth`,
`review-allocation`). wsflow did not follow: it still ships `wsflow:lead-edit`
and has `wsflow:lead-implement` delegate execution to it.

`260521-refactor-wsflow-lead-implement-mirroring-gap` (now `.done`) chose to
document this as an intentional divergence **"for now"** (commit `e15b0451`),
explicitly deferring the convergence decision. This ticket ends that deferral.

A second, related gap surfaced while scoping this: wsflow embeds the full ws
prompt bundle (identical `content_sha256` in `runtime.json`, including
`project-survey`, `plan-populator-*`, `code-reviewer`, `mental-model-updater`,
`implementer`) but exposes **no dispatch tool** (`agents.*`, `subquery`,
`exec.*` are all hidden in agentless mode). The prompts are dead weight: useful
delegate prompts exist but there is no host-neutral way to feed them to a native
subagent without the lead hand-pasting large playbook text. This ticket adds a
read-only `prompt.render` tool to close that gap.

## Decisions

Settled in design discussion (`lead-discuss`):

- **L1 — Converge the spine.** wsflow `lead-implement` adopts the unified spine
  shape and absorbs `wsflow:lead-edit` (delete the separate skill). The spine
  stays a lead-owned harness; convergence is structural and does not violate the
  wsflow "native-only, no agent runtime logic" motto.
- **L2 — No implementer named stage.** The Edit stage is lead direct edits plus
  lead-discretion scoped native subagent work (the capability `lead-edit`
  already had). The `implementer` prompt stays **unexposed** in wsflow.
  Rationale: in a one-shot (no continued-agent) host, the implementer loses its
  multi-turn fix-relay value; only "first implementation in a sub-context"
  remains, and for small/medium phases that costs more lead context than it
  saves (lead must reload another agent's code to apply review fixes). Larger
  delegation is still available at lead discretion via scoped native subagents.
- **D1 — Render-time namespace substitution.** Keep one shared prompt bundle;
  `content_sha256` stays identical across ws and wsflow. The `ws/` → `wsflow/`
  rewrite happens at render time, driven by the package namespace env
  (`WS_MCP_NAMESPACE`). The exposed prompts are namespace-only references
  (verified: `project-survey`, `plan-populator-*`, `code-reviewer`,
  `mental-model-updater` contain only `ws/<tool>` references, no
  feature-specific `agents.*`/`subquery` calls), so substitution is sufficient.
- **D2 — New `prompt.render` MCP tool.** `prompt.render(stem, context) ->
  { prompt_path }` writes a context-injected, namespace-substituted copy of the
  prompt to a tmp file and returns its path. The lead hands `prompt_path` to a
  native subagent. The tool is read-only with no decision logic — the lead picks
  the stem. It **must not** mint an `expected_output_path`.
- **D2b — wsflow-only visibility (mirror gate).** `prompt.render` is hidden from
  ws, implemented as the mirror image of the existing single-direction agentless
  gate. Today `NoAgentMode() && noAgentHiddenTool(name)` hides agent tools in
  wsflow; add the reverse (`!NoAgentMode() && agentfulHiddenTool(name)`, or an
  equivalently named predicate) at the same three gate points in
  `agents-plugin-tool/internal/mcp/server.go`: `callTool` (~341), `tools/list`
  advertise (~2915), and `toolAllowed` (~2983). Tool implementation is shared;
  only visibility is gated by namespace/agentless.
- **D3 — Exposed prompt set.** wsflow exposes exactly five prompts through
  render: `project-survey`, `plan-populator-survey`, `plan-populator-research`,
  `code-reviewer`, `mental-model-updater`. `implementer` is excluded (per L2).
- **Result channel.** Rely on the native subagent's free-text response; do not
  assume the subagent has write permission in the generic case. File-writing
  prompts (`plan-populator-*`, `mental-model-updater`) keep their **existing
  output-path contract**: the caller passes an output path (created via
  `path.generate`) as render `context`, and render injects it into the prompt
  body — these are recognized as a distinct "writes to a specific file" prompt
  class, not forced into a free-response shape. `project-survey` and
  `code-reviewer` are free-response (text returned to the lead). `prompt.render`
  does not synthesize or require an `expected_output_path`.

### Rejected alternatives

- **Mirror ws verbatim (keep implementer + multi-cycle review relay in
  wsflow).** Rejected: requires continued-agent semantics wsflow deliberately
  omits; one-shot re-spawn with full accumulated context is token-heavy and
  fragile.
- **Per-package prompt bundles / `{{ns}}` placeholders baked into stored
  prompts.** Rejected: breaks the `runtime_capabilities.match: exact` shared-sha
  contract and reintroduces drift; render-time substitution keeps a single
  source.
- **`prompt.render` mints `expected_output_path` and prompts instruct the
  subagent to write there.** Rejected: native subagents may lack write
  permission; free-text response is always available and the genuine
  file-writing prompts already carry their own caller-supplied output path.
- **Extend `infra.read` instead of a new tool.** Rejected: `infra.read` is
  static-resource reading; context injection would pollute its semantics. A
  dedicated tool is clearer.
- **Keep the documented `lead-edit` divergence permanently.** Rejected: the
  unified spine is a host-neutral structural improvement and the divergence was
  only ever deferred, not endorsed.

## Constraints

- Keep wsflow distributed skill/tool text non-ws-aware: users see `wsflow:`,
  `wsflow/`, and wsflow skill invocations only.
- Preserve `runtime_capabilities.match: exact` and the shared prompt-bundle
  `content_sha256` across ws and wsflow.
- Do not expose `prompt.render` in ws; do not expose `agents.*`/`subquery`/
  `exec.*` in wsflow. The two gates are mirror images and must stay symmetric.
- ws `lead-implement` and the ws named-agent path are unchanged by this ticket.

## Phases

### Phase 1: Converge wsflow implement and add wsflow-only prompt.render

Single reviewable slice delivering the converged wsflow implementation path and
its supporting dispatch tool together, because the converged
`wsflow:lead-implement` Edit/Doc stages depend on `prompt.render` existing.

Work:

1. **Tool + gating** (`agents-plugin-tool/`):
   - Implement `prompt.render(stem, context) -> { prompt_path }`: load the
     bundled prompt by stem, apply render-time `ws/` → `wsflow/` namespace
     substitution driven by `WS_MCP_NAMESPACE`, inject `context` values
     (including a caller-supplied output path when present) into the body, write
     to a tmp file, and return its path. No `expected_output_path`.
   - Add the mirror gate predicate (`agentfulHiddenTool` / `wsflowOnlyTool`) and
     apply `!NoAgentMode() && <predicate>(name)` at `callTool`, `tools/list`
     advertise, and `toolAllowed` so `prompt.render` is hidden from ws and shown
     in wsflow. Keep it symmetric with the existing `noAgentHiddenTool` gate.
2. **Runtime contract** (`agents-plugin-wsflow/runtime.json`): add
   `prompt.render` to `tools` (and the command mirror if applicable).
3. **Skill convergence** (`agents-plugin-wsflow/skills/`):
   - Rewrite `lead-implement/SKILL.md` to the unified spine shape; Edit stage is
     lead direct edits + lead-discretion scoped native subagent (no implementer
     named stage); wire render-mediated dispatch for the five exposed prompts at
     the survey/plan/review/doc stages.
   - Delete `lead-edit/SKILL.md` (absorbed); update any wsflow skill text that
     referenced `lead-edit` ownership.
4. **Mirroring + docs**:
   - `ai-docs/ref/wsflow-mirroring.md`: remove the documented `lead-edit`-only
     exception; relax the `mental-model-updater` forbidden-token entry now that
     render-time substitution sanitizes vocabulary; document the `prompt.render`
     dispatch pattern and the wsflow-only tool gate.
   - `ai-docs/spec/mcp-tools.md`: add the `prompt.render` contract and its
     wsflow-only visibility.
   - `ai-docs/spec/workflow-skills.md`: update the wsflow `lead-implement`
     contract to the converged shape.
   - Update the `workflow-skills` mental model for the converged shape.
5. **Tests** (`agents-plugin-wsflow/tests/`): drop the `lead-edit` divergence
   exception, update skill-inventory expectations, and add `prompt.render`
   runtime-contract coverage.

Constraints carried from Decisions above apply to every step.

Verification (end-to-end observable behavior):

- wsflow MCP advertises `prompt.render`; ws MCP does **not** (gate verified in
  both directions, including `tools/list`).
- `prompt.render` returns a tmp `prompt_path` whose body has `ws/` rewritten to
  `wsflow/` and the supplied `context` injected; no `expected_output_path` is
  minted; a caller-supplied output path (file-writing prompts) appears in the
  body.
- `python3 -m unittest discover agents-plugin-wsflow/tests` passes with the
  `lead-edit` divergence exception removed.
- wsflow distributed skill bundle test reports no forbidden full-ws references,
  no `lead-edit` in the shipped set, and no inventory drift.
