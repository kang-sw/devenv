---
name: parallel-implement
description: >
  Parallel implementation across N disjoint scope units. Spawns one
  implementer-reviewer pair per scope, serializes build commands through
  the lead, and commits one scope at a time.
argument-hint: "<ticket-path, plan-path, or inline scope description> [--main-branch <name>]"
---

# Parallel Implementation

Target: $ARGUMENTS

## Invariants

- The pre-flight manifest is mandatory — every invocation builds a scope manifest with `name`, `file_set`, `description`, and `test_command` per scope before any agent is spawned.
- Scopes must be disjoint — no two implementers may touch the same file; verify this before spawning.
- Every scope's `test_command` must be scope-specific — e.g., `cargo test auth::` not `cargo test`. Crate-wide test commands compile the whole project and will pick up another scope's in-progress stubs, breaking isolation even with run_request serialization.
- Scope-specific test commands are only safe because `/write-skeleton` ensures all stubs compile (see `/write-skeleton` step 3). If no skeleton exists for the target scopes, stop and route through `/write-skeleton` first.
- Implementers never commit — the lead commits all scopes sequentially in partition order after all reviewer clean reports arrive.
- Each scope gets its own implementer+reviewer pair running concurrently — not a single shared reviewer.
- All run_requests are serialized by the lead — only one build/test command executes at a time across all implementers.
- **Main-branch mode** (invoked from `main`/`master`/`trunk`): user approves the aggregate report before the lead proceeds to the doc pipeline.
- **Feature-branch mode** (invoked from any other branch): approval gate is skipped; lead proceeds directly to merge after clean review. The feature → main merge remains the user's responsibility.
- Teammates stay alive until cleanup — do not shut down before doc pipeline completes.
- Task list is registered via TaskCreate at pre-flight and tracked to completion — one task per scope plus one per workflow phase; no task may be skipped or reordered.
- `/team-lead` skill must be loaded before any team operations.
- When scope boundaries are ambiguous, stop and ask the user — do not guess.

## On: run_request

When `{"type": "run_request", "command": "...", "reason": "..."}` arrives from an implementer:

1. If another run is currently in progress (a `run_approved` has been sent and `run_complete` has not yet arrived), reply `{"type": "run_wait"}` to the requesting implementer and place the request in a queue.
2. When the current run completes (or if no run is in progress), reply `{"type": "run_approved"}` to the implementer whose slot is now free.
3. Wait for `{"type": "run_complete", "success": true|false}` from that implementer. Log success or failure internally. Do not intervene in the implementer's diagnosis or ask for stdout/stderr.
4. Dequeue the next pending request (if any) and repeat from step 2.

Serialize all run_requests — only one command executes at a time across all implementers. This prevents shared-state collisions: lock files, build artifacts, port conflicts.

This handler is active from the moment implementers are spawned (step 2) through cleanup (step 8). It is not phase-specific.

## On: invoke

### 0. Prerequisites

Load `/team-lead` if not already loaded.

### 1. Pre-flight manifest

0. Context survey: spawn `project-survey` with the ticket path or inline brief. Capture the returned `[Must|Maybe]` reference list — include it in each worker's spawn prompt at step 2.
1. Parse arguments: ticket path, plan path, or inline scope description.
2. Read the source. Infer N disjoint scope units. Each unit carries:
   - `name`: short identifier used in agent names and commit messages (lowercase, hyphenated, e.g. `auth-api`).
   - `file_set`: exhaustive list of files this scope may touch — no file may appear in two scope sets.
   - `description`: what needs to be implemented for this scope.
   - `test_command`: scope-specific test invocation (e.g., `cargo test auth::`, `pytest tests/auth/`, `npm test -- --testPathPattern=auth`). Must narrow to this scope's tests only — do not accept a crate-wide or suite-wide runner.
