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
- Exception: unimplemented ticket phases may be edited mid-discussion to keep the ticket accurate. Phases with a `### Result` section are frozen — do not edit them.
- Read mental-model docs on-demand as topics emerge.
- Read spec docs in `ai-docs/spec/` on-demand as topics emerge — the Project Map above lists available specs.
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
2. Read mental-model docs as conversation touches relevant domains; read spec docs as topics touch external-visible behavior; dispatch Explore agents for implementation details.
3. When discussion changes unimplemented ticket phases, update them in place with user agreement.
4. Continue until the user signals done.

## On: Ticket Status Transition

Triggers when the user requests a ticket status change — promoting an idea ticket to `todo/`, or dropping a ticket to `dropped/`.

1. Read the ticket file. Extract any `spec:` frontmatter field and body references to `{#YYMMDD-slug}` anchors.
2. **Promotion (idea/ → todo/)**:
   a. Perform `git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md`.
   b. Invoke `/write-spec` to add a `🚧` entry for each caller-visible behavior in the ticket.
3. **Drop (→ dropped/)**:
   a. For each linked spec stem: check whether any other non-dropped ticket also references it.
   b. No other ticket references this stem → invoke `/write-spec` to remove the `🚧` entry.
   c. Other tickets also reference this stem, or coverage is ambiguous → ask the user before removing.
   d. Perform `git mv ai-docs/tickets/<status>/<stem>.md ai-docs/tickets/dropped/<stem>.md`.
4. Create one commit covering the `git mv` and any spec changes together.

## On: user signals done

1. Always suggest `/write-spec` as the next step — write-spec's `judge: spec-impact` decides whether spec work is needed and exits immediately if not.
2. Then offer ticket persistence:
   - **New ticket** — invoke `ws:write-ticket`.
   - **Ticket update** — invoke `ws:write-ticket`, then append design notes to an existing ticket phase.
   - **Mental-model update** — if discussion surfaced new architectural understanding, suggest `/write-mental-model` for the update. Do not edit mental-model docs directly.
3. Apply **judge: needs-integration-tests** to ticket writes.
4. Write only what the user approves. No artifact needed for exploratory discussions.

## Workflow Context

Interface and scope decisions made in discussion become downstream inputs:
- Approach direction → spec update (`/write-spec` — always the next step after discuss)
- Scope, phases, acceptance criteria → ticket structure (`/write-ticket`)
- Type shapes, module boundaries, public API → skeleton contract directives (`/write-skeleton`)
- Approach choices, architectural trade-offs → plan directives (`/write-plan`)

The canonical chain is: `/discuss` → `/write-spec` → `/write-ticket` → `/proceed` → `/write-skeleton`? → `/write-plan`? → `/edit` | `/implement` | `/parallel-implement`.
Write-spec's judge handles the no-op case; the chain is uniform regardless of topic type.

When discussion converges on a decision in any of these categories, frame
the conclusion in terms its downstream consumer can directly act on.

## Judgments

**judge: needs-integration-tests** — Include integration-test criteria in a ticket phase when the change has end-to-end observable behavior. Skip for internal refactors.

## Doctrine

This skill optimizes for **decision quality per conversation turn**. The user is here to think, not to produce artifacts — so the agent's job is to sharpen reasoning by surfacing risks, reuse opportunities, and concrete alternatives, then capture only what the user approves. When a rule is ambiguous, apply whichever interpretation better preserves decision quality per turn.
