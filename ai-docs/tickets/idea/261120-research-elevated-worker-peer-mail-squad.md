---
title: "공유 워크트리 Peer-Mail 스쿼드 연구: 페르소나 기반 실시간 협동 구현·리뷰와 메일박스 감사"
related:
  261117-feat-ws-agent-tools-mailbox-routing: prerequisite - supplies ws/agent.spawn, in-memory peer mailbox routing ([relation: peer]), MaxSpawnDepth=4, and delivery timing
  261116-epic-lead-orchestration-profile: context - elevated worker orchestration context
---

# Shared-Worktree Peer-Mail Squad Research: Persona-Driven Collaborative Implementation, Real-Time Critique, and Mailbox Auditing

## Background

Today `ticket-worker-elevated` avoids concurrent edit races by running leaves
strictly one at a time on a single warm worktree, while parallel `lead-run`
isolates whole tickets in separate worktrees (paying cold builds and merge
integration per worktree). In both models, a single coding agent implements an
entire slice alone and only encounters a second perspective *after* the code
is written and committed (post-hoc review).

With `261117-feat-ws-agent-tools-mailbox-routing` now landed (`ws/agent.spawn`
+ in-memory `ws/mailbox.send` peer routing), a different coordination pattern
becomes testable: an opt-in knob mode on `ticket-worker-elevated` where 2–3
peer agents work on **one shared warm worktree** like a pair/squad programming
team. Rather than acting as interchangeable workers, each peer carries a
distinct engineering persona (for example, *Alex* focusing on correctness,
simplicity, and edge cases; *Chris* focusing on performance, allocations,
file-touch budget, and architectural fit) so they shine different-angle
flashlights on the change *before and during* implementation:

- Pre-edit intent checks ("I plan to touch X using approach Y" ↔ "Watch out
  for Z / optimization / invariant W") while code is still cheap to redirect.
- Live peer critique and boundary negotiation over shared seams.
- **Mini-lead (`ticket-worker-elevated`) as build, test, and commit barrier:**
  peers edit and converse without running competing builds or git commits; the
  mini-lead triggers warm incremental builds/tests at quiescent checkpoints,
  shares failures back to the squad for joint diagnosis, breaks deadlocks, and
  commits clean checkpoints.
- **Mailbox audit / oversight:** supervision over peer-to-peer chatter so
  exchanges remain bounded, on-contract with the ticket, and inspectable.

This ticket frames the hypotheses, failure modes, audit questions, and
empirical experiment needed to decide whether this mode earns a shipped knob.

## Core Mechanism & Persona Hypothesis

1. **Complementary Personas as Real-Time Flashlights.**
   - Hypothesis: Assigning explicit, orthogonal engineering personas (e.g.,
     Correctness/Clarity vs. Performance/Contract-Fit, or Implementer vs.
     Adversarial Tester) catches design/optimization/edge-case blind spots
     *before* code solidifies, reducing post-hoc Critical/Important review
     findings compared to single-author serial leaves.
   - Risk to test: Personas may collapse into performative politeness,
     redundant nitpicking, or endless style debates that burn tokens without
     improving defect detection.

2. **Shared Warm Worktree & Build Barrier.**
   - Leaves do not run builds, test suites, or `git.commit` independently.
   - Instead, leaves negotiate scope/edits and report `ready-for-check` to the
     mini-lead. Once all active leaves reach a quiescent checkpoint, the
     mini-lead runs one warm incremental build/test and broadcasts failures to
     the relevant peers for joint root-cause discussion and repair.
   - Single-committer rule: only the mini-lead commits at green checkpoints,
     preventing git index races and half-edited commits.

3. **File Ownership & Seam Coordination.**
   - Concurrent `edit_file` calls on the same file cause lost updates or broken
     syntax. Candidate guardrails to evaluate:
     - Static file partition assigned by the mini-lead (shared spine/types
       files locked to the mini-lead or one designated owner).
     - Lightweight peer claim messages before touching an unassigned file.

4. **Mailbox Audit & Supervision System.**
   - Peer-to-peer mail between two children is currently invisible to the
     parent's own `ws/mailbox.history` (which lists only the caller's own
     sent/received messages).
   - Without an audit mechanism, peer agents can enter unproductive ping-pong
     loops, drift from settled ticket decisions, or silently agree on a
     workaround that violates a constraint.
   - Candidate audit designs to compare:
     - **Protocol-level (no Go changes):** mandatory concise summary of peer
       agreements in every checkpoint report to the mini-lead + strict
       per-topic round-trip cap (e.g., max 2 round-trips before escalating to
       the mini-lead) + CC to mini-lead on contract/interface changes.
     - **Daemon/MCP-level (new primitive if needed):** parent-scoped read-only
       audit view of descendant peer-mail traffic, or daemon-side turn/chatter
       budget nudges and dashboard thread inspection.

