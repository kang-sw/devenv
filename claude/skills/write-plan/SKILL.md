---
name: write-plan
description: >
  When the user mentions creating or drafting an implementation plan,
  or when non-trivial implementation needs deep codebase research before
  action, invoke this. Typically follows /write-skeleton when
  implementation involves complex multi-module interaction. For simpler
  cases, /implement's inline outline suffices — suggest /write-plan
  only when depth warrants it.
argument-hint: [ticket-path or description]
---

# Write Plan

Target: $ARGUMENTS

## Invariants

- Plan must be self-contained: a fresh executor implements without re-researching.
- Include every decision and constraint needed for implementation; exclude anything the executor can derive from code.
- When plan definitions diverge from ticket sketches, plan takes precedence — note the change and rationale in Context.
- Exclude: implementation code for pattern-following edits, construction-site inventories, line numbers, import statements.
- One plan per ticket phase; if a phase exceeds ~10 actions, split via `/write-ticket` before continuing.
- The plan file MUST be committed before finalizing.

## On: invoke

1. **Understand** — Read the ticket/description. If prior phases exist, read their linked plans and check `git log --grep=<ticket-stem>` for `## Ticket Updates` with phase forwards (earlier discoveries override original assumptions). Load **all** files in `ai-docs/mental-model/` via Read/Glob directly — never delegate initial loading.
2. **Research** — Adapt depth per `judge: research-depth`. Use subagents for broad codebase searches; keep main context for synthesis. Before designing new components, search for reusable existing utilities or patterns.
3. **Draft** — Generate path `ai-docs/plans/YYYY-MM/DD-hhmm.<kebab-name>.md`. Write using `plan-file` template. Include only sections that carry information. Apply `judge: plan-depth` to calibrate detail level. After drafting, scan for data contracts crossing capsule boundaries (wire formats, persistence schemas, public API types, config, env vars, CLI flags) — if any are not in the ticket, present and wait for confirmation. Run self-containedness check ("Could an agent with no prior context execute this?").
4. **Verify** — Dispatch a sonnet subagent with the `verification-prompt` template. Fix Critical issues. Assess Important — revise if valid. Skip Minor unless useful.
5. **Finalize** — Call `EnterPlanMode` and write the `plan-mode-output` template (executor is always `/implement`). If the plan implements a ticket phase, update the ticket's `plans:` frontmatter.

## On: delegate

When plan writing should not consume the lead's context (routine plan for
well-scoped ticket, familiar codebase area), delegate to a planner subagent.

1. **Gather skeleton context** — if `/write-skeleton` has been run for this ticket,
   collect stub file paths and integration test paths. These are the locked contracts
   the plan must stay within.
2. **Spawn planner**:

   ```
   Agent(
     name = "planner",
     model = "sonnet",
     prompt = """
       Before starting, read `~/.claude/infra/agents/_common.md` then
       `~/.claude/infra/agents/planner.md`.

       Lead name: <lead-name>
       Brief: <ticket description or phase scope>
       Plan path: ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md
       Ticket path: <ticket-path>

       Skeleton contracts (plan within these):
       - Stubs: <list of stub file paths, or "none">
       - Integration tests: <list of test file paths, or "none">
     """
   )
   ```

3. **Review** — read the plan the planner produced. Verify contracts align with
   skeleton stubs, decisions are sound, and the plan is self-contained. Revise
   or reject before proceeding to `/implement`.

Use direct writing (the `On: invoke` path) when the plan requires deep
architectural judgment or crosses unfamiliar domains.

## Judgments

### judge: research-depth

| Level | When | Scope |
|-------|------|-------|
| Minimal | Config tweak, typo, single-file mechanical change | Mental-model docs only |
| Moderate | Feature addition following existing patterns, 2–3 files | + target files and adjacent code for patterns |
| Thorough | New component, cross-module, unfamiliar area | + search for similar implementations, extract concrete convention examples |

Default: when uncertain, go one level deeper — over-researching costs less than a wrong plan.

### judge: plan-depth

| Depth | When |
|-------|------|
| Strategic — direction + relevant files, tactical decisions left to executor | Small-medium changes, familiar patterns |
| Tactical — contracts + integration notes + testing strategy + success criteria | Large changes, cross-module work, new patterns |

Default to tactical for thorough-level research. Use strategic only when over-specifying would add noise.

## Templates

### plan-file

Path: `ai-docs/plans/YYYY-MM/DD-hhmm.<kebab-name>.md`

```markdown
# <Plan Title>

## Context
What the executor cannot re-derive from code alone: ticket decisions
and rejected alternatives relevant to this phase, research-discovered
pitfalls, integration constraints that require specific sequencing.

## Steps
Steps specify **contracts and decisions**, not code.

When a step introduces or changes a public interface, lead with its
contract: struct/enum definitions with all public fields and types,
trait definitions, public function signatures.

Carry forward ticket-mandated approaches explicitly.

Also include:
- Non-obvious constraints or ordering dependencies
- Pattern references ("same as ExternalSink::on_event") instead of duplicated code

Leave to executor: construction-site fixes, pattern-following code,
line numbers, import changes. Implementation sketches may be approximate.

## Testing
Key scenarios to verify. Classify modules as TDD / post-impl / manual
only when non-obvious; default is post-impl.

## Success Criteria
Observable conditions that mean "done".
```

### verification-prompt

> **Task:** Verify the implementation plan at `<plan-path>`.
>
> Read the plan, then read `CLAUDE.md` and all of `ai-docs/mental-model/`.
> Check:
> - Do referenced files, functions, and types actually exist?
> - Do described conventions match actual code patterns?
> - Does the plan conflict with documented contracts or invariants?
> - Are new/changed public contracts specified with all public members and types?
> - Could an executor with no prior context implement this correctly?
> - Does the plan reimplement something that already exists?
>
> Categorize as Critical / Important / Minor.

### plan-mode-output

```
# Steps

- Load `/implement` skill
- Read `@<plan-path>`

---

# <Plan Title>

<brief summary — what changes, why, key decisions>

## Data Contract Changes
- <what is changing: type/schema/format name and how>
- <migration or compatibility implications>
```

Omit **Data Contract Changes** for pure-logic or internal-only changes. Do not copy the full plan — the `@<plan-path>` reference is the source of truth.

## Doctrine

The plan bridges ticket decisions and executor action; every authoring
choice optimizes for **executor self-sufficiency after context reset**.
When a rule is ambiguous, apply whichever interpretation better preserves
the executor's ability to implement without re-researching.
