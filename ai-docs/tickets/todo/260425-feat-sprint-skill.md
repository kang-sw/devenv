---
title: Implement /sprint session-container skill
spec:
  - 260425-sprint
  - 260425-sprint-continue
  - 260425-sprint-session-loop
  - 260425-sprint-aware-survey
  - 260425-sprint-wrapup
  - 260425-sprint-implementation-delegation
  - 260425-sprint-discuss-hint
related-mental-model:
  - workflow-routing
  - executor-wrapup
---

# Implement /sprint session-container skill

## Background

The canonical `/discuss` → `/proceed` chain requires explicit user invocation at each step and runs the doc pipeline (spec-updater, mental-model-updater) after every implementation task. For multi-task feature-branch work, this creates friction: each task triggers a doc pass, context re-loading is manual, and there is no branch-scoped wrap-up.

`/sprint` addresses this by holding the full session lifecycle in a single container. The `sprint/` branch naming convention serves as the persistent-state signal — no external file or TaskCreate is needed. Doc pipeline is deferred from per-task to a single consolidated wrap-up at session end.

## Decisions

- **Branch-as-state**: `sprint/` prefix is the sole signal for "sprint in progress." A live `sprint/` branch = wrap-up not yet run. This eliminates TaskCreate-based persistence and enables continuation across context compactions without any resume artifact.
- **Doc pipeline deferral**: `ws:spec-updater` and `ws:mental-model-updater` are suppressed during task execution and run once at wrap-up. Per-task execution is lighter; wrap-up is heavier but more accurate (sees the full branch diff in one pass).
- **2-reviewer delegation**: Test partition omitted (Correctness + Fit only). Sprint execution is lighter-weight than standalone `/implement`.
- **Sprint-aware survey**: Standard `ws:project-survey` cannot detect stale docs. A dedicated Sonnet-overridden agent reads `parent..HEAD` commit messages and cross-references spec/mental-model to annotate staleness.
- **Discuss hint only**: `/discuss` is not blocked on `sprint/` branches — a one-line hint on invoke is sufficient. A hard redirect would break legitimate sub-discussion during sprints.

## Constraints

- `ws-call-agent`, `ws-infra-path`, `review-path`, `executor-wrapup.md` must be reused as-is — no forks or duplications.
- `sprint/` branch convention is load-bearing: renaming or weakening it breaks state detection.
- No doc pipeline per task; `executor-wrapup` runs only at the wrap-up step.
- Wrap-up dispatch order: `ws:spec-updater` first, `ws:mental-model-updater` second — mirrors the `/edit` pattern so `mental-model-updater` sees any 🚧 strips already committed by `spec-updater`.

## Prior Art

- `/implement` skill: `ws-call-agent` dispatch, file-based review protocol, `review-path` allocation, `executor-wrapup` — all reused directly.
- `/forge-spec`: TaskCreate-based persistence (considered for wrap-up state; replaced by branch-as-state).
- `executor-wrapup.md`: canonical playbook for doc-commit gate and ticket update — called at wrap-up step.

## Phases

### Phase 1: Sprint-aware survey infra doc

Author `claude/infra/sprint-survey.md` — the system prompt for the Sonnet-overridden survey agent.

Goals:
- Accepts branch range commit messages (`parent..HEAD`) and project map as input
- Cross-references commit message content against spec stems and mental-model file names to detect drift
- Returns standard `[Must|Maybe]` tier list; entries with potential drift carry a `[stale?]` annotation
- Output format must remain compatible with project-survey consumers (Must/Maybe structure)

Constraints:
- Staleness annotation is soft (`[stale?]`) — callers decide whether to re-read the flagged doc
- Agent is always invoked via `ws-call-agent sonnet --system-prompt "$(ws-infra-path sprint-survey.md)"` — never called with a smaller model

### Phase 2: Core skill file

Author `claude/skills/sprint/SKILL.md`. Phase 1 must be complete before starting.

Goals:
- On invoke: detect `sprint/` prefix on current branch → offer continue | wrap-up | abandon; otherwise ask for sprint name → create `sprint/<name>` branch
- Session loop driven by `judge: delegate` routing table:
  - Questions / explanations → inline answer
  - Codebase exploration → Explore agent
  - Design discussion → inline loop (no `/write-spec` auto-chaining)
  - Simple edits → direct edit, no doc pipeline
  - Complex implementation → delegation via `ws-call-agent` (see Phase 3 for delegation detail)
- Sprint-aware survey fires at session start; re-fires on demand when the domain shifts mid-session
- `ws-declare-agent` called at skill start to clear stale sessions for all named agents

Constraints:
- Session loop must NOT invoke `/discuss`, `/edit`, or `/implement` skills — internalizes their patterns without the doc pipeline
- Direct edit does not call `executor-wrapup`; delegation does not call `executor-wrapup` — all deferred to wrap-up

### Phase 3: Wrap-up procedure

Integrate the hardcoded wrap-up into the skill (may be authored in Phase 2 or as a standalone section). Phase 2 must be complete.

Goals:
- Trigger on explicit user done signal (e.g., "done", "wrap up", "finish sprint")
- Procedure: read full branch diff → dispatch `ws:spec-updater` and wait → dispatch `ws:mental-model-updater` and wait → call `executor-wrapup` → suggest merge or branch deletion
- Implementation delegation within the session uses `ws-call-agent` with 2 reviewers (Correctness + Fit) in parallel, file-based protocol; `review-path` allocated once and cleaned up after the review loop

Constraints:
- `ws:spec-updater` must complete before `ws:mental-model-updater` starts
- `executor-wrapup` doc-commit gate captures doc changes from both updaters
- Wrap-up is not re-entrant; once triggered it runs to completion or fails explicitly

### Phase 4: /discuss hook integration

Add the `sprint/` branch detection hint to `/discuss`. Phase 2 must be complete.

Goals:
- In `claude/skills/discuss/SKILL.md`, add to the `On: invoke` section: if `git branch --show-current` starts with `sprint/`, emit one line — "Note: sprint branch `sprint/<name>` detected — `/sprint` provides session continuity."
- Discuss proceeds normally after the hint; no gate, no redirect

Constraints:
- Change is additive only — no existing discuss behavior is modified
- The hint must not appear when the user is not on a `sprint/` branch
