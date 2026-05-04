---
name: sprint
description: Multi-task session container for feature-branch work. Defers the doc pipeline to wrap-up while each task commits only source changes.
---

# Sprint Session Container

Target: user request

## Project Map

Call `ws/project_tree()`.

## Invariants

- Sprint operates only on `sprint/`-prefixed branches; do not enter the loop or run wrap-up elsewhere.
- Doc pipeline is suppressed during task execution; it runs once at wrap-up only.
- All written artifacts must be in English regardless of conversation language.
- At wrap-up, commit each doc update immediately after it completes.

## On: invoke

1. Invoke `ws:workflow`.
2. Read `git branch --show-current`.
3. If branch starts with `sprint/`: detect sprint name from branch. Present options: continue, wrap-up, abandon.
4. If branch does not start with `sprint/`: infer a branch name without asking.
   - Clear topic exists: derive a short kebab-case slug from it.
   - Topic is vague or absent: generate a random three-word name in `<adjective>-<noun>-<noun>` form.
   - Run `git checkout -b sprint/<name>`. Enter session loop.

## On: session loop

1. Apply `judge: needs-survey`; if warranted, run the Sprint-Aware Survey Call and incorporate the returned tier list.
2. Accept user request.
3. Apply `judge: delegate`; route and execute per the routing table.
4. Return to step 2.

## On: wrap-up

Triggers on explicit user done signal: "done", "wrap up", "finish sprint", or equivalent.

1. Determine parent: `git merge-base HEAD main`.
2. Invoke `ws:update-spec` with args `<parent>..HEAD`.
3. Call `ws/agents.register(name: "mental-model-updater", prompts: ["mental-model-updater"])`, then call `ws/agents.call(name: "mental-model-updater", prompt: <block below>)`.
4. Wait for completion. Commit any file changes. Run after update-spec so the updater sees updated spec entries.
5. Call `ws/infra.read(name: "executor-wrapup")`. Follow Doc Pipeline and Doc Commit Gate. If ticket-driven, update existing tickets only; do not create new tickets.
6. Report to user: spec entries added, removed, and implemented markers stripped; mental-model sections updated.
7. Merge: merge `sprint/<name>` to `main` using the repository merge helper or equivalent non-interactive git sequence. If no source changes were made, skip merge and delete the branch.

```text
Commit range: <parent>..HEAD.
Note: docs may be stale from accumulated sprint commits - explore thoroughly.
```

## Judgments

### judge: delegate

Pick the first matching row, execute it, and return to the session loop.

| Request type | Routing |
|---|---|
| Question about behavior, concept, or status | Answer inline; call `ws/subquery(question: <block below>)` if codebase search is needed |
| Codebase exploration | Call `ws/subquery(question: <block below>)` |
| Design discussion | Inline discussion; do not auto-chain to `ws:write-spec` |
| Single-file edit or clear isolated change | Invoke `ws:edit` |
| Multi-file or new-pattern implementation | Invoke `ws:write-code` |
| Exploration required before routing is possible | Run sprint-aware survey; re-apply judge |

### judge: needs-survey

Fire the Sprint-Aware Survey Call when:

- Session loop is entered for the first time this session.
- Request touches a domain or component not yet surveyed this session.
- Domain shifts mid-session.

Do not fire for follow-up turns within an established domain, or for status / continuity queries.

## Templates

### Sprint-Aware Survey Call

Call `ws/agents.register(name: "sprint-survey", prompts: ["sprint-survey"])`, then call `ws/agents.call(name: "sprint-survey", prompt: <block below>)`.

```text
Sprint: <sprint-name>
Branch: <branch>
Commit range: <parent>..HEAD
Commits:
<git log <parent>..HEAD --oneline>

Project map:
<ws/project_tree() output>
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature branch** -
by deferring the doc pipeline to a single wrap-up pass and delegating
implementation to write-code and edit primitives, the session maintains momentum
without accumulating documentation debt or managing internal agent state. When a
rule is ambiguous, apply whichever interpretation better preserves throughput.
