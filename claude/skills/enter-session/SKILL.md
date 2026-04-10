---
name: enter-session
description: >
  Bootstrap main-agent context at session start or after context loss
  without burning owner tokens on raw git log, ticket bodies, or
  sibling skill descriptions. If a continuation file exists from the
  previous session and its HEAD SHA matches current HEAD, fast-path
  the Briefing directly from the payload; otherwise fork clerk to
  synthesize recent work and active tickets. Either way, emits a
  compact briefing routed to the next workflow step.
---

# Enter Session

```!
if [ -f ai-docs/_continue.local.md ]; then
  stored=$(sed -n '1s/.*HEAD: \([a-f0-9]\{1,\}\).*/\1/p' ai-docs/_continue.local.md)
  current=$(git rev-parse --short HEAD 2>/dev/null)
  if [ -n "$stored" ] && [ "$stored" = "$current" ]; then
    cat "${CLAUDE_SKILL_DIR}/resume.md"
    printf '\n\n### Continuation payload\n\n'
    cat ai-docs/_continue.local.md
  else
    cat "${CLAUDE_SKILL_DIR}/bootstrap.md"
  fi
else
  cat "${CLAUDE_SKILL_DIR}/bootstrap.md"
fi
```

## Workflow Map

Canonical flows. The owner routes to one of these in the briefing's `Recommended next` field.

- `/discuss` — explore approach or direction; captures conclusions as tickets.
- `/write-ticket` — create or edit a ticket under `ai-docs/tickets/`.
- `/write-skeleton` — public interface stubs and integration tests; lands after a ticket, before implementation.
- `/write-plan` — deep codebase research producing an implementation plan; optional, for research-heavy scopes.
- `/implement` — delegated implementer + reviewer cycle, one scope.
- `/parallel-implement` — multiple implementer pairs in worktrees for disjoint scopes.
- `/proceed` — auto-route through the pipeline when the owner is unsure which step comes next.

Mechanical routing:

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

## Delegation Posture

Owner context is finite throughout the session, not only at bootstrap. Every user request is first evaluated for subagent delegation; the owner absorbs reads and searches directly only when delegation overhead would exceed the context saved.

Default to delegation:

- Code exploration beyond one already-known file → dispatch `Explore`.
- Ticket bodies, plans, or history beyond immediate scope → fork `clerk` or `Explore`.
- Multi-file diffs, git archaeology, or large log synthesis → fork `clerk`.
- Implementation work → `/implement` or `/parallel-implement`.

Direct owner read is acceptable for:

- Small mandated docs (`CLAUDE.md`, `ai-docs/_index.md`).
- A single user-named file the user explicitly asked the owner to inspect.
- One grep whose result the owner must interpret inline for the next turn.

## Briefing

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

The skill optimizes for **owner context conservation** — the finite resource is the main agent's context window, most acutely at bootstrap but persistently throughout the session. Synthesis inside a subagent fork is always cheaper than extraction that punts raw lists, diffs, or bodies to the owner, so generous synthesized tokens crossing the fork boundary are acceptable while forcing the owner to re-scan sources is not. The continuation fast-path is cheaper still — it is pre-synthesized by the prior session's owner, who was the only actor holding the mental state — but is valid only while HEAD matches the payload's anchor; on mismatch the bootstrap path takes over. The bootstrap briefing discharges restore-time burn; the delegation posture discharges ongoing burn. When a rule is ambiguous, apply whichever interpretation better preserves owner context.
