---
title: Goal-run autonomy posture — stop only on critical during goal pursue
sage-review-design: required
---

# Goal-run autonomy posture — stop only on critical during goal pursue

## Background

During a `/goal` pursue run the lead repeatedly halts on trivial, reversible
decisions — the characteristic failure is "options a and b exist, my
recommendation is a" followed by waiting for the human and re-asking "which
one?" while the human is away. A goal run is meant to execute unattended, so
these discretionary pauses defeat the mode's purpose.

These stalls are diverse in origin (mid-implementation option choices, commit
points, proceed routing) — they are NOT confined to one named gate in
`lead-implement`. They are over-eager application of the AGENTS.md "Ask first"
approval protocol to reversible, local choices, not genuine hard gates.

The goal: during goal pursue, assume the boss is away — proceed on the
recommended option for reversible/local decisions and record it in one line;
reserve stops for genuinely critical (irreversible / destructive / scope-
expanding / premise-breaking) decisions.

## Decisions

### Two layers with distinct roles (authority vs reinforcement)

1. **Authoritative posture lives in the drain-loop skill body**
   (`agents-plugin/skills/lead-drain-ready-queue/SKILL.md`), NOT in
   `lead-implement`. Rationale: the `/goal` Stop-hook re-invokes the drain skill
   every turn until the queue is empty (documented in spec
   `workflow-skills.md:431`), so the drain body is re-surfaced at the top of
   every goal-run turn and therefore covers all sub-skills (proceed, implement,
   commit) for that turn — not just `lead-implement`'s named gates. Because the
   stalls are diverse, a mode-level posture in the loop controller covers them;
   a gate-level edit in one downstream skill would not.

2. **Just-in-time reinforcement tips** injected by decision-point MCP tools when
   the caller's root is on a `goal/*` branch. These are reminders that point
   back at the drain posture ("you are in goal pursue — recall the goal-run
   posture: stop only on critical"), NOT a second authoritative definition.
   Scope confirmed to BOTH sites:
   - `git.commit` handler (`agents-plugin-tool/internal/mcp/server.go` git.commit
     case, existing append point near the `appendSessionKeyTip` trailer).
   - `enter.implement` verdict rendering (the resolver already observes the
     current branch and branches on `impl/` prefixes; add a `goal/` case that
     appends the posture line to the verdict text).

### Layer-2 branch-coverage reality (design-review finding)

Per the drain branch model (`SKILL.md:54-58`, spec `workflow-skills.md:476`),
per-ticket implementation runs on an `impl/<stem>` branch merged into
`goal/<parent>/<slug>`. Consequences for the two Layer-2 sites under the
`goal/*`-only signal:

- `enter.implement` verdict tip fires correctly: at resolve time the lead is
  still on `goal/<parent>/<slug>` before the `impl/*` branch is created. This is
  the reliable per-slice reinforcement point.
- `git.commit` tip does NOT fire on implementation commits: those run on
  `impl/<stem>`, where `HasPrefix(head, "goal/")` is false. It fires only on
  commits made directly on the goal branch (plan artifacts, final merge).

Accepted resolution (keeps the branch-name-only signal intact): implementation
commits are covered authoritatively by Layer 1 (drain body, re-surfaced every
turn, branch-independent) and by the per-slice `enter.implement` verdict tip; the
`git.commit` tip is best-effort for direct goal-branch commits only, NOT a claim
of implementation-commit coverage. See the Open Design Decision below for the
one broadening alternative that would require revisiting a constraint.

### Wording constraint (both layers)

The posture RAISES the auto-proceed threshold; it does NOT nullify gates.
Reversible/local choices → proceed on the recommendation and record in one line,
do not wait for confirmation. Genuine hard gates remain and MUST still fire:
scope expanding into public API / cross-module patterns
(`lead-implement.md:13`), unresolved binding decisions
(`lead-implement.md:67`), resolver `Branch Action: stop`
(`lead-implement.md:62`), and every AGENTS.md "Always ask" item (deleting
functionality, changing canonical flows, protocol/API semantics — i.e.
irreversible/destructive). The tip wording must say "stop only on
critical/irreversible", never "never stop".

### Signal

The durable goal signal is the `goal/*` branch name itself (spec
`workflow-skills.md:490` — the branch checkout is the entire persistent signal;
no new persisted state). Layer 2 keys off `strings.HasPrefix(head, "goal/")`.
The `git.commit` handler already holds `root`; the fresh branch read is
`wsgit.Status(root).Branch.Head`. The `enter.implement` resolver already
observes the branch via `observeImplementBranch`.

## Constraints

- Single source of truth: the authoritative rule is the drain body only. Layer-2
  tips must reference / summarize it, never redefine it, to avoid drift between
  two wordings.
- Do NOT scatter goal-aware clauses into `lead-implement` / `lead-proceed` gate
  text in this ticket. Keep it single-source. Only if a specific gate is later
  observed to keep over-firing during goal runs, add a one-line "in goal-run
  this is discretionary; see drain posture" pointer to that specific gate — as
  separate follow-up, not preemptively.
- Layer 2 must not touch the shared `workflow_manual` banner injectors — those
  run only at session start and would be stale mid-goal. Inject at the decision-
  point handlers directly.
- Version bump on dev-merge per AGENTS.md (`bump-ws-version.sh`), since Layer 2
  changes the plugin runtime.

## Prior Art / Reuse

- `policy.branch.merge_confirm: "skip"` — drain already passes this on `goal/*`
  handoff (`SKILL.md:56-58`); it relaxes only the merge gate
  (`lead-implement.md:16`), NOT the diverse discretionary stalls this ticket
  targets. Precedent for goal-branch-conditional behavior, but not a vehicle
  reused here.
- Existing banner-injection pattern (`injectSkepticalPosture` etc. in
  `workflow_manual.go`) is the shape of a posture reminder, but is session-start
  only — this ticket deliberately injects at decision-point tools instead.

## Rejected Alternatives

- **Authoritative rule in `lead-implement` body** — rejected. Covers only that
  skill's gates; the stalls are diverse and mode-level. Drain (re-surfaced every
  turn) is the correct home.
