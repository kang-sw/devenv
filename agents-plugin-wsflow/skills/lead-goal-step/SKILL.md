---
name: lead-goal-step
description: Advance a goal-pursuit run by one step, picking the next `ready/` ticket and handing it to lead-proceed; `ready/` is the sole progress gate. Stop if ready/ is empty.
---

# Goal Step

**Goal-pursuit step; `ready/` is the sole progress gate.** This skill advances
a goal-pursuit run by one step: select and dispatch exactly one ticket from
`ready/` — nothing advances until a ticket reaches `ready/`. Invoked without
an active goal run, it degenerates to a single-cycle shim: one invocation
resolves at most one ready ticket and stops — it does not poll or repeat
internally. Repeated draining across the whole `ready/` backlog is the
caller's responsibility (for example, a standing `/goal` directive whose
Stop-hook re-invokes this skill each turn until the queue is empty).

**Goal-run posture.** During a goal run — the current branch is `goal/*`, or an
active `/goal` Stop-hook reminder is present — assume the user is away. Resolve
reversible, local decisions on your own stated recommendation, record the choice
in one line, and continue without waiting for confirmation; this posture carries
into the work handed off downstream, not only this skill's own steps. Stop only
for genuinely critical decisions: irreversible or destructive actions, scope
expanding into public API or cross-module patterns, unresolved binding
decisions, or any AGENTS.md "Always ask" item. The confirmation threshold rises;
the hard gates never dissolve.

Ticket-curation authority. Within the posture above, the lead may edit
existing tickets — record findings, restructure, re-triage status — and
create and link new tickets, through the normal ticket-write path
(`lead-write-ticket`); this introduces no new ticket-system state — a
recorded blocker and a captured bug are both ordinary ticket edits, not a
new field or status directory. A bug found mid-run that blocks or is
directly relevant to the current goal goes to `ready/` through that same
path (still subject to the sage ready-landing gate) for a later step to
pick up; an incidental or unrelated bug goes to `idea/`; an explicitly
deferred bug is captured only, not queued to `ready/`. This routing is
skill-intrinsic — judge it on its own terms, not against any downstream
project's own dogfood-capture convention.

Spawn a light-tier Explore-style subagent to pick the next ticket: list
`ai-docs/tickets/ready/`, and for each candidate check its body for a
recorded blocker note (e.g. a `## Blocked (...)` entry) and skip blocked
candidates; among the remaining advanceable candidates, prefer one named
as a prerequisite via another ready ticket's `related:`/`parent:`
frontmatter, otherwise the oldest (FIFO). Have it return exactly one
advanceable ticket path, or report that `ready/` is empty, or report that
every remaining `ready/` ticket is blocked. Do not list `ready/` or read
ticket files yourself — the subagent does that pinpoint read, not you.

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

If the subagent instead reports that every remaining `ready/` ticket is
blocked, this is a different outcome from the empty-queue completion
above — check the current branch the same way. When it is not `goal/*`,
this case does not apply and the unchanged non-goal stop above still
governs. When it is `goal/<parent>/<slug>`, this is the blocked-progress
conclusion: report the recorded blocker(s) to the user explicitly and end
the run — do not loop, and do not run the merge-approval flow above,
because merging here would misrepresent unfinished, blocked work as a
completed goal. The scoping guard: this conclusion applies only when every
remaining `ready/` ticket is blocked; one blocked ticket among otherwise-
workable ones is not a conclusion, the selection subagent simply skips it
and returns an advanceable one instead. The discriminator against a
hard-gate pause (the Goal-run posture paragraph above): is there any work
the lead could still do without the human? Work remaining with only a
final irreversible/destructive action awaiting sign-off is a hard-gate
pause, not this conclusion — a hard-gate pause must never be reclassified
as goal-complete or routed through this blocked-progress ending.

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

Record the blocker before yielding — this step is not skippable. When this
turn's downstream work concludes the dispatched ticket cannot advance
further without a human decision, record that blocker onto the ticket
itself before the turn ends — an ordinary body edit, e.g. a dated `##
Blocked (YYYY-MM-DD)` note mirroring the sage precedent. An unrecorded
blocker causes the next turn's selection subagent to re-pick the same
stuck ticket instead of skipping it.

Conserve lead context for the long-running goal this serves: beyond
selection, delegate everything else too — including simple tasks like
commits — to an appropriately tiered subagent or forked subagent, following
`lead-prefer-subagent`.
