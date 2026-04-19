---
name: write-plan
description: >
  Codebase research producing an implementation plan. Runs in warm,
  survey, or deep mode depending on session context and implementation
  risk.
argument-hint: [ticket-path or description]
---

# Write Plan

Target: $ARGUMENTS

## Invariants

- **Warm mode**: lead drafts the plan directly from session context (reading source as needed), then spawns `plan-populator` to enrich and verify in one pass.
- **Survey mode**: lead is a lightweight coordinator; a sonnet delegate produces a reconnaissance brief of reusable components, patterns, and constraints.
- **Deep mode**: lead is a lightweight coordinator; an opus delegate drafts a self-contained plan; a sonnet verifier fixes drift before finalization.
- Plan directives = lead's judgment on points a cold delegate cannot derive from ticket and code alone (cold modes only — warm mode embeds directives as the draft itself).
- When plan definitions diverge from ticket sketches, plan takes precedence — note the change and rationale in Context.
- One plan per ticket phase; if a phase exceeds ~10 actions, split via `/write-ticket` before continuing.
- The deliverable file MUST be committed before finalizing.

## On: invoke

### 1. Assess and route

1. Read the ticket. Note ambiguities needing lead judgment — scope boundaries, architectural choices, phase sequencing.
2. If `/write-skeleton` has been run, collect stub and test file paths — these are locked contracts.
3. Apply `judge: pipeline-mode` to determine warm, survey, or deep.
4. **Warm**: identify which source files the lead needs to read before drafting. Read them directly.
5. **Survey**: formulate **focus areas** — 2–3 codebase regions where reusable components are likely.
6. **Deep**: formulate **plan directives** — 2–5 binding decisions the delegate must follow.

### 2. Draft (warm) or delegate (cold)

#### Warm mode

1. Write the draft plan to `ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md` using direct session context. The draft carries:
   - Binding decisions already settled in conversation.
   - Specific file paths and symbols the lead knows from direct reads.
   - Explicit populate targets (e.g. "reuse the existing X utility") where the lead suspects reuse but has not confirmed the concrete symbol.
2. Spawn `plan-populator` to enrich and verify:

```
Agent(
  name = "plan-populator",
  description = "Enrich and verify draft plan",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Read `${CLAUDE_SKILL_DIR}/plan-populator.md` first.

    Draft plan path: <plan-path>

    ## Skeleton contracts (locked)
    - Stubs: <list of stub file paths, or "none">
    - Tests: <list of test file paths, or "none">
  """
)
```

3. After populator returns, proceed to step 4 (Accept / reject).

#### Survey mode

```
Agent(
  name = "surveyor",
  description = "Codebase survey for implementation",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Read `${CLAUDE_SKILL_DIR}/survey-writer.md` first.

    Ticket: <ticket-path>
    Survey path: ai-docs/plans/YYYY-MM/DD-hhmm.<name>.survey.md

    ## Focus areas
    - <codebase regions where reusable components are likely>

    ## Skeleton contracts (locked)
    - Stubs: <list of stub file paths, or "none">
    - Tests: <list of test file paths, or "none">
  """
)
```

After surveyor returns, skip to step 5 (Finalize).

#### Deep mode

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

### 3. Verify & revise (deep mode only)

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

    Read the plan, then read `CLAUDE.md`, `ai-docs/mental-model.md`, and relevant files in `ai-docs/mental-model/`.
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

### 4. Accept / reject (warm or deep mode)

Read the populator or verifier report only. Accept if no unresolved
Critical issues remain. Reject and re-delegate (cold) or re-draft the
affected sections (warm) if structural problems persist.

### 5. Finalize

1. If the plan implements a ticket phase, update the ticket's `plans:` frontmatter.
2. Commit the plan file.
3. Return the plan path to the caller — include it in the completion summary so
   the caller can pass it directly to `/delegate-implement`.

## Judgments

### judge: pipeline-mode

| Mode | When |
|------|------|
| Warm | Main agent already holds relevant codebase context from prior session turns, or user explicitly signals direct authorship; remaining risk is the main-agent draft missing reusable components or misciting existing shapes |
| Survey | Ticket has resolved the architectural approach, main agent is cold on the relevant area; remaining risk is the cold implementer reinventing existing utilities or missing established patterns |
| Deep | Multiple viable implementation strategies with non-obvious trade-offs, unfamiliar cross-module integration with cascading effects, or ticket explicitly flags unresolved complexity |

Prefer warm when the main agent is warm. Among cold modes, survey is the default — deep is the exception, reserved for genuine architectural novelty.

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

Write-plan bridges ticket decisions and executor action at **the right
weight** — warm mode optimizes for preserving session context that
would otherwise be discarded at subagent fork boundaries, survey mode
optimizes for implementer context efficiency (compact reconnaissance
that prevents wasted exploratory search), deep mode optimizes for
executor self-sufficiency after context reset (complete architectural
guidance). In cold modes the lead passes only binding decisions the
delegate cannot derive and the delegate owns research and drafting; in
warm mode the lead is the drafter and the populator grounds the draft
in concrete code. When a rule is ambiguous, apply whichever
interpretation better preserves the chosen mode's optimization target
while minimizing what the coordinator must serialize.
