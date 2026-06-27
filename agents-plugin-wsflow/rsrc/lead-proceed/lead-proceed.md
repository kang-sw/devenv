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
- Emit a Routing Verdict before execution; invoke only the route named by `NEXT:`, or invoke nothing when `NEXT: stop`.
- After the lead-write-ticket procedure returns with a ready `Ticket:` path, rebuild route context and emit a new Routing Verdict; otherwise stop and report the readiness blocker.

## Route Rules

Route Context
- `has-ticket` is artifact state; do not treat it as a judgment.
- Normalize ticket status to `idea`, `todo`, `ready`, `done`, `dropped`, `unknown`, or `n/a`; set `n/a` when `has-ticket=no`.
- `discussion-needed` blocks every implementation route.
- Set `needs-ticket=n/a` unless `target-kind=inline`, `actionable=yes`, and `has-ticket=no`.
- Freshness is lead-owned: compare active conversation decisions against the ticket, not source.
- `freshness=missing-settled-decisions` means the ticket needs a lead-write-ticket procedure run.
- Unconfirmed mechanisms or future-scope hints are not settled decisions; set `freshness=uncertain` and return to discussion instead of writing them.
- `migration-anchor=loaded|n/a|missing|conflict`; checks are artifact-only and never permit source inspection.
- If the migration anchor has binding decisions absent from the ticket, set `freshness=missing-settled-decisions`.
- If the migration anchor conflicts with the requested route, set `discussion-needed=yes`.

Routing
- Use the first matching route row.
- Captured `Ticket:` paths follow the post-write re-route rules in Execute Verdict.
- Honor one explicit phase name exactly.
- Stop when one proceed request names multiple phases.
- When the user does not name a phase, select the first unfinished phase.
- Stop when the next phase is too broad for one complete implementation unit.
- Treat `--auto-slice`, `auto-slice`, and equivalent phrasing as permission to select the first unfinished phase automatically; do not edit ticket phase structure.

## On: invoke

### 1. Build Route Context

#### Derivation Order

1. Parse target; resolve ticket stems to ticket paths when possible; set `target-kind`.
2. Resolve `has-ticket`, `ticket-missing`, and `status` from ticket artifacts.
3. Read ticket artifacts only when `has-ticket=yes`; extract category, scope, phases, phase results, open questions, `plans:`, and workset included-ticket labels.
4. Check workflow artifacts: ticket frontmatter and `ai-docs/.plans/`; do not inspect source stubs or tests.
5. Resolve migration-anchor facts when the target, ticket, or active conversation touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries.
6. Apply actionability, discussion, ticket-need, freshness, and category facts from the Route Facts table.
7. Resolve implementation scope only after container-ticket checks.

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

#### Fact Rules

- If `migration-anchor=missing`, do not continue to ticket writing or implementation.
- If the migration anchor has binding decisions absent from the ticket, set `freshness=missing-settled-decisions`.
- For `has-ticket=yes`, set `freshness=current` after anchor and conversation checks find no missing settled decisions or uncertainty.
- If freshness may be unsettled, unconfirmed, future-scoped, or missing, set `freshness=uncertain` and `discussion-needed=yes`.
- Set `category=workset` only when the ticket itself is declared as a workset by filename/stem category, frontmatter `category`/`type`, title/heading, or a top-level workset membership section.
- Set `category=epic` when the filename/stem category, frontmatter `category`/`type`, title, heading, or explicit epic section labels it as an epic.
- Set `category=other` for ticket artifacts that are neither epic nor workset; set `category=n/a` when `has-ticket=no`.
- If `category=epic` or `category=workset`, set `slice=blocked` and `scope-blocked=container-ticket`.
- For actionable inline targets with `has-ticket=no` and `needs-ticket=no`, set `slice=whole target` and `scope-blocked=none`.
- For ready tickets with no phase sections, set `slice=whole target` and `scope-blocked=none`.
- For one request that explicitly names multiple phases, set `scope-blocked=multiple-explicit-phases`.
- For one explicit phase with a `Result` section, set `scope-blocked=phase-already-complete` unless the user explicitly asked to revise or redo that phase.
- For one explicit phase without a `Result` section, set `slice` to that phase and `scope-blocked=none`.
- For no explicit phase and no unfinished phases, set `scope-blocked=no-unfinished-phase`.
- For no explicit phase and unfinished phases, set `slice` to the first unfinished phase and `scope-blocked=none`.
- If the selected scope is plainly too broad from ticket text, set `scope-blocked=too-broad`.

### 2. Select Route

Use the first matching route row. Blank cells mean any value. Extra conditions in `When` must also hold.

