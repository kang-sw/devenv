---
name: lead-goal-step
description: Advance a goal-pursuit run by one step, picking the next `ready/` ticket and handing it to lead-proceed; `ready/` is the sole progress gate. Stop if ready/ is empty.
---

# Goal Step

**Goal-pursuit step; `ready/` is the sole progress gate.** Select and dispatch
exactly one ticket from `ready/` — nothing advances until a ticket reaches it.
One invocation resolves at most one ticket and never repeats internally;
draining the whole backlog is the caller's job, typically a standing `/goal`
directive that re-invokes this skill each turn.

## Posture

A goal run — current branch `goal/*`, or an active `/goal` Stop-hook reminder —
means the user is away. Resolve reversible local decisions on your own stated
recommendation, record the choice in one line, and continue; this carries into
the work you hand off, not only your own steps. Stop only for the genuinely
critical: irreversible or destructive actions, scope expanding into public API
or cross-module patterns, unresolved binding decisions, any AGENTS.md "Always
ask" item. An action the ticket's sage-settled design already scopes is
pre-authorized — execute it, don't re-ask. Only what the ticket did **not**
settle still stops.

Curate tickets as you go, through `lead-write-ticket`: record findings,
restructure, re-triage status, create and link. Route bugs found mid-run by
relevance — blocking or goal-relevant to `ready/` (still subject to the sage
ready-landing gate), incidental to `idea/`, explicitly deferred captured only.
Judge this routing on its own terms, not against a downstream project's own
dogfood-capture convention.

Conserve your context for the long run this serves: beyond selection, delegate
everything — including commits — to an appropriately tiered subagent, following
`lead-prefer-subagent`.

## Select

Spawn a light-tier Explore-style subagent to pick the next ticket. Do not list
`ready/` or read ticket files yourself — the subagent does that pinpoint read.
Have it skip candidates carrying a recorded blocker note (e.g. a
`## Blocked (...)` entry), then among the rest prefer, in order: a ticket already
in progress (some phase has a `### Result`, at least one does not), one named as
a prerequisite by another ready ticket's `related:`/`parent:`, otherwise the
oldest (FIFO). It returns exactly one advanceable ticket path, or reports
`ready/` empty, or reports every remaining ticket blocked.

Both empty and all-blocked stop the turn with no handoff. Off a `goal/*` branch
that is the whole behavior; on `goal/<parent>/<slug>` each has its own terminal
below.

## Dispatch a returned ticket

If a `/goal` reminder is active and the branch is not already `goal/*`, stage
first: capture the current branch as PARENT via `git rev-parse --abbrev-ref
HEAD`, then `git checkout -b goal/<parent>/<slug>` with a random word-word-word
slug — never derived from the goal text, which collides across concurrent runs
of the same command. On a literal `HEAD` (detached), abort staging with a clear
message and dispatch unstaged.

Hand off to `lead-proceed` with the path as an explicit target; never call it
bare. On a `goal/*` branch add `policy.branch.merge_confirm: "skip"` as caller
policy, and set no merge target — the checked-out branch is picked up
automatically.

## Terminal: `ready/` empty on a goal branch

The run's completion point. PARENT is everything between the `goal/` prefix and
the last `/`; a single-segment `goal/<slug>` falls back to `main`. Ask the user
for explicit approval to merge into PARENT, and only on approval merge it
yourself with plain `git` (`--no-ff`, repository commit rules for the merge
commit). Never push.

## Terminal: every remaining ticket blocked on a goal branch

Report the recorded blockers and end the run. Do not merge — that would
misrepresent blocked work as a completed goal.

Discriminate this from a hard-gate pause: is there work you could still do
without the human? Work remaining behind a single pending sign-off is a pause,
and a pause must never be reclassified as goal-complete or routed through this
ending.

## Ending the turn

**One finished ticket is not a finished goal, and the turn's last line
decides.** Whatever re-invokes this skill judges from this skill's name and the
visible transcript, never from this body — so make the terminal call yourself
and hand it over verbatim as the turn's final line:

- Continuing: `Ready queue still has advanceable tickets — next cycle: lead-goal-step.`
- Ending: `Goal run finished — <reason>. Do not re-invoke lead-goal-step.`

Write nothing after that line; a wrap-up placed there is what gets read last.
Keep `finished`, `complete`, and `done` out of a continuing turn entirely — name
the ticket that landed instead.

Record a blocker before yielding — not skippable. When this turn's work
concludes the dispatched ticket cannot advance without a human decision, write
that onto the ticket as a dated `## Blocked (YYYY-MM-DD)` note. Unrecorded, the
next turn's selector re-picks the same stuck ticket.
