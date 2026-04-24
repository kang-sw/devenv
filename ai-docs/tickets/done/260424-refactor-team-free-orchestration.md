---
title: "Team-free orchestration: ws:implement refactor + parallel-implement and team-lead deletion"
spec:
  - 260422-implement-skill
spec-remove:
  - 260421-parallel-implement
  - 260421-team-lead
related-mental-model:
  - executor-wrapup
  - workflow-routing
---

# Team-free orchestration: ws:implement refactor + parallel-implement and team-lead deletion

## Background

The current `ws:implement` and `ws:parallel-implement` skills use `TeamCreate`/`SendMessage`/`TeamDelete` for subagent coordination. In practice:
- Mailbox (SendMessage) is used only for report handover, not interactive multi-turn coordination.
- The Team feature is non-deterministic and unstable; plan is to abandon it entirely.
- One-shot sub-agents lack continuation, which is needed for fix loops.

The infra primitives (`ws-call-agent`, `ws-agent`, `ws-declare-agent`) replacing this machinery are already implemented at `claude/infra/`. This ticket rewrites `ws:implement` to use them and deletes the now-unused `ws:parallel-implement` and `ws:team-lead` skills.

## Decisions

- **`ws:parallel-implement` deleted, not migrated.** The `run_request`/`run_approved`/`run_wait`/`run_complete` JSON serialization protocol is the only meaningful use of the mailbox; replacing it with `ws-call-agent` would require redesigning the protocol with no clear benefit. The skill was rarely used in practice.
- **`--bare` flag not used.** Confirmed broken in practice. CLAUDE.md loading by sub-agents is accepted as a side effect; the lead tolerates thinking-block noise in `.result` output without structured parsing.
- **Deterministic UUID scope.** `ws-agent <name>` derives UUID v5 from repo-root + git-branch + name. Same name on the same branch always maps to the same UUID. `ws-declare-agent` clears the session file before each run to prevent stale-session collisions.
- **`--session-id` is create-only.** It errors if the UUID already exists. `ws-declare-agent` + `ws-call-agent --agent <name>` handles the create-or-resume lifecycle automatically.
- **Session file path.** `~/.claude/projects/<escaped-cwd>/<uuid>.jsonl` where `/` in CWD is replaced with `-`.
- **Plugin-distributed reference.** `ai-docs/spec/` is project-local and does not ship with the plugin. Interface documentation for the orchestration primitives lives at `claude/infra/ws-orchestration.md` (already committed). Skills load it via `load-infra ws-orchestration.md`. Phase 1's rewritten SKILL.md should include this load at the start of On: invoke.
- **`load-infra` discoverability.** `load-infra` with no arguments now lists all available infra docs and bin scripts. Implementer may call it to orient.

## Constraints

- `$(ws-call-agent ...)` command substitution corrupts multi-byte characters. Always use the pipe-direct pattern: `ws-call-agent ... | jq -r '.result'`.
- UUID fields (`session_id`) are ASCII-only and safe for `$()` substitution.
- `ws-declare-agent` must be called before any `ws-call-agent --agent` call within a run.

## Phases

### Phase 1: Rewrite ws:implement SKILL.md

Replace all `TeamCreate`/`SendMessage`/`TeamDelete` machinery with `ws-call-agent` calls.

**Structural changes:**

1. Remove the `/team-lead` load step (Prerequisite step 0).
2. Replace `TeamCreate` with `ws-declare-agent` call at the start of Prepare, listing all agent slots upfront:
   ```
   ws-declare-agent implementer reviewer-correctness reviewer-fit reviewer-test
   ```
3. Spawn implementer:
   ```
   ws-call-agent sonnet --agent implementer \
     --system-prompt claude/infra/implementer.md \
     "<brief + context>" | jq -r '.result'
   ```
4. Spawn reviewers (parallel — issue multiple Bash calls in the same response):
   ```
   ws-call-agent sonnet --agent reviewer-correctness \
     --system-prompt claude/infra/code-review-correctness.md \
     "<diff range + scope>" | jq -r '.result'
   ```
   Same pattern for `reviewer-fit` and `reviewer-test`.
5. Fix loop (relay findings → implementer fixes → re-review):
   ```
   ws-call-agent sonnet --agent implementer \
     "Fix these issues:\n<findings>" | jq -r '.result'
   ws-call-agent sonnet --agent reviewer-correctness \
     "Re-review. Updated diff: <diff>" | jq -r '.result'
   ```
6. Remove `TeamDelete` cleanup step. Replace cleanup with a comment (no-op — session files are naturally scoped to the run via `ws-declare-agent`).

**Prompt content changes:** Remove all `SendMessage`-protocol instructions from implementer and reviewer spawn prompts. Replace with: "Report completion in plain text. For fix cycles, a follow-up message will arrive with review findings — fix and report back."

**Acceptance criteria:** SKILL.md contains no references to `TeamCreate`, `TeamDelete`, `SendMessage`, or `/team-lead`. All agent interactions route through `ws-call-agent`.

### Result (77effb0) - 2026-04-24

SKILL.md rewritten. All Team machinery replaced with ws-call-agent/ws-declare-agent. Stale references also cleaned in proceed, discuss, write-skeleton, manual-think skills (part of deletion commit a45ce1a).

Deviations:
- ws-* scripts were in `claude/infra/` but not on PATH — added bin/ symlinks (942ffac) then moved to actual files in bin/ per user direction (3241b58). `ws-orchestration.md` description updated accordingly.
- Post-review fix pass (cf66b43) cleaned stale "Teammates stay alive" invariant, orphaned `/tmp/claude-reviews/` rm, and `\n` literal in shell strings.

### Phase 2: Delete ws:parallel-implement and ws:team-lead

1. `git rm -r claude/skills/parallel-implement/`
2. `git rm -r claude/skills/team-lead/`
3. `git rm claude/infra/parallel-implementer.md` (only referenced by parallel-implement spawn prompts)
4. Commit with `## Spec` section containing:
   ```
   removed: 260421-parallel-implement
   removed: 260421-team-lead
   ```

Phase 2 has no dependency on Phase 1 ordering — both phases can be committed in either order. Phase 2 is mechanical; no logic changes.

**Acceptance criteria:** `grep -r "TeamCreate\|SendMessage\|TeamDelete\|team-lead\|parallel-implement" claude/skills/` returns no results.

### Result (a45ce1a) - 2026-04-24

Deleted `claude/skills/parallel-implement/` and `claude/skills/team-lead/`. Also cleaned stale references to deleted skills in proceed, discuss, write-skeleton, and manual-think SKILL.md files.

Deviations:
- Ticket step 3 (`git rm claude/infra/parallel-implementer.md`) was incorrect — that file lived inside `claude/skills/parallel-implement/` and was removed by step 1's `git rm -r`. No separate infra removal was needed.
