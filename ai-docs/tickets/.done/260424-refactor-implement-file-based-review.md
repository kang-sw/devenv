---
title: /implement file-based review loop + review-path collision resistance
spec:
  - 260424-implement-file-based-review
related-mental-model:
  - workflow-routing
---

# /implement file-based review loop + review-path collision resistance

## Background

The `/implement` skill lead currently consolidates full reviewer outputs in its own
context before relaying to the implementer. Each review partition returns its complete
findings text to the lead; the lead re-reads and re-encodes them into a consolidated
message. This creates token overhead proportional to findings volume × number of review
rounds. With three partitions and multiple fix cycles, the lead context accumulates
several thousand tokens of review content the lead itself does not need.

The `bin/review-path` script exists to route findings through files, but is not wired
into `implement/SKILL.md`. Additionally, the current `review-path` script generates
deterministic paths (`/tmp/claude-reviews/<stem>.md`) that collide across concurrent
runs in the same or different projects.

## Decisions

- **File-based routing**: reviewers write full findings to `review-path`-allocated files;
  stdout returns only `[clean|non-clean]: <brief>`. Lead reads summaries only. Implementer
  reads files directly when non-clean — no consolidation in lead context.

- **Lead context as path store**: Bash variable state does not persist across tool calls.
  The lead allocates all review-path slots in a single Bash invocation, reads the output
  lines into LLM context, and inlines those literal paths in all subsequent Bash calls.

- **Non-deterministic paths**: add pwd-hash (project scoping) + per-call run-id (collision
  resistance). Multi-stem support lets the lead allocate all partition paths in one call,
  sharing the same run-id — cleaner than separate calls. `shasum` used for cross-platform
  pwd hashing (macOS + Linux, no `md5`/`md5sum` divergence).

- **Implementer filtering guidance**: reviewers apply strict criteria by design; the
  implementer should read critically and prioritize correctness/contract/security findings
  over style feedback that conflicts with existing codebase conventions.

- **Rejected**: having the lead rm findings via glob (`*.md` on the run-id prefix) — the
  lead already holds exact paths and explicit rm is safer.

## Phases

### Phase 1: review-path redesign + implement/SKILL.md review loop restructure

Modify `claude/bin/review-path`:

1. Accept 1 or more stem arguments.
2. Compute `pwd_hash` once: `printf '%s' "$PWD" | shasum | cut -c1-8`.
3. Generate `run_id` once per invocation: `LC_ALL=C tr -dc 'a-z0-9' </dev/urandom | head -c8`.
4. For each stem argument: print `/tmp/claude-reviews/${pwd_hash}-${run_id}-${stem}.md`.
5. Create `/tmp/claude-reviews/` if absent.

New path format: `/tmp/claude-reviews/<pwd-hash>-<run-id>-<stem>.md`

Contract: caller must capture all output lines from a single invocation and hold them as
literals in agent context — paths are not reproducible after the call returns.

Modify `claude/skills/implement/SKILL.md`:

1. **Invariant update**: replace "Reviewers report to the lead only — never directly to
   the implementer. The lead consolidates findings and sends a single list to the
   implementer." with "Reviewers write findings to files; lead reads summaries only;
   implementer reads files directly when non-clean."

2. **Step 1 (Prepare), add after branch creation**:
   ```
   Allocate review-path slots — single Bash call, capture all output lines in lead context:
   review-path correctness fit test   # or partition-allocated subset from judge: partition-allocation
   ```
   Store the returned paths as literals; reference them by name (correctness-path, fit-path,
   test-path) throughout subsequent steps.

3. **Step 3b (Spawn reviewers)**: add to each reviewer spawn message:
   - `Write your full findings to: <allocated-path>` (the literal path from lead context)
   - `Return only: [clean|non-clean]: <one-line characterization of most significant issues>`

4. **Step 3c (Relay and loop)**:
   - Loop control: read stdout summary line only — `[clean]` → exit loop.
   - Non-clean: pass file paths to implementer, not consolidated content:
     `"Fix issues in these review reports: <correctness-path>, <fit-path>. Read each file directly."`
   - Re-review: same paths (reviewers overwrite on each pass).

5. **Step 8 (Cleanup)**: add `rm -f <correctness-path> <fit-path> <test-path>` (literal paths
   from lead context).

6. **Task list (Step 1.8)**: update the reviewer task description from "lead consolidates
   findings" to "reviewers write to files; lead reads summaries; implementer reads files."

Success: after implementation, the lead context holds at most two stdout summary lines per
review partition per round; no full findings text appears in lead context.

### Phase 2: implementer filtering guidance

Modify `claude/infra/implementer.md` — add to the `## Doctrine` section (or equivalent
guidance block):

> Reviewer criteria are strict by design. When review findings arrive as a file path:
> read the file, then apply judgment:
> - Address correctness, contract, and security findings.
> - Deprioritize style or naming feedback that conflicts with established patterns in the
>   surrounding codebase.
> - Never apply a finding without understanding why it matters for this specific change.

Phase 2 is independent of Phase 1 — it may ship first or after.
