---
name: lead-sprint
description: Use when the user wants an ongoing wsflow sprint session for discussion, exploration, small interactive edits, and normal workflow handoff.
---

# Sprint

Target: user request

## Project Map

Call `wsflow/project_tree()`.

## Invariants

Scope
- Stay on the current branch; never create or require `sprint/` branches.
- Keep `lead-sprint` responsible for routing, session continuity, and episode closure.
- Route general implementation through `wsflow:lead-proceed` or `wsflow:lead-implement`; do not weaken their gates.
- Allow `sprint-edit` only for one-context, lead-owned, small interactive edits.

Episodes
- Keep one active `sprint-edit` episode per edit context.
- Add both `Sprint-Edit:` and `Sprint-Edit-Context:` lines to every sprint-edit commit body.
- Run documentation closure when an episode wraps; do not batch one final sprint wrap-up.
- Return to the sprint loop after every route, handoff, or episode closure.

Language
- Render fixed English prompt templates in the user's active language.
- All written artifacts are English.

## On: invoke

1. Invoke `wsflow:lead-workflow-manual`.
2. Call `wsflow/git.status()`.
3. Call `wsflow/project_tree()`.
4. Initialize session state: `<current-edit-context>`, `<episode-slug>`, and `<episode-start>` are empty.
5. Enter session loop.

## On: session loop

1. Accept user request.
2. If a `sprint-edit` episode is active and the request answers the post-edit question, apply **Post-Edit Reply Routing**.
3. Otherwise apply `judge: route-request`; execute the first matching route.
4. Return to step 1.

## On: sprint-edit

Trigger: `judge: route-request` selects `Start or continue sprint-edit`.

1. Apply `judge: sprint-edit`; if it fails, route through normal workflow instead.
2. If no episode is active, set `<current-edit-context>` to a one-line context, set `<episode-slug>` to a short kebab-case slug, and set `<episode-start>` with `git rev-parse HEAD`.
3. Invoke `wsflow:lead-edit` only for a lead-owned direct edit; if the edit needs subagent implementation, stop and route through normal workflow instead.
4. Run focused verification; read full output before claiming pass.
5. Commit the edit with normal commit message content plus both marker lines:

```text
Sprint-Edit: <episode-slug>
Sprint-Edit-Context: <one-line context>
```

6. Ask, in the user's active language:

```text
[sprint] Should we keep refining <current edit context>, wrap it up here, or shift direction?
```

7. Return to session loop.

## On: wrap episode

Trigger: post-edit reply means wrap it up, done, or good.

1. Confirm `<episode-slug>` and `<episode-start>` exist; otherwise report that no active sprint-edit episode is open.
2. Set `<episode-range>` to `<episode-start>..HEAD`.
3. Verify the range contains commits with `Sprint-Edit: <episode-slug>`.
4. Invoke `wsflow:lead-update-spec` with `<episode-range>`.
5. Review changed source and update mental-model documents directly when module contracts, coupling, extension points, common mistakes, or technical debt changed.
6. Call `wsflow/infra.read(name: "executor-wrapup")`; follow Doc Pipeline and Doc Commit Gate for episode-scoped docs only.
7. Clear `<current-edit-context>`, `<episode-slug>`, and `<episode-start>`.
8. Report episode commits, documentation updates, and verification.
9. Return to session loop.

## On: shift direction

Trigger: post-edit reply means shift direction or change focus.

1. If an episode is active, ask whether to wrap the current episode before starting the new direction.
2. If the user chooses wrap, run **On: wrap episode** first.
3. If the user chooses leave open, preserve `<current-edit-context>`, `<episode-slug>`, and `<episode-start>` until the user returns or explicitly abandons it.
4. Route the new request through the session loop.

## Judgments

### judge: route-request

Pick first match, execute, return to loop.

| Request type | Routing |
|---|---|
| Behavior, concept, or status question | Answer inline; use direct search, wsflow read tools, or scoped subagents when codebase context is needed |
| Codebase exploration | Use direct local search or a scoped subagent |
| Design discussion | Discuss inline; do not auto-chain to spec or ticket authoring |
| Ticket, spec, or protocol change | Route through normal workflow; usually `wsflow:lead-proceed` when implementation is requested |
| One-context small interactive edit | Start or continue sprint-edit |
| Larger implementation, public contract work, cross-module change, new pattern, or review-worthy work | Continue through `wsflow:lead-proceed` or `wsflow:lead-implement` |
| Ambiguous request | Ask the smallest routing question, then re-apply judge |

### judge: sprint-edit

Allow only when every condition is true:

- one active edit context is enough to hold the whole change;
- the lead can keep the edit direct and verify inline without subagent implementation;
- no public contract, protocol, ticket phase, branch, or routing semantics change;
- no cross-module new pattern, plan allocation, or review allocation is needed;
- failure can be safely resolved or reverted before leaving the episode.

If any condition is false, route through normal workflow.

### Post-Edit Reply Routing

| Reply intent | Action |
|---|---|
| keep refining or continue | Keep the current episode active; run **On: sprint-edit** for the next edit |
| wrap it up, done, or good | Run **On: wrap episode** |
| shift direction or change focus | Run **On: shift direction** |
| unclear | Ask the fixed `[sprint]` question again with the current context |

## Doctrine

Sprint optimizes for **session continuity across exploratory workflow turns**. It keeps the lead oriented by preserving a lightweight loop, lets tiny interactive edits close as recoverable episodes, and routes anything larger through the normal workflow before sprint convenience can weaken implementation discipline.
