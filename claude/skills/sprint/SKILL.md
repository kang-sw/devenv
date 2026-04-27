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
- Do not invoke `/discuss`, `/edit`, or `/implement` skills from the session loop — internalize their patterns directly.
- Before each delegation task: register agents via `ws-new-named-agent` and allocate `ws-review-path` slots (two separate Bash calls — see Delegation Cycle).
- After each delegation task: `rm -f <correctness-path> <fit-path>` using the literal paths stored from allocation.
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
6. Suggest: `git checkout main && git merge --no-ff sprint/<name> && git branch -d sprint/<name>`, or `git branch -d sprint/<name>` if no merge is needed.

## Judgments

### judge: delegate

Pick the first matching row; execute it; return to the session loop.

| Request type | Routing |
|---|---|
| Question about behavior, concept, or status | Answer inline; dispatch Explore agent if codebase search is needed |
| Codebase exploration (locate symbols, read files, map structure) | Dispatch Explore agent |
| Design discussion (approach tradeoffs, alternatives) | Inline discussion — do not auto-chain to `/write-spec` at end |
| Single-file edit or clear isolated change | Direct edit via Read/Edit tools; no doc pipeline; commit; return |
| Multi-file or new-pattern implementation | **Delegation Cycle** template |
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

### Delegation Cycle

Bash shell state does not persist between tool calls. Each step below is a
separate Bash call. Path values from Step 1 are read into lead context as
literals and interpolated into subsequent calls — never relied on as shell
variables.

**Step 1 — Register agents (one Bash call)**

```bash
ws-new-named-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
ws-new-named-agent reviewer-correctness --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
ws-new-named-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"
```

**Step 2 — Allocate paths (one Bash call)**

```bash
ws-review-path correctness fit
```

Read the two output lines. Store as `<correctness-path>` and `<fit-path>` in
context.

**Step 3 — Spawn implementer (one Bash call, `run_in_background: true`, `timeout: 600000`)**

```bash
ws-call-named-agent implementer - <<'PROMPT'
Run `ws-print-infra implementer.md` first.
Mode: B (inline brief)
<task description — goals, constraints; no doc pipeline required>
No skeleton — implement to spec.
Commit on current branch.
PROMPT
```

After the completion notification arrives, read the implementer's report:

```bash
ws-print-named-agent-output implementer
```

Note commit range from implementer report as `<first>..<last>`.

**Step 4 — Spawn reviewers in parallel (two Bash calls in the same response turn, each with `run_in_background: true`, `timeout: 600000`)**

```bash
ws-call-named-agent reviewer-correctness - <<'PROMPT'
Diff range: <first>..<last>
Write full findings to: <correctness-path>
Return only: [clean|non-clean]: <one-line summary>
PROMPT
```

```bash
ws-call-named-agent reviewer-fit - <<'PROMPT'
Diff range: <first>..<last>
Write full findings to: <fit-path>
Return only: [clean|non-clean]: <one-line summary>
PROMPT
```

After all completion notifications arrive, read each reviewer's summary:

```bash
ws-print-named-agent-output reviewer-correctness
ws-print-named-agent-output reviewer-fit
```

Review loop: if non-clean, relay file paths (not content) to implementer
(`run_in_background: true`, `timeout: 600000`); wait for notification, then
read output via `ws-print-named-agent-output implementer`; re-review
(reviewers overwrite paths, each `run_in_background: true`) until both
return `[clean]`.

**Cleanup (one Bash call)**

```bash
rm -f <correctness-path> <fit-path>
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature branch** — by deferring the doc pipeline to a single wrap-up pass and internalizing routing, the session maintains momentum without manual skill-chaining between tasks. When a rule is ambiguous, apply whichever interpretation better preserves throughput without accumulating documentation debt.
