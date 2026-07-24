---
title: ws/git.commit verify fails on a rename already staged by ws/tickets.move
---

# ws/git.commit verify fails on a rename already staged by ws/tickets.move

## Background

The canonical ready-promotion flow is `ws/tickets.move(stem, to: "ready")`
(stages the `todo/ → ready/` rename atomically) followed by `ws/git.commit`.
Observed 2026-07-24 while promoting `260724-feat-lead-fan-out-worktree`:
`ws/git.commit` fails its own ticket-verify step and cannot commit.

## Evidence

- After `tickets.move`, `git diff --cached --name-status` showed the expected
  `R100 ai-docs/tickets/todo/…-lead-fan-out-worktree.md -> ai-docs/tickets/ready/…`
  plus `M ai-docs/_index.md` — index correct.
- `ws/git.commit(paths: ["ai-docs/tickets/ready/…", "ai-docs/_index.md"])` failed:
  `ticket verify failed: [file-exists] ai-docs/tickets/todo/…-lead-fan-out-worktree.md:
  cannot read ticket file: … no such file or directory`.
- Passing the old todo path in `paths` failed identically; omitting it failed too.
  The verify step resolves the ticket at its **pre-move (todo/)** path even though
  the rename is already staged and the file physically lives at the ready/ path.
- Native `git commit` of the same staged index succeeded (commit `4653cdc9`).

## Hypothesis

`git.commit`'s pre-commit ticket verify derives the ticket path from the staged
rename's *source* side (or from a moved-ticket detection that reads the old path)
and stats it on disk, where it no longer exists. It should resolve the staged
rename to its destination path (or read from the index/destination) before
file-exists verification.

## Impact

The documented `tickets.move` → `git.commit` promotion path is broken for any
status move, forcing a native-git fallback. Likely reproduces on every ready
promotion and every `.done`/`.dropped` close that stages a rename before commit.

## Notes

Surfaced during dogfooding the `lead-fan-out-worktree` capture
(`260724-feat-lead-fan-out-worktree`). Not yet reduced to a minimal repro or
traced to the verify implementation.
