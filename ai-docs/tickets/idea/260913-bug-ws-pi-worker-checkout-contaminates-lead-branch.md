---
title: "Worker checkout can contaminate lead-owned commits"
related:
  260912-feat-ws-pi-bounded-web-access-for-explore: dogfood run that exposed the shared-worktree branch leak
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: worker lifecycle must preserve parent completion and state boundaries
---

# Worker checkout can contaminate lead-owned commits

## Background

During the Pi goal run for bounded web access, the ticket worker created and checked out its `impl/goal/...` branch in the same Git worktree used by the lead. The checkout was worktree-global, not agent-process-local. After the worker returned stop `(c)`, the lead correctly authored ticket Editions and a localized hotfix but did not first restore its goal branch, so six lead-owned commits landed after the unfinished implementation tip.

Recovery required preserving the contaminated tip, selectively transplanting only lead-owned commits to the goal branch, excluding worker implementation ancestry and review findings, and resetting the inactive impl ref to the worker's last intended checkpoint.

## Decisions

- **Treat checkout state as shared.** The shared lead-run workflow must not assume a worker's branch checkout is isolated merely because the worker process is separate.
- **Record both ends of the handoff.** Before dispatch, record `base_branch` and `base_oid` in the existing session assignment note. On every terminal worker report, retain the reported `impl_branch` and `impl_oid` in that note across compaction until normal merge or blocker disposition.
- **Restore before lead writes.** Before every lead-owned ticket edit, hotfix commit, follow-up dispatch, or merge decision, restore `base_branch` and verify that `HEAD` is `base_oid`; do not rely only on one post-worker restoration.
- **Fail closed on unsafe restoration.** Stop with diagnostics on tracked-worktree or index changes, unmerged paths, an active sequencer, branch/ref mismatch, or base/impl ref drift. Preserve unrelated untracked files unless they prevent checkout.
- **Keep the impl ref intact.** Restoration must never reset, delete, or merge `impl_branch`; it preserves `impl_oid` exactly and does not import implementation content.
- **Use the shared workflow source.** Author this lead-run and worker-handoff contract on `develop` in the shared workflow surface, not in Pi adapter runtime enforcement. Regenerate required shipped mirrors; do not allocate a worktree implicitly.
- **No implicit worktree allocation.** Do not solve this by silently creating temporary worktrees; this repository requires explicit approval for worktree creation and cleanup. A future isolated-worktree design may be considered separately.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ticket-worker/ticket-worker.md, and their required shipped mirrors |
| scope.surface | public-interface | lead-run and ticket-worker are shared shipped workflow interfaces |
| scope.new_public_symbol | no | no new tool or exported symbol is required |
| scope.new_type_contract | yes | session assignment notes and terminal worker reports gain base_branch, base_oid, impl_branch, and impl_oid handoff fields |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go and internal/wsrsrc tests cover shared playbook rendering and mirror integrity |
| complexity.reuse_points | confirmed | existing lead-run session.note assignment record and ticket-worker terminal report contract |
| complexity.side_effect_risk | high | restoration or refusal can halt lead mutations and must preserve the implementation ref |
| risk.correctness | high | a wrong checkout can attach lead commits to unfinished implementation ancestry |
| risk.fit | high | the shared contract must preserve host-neutral lead-run behavior across shipped harnesses |
| risk.test | high | terminal reports, compaction recovery, dirty state, ref drift, and commit ancestry require regression coverage |
| risk.security_or_contract | high | branch integrity and fail-closed mutation refusal are workflow safety contracts |

## Phases

### Phase 1: Add shared branch restoration and mutation guards to lead-run

On `develop`, update the shared lead-run and ticket-worker handoff contract, then regenerate its required shipped mirrors. Before dispatch, write `base_branch` and `base_oid` to the existing session assignment note. On each terminal worker report, retain `impl_branch` and `impl_oid`; before any lead-owned mutation, restore `base_branch` and verify `HEAD` remains `base_oid`. Fail closed on tracked-worktree/index changes, unmerged paths, active sequencer state, branch/ref mismatch, or base/impl ref drift; preserve unrelated untracked files unless checkout cannot proceed. Never reset, delete, or merge the implementation ref during restoration. Do not add Pi-local runtime enforcement.

Verify stop `(b)` through `(e)`, successful phase/ticket completion, worker-created impl checkout, clean base restoration, tracked/index/unmerged/sequencer refusal, changed ref/tip refusal, unrelated untracked-file preservation, assignment-note recovery after compaction, no implementation ancestry in lead-only commits, and unchanged normal `ws/git.merge` ownership. Include a regression reproducing the 2026-09-13 contamination shape: worker commits followed by lead ticket/hotfix commits must leave the latter only on the restored goal branch.
