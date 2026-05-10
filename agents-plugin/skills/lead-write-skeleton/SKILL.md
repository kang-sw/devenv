---
name: lead-write-skeleton
description: Skeleton primitive for ready tickets whose public contracts must be locked before implementation.
---

# Write Skeleton

Target: user request

## Invariants

- Skeleton = the first code change for a ticket. No implementation code - only interface stubs and integration tests.
- Lead owns public contract draft: research insertion points, write low-resolution source, review, commit.
- The populator owns source discovery and compile-clean normalization, not public contract design.
- Draft markers are language-neutral comment text: `CONTRACT:`, `HINT:`, and `HOLE:`.
- `CONTRACT:` markers bind public names, visibility, module placement, call shape, and behavior targets.
- `HINT:` markers name approximate references the populator should research and normalize.
- `HOLE:` markers name intentionally unknown concrete types, imports, fixtures, helpers, or harnesses.
- Low-resolution source may be non-compiling only before populator handoff.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- The delegate does not commit - lead reviews and commits.
- Register the skeleton-populator agent once per invocation via `ws/agents.register`; resume via `ws/agents.call` for amendment rounds.

## On: invoke

### 1. Identify contract

1. Read the ticket. Note ambiguities requiring lead judgment: scope boundaries, design choices, and non-obvious integration constraints.
2. Skim relevant mental-model docs only if needed to resolve those ambiguities.
3. Call `ws/subquery(deep_research: true, question: "<candidate insertion points, adjacent conventions, public contracts, and integration test targets for this ticket>")`.
4. Read the result with `ws/agents.result(name: <subquery-key>, timeout_seconds: 600)`.
5. Choose insertion points, public shapes, and test target locations.

### 2. Draft

Write the first source edit directly as low-resolution source:

- Public API stubs, public fields, trait/function/method signatures, and module placement.
- Comment-only integration test targets that name the behavior or boundary to cover.
- `CONTRACT:` comments for binding public shape and behavior targets.
- `HINT:` comments for approximate type references, adjacent APIs, dependency direction, or "something like X" cues.
- `HOLE:` comments for unknown concrete types, imports, fixtures, helpers, or test harness locations.

Do not add implementation logic. Placeholder identifiers may be invalid when they
reduce lead effort and leave clear `HINT:` or `HOLE:` markers for the populator.

### 3. Populate

Call `ws/agents.register(name: "skeleton-populator", prompts: ["skeleton-populator"])`.

Call `ws/agents.call(name: "skeleton-populator", prompt: <block below>)`:

```text
Ticket: <ticket-path>

## Lead-authored draft
- Files and locations changed by the lead draft.
- CONTRACT markers are binding; report conflicts with existing public interfaces.
- HINT markers are research cues to normalize.
- HOLE markers are open source-discovery tasks to fill when one clear project-local choice exists.
```

### 4. Review

1. Call `ws/git.diff` and `ws/git.status` to review the skeleton output. Read specific files only if a reported deviation warrants deeper inspection.
2. Verify `CONTRACT:` semantics match the ticket intent and survived population.
3. Run build to confirm compilation. Do not run tests - tests will fail by design because stubs are unimplemented. Passing tests is the implementor's responsibility, not the skeleton's.
4. If issues found:
   - **Minor** - fix directly.
   - **Structural** - relay amended contract markers with `ws/agents.call(name: "skeleton-populator", prompt: <block below>)`:
     ```text
     Amend: <issues and revised CONTRACT/HINT/HOLE markers>
     ```
     Re-review after each round.

### 5. Commit

1. Commit stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton - <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.
5. Update the ticket's `skeletons:` frontmatter with the phase and commit hash (e.g., `phase-1: abc1234`). Only add entries for phases that have a skeleton - no null placeholders.

### 6. Suggest next step

Recommend the next step from implementation width and session warmth:
- **Wide** (multiple independent modules): suggest `ws:lead-write-code` (one scope at a time) or ask the user to split into separate tickets.
- **Narrow + warm** (single module, main agent already engaged the code): suggest `ws:lead-edit`.
- **Narrow + cold** (single module, main agent is cold on the target): suggest `ws:lead-write-code`.

Warmth means the main agent has read target files this session or the user explicitly signaled direct authorship. If ambiguous, suggest `ws:lead-proceed`.

Present the recommendation with brief rationale. Do not auto-invoke.

## Judgments

### judge: test-scope

| Layer | Default | Condition |
|---|---|---|
| Structural seam tests | Always | Every cross-module boundary |
| Behavioral tests | Include when ticket specifies behavior | Any behavior the ticket describes - drop the "complex" qualifier |
| Error / edge case tests | Opt-in | Only when the ticket explicitly specifies error contracts or edge conditions |

### judge: stub-granularity

| Level | When |
|-------|------|
| Module-level (types + top-level functions) | Most cases |
| Method-level (all public methods stubbed) | When the ticket specifies detailed API surface |

## Doctrine

The skeleton optimizes for **contract visibility before implementation**. The
lead spends context on low-resolution public shape; the populator spends context
on source discovery and build cleanup. When ambiguous, preserve contract
stability while minimizing lead serialization.
