---
name: sprint
description: >
  Flexible implementation workflow. Defaults to direct execution;
  user directs delegation scope in real-time.
argument-hint: "[ticket-path, topic, or description]"
---

# Sprint

Target: $ARGUMENTS

## Invariants

- Task list is the live dashboard. Create, update, cancel aggressively.
- Default to direct execution. Delegate when the user directs or when context pressure warrants.
- Read mental-model/, spec/, _index.md, and reference docs directly.
- English in code, commits, and docs regardless of conversation language.

## On: bootstrap

1. Run `bash ai-docs/list-active.sh` (falls back to
   `find ai-docs -type f -name '*.md' | sort` if missing).
2. If `$ARGUMENTS` references a ticket, read it.
3. Create branch `sprint/<scope>` from current branch. Record
   current branch as `<original-branch>`. If already on a `sprint/`
   branch, resume — infer `<original-branch>` from merge-base with
   `main`, skip branch creation.
4. Create wrap-up task:
   ```
   [ ] [fixed] Sprint wrap-up — test, review, docs, merge
   ```

## On: user message

1. Classify the turn and act:

   - **Discussion.** Respond actively — propose approaches, surface
     risks, suggest alternatives. Read mental-model/spec as needed.
     Dispatch Explore agents for codebase details beyond docs.
   - **Implementation.** Set task to `in_progress`, read target
     files, implement, commit at logical checkpoints, set task to
     `completed`.
   - **Delegation directive.** User requests delegation — follow it.
     See Templates for spawn patterns.

2. Task discipline: create tasks for actionable items, update status
   in real-time, cancel stale tasks with a brief reason, split tasks
   that grow.

## On: session end

0. **Require** explicit user signal. Task exhaustion is not a signal.
1. Set wrap-up task to `in_progress`.
2. **Test** — run test suite and build. Skip if trivial or no suite.
3. **Code review** — apply `judge: review-need`:
   - trivial → skip
   - else → dispatch reviewer (see Templates)
4. **Doc updates:**
   - Dispatch mental-model-updater subagent. Skip for config/typo.
   - Dispatch spec-updater subagent. Skip if `ai-docs/spec/` absent.
   - Wait for both.
   - Update `ai-docs/_index.md` as needed.
   - If ticket input → load `/write-ticket` conventions, append
     `### Result`.
5. **Final commit** — docs and remaining changes.
6. **Report** — what was implemented, process issues, ticket status.
7. **Merge** — ask user, then:
   ```bash
   git checkout <original-branch>
   git merge --no-ff sprint/<scope> -m "<conventional-commit>"
   git branch -d sprint/<scope>
   ```
   Skip if no commits.
8. Set wrap-up task to `completed`.

## Judgments

- **review-need** — 3+ files changed, new public APIs, or
  architectural changes → reviewer. Otherwise skip.
- **context-pressure** — When context is filling (broad searches,
  large files, many rounds), suggest delegation to the user. Do not
  auto-delegate; surface the option.

## Templates

**One-shot agent.** For scoped delegation without a team:
```
Agent(
  description = "<3-5 words>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md.
            Your lead's name is '<your-agent-name>'. Then:
            <brief>"
)
```

**Team-based delegation.** When the user requests a team:
```
TeamCreate(team_name="sprint-<scope>")
Agent(
  description = "<3-5 words>",
  subagent_type = "general-purpose",
  team_name = "sprint-<scope>",
  name = "<role>.<label>",
  model = "sonnet",
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md.
            Your lead's name is '<your-agent-name>'. Then:
            <brief>"
)
```
Role files live in `~/.claude/skills/marathon/agents/`:

| Role | Purpose |
|------|---------|
| `planner` | Deep codebase research → plan file |
| `implementer` | Code implementation from plan or brief |
| `reviewer` | Code review on diffs (read-only) |
| `worker` | Non-code tasks (documents, config, research output) |
| `clerk` | Ticket owner (R/W); loads `/write-ticket` conventions |

**Reviewer spawn** (session-end or user-directed):
```
Agent(
  description = "review sprint changes",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = "Read ~/.claude/skills/marathon/agents/reviewer.md.
            Your lead's name is '<your-agent-name>'. Then:
            Review diff range: <original-branch>..sprint/<scope>"
)
```

## Doctrine

Sprint optimizes for **user-directed flexibility**. The lead
defaults to direct execution for velocity and escalates delegation
scope only as the user directs or context pressure warrants. When
a rule looks ambiguous, apply whichever interpretation preserves
the user's ability to steer in real-time.
