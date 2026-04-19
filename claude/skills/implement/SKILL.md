---
name: implement
description: >
  Main-agent-direct single-scope implementation. The main agent reads,
  edits, verifies, and commits directly — no subagent delegation for the
  edit itself.
argument-hint: "[ticket-path or inline brief] [--plan <plan-path>]"
---

# Implement

Target: $ARGUMENTS

## Invariants

- The main agent edits directly — no subagent delegation for the edit itself.
- Follow impl-playbook — run `load-infra impl-playbook.md` for test strategy, verify, failure diagnosis, deviation protocol, and mechanical-edit criteria.
- Load relevant mental-model docs before editing — run `list-mental-model <target-paths>` and read every listed file.
- When skeleton exists for the target scope, its stubs and integration tests are the acceptance criteria.
- Commit per logical unit following CLAUDE.md commit rules; include `## AI Context`.
- Escalate to `/delegate-implement` or `/proceed` if scope grows beyond single-scope direct-edit capacity.
- When `judge: needs-review` fires, spawn a one-shot reviewer — no team, fresh context per iteration.
- Doc pipeline (mental-model-updater, `ai-docs/_index.md`, ticket status) runs before reporting to the user.
- Report completion to the user — commit range, test status, deviations.

## On: invoke

### 1. Prepare

1. Parse arguments: ticket path, inline brief, or `--plan` path.
2. If plan-driven: read the plan file. Note binding decisions and Skeleton Amendments.
3. If ticket-driven: read the ticket; collect skeleton and plan references from frontmatter.
4. Verify skeleton coverage when public contracts are touched: grep for stubs. If `judge: skeleton-check` requires a skeleton and none exists, stop and suggest `/write-skeleton`.
5. Load mental-model docs: `list-mental-model <target-paths>`; read every listed file.
6. Run `load-infra impl-playbook.md`.
7. Identify integration test file paths and the command to run them.

### 2. Edit

1. Edit files directly per plan/brief, honoring impl-playbook.md — test strategy, deviation protocol, mechanical-edit criteria.
2. Commit at logical checkpoints per CLAUDE.md commit rules. Include `## AI Context`.

### 3. Verify

1. Run the project's test suite(s) and build step. Read full output — never claim pass from a skimmed tail.
2. Resolve warnings per impl-playbook.md §Verify.
3. On failure: diagnose blame per impl-playbook.md §Test Failure Diagnosis. Never patch tests to match broken impl or vice versa.
4. Re-run until verify passes.

### 4. Review (conditional)

Apply `judge: needs-review`.

- If skip: proceed to step 5.
- If review: spawn a one-shot reviewer via Agent tool (no `team_name`):

```
Agent(
  description = "Review /implement diff",
  subagent_type = "ws:code-reviewer",
  model = "sonnet",
  prompt = """
    Diff range: <first-commit>..HEAD
    Scope: main-agent-direct implementation — <brief scope description>

    Follow your standard review process (CLAUDE.md + mental-model sweep + diff).
    Return findings report.
  """
)
```

On findings:
- Apply Critical/Important fixes directly; re-verify tests.
- If fixes are trivial and localized, self-confirm and proceed.
- If fixes are substantive or span multiple files, spawn a fresh reviewer (new Agent call, not a resume) — its context is clean per iteration.

Loop until no Critical/Important issues remain.

### 5. Doc pipeline

1. Dispatch **mental-model-updater** with the commit range and a brief implementation summary. Wait for completion.
2. Refresh `ai-docs/_index.md` — update inventory, descriptions, and layout to reflect current state.
3. If ticket-driven:
   1. Append `### Result (<short-hash>) - YYYY-MM-DD` to each completed phase. Content: what was implemented, deviations from plan, key findings for future phases. Short hash = last implementation commit.
   2. Move ticket to the next status directory (`git mv`) if all phases are complete.

### 6. Report

Report to the user:
- What was implemented (brief summary)
- Commit range
- Test status
- Any deviations or open items

## Judgments

### judge: scope-bound

| Decision | When |
|----------|------|
| Proceed with /implement | Main agent is warm on the target area; change is small (single file or tightly coupled pair, or clear cross-module changes with established pattern); verifiable with focused tests |
| Escalate to /delegate-implement | Cross-module without clear pattern, or main agent cannot hold full context while editing, or scope exceeds 3+ files with new public API |
| Escalate to /proceed | Upstream artifact missing (ticket, skeleton, or plan) |

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Change is a small isolated edit (single file, no new public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

### judge: needs-review

| Decision | When |
|----------|------|
| Skip | Single file, no public contract changes, follows established patterns |
| Review | 2+ files, or public API modification, or new pattern introduced |

Scope exceeding 3+ files with new public API or architectural change is already out of `/implement` capacity — `judge: scope-bound` escalates to `/delegate-implement` before review fires.

## Doctrine

Implement optimizes for **session-context preservation during code
changes** — the main agent retains its accumulated understanding of
the task through editing rather than forking that context to a
subagent and reconstructing it across the boundary. When review fires,
the reviewer fork is fresh per iteration, so the main agent's
continuity is preserved while the reviewer provides uncommitted
judgment on the diff. When a rule is ambiguous, apply whichever
interpretation better preserves the main agent's continuous context
over the change's full lifecycle.
