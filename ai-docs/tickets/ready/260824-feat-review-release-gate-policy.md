---
title: Review policy config + release gate — AGENTS.md review-track, host-neutral gating, devenv ship gate
parent: 260824-epic-review-watermark-model
related:
  260824-feat-lead-review-range-scenario: prerequisite — the gate reviews a range through this scenario
  260824-feat-review-watermark-ledger: prerequisite — the gate reviews the unreviewed range up to the marker
  260829-research-review-watermark-multi-maintainer-model: revises this ticket — re-keys the gate range to the last stable release tag (squash-robust), demotes the precise marker to advisory, adds the rendezvous-backend config field; the forcing function and mandatory boundary review are unchanged
sage-review-design: completed
sage-review-completeness: completed
---

# Review policy config + release gate — AGENTS.md review-track, host-neutral gating, devenv ship gate

## Background

The mechanism (range review + marker + sweep) is host-neutral; whether and where
review *blocks* is per-project policy. This ticket adds the policy surface and
the one mandatory gate — the release boundary — for projects that declare one.
devenv is such a project (`develop`→`main` ship); a messy downstream with no
release boundary declares none and gets advisory-only review. Depends on the
range scenario (②) and the marker/ledger (③).

Circled numbers denote the epic's sibling children: ② =
`260824-feat-lead-review-range-scenario`, ③ =
`260824-feat-review-watermark-ledger`, ④ = this ticket
(`260824-feat-review-release-gate-policy`) (see `related:`).

## Decisions

Settled at the epic level; restated as constraints:

- **config split (three homes):** `AGENTS.md` (tracked, per-track) declares the
  review-track branch, whether a release boundary exists, **and (2026-08-29,
  `260829`) which rendezvous backend the project uses — `platform` (GitHub branch
  protection) or `canary` (any-git ledger conflict)**; the `ai-docs/` ledger
  holds marker+verdict state; `_review.local.md` holds machine-local review
  mechanics. Do not put the review-track branch in `_review.local.md` (it is a
  shared structural fact, and local config is gitignored) or the marker in
  `AGENTS.md` (churn).
- **Gate range is keyed to the last stable release tag, not the precise marker
  (2026-08-29, `260829`).** The hard gate reviews `<last-release-tag>..HEAD`.
  Rationale: tags are not orphaned by squash/rebase, so the gate is **squash-robust
  by construction** and sweeps everything since the last release regardless of how
  it landed (skip-coverage at release granularity). The precise SHA marker is
  **demoted to the advisory mid-stream nudge layer** (sizing the checkpoint nudge),
  where squash imprecision is harmless. The forcing function (below) is unchanged —
  it keys off recorded `block` entries and their routed tickets, not off the
  marker. Note this is a *review-mandate* re-key, **not** a demotion of the gate to
  advisory: a boundary project still runs a mandatory review and the forcing
  function still hard-blocks; only the *range selector* changes from marker to tag.
- **Host-neutral first:** never encode devenv's `develop`/`main`/ship shape as
  the mechanism. Gating is opt-in; absence of a declared boundary means
  advisory-only, not "no review."
- **workflow_manual discovery:** when the review-track branch is unset,
  `workflow_manual` surfaces a **non-blocking** "configure this first" nudge at
  most **once per session** (session-scoped, not per-checkpoint), not a hard
  block.
- **Fallback:** a boundary project with no release tag or marker yet can review
  `main..develop` directly (the release branch is itself a natural "reviewed-up-to"
  proxy) until a tag/marker exists. Once stable release tags exist, `<tag>..HEAD`
  is the gate range (above).
- **Finding resolution (epic Cross-Child) — the ticket lifecycle resolves; there
  is no separate "waiver" artifact.** A blocking finding clears when its routed
  ticket reaches a *terminal* state: `.done` (fixed) or `.dropped` (a conscious
  won't-fix / superseded / invalid decision, which already carries a recorded
  rationale by ticket convention). It blocks while the routed ticket is *open*
  (idea/todo/ready), and — the **forcing function** — a recorded `block` ledger
  entry with **no routed ticket at all** is itself un-clearable, so a blocking
  finding cannot be dropped on the floor: the gate blocks until it is routed and
  then resolved. Routing an already-appended un-routed entry is a **corrective
  append-only follow-up** (`<range>: routed -> <stem>`, per ③), not a ledger edit —
  so the forcing function is not a permanent wedge. Never cleared by editing the
  ledger. (An earlier draft named a
  standalone "lead waiver"; it is removed — `.dropped` is the conscious-accept
  path, using existing ticket convention and needing no new config home.)
