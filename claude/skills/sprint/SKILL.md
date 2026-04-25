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
- Before each delegation task: run `ws-declare-agent implementer reviewer-correctness reviewer-fit` and allocate `review-path` slots in a single Bash call.
- After each delegation task: `rm -f <correctness-path> <fit-path>` using the literal paths stored from allocation.
- All written artifacts must be in English regardless of conversation language.

## On: invoke

1. Run `load-infra ws-orchestration.md` (Bash).
2. Read `git branch --show-current`.
   - Starts with `sprint/`: detect sprint name from branch. Present options:
     - **continue** → enter **On: session loop**.
     - **wrap-up** → go to **On: wrap-up**.
     - **abandon** → exit.
   - Does not start with `sprint/`: use `$ARGUMENTS` as sprint name if provided; else ask. Run `git checkout -b sprint/<name>`. Enter **On: session loop**.

## On: session loop

1. Apply **judge: needs-survey** — if warranted, run the Sprint-Aware Survey Call template (Bash) and incorporate the returned tier list into this turn's reasoning before responding.
2. Accept user request.
3. Apply **judge: delegate** — route and execute per the routing table.
4. Return to step 2.

## On: wrap-up

Triggers on explicit user done signal ("done", "wrap up", "finish sprint", or equivalent).

1. Determine parent: `PARENT=$(git merge-base HEAD main)`.
2. Dispatch `ws:spec-updater` with commit range `$PARENT..HEAD`. Wait for completion. Surface ambiguous stems to the user before proceeding.
3. Dispatch `ws:mental-model-updater` with the same range. Wait. (Must run after spec-updater so it captures any 🚧 strips.)
4. Run `load-infra executor-wrapup.md`. Follow §Doc Pipeline, §Doc Commit Gate, and (if ticket-driven) §Ticket Update.
5. Suggest: `git checkout main && git merge --no-ff sprint/<name>`, or `git branch -d sprint/<name>` if no merge is needed.

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
PARENT=$(git merge-base HEAD main)
COMMITS=$(git log "$PARENT"..HEAD --oneline 2>/dev/null || echo "(no commits yet)")
ws-call-agent sonnet --system-prompt "$(ws-infra-path sprint-survey.md)" \
  "Sprint: <sprint-name>
Branch: $(git branch --show-current)
Commit range: $PARENT..HEAD
Commits:
$COMMITS

Project map:
$(ws-proj-tree)"
```

### Delegation Cycle

```bash
# Re-scope agents for this task (clears prior session state)
ws-declare-agent implementer reviewer-correctness reviewer-fit

# Allocate review paths — single call, capture both lines
PATHS=$(review-path correctness fit)
CORRECTNESS_PATH=$(echo "$PATHS" | head -1)
FIT_PATH=$(echo "$PATHS" | tail -1)

# Spawn implementer
ws-call-agent sonnet --agent implementer \
  --system-prompt "$(ws-infra-path implementer.md)" \
  "Run \`load-infra implementer.md\` first.
Mode: B (inline brief)
<task description — goals, constraints; no doc pipeline required>
No skeleton — implement to spec.
Commit on current branch."

# Note commit range from implementer report as <first>..<last>

# Spawn reviewers in parallel (two Bash calls in the same response turn)
ws-call-agent sonnet --agent reviewer-correctness \
  --system-prompt "$(ws-infra-path code-review-correctness.md)" \
  "Diff range: <first>..<last>
Write full findings to: $CORRECTNESS_PATH
Return only: [clean|non-clean]: <one-line summary>"

ws-call-agent sonnet --agent reviewer-fit \
  --system-prompt "$(ws-infra-path code-review-fit.md)" \
  "Diff range: <first>..<last>
Write full findings to: $FIT_PATH
Return only: [clean|non-clean]: <one-line summary>"

# Review loop: if non-clean, relay file paths (not content) to implementer
# then re-review (reviewers overwrite paths) until both return [clean]

# Cleanup
rm -f "$CORRECTNESS_PATH" "$FIT_PATH"
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature branch** — by deferring the doc pipeline to a single wrap-up pass and internalizing routing, the session maintains momentum without manual skill-chaining between tasks. When a rule is ambiguous, apply whichever interpretation better preserves throughput without accumulating documentation debt.
