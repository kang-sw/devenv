# Plan: close triaged idea tickets (board (458) b)

Branch `delegate/idea-triage-close` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push.

The user approved closing groups A and B below (board (458) b, 2026-11-08)
after a read-only triage of every `idea/` ticket with a 2605-2609 stem. The
triage tables, with each ticket's verdict and file:line evidence, are
`/tmp/idea-triage/2605-2608.md` and `/tmp/idea-triage/2609.md`. Read
the ticket conventions (`ws/convention.read(name: "ticket-conventions")`)
first.

## Do

- Close each ticket below with `ws/tickets.close` (status `done` for A-1,
  `dropped` for A-2 and B), passing a one-line English reason taken from the
  triage table's evidence column (name the superseding `.done` stem or the
  fixing commit when the table gives one; for a duplicate, name the keeper).
- Call the ws tools with session key `gentleman-giggling-safari` (this
  worktree's worker key) and confirm with `git -C <worktree> status` that the
  move happened in this worktree, not in `/Users/kang-sw/devenv`. If the tool
  acts on the primary checkout or refuses, stop using it, restore anything it
  changed there, and fall back to `git mv` in the worktree after appending a
  `## Closed (2026-11-08)` section with the one-line reason to the ticket.
- Do not touch tickets outside these lists. In particular
  `260831-chore-remove-orphaned-review-adjudicator-delegate`,
  `260730-refactor-retire-goal-fan-out-step-and-session-note` and
  `260726-feat-proceed-sage-posture-consumption-guard` are closed by a
  parallel delegate, and group C stays open.
- Before closing, grep `ai-docs/` and `agents-plugin*/` for references to each
  stem outside `ai-docs/tickets/.done`, `.dropped` and `.plans`; report live
  references (for example `related:` links from open tickets or a test skip
  comment) instead of editing them.
- One commit (or a few, grouped by A-1 / A-2 / B) with `## AI Context` citing
  board (458) b.

## A-1 close as done (32)

- 260512-research-claude-cli-stream-json
- 260610-chore-wsflow-explore-playbook-mirroring
- 260611-bug-rsrc-manifest-regen-missed-after-shipped-edit
- 260708-feat-lead-revive-hook-replacement
- 260708-research-lead-revive-low-salience
- 260710-bug-release-downstream-plugin-layout-untested
- 260721-bug-codex-spawn-schema-routing-field-drift
- 260724-bug-ws-git-commit-verify-fails-on-staged-rename
- 260727-bug-goal-step-blocker-detection-ignores-resolution
- 260728-research-parallel-workflow-guide-divergence
- 260730-research-ws-dashboard-drop-for-orca
- 260731-bug-implementer-ticket-result-read-authorization
- 260802-research-ws-pi-native-framework
- 260811-bug-skills-manifest-regen-missing-from-authoring-procedure
- 260812-bug-wsflow-runtime-contract-missing-note-mute
- 260823-bug-testmain-ws-skills-root-default
- 260828-bug-sage-stamp-leaves-blocked-section
- 260904-bug-subagent-worktree-head-detach-orphans-commits
- 260906-workset-ws-pi-dogfood-ux
- 260911-epic-ws-pi-refound-resync-harness-peer
- 260911-bug-implement-route-reuses-prior-phase-branch
- 260913-research-cross-session-mailbox
- 260915-bug-ws-ticket-facts-new-public-symbol-likely-enum
- 260916-bug-exec-mcp-windows-tempdir-cleanup-race
- 260917-bug-ticket-close-checker-result-heading-level-drift
- 260917-bug-windows-smoke-servestdio-exit-timing-flake
- 260918-feat-route-implement-shared-branch-continuation-override
- 260919-research-mini-lead-intra-ticket-orchestration
- 260924-research-origin-ticket-ownership-index
- 260930-bug-lead-run-parallel-merge-before-release-orphans-branch
- 261004-bug-pi-identical-summary-compaction-event
- 261008-bug-pi-native-summary-background

## A-2 drop (58)

- 260513-research-dual-mcp-startup-order
- 260524-feat-exec-output-ask
- 260611-research-ws-per-role-delegation-tuning-config
- 260625-research-fork-posture-leak-system-guarantee
- 260626-research-ws-todo-stack-nesting-model
- 260627-research-lead-proceed-route-matrix-authoring
- 260629-research-fork-worker-persona-bleed
- 260703-chore-review-delegates-true-classification
- 260722-feat-goal-run-autonomy-posture
- 260723-research-spec-collocator-subagent
- 260725-research-goal-loop-restart-starved-by-background-delegation
- 260726-research-review-loop-nonclean-generator
- 260726-research-skill-prose-compression-ceiling
- 260726-research-spec-planned-marker-management-cost
- 260727-bug-review-findings-file-rewritten-on-disposition
- 260727-refactor-implementer-delegate-shared-base
- 260728-research-index-ticket-table-drift
- 260728-research-marker-retirement-instruction-no-check
- 260728-research-mcp-json-surface-removal-deprecation
- 260728-research-presentation-artifact-convention-policy
- 260729-bug-git-commit-omits-mental-model-checkpoint
- 260729-research-implement-router-prose-only-dimension
- 260729-research-survey-plan-scope-and-plan-depth-signal
- 260730-research-orca-plugin-ws-workflow-surface
- 260731-bug-dashboard-drop-property-scan-implementation-branch
- 260731-bug-todo-check-parallel-response-staleness
- 260806-bug-drain-select-primitive-unbound
- 260823-bug-workflow-manual-test-fixture-missing-prefer-subagent-skill
- 260828-bug-reviewer-playbook-render-context-contract
- 260830-bug-proceed-continuation-replays-stale-done-target
- 260901-bug-enter-proceed-misplaced-facts-silent-unknown-status
- 260901-research-enter-tool-direct-call-affordance-rename
- 260904-bug-stale-proceed-dispatch-contract-test
- 260904-refactor-mental-model-doc-drift-epic-renames
- 260904-research-mental-models-query-reconciliation
- 260907-feat-ws-tickets-query-pagination-and-keyword-ranking
- 260907-research-ws-pi-extreme-delegation-lead-profile
- 260908-research-ws-pi-agent-driven-branch-summary-return
- 260911-research-batch-promotion-cross-ticket-coherence-gap
- 260911-research-epic-close-on-last-child-prompt
- 260911-research-lead-commit-guard-during-worker-run
- 260911-research-ticket-decision-state-convention-gap
- 260912-bug-ws-todo-concurrent-updates-lose-write
- 260912-research-git-merge-epic-develop-boundary
- 260912-research-git-merge-release-target-policy
- 260912-research-september-active-ticket-inventory-triage
- 260913-bug-ws-pi-worker-checkout-contaminates-lead-branch
- 260913-research-promotion-dependency-landing-gate
- 260915-bug-pi-bridge-bundled-tool-count
- 260915-bug-ws-tickets-close-operates-on-server-cwd-not-worktree
- 260915-research-lead-run-tier-selection-qualitative
- 260919-research-mailbox-remote-control-target-arm-condition
- 260920-bug-pi-nested-subagent-terminal-delivery-stall
- 260923-bug-pi-execute-approval-accepted-worker-hangs
- 260923-bug-pi-fork-owner-thread-windows-instability
- 261029-bug-sage-stamp-restamp-repeats-fix-instruction
- 261105-bug-sage-stamp-move-next-instruction-loop
- 261007-bug-mailbox-tests-leak-ambient-ws-mailbox-env

## B drop (unclear, no repro) (8)

- 260616-epic-api-namespace-documentation-memory-tooling
- 260726-bug-playbook-render-serves-stale-prompt-text
- 260903-feat-human-relay-interactive-gate
- 260913-research-codex-hook-trust-persistence
- 260914-bug-pi-execute-worker-resume-session-home-unavailable
- 260916-research-lead-run-per-phase-context-and-dispatch
- 260926-bug-wsindex-hang-transport-stop-wait-fails-on-local-windows-host
- 260930-bug-close-lease-prune-test-tempdir-cleanup-flake

Extra rows not in the triage tables (from the earlier first-pass triage):
`261004-bug-pi-identical-summary-compaction-event` (fixed by 5a63a682d),
`261008-bug-pi-native-summary-background` (fixed by 0b1fdce02),
`261029-bug-sage-stamp-restamp-repeats-fix-instruction` and
`261105-bug-sage-stamp-move-next-instruction-loop` (duplicates of keeper
`261106-bug-sage-stamp-missing-restamp-loop`),
`261007-bug-mailbox-tests-leak-ambient-ws-mailbox-env` (duplicate of keeper
`260913-bug-mailbox-tests-read-ambient-ws-mailbox-env`).

## Verification

`find ai-docs/tickets/idea -name '<stem>.md'` finds none of the listed stems;
each is under `.done/` or `.dropped/`; `git status` clean after commit.

## Return

Commit hashes, count closed per group, any stem the tool could not close, the
fallback used if any, and live references found.
