---
name: lead-edit
description: Directly implement a narrow code change on the current branch, then verify it with one correctness-and-fit reviewer before updating specs and reporting completion.
---

# Edit

Target: user request

## Invariants

- Lead edits directly; do not delegate implementation.
- Load `ws/infra.read(name: "impl-playbook")` before editing.
- Use `ws/mental_models.find` or `ws/mental_models.status`; read returned paths.
- Ancestor loading: read `mental-model/<domain>/index.md` before `mental-model/<domain>/<sub>.md`.
- Existing skeleton stubs and integration tests are acceptance criteria.
- Commit logical units per CLAUDE.md; include `## AI Context`.
- Relay cap is 2 review cycles; clean up and return after the cap.
- Lead fixes correctness, security, contract, and regression findings.
- Lead may reject style-only or scope-expanding findings with reasons.
- Escalate to `ws:lead-write-code` if scope becomes multi-file with new public API or cross-module new pattern.
- Delete review path before returning.
- Run `ws:lead-update-spec` on the edit range before reporting.
- Output the completion report format exactly.

## On: invoke

### 1. Prepare

1. Parse ticket path or inline brief.
2. Record `<start-commit>` with `git rev-parse HEAD`.
3. If ticket-driven: read ticket; collect skeleton references.
4. Apply `judge: skeleton-check`; stop and suggest `ws:lead-write-skeleton` if required skeleton is absent.
5. Call `ws/mental_models.find(query: <target or domain>)` or `ws/mental_models.status(domain: <domain>)`; read returned docs, ancestors first.
6. Call `ws/infra.read(name: "impl-playbook")`.
7. Identify integration test paths and run command.

### 2. Edit

Edit directly per target and impl-playbook. Commit logical checkpoints with
CLAUDE.md commit rules.

### 3. Verify

1. Run tests/build; read full output before claiming pass.
2. Resolve introduced warnings per impl-playbook Verify.
3. On failure, diagnose blame before fixing; do not patch tests to fit broken implementation.
4. Re-run until verification passes or a real blocker is reported.

### 4. Review

1. Register reviewer:
   `ws/agents.register(name: "reviewer", prompts: ["code-reviewer", "code-review-correctness", "code-review-fit"])`.
2. Generate path:
   `ws/path.generate(kind: "review", stems: ["direct"])`; store `<review-path>`.
3. Call reviewer:

```text
Diff range: <start-commit>..HEAD
Scope: direct-edit - <brief scope description>

Review for correctness and fit.
Write full findings to: <review-path>
Return only: [clean|non-clean]: <one-line summary>
```

4. Read `ws/agents.print(name: "reviewer")` only if async result lacks usable summary.
5. If `[clean]`, proceed to cleanup.
6. If `[non-clean]`, read `<review-path>` and classify findings:
   - Fix: correctness, security, contract, regression.
   - Reject: style-only conflict with local patterns.
   - Reject: scope expansion beyond brief.
7. Apply fixes, keep rejected list with reasons, re-verify.
8. Re-call reviewer:

```text
Re-review. Updated diff: <start-commit>..HEAD
Rejected findings: <list with reasons>
For each rejected finding: respond [accepted] or [maintained: <brief reason>].
```

9. Repeat until `[clean]` or 2 cycles; then proceed to cleanup.

### 5. Cleanup

Delete `<review-path>`.

### 6. Spec Update

Invoke `ws:lead-update-spec` with `<start-commit>..HEAD`.
Output completion report.

## Judgments

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Small isolated edit: single file, no new public contracts |
| Require skeleton | Public interface or cross-module boundary changes |

## Templates

### Completion report format

```text
Edit complete.
Commit range: <start-commit>..HEAD
Test status: pass | fail | skipped
Review: clean | non-clean (<one-line summary>)
Spec: <N entries added, M [planned] stripped> | no changes
<if issues remain after cap:> Open issues: <list>
```

## Doctrine

Edit optimizes for **session-context preservation during code changes**. The
lead keeps implementation context; a fresh reviewer supplies uncommitted
judgment; the relay cap bounds negotiation. When a rule is ambiguous, apply
whichever interpretation keeps the lead's context continuous through the change.
