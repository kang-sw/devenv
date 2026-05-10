---
name: discuss
description: >
  Use before code changes when the user wants to explore workflow design,
  migration direction, ticket scope, risks, or implementation approach.
argument-hint: "[topic, ticket path, or question — optional]"
---

# Discuss

Topic: $ARGUMENTS

## Project Map

!`ws-proj-tree`

## Invariants

- No source edits. Only documentation writes, only in the capture step.
- Exception: unimplemented ticket phases may be edited mid-discussion to keep the ticket accurate. Phases with a `### Result` section are frozen — do not edit them.
- Read mental-model docs on-demand as topics emerge.
- Read spec docs in `ai-docs/spec/` on-demand as topics emerge — the Project Map above lists available specs.
- Dispatch Explore agents for implementation details beyond mental-model docs — never read source directly.
- When docs are stale or insufficient, say so — do not speculate.
- Before proposing new abstractions, surface existing patterns or components that already solve part of the problem.
- Evaluate each claim independently — call out unaddressed risks with reasoning; do not parrot back risks already discussed and resolved.
- Intent frames summarize decision rationale; they do not expose raw hidden reasoning.
- Never proactively ask to wrap up or persist; wait for the user's explicit signal.
- All written artifacts must be in English regardless of conversation language.

## On: invoke

1. Invoke `ws:workflow` via Skill tool (loads orchestration primitives reference).
2. Run `git branch --show-current`. If the result starts with `sprint/`, emit: "Note: sprint branch `<branch-name>` detected — `/sprint` provides session continuity."
3. If `$ARGUMENTS` references a ticket, read it.
4. Enter user-message handling.

## On: user message

1. Apply **judge: needs-survey** — identify every named component, skill, agent, spec, or ticket the current question touches. For each: was its doc loaded this session? If any was not, register and call `project-survey`:
   ```bash
   ws-new-named-agent project-survey -p project-survey --no-doc-system
   ws-call-named-agent project-survey "<topic brief>"
   ```
   Incorporate the returned reference list before responding.
2. Read mental-model docs as conversation touches relevant domains; read spec docs as topics touch external-visible behavior; dispatch Explore agents for implementation details.
   When reading a mental-model domain file, run `git log -1 --format="%ai" -- ai-docs/mental-model/<domain>.md`. If the result is more than 90 days before today, surface a staleness warning: "Domain `<domain>` last updated <date>."
3. Apply **judge: needs-intent-frame**. If it fires, emit an **Intent Frame** before advice.
4. Apply **judge: needs-interview**. If it fires, enter **Interview Workflow** before proposing a settled direction.
5. Brainstorm iteratively — suggest approaches, point out analogies, sketch concrete shapes for vague ideas.
6. When discussion changes unimplemented ticket phases, update them in place with user agreement.
7. Continue until the user signals done.

## On: Interview Workflow

1. Track an implicit decision tree: parent intent, current branch, unresolved child decisions.
2. Ask the highest-level unresolved question first; descend only after the parent branch is decided.
3. Ask one question per turn unless batching clearly reduces user burden.
4. When the user delegates remaining detail, close that child branch with an autonomous decision and return to the nearest unresolved parent branch.
5. Stop interviewing when the next useful action is a proposal, spec direction, ticket edit, skeleton directive, or implementation route.

## On: Ticket Status Transition

Triggers when the user requests a ticket status change — triaging an idea ticket to `todo/`, promoting a `todo/` ticket to `ready/`, or dropping a ticket to `.dropped/`.

1. Read the ticket file. Extract any `spec:` frontmatter field and body references to `{#YYMMDD-slug}` anchors.
2. **Triage (idea/ → todo/)**:
   a. Perform `git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md`.
   b. Do not require spec creation; `todo/` is accepted backlog, not the implementation queue.
