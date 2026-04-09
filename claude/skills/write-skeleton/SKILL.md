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
    You are writing skeleton stubs and integration tests for a ticket.

    ## Skeleton brief
    <brief from step 1 — contracts, type shapes, module interactions>

    ## Rules
    - Stubs: public interfaces only. Type definitions with all public fields,
      function/method signatures with placeholder bodies (todo!()/unimplemented/
      raise NotImplementedError). No private helpers or implementation logic.
    - Integration tests: exercise contract joints (cross-module seams, data flow
      across boundaries). Keep count small and targeted — acceptance criteria,
      not exhaustive coverage.
    - Do not modify existing public interfaces unless the brief explicitly says to.
    - Stubs must compile (or pass syntax checks for dynamic languages). Run build
      to verify. Fix compilation errors until clean.
    - Do not create commits — leave changes unstaged.

    ## Exploration
    Use `~/.claude/infra/ask.sh "<question>"` (Bash tool) for scoped lookups:
    placement conventions, adjacent API signatures, test file layout, import paths.
    Default haiku; use `--deep-research` for cross-module tracing. Prefer ask.sh
    over reading files directly — preserve your context for contract decisions.

    ## Output
    Report what was created:
    - Files created/modified with paths
    - Key contract decisions (type shapes, trait bounds, error types)
    - Any deviations from the brief with rationale
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