- **Blanket "auto-proceed except fatal" rule** — rejected. Would override the
  AGENTS.md "Always ask" irreversible/destructive items. Must be threshold-
  raising, not gate-nullifying.
- **New bespoke `policy.*` autonomy flag + Go resolver/schema wiring** —
  rejected as over-engineered for the symptom. The `goal/*` branch name is a
  sufficient, already-available signal; no new policy field is needed.
- **Layer 2 in `git.commit` only** — rejected. `enter.implement` verdict is where
  the per-slice "how to do this slice" decision is rendered and it fires while on
  `goal/*`, so it is the reliable reinforcement site; `git.commit` is included
  only as best-effort for direct goal-branch commits (see Layer-2 branch-coverage
  reality — implementation commits run on `impl/*` and are covered by Layer 1,
  not by the `git.commit` tip).

## Phases

### Phase 1: Authoritative goal-run posture in drain body (+ spec)

Add a goal-run autonomy paragraph to
`agents-plugin/skills/lead-drain-ready-queue/SKILL.md`: when in goal pursue
(on a `goal/*` branch / active `/goal` run), assume the boss is away — resolve
reversible, local decisions on the stated recommendation and record them in one
line; do not wait for confirmation; stop only on critical
(irreversible / destructive / scope-expanding / premise-breaking) decisions.
Wording must not contradict `lead-implement`'s genuine hard gates.

Follow `agents-plugin/skills/lead-skill-authoring/SKILL.md` invariant checklist
for the changed lines. Document the posture in the spec drain anchor (see
`## Spec Impact`).

Verification: skill-authoring invariant checklist passes for changed lines;
posture text is consistent with the hard-gate list; spec anchor updated.

Deferred: no Go changes in this phase.

### Phase 2: Just-in-time reinforcement tips (Go)

Inject a `goal/*`-conditional posture tip at two decision-point handlers:
- `git.commit` — append a tip line when `wsgit.Status(root).Branch.Head` starts
  with `goal/`, at the existing trailer append point.
- `enter.implement` verdict rendering — add a `goal/` branch case (the resolver
  already observes the branch) that appends the posture line to the verdict.

Tip wording references the Phase-1 drain posture; "stop only on critical", never
"never stop". Keep each site self-contained (a few lines per handler); do not
touch the `workflow_manual` banner injectors.

Verification: `go build` / existing MCP test suite passes; manual checks —
(a) `enter.implement` resolved while on `goal/<parent>/<slug>` shows the tip;
(b) `git.commit` made directly on a `goal/*` branch shows the tip;
(c) `git.commit` on an `impl/<stem>` branch does NOT show the tip (documented
limitation, not a bug — see Layer-2 branch-coverage reality);
(d) both on a plain non-goal branch show nothing. Bump plugin version on dev-merge.

Depends on Phase 1 (the authority the tips reference must exist first).

## Open Design Decision

The user explicitly wanted a commit-time reminder, but under the branch-name-only
signal the `git.commit` tip cannot fire on implementation commits (they run on
`impl/*`). Two options, to confirm before Phase 2 is finalized:

- **A (accept limitation — recommended, autonomous):** keep the `goal/*`-only
  signal. Implementation commits are covered by Layer 1 + the `enter.implement`
  verdict tip; the `git.commit` tip stays best-effort for direct goal-branch
  commits. No new state, no constraint change.
- **B (broaden signal — "ask first" scope):** make `git.commit` detect goal-run
  context from an `impl/*` branch by inspecting the fork parent / merge target
  (the goal-branch name is already encoded per commit `3ef852e5`). This delivers
  the literal commit-time reminder the user asked for, but revisits the stated
  "the `goal/*` branch name is the sole signal, no new persisted state"
  constraint, so it is an architecture decision, not an implementer call.

Default if unresolved: A.

## Spec Impact

- Target spec area: `ai-docs/spec/workflow-skills.md`, the drain anchors
  (`{#260703-drain-ready-queue-skill}` / `{#260707-drain-goal-branch-staging}`
  neighborhood) — document that goal pursue carries a goal-run autonomy posture
  (stop only on critical) surfaced authoritatively in the drain body and
  reinforced by `goal/*`-conditional tips at `git.commit` and `enter.implement`.
- Expected caller-visible change: on `goal/*` branches, decision-point tools
  emit a posture reminder and the goal loop proceeds on recommendations for
  reversible decisions instead of pausing.
- Contract-first spec: yes (cross-skill routing / goal-run behavior contract).
  Write the spec entry at ready-promotion / proceed via `lead-write-spec`.
