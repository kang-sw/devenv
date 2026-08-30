---
title: Review policy config + release gate — AGENTS.md review-track, host-neutral gating, devenv ship gate
parent: 260824-epic-review-watermark-model
related:
  260824-feat-lead-review-range-scenario: prerequisite — the gate reviews a range through this scenario
  260824-feat-review-watermark-ledger: prerequisite — the gate reviews the unreviewed range up to the marker
  260829-research-review-watermark-multi-maintainer-model: informed this ticket — contributed the rendezvous-backend config field (kept). Its tag-keyed gate-range re-key is SUPERSEDED (2026-08-30, `## Decisions`) by the ship/review decoupling: the gate range returns to the review marker (frontier), relying on the already-required no-squash/rebase convention rather than tag-robustness.
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-30
---

# Review policy config + release gate — AGENTS.md review-track, host-neutral gating, devenv ship gate

## Sign-off (2026-08-30)

Phase 1 is done (`### Result (ac3b7356)`). Phase 2 was paused pending user
sign-off; the user resolved it in a discuss session that **redesigned the
mechanism** rather than simply approving the paused plan. Ratified outcome (see
`## Decisions`, revised bullets dated 2026-08-30):

- **R5 approved, reshaped** — the mandatory gate lives as a `lead-ship` playbook
  branch (un-omittable), but its stop is a **strong recommendation the user can
  explicitly override**, not an absolute hard-block. Override defers, never
  waives.
- **R2 (new `review.gate` MCP tool) DROPPED** — under full ship/review
  decoupling the gate reduces to existing `review.marker` + one `git rev-list`,
  so no new MCP surface is justified.
- **`release-tag-glob` REMOVED** — the gate no longer keys its range to a
  release tag; the marker (frontier) is the sole anchor. This supersedes
  260829's tag re-key and its no-tag fallback (former R5-sub).