- **No-boundary scrutiny is a deliberate trade.** A project that declares no
  release boundary gets no hard gate and (per ③) advisory-only sweeps, while
  per-phase review is simultaneously lightened (①) — so its only *mandatory*
  scrutiny is the single per-phase reviewer. This is a stated, accepted outcome,
  not an oversight: ①'s single-reviewer floor keeps it from being "no review," and
  a project opts into the gate by declaring a boundary. (Epic risk framing is
  updated to watch this under-review direction, not only overhead.)

## Phases

### Phase 1: Policy config surface (AGENTS.md + _review.local.md) + workflow_manual nudge

- Define the `AGENTS.md` fields: review-track branch, release-boundary
  declaration (present/absent), **and rendezvous backend (`platform` | `canary`,
  2026-08-29 `260829`)**. When `platform`, document the recommended GitHub
  branch-protection set (require-up-to-date + dismiss-stale-approvals +
  required-checks + disable squash/rebase; merge queue at scale). **Release-tag
  identification (design review, 2026-08-29):** Phase 2's `<tag>..HEAD` range needs
  to know which tags are release tags. The gate identifies the last release tag via
  a tag-glob (default `v*`, resolved with `git describe --tags --match '<glob>'`),
  derivable for devenv from `ai-docs/ship/ws.md`'s `v<version>` scheme; a boundary
  project whose tags use a different namespace declares its release-tag glob
  alongside the boundary declaration. Define the `_review.local.md`
  review-mechanics home (already exists; note the split so nothing double-owns).
- `workflow_manual` scans for the review-track config and emits a non-blocking,
  scoped nudge when unset.

Verification: a repo with the field set exposes the review-track to the sweep;
an unset repo gets exactly one scoped nudge, never a block; the three config
homes have no overlapping ownership.

### Result (ac3b7356) - 2026-08-30

Config surface landed; the mechanism carries no devenv-specific shape.

