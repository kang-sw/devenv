---
title: wsflow product-mode convergence — remove curated skill bodies after M4
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260616-refactor-remove-agent-backed-api-tools: prerequisite — remove the agent-backed api.ask surface before product-mode rendering cleanup
  260616-bug-wsflow-playbook-tools-expose-full-ws-guidance: absorbed narrow dogfood symptom; the real issue is missing wsflow product-mode convergence
  260616-refactor-explicit-namespace-render-vars: Phase 1.5 stabilization — replace broad namespace substitution with explicit render vars
spec:
  - 260513-wsflow-agentless-runtime-mode
  - 260609-playbook-tools
  - 260609-playbook-harness-rendering
  - 260609-rsrc-playbook-distribution
  - 260513-wsflow-agentless-skill-surface
  - 260529-wsflow-converged-implement-spine
spec-remove:
  - 260529-prompt-render-tool
  - 260529-wsflow-only-tool-surface
related-mental-model:
  - prompt-bundle
  - workflow-skills
  - mcp-runtime
---

# wsflow product-mode convergence — remove curated skill bodies after M4

## Background

The 260605 pivot direction says ws and wsflow should converge onto one workflow
text source: wsflow's long-term distinction from full ws is product-mode
capability, not a separately maintained skill corpus. Earlier implementation
captured only the generated `rsrc/` mirror and preserved curated
`agents-plugin-wsflow/skills/lead-*` bodies, which was a useful intermediate but
not the desired endpoint.

Final target:

- Full ws and wsflow share the same playbook/resource text.
- Product-mode rendering supplies the user-facing namespace: `ws` vs `wsflow`.
- Full ws exposes mercenary subprocess-agent and exec surfaces; wsflow does not.
- wsflow has no separately curated workflow procedure bodies.
- `prompt.render` is removed after its allowlist/context behavior is absorbed
  into product-mode-aware `playbook.render`.

Timing: do this after M4 removes the agent-backed `api.ask` surface. Until then,
treat wsflow as not usable for serious dogfood because the current surface can
expose full ws guidance through `playbook.print` / `playbook.render` while
wsflow skills still carry a parallel curated procedure corpus.

## Decisions

- **Order:** M4 api tool deletion first; wsflow convergence last. Do not spend
  effort preserving the current wsflow curated-skill surface during M4 except to
  avoid breaking source-tree tests unintentionally.
- **Distinction rule:** ws and wsflow differ only by namespace and capability
  gates: mercenary/external-agent calls and exec permissions exist in full ws
  only; wsflow hides or rejects them.
- **Shared source rule:** workflow procedure text belongs in the rsrc playbook
  tree. wsflow-specific divergence is product-mode rendering/filtering, not
  hand-maintained `SKILL.md` bodies.
- **`prompt.render` retirement:** the wsflow-only `prompt.render` tool is a
  migration artifact. Its render-eligible allowlist, namespace substitution, and
  context-injection semantics move into `playbook.render`'s wsflow product-mode
  branch before the tool is removed.
- **Current wsflow status:** wsflow is considered temporarily not usable until
  this convergence lands.
- **Namespace rendering hardening:** Phase 1's token-safe namespace substitution
  is a stabilization bridge, not the desired long-term authoring contract.
  Before absorbing `prompt.render`, shared rsrc text should move display
  namespace output to explicit reserved render vars so literal MCP tool names
  and prose are not vulnerable to broad string substitution.

## Phases

### Phase 1: product-mode-aware playbook rendering

Teach `playbook.print` and `playbook.render` to render safely in wsflow product
mode. Apply `ws` -> `wsflow` namespace substitution, suppress or filter
mercenary/exec/full-ws-only guidance, and enforce the wsflow render allowlist
where needed. Verification: wsflow-mode playbook output for representative lead
and delegate playbooks contains wsflow notation and no hidden full-ws tools.

### Result (f6e2dc20) - 2026-06-16

Implemented product-mode-aware rendering for `playbook.print` and
`playbook.render`. Shared rsrc playbooks now use explicit full-only and
wsflow-only marker blocks; the renderer strips markers, selects the appropriate
blocks for full ws vs wsflow/no-agent mode, and applies token-safe namespace
substitution without corrupting words that merely contain `ws`.

Representative shipped playbooks were updated to hide mercenary/exec/full-ws-only
guidance from wsflow output while keeping native subagent alternatives where a
workflow still needs delegation. The canonical rsrc manifest and byte-identical
wsflow rsrc mirror were regenerated.

Verification added or extended:

- wsflow `playbook.print` output for `lead-workflow-manual` contains `wsflow`
  notation and no marker comments, `ws.mercenary.*`, `exec.*`, or full-ws-only
  wording.
- wsflow `playbook.render` output for a real shipped delegate (`implementer`)
  writes a prompt file without hidden full-ws guidance or marker comments.
