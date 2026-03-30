---
name: write-plan
description: Research the codebase and produce a self-contained implementation plan. Use before `/implement` or `/execute-plan` for non-trivial changes.
argument-hint: [ticket-path or description]
---

# Write Plan

Target: $ARGUMENTS

## Goal

Produce a **self-contained plan** that survives context reset. The plan must
carry enough context — decisions from the ticket, codebase mapping,
conventions, file roles, domain constraints — for a fresh executor to run
without re-researching.

The plan maps ticket decisions to the codebase: the ticket carries *what*
was decided and *which approaches were suggested* (including data formats,
algorithms, pseudo code); the plan evaluates suggested approaches against
the actual code, then specifies *where* and *how* to integrate.

The plan's depth determines which executor runs it:

| Plan depth | Executor | When |
|-----------|----------|------|
| **Strategic** — direction + relevant files, tactical decisions left to executor | `/implement` | Small-medium changes, familiar patterns |
| **Tactical** — step-by-step with testing strategy, delegation decisions, and success criteria per step | `/execute-plan` | Large changes, TDD-eligible modules, cross-module work |

Default to tactical for thorough-level research. Use strategic only when the
change is simple enough that over-specifying would add noise.

A single plan covers one ticket phase. If during research a phase proves
too large (e.g., ~10+ implementation actions), split it using `/write-ticket`
conventions before continuing. Create the first plan only; note remaining
scope in Context.

## Step 0: Understand

1. Read the ticket/description.
2. Load `ai-docs/_index.md` and `ai-docs/_memory.md` for project state.
3. Load **all** files in `ai-docs/mental-model/` — use Read/Glob directly,
   never delegate initial loading to subagents. Full architectural context
   is needed to identify cross-domain implications and write a sound plan.
4. Run `git log --oneline -10` for recent work context.

## Step 1: Research

Adapt depth to the change. Pick the appropriate level:

| Level | When | What to do |
|-------|------|------------|
| **Minimal** | Config tweak, typo, single-file mechanical change | Mental-model docs only |
| **Moderate** | Feature addition following existing patterns, 2-3 files | + Read target files and adjacent code for patterns |
| **Thorough** | New component, cross-module, unfamiliar area | + Search for similar implementations, extract concrete convention examples |

When uncertain, go one level deeper — over-researching costs less than a wrong
plan. Before designing new components, search for existing utilities or patterns
that can be reused or extended — include these in the plan's "Relevant Files."

Use subagents for broad codebase searches, including reuse candidates. Keep the
main context for synthesis.

## Step 2: Draft Plan

Generate a timestamp-based path:
`ai-docs/plans/YYYY-MM/DD-hhmm.<plan-name>.md`

Use a descriptive kebab-case name for the plan (e.g.,
`ai-docs/plans/2026-03/28-1430.event-serialization.md`).
The plan name is independent of the ticket stem. The `YYYY-MM/DD-hhmm`
prefix serves as the plan's unique hash.

Write the plan to that file using the `Write` tool, in this format:

```markdown
# <Plan Title>

## Context
- Ticket phase decisions and suggested approaches (goals, constraints,
  rejected alternatives, candidate approaches with rationale)
- Codebase mapping (existing types to reuse, integration points,
  file placement)

## Relevant Files
- `path/to/file` — role in this change, key types/functions to touch

## Conventions (verified from code)
- Naming, structure, error handling patterns observed
- Concrete examples from existing code where helpful

## Implementation Steps
1. Concrete action (which file, what change, why)
   - Delegation: [main | haiku | sonnet] — rationale
   - Depends on: step N (if ordering matters beyond sequence)
2. ...

## Testing Strategy

Classify each module or component:

| Module | Approach | Rationale |
|--------|----------|-----------|
| `path/module` | TDD / post-impl / manual | why this classification |

For TDD modules, specify:
- **Stub scope** — which signatures to stub, return types
- **Exemplar cases** — complex/edge cases the main agent should write
- **Population cases** — simple cases delegable to a subagent
- **Delegation model** — haiku (parameter-only variation) or sonnet (new scenarios)

For post-impl modules:
- **Key scenarios** — what observable behavior to test after implementation

For manual modules:
- **Verification method** — how to confirm correctness without automated tests

## Success Criteria
- Observable conditions that mean "done"
```

**Self-containedness check.** Before moving on, ask: "Could an agent with no
prior context execute this plan correctly?" If not, add what's missing.

## Step 3: Verify Plan

Dispatch a **sonnet-level general-purpose subagent** to verify the plan against
the actual codebase. Pass the **file path only** — the subagent reads the plan
itself:

> **Task:** Verify the implementation plan at `<plan-path>` against the actual
> source code.
>
> **Steps:**
> 1. Read the plan file.
> 2. Read `CLAUDE.md` code standards. Read **all** of
>    `ai-docs/mental-model/` regardless of apparent domain relevance —
>    cross-module contracts and invariants often surface in unrelated domains.
>    Do this before evaluating the plan.
> 3. Check each item in the plan:
>    - Do referenced files, functions, and types actually exist?
>    - Do described conventions match actual code patterns?
>    - Are there missing considerations (error handling, edge cases, dependencies)?
>    - Does the plan conflict with documented contracts or invariants?
>    - Does the plan introduce unintended coupling or violate module boundaries?
>    - Are implementation steps concrete enough to execute without ambiguity?
>    - Are testing classifications (TDD/post-impl/manual) appropriate for each module?
>    - Are delegation decisions (haiku/sonnet/main) reasonable given complexity?
>    - Do TDD stub definitions cover the necessary type signatures?
>    - Does the plan reimplement functionality that already exists in the
>      codebase? Search for existing utilities, helpers, or patterns that
>      could be reused or extended instead.
>
> **Be aggressive.** Flag anything suspicious — false positives are fine.
> Categorize as Critical / Important / Minor.

## Step 4: Triage & Revise

Read the verifier's report. For each flag:

- **Critical**: fix in the plan. These are factual errors.
- **Important**: assess whether the concern applies. Revise if it does.
- **Minor**: note if useful, skip if not.

The verifier is intentionally aggressive. Use your judgment on actual severity.

**Edit the plan file directly** using the `Edit` tool — do not rewrite the
entire file. After revision, do a final read-through for coherence and
completeness.

## Step 5: Finalize

Choose the executor based on plan depth:

- **Tactical plan** (has Testing Strategy classifications, delegation
  decisions, per-step detail) → `/execute-plan`
- **Strategic plan** (direction + relevant files, no tactical directives)
  → `/implement`

Call `EnterPlanMode`, then write the **plan file** with this structure:

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
or removes data formats that cross a capsule boundary: wire formats (API
payloads, IPC messages), persistence schemas (DB, file formats), public API
types consumed outside the owning package, config file formats, environment
variables, or CLI flags. Omit the entire section for pure-logic or
internal-only changes.

The plan file content is injected as the first prompt when the user clicks
"Reset Context and auto-accept" after `ExitPlanMode`. The `# Steps` block
ensures the executor loads before the plan is read.

Do **not** copy the full plan text into the plan file — the `@<plan-path>`
reference is the source of truth.

**The plan MUST be committed!**

If the plan implements a ticket phase, update the ticket's `plans:` frontmatter
to reference this plan (replacing `null` with the plan path stem).
