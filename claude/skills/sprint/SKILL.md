---
name: sprint
description: >
  Multi-task session container for feature-branch work. Holds routing,
  implementation, and wrap-up in a single persistent session. Doc pipeline
  is deferred to wrap-up; each task commits only source changes.
argument-hint: "[sprint-name]"
---

# Sprint Session Container

Target: $ARGUMENTS

## Invariants

- Sprint operates only on `sprint/`-prefixed branches — do not enter the session loop or run wrap-up on any other branch.
- Doc pipeline (`ws:spec-updater`, `ws:mental-model-updater`) is suppressed during task execution; it runs once at wrap-up only.
- All written artifacts must be in English regardless of conversation language.
- At wrap-up, commit each doc updater's output immediately after it completes — spec-updater changes in one commit, mental-model-updater changes in the next. Do not batch both into a single deferred commit.

## On: invoke

1. Run `ws-print-infra ws-orchestration.md` (Bash).
2. Read `git branch --show-current`.
   - Starts with `sprint/`: detect sprint name from branch. Present options:
     - **continue** → enter **On: session loop**.
     - **wrap-up** → go to **On: wrap-up**.
     - **abandon** → exit.
   - Does not start with `sprint/`: infer a branch name without asking.
     - Context (preamble or conversation) gives a clear topic → derive a short kebab-case slug from it (e.g., `refactor-agent-registry`).
     - Context is vague or absent → generate a random three-word name in `<adjective>-<noun>-<noun>` form (e.g., `amber-ridge-quill`).
     - Run `git checkout -b sprint/<name>`. Enter **On: session loop**.

## On: session loop

1. Apply **judge: needs-survey** — if warranted, run the Sprint-Aware Survey Call template (Bash) and incorporate the returned tier list into this turn's reasoning before responding.
2. Accept user request.
3. Apply **judge: delegate** — route and execute per the routing table.
4. Return to step 2.

## On: wrap-up

Triggers on explicit user done signal ("done", "wrap up", "finish sprint", or equivalent).

1. Determine parent: `PARENT=$(git merge-base HEAD main)`.
2. **Spec-update pass.** Dispatch `ws:spec-updater` with the commit range in Suggestion mode — pass `$PARENT..HEAD`, the commit log, and instruct: analyze only, do not edit files; propose strips for 🚧 entries whose stems appear in the commits; flag removals. Wait. Apply `### Proposed strips` directly (strip `🚧 ` prefix, remove any `> [!note] Planned 🚧` callout block); collect `### Pending removal`, `### Planned entry dropped`, and ambiguous cases for the step 5 report. Run `ws-spec-build-index` if any file was modified; commit spec changes.
3. Dispatch `ws:mental-model-updater` with commit range `$PARENT..HEAD`. Include a note that docs may be stale from accumulated sprint commits — explore thoroughly. Wait. (Must run after the spec-update loop so it sees the updated spec.)
4. Run `ws-print-infra executor-wrapup.md`. Follow §Doc Pipeline and §Doc Commit Gate. If ticket-driven, follow §Ticket Update: update existing tickets only — set `## Result` and advance state; do not create new tickets.
5. Report to user: spec entries added, removed, and 🚧-stripped; mental model sections updated.
6. Merge: run `ws-merge-branch main sprint/<name> "<commit-message>"`. Compose the commit message per CLAUDE.md commit rules — summarize the sprint's scope and key changes across all tasks. If no source changes were made (doc-only or exploration sprint), skip merge and delete the branch: `git branch -d sprint/<name>`.

## Judgments

### judge: delegate

Pick the first matching row; execute it; return to the session loop.

| Request type | Routing |
|---|---|
| Question about behavior, concept, or status | Answer inline; dispatch Explore agent if codebase search is needed |
| Codebase exploration (locate symbols, read files, map structure) | Dispatch Explore agent |
| Design discussion (approach tradeoffs, alternatives) | Inline discussion — do not auto-chain to `/write-spec` at end |
| Single-file edit or clear isolated change | Invoke `ws:edit` via Skill tool |
| Multi-file or new-pattern implementation | Invoke `ws:write-code` via Skill tool |
| Exploration required before routing is possible | Run sprint-aware survey; re-apply judge |

### judge: needs-survey

Fire the Sprint-Aware Survey Call when:
- Session loop is entered for the first time this session (always).
- Request touches a domain or component not yet surveyed this session.
- Domain shifts mid-session (request topic diverges significantly from prior turns).

Does NOT fire for follow-up turns within an established domain, or for status / continuity queries.

## Templates

### Sprint-Aware Survey Call

```bash
ws-new-named-agent sprint-survey --model sonnet --system-prompt "$(ws-infra-path sprint-survey.md)"
```

```bash
PARENT=$(git merge-base HEAD main)
COMMITS=$(git log "$PARENT"..HEAD --oneline 2>/dev/null || echo "(no commits yet)")
BRANCH=$(git branch --show-current)
PROJ_TREE=$(ws-proj-tree)
ws-call-named-agent sprint-survey - <<PROMPT
Sprint: <sprint-name>
Branch: $BRANCH
Commit range: $PARENT..HEAD
Commits:
$COMMITS

Project map:
$PROJ_TREE
PROMPT
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature branch** — by deferring the doc pipeline to a single wrap-up pass and delegating implementation to write-code and edit primitives, the session maintains momentum without accumulating documentation debt or managing internal agent state. When a rule is ambiguous, apply whichever interpretation better preserves throughput.
