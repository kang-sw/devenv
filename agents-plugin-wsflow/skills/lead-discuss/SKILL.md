---
name: lead-discuss
description: Use before code changes when the user wants to explore workflow design, migration direction, ticket scope, risks, or implementation approach.
---

# Discuss

Topic: user request

## Invariants

Scope
- No source edits. Only documentation writes, only in the capture step.
- Exception: unimplemented ticket phases may be edited mid-discussion to keep the ticket accurate. Phase plan text before a `### Result` is frozen after completion; append a `#### Edition` for later implementation tweaks.

Evidence
- Read mental-model docs on-demand as topics emerge.
- Read spec docs in `ai-docs/spec/` on-demand as topics emerge; the project map lists available specs.
- Use direct project search or subagent exploration for implementation details beyond mental-model docs; read the result before responding.
- When docs are stale or insufficient, say so - do not speculate.
- Before proposing new abstractions, surface existing patterns or components that already solve part of the problem.

Conversation
- Evaluate each claim independently - call out unaddressed risks with reasoning; do not parrot back risks already discussed and resolved.
- Use the user's active conversation language for discussion responses.
- Intent frames summarize decision rationale; they do not expose raw hidden reasoning.
- Never proactively ask to wrap up or persist; wait for the user's explicit signal.

## On: invoke

1. Invoke `wsflow:lead-workflow-manual` via Skill tool (loads orchestration primitives reference).
2. Call `wsflow/project_tree()` to load the current project map.
3. Call `wsflow/git.status()`. If the current branch starts with `sprint/`, emit: "Note: sprint branch `<branch-name>` detected - route sprint-scoped implementation through `wsflow:lead-sprint` or ask for an explicit non-sprint target branch."
4. If `user request` references a ticket, read it.
5. Enter user-message handling.

## On: user message

### 1. Gather Context

1. Apply `judge: needs-survey` to every named component, skill, spec, or ticket.
   For each unloaded doc, run a bounded survey through project tools, direct file reads, or subagent exploration; incorporate the returned reference list before responding.
2. Read mental-model docs for touched domains; read spec docs for external-visible behavior; use direct project search or subagent exploration for implementation details.
   For mental-model staleness, use native path-filtered Git history when no wsflow path-history primitive exists.

### 2. Route Intent

1. If the user explicitly wants implementation to start, continue through `wsflow:lead-proceed`; carry the current target and settled discussion context.
2. Apply `judge: needs-intent-frame`; if it fires, emit an Intent Frame before advice.
3. Apply `judge: needs-interview`; if it fires, enter Interview Workflow before proposing a settled direction.

### 3. Respond

1. Brainstorm iteratively: suggest approaches, point out analogies, sketch concrete shapes for vague ideas.
2. Continue until the user signals done.

### 4. Capture

1. When discussion changes unimplemented ticket phases, update them in place with user agreement.

## On: Interview Workflow

1. Track an implicit decision tree: parent intent, current branch, unresolved child decisions.
2. Ask the highest-level unresolved question first; descend only after the parent branch is decided.
3. Ask one question per turn unless batching clearly reduces user burden.
4. When the user delegates remaining detail, close that child branch with an autonomous decision and return to the nearest unresolved parent branch.
5. Stop interviewing when the next useful action is a proposal, spec direction, ticket edit, or implementation route.

## On: Ticket Status Transition

Triggers when the user requests a ticket status change - triaging an idea ticket to `todo/`, promoting a `todo/` ticket to `ready/`, or dropping a ticket to `.dropped/`.

1. Read the ticket file. Extract any `spec:` frontmatter field and body references to `{#YYMMDD-slug}` anchors.
2. **Triage (idea/ -> todo/)**:
   a. Perform native `git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md`.
   b. Do not require spec creation; `todo/` is accepted backlog, not the implementation queue.
3. **Ready promotion (todo/ -> ready/)**:
   a. Invoke `wsflow:lead-write-ticket` (Edit path) for the `todo/` -> `ready/` promotion.
   b. `wsflow:lead-write-ticket` owns spec coverage, frontmatter population, the `git mv`, queue update, and commit.
   c. Stop this handler after `wsflow:lead-write-ticket` returns.
4. **Drop (-> .dropped/)**:
   a. For each linked spec stem: check whether any other non-dropped ticket also references it.
   b. No other ticket references this stem -> invoke `wsflow:lead-write-spec` to remove the `🚧` entry.
   c. Other tickets also reference this stem, or coverage is ambiguous -> ask the user before removing.
   d. Perform native `git mv ai-docs/tickets/<status>/<stem>.md ai-docs/tickets/.dropped/<stem>.md`.
5. Commit through `wsflow/git.commit`.

## On: user signals done

1. If the user wants implementation to start, continue through `wsflow:lead-proceed`; carry the current target and settled discussion context.
2. For persistence without implementation, suggest `wsflow:lead-write-spec` as the next route; that skill owns whether spec changes are needed.
3. Then offer ticket persistence:
   - **New ticket** - invoke `wsflow:lead-write-ticket`.
   - **Ticket update** - invoke `wsflow:lead-write-ticket`, then append design notes to an existing ticket phase.
4. Apply **judge: needs-integration-tests** to ticket writes.
5. Write only what the user approves. No artifact needed for exploratory discussions.

## Context To Carry

Discussion outputs feed downstream skills:
- Continue through `wsflow:lead-write-spec`; carry approach direction.
- Continue through `wsflow:lead-write-ticket`; carry scope, phases, and acceptance criteria.
- Continue through `wsflow:lead-proceed`; carry implementation intent and settled discussion context.
- Carry type shapes, module boundaries, and public API notes into implementation routing.

Frame conclusions as directives the downstream consumer can execute.

## Judgments

### judge: needs-survey
Run a bounded survey when any of the following hold:
- The current question names a component, skill, spec, or ticket whose doc has NOT been loaded in this session - regardless of whether the model feels confident it knows the answer.
- The discussion direction shifts to a domain no doc for which has been loaded this session.

Does NOT fire for session-continuity queries ("what were we doing?", "where were we?") - those draw from session state or `wsflow/git.log`.

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
[reading]
- <claims, goals, constraints>

[check]
<implicit premise and failure condition>

[problem]
<neutral decision problem>

[options]
<viable interpretations or options>

[excluded]
<rejected interpretations or options and why>

[stance]
<agree | disagree | ambiguous | recommend X>
```

## Doctrine

This skill optimizes for **decision quality per conversation turn**. Sharpen
reasoning with risks, reuse opportunities, and concrete alternatives; capture
only what the user approves. When ambiguous, preserve decision quality per turn.
