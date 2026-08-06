# Worktree Ticket Scope (sparse-checkout)

This file is reference material for `ws:lead-scope-worktree`, not the
caller-visible contract itself. The MCP behavior contract lives in
`ai-docs/spec/mcp-tools.md`:

- Scope mechanism and board resolution:
  `#260806-worktree-sparse-checkout-ticket-scope`
- `workflow_manual` scope-announcement rendering:
  `#260626-workflow-manual-restoration-entry`

## What This Covers

`git sparse-checkout --no-cone` lets a worktree hide `ai-docs/tickets/ready/`
and `ai-docs/tickets/todo/` entries outside one work line, while
`ai-docs/tickets/idea/` always stays visible — hiding it would collide with
the mandatory dogfood-capture rule (a captured surprise is off-topic for the
worktree that captured it by nature, so a hidden `idea/` would fail every
capture at the staging step). Cone mode cannot express this scope — it
selects directories, not files (`git sparse-checkout set <file>` fails with
`is not a directory`) — so `--no-cone` is required.

## Verified Behavior (2026-08-06, git 2.43.0, Linux)

Established by direct experiment in throwaway repositories:

- Patterns are worktree-local: `core.sparseCheckout` lands in worktree-local
  config, and the pattern file is
  `$GIT_DIR/worktrees/<name>/info/sparse-checkout`. Enabling it flips
  `extensions.worktreeConfig=true` in shared config.
- Hiding is per file, not per directory. With `!/ai-docs/tickets/todo/*` plus
  topic re-includes, `todo/` keeps its matching tickets on disk while the
  index retains all of them. A status directory disappears from disk only
  when every file in it is excluded, because git does not track empty
  directories.
- `git ls-files -v` marks excluded entries `S` (skip-worktree); `git show
  :<path>` reads a hidden ticket's body from the index.
- Staging an out-of-scope path exits **1**, whether the path is
  tracked-but-hidden or a brand-new untracked file, in both `git add <path>`
  and `git add -A -- <path>` form. The path stays `??` in `git status`.
- Cross-scope `git mv` (e.g. triaging `idea/` -> hidden `todo/`) exits **1**
  and is an atomic no-op: index, working tree, and HEAD are all unchanged, and
  `git status` stays clean because nothing happened. Widening the pattern
  first lets the move proceed, with one caveat when the destination status
  directory was fully hidden off disk — see the remedy section below.
- `git sparse-checkout disable` fully restores the worktree.

## Cross-Scope `git mv` And The Widen-Then-Retry Remedy

Promoting an out-of-scope ticket into a hidden status directory — the
declared `idea/` -> hidden `todo/` dogfood-capture hot path — fails
atomically at the move step rather than partially applying. Widen the pattern
first, then promote with `ws/tickets.move`, which recreates the status
directory the scope had emptied off disk and stages the rename atomically:

```
git sparse-checkout add ai-docs/tickets/todo/<stem>.md
ws/tickets.move(stem: "<stem>", to: "todo")
```

`ws/tickets.move` is the correct promotion tool because a fully-hidden status
directory vanishes from disk (git does not track empty directories), and a
raw `git mv` into it fails with `No such file or directory` even after the
sparse pattern is widened — widening a pattern for a not-yet-checked-out file
does not recreate the directory. If you must use raw git, recreate the
directory yourself between the widen and the move:

```
git sparse-checkout add ai-docs/tickets/todo/<stem>.md
mkdir -p ai-docs/tickets/todo
git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md
```

(2026-08-06, git 2.43.0, Linux: with `todo/` fully hidden, the widened raw
`git mv` exits **128** with `No such file or directory`, while both the
`mkdir -p` form and `ws/tickets.move` succeed with a clean `R` rename.)

## Unreproduced Hazard

A prior session (Windows, git 2.48.1) reported that a re-include pattern
could make the whole status directory vanish, taking the explicitly
re-included file with it. A later 2x2 controlled experiment isolated the two
suspected variables — exclusion at `tickets/*/*` vs `tickets/<status>/*`,
re-include by glob vs by full path — and did **not** reproduce it: all four
cells behaved correctly. Two further hypotheses, cone mode left enabled and a
CRLF pattern file, also failed to reproduce it.

**The cause is unknown, not isolated. No "avoid this pattern shape" rule can
be written** — every pattern shape exercised in the 2x2 behaved correctly in
this experiment, so no shape is provably safe either. The only consequence
that follows is procedural: pattern application must be verified by listing
the affected directories every time, because the failure direction is "hides
too much", which is silent without that check. `ws:lead-scope-worktree`
treats this verification step as mandatory, not optional, for exactly this
reason — it is the only defense that survives the experiment not isolating a
cause.

## Restore

`git sparse-checkout disable` fully restores the worktree to its pre-scope
state.
