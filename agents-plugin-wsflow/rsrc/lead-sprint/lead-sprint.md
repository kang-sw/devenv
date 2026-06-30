---
kind: print
delegates: true
---

# Sprint

Target: user request

## Project Map

Call `{{.McpNamespace}}/project_tree()`.

## Invariants

Scope
- Stay on the current branch; never create or require `sprint/` branches.
- Keep `lead-sprint` responsible for routing, session continuity, and episode closure.
- Route general implementation through `{{.SkillNamespace}}:lead-proceed` or the lead-implement procedure; do not weaken their gates.
- Allow `sprint-edit` only for one-context, lead-owned, small interactive edits.
- Ticket creation must route through `lead-write-ticket` (which calls `ws/tickets.create`); do not create ticket files directly via `convention.read` + `Write`.
- Routing preference disambiguation: requests to "save a preference" or "remember a setting" route to `{{.SkillNamespace}}:lead-tune` (ws workflow preferences). `{{.SkillNamespace}}:lead-add-rule` is for repo-level coding/architecture rules only.

Episodes
- Keep one active `sprint-edit` episode per edit context.
- Add both `Sprint-Edit:` and `Sprint-Edit-Context:` lines to every sprint-edit commit body.
- Run documentation closure when an episode wraps; do not batch one final sprint wrap-up.
- Return to the sprint loop after every route, handoff, or episode closure.

Language
- Render fixed English prompt templates in the user's active language.
- All written artifacts are English.

## On: invoke

1. Call `{{.McpNamespace}}/workflow_manual(session_key: <your lead key>)` and execute the returned reference inline; reload after session compaction (a duplicate load is safe). After compaction, recover your key via `{{.SkillNamespace}}:lead-revive` first. No lead key yet (fresh start)? Call `{{.McpNamespace}}/workflow_manual(session_key: "obsidian-latch")` to bootstrap.
2. Call `{{.McpNamespace}}/git.status()`.
3. Call `{{.McpNamespace}}/project_tree()`.
4. Recover episode state from active conversation or recent `Sprint-Edit:` commit markers.
5. If recovery finds one open episode, set `<current-edit-context>`, `<episode-slug>`, and `<episode-start>` from it.
6. If recovery is empty or ambiguous, initialize `<current-edit-context>`, `<episode-slug>`, and `<episode-start>` as empty.
7. Enter session loop.

## On: session loop

1. Accept user request.
2. If a `sprint-edit` episode is active and the request answers the post-edit question, apply **Post-Edit Reply Routing**.
3. Otherwise apply `judge: route-request`; execute the first matching route.
4. Return to step 1.

## On: recover episode

1. Prefer active conversation state when it names an open `sprint-edit` episode.
2. Otherwise inspect recent commits for `Sprint-Edit:` and `Sprint-Edit-Context:` markers.
3. Treat the newest marker group with no later episode documentation closure as the open episode.
4. Set `<episode-start>` to the parent of that group's first marked commit.
5. If multiple marker groups could be open, leave state empty and report the ambiguity before routing.

## On: sprint-edit

Trigger: `judge: route-request` selects `Start or continue sprint-edit`.

1. Apply `judge: sprint-edit`; if it fails, route through normal workflow instead.
2. If no episode is active, set `<current-edit-context>` to a one-line context, set `<episode-slug>` to a short kebab-case slug, and set `<episode-start>` with `git rev-parse HEAD`.
3. If a new episode was just initialized, call `{{.McpNamespace}}/enter.sprint(session_key: <lead key>, episode_slug: <episode-slug>, episode_start: <episode-start>, current_edit_context: <current-edit-context>)` to record the episode so it is recoverable before its first commit.
4. Edit directly in the lead session; do not delegate implementation.
5. Run focused verification; read full output before claiming pass.
6. Commit the edit with normal commit message content plus both marker lines:

```text
Sprint-Edit: <episode-slug>
Sprint-Edit-Context: <one-line context>
```

7. Ask, in the user's active language:

```text
[sprint] Should we keep refining <current edit context>, wrap it up here, or shift direction?
```

8. Return to session loop.

## On: wrap episode

Trigger: post-edit reply means wrap it up, done, or good.

1. Confirm `<episode-slug>` and `<episode-start>` exist; otherwise report that no active sprint-edit episode is open.
2. Find commits in `<episode-start>..HEAD` whose commit body contains `Sprint-Edit: <episode-slug>`.
3. Stop if no marked commits are found.
4. Set `<episode-range>` to the smallest contiguous Git range that contains the marked commits; report any unmarked commits inside the range as excluded from sprint-edit intent.
5. Call `{{.McpNamespace}}/playbook.print(name: "lead-update-spec")` and execute the returned procedure inline with `<episode-range>` and the marked commit list.
6. Render `mental-model-updater` via `{{.McpNamespace}}/playbook.render(name: "mental-model-updater")` (self-contained prompt + `recommended-tier`).
7. Spawn it with task input `Commit range: <episode-range>\nMarked commits: <marked-commits>\nSprint-Edit: <episode-slug>\nContext: <current-edit-context>`; collect its result.
8. Wait for completion and apply any needed episode-scoped documentation updates.
9. Call `{{.McpNamespace}}/infra.read(name: "executor-wrapup")`; follow Doc Pipeline and Doc Commit Gate for episode-scoped docs only.
10. Commit documentation changes only after the Doc Commit Gate passes.
11. Clear `<current-edit-context>`, `<episode-slug>`, and `<episode-start>`.
12. Report marked episode commits, documentation updates, and verification.
13. Return to session loop.

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
| Behavior, concept, or status question | Answer inline; spawn a host-native exploration worker directly with a scoped task prompt if codebase search is needed |
| Codebase exploration | Spawn a host-native exploration worker directly with a scoped task prompt |
| Design discussion | Discuss inline; do not auto-chain to the lead-write-spec procedure |
| Ticket, spec, or protocol change | Route through normal workflow; usually `{{.SkillNamespace}}:lead-proceed` when implementation is requested |
| One-context small interactive edit | Start or continue sprint-edit |
| Larger implementation, public contract work, cross-module change, new pattern, or review-worthy work | Continue through `{{.SkillNamespace}}:lead-proceed` or the lead-implement procedure |
| Ambiguous request | Ask the smallest routing question, then re-apply judge |

### judge: sprint-edit

Allow only when every condition is true:

- one active edit context is enough to hold the whole change;
- the lead can edit and verify inline without implementation delegation;
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
