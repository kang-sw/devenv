---
title: "ws/tickets.close (and likely tickets.move) act on the MCP server cwd, not the caller's bound worktree root"
---

# `ws/tickets.close` acts on the MCP server cwd, not the caller's worktree

## Observed

During a parallel `ws:lead-run` batch (2026-09-15) with per-ticket
isolated worktrees, a worker whose ws session key was bound (via
`worktree.acquire` / `root_override`) to its own worktree
(`.ws-worktrees/uptown-swagger-schnapps`, branch
`impl/goal/develop/hazel-quartz-meadow/plum-tidal-wren`) called
`ws/tickets.close`. The close operated on the **MCP server process cwd**
— the lead's MAIN checkout (`/Users/kang-sw/devenv`, branch
`goal/develop/hazel-quartz-meadow`) — instead of the worker's bound
worktree. It staged a `ready → .done` rename plus a `completed:`
frontmatter edit on the lead's goal branch, materializing the moved blob
there (without the worker's Result text).

The worker had to surgically revert the stray staged change in the main
checkout and perform the close manually (`git mv` + `completed:`) on its
own impl branch. The lead independently confirmed the main checkout was
clean afterward and the `.done` transition landed only on the impl
branch.

## Why it matters

- In an isolated-worktree parallel batch, a worker closing its own ticket
  reasonably expects the board transition to land on its impl branch. A
  transition that lands on the lead's goal branch instead silently
  pollutes the integration branch and races other workers.
- The worker could not even file this idea ticket through ws tools,
  because that too would write to the same main checkout and re-pollute
  the lead's branch — so the escalation itself is blocked by the same
  bug. (The lead files it here, controlling the commit.)

## Suspected cause / scope

- Root-resolution for `tickets.close` (and probably `tickets.move`, and
  any tickets mutator) appears to use the server process cwd rather than
  the session key's bound root. `root_override` / worktree binding is
  honored by git primitives (`git.status`, `git.commit` observed correct
  in-worktree) but apparently not by the tickets mutators.

## Candidate follow-ups (triage)

- Make tickets mutators resolve their target tree from the caller's
  session-bound root, consistent with the git primitives.
- If a server-cwd fallback must remain, fail loud when the caller's bound
  root differs from the server cwd rather than silently writing to the
  latter.
- Audit `ws:lead-run`'s parallel route for whether workers should call
  `tickets.close` at all, or whether close/move must be lead-owned and
  performed post-merge on the integration branch.

## Notes

Captured under the "Dogfood surprises get captured" discipline; no design
commitment implied. Related same-session dogfood ticket:
`260915-bug-ws-ticket-facts-new-public-symbol-likely-enum`.