3. Assert disjoint: verify that no file appears in two scope sets. If any overlap is found, stop — do not proceed until scopes are corrected.
4. Verify skeleton coverage: confirm stubs exist for every scope (grep for `todo!()` / `unimplemented!()` / `NotImplementedError` in each scope's `file_set`). If absent, stop — route through `/write-skeleton` first to guarantee compilability during parallel work.
5. If scope boundaries are ambiguous at any point during inference, stop and ask the user before proceeding. Bias toward escalation — it is never correct to guess scope boundaries.
6. Derive a `<slug>` from the ticket or brief (lowercase, hyphenated, e.g. `parallel-impl-auth`).
7. Record current branch as `<original-branch>`. Detect **invocation mode**:
   - `<original-branch>` matches `main`, `master`, `trunk`, or the value of `--main-branch <name>` → **main-branch mode** (approval gate active).
   - Otherwise → **feature-branch mode** (approval gate skipped; proceed directly to merge after clean review).
   Create `parallel-impl/<slug>` branch.
8. Create the team:
   ```
   TeamCreate(team_name = "parallel-impl-<slug>", description = "<brief scope description>")
   ```
9. Register tasks via TaskCreate. One task per scope (carries the scope manifest in its description), plus one task per workflow phase. All are mandatory — do not skip or reorder:
   - Scope tasks (one per scope): `implement-<scope.name>` — description includes file_set + test_command + brief.
   - Phase tasks: fan-in all reviewer reports / collect and commit per scope / docs pre-pass (mental-model-updater + spec-updater) / report to user (approval wait — main-branch mode only) / merge to original branch / doc pipeline / cleanup.

### 2. Spawn N pairs

Spawn all 2N agents in the same message (fully parallel). For each scope unit, spawn one implementer and one reviewer:

**Implementer spawn template:**

```
Agent(
  name = "implementer-<scope.name>",
  description = "Implement <scope.name> scope",
  subagent_type = "general-purpose",
  model = "sonnet",
  team_name = "parallel-impl-<slug>",
  prompt = """
    Read `${CLAUDE_SKILL_DIR}/parallel-implementer.md` first.

    Lead name: <lead-name>
    Scope: <scope.name>
    Allowed files: <scope.file_set — one per line or comma-separated>
    Task: <scope.description>
    Scope-specific test command: <scope.test_command>

    Team rules:
    - Use ONLY the scope-specific test command above to verify your work.
      Do NOT run crate-wide or suite-wide runners (e.g. bare `cargo test`,
      `pytest` with no path, `npm test` with no filter) — another scope's
      in-progress code may compile or run alongside yours and produce
      failures that do not reflect your scope.
    - Request lead approval before any build/test/install command using the run_request protocol:
        send {"type": "run_request", "command": "<cmd>", "reason": "<why>"}
        wait — the lead will send {"type": "run_approved"} when the slot is free
        (if you receive {"type": "run_wait"}, do not re-send; wait for run_approved)
        execute locally; keep full output in your own context — do not forward to lead
        send {"type": "run_complete", "success": true|false}
    - Do NOT commit. The lead commits all changes after you report completion.
    - Report completion to lead via SendMessage with: summary, exact changed file list, test results, deviations.
    - The reviewer will contact you directly with findings — fix within the allowed file set, re-verify with the scope-specific test command, reply.
  """
)
```

**Reviewer spawn template:**

```
Agent(
  name = "reviewer-<scope.name>",
  description = "Review <scope.name> implementation",
  subagent_type = "ws:code-reviewer",
  model = "sonnet",
  team_name = "parallel-impl-<slug>",
  prompt = """
    Lead name: <lead-name>
    Implementer name: implementer-<scope.name>
    Scope: <scope.name>
    Files in scope: <scope.file_set>

    Note: implementers do not commit. To see changes, run:
      git diff HEAD -- <scope.file_set>
    Or read the files listed in scope directly. Use the implementer's
    completion report as the authoritative scope of what was changed.

    Team rules:
    - SendMessage findings directly to implementer-<scope.name> — do not route through the lead.
    - The implementer fixes and notifies you — re-review until the implementation is clean.
    - SendMessage the final clean report to the lead when done.
  """
)
```

All 2N agents are spawned in one batch. Do not wait for one pair before spawning the next.

### 3. Fan-in

Wait for all N reviewer clean reports to arrive. Implementers and reviewers remain alive — do not shut them down yet.

If a reviewer reports findings, it is communicating directly with its paired implementer. Do not intervene unless an implementer escalates a file-scope conflict or a run_request arrives (handled by `## On: run_request`).

### 4. Collect and commit

For each scope in partition order (same order as step 1's scope list):

```bash
git add <scope.file_set>
git commit -m "<scope-commit-message per CLAUDE.md rules>"
```

Each commit message must include an AI Context section describing that scope's implementation decisions, alternatives considered, and rationale — derived from the implementer's completion report.

Do not batch scopes into a single commit. One commit per scope preserves attribution and makes the history bisectable.

### 5. Docs pre-pass

1. Dispatch **spec-updater** with the full commit range covering all scope commits. Wait for it to complete. If **spec-updater** reports ambiguous stems, note them for step 6.
2. Dispatch **mental-model-updater**. Wait for it to complete. Running spec-updater first ensures mental-model-updater's spec-diff check captures any 🚧 strips committed by spec-updater.

### 6. Report and approval

> **Feature-branch mode**: emit the report below, then proceed directly to step 7 (Merge) — do not wait for user approval.

1. Report to the user:
   - Per-scope: what was implemented, reviewer verdict, test result
   - Any deviations or open items across all scopes
2. **Main-branch mode only** — wait for user approval before proceeding. If the user requests tweaks:
   1. Identify which scope(s) are affected.
   2. SendMessage the fix directive to the relevant implementer(s).
   3. Implementer fixes → requests execution approval via run_request gate → reviewer re-reviews.
   4. Re-run collect+commit for affected scope(s) only (new commit per CLAUDE.md rules).
   5. Re-run **spec-updater** with the new commit range. Wait. Then re-run **mental-model-updater**. Wait.
   6. Re-report. Loop until user approves.

### 7. Merge

1. Run `merge-branch <original-branch> parallel-impl-<slug> "<commit-message>"`.
   The script selects strategy by commit count: squash (1 commit) or --no-ff (2+).
   Compose the commit message per CLAUDE.md commit rules summarizing all scopes.

### 8. Doc pipeline

Run `load-infra executor-wrapup.md`. Follow §Doc Pipeline, §Doc Commit Gate, and (if ticket-driven) §Ticket Update. Pass the full commit range covering all scopes.

### 9. Cleanup

Send shutdown requests to all teammates (implementers and reviewers). Wait for shutdown approval from each. Call `TeamDelete` only if this invocation of `/parallel-implement` created the team.

## Doctrine

Parallel implementation optimizes for **throughput with isolation on a
shared branch** — maximize concurrent work while preventing interference
between scope units without resorting to worktrees. Four mechanisms
enforce isolation: disjoint file sets, scope-specific test commands
(standing on skeleton's compilability guarantee), serialized execution
via run_request, and lead-controlled commits. None may be relaxed for
throughput; weakening any one opens a path for one scope's mid-flight
state to pollute another's verification. When a rule is ambiguous, apply
whichever interpretation better preserves these four mechanisms.
