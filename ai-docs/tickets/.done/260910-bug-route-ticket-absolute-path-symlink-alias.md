---
title: "Accept equivalent absolute ticket paths through a symlinked root alias"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 303d1a06872053ed
sage-review-completeness-reviewed: 303d1a06872053ed
completed: 2026-09-11
---

# Accept equivalent absolute ticket paths through a symlinked root alias

## Background

While verifying `260910-chore-session-children-scope-unnoted-filters`, plain
`go test ./...` failed in
`TestEnterImplementResolvesTargetPathForms/absolute_ticket_path_under_the_root`:
the route reported an existing ticket as `missing (unreadable)` when its path
began with macOS's `/var/folders/` temporary directory alias. `/var` links to
`/private/var`; root resolution and the caller's absolute path can therefore
spell the same directory differently.

`TicketAt` in `agents-plugin-tool/internal/wsdoc/tickets.go` rebases an absolute
path using `filepath.Rel(root, relPath)` before checking the ticket-board
prefix. It does not reconcile symlink aliases at that point. The route wraps
this rejection as an unreadable ticket, hiding the confinement mismatch.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/tickets.go; a test to reproduce/cover, e.g. agents-plugin-tool/internal/mcp/implement_route_facts_test.go |
| scope.surface | cross-module | wsdoc.TicketAt is exported and called from agents-plugin-tool/internal/mcp/implement_resolver.go#L353 |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_route_facts_test.go#L210-L254 (TestTicketAtRefusesPathsOutsideTheBoard, TestTicketAtAcceptsAnAbsolutePathUnderTheRoot), agents-plugin-tool/internal/mcp/implement_route_facts_test.go#L244 (TestEnterImplementResolvesTargetPathForms) |
| complexity.reuse_points | confirmed | filepath.EvalSymlinks root-canonicalization already used in agents-plugin-tool/internal/mcp/server.go#L2630 and agents-plugin-tool/internal/wsstate/paths.go#L306 |
| complexity.side_effect_risk | low | change is confined to the absolute-path branch inside TicketAt (tickets.go#L495-L501); no new I/O beyond the existing os.ReadFile |
| risk.correctness | moderate | must accept symlink-equivalent paths while still rejecting confinement escapes, a security-relevant path check (tickets.go#L503-L509) |
| risk.fit | low | mirrors the EvalSymlinks canonicalization pattern already established in server.go#L2630 |
| risk.test | low | existing confinement and absolute-path tests give direct regression coverage to extend |
| risk.security_or_contract | moderate | touches the ticket-board confinement guard that keeps a caller-supplied path from escaping the board |

## Phases

### Phase 1: Normalize equivalent paths without weakening confinement

Reproduce the route failure using a symlink alias to a temporary repository,
then accept absolute ticket paths that resolve inside that repository's ticket
board. Preserve rejection of paths escaping the board, including symlink
escapes. Cover both equivalent aliases and confinement rejection through the
caller-visible route.

### Result (110ce16e) - 2026-09-11

`TicketAt` (agents-plugin-tool/internal/wsdoc/tickets.go) now evaluates both the
root and the absolute caller path with a best-effort `EvalSymlinks` before
`filepath.Rel`, so a parent directory reached through a symlink alias
(macOS `/var` -> `/private/var`) rebases to the same board-relative path instead
of a `../` escape. The board-prefix confinement guard is unchanged and runs on
the resolved path, so it still refuses genuine escapes; resolving the full
caller path additionally closes a pre-existing hole where a ticket-shaped
symlink on the board pointing off it would have been read (confirmed by the
correctness reviewer).

New `evalSymlinksBestEffort` helper reuses the `EvalSymlinks`-then-fallback
idiom already established for root canonicalization (`canonicalGitRoot`,
`wsstate.canonicalPath`).

Regression coverage at both layers the ticket names:
- `TestTicketAtAcceptsASymlinkAliasedAbsolutePath` (wsdoc) — accepts an aliased
  absolute path and rejects an off-board symlink escape.
- `TestEnterImplementResolvesSymlinkAliasedTicketPath` (mcp) — reproduces the
  exact `missing (unreadable)` route symptom through
  `route.resolve_implement`.
Both fail without the source change (verified by stashing the fix).

Verification: `go build ./...` clean; `go vet ./internal/wsdoc/ ./internal/mcp/`
clean; `go test ./internal/wsdoc/ ./internal/mcp/` both pass.

Review: partitioned correctness + fit, round 1 clean (no Critical/Important).
Two Minors recorded, neither actioned: fit flagged an unrelated idea-ticket edit
that is not in this diff range (false positive, verified via `git diff --stat`);
correctness noted that a *missing* ticket under an aliased root reports
`not a ticket path` rather than a read error — outside this ticket's
equivalent-existing-path contract, a message nuance only.

Decision: resolve the full caller path (not just its parent) via `EvalSymlinks`
— chosen because it both reconciles the alias and preserves symlink-escape
rejection in one step; rationale carried in the commit `## AI Context`.
