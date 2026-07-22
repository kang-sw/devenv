---
name: lead-drain-ready-queue
description: Pick the next ready/ ticket and hand it to lead-proceed. Stop if ready/ is empty.
---

# Drain Ready Queue

Spawn a light-tier Explore-style subagent to pick the next ticket: list
`ai-docs/tickets/ready/`, prefer one named as a prerequisite via another
ready ticket's `related:`/`parent:` frontmatter, otherwise the oldest
(FIFO); have it return exactly one ticket path, or report that `ready/` is
empty. Do not list `ready/` or read ticket files yourself — the subagent
does that pinpoint read, not you.

If the subagent reports `ready/` is empty, check the current branch. When
it is not `goal/*`, stop — do not hand off; this is today's behavior,
unchanged. When it is `goal/<parent>/<slug>`, this is the goal run's
completion point: derive PARENT and SLUG from the branch name — strip the
`goal/` prefix, then split on the LAST `/`; everything before that final
`/` is PARENT, the final segment is SLUG. If there is no parent segment
(old-format `goal/<slug>`, a single segment after the prefix), fall back
to `main` as the merge target instead. Ask the user for explicit approval
to merge `goal/<parent>/<slug>` into `<parent>` (or, for the old-format
fallback, `goal/<slug>` into `main`) — the same approval spirit as
`lead-implement`'s Branch invariant — wait for explicit approval before
merging — and only on approval perform the merge yourself with plain `git`
commands (e.g. `git checkout <parent> && git merge --no-ff
goal/<parent>/<slug>`, or the `main`-fallback equivalent, following
repository commit rules for the merge commit). This override never
extends to push or remote actions — do not push after this merge.

Otherwise, a ticket path was returned. Before dispatching it, check for an
active goal-staging context yourself (not the selection subagent): an
active `/goal` Stop-hook reminder present in the current turn, and the
current branch not already `goal/*`. When both hold, capture the current
branch name first — e.g. via `git rev-parse --abbrev-ref HEAD` — as
PARENT, the fork point this goal run branches from. Detached-HEAD guard:
if that command returns literal `HEAD` (no branch checked out), abort
staging-branch creation with a clear message instead of creating
`goal/HEAD/<slug>`, and fall through to the non-staging path below.
Otherwise generate an arbitrary random branch-safe slug (a short
word-word-word token, e.g. `canny-hello-stride` — never derived from the
goal text or command name) and create and check out the staging branch
directly — `git checkout -b goal/<parent>/<slug>` — before the handoff. A
slug derived from the goal text collides across independent concurrent
goal runs of the same command (git branches are shared across worktrees
of one repository), so the slug must be randomly generated per run
instead. When no such reminder is active, or the current branch is
already `goal/*`, skip this step and stay on the current branch; this
preserves today's non-staging behavior exactly when no goal context is
active.

Hand off to `lead-proceed` with the returned path as an explicit target;
never call it bare. When the current branch is `goal/*`, include
`policy.branch.merge_confirm: "skip"` as explicit caller policy alongside
the handoff so the ensuing implementation merges into the goal branch
without asking; do not set an explicit merge target — the checked-out goal
branch is picked up automatically. When no goal-staging context is active,
hand off exactly as before: no merge-confirm override, no staging branch.

Conserve lead context for the long-running goal this serves: beyond
selection, delegate everything else too — including simple tasks like
commits — to an appropriately tiered subagent or forked subagent, following
`lead-prefer-subagent`.
