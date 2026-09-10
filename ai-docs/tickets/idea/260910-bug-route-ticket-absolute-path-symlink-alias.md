---
title: "Accept equivalent absolute ticket paths through a symlinked root alias"
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

## Phases

### Phase 1: Normalize equivalent paths without weakening confinement

Reproduce the route failure using a symlink alias to a temporary repository,
then accept absolute ticket paths that resolve inside that repository's ticket
board. Preserve rejection of paths escaping the board, including symlink
escapes. Cover both equivalent aliases and confinement rejection through the
caller-visible route.
