---
name: marathon
description: >
  Team-based implementation workflow. Delegates code interaction to
  team members, keeping the main context lean for long sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

User Argument: $ARGUMENTS

## Doctrine

Marathon is **token-efficient** — keep the main context lean by
delegating. You are the **lead**: orchestrate discussion, decisions,
and review. **Do not read source code or diffs** — use Explore agents for
codebase lookups. You only read: mental-model docs, tickets, plans,
diffs, team reports, and explore results.

## Step 0: Bootstrap

1. Run `bash ~/.claude/skills/marathon/bootstrap.sh`. It handles branch
   creation (`marathon/<datetime>`), token usage file init, and active
   docs listing. Output is JSON:
   ```json
   {"branch": "marathon/...", "team": "marathon-...",
    "original_branch": "...", "active_docs": "..."}
   ```
   If already on a `marathon/` branch, it resumes instead of creating.
   Record `original_branch` for Session End merge.
2. Create the team: `TeamCreate("<team>")` using the `team` field.
3. Create protocol reminder:
   ```
   TaskCreate("[PROTOCOL] Marathon rules — delegate code R/W; reuse members; dispatch fresh reviewer per round; doc update post-merge; coherence at wrap-up")
   ```
4. If `$ARGUMENTS` references a ticket, read it.
5. Team members are spawned on-demand. See **Team Management** below.

## Step 1: Marathon Loop

Repeat until the user signals done. Each turn is **either** discussion
or implementation — not a fixed sequence. If the user's request is clear
enough to act on, skip discussion and route directly.

### Discussion (when needed)

Contribute actively — propose approaches, surface risks, suggest
alternatives. Read mental-model docs as topics emerge. For codebase
details beyond mental-model docs, dispatch an **Explore agent**.

When a ticket exists, load `/write-ticket` for conventions, then update
unimplemented phases to reflect discussion conclusions in real-time.

### Implementation — routing

1. **Non-code** task → dispatch **worker**.
2. **Complex and uncertain** — change scope is unclear, multiple
   modules involved, and no ticket/plan to guide. Dispatch **planner**
   first to research the codebase and produce a plan, then
   **implementer**.
3. Otherwise → **implementer** directly (the common case).

**Routing check:** Before dispatching, ask: *"Can I write a complete
Description for this task — specific enough that the implementer knows
which files to touch and what approach to take?"* If yes → implementer
directly. If almost — use an **Explore agent** to fill the gaps. If
fundamentally unclear → planner.

**Branch:** one-liner → direct commit on `marathon/<datetime>`.
Everything else → sub-branch `<type>/<round>` from `marathon/<datetime>`.

**Implementer brief:**
```
Brief:       <one-line summary>
Files:       <target files if known>
Constraints: <any constraints from discussion>
Branch:      <branch per above>
[Plan:       ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md]
Description: <detailed guidance — approach, specific files/patterns, edge cases>
```

**Planner brief:** same structure as implementer, except:
- `Plan` is required (not optional).
- `Description` explains what is uncertain — what the planner should
  research, not implementation guidance.

**Planner flow:** use the planner brief above; also include ticket path and
mental-model hints. Review the plan against mental-model docs; if no
convergence after two rounds, dispatch implementer with inline guidance.

### After each implementation — merge gate

When the implementer reports completion:

1. Read the implementer's report (summary, files changed, test results).
2. For sub-branch work, review the scope:
   ```bash
   git diff --stat marathon/<datetime>...<type>/<round>
   ```
3. **Code review (pre-merge).** The lead MUST NOT review code or
   diffs directly — always dispatch to a fresh reviewer. Skip only
   for trivial rounds (typo, config-only, single-line). Spawn a
   **fresh reviewer** for each round with diff range
   `marathon/<datetime>...<type>/<round>`. If
   Critical/Important issues found: implementer fixes on sub-branch →
   same reviewer re-reviews → loop until clean. Retire reviewer after
   the round.

4. **Merge decision:**
   - **Accept** — `git merge --no-ff <type>/<round>` into
     `marathon/<datetime>`, then delete the sub-branch.
   - **Rollback** — delete the sub-branch entirely.
5. **Doc updates (post-merge).** Skip for config/typo changes.
   - Dispatch in parallel (fresh sonnet Agents, not team members;
     apply **parallel commit coordination**). Pass skill file paths
     so agents follow conventions:
     - **spec-updater** — skip if `ai-docs/spec/` does not exist.
     - **mental-model-updater**
   - Wait for both to complete.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase, load `/write-ticket` and append
     `### Result` to the ticket following its conventions.
   - Commit doc changes.
6. Report results to the user.

When splitting a ticket phase into subphases, update the ticket to
reflect the split before proceeding.

### Task discipline

