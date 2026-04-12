---
name: write-skeleton
description: >
  After /write-ticket, before /implement. Crystallize
  public contracts as interface stubs and integration tests. Suggest
  this when a ticket is ready and code changes haven't started.
argument-hint: [ticket-path]
---

# Write Skeleton

Target: $ARGUMENTS

## Invariants

- Skeleton = the first code change for a ticket. No implementation code — only interface stubs and integration tests.
- Lead is a lightweight coordinator: identify the ticket, pass contract directives, review, commit.
- The delegate owns design: reads the ticket, explores the codebase, decides type shapes and test structure.
- Contract directives = lead's judgment on points the delegate cannot derive from ticket + code alone.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- The delegate does not commit — lead reviews and commits.

## On: invoke

### 1. Identify contract directives

1. Read the ticket. Note ambiguities that need lead judgment — scope boundaries, design choices between alternatives, integration constraints not obvious from the ticket alone.
2. Skim relevant mental-model docs only if needed to resolve those ambiguities.
3. Formulate **contract directives**: 2–5 binding decisions. Not a full design — just fences and choices the delegate must follow.

### 2. Delegate to opus subagent

```
Agent(
  description = "write skeleton stubs and tests",
  subagent_type = "general-purpose",
  model = "opus",
  prompt = """
    Read `${CLAUDE_SKILL_DIR}/skeleton-writer.md` first.

    Ticket: <ticket-path>

    ## Contract directives
    - <binding decisions only — things not derivable from ticket + codebase>
  """
)
```

### 3. Review

1. Run `git diff HEAD` and `git status --short` to review the skeleton output. Read specific files only if a reported deviation warrants deeper inspection.
2. Verify contracts match the ticket intent and honor the directives.
3. Run build to confirm compilation. Do not run tests — tests will fail by design because stubs are unimplemented. Passing tests is the implementor's responsibility, not the skeleton's.
4. If issues found, either fix directly (minor) or re-delegate with amended directives (structural).

### 4. Commit

1. Commit stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton — <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.
5. Update the ticket's `skeletons:` frontmatter with the phase and commit hash (e.g., `phase-1: abc1234`). Only add entries for phases that have a skeleton — no null placeholders.

### 5. Suggest next step

Based on implementation complexity:
- **Wide** (multiple independent modules): suggest `/parallel-implement`
- **Narrow** (single module or focused change): suggest `/implement`

Present the recommendation with brief rationale. Do not auto-invoke.

## Judgments

### judge: test-scope

| Scope | When |
|-------|------|
| Cross-module integration tests only | Default — skeleton tests verify seams |
| Include key behavioral tests | When the ticket specifies complex behavioral contracts |

### judge: stub-granularity

| Level | When |
|-------|------|
| Module-level (types + top-level functions) | Most cases |
| Method-level (all public methods stubbed) | When the ticket specifies detailed API surface |

## Doctrine

The skeleton optimizes for **contract-first delegation with minimal
coordinator overhead** — the lead passes only binding decisions the
delegate cannot derive, the delegate owns design and exploration. This
keeps the coordinator's context light while locking public interfaces
and acceptance criteria in code before implementation begins. When a rule
is ambiguous, apply whichever interpretation better preserves contract
stability while minimizing what the coordinator must serialize.
