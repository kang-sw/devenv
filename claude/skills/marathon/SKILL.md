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

Marathon is the **token-efficient implementation workflow**. You are the
**lead** — you orchestrate discussion, decisions, and review. All code
reading and writing is delegated to team members. This keeps the main
context lean,
enabling sessions that span many implementation cycles without hitting
context limits.

**You do not read source code.** You read: mental-model docs, tickets,
plan files, diff summaries, and team member reports. Everything else goes
through team members. If you catch yourself opening a source file, stop
and delegate.

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
5. Team members are spawned on-demand when the first implementation
   request arrives. See **Team Management** below for spawn conventions,
   naming, model selection, and parallel coordination.
6. Create **protocol tasks** — persistent reminders visible throughout
   the session. NOT work items; never complete until Session End.
   ```
   TaskCreate("[PROTOCOL] Delegate all code reading/writing to team members")
   TaskCreate("[PROTOCOL] Each round: assess reuse vs fresh spawn")
   TaskCreate("[PROTOCOL] Before merge: code review (fresh sonnet on sub-branch diff)")
   TaskCreate("[PROTOCOL] After merge: dispatch doc updaters if non-trivial")
   TaskCreate("[PROTOCOL] Wrap-up — coherence check, merge")
   ```

## Step 1: Marathon Loop

Repeat until the user signals done. Each turn is **either** discussion
or implementation — not a fixed sequence. If the user's request is clear
enough to act on, skip discussion and route directly.

### Discussion (when needed)

Contribute actively — propose approaches, surface risks, suggest
alternatives. Read mental-model docs as topics emerge. For codebase
details beyond mental-model docs, ask a team member.

When a ticket exists, update unimplemented phases to reflect discussion
conclusions in real-time. The ticket is the live spec for upcoming work.

### Implementation — routing

1. **Non-code** task → dispatch **worker**.
2. Needs codebase research (no ticket/contracts to guide) →
   **planner** first, then **implementer**.
3. Otherwise → **implementer** directly.

**Branch:** one-liner → direct commit on `marathon/<scope>`.
Everything else → sub-branch `<type>/<round>` from `marathon/<scope>`.

**Brief template** (all routes):
```
Brief: <what to change>
Files: <target files if known>
Constraints: <any constraints from discussion>
Branch: <branch per above>
```

**Planner flow:** brief with description, plan path
(`ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md`), ticket path, and
mental-model hints. Review the plan against mental-model docs; if no
convergence after two rounds, dispatch implementer with inline guidance.

### After each implementation — merge gate

When the implementer reports completion:

1. Read the implementer's report (summary, files changed, test results).
2. For sub-branch work, review the scope:
   ```bash
   git diff --stat marathon/<scope>...<type>/<round>
   ```
3. **Code review (pre-merge).** Run unless the round was trivial
   (typo, config-only, single-line fix). When in doubt, run it.

   Dispatch a fresh **sonnet** Agent (general-purpose, not a team
   member) with the sub-branch diff:
   ```
   git diff marathon/<scope>...<type>/<round>
   ```
   Review prompt: scope, requirements, CLAUDE.md standards, mental-model
   docs. Categorize as Critical / Important / Minor.

   Fix Critical/Important issues: message implementer to fix on the
   sub-branch → re-dispatch reviewer. Loop until clean.

4. **Merge decision:**
   - **Accept** — `git merge --no-ff <type>/<round>` into
     `marathon/<scope>`, then delete the sub-branch.
   - **Rollback** — delete the sub-branch entirely.
5. **Doc updates (post-merge).** Skip for config/typo changes.
   - Dispatch in parallel (fresh sonnet Agents, not team members;
     apply **parallel commit coordination**):
     - **spec-updater** — skip if `ai-docs/spec/` does not exist.
     - **mental-model-updater**
   - Wait for both to complete.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase, append `### Result` to the ticket
     (load `/write-ticket` for conventions).
   - Commit doc changes.
