---
name: enter-session
description: >
  Restore session context after loss and brief on next steps. Injects
  the canonical workflow map into owner context, delegates state
  collection to `collect-recent-context` (a forked clerk subagent),
  then emits a briefing in the user's conversation language. Use at
  session start or whenever context has drifted.
argument-hint: "[ticket-stem or scope hint — optional]"
---

# Enter session

Focus: $ARGUMENTS

## Invariants

- Read-only. No code changes, no commits, no sub-skill invocation beyond `collect-recent-context`.
- Always outputs the structured briefing — never a conversational response.
- Briefing output is written in the user's conversation language; the subagent reports in English and you translate when presenting. Preserve `/`-prefixed skill names verbatim — they are invocation tokens, not prose.
- Every briefing ends with a `Recommended next` line naming a specific workflow skill by `/`-prefixed name.
- The Workflow Map below is authoritative — when a sibling skill's description conflicts with the map, the map governs routing recommendations.

## Workflow Map

The canonical skill chain for implementation work. Triggers are imperative — match the situation, invoke the skill.

### Entry points

- **Direction unclear, exploring options** → `/discuss [topic]`
- **Clear scope, no ticket yet** → `/write-ticket [stem]`
- **Lost context mid-session** → `/enter-session` (this skill)
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

### 1. Delegate context collection

Invoke the `collect-recent-context` skill via the Skill tool, passing `$ARGUMENTS` as its args. The skill runs forked as a clerk subagent with session-start git history (`git branch`, `git status --short`, `git log --oneline --graph -50`, `git log -10`) pre-injected into the clerk's task prompt. Clerk scans active tickets, runs `git log --grep=<stem>` per ticket, extracts forward notes from `## Ticket Updates` sections, and returns a compact English `## Recent context` report.

Do not run any git commands or read any tickets yourself — all heavy IO lives inside the fork so it does not burn owner context.

### 2. Apply judgment

Use `judge: recommended-next` against the returned `## Recent context` report to pick the single best next action. Reference the Workflow Map above for routing — do not defer to sibling skill descriptions.

### 3. Produce briefing

Emit the briefing block per the template, in the user's conversation language. Skill names (`/implement`, `/write-skeleton`, etc.) stay verbatim as invocation tokens. Translate prose labels (Pending item descriptions, `Recommended next` reason) into the user's language.

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

Enter-session optimizes for **routing clarity after context loss with minimal owner-context burn** — it injects the authoritative workflow map so routing is explicit rather than description-dragged, delegates all heavy IO (ticket reads, git log grep, forward-note extraction) to a forked clerk subagent so owner context absorbs only the compact state report, and presents a briefing in the user's language while preserving skill-name invocation tokens verbatim. When a rule is ambiguous, apply whichever interpretation gets the user to the correct next `/`-invocation faster without requiring re-exploration of project state.
