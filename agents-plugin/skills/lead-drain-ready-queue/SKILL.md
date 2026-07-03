---
name: lead-drain-ready-queue
description: Select one ticket from ai-docs/tickets/ready/ by precedence-then-FIFO rule and hand it to lead-proceed. Use when the user wants the next ready ticket picked and started without manually naming a ticket path.
---

# Drain Ready Queue

Single-cycle only: this skill resolves at most one ticket per invocation. It
never loops internally over `ready/` and never repeats `tickets.list` polling
inside itself. Repeated draining across multiple ready tickets is owned by the
`/goal` Stop-hook, not by this skill.

## Selection Rule

1. If `ready/` is empty, report that and stop; do not hand off to
   `lead-proceed`.
2. For each ready candidate, read its `related:`/`parent:` frontmatter for
   precedence language ("prerequisite", "predecessor", "must land first",
   "blocks", "depends on") naming another ticket that is not yet
   `done`/`dropped`.
3. If that named ticket is also in `ready/`, prefer it first.
4. If no precedence signal exists among the current ready candidates,
   default to the oldest date-prefix ticket (FIFO).
5. A precedence annotation naming an unresolved ticket in `todo/` or `idea/`
   (not in `ready/`, not done/dropped) has no in-ready target to defer to —
   treat it as no signal and fall through to the FIFO default.
6. If two candidates carry conflicting or unresolvable precedence
   annotations, stop and ask the user; do not guess.
7. Precedence resolution is single-level only — do not chase transitive
   precedence chains.
8. Container tickets (epic/workset) are not filtered out at selection time;
   `lead-proceed`'s existing `scope_blocked=container-ticket` guard handles
   that case downstream.

## Process

1. List `ready/` tickets and apply the Selection Rule above to resolve
   exactly one ticket path (or stop per step 1 or step 6).
2. Apply the `lead-prefer-subagent` posture for this invocation by invoking
   that skill; do not restate or duplicate its body here.
3. Hand off to `lead-proceed` with the resolved ticket path passed as an
   explicit target. Never invoke `lead-proceed` bare expecting it to infer a
   ticket from `ready/` on its own.

## Handoff

Output the resolved ticket path (or the empty-queue / conflict stop reason)
before handing off, so the caller can see which ticket was selected and why.