- **AGENTS.md `### Review Policy` reader** (`wsreview/agents_config.go`, new):
  fail-open `ReadAgentsReviewPolicy` parses four `key: value` fields —
  `review-track`, `release-boundary` (present|absent, default absent),
  `rendezvous-backend` (platform|canary, default canary), `release-tag-glob`
  (default `v*`). Missing file/section/field or malformed enum degrades to the
  default, never an error (mirrors `config.go`'s `StalenessKnob` shape).
- **`ResolveTrack` precedence** (`wsreview/track.go`): the AGENTS.md
  `review-track` declaration now wins; the git heuristic
  (`origin/HEAD`→`main`→`master`) is the fallback only when the field is unset.
  Dropped the prior "does not exist yet / interim fallback" doc framing.
- **Non-blocking session-scoped nudge** (`mcp/review_track_alarm.go` +
  `sessionRecord.ReviewTrackNudgeShown` additive field + `setReviewTrackNudgeShown`
  on `mutateRecord` + two `workflow_manual.go` wiring sites): when `review-track`
  is unset, `workflow_manual` emits a "configure a review track" advisory at
  most once per session, never blocking. This is the first once-per-session
  `workflow_manual` nudge (all prior nudges recompute unconditionally) — captured
  as a mental-model modification guideline (`mcp-runtime.md`).
- **devenv's own config populated** (`AGENTS.md ### Review Policy`):
  `review-track: develop`, `release-boundary: present`, `rendezvous-backend: canary`,
  `release-tag-glob: v*` (lead-adjudicated; `canary` fits devenv's
  single-maintainer-serial reality and needs no platform config; `present` is
  inert until Phase 2 builds the gate). devenv is the dogfooded "set" case; the
  "unset → one nudge" case is covered by `mcp` tests.

Docs: spec `#260830-review-policy-config-surface` (workflow-skills.md, b5f3e38d);
mental-model updates to `review-watermark-ledger.md` + `mcp-runtime.md` (6daf82f7).

Verification: `go build ./... && go vet ./...` clean; `go test ./internal/wsreview/...`
(30) and `./internal/mcp/...` green, including the two new nudge tests. Partitioned
review (correctness=opus, fit, test) all **clean**, zero relays.

Deviations from plan: none. Scope split honored — implementer did code+tests +
devenv config; generic spec prose and mental-model went through the doc pipeline;
`AGENTS.template.md` bootstrap templates left untouched.

Incidental repair: this branch inherited a pre-existing **red** mcp test from ③
Phase 3 — `TestServeStdioReviewMarkerBootstrapCreatesExactlyOneEntryAndIsIdempotent`
asserted `len(raw lines) == 1` but ③'s 12-line banner made the file 13 lines
(banner + 1 entry); idempotency itself was never broken. Fixed to count only
non-`#` entry lines (test-only, 0fdbebc4); verified failing identically at goal
tip 6b8d72f0 before any Phase 1 change. Root cause of the miss: ③ Phase 3's review
ran only `./internal/wsreview/...` and never exercised the `./internal/mcp/`
integration test that also reads the raw ledger.

Deferred (Ask-first, downstream-affecting): adding the four fields to the two
`AGENTS.template.md` bootstrap templates (both packages, lockstep versioned
checklist) so downstream projects adopt the config via ordinary `lead-bootstrap`
upgrade. No downstream project gets these fields until a follow-up addresses it —
route as a follow-up child under epic `260824-epic-review-watermark-model` or fold
into Phase 2 rollout.

### Phase 2: Mandatory release gate (devenv ship)

- For a project that declares a release boundary, insert a **mandatory** range
  review into the promotion path. For devenv: into `lead-ship` pre-flight
  (`ai-docs/ship/ws.md` / `lead-ship` playbook), which today has no review step
  — review the unreviewed range (**`<last-release-tag>..HEAD`**, squash-robust per
  `260829`; `main..develop` fallback when no tag/marker exists yet) before
  `develop`→`main`.
- **Gate verdict (concrete, per the epic's finding-resolution decision):**
  promotion is blocked when **any** of: (a) the just-run range review raises a
  blocking finding; (b) a `block` ledger entry since the last release references a
  routed ticket still *open* (not terminal); or (c) a `block` ledger entry since
  the last release has **no routed ticket** (un-routed → un-clearable, forcing the
  finding to be captured). It clears only when every such finding's routed ticket
  is *terminal* — `.done` (fixed) or `.dropped` (consciously accepted). The gate
  cross-references recorded blocking entries against their routed tickets' status;
  there is no ledger "resolved" flag and no separate waiver record.
- **Promotion atomicity (pin-and-re-assert).** The gate reviews the gate range
  (`<tag>..HEAD`) and then the ship flow ff-merges. Record the reviewed through-SHA and assert the
  review-track tip still equals it at merge time; if the tip moved, re-absorb and
  re-review the delta before promoting (the ③ race path). For devenv's serial
  local ship this holds trivially, but the premise is named rather than assumed so
  the host-neutral generalization to any boundary project is safe.
- Downstream without a declared boundary: no gate inserted; advisory-only.
- Depends on Phase 1, ②, and ③.

Verification: on devenv, ship pre-flight refuses to promote when the range review
raises a blocking finding, when a ledger `block` entry since the last release
points at a still-open routed ticket, or when such an entry has no routed ticket
at all; it proceeds when the range is clean and every such finding's routed ticket
is terminal (`.done` or `.dropped`); the reviewed through-SHA is re-asserted at
ff-merge time (a moved tip forces re-review before promotion); a no-boundary
project's ship/promotion path is unchanged.

## Spec Impact

Target: `ai-docs/spec/workflow-skills.md` (lead-ship gains a pre-flight range
review for boundary projects; the review-track/boundary config contract) and the
`ai-docs/ship/ws.md` config. New caller-visible behavior: a mandatory review
gate at the declared release boundary (devenv ship), host-neutral advisory-only
elsewhere; `AGENTS.md` review-track/boundary fields and the `workflow_manual`
nudge.
