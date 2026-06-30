---
title: fresh-reader audit playbook for skill and prompt authoring
related:
  260605-epic-ws-playbook-factory-pivot: playbook-factory direction should make reusable delegate prompts available through rsrc
  260616-refactor-wsflow-product-mode-convergence: dogfood run exposed this gap while auditing wsflow thin skill shims
related-mental-model:
  - workflow-skills
  - prompt-bundle
sage-review: required
completed: 2026-06-30
---

# fresh-reader audit playbook for skill and prompt authoring

## Background

During wsflow Phase 3 dogfood, the `lead-skill-authoring` procedure required a
fresh-reader audit after editing skill files, but there was no dedicated bundled
playbook for that audit role. The lead manually composed a native subagent prompt
from the procedure text, including target-file isolation and finding format.

That worked, but it is a repeatability gap: every future caller must reconstruct
the same audit prompt correctly, and wsflow/full ws cannot dogfood a stable rsrc
delegate for this common authoring step.

## Desired Direction

Add a dedicated `fresh-reader-audit` render playbook or an equivalently named
skill-authoring audit delegate. It should:

- read only caller-specified target files or excerpts;
- refuse prior conversation, project docs, specs, tickets, or git history unless
  explicitly supplied as target material;
- flag awkward, surprising, context-dependent, underspecified, contradictory,
  duplicated, orphaned, or missing end-state/output wording;
- return each finding with quote, issue, severity, and suggested rewrite or
  deletion;
- support a clean result format suitable for lead classification.

This should be considered alongside the broader playbook-factory migration so
skill-authoring audits stop depending on hand-built subagent prompts.

## Decision (260629 sweep)

Build: Implement the fresh-reader-audit delegate playbook. The playbook takes a target spec or skill file and produces a structured audit from the perspective of a reader with no prior context: undefined terms, implicit assumptions, missing invariants, and drift from the described behavior. Bundle as a rsrc playbook; surface through lead-discuss on-demand. No new MCP tool needed.


## Resolution (2026-06-30)

Created fresh-reader-audit render playbook in agents-plugin/rsrc/fresh-reader-audit/. Takes TargetFiles and AuditScope inputs; produces structured findings (quote, issue type, severity, suggestion) from a zero-context reader perspective. Mirrored to agents-plugin-wsflow/rsrc/.
