---
title: "Restore trusted delegated-playbook rendering for Pi workers"
related:
  260915-bug-ws-pi-widget-context-value-removed: reproduction during required review
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a0d26f92ac62e916
sage-review-completeness-reviewed: a0d26f92ac62e916
---

# Restore trusted delegated-playbook rendering for Pi workers

## Background

A Pi ticket worker in an acquired implementation worktree implemented its phase but could not start the mandatory independent review. For both allocated partitions it attempted `playbook.render(name: "code-reviewer")`; the route failed with `ws-pi-agent: delegated playbook lacks trusted shipped provenance`. Reusing an earlier cached rendered prompt was correctly rejected by reviewer admission with `ws-pi-agent: nested spawn requires trusted render provenance`.

The failure is not a worktree identity handoff. `code-reviewer.md` is a flat shared include with no render-role frontmatter, while Pi's delegated profile classifier accepts structured `<name>/<name>.md` render playbooks. `reviewer/reviewer.md` is the full-scope wrapper, and `code-review-correctness/code-review-correctness.md`, `code-review-fit/code-review-fit.md`, and `code-review-test/code-review-test.md` are the partition wrappers; each carries reviewer metadata and includes the shared contract. The worker procedure forwards a generic `<reviewer>` allocation, while the route todo names only full-scope `reviewer`, not the partition-label-to-wrapper mapping (agents-plugin/rsrc/ticket-worker/ticket-worker.md#L71-L74; agents-plugin-tool/internal/mcp/session_state.go#L499-L504).

Review also exposed a separate trust gap on the accepted path: nested admission verifies the registered prompt digest, then asynchronous model resolution runs, and `spawnAgent` rereads the original path before copying it into the child home. A mutation in that interval can make the executed bytes differ from the verified bytes.

Together these defects block required review or weaken the promise that edited prompts are rejected.

## Decisions

- For a `single` allocation, render the structured `reviewer` wrapper; for each selected correctness, fit, or test partition, render the matching structured `code-review-<partition>` wrapper. Never render the flat `code-reviewer` include directly.
- Keep `code-reviewer.md` as the shared included contract rather than adding a name-specific provenance exception or treating metadata-less flat includes as independently delegated roles.
- Nested admission must capture or produce a verified prompt snapshot, and the later child-home copy must consume those same verified bytes instead of rereading the caller-controlled original after asynchronous model resolution.
- The nested reviewer must retain the exact generated findings-file write grant required by the reviewer playbook.
- Restore the required review path without bypassing, weakening, or disabling nested-spawn provenance checks.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Preserve rejection of arbitrary cached, copied, edited, or otherwise untrusted prompt files.
- Preserve worker capability ceilings, recursion depth, role-specific tool profiles, and monotonic write-scope rules.
- Keep the ticket-worker variants and their ws/wsflow/Pi mirrors behaviorally aligned under the repository's shipped-surface and mirroring rules.
- Add a regression that pins full-scope review to `reviewer` and every partition to its matching `code-review-<partition>` wrapper.
- Add a deterministic mutation-between-admission-and-launch regression proving the child receives the verified snapshot, while a prompt changed before admission is still rejected.
- Retain regression coverage for nested reviewer admission with exactly one findings-file grant.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-worker*/ticket-worker*.md; agents-plugin-wsflow/rsrc/ticket-worker*/ticket-worker*.md; agents-plugin-pi/rsrc/ticket-worker*/ticket-worker*.md; agents-plugin-tool/internal/mcp/session_state.go; agents-plugin-pi/src/bridge.ts; agents-plugin-pi/src/delegation-policy.ts; agents-plugin-pi/src/spawner.ts |
| scope.surface | cross-module | the route allocation, shipped worker wrappers and mirrors, and Pi nested-spawn boundary must agree; no new MCP tool is requested |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | yes | RenderProvenance must carry the verified prompt bytes from nested admission to child-home creation |
| scope.test_surface | existing | agents-plugin-pi/test/reviewer-artifact.integration.test.ts#L61-L87 covers structured reviewer admission and the exact-file grant; agents-plugin-pi/test/spawner.test.ts exercises spawn dispatch |
| complexity.reuse_points | confirmed | RenderRegistry already records manifest-verified render provenance in agents-plugin-pi/src/bridge.ts#L867-L872 and supplies it to nested admission at agents-plugin-pi/src/spawner.ts#L3566 |
| complexity.side_effect_risk | high | prompt bytes selected before asynchronous model resolution must remain the bytes passed by --append-system-prompt |
| risk.correctness | high | an incorrect wrapper blocks required partitioned review, and a verify/reread gap executes bytes that admission did not verify |
| risk.fit | high | the allocation labels must map to the existing structured wrappers while preserving the shared include and all three rsrc mirrors |
| risk.test | high | wrapper routing and both before-admission rejection and after-admission mutation require discriminating regression coverage |
| risk.security_or_contract | high | manifest verification, render provenance, and the immutable prompt snapshot form the nested-spawn trust boundary |

## Phases

### Phase 1: Route workers through structured reviewer wrappers and close the verified-prompt race

Make every ticket-worker variant and its review todo route `single` to the structured `reviewer` wrapper and each correctness, fit, or test partition to its structured `code-review-<partition>` wrapper; regenerate the required wsflow and Pi mirrors, and pin the rendered procedure. In Pi nested spawn, capture immutable verified prompt bytes at admission and use those bytes for child-home creation after asynchronous model resolution, never rereading the caller-controlled rendered path. Add discriminating tests for the wrapper route, accepted reviewer provenance with its exact findings-file grant, rejection before admission, and mutation after admission. Run the focused playbook/MCP and Pi adapter tests plus their full suites.