6. Report results to the user.

When splitting a ticket phase into subphases, update the ticket to
reflect the split before proceeding. Verification agents are always
fresh — no context carry-over between checkpoints.

### Task discipline

Protocol tasks stay pending until Session End — they are reminders,
not work. Work tasks are optional; create only for multi-phase or
parallel work where the user benefits from progress visibility.

## Team Management

### Spawning team members

Role descriptions are in `~/.claude/skills/marathon/agents/`:

| Role file | Purpose |
|-----------|---------|
| `planner.md` | Codebase exploration → plan file |
| `implementer.md` | Code implementation from plan or brief |
| `worker.md` | Non-code tasks (documents, config, research output) |

Spawn general-purpose agents with a role file reference:

```
Agent(
  subagent_type = "general-purpose",
  team_name = "marathon-<scope>",
  name = "<role>.<domain>",        -- e.g., "implementer.chunk", "planner.indexing"
  model = "sonnet",                -- override to "opus" for complex logic
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md to understand
            your role. Your lead's name is '<your-agent-name>'.
            Then: <brief or plan reference>"
)
```

**Naming:** Always `<role>.<domain>` — no bare "implementer" or "planner".

### Reuse vs. fresh spawn

| Relevant code size | Same domain | Different domain |
|--------------------|-------------|-----------------|
| Large (10K+ lines) | **Reuse** — context re-read cost is high | **Fresh spawn** |
| Small (<2K lines)  | **Fresh spawn** — re-read is cheap, parallelism wins | **Fresh spawn** |

"Relevant" = lines the implementer would need to read, not total codebase.

**Overrides:** Data dependency on prior output → reuse. Prior deviation
reported → fresh spawn. When in doubt, spawn fresh.

### Parallel commit coordination

When spawning **any** agents in parallel (implementers, updaters, or
mixed), append to each spawn prompt:

> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

Then serialize commit approvals one at a time. This applies to all
parallel agents — implementers, spec-updater, mental-model-updater,
or any combination. The git index is shared; concurrent commits corrupt
staging.

### Model selection

Default **sonnet** for all roles. Override to **opus** for novel
architecture or complex cross-module logic. Override to **haiku** for
mechanical worker tasks and `claude -p` exploration.

## Step 2: Session End (when user signals done)

Most verification work has been done incrementally via checkpoints.
Session end is lightweight:

1. **Final checkpoint** — run one if the last round didn't trigger one.

2. **Coherence review** — dispatch a fresh **opus** Agent to read all
   of `ai-docs/mental-model/` and `ai-docs/spec/` (if exists). Look
   for cross-document contradictions from incremental updates. Fix
   session-caused issues; flag pre-existing ones in the report.
   Skip if trivial session, no mental-model dir, or no docs updated.

3. **Final commit** — coherence fixes and any remaining changes.

4. **Report** — summarize to the user:
   - What was implemented across the session
   - Coherence review findings (if any)
   - Process issues (if any)
   - Ticket status (if applicable)

5. **Clean up protocol tasks** — mark all `[PROTOCOL]` tasks as
   completed.

6. **Shutdown team** — send shutdown request to all team members.

7. **Merge** — ask user for confirmation, then merge `marathon/<scope>`
   into `<original-branch>` with `--no-ff`. Commit message format:
   ```
   <type>(<scope>): <summary>
   <what changed>
   ## AI Context
   - <decisions, alternatives, directives>
   ```
   Delete `marathon/<scope>` after merge. Skip if no commits were made.

## Rules

- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **Ticket as live document.** Keep unimplemented phases accurate as
  discussion evolves (with user agreement). Completed phases (with
  `### Result`) are immutable.
- **User controls session lifecycle.** Never enter Session End unless
  the user explicitly signals done. Completing a phase or running out
  of tasks is NOT a signal — ask what's next.
- **Scope expansion.** New concerns mid-session → create a ticket now
  while context is fresh; defer implementation unless trivially small.
