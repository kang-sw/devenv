---
title: Implement parallel-implement skill and parallel-implementer agent
related:
plans:
skeletons:
started:
completed:
---

# Implement parallel-implement skill and parallel-implementer agent

## Background

`claude/skills/parallel-implement/SKILL.md` is an unimplemented stub. It carries
invariants and a doctrine statement but has no procedural `## On:` sections. When
invoked, the lead agent finds nothing to delegate and falls back to doing all work
itself — defeating the skill's purpose.

This ticket implements the full parallel-implement workflow: a new
`parallel-implementer` agent and a rewritten SKILL.md that orchestrates N concurrent
implementer+reviewer pairs across disjoint file scopes on a shared branch.

Follow-on: `proceed` routing strategy will need a separate ticket once this skill is
operational — `/proceed` must learn to trigger `/parallel-implement` when the skeleton
defines clearly disjoint scopes.

## Decisions

### No branch/worktree isolation

Parallel implementers work on the shared current branch. Safety is enforced entirely
by disjoint scope assignments — no two implementers may touch the same file. Worktree
isolation was considered and rejected: it introduces merge coordination overhead
(conflict detection, sequential merge loop, branch cleanup) that is unnecessary when
scope disjointness is properly enforced at spawn time.

### Lead-collects commit policy

Implementers do not commit. When all implementers report done, the lead commits each
scope's changes sequentially in partition order using `git add <scope-file-set>`. This
ensures deterministic commit history and prevents git staging races between concurrent
implementers. Implementer-driven commits on completion were rejected: non-deterministic
completion order and shared git staging state create races even with disjoint file sets.

### Separate parallel-implementer agent (not prompt override)

A new `claude/agents/parallel-implementer.md` is created rather than overriding
`implementer.md`'s commit step via spawn prompt. The parallel implementer diverges on
too many dimensions to rely on prompt override: no plan-file input mode, no commit
authority, execution approval gate, hard file scope enforcement.

- **Option A (prompt override)** rejected: relies on prompt-beats-definition precedence
  which is not contractually guaranteed; becomes fragile if `implementer.md` adds new
  steps.
- **Option B (conditional implementer.md)** rejected: couples a shared agent definition
  to a specific orchestration mode; affects `/implement` which must remain unchanged.

### Per-implementer reviewer

Each scope unit gets its own implementer+reviewer pair running in parallel. An aggregate
reviewer after all implementers finish was rejected: it loses the direct
implementer↔reviewer back-channel that enables fix-and-re-review cycles without lead
involvement, and cannot run concurrently with implementation.

### Execution approval gate (3-message handshake)

Parallel implementers retain Bash but must request lead approval before any build, test,
or install command. Protocol:

```
implementer → lead:  {type: "run_request", command: "...", reason: "..."}
lead → implementer:  {type: "run_approved"} | {type: "run_wait"}
implementer executes locally; output stays in implementer context
implementer → lead:  {type: "run_complete", success: true|false}
```

The lead approves or queues; the implementer executes locally and keeps full output in
its own context; the lead receives only the success/fail signal. This preserves lead
context while retaining distributed execution and lead serialization control.

- **Lead-executes model** rejected: build/test output would pollute the lead's context
  window with stdout/stderr noise on every implementer test cycle.
- **Blanket Bash prohibition** rejected: removes legitimate read-only Bash uses
  (e.g., querying file metadata, language-specific inspection commands).

### Lead infers scopes conservatively

The lead infers disjoint scope units from the ticket/skeleton at invoke time. An
explicit `## Scopes` section requirement in tickets was considered and rejected: too
prescriptive — not all tickets will be authored with this section, and the lead is
capable of inferring scope from skeleton stubs and task descriptions. When scope
boundaries are ambiguous, the lead stops and asks the user rather than guessing.

## Prior Art

- `claude/skills/implement/SKILL.md` — single-scope cycle. Parallel-implement does not
  wrap this skill; it independently instantiates the same implementer+reviewer pattern
  across N scopes.
- `claude/skills/team-lead/SKILL.md` — team orchestration primitives used directly.
- `claude/agents/implementer.md` — reference for parallel-implementer content; shared
  constraints (no design re-research, escalate plan deviations, test-before-reporting)
  carry over unchanged.

## Phases

### Phase 1: parallel-implementer agent

Create `claude/agents/parallel-implementer.md`.

Delta from `implementer.md`:

**Input mode**: Brief only — no plan-file mode. The spawn prompt carries: scope name,
exhaustive file set, task description. The agent reads this directly; no plan file path
is involved.

**File scope enforcement (hard)**: If implementation requires a file outside the
assigned set, stop and escalate to the lead via SendMessage rather than proceeding. Do
not attempt workarounds.

**Execution gate**: Before any build, test, or install command:
1. Send `{type: "run_request", command: "<cmd>", reason: "<why>"}` to lead.
2. Wait for `{type: "run_approved"}`. If `run_wait`, wait and retry when re-prompted.
3. Execute locally. Keep full output in local context.
4. Send `{type: "run_complete", success: true|false}` to lead.
Never forward stdout/stderr to the lead.

