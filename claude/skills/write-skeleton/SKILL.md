---
name: write-skeleton
description: >
  After /write-ticket, before /write-plan or /implement. Crystallize
  public contracts as interface stubs and integration tests. Suggest
  this when a ticket is ready and code changes haven't started.
argument-hint: [ticket-path]
---

# Write Skeleton

Target: $ARGUMENTS

## Invariants

- Skeleton = the first code change for a ticket. No implementation code — only interface stubs and integration tests.
- Lead formulates the skeleton brief (contract design); an opus subagent writes the code.
- Stubs define public interfaces only: type definitions, trait/interface declarations, function signatures with `todo!()` / `unimplemented` / `raise NotImplementedError` bodies.
- Integration tests verify contract joints — cross-module interactions, data flow across boundaries. Not unit tests for internals.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- The subagent does not commit — lead reviews and commits.

## On: invoke

### 1. Understand the contract

1. Read the ticket. Identify public contracts: new types, API surfaces, data formats, module boundaries.
2. Read `ai-docs/mental-model/overview.md` and docs touching the change area — understand existing contracts the new code must integrate with.
3. Formulate a **skeleton brief**: what stubs to create, what integration tests to write, which modules they interact with, key type shapes and signatures.

### 2. Delegate to opus subagent

```
Agent(
  description = "write skeleton stubs and tests",
  model = "opus",
  prompt = """
    Read `${CLAUDE_SKILL_DIR}/skeleton-writer.md` first.

    ## Skeleton brief
    <brief from step 1>
  """
)
```

### 3. Review

1. Read the files the subagent created/modified.
2. Verify contracts match the skeleton brief and ticket intent.
3. Run build to confirm compilation.
4. If issues found, either fix directly (minor) or re-delegate (structural).

### 4. Commit

1. Commit stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton — <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.
5. Update the ticket's `skeletons:` frontmatter with the phase and commit hash (e.g., `phase-1: abc1234`). Only add entries for phases that have a skeleton — no null placeholders.

### 5. Suggest next step

Based on implementation complexity:
- **Complex** (multi-module interaction, unfamiliar patterns): suggest `/write-plan`
- **Simple** (filling in private internals, following existing patterns): suggest `/implement`

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

The skeleton optimizes for **contract-first delegation** — by locking
public interfaces and acceptance criteria in code before implementation
begins, delegation becomes safe (implementers cannot deviate from
contracts) and reversible (implementation reverts without losing the
contract layer). When a rule is ambiguous, apply whichever interpretation
better preserves contract stability and delegation safety.
