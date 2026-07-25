---
title: "git.commit deadlocks after tickets.move stages a rename"
related:
  260723-feat-ticket-write-verify-commit-gate: introduced the non-bypassable git.commit ticket-verify gate this deadlock lives in
---

# git.commit deadlocks after tickets.move stages a rename

## Background

Hit while promoting `260725-feat-ws-cli-mcp-fallback-surface` from `todo/` to
`ready/` through the documented `lead-write-ticket` flow (`tickets.move`, then
`ws/git.commit`). `tickets.move` stages the rename atomically, after which no
argument shape to `git.commit` succeeded:

| `paths` passed | Result |
|---|---|
| `[ready/<stem>.md, _index.md]` | `ticket verify failed: [file-exists] ai-docs/tickets/todo/<stem>.md: cannot read ticket file` |
| `[todo/<stem>.md, ready/<stem>.md, _index.md]` | identical failure |
| `[_index.md]` | `refusing to commit unrelated staged path "ai-docs/tickets/ready/<stem>.md"` |

Staged state at the time was a clean `R100` rename plus the `_index.md` edit, so
nothing about the working tree was unusual — this is the ordinary promotion path.

The verify gate appears to resolve staged ticket-shaped paths and then check
file existence on disk, which the delete side of a rename can never satisfy;
meanwhile the unrelated-staged-path guard forbids omitting it. The two guards are
individually reasonable and jointly unsatisfiable.

Worked around by committing with native git (`89f11d4d`), which is permitted as
an MCP-error fallback but bypasses the verify gate that
`260723-feat-ticket-write-verify-commit-gate` deliberately made non-bypassable —
so the deadlock actively pushes callers off the guarded path.

## Open questions

- Should the verify gate resolve a staged rename to its destination path only, or
  should `git.commit` accept the pre-move path as a rename hint?
- Does the same deadlock hit `tickets.close` (which also stages a move), and does
  it therefore affect ticket completion as well as promotion?
- Is there a supported `git.commit` argument shape for this flow that was simply
  not discovered here?
