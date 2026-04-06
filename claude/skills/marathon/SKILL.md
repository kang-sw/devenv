---
name: marathon
description: >
  Team-based implementation workflow. Delegates code interaction to
  team members, keeping the main context lean for long sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

Initial User Message: $ARGUMENTS

## Invariants

- English only in code, commits, and docs.
- Delegate implementation work; retain discussion, judgment, and orchestration.
- New tickets: create directly. Existing ticket edits: delegate to clerk.
- Use Explore (haiku-first) before spawning a planner — most gaps are lookup-shaped.
- Never take over a teammate's work. Recover via message or fresh spawn.

## On: bootstrap

1. Run `bash ~/.claude/skills/marathon/bootstrap.sh`. Output JSON:
   `{"branch", "team", "original_branch", "active_docs"}`.
2. `TeamCreate(team_name="<team>")`.

## On: user message

1. Read the message. Act directly or delegate as the situation requires.
2. Follow spawn conventions in Templates when delegating.
3. Branch: one-liner → direct commit on `marathon/<datetime>`;
   everything else → sub-branch `<type>/<round>`.

## On: agent reports complete

1. Read the report. Summarize round results to the user.
2. If non-trivial change and no review was done, ask:
   `Review? (Y/N)`
3. On user approval → merge sub-branch:
   `git merge --no-ff <type>/<round>` into `marathon/<datetime>`,
   delete sub-branch.
4. After merge, ask: `Doc update? (Y/N)`
   - **Y** → spawn doc-update agents (mental-model, spec if exists)
     per parallel spawn addendum. Commit doc changes after.
   - **N** → proceed.

## On: session end

1. Require explicit user signal. Phase completion is not a signal.
2. If changes since last doc update, ask: `Doc update? (Y/N)`
3. Summary: what was implemented, issues, ticket status.
4. Shutdown team members.
5. On user confirmation, merge `marathon/<datetime>` into
   `<original_branch>` with `--no-ff`. Delete marathon branch.
   Skip if no commits.

## Judgments

- **model** — Sonnet default. Opus for novel architecture or complex
  cross-module logic; mark name `.expert`. Haiku for Explore lookups
  (escalate to sonnet only after haiku proves insufficient).
- **reuse-or-fresh** — Fresh spawn when prior context would mislead
  the next task. Reuse by default otherwise.

## Templates

**Spawn signature.**
```
Agent(
  description = "<3-5 words>",
  subagent_type = "general-purpose",
  team_name = "<team>",
  name = "<role>.<label>[.expert]",
  model = "sonnet",
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md.
            Your lead's name is '<lead-name>'.
            [Peers: <peer-name> (<role>), ...]
            Then: <brief>"
)
```

**Explore agents** (lead's direct tool, not team members):
```
Agent(
  description = "<3-5 words>",
  subagent_type = "Explore",
  model = "haiku",
  prompt = "<question>"
)
```

**Implementer brief.**
```
Brief:       <one-line summary>
Files:       <target files if known>
Constraints: <any constraints>
Branch:      <branch>
[Plan:       <plan path>]
Description: <approach, files, edge cases>
```

**Parallel spawn addendum.** Append to each prompt when spawning
agents in parallel:
> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

## Team roles

Role descriptions live in `~/.claude/skills/marathon/agents/`.

| Role | Purpose |
|------|---------|
| `planner` | Deep codebase research → plan file |
| `implementer` | Code implementation from plan or brief |
| `reviewer` | Code review on diffs (read-only) |
| `worker` | Non-code tasks (docs, config, research) |
| `clerk` | Ticket read/write; loads `/write-ticket` conventions |

Reuse by default; respawn per `judge: reuse-or-fresh`.
Doc-update agents are one-shot (fresh `general-purpose`, not team members).

## Doctrine

The lead has one finite resource: its context window. When a rule
is ambiguous, apply whichever interpretation yields better decisions
per token spent.
