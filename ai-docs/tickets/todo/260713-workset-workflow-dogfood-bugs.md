---
title: "Workset: workflow dogfood bug tickets"
---

# Workset: workflow dogfood bug tickets

## Context

A recent discussion review of the v0.33.9 "proportional low-ceremony
implementation" diet surfaced a broader backlog of `idea/`-stage bug tickets
accumulated from dogfooding since the 2026-06-05 ws-native-subagent-pivot
epic. This workset groups the workflow/tooling-facing bugs (excluding
dashboard-facing bugs, which stay under the dashboard epic's own scope) so
they can be verified and drained one at a time instead of sitting idle.

## Tickets

- `260630-bug-enter-proceed-status-report-dead-code` - idea; dead `status-report` NEXT case in proceed_resolver.go with no producing code path; trivial remove-vs-implement decision.
- `260710-bug-windows-release-cache-dependency-path` - idea; Windows release CI Go cache path not scoped to `agents-plugin-tool/go.mod`; scoped one-line CI fix.
- `260710-bug-project-index-ticket-focus-stale-status` - idea; `_index.md` Ticket Focus claims stale `ready` status for tickets already moved to `.done/`; confirmed still reproducing.
- `260710-bug-release-downstream-plugin-layout-untested` - idea; Phases 1-2 (repro + launcher fix) already landed; only Phase 3 (release-gated fresh-install acceptance check) remains open.
- `260713-bug-tickets-move-error-mutates-frontmatter` - idea; `tickets.move` writes `sage-review-completeness: required` before validating, causing duplicate-key frontmatter corruption on a blocked promotion; confirmed to recur (one prior manual hand-patch, tool behavior unchanged).
- `260703-bug-claude-plugin-cache-stuck-below-source-version-mcp-refuses-start` - idea; installed Claude plugin cache pinned below source HEAD causes the MCP server to refuse to start; related to 260627 as the more severe case in the same stale-cache class.
- `260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood` - idea; MCP-installed plugin cache serves stale `rsrc/*.md` playbook templates during source-branch dogfooding; adjacent/lesser case of 260703.
- `260626-bug-workflow-manual-bootstrap-sentinel-surface` - idea; `ws.workflow_manual` is not discoverable/callable in some MCP profiles even with a valid session; needs investigation plus a profile-visibility policy decision (security-invariant sensitive).

## Planned References

(none)

## Focus

Drain this workset by, for each included ticket: dispatching a subagent to
verify current relevance/reproduction, then triaging `idea/` -> `todo/` and,
where the ticket is autonomously decidable (no open design/policy question
requiring the user), promoting `todo/` -> `ready/` through the normal
`lead-write-ticket` spec-address and sage-review gates. Tickets that surface
a genuine design or policy decision stop at `todo/` and report the blocker
instead of being force-promoted.

## Exit Criteria

- Done: every included ticket is either promoted to `ready/`, or stopped at
  `todo/` with a reported decision blocker for the user.
- Deferred: dashboard-facing bug tickets (`260523-bug-worktree-local-index-missing`,
  `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky`) are explicitly
  out of scope for this workset.