3. **Ready promotion (todo/ → ready/)**:
   a. If category is `epic` or `research`, skip spec creation and spec frontmatter population.
   b. Otherwise, invoke `/write-spec` to add a `🚧` entry for each caller-visible behavior in the ticket.
   c. Invoke `ws:write-ticket` (Edit path) to populate the `spec:` frontmatter field when missing.
   d. Perform `git mv ai-docs/tickets/todo/<stem>.md ai-docs/tickets/ready/<stem>.md`.
   e. Add an entry to the `## Ticket Queue` section in `ai-docs/_index.md`. Format: `` `stem` — one-line purpose and dependency notes ``.
4. **Drop (→ .dropped/)**:
   a. For each linked spec stem: check whether any other non-dropped ticket also references it.
   b. No other ticket references this stem → invoke `/write-spec` to remove the `🚧` entry.
   c. Other tickets also reference this stem, or coverage is ambiguous → ask the user before removing.
   d. Perform `git mv ai-docs/tickets/<status>/<stem>.md ai-docs/tickets/.dropped/<stem>.md`.
4. Create one commit covering the `git mv` and any spec changes together.

## On: user signals done

1. Always suggest `/write-spec` as the next step — write-spec's `judge: spec-impact` decides whether spec work is needed and exits immediately if not.
2. Then offer ticket persistence:
   - **New ticket** — invoke `ws:write-ticket`.
   - **Ticket update** — invoke `ws:write-ticket`, then append design notes to an existing ticket phase.
3. Apply **judge: needs-integration-tests** to ticket writes.
4. Write only what the user approves. No artifact needed for exploratory discussions.

## Workflow Context

Interface and scope decisions made in discussion become downstream inputs:
- Approach direction → spec update (`/write-spec` — always the next step after discuss)
- Scope, phases, acceptance criteria → ticket structure (`/write-ticket`)
- Type shapes, module boundaries, public API → skeleton contract directives (`/write-skeleton`)
The canonical chain is: `/discuss` → `/write-spec` → `/write-ticket` → `/proceed` → `/write-skeleton`? → `/edit` | `/implement`.
Write-spec's judge handles the no-op case; the chain is uniform regardless of topic type.

When discussion converges on a decision in any of these categories, frame
the conclusion in terms its downstream consumer can directly act on.

## Judgments

### judge: needs-survey
Spawn `project-survey` when any of the following hold:
- The current question names a component, skill, agent, spec, or ticket whose doc has NOT been loaded in this session — regardless of whether the model feels confident it knows the answer.
- The discussion direction shifts to a domain no doc for which has been loaded this session.

Does NOT fire for session-continuity queries ("what were we doing?", "where were we?") — those draw from session state or git log.

### judge: needs-intent-frame
Emit an Intent Frame when the user message contains a proposal, evaluation, design direction, causal claim, scope assumption, or trade-off-heavy request.

Does NOT fire for mechanical commands, status checks, or implementation requests whose premises do not affect the chosen action.

### judge: needs-interview
Enter Interview Workflow when a decision branch remains open after the Intent Frame and the next answer depends on user priorities, scope boundaries, or trade-off weighting.

Do NOT interview when the user gave enough context for a proposal, when the remaining choices are local implementation details, or when a stated assumption is sufficient.

### judge: needs-integration-tests
Include integration-test criteria in a ticket phase when the change has end-to-end observable behavior. Skip for internal refactors.

## Templates

### Intent Frame

```text
I read this as:
- <claims, goals, constraints>

Premise check:
- <implicit premise> — fails if <condition>

Objectified:
- <neutral decision problem>

Considered:
- <viable interpretations or options>

Dropped:
- <rejected interpretations or options and why>

Stance:
- <agree | disagree | ambiguous | recommend X>
```

## Doctrine

This skill optimizes for **decision quality per conversation turn**. The user is here to think, not to produce artifacts — so the agent's job is to sharpen reasoning by surfacing risks, reuse opportunities, and concrete alternatives, then capture only what the user approves. When a rule is ambiguous, apply whichever interpretation better preserves decision quality per turn.
