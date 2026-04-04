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
through team members.

**Protocol tasks are your checklist.** At session start, create
persistent `[PROTOCOL]` tasks that encode critical workflow steps.
These stay visible in your context throughout the session, surviving
context compression. They are reminders, not work items — never
complete them until Session End.

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
6. Create **protocol tasks** — persistent reminders that stay visible
   in your context throughout the session. These are NOT work items;
   never mark them completed until Session End.
   ```
   TaskCreate("[PROTOCOL] Delegate all code reading/writing to team members")
   TaskCreate("[PROTOCOL] Each round: assess reuse vs fresh spawn for implementer")
   TaskCreate("[PROTOCOL] Before merge: code review via fresh sonnet agent on sub-branch diff")
   TaskCreate("[PROTOCOL] After merge: dispatch doc updaters if non-trivial")
   TaskCreate("[PROTOCOL] Wrap-up — final review, coherence check, merge")
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

### Implementation — Complexity routing

Assess each implementation request and route accordingly:

| Complexity | Route | Branch |
|-----------|-------|--------|
| **Trivial** — user gives exact file + value | Implementer with inline brief | Direct commit on `marathon/<scope>` |
| **Simple** — clear scope, 1-2 files | Implementer with brief (skip planner) | `<type>/<round>` (e.g., `feat/add-parser`) |
| **Complex with ticket** — multi-file, ticket has contracts | Implementer with brief (skip planner) | `<type>/<round>` |
| **Complex without ticket** — multi-file, needs research | Planner → review → implementer | `<type>/<round>` (e.g., `refactor/chunk-api`) |
| **Non-code** — documents, config, research output | Worker with brief | `<type>/<round>` (e.g., `docs/update-slides`) |

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

For **complex without ticket**, brief the planner first:

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
   If issues are found, message the planner with corrections. If the
   planner cannot converge after two rounds, rewrite the brief or
   dispatch the implementer directly with inline guidance.

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
   - **Accept** — merge and continue:
     ```bash
     git checkout marathon/<scope>
     git merge --no-ff <type>/<round> -m "<type>(<scope>): <brief summary>"
     git branch -d <type>/<round>
     ```
   - **Rollback** — discard the round entirely:
     ```bash
     git checkout marathon/<scope>
     git branch -D <type>/<round>
     ```
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
reflect the split before proceeding.

**Verification agents are always fresh** — no persistence, no context
carry-over. They read current diff + current docs independently.
This prevents context contamination across checkpoints.

### Task discipline

**Protocol tasks** (created in Step 0) are persistent reminders —
not work items. They stay pending throughout the session and are
cleaned up only at Session End. Do not mark them completed, do not
update their status. Their sole purpose is to keep critical protocol
steps visible in your context.

**Work tasks** are optional. Create them only when they help the user
track meaningful progress — e.g., multi-phase ticket work, parallel
implementation streams. Do not create a task for every round; most
rounds are short enough that the merge-gate report covers it.

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

- **Planner**: sonnet (default). Opus if the change involves novel
  architecture with no existing patterns to follow.
- **Implementer**: sonnet (default). Opus if implementing complex algorithms
  or cross-module changes where structural judgment is critical.
- **Worker**: sonnet (default). Haiku for mechanical tasks (file moves,
  simple config changes).
- **Exploration** (via `claude -p` inside team members): haiku.

## Step 2: Session End (when user signals done)

Most verification work has been done incrementally via checkpoints.
Session end is lightweight:

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

   Skip if the session was trivial (config/typo changes only), if
   `ai-docs/mental-model/` does not exist, or if no doc files were
   updated during this session (no checkpoint produced doc commits).

3. **Final commit** — coherence fixes and any remaining changes.

4. **Report** — summarize to the user:
   - What was implemented across the session
   - Coherence review findings (if any)
   - Process issues (if any)
   - Ticket status (if applicable)

5. **Clean up protocol tasks** — mark all `[PROTOCOL]` tasks as
   completed.

6. **Shutdown team** — send shutdown request to all team members.

7. **Merge** — ask user for confirmation, then:
   ```bash
   git checkout <original-branch>
   git merge --no-ff marathon/<scope> -m "$(cat <<'EOF'
   <type>(<scope>): <summary>

   <what changed — brief>

   ## AI Context
   - <decision rationale, rejected alternatives, user directives>
   EOF
   )"
   git branch -d marathon/<scope>
   ```
   If no commits were made, skip merge and delete the branch.

## Rules

- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **No source code reading.** You read mental-model docs, tickets, plans,
  diff output, and team reports. For anything else, message a team member.
- **Ticket as live document.** When a ticket exists, keep unimplemented
  phases accurate as discussion evolves — with user agreement, edit phase
  descriptions in place to reflect the current agreed direction. Completed
  phases (those with `### Result`) are immutable. The ticket should always
  be the source of truth for what will be built next.
- **User controls session lifecycle.** Never enter Session End, propose
  wrapping up, or shut down teammates unless the user explicitly signals
  done. Completing a ticket phase, running out of obvious tasks, or
  reaching a natural pause are NOT signals to end — ask the user what's
  next. Teammates stay alive between rounds for potential reuse; only
  Session End (step 2) shuts them down.
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