| Priority | target-kind | has-ticket | status | category | freshness | discussion-needed | scope-blocked | When | NEXT / Route |
|----------|-------------|------------|--------|----------|-----------|-------------------|---------------|------|--------------|
| 1 | `inline` |  |  |  |  |  |  | `actionable=no` | Continue through `{{.SkillNamespace}}:lead-discuss`; stop. |
| 2 |  |  |  |  |  |  |  | `ticket-missing=yes` | Stop; report that the ticket path does not exist and ask for a valid ticket path or inline implementation target. |
| 3 |  | `yes` | `done` |  |  |  |  |  | Stop; report that the ticket is already done. |
| 4 |  | `yes` | `dropped` |  |  |  |  |  | Stop; report that the ticket was dropped and needs explicit revival or replacement. |
| 5 |  | `yes` | `unknown` |  |  |  |  |  | Stop; report that ticket status could not be determined from its path. |
| 6 |  | `yes` |  | `epic` |  |  |  |  | Stop; suggest child ticket creation, child promotion, or proceed on a ready child. |
| 7 |  | `yes` |  | `workset` |  |  |  |  | Stop; report that worksets are containers, list included actionable ticket paths grouped as `ready`, `not-ready`, and `unknown` from explicit path/status labels or already-loaded artifacts, and suggest one safe next request. |
| 8 |  |  |  |  |  |  |  | `migration-anchor=missing` | Stop; report that the required migration anchor could not be read and do not continue to ticket writing or implementation. |
| 9 |  |  |  |  |  | `yes` |  |  | Continue through `{{.SkillNamespace}}:lead-discuss`; stop. If `migration-anchor=conflict`, name the conflict in Reason. |
| 10 |  | `yes` | `idea` |  |  |  |  |  | Call `{{.McpNamespace}}/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| 11 |  |  |  |  |  |  | `multiple-explicit-phases` |  | Stop; ask the user to choose one phase or create/slice tickets. |
| 12 |  |  |  |  |  |  | `too-broad` |  | Stop; ask for phase or ticket slicing before implementation. |
| 13 |  |  |  |  |  |  | `no-unfinished-phase` |  | Stop; report that all ticket phases appear complete and ask whether to close, reopen, or name a follow-up target. |
| 14 |  |  |  |  |  |  | `phase-already-complete` |  | Stop; report that the named phase already has a result and ask for explicit redo/revision confirmation or a different phase. |
| 15 |  | `yes` | `todo` |  |  |  |  |  | Call `{{.McpNamespace}}/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| 16 |  | `yes` |  |  | `missing-settled-decisions` |  |  |  | Call `{{.McpNamespace}}/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| 17 |  | `yes` | `ready` |  | `current` | `no` | `none` |  | Call `{{.McpNamespace}}/playbook.print(name: "lead-implement")` and execute the returned procedure inline. |
| 18 |  | `no` |  |  |  |  |  | `needs-ticket=yes` | Call `{{.McpNamespace}}/playbook.print(name: "lead-write-ticket")` and execute the returned procedure inline; capture `Ticket:` and re-route. |
| 19 |  | `no` |  |  |  |  |  | `needs-ticket=no` | Call `{{.McpNamespace}}/playbook.print(name: "lead-implement")` and execute the returned procedure inline. |
| 20 |  |  |  |  |  |  |  | route facts are insufficient or inconsistent | Stop; report the missing or inconsistent route facts required to continue. |

### 3. Emit Routing Verdict

```text
## Routing Verdict

NEXT: <{{.SkillNamespace}}:lead-discuss | lead-write-ticket | lead-implement | stop>

- **Target**: <ticket path or brief summary>
- **Route**: <first matching route row>
- **Reason**: <decisive facts only>
- **Ticket Status**: <absent | idea | todo | ready | done | dropped | unknown | n/a>
- **Ticket Category**: <epic | workset | other | n/a>
- **Freshness**: <current | missing-settled-decisions | uncertain | n/a>
- **Migration Anchor**: <loaded | n/a | missing | conflict>
- **Discussion**: <not needed | needed - blocker>
- **Slice**: <Phase N[: title] | whole target | blocked | n/a>
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
Do not ask for confirmation before invoking a non-stop route; when `NEXT: stop`, ask only for the blocking user action required by the selected route.

### 4. Execute Verdict

1. Read the emitted `NEXT:` line.
2. If `NEXT:` names a downstream skill (`{{.SkillNamespace}}:lead-discuss`, `lead-write-ticket`, or `lead-implement`), call `{{.McpNamespace}}/enter.proceed(session_key: <lead key>, ticket: <Target ticket path or stem>, phase: <Slice>, next_skill: <NEXT value>, conditions: [<notable route-context flags, e.g. "freshness=<value>", "discussion=<value>", "scope-blocker=<value>">])` to record routing context before invoking the route.
3. If `NEXT:` names an entry skill (`{{.SkillNamespace}}:lead-discuss`), invoke that skill. If `NEXT:` names `lead-implement`, call `{{.McpNamespace}}/playbook.print(name: "lead-implement")` and execute it inline with the current target plus Routing Verdict fields, especially Slice and Reason, as caller-provided scope before any source inspection, planning, or editing. If `NEXT:` names another procedure, call `{{.McpNamespace}}/playbook.print(name: "<name>")` and execute the returned procedure inline. Stop when `NEXT: stop`.
4. When `NEXT: stop`, report the blocking condition, required user or workflow action, and any safe next request; do not invoke another skill.
5. Do not call implementation tools from `lead-proceed`.
6. After each invoked stage, verify its result from stage output and, when applicable, committed artifacts.
7. Stop on failure or user interruption.
8. If the lead-write-ticket procedure ran, capture its `Ticket:` path before downstream routing.
9. If the captured path is not under `ai-docs/tickets/ready/`, stop and report the remaining readiness blocker.
10. If a ticket path was captured, rebuild route context from that path and re-enter `Select Route`.

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
