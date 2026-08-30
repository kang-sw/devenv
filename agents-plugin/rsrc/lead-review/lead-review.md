---
kind: print
---

# Review

Target: user request

## Invariants

Config Load
- Branch scenario: load `ai-docs/_review.local.md` before any review step; run setup if absent.
- Range scenario: load `ai-docs/_review.local.md` if present; if absent, proceed on built-in Review Phases / Landing Lens / Deep Review defaults and never run setup.
- A present config's Review Phases, Checklist, Blocked Paths, and Deep Review sections are honored by both scenarios; `## Landing Lens` is honored by the range scenario only — branch scenario ignores it even if present.

Landing Lens
- Range scenario runs a required `landing` phase — convention adherence plus spec/mental-model update completeness — using config text if `## Landing Lens` is present, else the built-in default.
- Branch scenario never runs the `landing` phase; no config section re-enables it there.

- Never push, force-push, or modify remote branches without user confirmation.
- Branch scenario: record the current branch before checkout; offer to restore it after review.
- Workflow mutations (fixes, commits) are lead-owned; route through `{{.SkillNamespace}}:lead-discuss` and the lead-implement procedure.
- All written artifacts (review config, findings) must be in English regardless of conversation language.

## On: invoke [branch?] [range: <base>..<head>]

Determine scenario kind first: `range` argument supplied → range scenario; `branch` argument or default → branch scenario. `range` and `branch` are mutually exclusive; if both are supplied, range takes precedence. Each sub-step below follows the branch matching the determined scenario.

### 1. Load config

1. Check for `ai-docs/_review.local.md`.
2. Branch scenario, absent → go to **On: setup**.
3. Range scenario, absent → proceed on the built-in Review Phases / Landing Lens / Deep Review defaults from **On: setup**'s Review Config Template; never go to **On: setup**.
4. If present (either scenario): load all sections present: Remote, Branch Naming, Review Phases, Landing Lens, Checklist, Blocked Paths, Comment Method, Merge Approval Method, Notification Method, Contributor Workflow, Deep Review. Range scenario ignores Remote, Branch Naming, Comment Method, Merge Approval Method, Notification Method, and Contributor Workflow — none apply to a checkout-free review. Branch scenario ignores Landing Lens regardless of Contributor Workflow — it is a range-scenario-only check, not a contributor-type exception.

### 2. Identify branch (branch scenario only)

1. If `branch` argument provided, use it.
2. Else → go to **On: branch discovery**.
3. Range scenario: skip this sub-step — the caller-supplied `base..head` is the identified target.

### 3. Prepare

1. Branch scenario: record `<current-branch>`.
2. Branch scenario: run fetch per Remote config.
3. Branch scenario: checkout target branch.
4. Apply `judge: has-blocked-paths` against the target diff (branch scenario: post-checkout diff; range scenario: `range: <base>..<head>` diff, no checkout) → if any blocked path found, emit BLOCKED verdict and stop.

### 4. Review

1. Branch scenario: run `{{.McpNamespace}}/git.diff(mode: "stat")` → present scope summary.
   Range scenario: run `{{.McpNamespace}}/git.diff(range: "<base>..<head>", mode: "stat")` → present scope summary; use `{{.McpNamespace}}/git.log(range: "<base>..<head>")` for commit enumeration.
2. Apply `judge: follows-ws-workflow` → determine intention analysis path.
3. Apply `judge: is-large-diff` → determine phase execution depth.
4. Run review phases in order: intent, alignment, risk, then any custom phases from config. Range scenario also runs the required `landing` phase last (see Invariants: Landing Lens).
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

## Landing Lens                        ← optional to customize; range scenario always runs it (built-in default below if omitted); branch scenario never runs it
Diff follows repo conventions (AGENTS.md, skill-authoring, wsflow-mirroring
where applicable). Caller-visible behavior changes have a matching spec update
(spec describes caller-visible behavior); workflow-system modification-relevant
changes have a matching mental-model update (mental model captures
modification-relevant operational knowledge) — each doc updated per its own
function, not just "any doc touched."

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
   a. Route to `{{.SkillNamespace}}:lead-discuss` with review findings as context.
   b. User proceeds through normal development route (lead-proceed → lead-implement).
   c. Re-invoke `{{.SkillNamespace}}:lead-review` at user's discretion after fixes.
4. **Post to contributor**:
   a. Apply `judge: has-comment-method` → post findings per configured method.
   b. If no comment method: write findings to `{{.McpNamespace}}/path.generate(kind: "review")` artifact.
   c. Exit.

### OPEN

Intent is unclear or architectural judgment is required before a fix decision.

1. Enter discussion with review findings as context.
2. After discussion, re-route to LGTM or NEEDS FIX.

---

## Judgments

### judge: has-blocked-paths

Fires when `## Blocked Paths` is present in config.
Check the target diff for matching paths before running review phases (branch scenario: post-checkout; range scenario: `range: <base>..<head>` diff, no checkout); emit BLOCKED if any match.

### judge: follows-ws-workflow

Auto-detect from the commit log (branch scenario: target branch; range scenario: `range: <base>..<head>` log):
- **YES**: all commits have `## AI Context` sections and use conventional commit format.
- **PARTIAL** (some commits qualify): treat as NO (conservative).
- **NO**: plain commit messages without structured AI context.

Config `## Contributor Workflow: ws` forces YES; `external` forces NO; `mixed` (default) auto-detects.

Effect on intent phase:
- YES → in-context analysis; `## AI Context` documents intention directly.
- NO → use subagent analysis for intention inference; present inferred intent to user for confirmation before proceeding with remaining phases.

### judge: is-large-diff

Fires when diff exceeds the configured threshold (default: 20 files or 500 lines).
When fires: use subagents for parallel alignment and risk analysis across modules.
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
