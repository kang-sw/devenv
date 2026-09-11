---
title: "Accept equivalent absolute ticket paths through a symlinked root alias"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 303d1a06872053ed
sage-review-completeness-reviewed: 303d1a06872053ed
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
