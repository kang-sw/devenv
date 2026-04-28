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
- Doc pipeline (spec audit, `ws:mental-model-updater`) is suppressed during task execution; it runs once at wrap-up only.
- All written artifacts must be in English regardless of conversation language.
- At wrap-up, commit each doc update immediately after it completes — spec changes in one commit, mental-model-updater changes in the next. Do not batch both into a single deferred commit.

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
2. **Spec-update pass (lead-driven).** The lead reads conventions and audits commits directly.
   a. Load: `ws-print-infra spec-conventions.md` (Bash). Read `claude/skills/write-spec/SKILL.md`.
   b. Scan commits: `git log "$PARENT"..HEAD --oneline`. For each commit, judge **spec-impact** — does it introduce or change behavior a caller can observe? (New flags, changed outputs, new commands, updated conventions all qualify. Internal refactors that preserve behavior do not.)
   c. For each impacted area: read the relevant spec file(s) from `ai-docs/spec/`. If no entry exists for the new behavior, add one — run `ws-generate-spec-stem <slug>` for each new anchor, follow the `spec-format` template from `write-spec/SKILL.md`. Do not add 🚧 unless the feature is genuinely planned-but-unimplemented.
   d. Strip 🚧: scan all spec files for `🚧` entries. For each: check whether the stem appears in the commit log (`git log "$PARENT"..HEAD | grep <stem>`). If yes and the feature is confirmed implemented, strip `🚧 ` from the heading and remove any `> [!note] Planned 🚧` callout block.
   e. Removals: check commits for `removed: <stem>` in `## Spec` sections. Remove the corresponding entry from the spec file.
   f. Run `ws-spec-build-index` if any file was modified. Commit all spec changes in a single commit.
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
ws-new-named-agent sprint-survey --model sonnet --system-prompt sprint-survey
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
