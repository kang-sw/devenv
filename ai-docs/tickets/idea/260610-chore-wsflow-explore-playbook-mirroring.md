---
title: wsflow parity for the explore playbook + native-Explore delegation
related:
  260609-refactor-ws-skill-text-playbook-conversion: source change — M2 shifted full-ws skill delegation to the explore playbook
  260605-epic-ws-playbook-factory-pivot: parent direction — wsflow convergence is deferred epic non-scope
---

# wsflow parity for the explore playbook + native-Explore delegation

## Background

M2 Phase 1 (`260609-refactor-ws-skill-text-playbook-conversion`) added the
`explore` render playbook and shifted every `ws/subquery` delegation call site in
the full-ws `agents-plugin/skills/` to the native-Explore pattern. Per
`ai-docs/ref/wsflow-mirroring.md`, editing mirrored full-ws skills requires either
a same-change wsflow update or a follow-up ticket. This is that follow-up.

No same-change wsflow edit was made because:

- wsflow distributed skill text already forbids `ws/`, `ws:`, `subquery`, and
  `agents.*` and already uses native subagents plus `wsflow/prompt.render`, so the
  full-ws subquery→Explore shift caused no forbidden-reference breakage in wsflow.
- The `explore` playbook lives in the full-ws rsrc tree consumed by
  `ws/playbook.render`; wsflow uses the separate `prompt.render` embedded-prompt
  mechanism with a five-stem allowlist. Giving wsflow explore-playbook parity is
  rsrc-playbook convergence, which epic `260605` explicitly defers as non-scope.

## Scope to decide

- Whether wsflow should gain an Explore delegate prompt (added to the
  `prompt.render` render-eligible allowlist) or its own explore playbook, or keep
  describing scoped exploration generically by task scope (current wsflow style).
- Verify the wsflow `test_workflow_manual_documents_subagent_guidance` assertion
  (in `agents-plugin-wsflow/tests`) still matches the wsflow `lead-workflow-manual`
  copy after any change here; the full-ws M2 Phase 1 did not touch it, so it is not
  currently broken, but post-M2/M3 convergence should re-check it.
- Coordinate with M2 Phase 2 (internal skill-body → playbook migration) and the
  broader wsflow convergence deferral before promoting.

## Notes

- This is a deferred convergence item, not a blocker for M2/M3 full-ws work.
- Triage decision (keep as wsflow-style generic guidance vs add an Explore
  delegate) belongs in discussion before promotion.