**No commit**: Replace implementer.md step 8 ("Commit at logical checkpoints") with:
"Do NOT commit. When implementation is complete, send a completion report to the lead
listing changed files and a summary."

**Output** (via SendMessage to lead): summary of what was implemented, exact list of
files changed, test results (pass/fail/skipped), any deviations with rationale.

Success criteria: a fresh agent reading only this file and its spawn prompt has no
ambiguity about (1) which files it may touch, (2) that it must not commit, and (3) that
any execution requires lead approval first.

### Phase 2: parallel-implement SKILL.md rewrite

Rewrite `claude/skills/parallel-implement/SKILL.md`. Remove the TODO block entirely.
The invariants block is retained as-is. Add `## On: invoke` with the following steps.

**Depends on**: Phase 1 (parallel-implementer agent must exist before this skill
references it).

---

**0. Prerequisites**

Load `/team-lead` if not already loaded.

---

**1. Prepare**

1. Parse arguments: ticket path, plan path, or inline scope description.
2. Read the source. Infer N disjoint scope units. Each unit carries:
   - `name`: short identifier (used in agent names and commit messages)
   - `file_set`: exhaustive list of files this scope may touch
   - `description`: what needs to be implemented
3. Assert disjoint: verify no file appears in two scope sets. Stop if violated.
4. If scope boundaries are ambiguous at any point, stop and ask the user before
   proceeding. Bias toward escalation — do not guess.
5. `TeamCreate("parallel-impl-<slug>")`.
6. Create task list (all mandatory, do not skip or reorder):
   ```
   [ ] All N implementer+reviewer pairs — wait for all clean reports
   [ ] Lead collects + commits per scope in partition order
   [ ] Report to user — wait for approval
   [ ] Doc pipeline
   [ ] Cleanup
   ```

---

**2. Spawn N pairs (all simultaneous)**

For each scope unit, spawn two agents concurrently:

```
Agent(
  name = "implementer-<scope.name>",
  subagent_type = "parallel-implementer",
  team_name = "parallel-impl-<slug>",
  prompt = """
    Lead name: <lead-name>
    Scope: <scope.name>
    Allowed files: <scope.file_set>
    Task: <scope.description>

    Team rules:
    - Request lead approval before any build/test/install command (run_request protocol).
    - Do NOT commit.
    - Report completion to lead via SendMessage with changed files + summary.
    - The reviewer will contact you directly with findings — fix, re-verify, reply.
  """
)

Agent(
  name = "reviewer-<scope.name>",
  subagent_type = "reviewer",
  team_name = "parallel-impl-<slug>",
  prompt = """
    Lead name: <lead-name>
    Implementer name: implementer-<scope.name>
    Scope: <scope.name> (files: <scope.file_set>)

    Team rules:
    - SendMessage findings to implementer-<scope.name> directly.
    - Implementer fixes and notifies you — re-review until clean.
    - SendMessage final clean report to the lead.
  """
)
```

All 2N agents are spawned in the same message (fully parallel).

---

**3. On: run_request from implementer**

When a `run_request` message arrives:

1. If another run is currently in progress, reply `{type: "run_wait"}` and queue the
   request.
2. When the slot is free, reply `{type: "run_approved"}`.
3. Wait for `{type: "run_complete"}`. Log success/fail internally; do not intervene
   in implementer diagnosis.
4. Process the next queued request.

Serialize all run_requests — only one executing at a time. This prevents shared-state
collisions (lock files, build artifacts, port conflicts).

---

**4. Fan-in**

Wait for all N reviewer clean reports to arrive. Implementers and reviewers remain alive.

---

**5. Collect and commit**

For each scope in partition order (same order as step 1's scope list):

```bash
git add <scope.file_set>
git commit -m "<scope-commit-message>"  # per CLAUDE.md commit rules
```

Each commit message includes an AI Context section scoped to that implementer's work.
The overall commit sequence constitutes the complete implementation record.

---

**6. Report and approval**

Report to the user:
- Per-scope: what was implemented, reviewer verdict, test result
- Any deviations or open items across all scopes

Wait for user approval. If tweaks are requested:
1. Identify which scope(s) the tweak affects.
2. SendMessage to the relevant implementer with the fix directive.
3. Implementer fixes → re-requests execution approval if needed → reviewer re-reviews.
4. Re-run collect+commit for affected scope(s) only (amend or new commit per CLAUDE.md
   rules).
5. Re-report. Loop until user approves.

---

**7. Doc pipeline**

Dispatch mental-model-updater with the full commit range. Wait for completion before
proceeding.

---

**8. Cleanup**

Shutdown all teammates. `TeamDelete` only if this invocation created the team.

---

Success criteria: invoking `/parallel-implement` on a ticket with two clearly disjoint
scopes results in two implementer+reviewer pairs running concurrently, neither
committing independently, the lead serializing all run_requests, and the lead committing
both scopes sequentially after both reviewers report clean.
