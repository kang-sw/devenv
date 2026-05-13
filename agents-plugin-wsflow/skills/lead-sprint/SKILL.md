---
name: lead-sprint
description: Use when the user asks for a wsflow sprint or multi-task branch session; defers documentation until wrap-up while source changes route through direct edit.
---

# Sprint

Target: user request

## Project Map

Call `wsflow/project_tree()`.

## Invariants

- Operate only on `sprint/` branches; create, continue, wrap up, or abandon explicitly.
- Suppress documentation pipeline during task execution; run one wrap-up pass.
- Execute source changes through `wsflow:lead-edit`; `lead-edit` chooses direct or subagent implementation.
- Use subagents for bounded exploration, implementation support, verification, audit, or review.
- Keep wrap-up integration lead-owned: specs, mental models, tickets, index, commits, merge, and cleanup.
- Commit each logical source task before returning to the sprint loop.
- All written artifacts are English.

## On: invoke

1. Invoke `wsflow:lead-workflow-manual`.
2. Call `wsflow/project_tree()`.
3. Call `wsflow/git.status()`.
4. On `sprint/` branch: detect sprint name; present continue, wrap-up, abandon.
5. Outside `sprint/`: infer name without asking, then create `sprint/<name>`.
   - Clear topic: short kebab-case slug.
   - Vague or absent topic: random `<adjective>-<noun>-<noun>`.
6. Enter session loop.

## On: session loop

1. Apply `judge: needs-survey`; run Sprint-Aware Survey when warranted.
2. Accept user request.
3. Apply `judge: route-task`; execute the first matching route.
4. Return to step 2.

## On: wrap-up

Trigger: explicit done signal such as "done", "wrap up", or "finish sprint".

1. Confirm current branch is `sprint/<name>`.
2. Set `<parent>` through `wsflow/git.merge_base(base: "main", head: "HEAD")`.
3. Invoke `wsflow:lead-update-spec` with `<parent>..HEAD`.
4. Review changed source and update mental-model documents directly when module contracts, coupling, extension points, common mistakes, or technical debt changed.
5. Call `wsflow/infra.read(name: "executor-wrapup")`; follow Doc Pipeline and Doc Commit Gate.
6. If ticket-driven, update existing tickets only; do not create new tickets during wrap-up.
7. Run relevant verification commands after documentation updates.
8. Report source commits, spec changes, mental-model changes, ticket/index updates, and verification.
9. If source changes exist, merge `sprint/<name>` to `main` with a non-interactive Git sequence after user approval.
10. If no source changes exist, skip merge and delete the branch after user approval.

## On: abandon

1. Confirm current branch is `sprint/<name>`.
2. Report unmerged commits with `wsflow/git.log(range: "main..HEAD")`.
3. Ask for explicit approval before deleting the branch or discarding work.
4. Do not move tickets or edit docs unless the user explicitly requests cleanup.

## Judgments

### judge: route-task

Pick first match, execute, return to loop.

| Request type | Routing |
|---|---|
| Behavior, concept, or status question | Answer inline; use direct search, wsflow read tools, or Sprint-Aware Survey when codebase context is needed |
| Codebase exploration | Run Sprint-Aware Survey |
| Design discussion | Discuss inline; do not auto-chain to spec or ticket authoring |
| Source change | Invoke `wsflow:lead-edit` |
| Ticket, spec, or mental-model maintenance | Handle directly only when the user explicitly asks; otherwise defer to wrap-up |
| Exploration required before routing | Run Sprint-Aware Survey; re-apply judge |

### judge: needs-survey

Fire when:

- first session-loop entry this session;
- request touches an unsurveyed domain or component;
- domain shifts mid-session.

Skip for follow-ups in an established domain, status, or continuity queries.

## Templates

### Sprint-Aware Survey

Use direct local search or a scoped subagent.

```text
Sprint: <sprint-name>
Branch: <branch>
Commit range: <parent>..HEAD
Commits:
<wsflow/git.log(range: "<parent>..HEAD") output>

Project map:
<wsflow/project_tree() output>

Question:
<focused read-only question>

Return:
- relevant files or docs
- existing implementation or reusable patterns
- risks that affect this sprint
- next routing recommendation
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature
branch**. It keeps throughput by batching documentation to wrap-up and routing
source tasks through `lead-edit`, while using scoped subagents when they improve
exploration, implementation, or review. When ambiguous, preserve branch
continuity without losing wrap-up accountability.
