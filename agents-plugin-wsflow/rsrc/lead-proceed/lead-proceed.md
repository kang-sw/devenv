---
kind: print
---

# Proceed

Target: user request

## Invariants

- Keep a direct execution within the requested scope; stop before it expands.
- Reload `{{.McpNamespace}}/workflow_manual` after session compaction; recover key via `{{.SkillNamespace}}:lead-revive` if lost. Fresh start: call `{{.McpNamespace}}/workflow_manual(session_key: "obsidian-latch")`.
- Limit pre-route source reads to explicitly requested paths and direct-execution judgment.
- Treat an `route.resolve_proceed` verdict as authoritative; follow its `Next:` exactly.

## On: invoke

1. If `workflow_manual` was already loaded this session and no compaction or
   continuation occurred since, call only `{{.McpNamespace}}/git.status(session_key: <key>)`.
   Otherwise call `{{.McpNamespace}}/workflow_manual(session_key: <key>)` and
   `{{.McpNamespace}}/git.status(session_key: <key>)` in parallel.
2. For an inline target, apply `judge: direct-execution`.
   - On `Yes`, state the reason, perform and verify the bounded request
     directly, then return without calling `route.resolve_proceed`.
3. If the target references a ticket, read it. Apply remaining judgments and
   resolve facts.
4. Scope resolution before calling:
   - Unfinished phase = first phase with no `### Result` section; use it when the user names none.
   - Every phase has `### Result` → `scope_blocked=no-unfinished-phase`; do not route to promotion.
   - Container ticket (epic/workset) → `scope_blocked=container-ticket`.
   - Two or more explicit phases in one request → `scope_blocked=multiple-explicit-phases`.
5. Call `{{.McpNamespace}}/route.resolve_proceed(session_key: <key>, target: ..., facts: ...)`.
6. Follow `Next:` exactly.

## Judgments

### judge: actionable

| Decision | When |
|----------|------|
| No | Target does not name a concrete change, observable outcome, or accepted implementation direction |
| Yes | Target gives enough implementation intent to route without another design turn |

### judge: direct-execution

| Decision | When |
|----------|------|
| Yes | The inline request has clear local scope and verification, and neither planning nor independent review materially improves the outcome. |
| No | A ticket phase, unresolved choice, contract or canonical-flow impact, broad scope, or requested review is present. |
| Unknown | Treat as No. |

### judge: discussion-needed

| Decision | When |
|----------|------|
| Yes | User-blocking design choice, scope boundary, acceptance criterion, trade-off, or delegation decision remains open |
| Yes | Migration-anchor conflicts with the requested route |
| No | Missing spec addressing, frontmatter, focus hygiene, tests, or local implementation details can be resolved autonomously |
| No | Ticket promotion is mechanical or can be handled by `lead-write-ticket` |

### judge: needs-ticket

| Decision | When |
|----------|------|
| Yes | Accepted work spans multiple independently reviewable phases or needs pre-implementation contract/verification traceability beyond its eventual implementation commit and any relevant existing spec |
| No | Accepted work is one bounded reviewable slice recoverable from its eventual implementation commit plus any relevant existing spec, regardless of file count or public surface |

## Doctrine

Proceed optimizes for **workflow attention**: reserve the full pipeline for work
where planning or independent review changes the outcome; complete bounded work
directly when it does not. When ambiguous, preserve the user's intervention point.
