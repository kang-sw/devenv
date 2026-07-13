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

- `260630-bug-enter-proceed-status-report-dead-code` - **done**; removed the dead `status-report` `NEXT:` case and its test assertion, `go test ./...` clean, review clean. Closed to `.done/`.
- `260713-bug-tickets-move-error-mutates-frontmatter` - **done**; `tickets.move`'s error branch now surfaces a loud partial-mutation notice when a self-healing frontmatter write preceded a block/fail. Correctness + fit review both clean, `go test ./...` clean. Closed to `.done/`.
- `260710-bug-project-index-ticket-focus-stale-status` - todo, **blocked on design review**; `_index.md` Ticket Focus claims stale `ready` status for 12+ tickets already moved to `.done/`/`.dropped/`, confirmed still reproducing (`ready/` doesn't even exist). Mechanical reconciliation is decidable; recurrence-prevention mechanism (automated guard vs. documented manual procedure vs. other) is an open design question needing the user's direction before this can proceed.
- `260710-bug-release-downstream-plugin-layout-untested` - todo; Phases 1-2 (repro + launcher fix) already landed; user decided to backlog Phase 3 (release-gated fresh-install acceptance) rather than resolve automate-vs-manual-gate now.
- `260703-bug-claude-plugin-cache-stuck-below-source-version-mcp-refuses-start` - todo; installed Claude plugin cache pinned below source HEAD causes the MCP server to refuse to start. Design review passed (folded in a spec cross-reference and a related-ticket link). User decided to backlog: recent hotfixes have made this class of install clutter rare in practice, and the immediate symptom was worked around manually at the time.
- `260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood` - todo, **blocked on design review**; MCP-installed plugin cache serves stale `rsrc/*.md` playbook templates during source-branch dogfooding; adjacent/lesser case of 260703. Reviewer found the ticket omits the existing `WS_RSRC_ROOT` dev affordance and overlaps open ticket `260612-bug-ws-rsrc-dev-server-new-file-staleness` — needs investigation before a solution direction can be chosen.

## Dropped (resolved by unrelated prior work)

- `260710-bug-windows-release-cache-dependency-path` - moved to `.dropped/`; no longer reproduces.
- `260626-bug-workflow-manual-bootstrap-sentinel-surface` - moved to `.dropped/`; superseded by the M3 mercenary-reshape capability-scope gate.

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
