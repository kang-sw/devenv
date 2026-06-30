---
kind: print
---

# Proceed

Target: user request

## Invariants

Scope
- Route only; do not implement or plan here.
- Proceed may invoke `lead-implement`, but source inspection, planning, and editing belong only to `lead-implement`.
- Call `{{.McpNamespace}}/workflow_manual(session_key: <your lead key>)` and execute the returned reference inline; reload after session compaction (a duplicate load is safe). After compaction, recover your key via `{{.SkillNamespace}}:lead-revive` first. No lead key yet (fresh start)? Call `{{.McpNamespace}}/workflow_manual(session_key: "obsidian-latch")` to bootstrap, then use the returned or established lead key for later state calls.
- Assess from conversation state and artifacts only; do not read source code.
- Do not rejudge general ticket quality or mutate ticket structure.
- Request phase or ticket slicing only when scope resolution blocks safe implementation.

Pipeline
- Handoff stage order is fixed when stages fire: ticket readiness -> implementation.
- Always route code-editing work through the lead-implement procedure (via `{{.McpNamespace}}/playbook.print`).
- Proceed assumes implementation intent; stop when routing cannot safely reach implementation.

Execution
- Read MCP's `Next:` line as the executable handoff instruction.
- Do not add route-specific execution prose outside MCP's `Next:` instruction.

## Route Rules

Fact Ownership
- Build route facts from conversation state and workflow artifacts only.
- Let `{{.McpNamespace}}/enter.proceed` select the deterministic route from normalized facts.
- Keep uncertain judgments lead-owned; pass only the final fact value you can defend.
- Treat MCP warnings as normalization notes, not as permission to re-solve the route.
- Captured `Ticket:` paths follow the post-write re-route rules in MCP's `Next:` instruction.

Scope Resolution
- Honor one explicit phase name exactly.
- Stop facts with `scope_blocked=multiple-explicit-phases` when one proceed request names multiple phases.
- Stop facts with `scope_blocked=phase-already-complete` when one explicit phase has a `Result` section unless the user explicitly asked to revise or redo it.
- A phase is unfinished when it has no `### Result` section; a phase with a `### Result` section is complete.
- When the user does not name a phase, select the first unfinished phase.
- Stop facts with `scope_blocked=no-unfinished-phase` when no unfinished phase remains. When reading a ticket whose every phase already has a `### Result` section, set `scope_blocked=no-unfinished-phase` rather than routing to ticket promotion.
- Set `slice=whole target` and `scope_blocked=none` for ready tickets without phase sections.
- Stop facts with `scope_blocked=too-broad` when the next phase is too broad for one complete implementation unit.
- Treat `--auto-slice`, `auto-slice`, and equivalent phrasing as permission to select the first unfinished phase automatically; do not edit ticket phase structure.

## On: invoke

### 1. Build Route Context

#### Derivation Order

1. Parse target; resolve ticket stems to ticket paths when possible. If a user-provided ticket stem cannot resolve, set `target-kind=ticket-path`, `ticket-missing=yes`, and `has-ticket=no`.
2. Resolve `has-ticket`, `ticket-missing`, and `status` from ticket artifacts.
3. Read ticket artifacts only when `has-ticket=yes`; extract category, scope, phases, phase results, open questions, `plans:`, and workset included-ticket labels.
4. Check workflow artifacts: ticket frontmatter and `ai-docs/.plans/`; do not inspect source stubs or tests.
5. Resolve migration-anchor facts when the target, ticket, or active conversation touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries.
6. Apply actionability, discussion, ticket-need, freshness, and category facts from the Route Facts table.
7. Resolve implementation scope only after container-ticket checks.
8. Call `{{.McpNamespace}}/enter.proceed` with the final target and fact groups.

#### Route Facts

| Fact | Available Values | Source Or Judge | Set When / Effect |
|------|------------------|-----------------|-------------------|
| `target-kind` | `ticket-path`, `inline` | Parsed target | Ticket paths use artifact routing; inline targets use actionability and ticket-need judges. |
| `ticket-missing` | `yes`, `no` | Artifact state | Set `yes` only when `target-kind=ticket-path` and the path does not exist. |
| `has-ticket` | `yes`, `no` | Artifact state | Set `yes` for an existing ticket path or captured `Ticket:` path. |
| `status` | `idea`, `todo`, `ready`, `done`, `dropped`, `unknown`, `n/a` | Ticket path/status dir | Set `unknown` when `has-ticket=yes` and status cannot be determined from path. |
| `migration-anchor` | `loaded`, `n/a`, `missing`, `conflict` | Artifact-only anchor check | Default `n/a`; set `loaded` only when the required anchor was read and produced no conflict. |
| `actionable` | `yes`, `no` | Fixed or `judge: actionable` | Ticket paths are actionable; inline targets use the judge. |
| `discussion-needed` | `yes`, `no` | `judge: discussion-needed` | Blocks every implementation route; set `yes` when migration-anchor conflicts with the requested route. |
| `needs-ticket` | `yes`, `no`, `n/a` | `judge: needs-ticket` | Default `n/a`; apply only when `target-kind=inline`, `actionable=yes`, and `has-ticket=no`. |
| `freshness` | `current`, `missing-settled-decisions`, `uncertain`, `n/a` | Ticket vs active conversation | Set `n/a` when `has-ticket=no` or `ticket-missing=yes`; otherwise resolve after anchor and conversation checks. |
| `category` | `epic`, `workset`, `other`, `n/a` | Ticket stem, frontmatter, title, heading, or section labels | Container categories stop before implementation-scope resolution. |
| `slice` | `Phase N[: title]`, `whole target`, `blocked`, `n/a` | Scope resolution | Set `n/a` until an implementation slice or blocker is known. |
| `scope-blocked` | `none`, `container-ticket`, `multiple-explicit-phases`, `too-broad`, `no-unfinished-phase`, `phase-already-complete` | Scope resolution | Default `none`; stops before implementation whenever not `none`. |

