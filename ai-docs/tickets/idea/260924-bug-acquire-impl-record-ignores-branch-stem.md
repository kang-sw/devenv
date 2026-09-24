---
title: tickets.acquire records any impl branch as the ticket's impl
---

# tickets.acquire records any impl branch as the ticket's impl

## Background

Dogfood surprise on 2026-09-24, in the Phase 4 H1 cycle of
`260924-feat-origin-ticket-ownership-index`. The worktree on
`impl/develop/irate-growl-half` acquired the unleased scratch stem
`260924-idea-ownership-dogfood-scratch-b`. The result was
`status: acquired` with `impl_branch: impl/develop/irate-growl-half`, a branch
that belongs to a different ticket.

- `indexCaller.implBranch` (`agents-plugin-tool/internal/mcp/ticket_index.go`)
  returns the caller's branch for any `impl/` or `implement/` prefix, and
  acquire stores it on every lease it writes.
- The ticket's Worker impl record decision covers one case only: a worker's
  acquire from its impl branch that matches the existing lease's triple.
  Neither the decision nor the tests cover a fresh acquire, or an acquire of an
  unrelated stem, made from an impl branch.
- The canonical impl branch for a stem is derivable
  (`implTicketBranch(base, stem)`, `implTicketSuffix`), so acquire could
  record `impl` only when the caller's branch matches the stem's derived
  suffix.

Effect: the view and `git.status` show a wrong `impl <branch>` for a ticket
that was acquired while standing on another ticket's impl branch, for example
from `lead-scope-worktree` run inside an impl worktree.

## Open questions

- Should acquire record `impl` only when the branch suffix matches
  `implTicketSuffix(stem)`, and otherwise behave as a plain acquire?
- Should a fresh acquire (no existing lease) from an impl branch record `impl`
  at all, or only the worker's impl-record path?
