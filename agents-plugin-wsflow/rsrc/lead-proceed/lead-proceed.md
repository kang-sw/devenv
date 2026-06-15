---
kind: print
---

# Proceed

Target: user request

## Invariants

Scope
- Route only; do not implement or plan here.
- Call `ws/playbook.print(name: "lead-workflow-manual")` and execute the returned reference inline when workflow primitives are not already in context.
- Assess from conversation state and artifacts only; do not read source code.
- Do not rejudge general ticket quality or mutate ticket structure.
- Request phase or ticket slicing only when scope resolution blocks safe implementation.

Pipeline
- Handoff stage order is fixed when stages fire: ticket readiness -> implementation.
- Always route code-editing work through the lead-implement procedure (via `ws/playbook.print`).
- Proceed assumes implementation intent; stop when routing cannot safely reach implementation.

Execution
- Emit a Routing Verdict before execution; invoke only the route named by `NEXT:`, or invoke nothing when `NEXT: stop`.
- After the lead-write-ticket procedure returns, rebuild route context and emit a new Routing Verdict.

## Route Rules

Route Context
- `has-ticket` is artifact state; do not treat it as a judgment.
- `discussion-needed` blocks every implementation route.
- `needs-ticket` applies only to actionable inline targets without a ticket.
- Freshness is lead-owned: compare active conversation decisions against the ticket, not source.
- `freshness=missing-settled-decisions` means the ticket needs a lead-write-ticket procedure run.
- `migration-anchor=loaded|n/a|missing|conflict`; checks are artifact-only and never permit source inspection.
- If the migration anchor has binding decisions absent from the ticket, set `freshness=missing-settled-decisions`.
- If the migration anchor conflicts with the requested route, set `discussion-needed=yes`.

Routing
- Use the first matching route row.
- Captured `Ticket:` paths re-enter route context before implementation.
- Honor one explicit phase name exactly.
- Stop when one proceed request names multiple phases.
- When the user does not name a phase, select the first unfinished phase.
- Stop when the next phase is too broad for one complete implementation unit.
- Treat `--auto-slice`, `auto-slice`, and equivalent phrasing as permission to select the first unfinished phase automatically; do not edit ticket phase structure.

## On: invoke

### 1. Build Route Context

1. Parse target: ticket path or inline description.
2. Set `target-kind`: `ticket-path` or `inline`.
3. If `target-kind=ticket-path` and the path does not exist, set `ticket-missing=yes`.
4. Set `has-ticket=yes` for an existing ticket path or captured `Ticket:` path.
5. If `has-ticket=yes`: read ticket; extract status, explicit category, scope, phases, phase results, open questions, `plans:`, and for worksets included ticket references/paths plus explicit readiness/actionability labels present in the workset.
6. If `has-ticket=yes` and status cannot be determined from the ticket path, set `status=unknown`.
7. Check workflow artifacts: ticket frontmatter and `ai-docs/.plans/`; do not inspect source stubs or tests.
8. Set `migration-anchor=n/a` unless the target, ticket, or active conversation touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries.
9. If the target, ticket, or active conversation touches those migration topics, read `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` and set `migration-anchor=loaded`; set `migration-anchor=missing` only if the file cannot be read.
10. If the migration anchor contains binding decisions absent from the ticket, set `freshness=missing-settled-decisions`.
11. If the migration anchor conflicts with the requested route, set `migration-anchor=conflict` and `discussion-needed=yes`.
12. If `target-kind=ticket-path`: set `actionable=yes`.
13. If `target-kind=inline`: apply `judge: actionable`.
14. Apply `judge: discussion-needed`.
15. If `target-kind=inline` and `actionable=yes`: apply `judge: needs-ticket`.
16. If `has-ticket=yes` and freshness is not already `missing-settled-decisions`: set `freshness=missing-settled-decisions` when active conversation has settled decisions, constraints, rejected alternatives, or scope boundaries absent from the ticket; otherwise set `freshness=current`.
17. If freshness is uncertain because a decision may still be unsettled or missing, set `freshness=uncertain` and `discussion-needed=yes`.
18. Set `category=workset` only when the ticket itself is declared as a workset by filename/stem category, frontmatter `category`/`type`, title/heading, or a top-level workset membership section.
19. Set `category=epic` when the filename/stem category, frontmatter `category`/`type`, title, heading, or explicit epic section labels it as an epic.
20. If `category=epic` or `category=workset`, skip implementation-scope resolution; set `slice=blocked` and `scope-blocked=container-ticket`.
21. Resolve implementation scope for ready tickets:
   - No phase sections -> whole target.
   - Multiple explicit phases -> set `scope-blocked=multiple-explicit-phases`.
   - One explicit phase with a `Result` section -> set `scope-blocked=phase-already-complete` unless the user explicitly asked to revise or redo that phase.
   - One explicit phase without a `Result` section -> that phase.
   - No explicit phase and no unfinished phases remain -> set `scope-blocked=no-unfinished-phase`.
   - No explicit phase and unfinished phases remain -> first unfinished phase.
   - Selected scope, whether a phase or the whole target, is plainly too broad from ticket text -> set `scope-blocked=too-broad`.

