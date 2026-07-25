---
title: "git.commit deadlocks after tickets.move stages a rename"
related:
  260723-feat-ticket-write-verify-commit-gate: introduced the non-bypassable git.commit ticket-verify gate this deadlock lives in
dropped: 2026-07-25
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


## Resolution (2026-07-25)

Duplicate. Absorbed into `260725-idea-ws-git-commit-rename-and-payload-rejections`, which was written the same day on the `impl/macos-terminal-acceptance-phase2` worktree and cherry-picked into `main` afterwards. That ticket already covered this rename defect as its Finding 1 and additionally carries a second, independent `ws/git.commit` defect (a large `ai_context` array rejected with `ai_context requires at least one entry`, triggered by payload size rather than emptiness) that this ticket never observed.

Nothing was lost in the merge: this ticket's unique evidence — the full three-row argument matrix, including the `[_index.md]`-only case that is refused by the *unrelated-staged-path* guard, which is what makes the failure a genuine deadlock rather than an undiscovered argument shape — was folded into the surviving ticket as an "Independent reproduction" subsection, along with the observation that the deadlock forces callers onto the native-git fallback that `260723-feat-ticket-write-verify-commit-gate` exists to prevent.
