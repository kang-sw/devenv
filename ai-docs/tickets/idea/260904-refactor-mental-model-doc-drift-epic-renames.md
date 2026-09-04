---
title: "Sweep mental-model prose for the epic's MCP tool-surface renames"
related:
  - 260903-epic-mcp-tool-surface-affordance-reduction
  - 260903-refactor-mcp-verb-vocabulary-unification
  - 260904-refactor-enter-affordance-rename-route-opaque
---

## Background

The `260903-epic-mcp-tool-surface-affordance-reduction` layers renamed MCP tool
names in-package under a one-shot hard-cut invariant, but each layer's plan
deferred `ai-docs/mental-model/**` prose out of its surgical acceptance scope
(the grep-sweep boundary covers code, `runtime.json`, specs, and playbooks
only). That deferral was never captured as a ticket, so the drift accumulated
silently across layers.

As of layer ① Phase 1 landing, the mental-model tree still names removed tools:

- **Layer ④ (verb unification)** left `find`/`list`/`status` → `query` drift in
  `documentation-system.md`, `mcp-runtime.md`, `workflow-skills.md`,
  `prompt-bundle.md`.
- **Layer ① (`enter.* → route.resolve_*`)** left `enter.implement`/
  `enter.proceed`/`ws.enter.*` references in `mcp-runtime.md` (lines ~52, 54,
  188) and `workflow-skills.md` (lines ~41, 49, 74, 81, 89, 132).

These are not trivial token swaps: the mental-model prose is dense, behaviorally
descriptive, and interweaves both rename families, so a blind `sed` risks
corrupting load-bearing invariant text or anchor references.

## Open Questions

- Is a single careful sweep across both rename families correct, or should each
  file be re-derived from the current spec rather than patched token-by-token?
- Do any of these mental-model sentences encode a distinction that the rename
  changed in meaning (not just in name), needing a real rewrite rather than a
  swap?
- Should the epic adopt a standing rule that each layer's plan includes the
  mental-model sweep in acceptance, closing this deferral class at the source?

## Notes

- Sequencing: safe to run after all epic rename layers land, or incrementally
  per layer. Layer ① Phase 2 (schema hollowing) may itself touch related prose;
  coordinate so this sweep does not collide with that authoring pass.
- Scope guard: prose/naming reconciliation only — do not change documented
  invariants beyond the tool-name/verb rename.