5. **Harness Delivery Timing Asymmetry.**
   - Per `261117-feat-ws-agent-tools-mailbox-routing` Decision 4, `mailbox.send`
     delivers as mid-turn steer only where the harness supports it, and as
     next-turn otherwise (e.g., Claude workers are currently `unsupported` for
     mid-turn steer).
   - Research must check whether next-turn delivery causes stale crossed
     messages during active editing, or whether checkpointed turn boundaries
     actually make next-turn delivery *cleaner* than mid-turn interruption.

## Experiment & Evaluation Plan

Compare **Baseline** (`ticket-worker-elevated` default serial-leaf mode) vs.
**Squad Prototype** (2–3 persona peers on one shared warm worktree with
mini-lead build/commit barrier) on 2–3 realistic multi-module tickets (e.g.,
backend + UI/tests, or cross-cutting refactor + performance constraints):

- **Metrics:** wall-clock time, total tokens & agent turns, build/test
  invocations, post-hoc independent reviewer findings (Critical / Important),
  file-race or broken-snapshot incidents, and peer-mail round-trip count.
- **Go / No-Go Bar:** Proceed to an actionable knob ticket only if the squad
  mode shows either (a) measurably fewer post-hoc Critical/Important review
  findings at comparable cost, or (b) materially faster wall-clock completion
  on build-heavy/cross-module tickets with zero file-race incidents and
  bounded peer chatter.

## Outcome Ledger

### Verified Findings

- `261117-feat-ws-agent-tools-mailbox-routing` (completed 2026-11-18):
  - `ws/agent.spawn` returns `{address, status}` with `MaxSpawnDepth = 4` and
    no concurrent-agent cap (Decisions 1, 7).
  - `ws/mailbox.send(to: <address>)` routes daemon-owned addresses in memory
    with lineage-computed `[relation: peer]` labeling (Decisions 2, 3).
  - Delivery is mid-turn steer when supported by the target harness, else next
    turn (waking sleeping agents); Claude workers currently grade `unsupported`
    for steer (Decision 4).
  - A child's automatic final report to its parent is held while its own
    children are awake (Decision 5).
- Current `ticket-worker-elevated.md` (`c3c1fb1ba`) runs leaves strictly
  serially on one warm worktree (`never in parallel`) specifically to prevent
  concurrent seam-write races and preserve warm-incremental builds.
- `ws/mailbox.history` currently returns only messages sent or received by the
  calling session key; a parent mini-lead has no direct read tool for
  child-to-child peer mail today.
- **Pilot 1 empirical run (2026-11-20, `261120-bug-ws-rail-heartbeat-starvation`,
  code `cd667bc5a`, merged `22f7939ec`):** 3-harness shared-worktree squad
  (Alex `codex.large` Fix 2, Chris `claude.large` Fix 3 + lock hawk, Dana
  `agy.large` adversarial `-race` tester) + independent fresh-context post-hoc
  reviewer (`claude.large`):
  - **Pre-edit huddle value:** Caught 4 concrete concurrency/contract defects
    before any line of code was edited — Trap D (`wsdashboard/live_hosted.go`
    holding global `d.live.mu` across ring I/O), F1/F2 (`conv.regMu` across OS
    flock turning 2s timeout into unbounded wait/re-entrancy deadlock), TOCTOU
    generation-swap race in the sweeper (resolved via `lockSummaryOwner`), and
    unsynchronized mock fidelity (`daemonLiveSlot` + `wsstate.FileIOCounters()`
    budget).
  - **Shared-worktree barrier & failover:** Disjoint file ownership + single
    mini-lead build/test/commit barrier produced **0 file-race incidents**,
    first-try `-race` green builds across all 9 production files and both new
    regression test files, and clean mid-run failover when `agy.large` hit two
    consecutive 45s stream stalls (splitting remaining new tests cleanly across
    `claude.large` and `codex.large`).
  - **Independent post-hoc review stayed mandatory:** Even after the 4 pre-edit
    catches, the fresh-context reviewer caught 2 Important shared-context blind
    spots (`I-1`: sweeper firing at the same 2m threshold as healthy conv loops
    caused an extra `checkRow()` file read and stopped full-row repair; `I-2`:
    missing `Runner.Start` → ticker → `Runner.Close` lifecycle test) plus `M-4`
    (releasing locks before `notify`). Peer-mail collaboration did *not* replace
    fresh-context review.
  - **Voluntary CC leaked:** Under a max-2-peer-message cap, 2 of 3 peers sent
    direct peer messages to squadmates and omitted separate CC mail to the
    mini-lead, empirically confirming that prompt-level CC is unreliable for
    oversight without a daemon-level parent-scoped audit view.
  - **Token ROI signal:** Almost all high-leverage value came from the *pre-edit
    design critique*, while parallel 3-writer editing plus a 4th fresh reviewer
    (~3–4x token footprint) added coordination/failover ceremony without
    proportional speedup.

