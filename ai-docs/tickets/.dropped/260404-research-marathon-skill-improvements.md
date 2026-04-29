---
title: "Marathon skill improvements — reuse heuristics, checkpoint model, persistent reviewers"
related: []
dropped: 2026-04-24
---

# Marathon Skill Improvements

Feedback from first marathon session (Phase 1B core indexing pipeline).
Captures observed friction points and proposed structural changes to
the marathon workflow skill.

## Session Metrics (reference data)

| Metric | Value |
|--------|-------|
| Scope | Phase 1B: 4 implementation phases + mid-session refactor ticket |
| Executors spawned | 5 (chunk, git, chunker, store, ticket-update) |
| Model split | 1 opus (Phase 2 tree-sitter), 4 sonnet |
| Background agents | 2 (mental-model-updater, spec-updater) |
| Source lines added | 1448 |
| Doc lines added | 213 |
| Tests | 27 pass |
| Commits | 7 (4 impl + 3 docs) |
| Review findings | 3 minor, 0 blocker |
| Planner used | 0 (all phases had sufficient ticket spec) |

## 1. Agent Reuse vs Parallel Spawn Heuristics

Current guidance: "Reuse when next task is in the same domain."
Missing: codebase size and parallelism trade-off.

**Proposed decision matrix:**

| Codebase size | Same domain | Different domain |
|---------------|-------------|-----------------|
| Large (10K+ lines relevant) | **Reuse** — context re-read cost is high | **Fresh spawn** — shared context is small |
| Small (<2K lines relevant) | **Fresh spawn** — re-read is cheap, parallelism wins | **Fresh spawn** — no shared context |

"Relevant" = lines the executor would need to read to understand the
task context, not total codebase size.

Additional factor: if the follow-up task has a **data dependency** on
the previous task's output (not just shared types, but actual runtime
results or generated artifacts), reuse is strongly preferred regardless
of size — the executor already knows what it produced.

## 2. Session-Mode Marathon with Checkpoints

### Problem

Current marathon is ticket-scoped with a heavy wrap-up phase:
code review → mental model → spec → ticket docs → merge. This creates:

- A large batch doc update that's harder to review than incremental ones.
- Artificial session boundaries that force team teardown/rebuild.
- Lead reading 1000+ line diffs directly, violating the "don't read
  source" principle as scale grows.

### Proposed Model

Marathon becomes **session-scoped** rather than ticket-scoped. The
current wrap-up is replaced by lightweight **checkpoints** triggered
after each logical implementation unit (roughly: per-phase or per-task
completion).

**Persistent agents across checkpoints:**

| Agent | Role | Model | Lifespan |
|-------|------|-------|----------|
| reviewer | Code review at each checkpoint. Holds accumulated code context. | opus | Full session |
| spec-watcher | Incremental spec updates. | sonnet | Full session |
| mental-model-watcher | Incremental mental model updates. | sonnet | Full session |

**Checkpoint flow:**

1. Executor completes a task and reports.
2. Lead dispatches reviewer (with diff or changed files).
3. Reviewer reports findings → lead compares with mental model.
4. If issues: lead routes fixes to executor.
5. If clean: lead triggers spec-watcher + mental-model-watcher in parallel.
6. Repeat.

**Session end** is a lightweight close: final commit, merge, ticket
update. No batch doc work — it's been done incrementally.

### Token Visibility and Agent Refresh

**Resolved:** The user has real-time visibility into each teammate's
token consumption via the UI. The lead does not — but the user can
request agent refresh at any time ("refresh teammate X").

**Refresh protocol:** On user request, the lead retires the target
agent and spawns a fresh instance with a summary of prior work as
seed context. This is **normal operation**, not a failure mode.

**Dual monitoring model:**
- **Quantitative** (token accumulation → context window pressure):
  monitored by user via UI token counts.
- **Qualitative** (stale assumptions from earlier checkpoints):
  monitored by lead, who holds the mental model and can detect
  inconsistencies in agent reports.

Either party can trigger a refresh. This eliminates the need for
self-report mechanisms or proxy signal heuristics.

**Implication:** Persistent agents are viable without staged rollout.
The reviewer, spec-watcher, and mental-model-watcher can all be
long-lived from day one, with user-initiated refresh as the safety
valve.

### Discarded Candidates

**Staged rollout (discarded).** Lead initially proposed a 3-phase
introduction: (1) wrap-up parallelization + reuse matrix only,
(2) reviewer agent separation, (3) persistent watcher agents.
Rationale was caution around token visibility. Discarded after
user clarified they have real-time token counts in UI — the safety
mechanism is already present, eliminating the need for gradual
adoption.

**Agent self-report of token consumption (discarded).** Lead proposed
executors estimate and report their own token usage (e.g., cumulative
diff lines × ~12 tokens/line) so the lead could decide when to
retire. Discarded — user's UI provides exact data, making
self-report redundant and less accurate.

**Reviewer holding cumulative raw diffs (discarded).** Initial
proposal had the reviewer accumulate raw diffs across checkpoints
for full context. Concern: unbounded growth. Alternative proposed:
reviewer holds only current diff + summary notes from prior reviews.
Resolved by refresh protocol — if the reviewer's context grows too
large, user triggers refresh with summary handoff. The reviewer
does not need to self-manage its context budget.

### Resolved Concerns

**Silent performance degradation.** Concern: persistent agents
hit context limits without the lead noticing, producing degraded
output. Resolved: user monitors token counts and triggers refresh
before limits are reached. Lead monitors output quality for
qualitative drift.

**Context contamination from stale code assumptions.** Concern:
reviewer remembers code from checkpoint N that was refactored at
checkpoint N+2, leading to incorrect review comments. Resolution:
lead holds the mental model (not the reviewer) and cross-checks
reviewer reports. If the lead detects a stale assumption, they
either correct the reviewer via message or trigger a refresh.
This makes the lead the coherence authority, not the reviewer.

**Section 3 (wrap-up parallelization) subsumed by checkpoint model.**
If checkpoints replace batch wrap-ups, the parallelization concern
disappears — persistent watchers update incrementally at each
checkpoint, no batch to parallelize. Section 3 remains as a
standalone improvement only if the checkpoint model is not adopted.

### Checkpoint Trigger

When to run a checkpoint:
- After every ticket phase (natural boundary — matches current
  marathon task granularity).
- Lead's judgment for smaller changes that don't warrant full
  checkpoint overhead.
- NOT after every individual executor task — too fine-grained.

## 3. Wrap-Up Parallelization

Current skill lists wrap-up steps sequentially ("wait for completion
before step 3/4"). Mental model + spec + ticket updates are
independent and should be explicitly marked as parallelizable.

## 4. Scope Expansion During Session

No current guidance for mid-session scope changes (e.g., discovering
a trait needs refactoring while implementing a feature). Observed
pattern that worked:

- **Discussion + ticket creation**: do it in the current session while
  context is fresh.
- **Implementation**: defer to next session unless trivially small.
- **Rationale**: design quality benefits from live context; implementation
  benefits from clean scope.

This should be codified in the skill.