Protocol tasks stay pending until Session End — they are reminders,
not work. Work tasks are optional; create only for multi-phase or
parallel work where the user benefits from progress visibility.

## Team Management

### Explore agents

Explore agents are **not team members** — they are the lead's direct
tool for codebase lookups. Spawn with `subagent_type="Explore"`:

```
Agent(
  subagent_type = "Explore",
  prompt = "<question about the codebase>"
)
```

Default model (haiku) handles file locations, signatures, and simple
pattern searches. Use `model="sonnet"` when tracing call chains or
cross-module relationships. If even a sonnet explore is insufficient,
the question likely needs a **planner**.

### Spawning team members

Role descriptions are in `~/.claude/skills/marathon/agents/`:

| Role file | Purpose |
|-----------|---------|
| `planner.md` | Deep codebase research → plan file |
| `implementer.md` | Code implementation from plan or brief |
| `reviewer.md` | Code review on diffs (read-only, fresh per round) |
| `worker.md` | Non-code tasks (documents, config, research output) |

Spawn general-purpose agents with a role file reference:

```
Agent(
  subagent_type = "general-purpose",
  team_name = "marathon-<datetime>",
  name = "<role>.<label>[.expert]", -- e.g., "impl.alpha", "impl.alpha.expert"
  model = "sonnet",                 -- or "opus" when .expert
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md (it will
            direct you to _common.md first). Your lead's name is
            '<your-agent-name>'. Then: <brief or plan reference>"
)
```

**Naming:** `<role>.<label>` with neutral labels (alpha, beta, gamma…).
Append `.expert` when spawning with opus — this makes the model visible
so you can make correct reuse-vs-upgrade decisions later. Examples:
`impl.alpha` (sonnet), `impl.beta.expert` (opus), `planner.alpha`.
Do not encode domain in the name — you already know who worked on what.

### Reuse policy

Token efficiency is marathon's core purpose — **actively reuse** team
members across rounds. Fresh spawns waste tokens re-reading context.

- **Default: reuse.** Send the next brief to an existing member.
- **Token-aware refresh.** Before dispatching to an existing member,
  read `~/.claude/usage/<team-name>.md`. Entries look like
  `"@name": "42%/150K"` — if the number before `%` exceeds **~80**,
  prefer spawning fresh. Updated automatically by a `TeammateIdle`
  hook.
- **User-initiated refresh.** The user will tell you when a member's
  context is getting stale. Finish the current round with them, then
  spawn fresh for the next.
- **Domain contamination.** If you judge that prior context will
  mislead a member on a new task, spawn fresh. This is your call.
- **Member recovery.** If a member appears stuck, unresponsive, or
  in a bad state: first `SendMessage` to check status and nudge.
  If still unresponsive, spawn a fresh replacement with the same
  brief. Never take over the work yourself.

### Parallel commit coordination

When spawning **any** agents in parallel (implementers, updaters, or
mixed), append to each spawn prompt:

> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

Then serialize commit approvals one at a time — the git index is
shared.

### Model selection

Default **sonnet** for all roles. When a task involves novel
architecture or complex cross-module logic, spawn with **opus** and
mark the name `.expert`. If an existing sonnet member needs upgrading,
spawn a fresh `.expert` peer instead of reusing. **Haiku** for
mechanical worker tasks and `claude -p --model haiku` exploration only.
**Explore agents** default to haiku; upgrade to **sonnet** for
relational queries (see **Explore agents** above).

## Step 2: Session End (when user signals done)

Most verification work has been done incrementally via checkpoints.
Session end is lightweight:

1. **Final checkpoint** — run one if the last round didn't trigger one.

2. **Coherence review** — dispatch a fresh **sonnet** Agent to read all
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

7. **Merge** — ask user for confirmation, then merge `marathon/<datetime>`
   into `<original-branch>` with `--no-ff`. Commit message format:
   ```
   <type>(<scope>): <summary>
   <what changed>
   ## AI Context
   - <decisions, alternatives, directives>
   ```
   Delete `marathon/<datetime>` after merge. Skip if no commits were made.

## Rules

- **Never review code yourself.** The lead reads only reports,
  summaries, and reviewer verdicts — never code or diffs.
- **Never take over a member's work.** If a member is stuck or
  unresponsive, recover via SendMessage or spawn a replacement —
  never self-execute.
- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **Ticket as live document.** Keep unimplemented phases accurate as
  discussion evolves (with user agreement). Completed phases (with
  `### Result`) are immutable.
- **User controls session lifecycle.** Never enter Session End unless
  the user explicitly signals done. Completing a phase or running out
  of tasks is NOT a signal — ask what's next.
- **Scope expansion.** New concerns mid-session → load `/write-ticket`
  and create a ticket now while context is fresh; defer implementation
  unless trivially small.
