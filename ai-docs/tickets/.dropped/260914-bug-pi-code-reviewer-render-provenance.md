---
title: "Bug: Pi rejects rendered code-reviewer playbooks for missing trusted provenance"
dropped: 2026-09-14
---

# Bug: Pi rejects rendered code-reviewer playbooks for missing trusted provenance

## Background

On a delegated Pi adapter call, `ws/playbook.render(name: "code-reviewer")`
is rejected before the MCP render with `delegated playbook lacks trusted shipped
provenance` because `playbookProfile` only looks for
`code-reviewer/code-reviewer.md`, while the bundled manifest declares
`code-reviewer.md` (`agents-plugin-pi/src/bridge.ts#L812-L823`,
`agents-plugin-pi/src/delegation-policy.ts#L119-L124`,
`agents-plugin-pi/rsrc/manifest.json`). The ticket-worker workflow renders the
reviewer playbook(s) its route verdict supplies, not `code-reviewer` by name,
so whether this rejection blocks independent review depends on that unresolved
route allocation (`agents-plugin-pi/rsrc/ticket-worker/ticket-worker.md#L71-L74`).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/delegation-policy.ts and agents-plugin-pi/test/reviewer-artifact.integration.test.ts |
| scope.surface | public-interface | delegated ws/playbook.render admission changes at agents-plugin-pi/src/bridge.ts#L812-L823 |
| scope.new_public_symbol | no | no new symbol is named |
| scope.new_type_contract | no | no type or signature change is named |
| scope.test_surface | existing | agents-plugin-pi/test/reviewer-artifact.integration.test.ts#L61-L87 covers reviewer profile and artifact admission |
| complexity.reuse_points | confirmed | agents-plugin-pi/src/skills-dir.ts#L71-L74 already recognizes both nested and flat manifest paths |
| complexity.side_effect_risk | high | delegated render admission also protects reviewer artifact scope enforcement |
| risk.correctness | high | a bad profile classification can block required review or misclassify a child |
| risk.fit | high | code-reviewer.md is a shared include, while reviewer and code-review-* are render playbooks |
| risk.test | high | regression coverage must preserve both accepted reviewer renders and rejected arbitrary paths |
| risk.security_or_contract | high | the fix must retain trusted provenance and arbitrary-prompt rejection |

## Phases

### Phase 1: Restore bundled reviewer rendering on Pi

Trace the provenance check and make the installed/bundled code-reviewer
playbook renderable without weakening reviewer artifact scope enforcement or
accepting arbitrary prompt paths. Add regression coverage for the successful
render and the preserved rejection boundary.


## Resolution (2026-09-14)

The failure was correct admission behavior: `code-reviewer` is an include, not a standalone delegated render target. Route-selected reviewer wrappers provide the supported paths; the missing label-to-wrapper mapping belongs to ws-mcp on develop.