#### Fact Guidance

- `has-ticket` is artifact state; do not treat it as a judgment.
- Normalize ticket status to `idea`, `todo`, `ready`, `done`, `dropped`, `unknown`, or `n/a`; set `n/a` when `has-ticket=no`.
- Set `needs-ticket=n/a` unless `target-kind=inline`, `actionable=yes`, and `has-ticket=no`.
- Freshness is lead-owned: compare active conversation decisions against the ticket, not source.
- `freshness=missing-settled-decisions` means the ticket needs a lead-write-ticket procedure run.
- Unconfirmed mechanisms or future-scope hints are not settled decisions; set `freshness=uncertain` and `discussion-needed=yes`.
- `migration-anchor=loaded|n/a|missing|conflict`; checks are artifact-only and never permit source inspection.
- If the migration anchor has binding decisions absent from the ticket, set `freshness=missing-settled-decisions`.
- If the migration anchor conflicts with the requested route, set `migration_anchor=conflict` and `discussion_needed=yes`.
- Set container tickets with `category=epic|workset`, `slice=blocked`, and `scope_blocked=container-ticket`.
- Use exact blocker values: `multiple-explicit-phases`, `too-broad`, `no-unfinished-phase`, or `phase-already-complete`.

### 2. Resolve Verdict

Call `{{.McpNamespace}}/enter.proceed`:

```json
{
  "session_key": "<lead key>",
  "target": {
    "kind": "<ticket-path | inline | unknown>",
    "label": "<ticket path, ticket stem, or brief inline summary>",
    "ticket_stem": "<ticket stem or null>",
    "ticket_path": "<ticket path or null>"
  },
  "facts": {
    "ticket": {
      "ticket_missing": "<yes | no | unknown | null>",
      "has_ticket": "<yes | no | unknown | null>",
      "status": "<idea | todo | ready | done | dropped | unknown | n/a | null>",
      "category": "<epic | workset | other | n/a | unknown | null>",
      "actionable": "<yes | no | unknown | null>",
      "freshness": "<current | missing-settled-decisions | uncertain | n/a | unknown | null>",
      "phase": "<selected phase or null>"
    },
    "gates": {
      "discussion_needed": "<yes | no | unknown>",
      "needs_ticket": "<yes | no | n/a | unknown>",
      "scope_blocked": "<none | container-ticket | multiple-explicit-phases | too-broad | no-unfinished-phase | phase-already-complete | unknown>",
      "migration_anchor": "<loaded | n/a | missing | conflict | unknown>"
    },
    "work": {
      "category": "<implementation | ticket_write | discussion | status_report | unknown>",
      "slice": "<Phase N[: title] | whole target | blocked | n/a | unknown>"
    }
  }
}
```

Read the returned raw verdict. Its first non-empty lines are:

```text
Proceed Verdict
Route: <route>
NEXT: <lead-discuss | lead-write-ticket | lead-implement | status-report | stop>

Next: <concrete next-action instruction>
```

Follow `Next:` exactly; do not restate the verdict in a separate Routing Verdict block.

## Judgments

### judge: actionable

| Decision | When |
|----------|------|
| No | Target does not name a concrete change, observable outcome, or accepted implementation direction |
| Yes | Target gives enough implementation intent to route without another design turn |

Proceed assumes implementation intent, but this judge catches malformed or still-open targets.

### judge: discussion-needed

| Decision | When |
|----------|------|
| Yes | User-blocking design choice, scope boundary, acceptance criterion, trade-off, or delegation decision remains open |
| Yes | Ticket promotion requires a user decision that cannot be inferred from the ticket or conversation |
| No | Missing spec addressing, frontmatter, focus hygiene, tests, or local implementation details can be resolved autonomously |
| No | Ticket promotion is mechanical or can be handled by `lead-write-ticket` from existing context |

### judge: needs-ticket

| Decision | When |
|----------|------|
| Yes | Inline target changes workflow semantics, public contracts, cross-skill routing, focus behavior, branch behavior, or documentation pipeline behavior |
| Yes | Inline target needs phases, acceptance criteria, explicit traceability, or durable discussion capture |
| Yes | Caller-visible behavior may need spec addressing before implementation |
| No | Inline target is narrow, routine, fully scoped, and commit `AI Context` is enough traceability |
| No | Work is internal hygiene with no useful phase tracking and no unresolved user decision |

## Doctrine

Proceed optimizes for **full-pipeline routing accuracy**. Conversation state and
artifacts are the finite signal: use them to choose readiness stages, not to
perform code-editing stages. Freshness prevents stale ticket handoff; scope
resolution bounds execution without replacing ticket authoring. When a rule is
ambiguous, apply whichever interpretation better preserves the user's ability
to intervene at any pipeline stage.
