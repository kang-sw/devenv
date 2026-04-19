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
disable-model-invocation: true
argument-hint: "[optional initial context]"
---

# Enter Session

Initial Context: $ARGUMENTS

## Invariants

- The Briefing is a structured block with `### Context` and `### Recommended next` sections — never unstructured prose.
- Skill names in the Briefing are `/`-prefixed tokens — never paraphrased, reformatted, or translated.
- Owner absorbs reads and searches directly only when delegation overhead would exceed the context saved.

### Delegation routing

Delegate:

- Code exploration beyond one already-known file → `Explore`.
- Ticket bodies, plans, or history beyond immediate scope → `clerk` or `Explore`.
- Multi-file diffs, git archaeology, or large log synthesis → `clerk`.
- Cold or wide-scope implementation → `/delegate-implement` (single) or `/parallel-implement` (disjoint multi-scope).
- Warm single-scope implementation → `/implement` (owner edits directly — not delegation, but the canonical entry point).

Read directly:

- Small mandated docs (`CLAUDE.md`, `ai-docs/_index.md`).
- A single user-named file the user explicitly asked the owner to inspect.
- One grep whose result the owner must interpret inline for the next turn.

## Judgments

**judge: scope-complexity** — Route to `/write-plan` when the ticket requires understanding three or more unfamiliar modules, introduces a new architectural pattern, or crosses established boundaries. Skip `/write-plan` for well-scoped changes with single-module impact.

**judge: parallelizable** — Route to `/parallel-implement` when the skeleton defines two or more scopes with no shared mutable state and independent test paths. Single-scope, interdependent, or sequentially-ordered work routes to `/implement` (warm owner, direct) or `/delegate-implement` (cold owner or wide scope).

**judge: warmth** — Owner is warm on the target when prior session turns read files in the scope or the user explicitly signaled direct authorship. Warm + small + single-scope → `/implement`. Otherwise → `/delegate-implement`.

## Workflow Map

Canonical flows. The owner routes to one of these in the briefing's `Recommended next` field.

- `/discuss` — explore approach or direction; captures conclusions as tickets.
- `/write-ticket` — create or edit a ticket under `ai-docs/tickets/`.
- `/write-skeleton` — public interface stubs and integration tests; lands after a ticket, before implementation.
- `/write-plan` — deep codebase research producing an implementation plan; optional, for research-heavy scopes.
- `/implement` — owner-direct cycle, one scope; for warm sessions where the owner edits, verifies, and commits without delegation.
- `/delegate-implement` — delegated implementer + reviewer cycle, one scope; for cold sessions or when delegation improves reliability.
- `/parallel-implement` — multiple implementer pairs on a shared branch with run_request-serialized execution, for disjoint scopes.
- `/proceed` — auto-route through the pipeline when the owner is unsure which step comes next.

Mechanical routing:

- No active ticket, user exploring direction → `/discuss`
- Direction clear, no ticket yet → `/write-ticket`
- Ticket exists, no skeleton → `/write-skeleton`
- Skeleton exists, scope is research-heavy (see `judge: scope-complexity`) → `/write-plan`
- Skeleton exists, plan exists or not needed, owner is warm on the target (see `judge: warmth`) → `/implement`
- Skeleton exists, plan exists or not needed, owner is cold or scope is wide → `/delegate-implement`
- Multiple disjoint scopes ready (see `judge: parallelizable`) → `/parallel-implement`
- Any of the above is unclear → `/proceed`

## Briefing

```
## Briefing

### Context
<free-form structured content>

### Recommended next
`<skill>` — <one-line reason>
```

**Context** may include any combination of the following — include what is relevant, omit what is not, add unlisted items when they aid orientation:

- Branch name and status
- Recent work (thematic bullets, not raw commit lines)
- Active ticket stem and purpose
- Open threads or unresolved questions
- Queue depth and top stems

**Recommended next** stays rigid: backtick-quoted skill name followed by a one-line reason.

## Doctrine

The skill optimizes for **owner context conservation** — the finite resource is the main agent's context window, most acutely at bootstrap but persistently throughout the session. Synthesis inside a subagent fork is always cheaper than extraction that punts raw lists, diffs, or bodies to the owner, so generous synthesized tokens crossing the fork boundary are acceptable while forcing the owner to re-scan sources is not. The continuation fast-path is cheaper still — it is pre-synthesized by the prior session's owner, who was the only actor holding the mental state — but is valid only while HEAD matches the payload's anchor; on mismatch the bootstrap path takes over. The bootstrap briefing discharges restore-time burn; the delegation posture discharges ongoing burn. When a rule is ambiguous, apply whichever interpretation better preserves owner context.

---

!`bash "${CLAUDE_SKILL_DIR}/dispatch.sh"`
