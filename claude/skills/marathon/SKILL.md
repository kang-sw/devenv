---
name: marathon
description: >
  When the user requests implementation work — from ad-hoc fixes through
  multi-step features — invoke this. The default implementation workflow.
  Delegates code interaction to team members, keeping the main context
  lean for long-running sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

Target: $ARGUMENTS

## Doctrine

Marathon is the **token-efficient implementation workflow**. The main agent
(you) orchestrates discussion, decisions, and review. All code reading and
writing is delegated to team members. This keeps the main context lean,
enabling sessions that span many implementation cycles without hitting
context limits.

**You do not read source code.** You read: mental-model docs, tickets,
plan files, diff summaries, and team member reports. Everything else goes
through team members.

**Task list is the live dashboard.** Create, update, and cancel tasks as
the conversation evolves. The user should be able to glance at the task
list at any time and see the current state.

## Step 0: Bootstrap

1. Run `bash ai-docs/list-active.sh` (falls back to `find ai-docs -type f
   -name '*.md' | sort` if the script is missing).
2. If `$ARGUMENTS` references a ticket, read it.
3. Create a feature branch: `marathon/<scope>` from the current branch.
   Record the current branch as `<original-branch>`. If already on a
   `marathon/` branch, treat as a resumed session — infer
   `<original-branch>` from the merge-base with `main`, skip branch
   creation, and continue.
4. Create the team:
   ```
   TeamCreate("marathon-<scope>")
   ```
5. Spawn initial team members as needed (see Team Management below).
6. Create an initial task:
   ```
   [ ] [fixed] Marathon wrap-up — review, docs, merge
   ```

## Step 1: Marathon Loop

Repeat until the user signals done. Each turn is **either** discussion
or implementation — not a fixed sequence. If the user's request is clear
enough to act on, skip discussion and route directly.

### Discussion (when needed)

Contribute actively — propose approaches, surface risks, suggest
alternatives. Read mental-model docs as topics emerge. For codebase
details beyond mental-model docs, ask a team member.

When a ticket exists, record decisions in the ticket in real-time.

### Implementation — Complexity routing

Assess each implementation request and route accordingly:

| Complexity | Route | What the lead does |
|-----------|-------|-------------------|
| **Trivial** — user gives exact file + value | Message executor with inline brief | Review one-line report |
| **Simple** — clear scope, 1-2 files, no exploration needed | Message executor with brief (skip planner) | Review report + diff summary |
| **Complex** — multi-file, needs codebase research | Message planner → review plan → message executor | Review plan, then report + diff summary |

#### Trivial / Simple route

Send the executor an inline brief:
```
Implement: <what to change>
Files: <target files if known>
Constraints: <any constraints from discussion>
Branch: marathon/<scope>
```

#### Complex route

1. **Brief the planner.** Send a message with:
   ```
   Brief: <natural-language description of the change>
   Plan path: ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md
   Ticket: <path if applicable>
   Mental-model hints: <relevant domains>
   ```

2. **Review the plan.** When the planner reports completion, read the
   plan file. Verify against mental-model docs — check that contracts
   make sense and the approach aligns with architectural conventions.
   If issues are found, message the planner with corrections.

3. **Dispatch the executor.** Send:
   ```
   Plan: <plan-path>
   Branch: marathon/<scope>
   ```

### After each implementation

- Read the executor's report (summary, files changed, test results).
- If tests failed or deviations occurred, decide: message executor to
  fix, or discuss with the user first.
- Report results to the user.
- Update task status.

### Task discipline

Create tasks eagerly for any actionable item. Update status in real-time.
Cancel stale tasks immediately with a brief reason. Split tasks that grow
larger than expected.

## Team Management

### Spawning team members

```
Agent(
  subagent_type = "marathon-planner" or "marathon-executor",
  team_name = "marathon-<scope>",
  name = "planner" or "executor",  -- or domain-specific names
  model = "sonnet"                 -- override to "opus" for complex logic
)
```

### Lifecycle decisions (your judgment)

- **Reuse** a team member when the next task is in the same domain and
  the member's context is still relevant.
- **Spawn fresh** when switching to a different domain, or when the
  member's accumulated context might cause confusion.
- **Multiple members** are fine — e.g., one executor per domain if working
  on parallel concerns. Name them descriptively (e.g., "executor-ui",
  "executor-backend").
- **Correctness > token savings.** When in doubt, spawn fresh.

### Model selection

- **Planner**: sonnet (default). Opus if the change involves novel
  architecture with no existing patterns to follow.
- **Executor**: sonnet (default). Opus if implementing complex algorithms
  or cross-module changes where structural judgment is critical.
- **Exploration** (via `claude -p` inside team members): haiku.

## Step 2: Wrap-up (when user signals done)

Set the wrap-up task to `in_progress`. Execute in order:

1. **Code review** — read `git diff <base>..HEAD` yourself. You have the
   discussion context (what was intended) but no code context (how it was
   built), making you a natural independent reviewer. Check:
   - Does the diff match the discussed intent?
   - Any obvious issues visible in the diff?
   - For large diffs, focus on public interfaces and tests.

2. **Update mental model** — dispatch a mental-model-updater agent (not
   team member — standard Agent subagent). Skip for config/typo changes.
   Wait for completion before step 3.

3. **Update spec** — dispatch a spec-updater agent. Skip if
   `ai-docs/spec/` does not exist. Wait for completion before step 4.

4. **Update docs** — `ai-docs/_index.md` as needed. If a ticket was the
   input, load `/write-ticket` for conventions, then append `### Result`.
   If no ticket but changes relate to an existing ticket, ask the user.

5. **Final commit** — docs and remaining changes.

6. **Report** — summarize to the user:
   - What was implemented
   - Review findings (if any)
   - Process issues (if any)
   - Ticket status (if applicable)

7. **Shutdown team** — send shutdown request to all team members.

8. **Merge** — ask user for confirmation, then:
   ```bash
   git checkout <original-branch>
   git merge --no-ff marathon/<scope> -m "<conventional-commit message>"
   git branch -d marathon/<scope>
   ```
   If no commits were made, skip merge and delete the branch.

Set wrap-up task to `completed`.

## Rules

- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **No source code reading.** You read mental-model docs, tickets, plans,
  diff output, and team reports. For anything else, message a team member.
- **Ticket constraint:** When a ticket exists, record decisions there.
  Update the ticket after each meaningful implementation cycle.
- **Context conservation.** The entire point of marathon is keeping the
  main context lean. If you catch yourself reading source files, stop and
  delegate.
- **Team members cannot spawn subagents.** They use `claude -p` via Bash
  for exploration instead. This is documented in their agent definitions.
