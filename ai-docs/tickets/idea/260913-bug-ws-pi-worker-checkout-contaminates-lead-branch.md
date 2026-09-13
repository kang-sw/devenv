---
title: "Worker checkout can contaminate lead-owned commits"
related:
  260909-epic-ws-worker-interpreter-refoundation: constraint — lead-run/ticket-worker are the shared worker-interpreter surface this touches
---

# Worker checkout can contaminate lead-owned commits

> Upstream report raised from a downstream project (ws Pi). The Pi-side stems
> and the goal-run reproduction below are that project's, not this repo's; the
> fix lands here in the shared lead-run/ticket-worker playbook surface. The
> originating downstream tickets were `feat-ws-pi-bounded-web-access-for-explore`
> (the dogfood run that exposed the shared-worktree branch leak) and
> `feat-ws-pi-recursive-worker-subtree-lifecycle` (worker lifecycle preserving
> parent completion and state boundaries).

## Background

During the Pi goal run for bounded web access, the ticket worker created and checked out its `impl/goal/...` branch in the same Git worktree used by the lead. The checkout was worktree-global, not agent-process-local. After the worker returned stop `(c)`, the lead correctly authored ticket Editions and a localized hotfix but did not first restore its goal branch, so six lead-owned commits landed after the unfinished implementation tip.

Recovery required preserving the contaminated tip, selectively transplanting only lead-owned commits to the goal branch, excluding worker implementation ancestry and review findings, and resetting the inactive impl ref to the worker's last intended checkpoint.

## Decisions

- **Root cause is branch-unawareness, not missing isolation.** When a worker checks out its `impl/...` branch in the shared worktree, the worktree-global `HEAD` stays on that branch after the worker returns. Contamination happens only because the lead then performs HEAD-relative writes without noticing which branch it is on — not because the checkout was shared. The fix is to remove the unawareness, not the sharing.
- **Mandate awareness; leave the rest to lead discretion.** On receiving a worker's terminal (stop-condition) report, before any lead-owned HEAD-relative write (ticket Edition/revision, hotfix commit, follow-up dispatch base), the lead must call `ws/git.status`, read `branch.head` and `impl_ticket`, and make an explicit stack-vs-return decision: stack on the impl branch when the write belongs to that impl ticket; check out the base branch when the write is unrelated or the ticket is fully blocked and the lead is exiting. Everything past that check is lead judgment; there is no fail-closed machinery.
- **The command is a forcing function, not the requirement.** Running `ws/git.status` and ignoring its result reproduces the same unawareness. The enforced obligation is *awareness plus the branch decision*, not the bare call.
- **Base branch is derivable, not recorded.** The return target is encoded in the impl branch name (`impl/<root>/<stem>` → `<root>`, the same convention `ws/git.merge` already uses), so a compacted lead reconstructs it from `ws/git.status` alone. Do not add `base_branch`/`base_oid`/`impl_oid` handoff fields to the assignment note.
- **Branch-explicit lead operations are already safe.** `ws/git.merge` names its source and target explicitly and is unaffected by the current checkout; the awareness step targets only HEAD-relative writes, not branch-explicit tools.
- **Reject worktree isolation as the main strategy.** Per-worker git worktrees would remove HEAD sharing but impose a worktree burden on downstream projects and on developers unfamiliar with worktrees, for a plugin that ships broadly. Do not adopt it as the default lead-run flow; a future isolated-worktree design may still be considered separately.
- **Use the shared workflow source.** Author this awareness contract on `develop` in the shared lead-run playbook text (and ticket-worker where it documents the handoff), not in host-local runtime enforcement. Regenerate the required shipped mirrors; do not allocate a worktree implicitly.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ticket-worker/ticket-worker.md, and their required shipped mirrors |
| scope.surface | public-interface | lead-run and ticket-worker are shared shipped workflow interfaces |
| scope.new_public_symbol | no | no new tool or exported symbol is required; the awareness step reuses the existing ws/git.status impl_ticket surface |
| scope.new_type_contract | no | the light approach records nothing new — no base_branch/base_oid/impl_oid handoff fields are added to notes or reports |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go and internal/wsrsrc tests cover shared playbook rendering and mirror integrity |
| complexity.reuse_points | confirmed | ws/git.status already surfaces branch.head + impl_ticket (state/stem/path/status); the change is playbook text over that surface |
| complexity.side_effect_risk | low | text-only playbook change; adds a read-only status check and a judgment prompt, no destructive or halting machinery |
| risk.correctness | moderate | stack-vs-return guidance must name both branches of the decision or a lead could still mis-target a HEAD-relative write |
| risk.fit | moderate | the awareness contract must stay host-neutral in shared shipped playbook text |
| risk.test | low | verified by mirror-integrity and playbook-render tests; no behavioral regression harness is required for a text contract |
| risk.security_or_contract | moderate | branch-awareness is a workflow-safety contract, but advisory (judgment-based), not fail-closed enforcement |

## Phases

### Phase 1: Add a mandatory branch-awareness step to lead-run stop handling

On `develop`, update the shared lead-run playbook (and the ticket-worker handoff note where it documents the return) so that, on a worker's terminal stop-condition report, the lead must call `ws/git.status` and read `branch.head`/`impl_ticket` before any HEAD-relative lead-owned write (ticket Edition/revision, hotfix commit, follow-up dispatch base), then make an explicit stack-vs-return decision stated in the playbook text: stack on the impl branch when the write belongs to that impl ticket; check out the derived base branch (`impl/<root>/<stem>` → `<root>`) when the write is unrelated or the ticket is fully blocked and the lead is exiting. Do not add base/impl handoff fields to the assignment note, do not add fail-closed refusal machinery, and do not allocate a worktree. Regenerate the required shipped mirrors.

Verify: the awareness step is present in the shared source and its shipped mirrors (mirror-integrity); the stack-vs-return guidance names both branches of the decision; the base-branch derivation matches the `ws/git.merge` `impl/<root>/<stem>` convention; and `ws/git.merge`'s existing branch-explicit behavior is documented as unaffected by the current checkout. No behavioral regression harness is required because the change is playbook text; the 2026-09-13 six-commit contamination is cited as the motivating example, not a coded test.