The paused plan `ai-docs/.plans/2026-08/30-1751-260824-review-release-gate-policy-p2.md`
is superseded by this redesign (see its appended supersession note). Phase 2 is
re-scoped below.

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
- **Ship and review are fully decoupled; the review marker is the sole gate
  anchor (2026-08-30, supersedes 260829's tag re-key).** `lead-ship` *triggers*
  `lead-review` but nothing about ship — tag, merge, or override — feeds back
  into the review range. The gate reviews **`<marker>..HEAD`**, where `marker`
  is the review frontier, not a release tag. Dependency flows one way:
  `ship → review → marker`, never the reverse.
- **Single-writer marker invariant.** The review marker advances **only** when
  `lead-review` completes and stamps (via `review.stamp`); `review.marker(bootstrap:
  true)` seeds the first entry. Nothing else — not `lead-ship`, not an override —
  ever advances it. (Enforced by convention + the frontier rule below, not by a
  new lock.)
- **Frontier = last *clearing* entry.** Marker resolution advances the frontier
  only past `pass`/`concern` entries; `block` and incomplete/absent reviews do
  **not** advance it. All five ledger tokens are classified explicitly (per sage
  design review, 2026-08-30): **`pass`/`concern` = clearing** (advance the
  frontier); **`block` = non-clearing** (holds it); **`routed` = non-clearing** —
  the corrective `<range>: routed -> <stem>` append (③) shares the blocked range's
  head SHA, so treating it as clearing would advance the frontier to an
  un-reviewed tip and silently defeat the forcing function; **`bootstrap` = the
  baseline floor** — it is a resolvable frontier base (so `<marker>..HEAD` always
  has a base once seeded) but represents "nothing reviewed yet," not a clearing
  review. Consequence: an unresolved block holds the frontier, so
  `<marker>..HEAD` keeps re-including that range until a review actually passes —
  the forcing function is the non-advancing marker itself, not a ledger/ticket
  cross-check. `CheckpointNudge` and the ship gate share this one resolver.
  (Relies on the already-required no-squash/rebase convention on the review
  track — `review-watermark-ledger.md` — so the frontier SHA stays live; this is
  why tag-robustness is no longer needed.)
- **`review.gate` MCP tool is NOT added (R2 dropped).** With the above, the
  ship-time check is `review.marker` (existing) + `git rev-list --count
  <marker>..HEAD`; a non-empty count means "not clear." No release-tag
  resolution, no `block`-entry enumeration, no ticket-status cross-check at ship —
  so no new MCP surface.
- **`release-tag-glob` config field is removed.** Phase 1 shipped it
  (`ac3b7356`); Phase 2 removes it from the `AGENTS.md` `### Review Policy`
  reader (`ReadAgentsReviewPolicy`) and from devenv's own policy, since the gate
  no longer reads release tags. `release-boundary` (present|absent) still governs
  whether the gate fires; `rendezvous-backend` still selects the no-squash/rebase
  enforcement path (`platform` branch-protection vs `canary` convention).
- **Override defers, never waives; durable recording is NOT required (2026-08-30).**
  When the gate is not clear, `lead-ship` surfaces a strong recommendation and
  stops for an **explicit** user decision; the user may override and proceed.
  "Never silent" is satisfied at **decision time** — the gate always runs, always
  surfaces the block, and only a conscious human act passes it. Because the
  override never stamps, the marker does not advance, so the un-reviewed range
  automatically rolls into the next `lead-review` (batched "몰아서" catch-up) —
  this non-advancing marker, not any log, is the "cannot drop on the floor"
  guarantee. A durable audit record is therefore **not part of the host-neutral
  mechanism**: the gate requires and defines no audit home. Rationale for
  dropping it: recording adds no correctness (eventual review is already
  guaranteed), a "ship promotion commit" home is not host-neutral (a downstream
  with no release-branch topology has no such commit), and where accountability
  matters most it already exists elsewhere (`platform` backend → GitHub logs the
  required-check bypass; `canary` → single-maintainer-serial). A boundary project
  MAY optionally record overrides in a project-local home that fits its topology,
  but the mechanism neither requires nor specifies one. (Earlier drafts recorded
  to a "ship log"; that targeted a drop-on-the-floor risk the decoupled model now
  resolves structurally.) Still strictly stronger than the rejected config-bullet
  alternative (defeatable by silent omission): the gate is un-omittable and always
  surfaces.
- **Host-neutral first:** never encode devenv's `develop`/`main`/ship shape as
  the mechanism. Gating is opt-in; absence of a declared boundary means
  advisory-only, not "no review."
- **workflow_manual discovery:** when the review-track branch is unset,
  `workflow_manual` surfaces a **non-blocking** "configure this first" nudge at
  most **once per session** (session-scoped, not per-checkpoint), not a hard
  block.
- **Empty/no-marker ledger (revised 2026-08-30, supersedes the tag-model
  "Fallback").** When `review.marker` reports no entry (a boundary project whose
  ledger was never bootstrapped — the common first-ship state), the gate treats
  all prior history as review-skipped: **not clear**, so it **stops for an
  explicit decision** rather than passing silently. It never auto-bootstraps and
  proceeds (that would silently pass all prior history). The human's explicit
  choices are: **(i)** consciously bootstrap the baseline at HEAD — an explicit
  accept of prior history as unreviewed, seeding the marker so future gates work
  (equivalent to an override, not a review); or **(ii)** run a review over an
  explicitly chosen base (repo root or a named commit), which stamps and advances
  the marker. Note bootstrap and "review the shipped range" do **not** compose:
  bootstrap seeds `<HEAD>..<HEAD>` (empty range, nothing reviewed), whereas a
  review needs a non-HEAD base the empty ledger cannot supply — hence the base is
  a human input on first ship, never a tag-keyed or `main..develop` fallback (the
  marker is the sole anchor). (The former tag-model bullet is deleted.)
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
  **Revised (2026-08-30): the gate no longer cross-checks routed-ticket status.**
  Under the frontier rule above, a `block` does not advance the marker, so the
  blocked range keeps re-surfacing in `<marker>..HEAD` until a review passes —
  the non-advancing marker *is* the "cannot drop on the floor" guarantee, and
  clearing happens by a genuine passing review, not by a routed ticket reaching
  terminal. The routed ticket (③'s `block`-requires-`ref` invariant) is retained
  as fix-tracking bookkeeping, not as the gate-clearing mechanism. Whether to
  relax `block`-requires-`ref` now that gate-clearing no longer depends on it is
  an open implementation question for Phase 2 / sage review, not settled here.
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

### Phase 2: Mandatory release gate (devenv ship) — decoupled marker model

Re-scoped 2026-08-30 (see `## Sign-off` and revised `## Decisions`). Four
deliverables; no new MCP tool.

1. **Marker-resolution refactor (`wsreview`, internal — no MCP surface).**
   Resolve the review frontier as the last *clearing* (`pass`/`concern`) entry,
   so `block`/incomplete reviews do not advance it. Expose it through one shared
   internal resolver consumed by both `CheckpointNudge` and the ship gate. This
   changes what "the marker" means for `review.marker`'s returned entry — verify
   `CheckpointNudge`'s existing tests still hold and add frontier-vs-block-entry
   coverage.
2. **`lead-ship` gate branch (R5, playbook — un-omittable, overridable).** In
   the shared `agents-plugin/rsrc/lead-ship/lead-ship.md`, add a generic branch:
   *if `release-boundary: present`*, before `develop`→`main`, read the frontier
   **head SHA** from `review.marker` and compute `git rev-list --count
   <frontier-head>..HEAD`. Empty → proceed. Non-empty → trigger `lead-review`
   over `<frontier-head>..HEAD`; if still not clear (review blocking, or user
   declines to complete it), surface a **strong recommendation** and stop for an
   **explicit** user decision. On override: proceed — the marker stays put,
   deferring the range to the next review. No durable audit record is required or
   defined by the mechanism (see the "Override defers" decision); a boundary
   project may optionally record overrides in a project-local home, but Phase 2
   adds none. Amend the "ship config is the single source of truth" Invariant to
   carve out this un-omittable gate. Regen the wsflow rsrc mirror.
   - **Marker read must not string-scrape (sage design review).** To keep the
     doc-map's "agent never hand-parses the ledger" honest, obtain a bare/
     structured head from `review.marker` (add a bare-SHA or `--format json`
     accessor to the existing tool if it only returns a formatted line — a small
     output addition to an existing tool, not a new tool), and have `review.marker`
     return the **frontier** (last clearing entry, deliverable 1), not the raw
     latest entry.
3. **Drop `release-tag-glob`.** Remove the field from `ReadAgentsReviewPolicy`
   (`wsreview/agents_config.go`) and from devenv's `AGENTS.md ### Review Policy`;
   remove the `git describe --match <glob>` machinery entirely (it was never
   consumed — Phase 1 only parsed the field). Keep `release-boundary` and
   `rendezvous-backend`.
4. **devenv concrete wiring (`ai-docs/ship/ws.md`).** Place the gate in ship
   pre-flight ordering. Single-maintainer-serial + `canary`, so the pin-and-
   re-assert race below holds trivially, but wire it so the host-neutral branch
   is exercised.

- **Promotion atomicity (pin-and-re-assert, R4 — retained).** Record the
  reviewed through-SHA; immediately before `git merge --ff-only develop`,
  re-assert `git rev-parse develop` still equals it. A moved tip aborts and
  re-runs the gate over the delta before promoting (the ③ race path). Named
  rather than assumed so the host-neutral generalization is safe.
- Downstream without a declared boundary: no gate inserted; advisory-only.
- Depends on Phase 1, ②, and ③.

**Doc-map (where the agent-facing guidance attaches):**
- `lead-review.md` (step 7, already the sole marker writer): add the
  single-writer invariant + which verdicts advance the frontier. Extends
  existing text.
- `lead-ship.md`: the new gate branch + the negative invariant "ship never
  advances the marker."
- `ai-docs/ship/ws.md`: concrete pre-flight placement (no tag content).
- **NOT** `lead-workflow-manual` — marker behavior is skill-specific
  (lead-review/lead-ship), not a general primitive; marker *interpretation* is
  in code, so the agent never hand-parses the ledger.
- spec `mcp-tools.md`: frontier resolution contract (`review.marker` returns the
  frontier, not the raw latest entry) + drop tag-glob refs. Also reconcile the
  nudge anchor `#260830-review-watermark-checkpoint-nudge` — it still says
  `CheckpointNudge` reads "the ledger's latest entry (the marker)"; it now counts
  from the last clearing entry, so a tail `block` changes the nudge origin.
  `workflow-skills.md`: lead-ship gate + lead-review sole-writer; clean the
  Phase-1 entry of `review.gate`/tag-glob traces.
- mental-model `review-watermark-ledger.md`: frontier=clearing-entry + the
  single-writer/decoupling rationale.

Verification: on devenv, ship pre-flight stops for an explicit decision when
`<marker>..HEAD` is non-empty and the triggered review does not clear it; it
proceeds silently when the range is empty; an override proceeds and leaves the
marker unadvanced (next `lead-review` re-includes the deferred range), with no
audit record required by the mechanism; the reviewed through-SHA is re-asserted at ff-merge time (a moved
tip forces re-review); a no-boundary project's ship path is unchanged; the
frontier resolver does not advance past a `block` entry.

### Result (80e5dc1d) - 2026-08-30

Implemented on `impl/goal/develop/copper-lantern-drizzle/flock-calm-speed`,
range `9c36958a..80e5dc1d` (survey plan `30-2239-...-p2.md`). All four
deliverables + R4 + the doc-map landed.

- **Frontier resolver** (`wsreview.ParseFrontier`/`Frontier`, new read-side
  siblings; `ParseLatest`/`Read`/`Bootstrap` untouched): resolves the last
  *clearing* entry, skipping `block`/`routed` via an explicit allow-list,
  with `bootstrap` as the floor and `found=false` on an empty/all-block
  ledger. `CheckpointNudge` switched `Read`→`Frontier`, so a trailing `block`
  now shifts the nudge origin to the last clearing entry.
- **`review.marker` output** gained a `format: json` mode (reusing the
  `tickets.status` `wantsJSON`/`toolJSONResponse` pattern — output addition to
  the existing tool, no new MCP tool) reporting the frontier `Entry`
  (`base`/`head`/`verdict`/`ref`/`found`) from both handler branches.
- **`lead-ship` gate** (own `### 2. Release gate` numbered section, renumbering
  Execute to `### 3.`; wsflow mirror byte-identical): un-omittable via an
  amended single-source-of-truth Invariant carve-out, user-overridable. Reads
  `found` first (guarding the empty-`head`→`rev-list`→silent-`0` footgun), then
  branches: `found=false` → stop for an explicit decision with the two
  non-composing options (i bootstrap-accept / ii review-from-chosen-base);
  `found=true` → `rev-list --count <frontier-head>..HEAD` → empty proceeds,
  non-empty triggers `lead-review`, still-not-clear stops for override. The
  gate never calls `review.stamp` — `lead-review` step 7 is the sole marker
  writer (invariant now stated in both skills + the mental model).
- **`release-tag-glob` dropped** entirely from `ReadAgentsReviewPolicy` and
  devenv's `AGENTS.md`; grep-confirmed zero remaining references (the
  `git describe --match` machinery never existed — only a doc comment).
- **devenv wiring** (`ai-docs/ship/ws.md`): gate in Pre-flight; R4
  pin-and-re-assert in Publish immediately before the ff-only merge.

Deviations (both docs-only, accepted): (1) the gate is its own numbered
section rather than a Pre-flight bullet — required by R5 un-omittability
(a bullet is defeatable by config omission); fit review confirmed this is the
file's own established idiom. (2) the R4 through-SHA is pinned at the *end* of
Pre-flight (after the version-bump commit) rather than at gate time, so the
ship's own mechanical bump does not trip R4; correctness review confirmed no
coverage gap (an external `origin` arrival is caught by the existing
"develop up to date" check, which restarts Pre-flight and re-runs the gate).

Verification: `go build`/`go vet` clean; `go test ./...` all packages ok; new
frontier/`format:json`/nudge-origin/all-block-no-floor tests added and passing;
wsflow rsrc mirror + manifests regenerated (`-count=1`), drift guards + wsflow
Python suite green; `spec_index_verify` clean. Review: 1 critical (empty-ledger
silent-pass, ticket-binding-decision violation, reproducible on devenv's own
first ship) + 2 minors, all resolved across two relay cycles.

Deferred (not blockers): the interactive ship dry-run (live skill invocation,
exercised naturally at the next real ship); and the ledger
`block`-requires-`ref` invariant relaxation, explicitly left open by this phase.

## Spec Impact

Targets:
- `ai-docs/spec/workflow-skills.md` — `lead-ship` gains an un-omittable,
  user-overridable pre-flight review gate for `release-boundary: present`
  projects, keyed to `<marker>..HEAD`; `lead-review` documented as the sole
  marker advancer. Clean the Phase-1 entry (`#260830-review-policy-config-surface`)
  of `review.gate`/`release-tag-glob` traces.
- `ai-docs/spec/mcp-tools.md` — frontier resolution contract (`review.marker`
  returns the last *clearing* entry) **and its new bare-SHA/`--format json` head
  output** (a caller-visible output addition, not a new tool); drop
  `release-tag-glob` references.
- `ai-docs/ship/ws.md` — devenv pre-flight gate placement.

New caller-visible behavior: a mandatory-but-overridable review gate at the
declared release boundary (devenv ship), host-neutral advisory-only elsewhere;
override defers (marker unadvanced; no audit record required by the mechanism).
Removed caller-visible surface: `release-tag-glob` config field (shipped in Phase
1, removed here); no `review.gate` MCP tool is added.
