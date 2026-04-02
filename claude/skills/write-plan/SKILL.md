---
name: write-plan
description: >
  Use when the user mentions creating, writing, or drafting an
  implementation plan. Research the codebase and produce a self-contained
  implementation plan. Use before `/implement` or `/execute-plan` for
  non-trivial changes.
argument-hint: [ticket-path or description]
---

# Write Plan

Target: $ARGUMENTS

## Goal

Produce a **self-contained plan** that survives context reset. The plan must
carry enough context for a fresh executor to implement without re-researching.

### Role: fill the gap between ticket and codebase

The ticket owns *what* was decided (contracts, design choices, rejected
alternatives). The executor owns *how* to implement (code, construction-site
fixes, test code). The plan fills the gap:

1. **Distill** — extract this phase's contracts and decisions from the ticket
   (which may be messy or multi-phase). Do not duplicate; summarize and
   reference.
2. **Map** — identify which files, types, and patterns in the codebase are
   affected. Add integration notes the executor would miss without reading
   the code (conventions, gotchas, pattern references).
3. **Supplement** — when the ticket lacks contracts or leaves decisions open,
   research the codebase and propose them (subject to the data contract gate).

When the plan's formal definitions differ from the ticket's sketches, the
plan takes precedence — it reflects codebase research the ticket did not
have. Note the change and rationale in Context.

**Do not include:** implementation code for pattern-following edits,
construction-site inventories the compiler will surface, line numbers,
import statements, or delegation strategy that would be "main" for every
step.

### Plan depth

| Plan depth | Executor | When |
|-----------|----------|------|
| **Strategic** — direction + relevant files, tactical decisions left to executor | `/implement` | Small-medium changes, familiar patterns |
| **Tactical** — contracts + integration notes + testing strategy + success criteria | `/execute-plan` | Large changes, cross-module work, new patterns |

Default to tactical for thorough-level research. Use strategic only when the
change is simple enough that over-specifying would add noise.

A single plan covers one ticket phase. If during research a phase proves
too large (e.g., ~10+ implementation actions), split it using `/write-ticket`
conventions before continuing. Create the first plan only; note remaining
scope in Context.

## Step 0: Understand

1. Read the ticket/description.
2. Load `ai-docs/_index.md` for project state.
3. Load **all** files in `ai-docs/mental-model/` — use Read/Glob directly,
   never delegate initial loading to subagents. Full architectural context
   is needed to identify cross-domain implications and write a sound plan.

## Step 1: Research

Adapt depth to the change. Pick the appropriate level:

| Level | When | What to do |
|-------|------|------------|
| **Minimal** | Config tweak, typo, single-file mechanical change | Mental-model docs only |
| **Moderate** | Feature addition following existing patterns, 2-3 files | + Read target files and adjacent code for patterns |
| **Thorough** | New component, cross-module, unfamiliar area | + Search for similar implementations, extract concrete convention examples |

When uncertain, go one level deeper — over-researching costs less than a wrong
plan. Before designing new components, search for existing utilities or patterns
that can be reused or extended — reference them in the plan's Steps.

Use subagents for broad codebase searches, including reuse candidates. Keep the
main context for synthesis.

## Step 2: Draft Plan

Generate a timestamp-based path:
`ai-docs/plans/YYYY-MM/DD-hhmm.<plan-name>.md`

Use a descriptive kebab-case name for the plan (e.g.,
`ai-docs/plans/2026-03/28-1430.event-serialization.md`).
The plan name is independent of the ticket stem. The `YYYY-MM/DD-hhmm`
prefix serves as the plan's unique hash.

Write the plan to that file using the `Write` tool. Include only
sections that carry information — omit empty or trivial sections.
Length varies with complexity; the content-type rules below govern
what belongs, not a line count.

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
trait definitions, public function signatures. These are the plan's
primary deliverable — the executor must not have to invent them.

Carry forward ticket-mandated approaches (algorithms, patterns,
constraints agreed during discussion) explicitly.

Also include:
- Non-obvious constraints or ordering dependencies
- Pattern references ("same as ExternalSink::on_event") instead of
  duplicated code

Leave to the executor: construction-site fixes (compiler-guided),
pattern-following code, line numbers, import changes.

Implementation sketches may be approximate or pseudo-code — precision
is the executor's responsibility, not the plan's.

## Testing
Key scenarios to verify after implementation. Classify modules as
TDD / post-impl / manual only when non-obvious; default is post-impl.

## Success Criteria
Observable conditions that mean "done".
```

**Data contract gate.** Scan the draft for data contract changes — formats
that cross a capsule boundary (wire formats, persistence schemas, public API
types consumed outside the owning package, config file formats, environment
variables, or CLI flags). If any contract is defined or modified but **not**
specified in the ticket, present the proposed shape and rationale to the user
and wait for confirmation. Do not proceed to verification with unconfirmed
contracts.

**Self-containedness check.** Before moving on, ask: "Could an agent with no
prior context execute this plan correctly?" If not, add what's missing.

## Step 3: Verify

Dispatch a **sonnet subagent** to verify the plan from a fresh context.
The value is not thoroughness but **fresh eyes** — the planner has
accumulated assumptions that a new context window does not share.

> **Task:** Verify the implementation plan at `<plan-path>`.
>
> Read the plan, then read `CLAUDE.md` and all of `ai-docs/mental-model/`.
> Check:
> - Do referenced files, functions, and types actually exist?
> - Do described conventions match actual code patterns?
> - Does the plan conflict with documented contracts or invariants?
> - Are new/changed public contracts (structs, traits, function
>   signatures) specified with all public members and types?
> - Could an executor with no prior context implement this correctly?
> - Does the plan reimplement something that already exists?
>
> Categorize as Critical / Important / Minor.

Fix Critical issues in the plan. Assess Important — revise if valid.
Skip Minor unless useful.

## Step 4: Finalize

Choose the executor based on plan depth:

- **Tactical** (contracts with integration notes, testing classifications,
  success criteria) → `/execute-plan`
- **Strategic** (direction + relevant files, decisions left to executor)
  → `/implement`

Call `EnterPlanMode` and write this structure:

```
# Steps

- Load `/<executor>` skill
- Read `@<plan-path>`

---

# <Plan Title>

<brief summary — what changes, why, key decisions>

## Data Contract Changes
- <what is changing: type/schema/format name and how>
- <migration or compatibility implications>
```

**Data Contract Changes** — include this section when the plan adds, changes,
or removes data contracts (see Step 2 gate for the full list). Omit the
entire section for pure-logic or internal-only changes.

The plan file content is injected as the first prompt when the user clicks
"Reset Context and auto-accept" after `ExitPlanMode`. The `# Steps` block
ensures the executor loads before the plan is read.

Do **not** copy the full plan here — the `@<plan-path>` reference is the
source of truth.

**The plan MUST be committed!**

If the plan implements a ticket phase, update the ticket's `plans:` frontmatter
to reference this plan (replacing `null` with the plan path stem).
