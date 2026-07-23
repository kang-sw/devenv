---
title: Reshape drain-ready-queue into lead-goal-step — goal-primary identity, blocked-progress completion, autonomous in-scope bug capture
sage-review-design: completed
related:
  260722-feat-goal-run-autonomy-posture: sibling goal-loop body edit; its Phase 2 (Go tips + spec) references the same SKILL.md this rename moves — coordinate sequencing
spec:
  - 260723-lead-goal-step-rename-reposition
  - 260723-goal-step-ticket-curation-authority
  - 260723-goal-step-blocked-progress-conclusion
sage-review-completeness: completed
---

# Reshape drain-ready-queue into lead-goal-step

## Background

`lead-drain-ready-queue` has accreted the goal-run loop role — goal-branch
staging, autonomy posture, terminal merge — on top of its original single-shot
"drain one ready ticket" utility. Two problems follow:

1. **Name/identity lag.** The name advertises a single-shot drain, but in
   practice the skill is invoked only as the per-turn step of a `/goal` pursuit
   run (user confirmed the bare non-goal drain entry point is effectively never
   used). The name and the spec/mental-model framing still present the standalone
   single-cycle shim as the primary identity.

2. **No blocked-progress terminal.** The only clean terminal condition today is
   "`ready/` queue empty". A genuinely blocked goal run (no makeable progress)
   surfaces only as an AI-judged hard-gate *pause* — during an unattended goal
   run that means the loop stalls waiting for an away human while the `/goal`
   Stop-hook keeps re-surfacing the reminder, i.e. thrash. There is no framing
   that treats "no path forward" as a clean goal-run conclusion.

