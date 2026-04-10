---
name: brief
description: >
  Restore session context and brief on next steps. Loads current state
  (branch, WIP, active ticket, pending artifacts) and recommends the
  specific workflow skill to invoke next. Also injects the canonical
  workflow map so downstream routing is explicit, not description-dragged.
disable-model-invocation: true
argument-hint: "[ticket-stem or scope hint — optional]"
---

# Brief

Focus: $ARGUMENTS

## Invariants

- Read-only. No code changes, no commits, no sub-skill invocation.
- Always outputs the structured briefing — never a conversational response.
- Every briefing ends with a `Recommended next` line naming a specific workflow skill by `/`-prefixed name.
- Does not re-load what CLAUDE.md session-start already loaded; assumes `ai-docs/_index.md` and recent `git log` are in context.
- The Workflow Map section is authoritative — when a sibling skill's description conflicts with the map, the map governs routing recommendations.

## Workflow Map

The canonical skill chain for implementation work. Triggers are imperative — match the situation, invoke the skill.

### Entry points

- **Direction unclear, exploring options** → `/discuss [topic]`
- **Clear scope, no ticket yet** → `/write-ticket [stem]`
- **Lost context mid-session** → `/brief` (this skill)
- **Know the target, want auto-routing** → `/proceed [ticket or description]`

### Implementation chain

1. **Ticket exists, change touches public contracts or cross-module boundaries** → `/write-skeleton [ticket]`
2. **Skeleton exists, scope is complex or unfamiliar** → `/write-plan [ticket]` (consumes skeleton contracts as locked inputs)
3. **Skeleton + plan (if needed) ready, single cohesive scope** → `/implement [plan or brief]`
4. **Skeleton defines 2+ disjoint scopes** → `/parallel-implement [ticket]`

### Precedence rules

- `/write-skeleton` before `/write-plan` — the plan consumes skeleton contracts.
- `/parallel-implement` requires a skeleton — it partitions skeleton-defined scopes.
- `/implement` may internally invoke `/write-plan` for complex scopes without requiring a standalone plan file.
- `/proceed` subsumes the routing decision — use it when the target is clear and execution should proceed without further discussion.

## On: invoke

### 1. Load state

Gather facts without reading source code. All data comes from git and ticket metadata.

1. Current branch: `git branch --show-current`.
2. Last commit: `git log -1 --oneline`.
3. Working tree status: `git status --short`.
4. If `$ARGUMENTS` names a ticket stem, read that ticket. Otherwise scan `ai-docs/tickets/` for tickets with unfinished phases.

### 2. Scan pending artifacts

1. For each active ticket, inspect frontmatter `skeletons:` and `plans:` fields to identify which phases have artifacts and which remain.
2. If current branch matches `implement/<scope>`, note it as in-progress implementation.
3. If uncommitted changes exist, classify as: WIP on a known phase, orphan changes, or merge conflict.

### 3. Apply judgment

Use `judge: recommended-next` to pick the single best next action based on state. Name the specific skill.

### 4. Produce briefing

Emit the briefing block per the template. Do not add conversational prose before or after — the block is the entire response.

## Judgments

### judge: recommended-next

| State | Recommendation |
|-------|----------------|
| Uncommitted WIP on an `implement/<scope>` branch | Resume with implementer via team rejoin, or commit/stash and `/implement` fresh |
| Active ticket, unfinished phase with no skeleton, change touches public contracts | `/write-skeleton <ticket>` |
| Active ticket, unfinished phase with skeleton but no plan, scope is complex | `/write-plan <ticket>` |
| Active ticket, skeleton exists (+ plan if complex), phase unimplemented | `/implement <plan-or-ticket>` or `/parallel-implement <ticket>` if skeleton has disjoint scopes |
| Multiple active tickets with unfinished phases | List them, ask user to pick — do not guess |
| No active ticket, clear scope in `$ARGUMENTS` | `/write-ticket <stem>` |
| No active ticket, no clear scope | `/discuss` |
| State is clear and target is obvious | `/proceed <target>` — subsumes the above chain |

## Templates

### briefing-block

```
## Briefing

### State
- Branch: <branch>
- Last commit: <hash> <subject>
- WIP: <short status or "clean">
- Active ticket: <ticket stem or "none">

### Pending
- <open phase / skeleton-only scope / unimplemented plan / ...>
- <additional items, one per line>

### Recommended next
`<skill>` — <one-line reason>
```

Omit a bullet entirely when it has no content (e.g., no WIP → drop the line). Do not emit placeholder text.

## Doctrine

Brief optimizes for **routing clarity after context loss** — it restores
enough state signal to pick the right next action and injects the
authoritative workflow map so the recommendation is explicit rather than
description-dragged. When a rule is ambiguous, apply whichever
interpretation gets the user to the correct next `/`-invocation faster
without requiring re-exploration of project state.