- namespace substitution preserves words such as `shows`, `knows`, `follows`,
  `rows:`, `news/`, and `workflows`.
- package tests cover the new raw rsrc marker structure and the regenerated
  wsflow mirror.

### Phase 1.5: explicit namespace template variables

Implement `260616-refactor-explicit-namespace-render-vars` before Phase 2. Move
user-facing namespace output in shared rsrc playbooks from implicit render-time
string substitution to explicit reserved render vars such as `McpNamespace` and
`SkillNamespace`. Keep actual MCP tool identifiers, such as `ws.lead.login`,
distinct from display namespace notation.

Verification: wsflow and full-ws playbook outputs use the correct namespace
notation from reserved vars, caller context cannot override the reserved vars,
and tests fail if shared rsrc playbooks depend on broad `ws` string substitution
for namespace output.

### Result (ae0c6959) - 2026-06-16

Completed the Phase 1.5 hardening through
`260616-refactor-explicit-namespace-render-vars`. Playbook product-mode
namespace output now comes from explicit implicit rsrc variables
(`McpNamespace`, `SkillNamespace`) rather than broad playbook string
substitution. Actual MCP tool identifiers remain literal, product markers remain
the section-selection mechanism, and the canonical rsrc manifest plus wsflow
mirror were regenerated.

### Phase 2: absorb prompt.render behavior into playbook.render

Move the current `prompt.render` stem allowlist, context injection, and
namespace substitution behavior into the wsflow branch of `playbook.render`.
Preserve the existing five renderable delegate stems unless M4 changes the
delegate set. Verification: existing prompt.render use cases can be expressed
through playbook.render and produce equivalent prompt files.

### Result (6ca530ab) - 2026-06-17

Implemented the Phase 2 compatibility bridge. In wsflow/no-agent mode,
`playbook.render` now accepts the existing five legacy `prompt.render` stems
(`reference-discovery`, `plan-populator-survey`, `plan-populator-research`,
`code-reviewer`, and `mental-model-updater`) and appends caller `context` as the
same sorted free-text `## Render Context` block used by `prompt.render`.

The bridge is intentionally narrow: full ws `playbook.render` and wsflow
non-legacy playbooks still treat `context` as declared template variables and
reject undeclared keys. `prompt.render` remains callable and wsflow-only for the
later removal phase; no wsflow skill bodies or rsrc files were changed.

Verification added or extended:

- wsflow `playbook.render` materializes context for a free-response legacy stem
  (`code-reviewer`) and a file-writing legacy stem (`plan-populator-survey`).
- full ws `playbook.render` still rejects undeclared context for a legacy stem.
- wsflow `playbook.render` still rejects undeclared context for a non-legacy
  stem (`implementer`).
- existing `prompt.render` tests continue to cover advertisement, allowlist, and
  retained context behavior.

### Phase 3: collapse wsflow skills to thin shims

Replace curated `agents-plugin-wsflow/skills/lead-*` procedure bodies with thin
entry shims that call the product-mode playbook surface. Update wsflow skill
tests from curated-body assertions to shim and rendered-output assertions.
Verification: wsflow skill inventory remains correct, but procedure behavior is
loaded from shared rsrc playbooks.

### Result (87aec145) - 2026-06-17

Collapsed all shipped `agents-plugin-wsflow/skills/lead-*` `SKILL.md` files to
thin entry shims. The shims preserve package-local bare `name: lead-*`
frontmatter, call `wsflow/playbook.print(name: "<lead-name>")`, execute the
returned procedure against the current request, and report a blocker if the
playbook cannot be loaded.

Updated wsflow package tests so curated-body assertions are replaced by thin
shim shape and shared-playbook coverage checks. Inventory, forbidden full-ws
reference, runtime-contract, and bootstrap-template lineage checks remain
active. The wsflow rsrc mirror, runtime metadata, skill inventory, and
`prompt.render` surface were not changed; `prompt.render` removal remains Phase
4.

Dogfood notes: the root `AGENTS.md` named-agent-first delegation rule was removed
in a separate user-requested commit (`c430c1ee`) after it caused optional audits
to route through mercenary instead of native subagents. A follow-up idea ticket
(`260617-feat-fresh-reader-audit-playbook`) captures the missing reusable
fresh-reader audit playbook surfaced during the skill-authoring audit.

### Phase 4: remove prompt.render and stale migration doctrine

Remove the wsflow-only `prompt.render` MCP/runtime surface and update specs,
mental models, package runtime contracts, and wsflow mirroring docs. Drop the
remaining migration doctrine that treats legacy prompt rendering as the normal
wsflow delegate path. Verification: full ws and wsflow package tests pass,
wsflow runtime capabilities omit `prompt.render`, and wsflow skills remain thin
playbook shims.
