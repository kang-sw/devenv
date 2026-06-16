---
title: wsflow product-mode convergence — remove curated skill bodies after M4
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260616-refactor-remove-agent-backed-api-tools: prerequisite — remove the agent-backed api.ask surface before product-mode rendering cleanup
  260616-bug-wsflow-playbook-tools-expose-full-ws-guidance: absorbed narrow dogfood symptom; the real issue is missing wsflow product-mode convergence
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

## Phases

### Phase 1: product-mode-aware playbook rendering

Teach `playbook.print` and `playbook.render` to render safely in wsflow product
mode. Apply `ws` -> `wsflow` namespace substitution, suppress or filter
mercenary/exec/full-ws-only guidance, and enforce the wsflow render allowlist
where needed. Verification: wsflow-mode playbook output for representative lead
and delegate playbooks contains wsflow notation and no hidden full-ws tools.

### Phase 2: absorb prompt.render behavior into playbook.render

Move the current `prompt.render` stem allowlist, context injection, and
namespace substitution behavior into the wsflow branch of `playbook.render`.
Preserve the existing five renderable delegate stems unless M4 changes the
delegate set. Verification: existing prompt.render use cases can be expressed
through playbook.render and produce equivalent prompt files.

### Phase 3: collapse wsflow skills to thin shims

Replace curated `agents-plugin-wsflow/skills/lead-*` procedure bodies with thin
entry shims that call the product-mode playbook surface. Update wsflow skill
tests from curated-body assertions to shim and rendered-output assertions.
Verification: wsflow skill inventory remains correct, but procedure behavior is
loaded from shared rsrc playbooks.

### Phase 4: remove prompt.render and stale curated-skill doctrine

Remove the wsflow-only `prompt.render` MCP/runtime surface and update specs,
mental models, package runtime contracts, and wsflow mirroring docs. Drop the
old doctrine that wsflow skills are curated semantic rewrites, replacing it with
the product-mode rendering contract. Verification: full ws and wsflow package
tests pass, wsflow runtime capabilities omit `prompt.render`, and no maintained
wsflow skill body duplicates shared playbook text.
