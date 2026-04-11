---
name: write-plan
description: >
  Deep codebase research producing an implementation plan. Invoke when
  the user explicitly requests a plan, or when chained from /proceed
  before /implement.
argument-hint: [ticket-path or description]
---

# Write Plan

Target: $ARGUMENTS

## Invariants

- Plan must be self-contained: a fresh executor implements without re-researching.
- Lead is a lightweight coordinator: identify the ticket, pass directives, review verifier report, finalize.
- The delegate owns research and drafting: reads the ticket, explores the codebase, writes the plan.
- Plan directives = lead's judgment on points the delegate cannot derive from ticket + code alone.
- When plan definitions diverge from ticket sketches, plan takes precedence — note the change and rationale in Context.
- One plan per ticket phase; if a phase exceeds ~10 actions, split via `/write-ticket` before continuing.
- The plan file MUST be committed before finalizing.

## On: invoke

### 1. Identify plan directives

1. Read the ticket. Note ambiguities needing lead judgment — scope boundaries, architectural choices, phase sequencing.
2. If `/write-skeleton` has been run, collect stub and test file paths — these are locked contracts.
3. Formulate **plan directives**: 2–5 binding decisions the delegate must follow.

### 2. Delegate to planner

```
Agent(
  name = "planner",
  description = "Draft implementation plan",
  subagent_type = "general-purpose",
  model = "opus",
  prompt = """
    Read `${CLAUDE_SKILL_DIR}/plan-writer.md` first.

    Ticket: <ticket-path>
    Plan path: ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md

    ## Plan directives
    - <binding decisions only — things not derivable from ticket + codebase>

    ## Skeleton contracts (locked)
    - Stubs: <list of stub file paths, or "none">
    - Tests: <list of test file paths, or "none">
  """
)
```

### 3. Verify & revise

Dispatch a sonnet subagent to verify and fix the plan in-place.
The lead reads only the verifier's report — not the plan file or source code.

```
Agent(
  name = "plan-verifier",
  description = "Verify and fix plan",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Verify the implementation plan at `<plan-path>`.

    Read the plan, then read `CLAUDE.md` and all of `ai-docs/mental-model/`.
    Check:
    - Do referenced files, functions, and types actually exist?
    - Do described conventions match actual code patterns?
    - Does the plan conflict with documented contracts or invariants?
    - Are new/changed public contracts specified with all public members and types?
    - Could an executor with no prior context implement this correctly?
    - Does the plan reimplement something that already exists?
    - If skeleton stubs/tests exist: does the plan contradict a skeleton
      contract without listing it in Skeleton Amendments? (Critical if yes.)

    Categorize each finding as Critical / Important / Minor.
    - Critical and Important: apply inline fixes to the plan file directly.
    - Minor: note but do not fix unless trivial.

    After fixes, return a summary report to the lead:
      ## Verification Report
      - Findings: <count by severity>
      - Fixes applied: <list of changes made>
      - Unresolved: <any issues that need lead judgment>

    Skeleton contracts (locked):
    - Stubs: <list of stub file paths, or "none">
    - Tests: <list of test file paths, or "none">
  """
)
```

### 4. Accept / reject

Read the verifier's report only (not the plan or source code). Accept if
no unresolved Critical issues remain. Reject and re-delegate if structural
problems persist.

### 5. Finalize

1. If the plan implements a ticket phase, update the ticket's `plans:` frontmatter.
2. Commit the plan file.
3. Return the plan path to the caller — include it in the completion summary so
   the caller can pass it directly to `/implement`.

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

## Doctrine

The plan bridges ticket decisions and executor action with **minimal
coordinator overhead** — the lead passes only binding decisions the
delegate cannot derive, the delegate owns research and drafting. The
plan itself optimizes for executor self-sufficiency after context reset.
When a rule is ambiguous, apply whichever interpretation better preserves
executor self-sufficiency while minimizing what the coordinator must
serialize.
