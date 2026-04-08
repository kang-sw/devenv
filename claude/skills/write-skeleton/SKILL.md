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
- Main agent writes the skeleton directly. Gathering adjacent contracts may be delegated; writing stubs and tests is not.
- Stubs define public interfaces only: type definitions, trait/interface declarations, function signatures with `todo!()` / `unimplemented` / `raise NotImplementedError` bodies.
- Integration tests verify contract joints — cross-module interactions, data flow across boundaries. Not unit tests for internals.
- The skeleton commit is a stable foundation: implementation can be reverted without losing contracts.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.

## On: invoke

### 1. Understand the contract

1. Read the ticket. Identify public contracts: new types, API surfaces, data formats, module boundaries.
2. Read `ai-docs/mental-model/overview.md` and docs touching the change area — understand existing contracts the new code must integrate with.
3. Formulate a skeleton brief: what stubs to create, what integration tests to write, which modules they interact with.
4. Dispatch an Explore agent with the brief to scout placement and gather adjacent APIs:

   ```
   Agent(
     description = "scout skeleton placement",
     subagent_type = "Explore",
     prompt = "Skeleton brief: <what stubs and tests are planned, which modules they interact with>

              Find and return:
              1. Placement: where should new files go? (directory conventions, sibling modules, test file layout)
              2. Adjacent APIs: public type definitions, function signatures, trait impls of modules the skeleton will interact with.
              3. Test conventions: how are integration tests organized in this project? (directory, naming, fixtures)

              Read source files directly — do not summarize."
   )
   ```

   Write stubs and tests at the paths the scout identifies. Do not explore the codebase further.

### 2. Write stubs

1. Create or edit source files with public interface stubs.
   - Type definitions with all public fields and types.
   - Function/method signatures with placeholder bodies.
   - Trait/interface declarations.
2. Stubs must compile (or pass syntax checks for dynamic languages). Run build to verify.
3. Do not write private helper functions, internal logic, or implementation details.

### 3. Write integration tests

1. Write tests that exercise contract joints — the seams where this module meets others.
2. Tests should fail now (stubs are unimplemented) but define the acceptance criteria.
3. Focus on: data flow across boundaries, expected input/output contracts, error contracts.
4. Keep test count small and targeted — these are acceptance criteria, not exhaustive coverage.

### 4. Commit

1. Commit stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton — <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.

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
