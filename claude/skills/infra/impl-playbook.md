# Implementation Playbook

Shared implementation discipline for anyone writing code — top-level
skills and subagents alike. No dispatch mechanics; consumers handle
invocation in their own context.

## Test Strategy

Route by the nature of the code under change:

| Code type | Approach |
|---|---|
| **Testable pure logic** (calculations, parsing, state transitions) | Define expected behavior first → write tests → implement until tests pass |
| **Integration / FFI / IO-bound** | Implement first → add tests for observable behavior |

When the plan specifies TDD/post-impl/manual annotations, those
override these defaults.

For TDD work: write complex/edge-case tests first (exemplars). If 3+
simple cases remain, populate from the exemplar pattern rather than
writing each from scratch.

## Test Failure Diagnosis

When tests fail, diagnose **blame** before fixing:

- Is the test's expectation wrong (stale assumption, incorrect setup)?
- Is the implementation wrong (logic error, missing edge case)?

Fix the side that is actually wrong. Do not patch tests to match broken
implementation, and do not patch implementation to match broken tests.

## Verify

Run the project's test suite(s) and build step (see `ai-docs/_index.md`
for commands). Skip if the project has no test suite. Read the **full**
output. Claim "pass" only after confirming actual results — never
"should pass" or "looks correct."

## Deviation Protocol

When assumptions (from plan, brief, or ticket) don't match the current
codebase:

- **Cosmetic** (renamed param, minor signature change) — adapt silently,
  note in report.
- **Structural** (referenced file/type/function missing or fundamentally
  different interface) — escalate before proceeding.

Who to escalate to depends on context: the user (top-level) or the
team lead (subagent).

## Mechanical-Edit Criteria

When a repetitive edit spans 3+ locations, delegate rather than
applying manually. The delegation prompt must include:

1. **Before/after example** from the first instance
2. **Target file list**
3. **Success criteria** (e.g., `cargo check` passes)
4. **Bail-out condition** — skip and report if structure differs from
   the example

Routing:

| Method | When |
|---|---|
| **sed / replace_all** | Pure text substitution expressible as regex |
| **Cheap subprocess** | Fixed pattern, no ambiguity, no judgment needed |
| **Capable subprocess** | Needs structural understanding or has any ambiguity |

On criteria failure, roll back modified files (`git checkout -- <files>`)
and report. Review any subprocess result before committing.