Separately, in devenv today bugs discovered mid-run are captured (per this
repo's AGENTS.md "Dogfood surprises get captured") at `idea/` level — inert,
awaiting human triage. During an unattended goal run that parks actionable work
the loop could otherwise fix in a later iteration. Note: that AGENTS.md rule is
devenv's local convention; the shipped skill's own bug routing must be judged
separately (see the bug-capture decision), since the skill runs in downstream
repos.

### Mechanism facts (verified, load-bearing for the design)

- `/goal` is a **Claude Code harness built-in**, external to this repo. This repo
  has no `/goal` command file and no Stop-hook config; it only consumes the
  reminder as a signal (confirmed: no `commands/`/`hooks/` dir in
  `agents-plugin*`; `workflow-skills.md:425-432`).
- The Stop-hook *re-firing* is deterministic (harness re-surfaces the goal
  reminder every turn), but the **continue-vs-stop decision is AI judgment over
  the skill body prose** — there is no deterministic branch/queue-checking gate
  script. Therefore the completion semantics are controlled entirely by body
  wording; **the skill name does not drive loop behavior** (the hook reads the
  body, not the name).
- The skill is a single-cycle shim: one invocation resolves at most one ready
  ticket and stops; the loop is the caller's (`/goal`) responsibility
  (`workflow-skills.md:425`).

## Decisions

### Rename to `lead-goal-step` (identity reposition, not a bare swap)

- New name: **`lead-goal-step`**. Rationale: `goal` carries the pursuit/mode
  identity the user wants surfaced; `step` matches the actual per-invocation
  single-cycle behavior. Crucially, `step` reads as single-shot only when
  unanchored — anchored by `goal` it means "one step toward the goal", which
  implies further steps follow, directly negating the single-shot connotation
  that `drain`/`step-ready-queue` carry.
- Because the bare non-goal drain is de-facto unused, the fix is a **reposition**:
  promote goal-pursuit-step to the primary identity in the SKILL.md body, spec
  (`workflow-skills.md`), and mental-model (`workflow-skills.md`), demoting the
  single-cycle shim to a degenerate case. A name swap without this reposition
  would leave the spec/mental-model contract still describing a standalone shim
  as primary.
- The `ready/` queue remains the sole progress gate; the body's first line must
  state this explicitly so dropping the `ready` token from the name loses no
  information. Misfire is self-correcting: with no ready tickets the agent simply
  reports "nothing to push" and stops.

### Rejected name alternatives

- **`lead-goal-step-on-ready-queue` / `step-ready-queue`** — rejected. First is
  too long to type for a frequently-invoked skill; second still triggers the
  single-shot reading (no `goal` anchor) the rename is meant to remove.
- **Keep `lead-drain-ready-queue`** — rejected. Name lags the real (goal-only)
  usage; `drain` reads as a terminal single action.
- **Rename only (no reposition)** — rejected. Leaves spec/mental-model presenting
  the standalone shim as primary identity; incoherent with the new name.

### Lead ticket-curation authority during the goal step (prose, no new ticket-system state)

The goal step grants the lead explicit authority to curate the ticket queue as
part of advancing the goal — not only "pick one and hand off": autonomously edit
existing tickets (record findings, restructure, re-triage status) and create +
link new tickets, all via the normal ticket-write path (`lead-write-ticket` /
`ws/tickets.*`), within the goal-run autonomy bounds. This single authority
clause is the shared mechanism for BOTH blocked-progress handling and in-scope
bug capture below — **no new ticket-system state** (no `blocked:` field / marker
substrate) is introduced. Rationale: the entire loop is already AI-judged prose
(see Mechanism facts); a lone structured-determinism island would be inconsistent
and would add set/clear lifecycle machinery the prose authority does not need.
Blocked state and discovered bugs are recorded as ordinary ticket edits on living
documents, which the selector (which reads ticket state/bodies) then sees —
durable across the stateless re-invocations and compaction because it is on disk,
not in the lead's context.

### Blocked-progress = clean goal-run conclusion (distinct from hard-gate pause)

Add a completion-term to the body so the AI-judged loop treats "no makeable
progress" as a clean terminal, not a stall. Two states MUST stay distinct — the
wording must not let one collapse into the other:

- **Hard-gate pause** (progress IS possible but needs human sign-off:
  irreversible/destructive, scope into public API / cross-module, unresolved
  binding decision, any AGENTS.md "Always ask"): keep today's behavior — stop and
  wait. The goal-run autonomy posture (shipped in `260722`) governs this.
- **Blocked-progress conclusion** (no path forward without a human decision, and
  continuing would only re-surface the same block every turn): terminate the
  goal run cleanly and report the blocker; do NOT loop.

Scoping guard: "blocked-progress" means **all remaining `ready/` tickets are
blocked / none can advance**. If ticket A is blocked but B/C are workable, this
is NOT a conclusion — skip A and continue to the next workable ticket.

Mechanism (per the ticket-curation authority above, no new state): when the lead
hits a genuine block on a ready ticket, it **records the blocker onto that ticket
and re-triages it out of the active ready path before yielding the turn** — an
ordinary ticket edit (a recorded blocker note and/or a status re-triage), not a
new marker field. The selection step **reads ticket state/bodies to judge
"advanceable now"** and skips tickets carrying a recorded block, replacing the
body-blind FIFO pick of today. This is why the block survives the stateless
re-invocations and compaction: it lives on the ticket, not in the lead's head.

Discriminator (pause vs conclusion), sharpened per design review: the test is
**"is there any work I could still do without the human?"** — NOT "does this
re-surface every turn?" (which fails to separate the two in the single-ready,
human-away case). Work remains and only a final irreversible/destructive action
awaits sign-off → pause. No advanceable work remains anywhere in `ready/` (every
path needs a human decision) → conclude with a blocker report.

Robustness risk (named): if a block is discovered but NOT recorded before the
turn yields (e.g. mid-turn compaction), the next selector re-picks it and thrash
returns. Mitigation is prose, not structure — "record the blocker before
yielding" must be an explicit, non-skippable body step, and the selector's
advanceable-now read is the second line of defense.

Anti-abuse: the conclusion term must not become an escape hatch that reclassifies
a hard-gate pause as "goal complete" to avoid waiting. It fires only when there
is genuinely no makeable progress, and it terminates with an explicit blocker
report rather than silently marking the goal done.

### Autonomous in-scope bug capture (bounded)

When a bug is discovered mid-goal-run, make it actionable for a later loop
iteration instead of parking it as an inert `idea/` note — but bounded to prevent
unattended scope explosion and non-termination. Three guardrails (all confirmed):

