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
- Before each delegation task: register agents via `ws-new-agent` and allocate `ws-review-path` slots (two separate Bash calls — see Delegation Cycle).
- After each delegation task: `rm -f <correctness-path> <fit-path>` using the literal paths stored from allocation.
- All written artifacts must be in English regardless of conversation language.

## On: invoke

1. Run `ws-print-infra ws-orchestration.md` (Bash).
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
2. **Spec-update loop** (max 2 iterations — sonnet first, opus on retry). Use the **Spec-Update Override** template. After each run, read `git diff ai-docs/spec/` and judge whether the edits are coherent and grounded in the commit list:
   - **Yes** → commit spec changes; proceed to step 3.
   - **No, iteration 1** → `git checkout ai-docs/spec/`; retry at opus with explicit feedback on what was wrong.
   - **No, iteration 2** → commit as-is; warn user with a list of specific concerns; proceed to step 3.
3. Dispatch `ws:mental-model-updater` with commit range `$PARENT..HEAD`. Include a note that docs may be stale from accumulated sprint commits — explore thoroughly. Wait. (Must run after the spec-update loop so it sees the updated spec.)
4. Run `ws-print-infra executor-wrapup.md`. Follow §Doc Pipeline and §Doc Commit Gate. If ticket-driven, follow §Ticket Update: update existing tickets only — set `## Result` and advance state; do not create new tickets.
5. Report to user: spec entries added, removed, and 🚧-stripped; mental model sections updated.
6. Suggest: `git checkout main && git merge --no-ff sprint/<name>`, or `git branch -d sprint/<name>` if no merge is needed.

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
ws-new-agent sprint-survey --model sonnet --system-prompt "$(ws-infra-path sprint-survey.md)"
```

```bash
PARENT=$(git merge-base HEAD main)
COMMITS=$(git log "$PARENT"..HEAD --oneline 2>/dev/null || echo "(no commits yet)")
ws-call-agent sprint-survey \
  "Sprint: <sprint-name>
Branch: $(git branch --show-current)
Commit range: $PARENT..HEAD
Commits:
$COMMITS

Project map:
$(ws-proj-tree)"
```

### Delegation Cycle

Bash shell state does not persist between tool calls. Each step below is a
separate Bash call. Path values from Step 1 are read into lead context as
literals and interpolated into subsequent calls — never relied on as shell
variables.

**Step 1 — Register agents (one Bash call)**

```bash
ws-new-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
ws-new-agent reviewer-correctness --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
ws-new-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"
```

**Step 2 — Allocate paths (one Bash call)**

```bash
ws-review-path correctness fit
```

Read the two output lines. Store as `<correctness-path>` and `<fit-path>` in
context.

**Step 3 — Spawn implementer (one Bash call)**

```bash
ws-call-agent implementer \
  "Run \`ws-print-infra implementer.md\` first.
Mode: B (inline brief)
<task description — goals, constraints; no doc pipeline required>
No skeleton — implement to spec.
Commit on current branch."
```

Note commit range from implementer report as `<first>..<last>`.

**Step 4 — Spawn reviewers in parallel (two Bash calls in the same response turn)**

```bash
ws-call-agent reviewer-correctness \
  "Diff range: <first>..<last>
Write full findings to: <correctness-path>
Return only: [clean|non-clean]: <one-line summary>"
```

```bash
ws-call-agent reviewer-fit \
  "Diff range: <first>..<last>
Write full findings to: <fit-path>
Return only: [clean|non-clean]: <one-line summary>"
```

Review loop: if non-clean, relay file paths (not content) to implementer;
re-review (reviewers overwrite paths) until both return `[clean]`.

**Cleanup (one Bash call)**

```bash
rm -f <correctness-path> <fit-path>
```

### Spec-Update Override

Register and call `ws:spec-updater` in active-editing mode. Use `sonnet` on the first iteration, `opus` on the retry.

**Step 1 — Register (one Bash call)**

```bash
ws-new-agent spec-updater --agent ws:spec-updater --model <sonnet|opus>
```

**Step 2 — Call (one Bash call)**

```bash
PARENT=$(git merge-base HEAD main)
COMMITS=$(git log "$PARENT"..HEAD --oneline)
ws-call-agent spec-updater \
  "SPRINT WRAP-UP — docs are likely stale from accumulated sprint commits.
Override default conservative behavior: actively edit ai-docs/spec/ files.

Commit range: $PARENT..HEAD
Commits:
$COMMITS

Required tasks:
1. Strip 🚧 markers from entries whose spec stems appear in the commit list.
2. Add new spec entries for features introduced in the commits that have no existing entry.
3. Remove spec entries for features explicitly dropped in the commits.

Apply all edits directly. Do not defer or recommend — make the changes."
```

## Doctrine

Sprint optimizes for **sustained implementation throughput across a feature branch** — by deferring the doc pipeline to a single wrap-up pass and internalizing routing, the session maintains momentum without manual skill-chaining between tasks. When a rule is ambiguous, apply whichever interpretation better preserves throughput without accumulating documentation debt.
