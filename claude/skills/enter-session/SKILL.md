---
name: enter-session
description: >
  Bootstrap main-agent context at session start or after context loss
  without burning owner tokens on raw git log, ticket bodies, or sibling
  skill descriptions. Forks clerk to synthesize recent work and active
  tickets, then emits a compact briefing routed to the next workflow
  step.
---

# Enter Session

## Invariants

- Owner never runs `git log`, `git diff`, or reads ticket bodies under `ai-docs/tickets/` directly during bootstrap — all raw scanning happens inside the clerk fork.
- The clerk fork is scoped to context collection only — no ticket edits, no status transitions, no source or mental-model reads.
- The briefing is emitted as a single structured block matching the template — never prose, never merged sections, never reordered.
- Skill names in the briefing are `/`-prefixed tokens — never paraphrased, reformatted, or translated.
- Empty fields are omitted entirely rather than filled with placeholders.
- All output in English regardless of conversation language.

## On: invoke

1. Read `ai-docs/_index.md` directly — it is small, mandated by `CLAUDE.md`, and anchors the project-level truth the briefing depends on.
2. Spawn the clerk subagent with the **Clerk spawn prompt** template below. Wait for its report.
3. Consult the **Workflow Map** section against the clerk report and `_index.md` to pick the next step. Apply `judge: scope-complexity` and `judge: parallelizable` when the mechanical lookup leaves room.
4. Emit the **Briefing** template, filling fields from the clerk report and omitting empty ones.
5. Stop. Do not proceed into the recommended next step — the user decides.

## Workflow Map

Canonical flows. The owner routes to one of these in the briefing's `Recommended next` field.

- `/discuss` — explore approach or direction; captures conclusions as tickets.
- `/write-ticket` — create or edit a ticket under `ai-docs/tickets/`.
- `/write-skeleton` — public interface stubs and integration tests; lands after a ticket, before implementation.
- `/write-plan` — deep codebase research producing an implementation plan; optional, for research-heavy scopes.
- `/implement` — delegated implementer + reviewer cycle, one scope.
- `/parallel-implement` — multiple implementer pairs in worktrees for disjoint scopes.
- `/proceed` — auto-route through the pipeline when the owner is unsure which step comes next.

Mechanical routing (handler step 3):

- No active ticket, user exploring direction → `/discuss`
- Direction clear, no ticket yet → `/write-ticket`
- Ticket exists, no skeleton → `/write-skeleton`
- Skeleton exists, scope is research-heavy (see `judge: scope-complexity`) → `/write-plan`
- Skeleton exists, plan exists or not needed → `/implement`
- Multiple disjoint scopes ready (see `judge: parallelizable`) → `/parallel-implement`
- Any of the above is unclear → `/proceed`

## Judgments

**judge: scope-complexity** — Route to `/write-plan` when the ticket requires understanding three or more unfamiliar modules, introduces a new architectural pattern, or crosses established boundaries. Skip `/write-plan` for well-scoped changes with single-module impact.

**judge: parallelizable** — Route to `/parallel-implement` when the skeleton defines two or more scopes with no shared mutable state and independent test paths. Single-scope, interdependent, or sequentially-ordered work stays on `/implement`.

## Templates

### Clerk spawn prompt

```
Collect recent-work context for session bootstrap. Context collection only — no ticket edits, no status transitions, no source or mental-model reads.

Tasks:
1. Capture branch state: `git rev-parse --abbrev-ref HEAD` and `git status --short`.
2. Run `git log --oneline -15`. Synthesize into 2-4 thematic bullets (themes, not per-commit). Do not include raw log lines in output.
3. List ticket stems + titles under `ai-docs/tickets/wip/` and `ai-docs/tickets/todo/`.
4. For each `wip/` ticket, read the body and extract:
   - Purpose: 1-line paraphrase of the intent; quote-adjacent language, no editorializing.
   - Open threads: unresolved design questions, un-acted forward notes, pending decisions carried from the ticket body or recent commits. Omit the bullet entirely if none.

Output format:

### Branch
- <name> (<status summary>)

### Recent work
- <thematic bullet>
- <thematic bullet>

### Active (wip)
#### <stem>
Purpose: <1-line>
Open threads:
- <thread>
- <thread>

### Queue (todo)
- <stem>: <title>

Output in English.
```

### Briefing

```
## Briefing

### Context
- Branch: <name> (<status>)
- Recent work:
  - <thematic bullet>
  - <thematic bullet>
- Active: <stem> — <Purpose>
- Open threads:
  - <thread>
  - <thread>
- Queue: <N tickets> — <top 1-3 stems, or "none">

### Recommended next
`<skill>` — <one-line reason>
```

Per-field shape is flexible (multi-line, sub-bulleted, or omitted when empty). The `Recommended next` line stays rigid: backtick-quoted skill name followed by a one-line reason.

## Doctrine

The skill optimizes for **owner context burn at session restore** — the finite resource is the main agent's context window at bootstrap, which must absorb project memory, workflow knowledge, and current state without the cumulative cost of raw scanning. Synthesis inside the clerk fork is always cheaper than extraction that punts raw lists to the owner, so thousands of synthesized tokens crossing the fork boundary is acceptable while forcing the owner to re-scan sources is not. When a rule is ambiguous, apply whichever interpretation better preserves owner context at bootstrap.
