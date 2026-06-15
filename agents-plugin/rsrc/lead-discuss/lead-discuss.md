---
kind: print
delegates: true
---

# Discuss

Topic: user request

## Invariants

Scope
- No source edits during discussion.
- Documentation writes are allowed only in Capture, Ticket Status Transition, or user-approved persistence handlers.
- With user agreement, unimplemented ticket phases may be edited during Capture to keep the ticket accurate. Phase plan text before a `### Result` is frozen after completion; append a `#### Edition` for later implementation tweaks.

Evidence
- Read mental-model docs on-demand as topics emerge.
- Read spec docs in `ai-docs/spec/` on-demand as topics emerge; the project map lists available specs.
- Use direct host-native exploration-worker dispatch (see `lead-workflow-manual`) for focused implementation-detail questions beyond mental-model docs; read the result before responding.
- When docs are stale or insufficient, say so - do not speculate.
- Before proposing new abstractions, surface existing patterns or components that already solve part of the problem.

Conversation
- Act like a careful senior engineer: stress-test premises, trade-offs, and failure modes before endorsing a direction.
- Evaluate each claim independently - call out unaddressed risks with reasoning; do not parrot back risks already discussed and resolved.
- Use the user's active conversation language for discussion responses.
- Intent frames summarize decision rationale; they do not expose raw hidden reasoning.
- Never proactively ask to wrap up or persist; wait for the user's explicit signal.

Response
- Lead with the load-bearing point before options, caveats, or history.
- Keep each actionable claim adjacent to its evidence, gap, or assumption label.
- Put user decisions and next actions immediately after the fact that motivates them.
- Prefer a concise stance plus the strongest caveat over exhaustive option dumps.
- If evidence is incomplete, label the gap and next lookup instead of filling it with inference.

## On: invoke

1. Call `ws/playbook.print(name: "lead-workflow-manual")` and execute the returned reference inline (loads orchestration primitives reference).
2. Call `ws/project_tree()` to load the current project map.
3. Call `ws/git.status()`.
4. If `user request` references a ticket, read it.
5. Enter user-message handling.

## On: user message

### 1. Gather Context

1. Apply `judge: needs-survey` to every named component, skill, agent, spec, or ticket.
   For each unloaded doc, run `reference-discovery` and incorporate its returned reference list before responding.
2. Read mental-model docs for touched domains; read spec docs for external-visible behavior; use direct host-native exploration-worker dispatch (see `lead-workflow-manual`) for focused implementation details.
   For mental-model staleness, use native path-filtered Git history until ws exposes a path-history primitive.

### 2. Route Intent

1. If the user explicitly wants implementation to start, hand off to `ws:lead-proceed` and stop the discuss handler after that procedure takes over.
2. Apply `judge: needs-intent-frame`; if it fires, emit an Intent Frame before advice.
3. Apply `judge: needs-interview`; if it fires, enter Interview Workflow before proposing a settled direction.

### 3. Respond

1. Shape the reply as load-bearing point -> evidence or gap -> user decision or next action.
2. Brainstorm iteratively: suggest approaches, point out analogies, sketch concrete shapes for vague ideas.
3. Answer bounded requests directly; continue discussion only while the user keeps asking follow-up questions.

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
   b. Do not require spec creation; `todo/` is accepted backlog, not implementation-ready status.
3. **Ready promotion (todo/ -> ready/)**:
   a. Call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline (Edit path) for the `todo/` -> `ready/` promotion.
   b. The lead-write-ticket procedure owns spec addressing, frontmatter population, the `git mv`, focus update, and commit.
   c. Stop this handler after the lead-write-ticket procedure returns.
4. **Drop (-> .dropped/)**:
   a. For each linked spec stem: check whether any other non-dropped ticket also references it.
   b. No other ticket references this stem -> call `ws/playbook.print(name: "lead-write-spec")` and execute the returned procedure inline to remove the `🚧` entry.
   c. Other tickets also reference this stem, or coverage is ambiguous -> ask the user before removing.
   d. Perform native `git mv ai-docs/tickets/<status>/<stem>.md ai-docs/tickets/.dropped/<stem>.md`.
5. Commit through `ws/git.commit`.

## On: user signals done

1. If the user wants implementation to start, hand off to `ws:lead-proceed` and stop the discuss handler after that procedure takes over.
2. If the user explicitly asks to persist the discussion, route by requested artifact:
   - **Spec update** - call `ws/playbook.print(name: "lead-write-spec")` and execute the returned procedure inline.
   - **New ticket** - call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline.
   - **Ticket update** - call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline, using its Edit path to append approved design notes to the existing ticket phase.
3. If persistence artifact is unclear, ask one clarifying question and stop.
4. When ticket persistence creates or edits an implementation phase, apply **judge: needs-integration-tests** and include criteria only when the judged change has end-to-end observable behavior.
5. Write only what the user approves.
6. If no artifact is written, respond with the current conclusion, any unresolved decision, and that no files were changed.

## Judgments

### judge: needs-survey
Spawn `reference-discovery` when any of the following hold:
- The current question names a component, skill, agent, spec, or ticket whose doc has NOT been loaded in this session - regardless of whether the model feels confident it knows the answer.
- The discussion direction shifts to a domain no doc for which has been loaded this session.

Does NOT fire for session-continuity queries ("what were we doing?", "where were we?") - those draw from session state or `ws/git.log`.

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
