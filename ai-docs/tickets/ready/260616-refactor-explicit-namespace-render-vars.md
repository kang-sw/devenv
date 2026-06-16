---
title: explicit namespace render vars for wsflow playbooks
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260616-refactor-wsflow-product-mode-convergence: Phase 1.5 stabilization before prompt.render absorption
spec:
  - 260513-wsflow-agentless-runtime-mode
  - 260609-playbook-tools
  - 260609-playbook-harness-rendering
  - 260609-rsrc-playbook-distribution
related-mental-model:
  - prompt-bundle
  - workflow-skills
  - mcp-runtime
---

# explicit namespace render vars for wsflow playbooks

## Background

Phase 1 of `260616-refactor-wsflow-product-mode-convergence` made
`playbook.print` and `playbook.render` safe enough for wsflow product-mode
dogfood by applying token-safe `ws` -> `wsflow` substitution at render time and
filtering full-ws-only playbook sections with explicit product markers.

That is acceptable as a stabilization hotfix, but it leaves an avoidable
authoring risk: shared rsrc text does not explicitly mark which `ws` references
are user-facing namespace notation and which references are literal MCP tool
identifiers or implementation names. Future prose can therefore fail in
surprising places even if the current token-safe substitution tests cover known
cases.

The safer long-term contract is explicit namespace templating. Shared rsrc text
should use reserved render variables for display namespace values, while actual
tool identifiers stay literal or move to separate semantic variables.

## Decisions

- Introduce reserved namespace render vars for playbook text, with at least:
  `McpNamespace` for `ws/<tool>` notation and `SkillNamespace` for
  `ws:<skill>` notation.
- Prefer a small reserved-var allowlist that does not require every playbook
  frontmatter block to declare these common product variables.
- Runtime-injected reserved namespace vars must win over caller-supplied
  `context` keys.
- Keep actual MCP tool identifiers distinct from display namespace text.
  Example: `ws.lead.login` remains a literal actual tool name unless a dedicated
  semantic var is introduced for that exact tool.
- Keep product-mode full-only/wsflow-only markers from Phase 1; this ticket
  changes namespace rendering, not product-mode section selection.
- Remove or sharply narrow render-time generic namespace substitution after the
  rsrc corpus has moved to explicit variables. Any remaining substitution must
  be treated as a migration guard, not the primary authoring contract.

## Phases

### Phase 1: replace implicit namespace substitution with reserved render vars

Add reserved namespace render vars to the playbook rendering path and update
shared rsrc playbooks to use them for user-facing `ws/` and `ws:` notation.
Actual MCP tool identifiers such as `ws.lead.login` and full-ws-only tool names
must stay explicit unless a dedicated semantic variable is intentionally added.

The implementation should keep Phase 1 product markers working, preserve
wsflow's byte-identical rsrc mirror policy, and regenerate manifests/mirrors
after rsrc edits.

Verification:

- wsflow `playbook.print` and `playbook.render` output contains `wsflow/` and
  `wsflow:` notation from explicit vars, not broad string substitution.
- full ws output still contains `ws/` and `ws:` notation.
- literal actual tool identifiers that should remain `ws.*` are not rewritten.
- caller `context` cannot override reserved namespace vars.
- tests fail if new shared rsrc text relies on broad `ws` string substitution
  for namespace output.
