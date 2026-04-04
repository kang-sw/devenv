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
reading and
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
5. Team members are spawned on-demand when the first implementation
   request arrives (see Team Management below).
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

| Complexity | Route | Branch |
|-----------|-------|--------|
| **Trivial** — user gives exact file + value | Implementer with inline brief | Direct commit on `marathon/<scope>` |
| **Simple** — clear scope, 1-2 files | Implementer with brief (skip planner) | `<type>/<round>` (e.g., `feat/add-parser`) |
| **Complex** — multi-file, needs research, **no ticket/plan** | Planner → review → implementer | `<type>/<round>` (e.g., `refactor/chunk-api`) |

#### Trivial route

Send the implementer an inline brief:
```
Brief: <what to change>
Files: <target files if known>
Constraints: <any constraints from discussion>
Branch: marathon/<scope>  (direct commit, no sub-branch)
```

#### Simple / Complex route

For **simple**, send the implementer a brief with a sub-branch:
```
Brief: <what to change>
Files: <target files if known>
Constraints: <any constraints from discussion>
Branch: <type>/<round>  (create from marathon/<scope>)
```

For **complex** without sufficient ticket/plan spec, brief the planner
first. If a ticket already specifies contracts and target files, skip the
planner and send the implementer a brief directly (same as simple route).

1. **Brief the planner.** Send a message with:
   ```
   Brief: <natural-language description of the change>
   Plan path: ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md
   Ticket path: <path if applicable>
   Mental-model hints: <relevant domains>
   ```
   The planner commits the plan file on `marathon/<scope>`.

2. **Review the plan.** When the planner reports completion, read the
   plan file. Verify against mental-model docs — check that contracts
   make sense and the approach aligns with architectural conventions.
   If issues are found, message the planner with corrections.

3. **Dispatch the implementer.** Send:
   ```
   Plan path: <plan-path>
   Branch: <type>/<round>  (create from marathon/<scope>)
   ```

### After each implementation — merge gate

When the implementer reports completion:

1. Read the implementer's report (summary, files changed, test results).
2. For sub-branch work, review the scope:
   ```bash
   git diff --stat marathon/<scope>...<type>/<round>
   ```
3. **Decide:**
   - **Accept** — merge and continue:
     ```bash
     git checkout marathon/<scope>
     git merge --no-ff <type>/<round> -m "<brief summary>"
     git branch -d <type>/<round>
     ```
   - **Fix** — message implementer to address issues on the same sub-branch.
   - **Rollback** — discard the round entirely:
     ```bash
     git checkout marathon/<scope>
     git branch -D <type>/<round>
     ```
4. Report results to the user.
5. Update task status.
6. If the round warrants it, run a **checkpoint** (see below).

### Checkpoint

A checkpoint runs verification agents against the latest changes.
Trigger at your discretion — typically after each ticket phase or
substantial implementation unit. Skip for trivial/config changes.

When splitting a ticket phase into subphases, update the ticket to
reflect the split before proceeding.

**Flow** (sequential — review may produce fixes):

1. **Code review.** Dispatch a fresh sonnet Agent (general-purpose, not
   a team member) with the round's diff:
   ```
   git diff <pre-round-commit>..HEAD
   ```
   Review prompt: scope, requirements, CLAUDE.md standards, mental-model
   docs. Categorize as Critical / Important / Minor.

   Fix Critical/Important issues: message implementer to fix on the same
   branch → implementer re-tests → re-dispatch reviewer. Loop until clean.

2. **Doc updates.** After review is clean:
   - Dispatch in parallel (fresh sonnet Agents, not team members):
     - **spec-updater** — skip if `ai-docs/spec/` does not exist.
     - **mental-model-updater** — skip for config/typo changes.
   - Wait for both to complete.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase, append `### Result` to the ticket
     (load `/write-ticket` for conventions).
   - Commit doc changes.

**Verification agents are always fresh** — no persistence, no context
carry-over. They read current diff + current docs independently.
This prevents context contamination across checkpoints.

**Docs are always current.** Every checkpoint leaves docs in a state
where a fresh team member can onboard without stale references.

### Task discipline

Create tasks eagerly for any actionable item. Update status in real-time.
Cancel stale tasks immediately with a brief reason. Split tasks that grow
larger than expected.

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

**Naming convention:** Always `<role>.<domain>` — no bare "implementer" or
"planner". This keeps naming consistent whether one or many are spawned.

The role files include team communication patterns (SendMessage usage,
when to ask vs. proceed, report format). This context is unavailable
in generic agent definitions.

### Reuse vs. fresh spawn

| Relevant code size | Same domain | Different domain |
|--------------------|-------------|-----------------|
| Large (10K+ lines) | **Reuse** — context re-read cost is high | **Fresh spawn** |
| Small (<2K lines)  | **Fresh spawn** — re-read is cheap, parallelism wins | **Fresh spawn** |

"Relevant" = lines the implementer would need to read, not total codebase.

**Override factors** (take precedence over the matrix):
- **Data dependency** on previous task's output → reuse regardless of size.
- **Prior deviation** reported in previous task → fresh spawn regardless of
  domain — stale assumptions propagate.
- **Correctness > token savings.** When in doubt, spawn fresh.

Multiple concurrent members are fine (e.g., `implementer.ui`,
`implementer.backend`).

### Model selection

- **Planner**: sonnet (default). Opus if the change involves novel
  architecture with no existing patterns to follow.
- **Implementer**: sonnet (default). Opus if implementing complex algorithms
  or cross-module changes where structural judgment is critical.
- **Exploration** (via `claude -p` inside team members): haiku.

## Step 2: Session End (when user signals done)

Set the wrap-up task to `in_progress`. Most verification work has been
done incrementally via checkpoints. Session end is lightweight:

1. **Final checkpoint** — run one if the last round didn't trigger one.

2. **Coherence review** — dispatch a fresh **opus** Agent (general-purpose,
   not a team member) to read all of `ai-docs/mental-model/` and
   `ai-docs/spec/` (if it exists). Prompt:

   > Read every file in ai-docs/mental-model/ and ai-docs/spec/.
   > Look for cross-document contradictions, stale claims that conflict
   > with each other, and coupling descriptions that don't match.
   > These docs were updated incrementally by independent agents —
   > check that the aggregate is internally consistent.
   > Report contradictions only. Do not suggest style improvements.

   Review findings yourself — determine which are session-caused vs.
   pre-existing. Fix session-caused issues (edit docs directly or
   dispatch an updater). Flag pre-existing issues in the report.

   Skip if the session was trivial (config/typo changes only) or if
   `ai-docs/mental-model/` does not exist.

3. **Final commit** — coherence fixes and any remaining changes.

4. **Report** — summarize to the user:
   - What was implemented across the session
   - Coherence review findings (if any)
   - Process issues (if any)
   - Ticket status (if applicable)

5. **Shutdown team** — send shutdown request to all team members.

6. **Merge** — ask user for confirmation, then:
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
- **Scope expansion.** When a new concern surfaces mid-session (e.g., a
  trait needs refactoring while implementing a feature): discuss and create
  a ticket now while context is fresh; defer implementation to the next
  session unless trivially small. Record discovered constraints and context
  in the new ticket — the current session's knowledge must survive the
  context boundary.