1. **Route through the ticket-write path, not direct-to-`ready/` file drop.**
   Promote via the normal `lead-write-ticket` / `ws/tickets.create` path (under
   the goal-run autonomy posture so reversible parts self-resolve), so the sage
   ready-landing gate and frontmatter discipline are not bypassed. (Bug fixes are
   AGENTS.md "auto-proceed" and bug tickets commonly skip spec, so spec-addressing
   is usually not a hard blocker; the real gate is the sage design gate.)
2. **Scope bound: only goal-relevant bugs reach `ready/` for same-run fixing.**
   A bug that blocks or is directly relevant to the current goal → promote to
   `ready/`. An incidental / unrelated-module bug → `idea/` capture (the away
   human triages later). This routing is **skill-intrinsic behavior stated in the
   skill body**, judged separately from any downstream project's own dogfood-
   capture convention (e.g. devenv's AGENTS.md "Dogfood surprises → idea/") — the
   skill ships into downstream repos and must not anchor its routing to devenv's
   local rule. It preserves scope control and prevents the loop self-feeding on
   tangential bugs into non-termination.
3. **Respect explicit deferral.** If implementation of the bug was explicitly
   deferred, capture only — do not queue it to `ready/`.

Same-run fixing vs the sage gate (resolved per design review): a goal-relevant
bug ticket routed to `ready/` still passes the sage ready-landing gate. If sage
blocks it, the ticket parks in `ready/` carrying sage's existing rendered Blocked
section — which is exactly an on-ticket blocked state the selector already handles
via the blocked-progress mechanism above: the next step skips it and works other
tickets, or concludes if nothing else can advance. So same-run fixing holds for
bugs that clear the gate; sage-blocked bugs park visibly and are picked up once
unblocked. This is the honest contract — the feature does not undermine or
auto-clear the gate. (The sage Blocked section is also the reuse precedent showing
on-ticket blocked state needs no new field.)

### Rejected: unconditional direct-to-`ready/` bug drop

Rejected. Bypasses the sage ready-landing gate and frontmatter/spec discipline,
and — with only an "unless explicitly deferred" opt-out — defaults to fixing
every discovered bug unattended, risking scope explosion and loop
non-termination (bug-fix reveals bug reveals bug...). The scope bound above is
the accepted form.

### Rejected: structured blocked-marker / new ticket-system state

Rejected. Adding a `blocked:` frontmatter field (or a `.blocked/` status) plus its
set/clear lifecycle would be a lone island of determinism in an otherwise
prose-judged loop, and new substrate to maintain. The lead's ticket-curation
authority + the selector's advanceable-now read over ordinary ticket edits cover
the same need on living documents, durably across compaction, with no new state.
(Chosen over the earlier discussion's separate substrate-ticket option.)

## Constraints

- **Single source of truth for completion semantics.** The authoritative
  completion/posture rule lives in the skill body only. Do not scatter goal-aware
  or completion clauses into `lead-implement`/`lead-proceed` (mirror the
  `260722` single-source constraint).
- **Rename blast radius (verified) — the rename is a mechanical sweep, not just a
  frontmatter edit.** Touch points: both skill directory names
  (`agents-plugin/skills/...` + `agents-plugin-wsflow/skills/...`),
  `name:` frontmatter in both, `agents-plugin/skills/manifest.json` path key,
  the `substitutionMirroredSkills` list in
  `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go` (shipping Go is
  generic — no production Go change), Python tests
  (`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` EXPECTED_SKILLS /
  EXPECTED_INLINE_SKILLS; `agents-plugin/tests/test_skill_dispatch_contracts.py`
  method + `SKILLS_DIR` path + the inlined-body assertion string),
  spec + mental-model prose (`ai-docs/spec/workflow-skills.md`,
  `ai-docs/mental-model/workflow-skills.md`), and `ai-docs/ref/wsflow-mirroring.md`.
  Spec/mental-model anchors (`{#260703-drain-ready-queue-skill}`,
  `{#260707-drain-goal-branch-staging}`) are date-keyed stable IDs — do NOT change
  them on rename. Historical CHANGELOG and `.done/`/`.dropped/` ticket references
  are left as-is (immutable history).
- **wsflow substitution mirror.** `lead-drain-ready-queue` is a
  substitution-mirrored inline-body wsflow skill: edit canonical source only,
  regenerate the `agents-plugin-wsflow` mirror (`WS_REGEN_WSFLOW_SKILLS`), and
  keep the drift guard + wsflow package tests green. Posture/completion text must
  stay namespace-clean to clear the generation eligibility guard.
- **Skill-authoring invariants.** Every changed Invariants/Constraints/behavior
  line follows `agents-plugin/skills/lead-skill-authoring/SKILL.md` checklist.
- **Version bump on dev-merge** per AGENTS.md (`bump-ws-version.sh`).
- **Do NOT remove the non-goal defensive branches in this ticket.** Removing the
  documented "when not `goal/*`, behave like today" branches is deleting
  documented behavior (AGENTS.md "Always ask") and is deliberately out of scope
  here; deferred to a separate ticket.

## Prior Art / Reuse

- `260722-feat-goal-run-autonomy-posture` — sibling goal-loop body edit. Its
  Phase 1 (autonomy posture, shipped v0.34.5) is the "hard-gate pause" side this
  ticket's completion-term must stay consistent with. Its Phase 2 (Go
  reinforcement tips + spec) is unshipped and edits the same SKILL.md this rename
  moves — see sequencing below.
- Existing goal-branch-conditional behavior (`policy.branch.merge_confirm: "skip"`
  on `goal/*` handoff) is precedent for goal-branch-conditional logic already in
  this body.

## Sequencing with 260722

The rename moves the canonical SKILL.md path that `260722` Phase 2 edits, and the
completion-term must not contradict `260722` Phase 1's autonomy posture wording.
Coordinate: either land `260722` Phase 2 first, or land this rename first and
re-point `260722` Phase 2's file target. Resolve the order at proceed/ready-promotion time.

## Phases

### Phase 1: Rename + identity reposition

Rename `lead-drain-ready-queue` → `lead-goal-step` across the verified blast
radius (see Constraints), and reposition the identity: promote goal-pursuit-step
to primary in the SKILL.md body opening, spec (`workflow-skills.md`), and
mental-model (`workflow-skills.md`); demote the single-cycle shim to a degenerate
case; state in the body's first line that `ready/` is the sole progress gate.
Keep date-keyed anchors unchanged. Regenerate the wsflow mirror and the
`agents-plugin/skills/manifest.json`.

Verification: skill-authoring invariant checklist passes for changed lines; drift
guard + wsflow package tests + both Python test suites green (run with
`-count=1` / fresh regen to avoid a cached pass — cf. the `260722` v0.34.4 stale
manifest incident); `go test ./...` green (test-list update only).

### Phase 2: Blocked-progress completion term + autonomous in-scope bug capture

Add to the (renamed) skill body: (a) the lead ticket-curation authority clause;
(b) the blocked-progress-conclusion term, distinct from hard-gate pause, with the
"all remaining ready blocked" scoping guard, the non-skippable "record the blocker
before yielding" step, the selector's advanceable-now read (replacing body-blind
FIFO), and the sharpened "is there work I could do without the human?"
discriminator; (c) the bounded autonomous bug-capture posture (route via
ticket-write path; goal-relevant → `ready/`, unrelated → `idea/`; respect explicit
deferral; routing is skill-intrinsic, not anchored to a downstream AGENTS.md).
Wording must not nullify hard gates, let a pause be reclassified as completion, or
introduce any new ticket-system state field.

Verification: skill-authoring invariant checklist passes; completion/pause
distinction is unambiguous against the `260722` hard-gate list; the discriminator
and record-before-yield step are explicit; bug-capture guardrails all present and
decoupled from devenv's AGENTS.md; no new frontmatter/state field introduced;
wsflow mirror regenerated and package tests pass.

Depends on Phase 1 (edits the renamed body).

Spec addressing: contract-first `🚧` planned entries authored at ready-promotion
in `ai-docs/spec/workflow-skills.md` under the existing drain anchors (stems in
`spec:` — `260723-lead-goal-step-rename-reposition`,
`260723-goal-step-ticket-curation-authority`,
`260723-goal-step-blocked-progress-conclusion`). Implementation removes the `🚧`
markers as each behavior lands.
