---
name: lead-review
description: Use when the user wants to review a pull request or merge request branch; loads or creates a review config, runs structured review phases, and routes to fix, comment, or merge.
---

# Review

Target: user request

## Invariants

- Load `ai-docs/_review.local.md` before any review step; run setup if absent.
- Never push, force-push, or modify remote branches without user confirmation.
- Record the current branch before checkout; offer to restore it after review.
- Workflow mutations (fixes, commits) are lead-owned; route through `wsflow:lead-discuss` and `wsflow:lead-implement`.
- All written artifacts (review config, findings) must be in English regardless of conversation language.

## On: invoke [branch?]

### 1. Load config

1. Check for `ai-docs/_review.local.md`.
2. If absent → go to **On: setup**.
3. Load all sections present: Remote, Branch Naming, Review Phases, Checklist, Blocked Paths, Comment Method, Merge Approval Method, Notification Method, Contributor Workflow, Deep Review.

### 2. Identify branch

1. If `branch` argument provided, use it.
2. Else → go to **On: branch discovery**.

### 3. Prepare

1. Record `<current-branch>`.
2. Run fetch per Remote config.
3. Checkout target branch.
4. Apply `judge: has-blocked-paths` → if any blocked path found in diff, emit BLOCKED verdict and stop.

### 4. Review

1. Run `wsflow/git.diff(mode: "stat")` → present scope summary.
2. Apply `judge: follows-ws-workflow` → determine intention analysis path.
3. Apply `judge: is-large-diff` → determine phase execution depth.
4. Run review phases in order: intent, alignment, risk, then any custom phases from config.
5. Apply `judge: has-checklist` → present checklist items; collect user confirmation per item.
6. Aggregate findings → emit verdict in **On: verdict**.

---

## On: setup

No config found. Ask for:

1. **Remote access method** — how to list open MR/PR branches and fetch them (glab, API token, git fetch, other).
2. **Branch naming convention** — optional prefix or pattern to filter branches (e.g. `feature/`, `TICKET-[0-9]+`).
3. **Custom review phases** — any checks beyond default intent / alignment / risk.
4. **Blocked paths** — file patterns that must never appear in an MR (optional).
5. **Comment method** — how to post review feedback (glab mr note, GitLab Web UI, none).
6. **Merge approval method** — merge sequence (local merge → push / push → web approve → merge / other).
7. **Notification method** — post-merge contributor notification (comment, Slack, none).
8. **Contributor workflow** — `ws` / `external` / `mixed` (default: `mixed`; auto-detects per MR).
9. **Deep review threshold** — file or line count that triggers subagent parallel analysis (optional; default: 20 files or 500 lines).

Then:
1. Write `ai-docs/_review.local.md` using the template below.
2. Confirm the written config with the user.
3. Return to **invoke step 2**.

### Review Config Template

```markdown
# Review: <project>

## Remote
<how to list and fetch MR/PR branches>

## Branch Naming                       ← optional
<prefix or regex pattern, e.g. feature/, fix/, TICKET-[0-9]+>

## Review Phases
### intent
Commit messages and ## AI Context match the stated ticket or MR purpose.
### alignment
Diff is consistent with ai-docs/spec and mental-model docs.
### risk
No breaking changes, security issues, or missing tests without justification.

## Checklist                           ← optional
- [ ] <gate item>

## Blocked Paths                       ← optional
- <path pattern>

## Comment Method                      ← optional
<glab mr note / GitLab Web UI / none>

## Merge Approval Method               ← optional
<local merge → push / push → web approve → merge>

## Notification Method                 ← optional
<post-merge contributor notification method>

## Contributor Workflow                ← optional; default: mixed
mixed

## Deep Review                         ← optional
threshold: 20 files / 500 lines
```

---

## On: branch discovery

1. Run fetch per Remote config.
2. List remote branches per Remote config (e.g. `glab mr list`, `git branch -r`).
3. If Branch Naming defined, filter by pattern.
4. Present list; ask user to select target branch.

---

## On: verdict

### BLOCKED

Blocked path found in diff. Report offending paths. Do not proceed.
Offer: remove offending files from the branch, or abandon review.

### LGTM

All phases passed; all checklist items confirmed.

1. Apply `judge: has-merge-approval-method` → follow configured merge sequence; else ask "Merge?" and wait for confirmation.
2. Merge on user confirmation.
3. Apply `judge: has-notification-method` → notify contributor per config.

### NEEDS FIX

One or more phases flagged issues.

1. Present findings summary.
2. Ask: fix locally or post to contributor?
3. **Fix locally**:
   a. Route to `wsflow:lead-discuss` with review findings as context.
   b. User proceeds through normal development route (lead-proceed → lead-implement).
   c. Re-invoke `wsflow:lead-review` at user's discretion after fixes.
4. **Post to contributor**:
   a. Apply `judge: has-comment-method` → post findings per configured method.
   b. If no comment method: write findings to `wsflow/path.generate(kind: "review")` artifact.
   c. Exit.

### OPEN

Intent is unclear or architectural judgment is required before a fix decision.

1. Enter discussion with review findings as context.
2. After discussion, re-route to LGTM or NEEDS FIX.

---

## Judgments

### judge: has-blocked-paths

Fires when `## Blocked Paths` is present in config.
Check diff for matching paths immediately after checkout; emit BLOCKED before running review phases.

### judge: follows-ws-workflow

Auto-detect from commit log of the target branch:
- **YES**: all commits have `## AI Context` sections and use conventional commit format.
- **PARTIAL** (some commits qualify): treat as NO (conservative).
- **NO**: plain commit messages without structured AI context.

Config `## Contributor Workflow: ws` forces YES; `external` forces NO; `mixed` (default) auto-detects.

Effect on intent phase:
- YES → in-context analysis; `## AI Context` documents intention directly.
- NO → spawn host-native one-shot subagent for intention inference; present inferred intent to user for confirmation before proceeding with remaining phases.

### judge: is-large-diff

Fires when diff exceeds the configured threshold (default: 20 files or 500 lines).
When fires: spawn host-native subagent(s) for parallel alignment and risk analysis across modules.
When silent: in-context analysis for all phases.

### judge: has-checklist

Fires when `## Checklist` is present in config.
Before final verdict: present each item; collect user confirmation. Any unchecked item → verdict is NEEDS FIX regardless of phase results.

### judge: has-comment-method

Fires when `## Comment Method` is present in config.
In NEEDS FIX post-contributor path: offer comment posting per configured method.

### judge: has-merge-approval-method

Fires when `## Merge Approval Method` is present in config.
In LGTM path: follow configured merge sequence instead of default ask.

### judge: has-notification-method

Fires when `## Notification Method` is present in config.
After merge: execute notification step per config.

---

## Doctrine

Review optimizes for **maintainer decision quality with minimum friction**. Config
captures environment judgment once so invocations stay lightweight. Subagent
depth scales to diff complexity and contributor context, not to a fixed cost.
When a rule is ambiguous, surface the question to the maintainer rather than
assuming.