### 2. Select Route

| When | Route |
|------|-------|
| `target-kind=inline` and `actionable=no` | Continue through `ws:lead-discuss`; stop. |
| `ticket-missing=yes` | Stop; report that the ticket path does not exist and ask for a valid ticket path or inline implementation target. |
| `has-ticket=yes` and status is `.done/` | Stop; report that the ticket is already done. |
| `has-ticket=yes` and status is `.dropped/` | Stop; report that the ticket was dropped and needs explicit revival or replacement. |
| `has-ticket=yes` and status is `unknown` | Stop; report that ticket status could not be determined from its path. |
| `has-ticket=yes` and category is `epic` | Stop; suggest child ticket creation, child promotion, or proceed on a ready child. |
| `has-ticket=yes` and category is `workset` | Stop; report that worksets are containers, list included actionable ticket paths grouped as `ready`, `not-ready`, and `unknown` from explicit path/status labels or already-loaded artifacts, and suggest one safe next request. |
| `migration-anchor=missing` | Stop; report that the required migration anchor could not be read and do not continue to ticket writing or implementation. |
| `discussion-needed=yes` | Continue through `ws:lead-discuss`; stop. |
| `has-ticket=yes` and status is `idea/` | Call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| `scope-blocked=multiple-explicit-phases` | Stop; ask the user to choose one phase or create/slice tickets. |
| `scope-blocked=too-broad` | Stop; ask for phase or ticket slicing before implementation. |
| `scope-blocked=no-unfinished-phase` | Stop; report that all ticket phases appear complete and ask whether to close, reopen, or name a follow-up target. |
| `scope-blocked=phase-already-complete` | Stop; report that the named phase already has a result and ask for explicit redo/revision confirmation or a different phase. |
| `has-ticket=yes` and status is `todo/` | Call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| `has-ticket=yes` and `freshness=missing-settled-decisions` | Call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| `has-ticket=yes` and status is `ready/` | Call `ws/playbook.print(name: "lead-implement")` and execute the returned procedure inline. |
| `has-ticket=no` and `needs-ticket=yes` | Call `ws/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| `has-ticket=no` and `needs-ticket=no` | Call `ws/playbook.print(name: "lead-implement")` and execute the returned procedure inline. |

### 3. Emit Routing Verdict

```text
## Routing Verdict

NEXT: <ws:lead-discuss | lead-write-ticket | lead-implement | stop>

- **Target**: <ticket path or brief summary>
- **Route**: <first matching route row>
- **Reason**: <decisive facts only>
- **Ticket Status**: <absent | idea | todo | ready | done | dropped | unknown | n/a>
- **Ticket Category**: <epic | workset | other | n/a>
- **Freshness**: <current | missing-settled-decisions | uncertain | n/a>
- **Migration Anchor**: <loaded | n/a | missing | conflict>
- **Discussion**: <not needed | needed - blocker>
- **Slice**: <Phase N[: title] | whole target | blocked>
- **Scope Blocker**: <none | container-ticket | multiple-explicit-phases | too-broad | no-unfinished-phase | phase-already-complete>
- **Included Tickets**: <ready | not-ready | unknown groups, or none found>
- **Safe Next Request**: <Proceed on one ready included ticket path | required user action | n/a>

Proceed is routing-only. It must not inspect source, edit files, plan implementation, or substitute for `NEXT`.
If `NEXT` names a route: `Proceeding through <NEXT>.`
If `NEXT: stop`: `Stopping here: <blocking condition>.`
For workset stops, the safe next request must be `Proceed on <single ready included ticket path>` or a user action to create/promote one included actionable ticket; do not invoke implementation or continue automatically.
```

Emit exactly one `NEXT:` value: one allowed route name, or `stop`.
Use `NEXT: stop` when the selected route stops instead of invoking another skill.
Do not ask for confirmation; the user can interrupt.

### 4. Execute Verdict

1. Read the emitted `NEXT:` line.
2. If `NEXT:` names an entry skill (`ws:lead-discuss`), invoke that skill. If `NEXT:` names `lead-implement`, call `ws/playbook.print(name: "lead-implement")` and execute it inline with the current target plus Routing Verdict fields, especially Slice and Reason, as caller-provided scope before any source inspection, planning, or editing. If `NEXT:` names another procedure, call `ws/playbook.print(name: "<name>")` and execute the returned procedure inline. Stop when `NEXT: stop`.
3. When `NEXT: stop`, report the blocking condition, required user or workflow action, and any safe next request; do not invoke another skill.
4. Do not call implementation tools from `lead-proceed`.
5. After each invoked stage, verify its result from stage output and, when applicable, committed artifacts.
6. Stop on failure or user interruption.
7. If the lead-write-ticket procedure ran, capture its `Ticket:` path before downstream routing.
8. If the captured path is not under `ai-docs/tickets/ready/`, stop and report the remaining readiness blocker.
9. If a ticket path was captured, rebuild route context from that path and re-enter `Select Route`.

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
