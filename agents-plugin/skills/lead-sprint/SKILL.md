---
name: lead-sprint
description: Multi-task session container for feature-branch work. Defers the doc pipeline to wrap-up while each task commits only source changes.
---

# Sprint

Target: user request

## Project Map

Call `ws/project_tree()`.

## Invariants

- Operate only on `sprint/` branches; do not loop or wrap up elsewhere.
- Suppress doc pipeline during tasks; run it once at wrap-up.
- Commit each wrap-up doc update immediately after it completes.
- All written artifacts are English.

## On: invoke

1. Invoke `ws:lead-workflow-manual`.
2. Call `ws/git.status()`.
3. On `sprint/` branch: detect sprint name; present continue, wrap-up, abandon.
4. Outside `sprint/`: infer name without asking, then `git checkout -b sprint/<name>`.
   - Clear topic: short kebab-case slug.
   - Vague/absent topic: random `<adjective>-<noun>-<noun>`.
5. Enter session loop.

## On: session loop

1. Apply `judge: needs-survey`; run Sprint-Aware Survey Call when warranted.
2. Accept user request.
3. Apply `judge: delegate`; execute the first matching route.
4. Return to step 2.

## On: wrap-up

Trigger: explicit done signal such as "done", "wrap up", or "finish sprint".

1. Set `<parent>` through `ws/git.merge_base(base: "main", head: "HEAD")`.
2. Invoke `ws:lead-update-spec` with `<parent>..HEAD`; commit changes.
3. Register `mental-model-updater`: `ws/agents.register(name: "mental-model-updater", prompts: ["mental-model-updater"])`.
4. Call it with the wrap-up prompt below; wait; commit changes.
5. Call `ws/infra.read(name: "executor-wrapup")`; follow Doc Pipeline and Doc Commit Gate.
6. If ticket-driven, update existing tickets only; do not create new tickets.
7. Report spec entries added/removed/stripped and mental-model updates.
8. Merge `sprint/<name>` to `main` with repository helper or equivalent non-interactive git sequence.
9. If no source changes exist, skip merge and delete the branch.

```text
Commit range: <parent>..HEAD.
Note: docs may be stale from accumulated sprint commits - explore thoroughly.
```

## Judgments

### judge: delegate

Pick first match, execute, return to loop.

| Request type | Routing |
|---|---|
| Behavior, concept, or status question | Answer inline; use `ws/subquery(question: <block below>)`, then `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`, if codebase search is needed |
| Codebase exploration | Call `ws/subquery(question: <block below>)`, then `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)` |
| Design discussion | Discuss inline; do not auto-chain to `ws:lead-write-spec` |
| Single-file edit or clear isolated change | Invoke `ws:lead-edit` |
| Multi-file or new-pattern implementation | Invoke `ws:lead-write-code` |
| Exploration required before routing | Run sprint-aware survey; re-apply judge |

### judge: needs-survey

Fire when:

- first session-loop entry this session;
- request touches an unsurveyed domain/component;
- domain shifts mid-session.

Skip for follow-ups in an established domain, status, or continuity queries.

## Templates

### Sprint-Aware Survey Call

Call `ws/agents.register(name: "sprint-survey", prompts: ["sprint-survey"])`,
then `ws/agents.call(name: "sprint-survey", prompt: <block below>)`.

```text
Sprint: <sprint-name>
Branch: <branch>
Commit range: <parent>..HEAD
Commits:
<ws/git.log(range: "<parent>..HEAD") output>

Project map:
<ws/project_tree() output>
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature
branch**. It keeps task execution moving by delegating to edit/write-code and
deferring documentation to one wrap-up pass. When a rule is ambiguous, apply
whichever interpretation better preserves throughput without losing wrap-up
accountability.
