# Implementation Playbook

## Invariants

- Before implementing, load relevant mental-model docs: `list-mental-model <target-paths>`; read the listed files.
- Claim "pass" only after reading full test/build output — never "should pass" or "looks correct."
- Diagnose blame (test vs implementation) before fixing any failure.
- Structural deviations → escalate before proceeding. Cosmetic → adapt silently, note in report.
- Review subprocess results before committing. Roll back on criteria failure.
- Plan annotations (TDD/post-impl/manual) override default test strategy when present.

## §Test Strategy

| Code type | Approach |
|---|---|
| **Pure logic** (calculations, parsing, state transitions) | Tests first → implement until pass |
| **Integration / FFI / IO-bound** | Implement first → test observable behavior |

TDD: write complex/edge-case tests first (exemplars); populate 3+ remaining simple cases from the exemplar pattern.

## §Test Failure Diagnosis

Determine blame before fixing:

- **Implementation wrong** — logic error, missing edge case → fix the code.
- **Test wrong** — stale assumption, incorrect setup → fix the test.
- **Spec ambiguity** — both readings are plausible → escalate for clarification.
- **Test infrastructure** — setup/teardown/harness issue, not a logic defect → fix the test environment.

Never patch tests to match broken implementation or vice versa.

**Repetition check** (mechanical, every failure): Before fixing, ask and answer: "Is this the same root cause as a previous failure in this session?" If yes, stop and escalate immediately. Do not attempt another fix.

## §Verify

Run the project's test suite(s) and build step (`ai-docs/_index.md` for commands). Skip if no test suite exists. Read the full output.

**Warnings.** Resolve any warnings introduced by your change (in edited files or in downstream files affected by the change) before claiming pass — fix the cause, or suppress at source with a scoped annotation and a one-line comment explaining why. Pre-existing warnings in untouched, unaffected files are out of scope. Rationale: unresolved warnings re-emit on every incremental build and compound in the session context window.

## §Deviation Protocol

| Gap type | Action |
|---|---|
| **Cosmetic** (renamed param, minor signature change) | Adapt silently, note in report |
| **Structural** (missing file/type/function, different interface) | Escalate before proceeding |

Escalation target: user (top-level) or team lead (subagent).

## §Mechanical-Edit Criteria

Trigger: repetitive edit spans 3+ locations.

Delegation prompt must include: (1) before/after example, (2) target file list, (3) success criteria, (4) bail-out condition.

| Method | When |
|---|---|
| **sed / replace_all=true** | Pure text substitution expressible as regex |
| **Direct edit loop** | File-by-file judgment needed (Read → Edit per file) |

On failure: `git checkout -- <files>`, report.

## Doctrine

The playbook optimizes for **defect prevention before commit** — every
procedure exists so that incorrect code, misdiagnosed failures, and
silent deviations do not reach the branch. When a rule is ambiguous,
apply whichever interpretation catches more errors before they
propagate.
