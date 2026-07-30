---
name: lead-drain-ready-queue
description: Drain the `ready/` ticket queue — one ticket per invocation, re-invoked until nothing advanceable remains. Picks the next `ready/` ticket and hands it to lead-proceed; `ready/` is the sole progress gate.
---

# Drain Ready Queue

**Draining `ready/`, which is the sole progress gate.** Select and dispatch
exactly one ticket from `ready/` — nothing advances until a ticket reaches it.
One invocation resolves at most one ticket and never repeats internally;
draining the queue takes repeated invocation, typically a standing `/goal`
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

<playbook name="lead-prefer-subagent" title="Prefer Subagent">

# Prefer Subagent

Maximum-delegation posture for this session: delegate all payload execution to a fresh subagent by default. The lead reads this playbook, chooses delegation strategy, writes delegate prompts, adjudicates results, asks the user for approval or judgment, and writes the final synthesis. The sole carve-out to full delegation: authoring or mutating a durable artifact (ticket, spec) stays with whichever session already holds the authoritative context for that decision — see the whitelist below. Outside that carve-out, no inline reads, searches, edits, tests, commits, or artifact writing to solve the task.

Keep workflow state-machine ownership with the lead. The lead follows the active skill to select the execution payload, record workflow state, and delegate only that selected payload. An execution payload is the scoped work item the delegate must perform, including artifact paths, constraints, and stop condition.

Route every **new** delegated task to a fresh spawn built from named artifacts plus general constraints, never from a copy of this conversation. A standing role (implementer, reviewer, …) **opens with** a fresh spawn — this is unconditional — and captures the conversation's decisions into its spec so the fresh spawn stays self-contained.

Continue an existing delegate's session when the instruction is the same work item that delegate already owns — a review finding relayed back to its implementer, a widened query to the explorer that ran it, a gap filled by the survey agent that produced it. Open a fresh spawn instead when the work item is new, or when the judgment must not inherit the prior agent's conclusion — an independent review verdict, or a re-check of a claim that agent itself made.

Central authoring/mutation whitelist (owned here, not by individual skills): durable-artifact authoring or mutation stays with the session that already holds the authoritative context for the decision — lead-inline when the decision was settled in this conversation, or the delegated subagent's own continuing session when it was settled there. Never hand a durable artifact's authoring to a separate fresh spawn working only from an after-the-fact summary of a decision it did not make; a summary loses the reasoning a correct write depends on.

Keep the delegate prompt concise, usually under 300 words, since a fresh spawn starts with no other context. State the task, the artifacts and constraints it needs, permitted files/actions, and a concrete stop condition. Require this exact return format: `Outcome: ...`; `Files changed: ... or none`; `Verification: command/result summary, or not applicable`; `Blockers: ... or none`; `Commit: <hash> or none`.
</playbook>

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

- Continuing: `Ready queue still has advanceable tickets — next cycle: lead-drain-ready-queue.`
- Ending: `Goal run finished — <reason>. Do not re-invoke lead-drain-ready-queue.`

Write nothing after that line; a wrap-up placed there is what gets read last.
Keep `finished`, `complete`, and `done` out of a continuing turn entirely — name
the ticket that landed instead.

Record a blocker before yielding — not skippable. When this turn's work
concludes the dispatched ticket cannot advance without a human decision, write
that onto the ticket as a dated `## Blocked (YYYY-MM-DD)` note. Unrecorded, the
next turn's selector re-picks the same stuck ticket.
