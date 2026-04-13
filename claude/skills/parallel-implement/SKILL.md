---
name: parallel-implement
description: >
  When implementation spans multiple independent modules that can be
  built concurrently. Use instead of /implement when skeleton defines
  disjoint scopes suitable for parallel execution.
argument-hint: "<ticket-path, plan-path, or inline scope description>"
---

# Parallel Implementation

Target: $ARGUMENTS

## Invariants

- Scopes must be disjoint — no two implementers may touch the same file; verify this before spawning.
- Implementers never commit — the lead commits all scopes sequentially in partition order after all reviewer clean reports arrive.
- Each scope gets its own implementer+reviewer pair running concurrently — not a single shared reviewer.
- All run_requests are serialized by the lead — only one build/test command executes at a time across all implementers.
- User approves the aggregate report before the lead proceeds to the doc pipeline.
- Teammates stay alive until cleanup — do not shut down before doc pipeline completes.
- Task list is created at prepare and tracked to completion — no task may be skipped or reordered.
- `/team-lead` skill must be loaded before any team operations.
- When scope boundaries are ambiguous, stop and ask the user — do not guess.

## On: run_request

When `{"type": "run_request", "command": "...", "reason": "..."}` arrives from an implementer:

1. If another run is currently in progress (a `run_approved` has been sent and `run_complete` has not yet arrived), reply `{"type": "run_wait"}` to the requesting implementer and place the request in a queue.
2. When the current run completes (or if no run is in progress), reply `{"type": "run_approved"}` to the implementer whose slot is now free.
3. Wait for `{"type": "run_complete", "success": true|false}` from that implementer. Log success or failure internally. Do not intervene in the implementer's diagnosis or ask for stdout/stderr.
4. Dequeue the next pending request (if any) and repeat from step 2.

Serialize all run_requests — only one command executes at a time across all implementers. This prevents shared-state collisions: lock files, build artifacts, port conflicts.

This handler is active from the moment implementers are spawned (step 2) through cleanup (step 7). It is not phase-specific.

## On: invoke

### 0. Prerequisites

Load `/team-lead` if not already loaded.

### 1. Prepare

1. Parse arguments: ticket path, plan path, or inline scope description.
2. Read the source. Infer N disjoint scope units. Each unit carries:
   - `name`: short identifier used in agent names and commit messages (lowercase, hyphenated, e.g. `auth-api`)
   - `file_set`: exhaustive list of files this scope may touch — no file may appear in two scope sets
   - `description`: what needs to be implemented for this scope
3. Assert disjoint: verify that no file appears in two scope sets. If any overlap is found, stop — do not proceed until scopes are corrected.
4. If scope boundaries are ambiguous at any point during inference, stop and ask the user before proceeding. Bias toward escalation — it is never correct to guess scope boundaries.
5. Derive a `<slug>` from the ticket or brief (lowercase, hyphenated, e.g. `parallel-impl-auth`).
6. Create the team:
   ```
   TeamCreate(team_name = "parallel-impl-<slug>", description = "<brief scope description>")
   ```
7. Create task list. All tasks are mandatory — do not skip or reorder:
   ```
   [ ] All N implementer+reviewer pairs — wait for all N clean reports
   [ ] Lead collects + commits per scope in partition order
   [ ] Report to user — wait for approval
   [ ] Doc pipeline
   [ ] Cleanup
   ```

### 2. Spawn N pairs

Spawn all 2N agents in the same message (fully parallel). For each scope unit, spawn one implementer and one reviewer:

**Implementer spawn template:**

```
Agent(
  name = "implementer-<scope.name>",
  description = "Implement <scope.name> scope",
  subagent_type = "parallel-implementer",
  model = "sonnet",
  team_name = "parallel-impl-<slug>",
  prompt = """
    Lead name: <lead-name>
    Scope: <scope.name>
    Allowed files: <scope.file_set — one per line or comma-separated>
    Task: <scope.description>

    Team rules:
    - Request lead approval before any build/test/install command using the run_request protocol:
        send {"type": "run_request", "command": "<cmd>", "reason": "<why>"}
        wait — the lead will send {"type": "run_approved"} when the slot is free
        (if you receive {"type": "run_wait"}, do not re-send; wait for run_approved)
        execute locally; keep full output in your own context — do not forward to lead
        send {"type": "run_complete", "success": true|false}
    - Do NOT commit. The lead commits all changes after you report completion.
    - Report completion to lead via SendMessage with: summary, exact changed file list, test results, deviations.
    - The reviewer will contact you directly with findings — fix within the allowed file set, re-verify, reply.
  """
)
```

**Reviewer spawn template:**

```
Agent(
  name = "reviewer-<scope.name>",
  description = "Review <scope.name> implementation",
  subagent_type = "reviewer",
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

### 5. Report and approval

Report to the user:
- Per-scope: what was implemented, reviewer verdict, test result
- Any deviations or open items across all scopes

Wait for user approval before proceeding. If the user requests tweaks:
1. Identify which scope(s) are affected.
2. SendMessage the fix directive to the relevant implementer(s).
3. Implementer fixes → requests execution approval via run_request gate → reviewer re-reviews.
4. Re-run collect+commit for affected scope(s) only (new commit per CLAUDE.md rules).
5. Re-report. Loop until user approves.

### 6. Doc pipeline

Dispatch **mental-model-updater** with the full commit range covering all scope commits. Wait for completion before proceeding — the cleanup step must not begin until the doc state is final.

### 7. Cleanup

Send shutdown requests to all teammates (implementers and reviewers). Wait for shutdown approval from each. Call `TeamDelete` only if this invocation of `/parallel-implement` created the team.

## Doctrine

Parallel implementation optimizes for **throughput with isolation** —
maximize concurrent work while preventing interference between scope units.
When a rule is ambiguous, apply whichever interpretation better preserves
isolation: disjoint file sets, serialized execution, and lead-controlled commits
are the three mechanisms; none may be relaxed for throughput.
