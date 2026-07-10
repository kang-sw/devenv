---
kind: print
---

# Proceed

Target: user request

## Invariants

- Route only; do not implement or plan here.
- Reload `{{.McpNamespace}}/workflow_manual` after session compaction; recover key via `{{.SkillNamespace}}:lead-revive` if lost. Fresh start: call `{{.McpNamespace}}/workflow_manual(session_key: "obsidian-latch")`.
- Build route facts from conversation state and workflow artifacts only; do not inspect source code.
- Always route code-editing work through `lead-implement` via `{{.McpNamespace}}/playbook.print`.
- Follow `Next:` from `enter.proceed` exactly; do not add route-specific prose.

## On: invoke

1. If `workflow_manual` was already loaded this session and no compaction or
   continuation occurred since, call only `{{.McpNamespace}}/git.status(session_key: <key>)`.
   Otherwise call `{{.McpNamespace}}/workflow_manual(session_key: <key>)` and
   `{{.McpNamespace}}/git.status(session_key: <key>)` in parallel.
2. If the target references a ticket, read it. Apply judgments and resolve facts.
3. Scope resolution before calling:
   - Unfinished phase = first phase with no `### Result` section; use it when the user names none.
   - Every phase has `### Result` → `scope_blocked=no-unfinished-phase`; do not route to promotion.
   - Container ticket (epic/workset) → `scope_blocked=container-ticket`.
   - Two or more explicit phases in one request → `scope_blocked=multiple-explicit-phases`.
4. Call `{{.McpNamespace}}/enter.proceed(session_key: <key>, target: ..., facts: ...)`.
5. Follow `Next:` exactly.

## Judgments

### judge: actionable

| Decision | When |
|----------|------|
| No | Target does not name a concrete change, observable outcome, or accepted implementation direction |
| Yes | Target gives enough implementation intent to route without another design turn |

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
| Yes | Inline target changes workflow semantics, public contracts, cross-skill routing, focus, branch, or documentation pipeline behavior |
| Yes | Inline target needs phases, acceptance criteria, traceability, or durable discussion capture |
| No | Inline target is narrow, routine, fully scoped, and commit `AI Context` is sufficient traceability |
| No | Work is internal hygiene with no useful phase tracking and no unresolved user decision |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose readiness stages, not to
perform code-editing stages. When a rule is ambiguous, apply whichever
interpretation better preserves the user's ability to intervene at any pipeline stage.
