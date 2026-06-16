---
title: wsflow playbook tools expose full ws guidance
related:
  260605-epic-ws-playbook-factory-pivot: parent pivot whose wsflow coverage check surfaced this dogfood surprise
  260609-feat-ws-playbook-surface-mvp: introduced the shared playbook.print/playbook.render tools
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: converged rsrc prompt sources and wsflow prompt.render onto the rsrc tree
  260616-refactor-wsflow-product-mode-convergence: absorbs this narrow symptom as the real post-M4 wsflow convergence work
related-mental-model:
  - prompt-bundle
  - workflow-skills
  - mcp-runtime
---

# wsflow playbook tools expose full ws guidance

## Background

Dogfood coverage check on 2026-06-16 found that wsflow no-agent mode advertises
and serves `playbook.print` / `playbook.render` from the byte-identical
`agents-plugin-wsflow/rsrc` tree. That tree intentionally stores canonical full
ws text, but only `prompt.render` applies render-time `ws/` -> `wsflow/`
namespace substitution and a wsflow render-eligible stem allowlist.

Observed behavior:

- `tools/list` in `WS_MCP_NO_AGENT=1 WS_MCP_NAMESPACE=wsflow` includes
  `playbook.print` and `playbook.render`.
- `playbook.print(name: "lead-workflow-manual")` in the same wsflow mode returns
  full ws guidance, including `ws/playbook.print`, `ws.mercenary.*`, `ws/api.ask`,
  and other tools hidden or unavailable in wsflow.
- Existing tests assert `playbook.print` and `playbook.render` are not
  wsflow-only and not hidden in no-agent mode, while `prompt.render` has the
  wsflow substitution and allowlist checks.

This may be intentional if wsflow is expected to expose full playbook tools as a
developer/debug surface, but it conflicts with the package-level rule that
distributed wsflow text presents wsflow naming and primitives to users.

## Open Questions

- Should wsflow hide `playbook.print` / `playbook.render` and keep
  `prompt.render` as the only rsrc-backed delegate rendering surface?
- If wsflow should keep playbook tools, should those tools apply namespace
  substitution and no-agent guidance filtering in wsflow mode?
- Should tests cover that wsflow-visible playbook output cannot mention hidden
  tools such as `ws.mercenary.*`, `api.ask`, `api.ask_async`, or `exec.*`?

## Dropped

Dropped on 2026-06-16 because this ticket captured only the symptom. The
confirmed issue is broader: wsflow should not maintain curated workflow skill
bodies long term, and product-mode playbook rendering should supply namespace
and capability differences after M4 api.ask. The replacement ticket is
`260616-refactor-wsflow-product-mode-convergence`.
