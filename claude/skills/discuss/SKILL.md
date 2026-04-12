---
name: discuss
description: >
  When the user explores approach or direction rather than requesting
  immediate code changes, invoke this. Captures conclusions as tickets
  or plans.
argument-hint: "[topic, ticket path, or question — optional]"
---

# Discuss

Topic: $ARGUMENTS

## Project Map

!`python3 "${CLAUDE_SKILL_DIR}/list-active.py"`

## Invariants

- No source edits. Only documentation writes, only in the capture step.
- Exception: unimplemented ticket phases may be edited mid-discussion to keep the ticket accurate. Completed phases are immutable.
- Read mental-model docs on-demand as topics emerge.
- Dispatch Explore agents for implementation details beyond mental-model docs — never read source directly.
- When docs are stale or insufficient, say so and suggest `/write-mental-model` — do not speculate.
- Before proposing new abstractions, surface existing patterns or components that already solve part of the problem.
- Evaluate each claim independently — call out unaddressed risks with reasoning; do not parrot back risks already discussed and resolved.
- Never proactively ask to wrap up or persist; wait for the user's explicit signal.
- All written artifacts must be in English regardless of conversation language.

## On: invoke

1. If `$ARGUMENTS` references a ticket, read it.
2. Enter discussion loop.

## On: discussion loop

1. Brainstorm iteratively — suggest approaches, point out analogies, sketch concrete shapes for vague ideas.
2. Read mental-model docs as conversation touches relevant domains; dispatch Explore agents for deeper detail.
3. When discussion changes unimplemented ticket phases, update them in place with user agreement.
4. Continue until the user signals done.

## On: user signals done

1. Offer persistence options only if conclusions warrant it:
   - **New ticket** — invoke `/write-ticket`.
   - **Ticket update** — invoke `/write-ticket`, then append design notes to an existing ticket phase.
   - **Mental-model update** — if discussion surfaced new architectural understanding, suggest `/write-mental-model` for the update. Do not edit mental-model docs directly.
2. Apply **judge: needs-integration-tests** to ticket writes.
3. Write only what the user approves. No artifact needed for exploratory discussions.

## Judgments

**judge: needs-integration-tests** — Include integration-test criteria in a ticket phase when the change has end-to-end observable behavior. Skip for internal refactors.

## Doctrine

This skill optimizes for **decision quality per conversation turn**. The user is here to think, not to produce artifacts — so the agent's job is to sharpen reasoning by surfacing risks, reuse opportunities, and concrete alternatives, then capture only what the user approves. When a rule is ambiguous, apply whichever interpretation better preserves decision quality per turn.