### Confirmed Decisions

- Scope of this research: evaluate a shared-worktree, persona-driven
  peer-mail collaboration mode as an **opt-in knob** on
  `ticket-worker-elevated` (leaving the existing serial mode as the untouched
  default).
- In the proposed mode, the mini-lead (`ticket-worker-elevated`) owns build,
  test, and feedback timing rather than each leaf running competing builds.
- A mailbox audit/supervision mechanism for peer-to-peer exchanges is an
  explicit pillar of this research.

### Proposals

- **Prototype A (Persona Co-Authors):** 2–3 peers with orthogonal lenses
  (e.g., Alex: correctness/clarity/invariants; Chris: performance/I/O/fit)
  owning disjoint files on one warm worktree, exchanging pre-edit intent
  checks and live critiques via peer mail.
- **Prototype B (Implementer + Adversarial Tester Pair):** One peer implements
  while the other writes edge-case/contract tests on the same worktree; both
  diagnose mini-lead test failures together via peer mail.
- **Prototype C (Single Writer + Early-Spawned Reviewer Pair — high-ROI
  candidate from Pilot 1):** Keep a single `workdir-write` implementer (zero
  file partitioning, zero edit races, zero build-barrier ceremony), but spawn
  the `read-only` reviewer *before* editing begins (optionally cross-model /
  cross-harness). Flow: Writer sends a concise pre-edit design/intent brief →
  Reviewer challenges traps/locks/invariants and issues `GO` → Writer
  implements and runs verification → Reviewer audits the resulting diff (with an
  explicit adversarial prompt to challenge its own Step 1 approval, escalating
  to a fresh third reviewer only on high-risk slices). Reuses the reviewer's
  already-read file context (~1.3–1.5x cost vs. ~3–4x for a 3-writer squad) and
  allows either direct peer-mail or mini-lead-mediated handshakes (built-in
  auditability with zero daemon changes).
- **Guardrail set for first trial (playbook-only, no Go changes):**
  disjoint file partition by mini-lead, single-committer mini-lead, explicit
  `ready-for-check` build barrier, max 2 peer round-trips per topic before
  escalating to mini-lead, and mandatory summary of peer agreements in
  checkpoint reports.
- **Audit escalation option:** if playbook-level checkpoint summaries prove
  insufficient, evaluate a parent-scoped read-only peer-mail audit primitive
  in the daemon (T0 MCP addition) and dashboard visualization.

### Open Questions

1. **Persona calibration:** Which persona splits produce genuine defect/design
   catches without degenerating into performative chatter or sycophancy?
2. **File-ownership granularity:** Is static file partitioning by the
   mini-lead sufficient, or do real cross-module tasks need dynamic file
   claims / shared-interface locks?
3. **Barrier ergonomics:** How reliably does the mini-lead detect a quiescent
   tree across 2–3 peers (`agent.list` + explicit `ready-for-check` mail)
   without stalling when one peer finishes much earlier than another?
4. **Mailbox audit sufficiency:** Are checkpoint summaries + CC rules enough
   for oversight, or does safe operation require daemon-level parent visibility
   into child↔child mail (and/or automated loop/chatter caps)?
5. **Steer vs. next-turn dynamics:** Does next-turn delivery (vs. mid-turn
   steer) help or hurt real-time pre-edit "is this optimization OK?" checks?
6. **Net ROI:** Under the paired benchmark, does squad mode beat serial leaves
   enough on defect rate or wall-clock time to justify the knob and prompt
   complexity?
7. **Prototype C (Early-Spawned Reviewer) anchoring vs. ROI:** When the same
   reviewer approves a single writer's pre-edit plan and later reviews the diff,
   how much confirmation bias / shared-design blindness remains (e.g., would it
   still catch steady-state issues like Pilot 1's `I-1`), and is a
   mini-lead-mediated 2-agent handshake cleaner than direct peer-mail?

### Rejected Alternatives

- Replacing `ticket-worker-elevated`'s default serial-leaf strategy directly
  without an opt-in knob or empirical evidence (too high risk of file races
  and runaway token burn on routine tickets).
