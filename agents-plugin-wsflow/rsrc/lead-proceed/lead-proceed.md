---
kind: print
---

# Proceed

Target: user request

## Invariants

- Keep a free-form execution (an early return that skips route, plan, and review) within the requested scope; stop before it expands.
- Reload `{{.McpNamespace}}/workflow_manual` after session compaction; recover key via `{{.SkillNamespace}}:lead-revive` if lost. Fresh start: call `{{.McpNamespace}}/workflow_manual(session_key: "obsidian-latch")`.
- Limit pre-route source reads to explicitly requested paths and what the free-form judgment needs.
- Treat an `route.resolve_proceed` verdict as authoritative; follow its `Next:` exactly.

## On: invoke

1. If `workflow_manual` was already loaded this session and no compaction or
   continuation occurred since, call only `{{.McpNamespace}}/git.status(session_key: <key>)`.
   Otherwise call `{{.McpNamespace}}/workflow_manual(session_key: <key>)` and
   `{{.McpNamespace}}/git.status(session_key: <key>)` in parallel.
2. For an inline target, apply `judge: free-form`.
   - On `Yes`, say so with the reason, do the work under project conventions,
     then return without calling `route.resolve_proceed`.
3. If the target references a ticket, read it. Apply remaining judgments and
   resolve facts.
4. Scope resolution before calling:
   - Unfinished phase = first phase with no `### Result` section; use it when the user names none.
   - Every phase has `### Result` → `scope_blocked=no-unfinished-phase`; do not route to promotion.
   - Container ticket (epic/workset) → `scope_blocked=container-ticket`.
   - Two or more explicit phases in one request → `scope_blocked=multiple-explicit-phases`.
5. Call `{{.McpNamespace}}/route.resolve_proceed(session_key: <key>, params: {target: ..., facts: ...})`.
6. Follow `Next:` exactly.

## Judgments

### judge: actionable

| Decision | When |
|----------|------|
| No | Target does not name a concrete change, observable outcome, or accepted implementation direction |
| Yes | Target gives enough implementation intent to route without another design turn |

### judge: free-form

| Decision | When |
|----------|------|
| Yes | Every touched path is a manual, note, or similar working document that no spec, mental model, or distributed artifact governs, regardless of file count, and no No row matches. |
| Yes | Otherwise, local scope and verification are clear and neither planning nor independent review materially improves the outcome. |
| No | A ticket phase or ticket edit, unresolved choice, contract or canonical-flow impact, or requested review is present. |
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

## Fact Contract

`{{.McpNamespace}}/route.resolve_proceed`'s published schema is opaque
(`params: object`); this table is the authoritative field contract the
resolver reads. Send an outer `session_key` and one `params` object containing
`target`, `facts`, and optional `format`; do not put a `session_key` in
`params` or mix typed fields into the outer envelope. Unwrapped typed calls
remain compatible.

`target`
| Field | Type | Notes |
|-------|------|-------|
| `kind` | `ticket-path\|inline\|unknown` | |
| `label` | string\|null | Short target label; defaults from path/stem/kind when omitted. |
| `ticket_stem` | string\|null | |
| `ticket_path` | string\|null | |

`facts.ticket`
| Field | Enum |
|-------|------|
| `ticket_missing` | `yes\|no\|unknown` |
| `has_ticket` | `yes\|no\|unknown` |
| `status` | `idea\|todo\|ready\|done\|dropped\|unknown\|n/a` |
| `category` | `epic\|workset\|other\|n/a\|unknown` |
| `actionable` | `yes\|no\|unknown` |
| `freshness` | `current\|missing-settled-decisions\|uncertain\|n/a\|unknown` |
| `phase` | string (free text) |

`facts.gates`
| Field | Enum |
|-------|------|
| `discussion_needed` | `yes\|no\|unknown` |
| `needs_ticket` | `yes\|no\|n/a\|unknown` |
| `scope_blocked` | `none\|container-ticket\|multiple-explicit-phases\|too-broad\|no-unfinished-phase\|phase-already-complete\|unknown` |
| `migration_anchor` | `loaded\|n/a\|missing\|conflict\|unknown` |

`facts.work`
| Field | Enum |
|-------|------|
| `category` | `implementation\|ticket_write\|discussion\|status_report\|unknown` |
| `slice` | string (free text) |

`format`: `text` (default) \| `json`. All fields are optional; unknown/null
values are normalized by the resolver.

## Doctrine

Proceed optimizes for **workflow attention**: reserve the full pipeline for work
where planning or independent review changes the outcome; complete working-document
and bounded work free-form when it does not. When ambiguous, preserve the user's intervention point.
