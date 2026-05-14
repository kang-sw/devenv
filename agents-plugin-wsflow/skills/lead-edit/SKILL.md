---
name: lead-edit
description: Edit primitive for wsflow; lead integrates changes on the current branch, verifies, performs bounded review, and reports.
---

# Edit

Target: user request

## Invariants

Scope
- Lead owns edit routing, integration, verification, and commits.
- Honor caller-provided scope or phase slices as hard edit boundaries.

Context
- Load `wsflow/infra.read(name: "impl-playbook")` before editing.
- Use `wsflow/mental_models.find` or `wsflow/mental_models.status`; read returned paths.
- Ancestor loading: read `mental-model/<domain>/index.md` before `mental-model/<domain>/<sub>.md`.

Commit
- Commit logical units per repository commit rules; include `## AI Context`.

Review
- Review after verification; use subagent review when useful.
- Lead fixes correctness, security, contract, and regression findings.
- Lead may reject style-only or scope-expanding findings with reasons.

Output
- Output the completion report format exactly.

## On: invoke

### 1. Prepare

1. Parse ticket path or inline brief.
2. Record `<start-commit>` with `git rev-parse HEAD`.
3. If ticket-driven: read ticket and caller-provided scope boundary.
4. Call `wsflow/mental_models.find(query: <target or domain>)` or `wsflow/mental_models.status(domain: <domain>)`; read returned docs, ancestors first.
5. Call `wsflow/infra.read(name: "impl-playbook")`.
6. Identify verification commands from package scripts, tests, ticket notes, or changed files.

### 2. Edit

1. Implement per target and impl-playbook.
2. Use direct edits or scoped subagent implementation when it improves throughput.
3. For subagent implementation, state scope, writable paths or modules, verification expectations, and required changed-file summary.
4. Commit logical checkpoints with repository commit rules.

### 3. Verify

1. Run tests/build; read full output before claiming pass.
2. Resolve introduced warnings per impl-playbook Verify.
3. On failure, diagnose blame before fixing; do not patch tests to fit broken implementation.
4. Re-run until verification passes or a real blocker is reported.

### 4. Review

1. Apply `judge: review-scope`.

2. If subagent review is selected, ask one read-only reviewer:

```text
Review diff range: <start-commit>..HEAD
Scope: direct edit - <brief scope description>
Use wsflow read tools if useful: wsflow/git.diff, wsflow/git.log, wsflow/specs.*, wsflow/tickets.*, wsflow/mental_models.*.
Review for correctness, contracts, regressions, and local fit.
Ignore broad style or unrelated architecture unless directly broken by the diff.
Return:
- verdict: clean | findings
- findings: file/path references, severity, and concise rationale
```

3. Otherwise, perform a lead-only review and record the rationale.

4. Classify findings:
   - Fix: correctness, security, contract, regression.
   - Reject: style-only conflict with local patterns.
   - Reject: scope expansion beyond brief.

5. Apply fixes, re-run verification, and perform one focused re-review of fixed areas.
6. Stop after one re-review cycle and report any remaining open issues.

### 5. Report

Output completion report.

## Judgments

### judge: review-scope

Use a subagent reviewer by default for source changes. Lead-only review is
allowed for mechanical, doc-only, or low-risk edits with rationale.

## Templates

### Completion report format

```text
Edit complete.
Commit range: <start-commit>..HEAD
Test status: pass | fail | skipped
Review: clean | non-clean (<one-line summary>)
<if issues remain after cap:> Open issues: <list>
```

## Doctrine

Edit optimizes for **session-context preservation during code changes**. The
lead owns integration, verification, and commits; a subagent reviewer supplies
a fresh read-only check when useful. When a rule is ambiguous, keep the lead's
context continuous through the change.
