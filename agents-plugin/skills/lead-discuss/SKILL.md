---
name: lead-discuss
description: Explore a workflow design, migration direction, ticket scope, or implementation approach before making code changes. Use when the user wants to reason through options, risks, or next steps rather than immediately edit files.
---

# Discuss

Topic: user request

## Project Map

Call `ws/project_tree()` to load the current project map.

## Invariants

- No source edits. Only documentation writes, only in the capture step.
- Exception: unimplemented ticket phases may be edited mid-discussion to keep the ticket accurate. Phases with a `### Result` section are frozen - do not edit them.
- Read mental-model docs on-demand as topics emerge.
- Read spec docs in `ai-docs/spec/` on-demand as topics emerge; Project Map lists available specs.
- Use `ws/subquery(question: "<focused implementation-detail question>")`, then `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`, for implementation details beyond mental-model docs.
- When docs are stale or insufficient, say so - do not speculate.
- Before proposing new abstractions, surface existing patterns or components that already solve part of the problem.
- Evaluate each claim independently - call out unaddressed risks with reasoning; do not parrot back risks already discussed and resolved.
- Never proactively ask to wrap up or persist; wait for the user's explicit signal.
- All written artifacts must be in English regardless of conversation language.

## On: invoke

1. Invoke `ws:lead-workflow` via Skill tool (loads orchestration primitives reference).
2. Call `ws/git.status()`. If the current branch starts with `sprint/`, emit: "Note: sprint branch `<branch-name>` detected - `ws:lead-sprint` provides session continuity."
3. If `user request` references a ticket, read it.
4. Enter discussion loop.

## On: discussion loop

1. Apply **judge: needs-survey** to every named component, skill, agent, spec, or ticket.
   For each unloaded doc, call `ws/agents.register(name: "project-survey", prompts: ["project-survey"])`, then `ws/agents.call(name: "project-survey", prompt: "<topic brief>")`.
   Incorporate the returned reference list before responding.
2. Brainstorm iteratively - suggest approaches, point out analogies, sketch concrete shapes for vague ideas.
3. Read mental-model docs for touched domains; read spec docs for external-visible behavior; use `ws/subquery(question: "<focused implementation-detail question>")`, then `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`, for implementation details.
   For mental-model staleness, use native path-filtered Git history until ws exposes a path-history primitive.
4. When discussion changes unimplemented ticket phases, update them in place with user agreement.
5. Continue until the user signals done.

## On: Ticket Status Transition

Triggers when the user requests a ticket status change - promoting an idea ticket to `todo/`, or dropping a ticket to `.dropped/`.

1. Read the ticket file. Extract any `spec:` frontmatter field and body references to `{#YYMMDD-slug}` anchors.
2. **Promotion (idea/ -> todo/)**:
   a. Invoke `ws:lead-write-spec` to add a `🚧` entry for each caller-visible behavior in the ticket.
   b. Perform native `git mv ai-docs/tickets/idea/<stem>.md ai-docs/tickets/todo/<stem>.md`.
   c. Invoke `ws:lead-write-ticket` (Edit path) on the promoted ticket to populate the `spec:` frontmatter field with the stems created in step (a).
   d. Add an entry to the `## Ticket Queue` section in `ai-docs/_index.md`. Format: `` `stem` - one-line purpose and dependency notes ``.
3. **Drop (-> .dropped/)**:
   a. For each linked spec stem: check whether any other non-dropped ticket also references it.
   b. No other ticket references this stem -> invoke `ws:lead-write-spec` to remove the `🚧` entry.
   c. Other tickets also reference this stem, or coverage is ambiguous -> ask the user before removing.
   d. Perform native `git mv ai-docs/tickets/<status>/<stem>.md ai-docs/tickets/.dropped/<stem>.md`.
4. Commit through `ws/git.commit`.

## On: user signals done

1. Always suggest `ws:lead-write-spec` as the next step - write-spec's `judge: spec-impact` decides whether spec work is needed and exits immediately if not.
2. Then offer ticket persistence:
   - **New ticket** - invoke `ws:lead-write-ticket`.
   - **Ticket update** - invoke `ws:lead-write-ticket`, then append design notes to an existing ticket phase.
3. Apply **judge: needs-integration-tests** to ticket writes.
4. Write only what the user approves. No artifact needed for exploratory discussions.

## Workflow Context

Discussion outputs feed downstream skills:
- Approach direction -> `ws:lead-write-spec` (always next; its judge handles no-op)
- Scope, phases, acceptance criteria -> `ws:lead-write-ticket`
- Type shapes, module boundaries, public API -> `ws:lead-write-skeleton`

Canonical chain: `ws:lead-discuss` -> `ws:lead-write-spec` -> `ws:lead-write-ticket` -> `ws:lead-proceed` -> `ws:lead-write-skeleton`? -> `ws:lead-edit` | `ws:lead-write-code`.
Frame conclusions as directives the downstream consumer can execute.

## Judgments

### judge: needs-survey
Spawn `project-survey` when any of the following hold:
- The current question names a component, skill, agent, spec, or ticket whose doc has NOT been loaded in this session - regardless of whether the model feels confident it knows the answer.
- The discussion direction shifts to a domain no doc for which has been loaded this session.

Does NOT fire for session-continuity queries ("what were we doing?", "where were we?") - those draw from session state or `ws/git.log`.

### judge: needs-integration-tests
Include integration-test criteria in a ticket phase when the change has end-to-end observable behavior. Skip for internal refactors.

## Doctrine

This skill optimizes for **decision quality per conversation turn**. Sharpen
reasoning with risks, reuse opportunities, and concrete alternatives; capture
only what the user approves. When ambiguous, preserve decision quality per turn.
