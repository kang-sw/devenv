---
title: "Pre-merge idea-backlog sweep (ferrule)"
sage-review: required
completed: 2026-06-29
---

# Pre-merge idea-backlog sweep (ferrule)

## Context

Before merging `feature/ferrule` toward `main`, the `idea/` backlog accumulated dogfood-surfaced tickets. This workset is a non-hierarchical operating board for one pre-merge triage pass: separate simple tweaks from policy-character changes (tickets that alter observable workflow behavior, conventions, interfaces, or defaults), pin policy decisions in the owning tickets, and route every idea ticket. It reparents nothing; tickets owned by living epics (`260514` dashboard, `260524` exec, `260616` api) and the pivot direction anchor are listed for visibility only and stay owned where they are. The Windows real-hardware gate (`260622-chore-windows-shipping-hardening`) is tracked separately and excluded.

## Tickets

### Decided this pass — see each ticket's `## Decision (260629 sweep)`
- `260624-feat-prefer-mercenary-hide-option` - closed; `hide` value already implemented and tested.
- `260620-bug-mercenary-path-visible-when-prefer-off` - policy; decided, promoted to todo.
- `260620-bug-ws-delegate-playbook-output-language-unbound` - policy (low risk); decided, promoted to todo.
- `260626-bug-sage-review-config-setter-missing` - policy; decided, promoted to todo.

### Policy-character, decision pending
- `260525-bug-lead-implement-delegation-pre-edit-guard` - implement-skill execution boundary.
- `260617-feat-fresh-reader-audit-playbook` - new delegate playbook/role.
- `260619-research-ws-delegate-continuity-host-neutral-fallback` - delegate-continuity fallback behavior.
- `260626-bug-prefer-subagent-fork-executor-narration` - fork-directive guidance.
- `260626-feat-user-preference-save-routing` - skill routing boundary.
- `260629-design-enter-proceed-no-obsolete-target-route` - proceed verdict vocabulary/routing.
- `260620-bug-ws-prompt-override-no-unset-path` - new config op + harness default semantics (verify `config.prompt.unset` residual).
- `260627-bug-enter-implement-direct-edit-policy-gap` - implementation routing policy.
- `260627-bug-write-ticket-bypasses-tickets-create` - ticket-creation routing default.

### Tweaks — auto-proceed, no policy decision
- `260610-chore-wsflow-explore-playbook-mirroring`
- `260622-bug-wsflow-launcher-coldload-divergence`
- `260626-bug-prefer-subagent-recursive-delegate-escape`
- `260622-bug-bump-version-script-edits-legacy-launcher`
- `260624-design-session-scope-hide-not-reflected-in-tools-list`
- `260624-perf-mercenary-hidden-config-read-per-tool`
- `260625-bug-mcp-test-suite-baseline-failures`
- `260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit`
- `260626-bug-workflow-manual-bootstrap-sentinel-surface`
- `260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood`
- `260523-chore-implement-branch-cleanup-guidance`

### Research / direction — no implementation; fold-or-keep decided later
- `260611-research-ws-per-role-delegation-tuning-config`
- `260625-research-fork-posture-leak-system-guarantee`
- `260626-research-ws-todo-stack-nesting-model`
- `260626-research-playbook-print-lead-surface-leak`
- `260627-research-lead-proceed-route-matrix-authoring`

### Listed for visibility — owned by living epics / separate tracks, not active here
- Dashboard epic `260514`: `260514-research-ws-web-dashboard-direction`, `260523-bug-worktree-local-index-missing`, `260523-feat-ws-dashboard-main-session-activity-source`, `260523-research-ws-dashboard-persistable-ui-state-map`, `260524-research-ws-dashboard-react-aria-ui-primitives`, `260524-research-ws-dashboard-visual-design-system-refresh`, `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky`
- Infra/transport track: `260512-research-claude-cli-stream-json`, `260513-research-dual-mcp-startup-order`, `260513-research-streamable-http-mcp-transport`
- Pivot direction anchor (reference): `260605-research-ws-native-subagent-pivot`

## Planned References

- None.

## Focus

Pre-merge triage of the `idea/` backlog for `feature/ferrule`: every idea ticket either routed with a pinned decision, classified as a ready tweak, parked as research, or attributed to a living epic.

## Exit Criteria

- Done: every policy-character idea ticket has a pinned decision in its own body; tweaks implemented or promoted; research parked; epic/infra-owned tickets attributed. Closes when no undecided policy item remains in the pre-merge backlog.
- Deferred: research-class and epic-owned tickets exit to their own tracks; post-merge follow-ups leave the workset.


## Resolution (2026-06-29)

All exit criteria met: every policy-character ticket has a pinned decision; tweaks implemented; research parked; epic/infra-owned tickets attributed. Policy implementation follow-up tracked in 260629-workset-policy-sweep-implementation. Track 2 design session (260525, 260627) completed in same session.
