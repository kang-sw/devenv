# Plan: 260824-feat-review-release-gate-policy — Phase 2: Mandatory release gate (devenv ship)

> Research pass. Refines the prior survey output at this path: keeps its
> verified Codebase Findings, resolves the six open design decisions the survey
> escalated, and replaces the placeholder Implementation Plan / Escalations with
> a concrete, executor-ready mechanism. **Six recommendations below are marked
> `[AWAITING LEAD]`; two of them (R2 new MCP tool, R5 canonical lead-ship flow
> change) are new public surface and flagged for user sign-off. Do not begin
> execution until the lead ratifies.**

## Relevant Ticket Contract

- Insert a **mandatory** range review into the promotion path for a project
  declaring a release boundary; for devenv, into `lead-ship` pre-flight
  (`ai-docs/ship/ws.md` / `agents-plugin/rsrc/lead-ship/lead-ship.md`), which
  today has no review step. Gate range is `<last-release-tag>..HEAD`
  (squash-robust, `260829`), falling back to `main..develop` when no release
  tag exists yet.
- **Gate verdict** blocks when ANY of: (a) the just-run range review raises a
  blocking finding; (b) a `block` ledger entry since the last release
  references a routed ticket still open (idea/todo/ready/wip); (c) a `block`
  entry since the last release has no routed ticket at all (un-routed →
  un-clearable forcing function). Clears only when every such finding's routed
  ticket is terminal (`.done` or `.dropped`). No ledger "resolved" flag, no
  separate waiver record — cross-reference block entries against routed-ticket
  status directly.
- **Promotion atomicity (pin-and-re-assert):** record the reviewed
  through-SHA; assert the review-track tip still equals it at ff-merge time; a
  moved tip forces re-absorb + re-review of the delta before promoting.
- Host-neutral first: never encode devenv's develop/main/ship shape into the
  mechanism itself; a project without a declared boundary gets no gate
  (advisory-only, unchanged).
- Depends on Phase 1 (done, `ac3b7356`), ② `260824-feat-lead-review-range-scenario`
  (done), ③ `260824-feat-review-watermark-ledger` (done).

## Out of Scope

- Phase 1's config surface — landed/merged; do not re-touch
  `wsreview/agents_config.go`, `wsreview/track.go`, or the `workflow_manual`
  nudge wiring (except to *read* `ReadAgentsReviewPolicy`, which is reuse).
- Platform-backend (GitHub branch-protection) rendezvous mechanics — devenv is
  `rendezvous-backend: canary`; the `platform` hardening is out of this ticket.
- `AGENTS.template.md` bootstrap-template propagation — deferred (Phase 1 Result).
- Any change to the `lead-review` range scenario or landing lens themselves (②
  is done; Phase 2 only *calls* the range scenario).
- Any change to ledger `Append`/`Read`/`ParseLatest` semantics or the entry
  format — this phase adds only new **read-only** enumeration; no new verdict
  tokens, no format change.
- The advisory mid-stream `CheckpointNudge` and its marker semantics — the gate
  is tag-keyed and independent of the demoted precise marker (`260829`).

## Codebase Findings

Verified against source this pass. The survey's 11 findings hold; the
load-bearing ones, re-confirmed:

- **`wsreview/ledger.go`** — `ParseLatest` (L90) is the *only* read function;
  returns just the last matching entry. `entryLineRE` (L77) is the single
  source of truth for entry parsing (`<base>..<head>: <verdict>[ -> <ref>]`,
  banner/comment/blank lines skipped). Verdict tokens: `pass|concern|block|
  routed|bootstrap`. **`Append` (L155) already requires a non-empty `Ref` for a
  `block` entry** — so gate condition (c) ("block with no routed ticket") can
  arise only from a *legacy/malformed* pre-validation entry; the current path
  is a legacy `block` (no ref) later corrected by a `routed` append
  (`<same-range>: routed -> <stem>`, per ③). Entries carry **no timestamp** —
  "since the last release" is a git-ancestry question, not a field read.
- **No enumeration helper exists** (grep-confirmed): only `ParseLatest`. A gate
  inspecting *several* block entries since a tag is genuinely new read surface.
- **No `git describe` wrapper exists** in `wsgit` (grep-confirmed; the only
  `describe` mention is the doc comment on `ReleaseTagGlob`). `git describe
  --tags --match 'v*' --abbrev=0` runs correctly in this repo today (returns
  `v0.43.7`).
- **`wsreview/agents_config.go` `ReadAgentsReviewPolicy(root)`** (L80) is
  fail-open and returns `ReviewTrack`, `ReleaseBoundary`
  (`present`/`absent`), `RendezvousBackend`, `ReleaseTagGlob` (default `v*`).
  This is the reusable policy input for the gate — the gate reads
  `ReleaseBoundary` (to honor host-neutral opt-in) and `ReleaseTagGlob` (to
  resolve the tag). Only consumer today is the once-per-session nudge; the gate
  is its second consumer.
- **`wsgit/git.go` `MergeBase` / `ExecRunner.RunGit`** — the ancestry/plumbing
  primitives already exist. `MergeBase` (L408) wraps `git merge-base`;
  `ExecRunner.RunGit` (L26) is the raw `git -C <root> …` escape hatch used by
  `checkpoint.go` for `rev-list --count`. `git merge-base --is-ancestor` has no
  typed wrapper but is a one-line `RunGit` call (exit-status read), the same
  shape `ai-docs/ship/ws.md` pre-flight already uses in prose.
- **`wsdoc.TicketsStatus`** (MCP `tickets.status`, `server.go:1190`) resolves a
  stem to its directory status (idea/todo/ready/wip/`.done`/`.dropped`) with
  `Resolve: true` (reports hidden-but-found). Directly reusable, unmodified,
  for the per-entry terminal-status cross-reference. Terminal = `.done` |
  `.dropped`; everything else = open/blocking.
- **`server.go` review tools (L1253–1288)** — `review.marker` (read/bootstrap
  latest) and `review.stamp` (append) are the only two review MCP tools; both
  single-entry, root-aware (`resolveToolRoot` + `session_key`), **not**
  lead-only (not in `isLeadOnlyTool`). Any new gate tool should follow this
  same root-aware, non-lead-only shape.
- **`agents-plugin/rsrc/lead-ship/lead-ship.md`** (`kind: print`) — Invariants
  state "The ship config is the single source of truth; do not improvise steps
  not listed there." `### 2. Execute` step 1 is "Pre-flight — run any listed
  checks." The playbook is config-driven and today knows nothing about
  review-track/boundary policy. `lead-ship` is a shipped wsflow skill (thin
  `playbook.print` shim over this same rsrc — `wsflow-mirroring.md:48`); editing
  this rsrc requires the standard wsflow rsrc-mirror regen, **not** a curated
  divergence.
- **`ai-docs/ship/ws.md`** — devenv ship config. Pre-flight (L20–49) runs raw
  bash checks in prose (`git fetch`, `merge-base --is-ancestor`, version bump,
  tag-uniqueness, `go test`, marketplace sanity) — no review step; the insertion
  point. Note: its version logic reads `.plugin_version`/`.release_tag` from
  `runtime.json`, and **`.release_tag` is the *upcoming* tag being shipped (the
  just-bumped `v<version>`), not the last released tag** — so it answers a
  different question than the gate's "last release tag" and must not be reused
  as the gate base (see R3). Publish section (L64–76) is where the
  pin-and-re-assert atomicity lives, immediately before `git merge --ff-only
  develop`.
- **`mcp-runtime.md:90`** — "Add an MCP tool" recipe: schema in `tools()`,
  dispatch in `callTool`, keyed-capability/visibility gates, matching visibility
  tests, `runtime.json`. Coupling note (L80): `runtime.capabilities` derives
  names from `tools()` but `runtime.json` must be updated separately.
- **`runtime.json` (both packages)** — `review.marker`/`review.stamp`,
  `tickets.status`, `tickets.close` are already registered in **both**
  `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`. A new
  gate tool composing these primitives has no agent dependency of its own, so
  registering it in both is mechanically clean (see R6).
- **`mental-model/review-watermark-ledger.md`** — the governing invariant is
  **fail-safe over-review, never under-review**: a squash/rebase-orphaned entry
  Head simply stops being an ancestor of anything current, and "the next
  sweep's range naturally re-covers it." The enumeration algorithm (R1) must
  preserve this direction — an entry whose ancestry can't be proven contained in
  the release must be *included* (over-review), never silently excluded.

## Implementation Plan

The gate splits cleanly into a **Go/MCP piece** (deterministic ledger + tag +
ticket-status cross-reference — condition b/c) and a **playbook piece** (the
condition-(a) fresh range review, which is inherently agent orchestration, plus
the pin-and-re-assert git plumbing). Recommendations R1–R6 resolve each open
decision; each names its evidence and is marked `[AWAITING LEAD]`.

### R1 — "Entries since last release" selection: git-ancestry, over-review-biased `[AWAITING LEAD]`

**Recommendation:** per-entry git-ancestry, not a positional rule. An entry is
**excluded** from the gate set only when its `Head` is a *proven* ancestor of
the last release tag (`git merge-base --is-ancestor <entry.Head> <tag>` exits 0
→ already in the release → skip). Every other block entry is **included**,
including entries whose `Head` is orphaned/unresolvable (the `--is-ancestor`
check errors or exits non-zero → include).

- **Evidence:** the ledger has no timestamp (`ledger.go:69-77`), so ordering
  cannot substitute for ancestry; a positional "tail from the entry bracketing
  the tag" rule silently breaks under squash/rebase reordering. The ancestry
  rule is the only one that satisfies the mental-model's fail-safe
  over-review invariant: an orphaned Head fails the ancestor test and is
  included → re-reviewed, exactly matching "the next sweep re-covers it."
- **Routed-stem resolution within the set:** for each included `block` entry,
  its stem is either the entry's own `Ref` (current path — `Append` enforces
  it) or, for a legacy un-routed `block`, a later `routed` entry over the *same
  `<base>..<head>` range* supplying the stem. No stem from either → condition
  (c), un-clearable → BLOCK. Stem present → cross-reference via
  `tickets.status`; terminal (`.done`/`.dropped`) → cleared; open → condition
  (b) → BLOCK.
- **No-tag case:** when `git describe` finds no release tag (fresh project,
  no release yet), there is no release boundary to bound "since" against — so
  **all** block entries in the ledger are in-scope (maximally over-reviewing,
  consistent). The condition-(a) review range in that case is the config
  fallback (R3).

### R2 — Go/MCP vs skill-text split: a new Go-backed MCP tool for b/c `[AWAITING LEAD — NEW MCP TOOL, USER SIGN-OFF]`

**Recommendation:** put the deterministic enumeration + tag resolution +
ticket cross-reference behind **one new Go-backed MCP tool**; keep condition
(a)'s fresh range review and the pin-and-re-assert in playbook prose (they are
irreducibly agent/plumbing orchestration).

- **Evidence:** the b/c logic re-implements `entryLineRE` parsing, ancestry
  looping, routed-stem pairing, and status cross-reference. Doing that in
  `lead-ship` bash prose would fork the ledger grammar into fragile shell —
  precisely the failure the survey flagged, and against precedent: Phase 1
  chose a dedicated Go parser (`agents_config.go`) for a *much simpler* 4-field
  format. Keeping ledger parsing in `wsreview` preserves the single-source
  `entryLineRE` contract and reuses `wsdoc.TicketsStatus` unchanged.
- **Proposed tool — `review.gate` (name follows `review.marker`/`review.stamp`):**
  - **Shape:** root-aware (`resolveToolRoot` + required `session_key`, `root`
    stripped from schema), **not** lead-only — mirrors the two existing
    `review.*` tools.
  - **Input:** `session_key` (required). No other required args; it reads
    everything from `root` (AGENTS.md policy + ledger + git).
  - **Behavior:** (1) read `ReadAgentsReviewPolicy(root)`; if
    `ReleaseBoundary == absent`, return an explicit **not-applicable** result
    (host-neutral opt-out centralized in Go, so the mechanism carries no
    devenv shape). (2) Resolve the last release tag via `git describe --tags
    --match <ReleaseTagGlob> --abbrev=0`. (3) Compute the gate range base
    (`<tag>` or a no-tag signal). (4) Enumerate since-release `block` entries
    per R1, resolve routed stems, cross-reference each via ticket status. (5)
    Return: the resolved gate **base** + through-**Head** (for the playbook to
    feed `<base>..HEAD` to `lead-review` and to pin for R4), a **blocking |
    clear** verdict on the b/c conditions, and per-entry **reasons** (range,
    stem, ticket status, or "un-routed").
  - **Output:** compact LLM-readable text (per `mcp-runtime.md` domain rule),
    JSON on `--format json` for machine parsing, following the `review.*`
    formatter precedent.
  - **Note the tool does NOT run the range review** — condition (a) is the
    playbook's `lead-review` invocation. The tool answers "is the gate range,
    and are b/c satisfied"; the playbook composes (a) + the tool's b/c verdict.
- **Go placement:** a new `wsreview` enumeration function (e.g.
  `BlockEntriesSince(content, isAncestor func(head string) bool) []Entry` — the
  ancestry predicate injected so `wsreview` stays git-runner-free at the parse
  layer, mirroring how `entryLineRE` parsing is pure and `Bootstrap` takes the
  runner explicitly) + a gate-resolution function that wires the git predicate
  (`ExecRunner.RunGit` `merge-base --is-ancestor` / `describe`) and the
  ticket-status cross-reference. Follow `Add an MCP tool` recipe
  (`mcp-runtime.md:90`): schema in `tools()`, dispatch in `callTool`,
  `runtime.json` in both packages (R6), matching `internal/mcp` + `wsreview`
  tests.
- **This is new public API surface → flagged for user sign-off** (see Escalations).

### R3 — Release-tag resolution: `git describe`, reconciled with `.release_tag` `[AWAITING LEAD]`

**Recommendation:** resolve the last release tag with `git describe --tags
--match <ReleaseTagGlob> --abbrev=0` (consuming Phase 1's `ReleaseTagGlob`).
This does **not** conflict with `ai-docs/ship/ws.md`'s `.release_tag` mechanism.

- **Evidence:** `.release_tag`/`runtime.json` holds the *upcoming* tag (version
  already bumped in Pre-flight before the gate would even run) — it is the tag
  *about to be created*, not the previous release boundary. `git describe
  --abbrev=0` returns the most recent existing matching tag reachable from HEAD
  (verified: `v0.43.7`), which is exactly the gate's base. They answer
  different questions and coexist; the plan must state this so an executor
  doesn't "reconcile" by reusing `.release_tag` (which would move the gate base
  to the wrong commit).
- **Ordering caveat for the executor:** the gate must resolve `git describe`
  **before** the Pre-flight version-bump commit if the bump ever tagged inline
  — it does not today (the tag is pushed only in Publish), so `git describe` on
  develop pre-push correctly returns the *previous* release. Keep the gate
  early in Pre-flight regardless, so the reviewed range is stable.

### R4 — Promotion atomicity (pin-and-re-assert): pure playbook prose `[AWAITING LEAD]`

**Recommendation:** pure prose in `ai-docs/ship/ws.md`'s Publish section (and,
generically, the lead-ship playbook branch of R5). No Go support.

- **Evidence:** the pin is `git rev-parse develop` at gate time (or reuse the
  gate tool's returned through-Head), re-asserted with a second `git rev-parse
  develop` immediately before `git merge --ff-only develop`; a mismatch aborts
  and re-runs the gate over the delta. This is the identical idiom the
  Pre-flight section already uses (`git merge-base --is-ancestor` in prose).
  For devenv's serial local ship it holds trivially; naming it keeps the
  host-neutral generalization safe. Adding Go for a two-line
  rev-parse-and-compare would be over-engineering with no reuse payoff.

### R5 — Host-neutral placement: generic branch in shared `lead-ship` playbook `[AWAITING LEAD — CANONICAL FLOW CHANGE, USER SIGN-OFF]`

**Recommendation:** the *mandatory* gate lives as a **generic branch in the
shared `lead-ship.md` playbook** ("if `release-boundary: present`, run the
release gate before promotion"), with devenv's `ai-docs/ship/ws.md` supplying
only the concrete no-tag fallback range and Pre-flight ordering.

- **Evidence / rationale:** the ticket's whole ethos is a **forcing function**
  ("mandatory," "un-clearable," "cannot be dropped on the floor"). If the gate
  were merely a Pre-flight bullet in each project's ship config, a boundary
  project could silently defeat it by omitting the bullet — the exact
  drop-on-the-floor failure the ticket forbids. A playbook-level branch keyed
  off `ReadAgentsReviewPolicy` makes the gate un-omittable for every boundary
  project and carries no devenv shape (host-neutral-first).
- **Cost this incurs — the flag:** it **contradicts the current lead-ship
  Invariant** "the ship config is the single source of truth; do not improvise
  steps not listed there." Adding a mandatory playbook-level step is an
  observable change to the canonical lead-ship flow and to that invariant's
  wording. → **user sign-off** (see Escalations). The alternative (config-only
  bullet, R5-B) is simpler and needs no playbook/invariant change but is
  defeatable by omission — record it as the rejected shortcut.
- **The no-tag fallback (`main..develop`) is devenv-shaped and stays in the
  config, not the mechanism:** the gate tool returns a "no-release-tag" signal;
  `ai-docs/ship/ws.md` supplies `main..develop` as the condition-(a) review
  range for that case. This resolves the host-neutral tension: the mechanism
  never learns "main"/"develop." (If the lead prefers zero project-specific
  fallback, the host-neutral alternative is to fall back to the ledger marker's
  Head, or the review-track merge-base with HEAD — flagged as a sub-decision.)

### R6 — wsflow parity `[AWAITING LEAD]`

**Recommendation:** register the new `review.gate` tool in **both**
`agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` (per
`mcp-runtime.md` "Change wsflow no-agent mode" recipe). The b/c Go tool has no
agent dependency and its composed primitives (`review.marker`/`review.stamp`/
`tickets.status`/`tickets.close`) are already in both manifests, so wsflow's
ship flow can enforce the ledger forcing-function identically.

- **Wrinkle to flag:** condition **(a)** — the fresh range review — runs
  through `lead-review`'s range scenario, which is agent-based. In wsflow
  no-agent mode there are no subagents, so full condition-(a) parity is *not*
  automatic: wsflow either runs the range review inline (non-delegated) or the
  wsflow gate enforces only b/c (the ledger forcing-function) while (a) degrades
  to the single-reviewer floor. This is an architecture question for the lead,
  not something research should settle. The b/c Go tool parity is unambiguous
  (register it); the (a) parity in no-agent mode is the open sub-decision.

### Executor sequencing (once ratified)

1. `wsreview`: add pure `BlockEntriesSince(content, isAncestorPredicate)`
   enumeration + a gate-resolution function wiring `git describe`,
   `merge-base --is-ancestor`, and the ticket-status cross-reference. Unit
   tests in `internal/wsreview` (ancestry inclusion/exclusion, orphan →
   include, routed-stem pairing, terminal vs open clearing, no-tag → all).
2. `internal/mcp`: add `review.gate` schema in `tools()`, dispatch in
   `callTool` (root-aware, non-lead-only), text + JSON formatters, visibility
   unchanged (not agent-backed). `internal/mcp` integration tests.
3. `runtime.json` in both packages; wsflow package tests.
4. `agents-plugin/rsrc/lead-ship/lead-ship.md`: generic boundary-gate branch
   (R5); regen the wsflow rsrc mirror (`WS_REGEN_WSFLOW_RSRC=1` after
   `WSRSRC_REGEN=1` — per the survey's mirror note). Amend the "single source of
   truth" Invariant to carve out the mandatory gate.
5. `ai-docs/ship/ws.md`: Pre-flight gate ordering + `main..develop` no-tag
   fallback range; Publish-section pin-and-re-assert (R4).
6. Spec + mental-model doc pass through the doc pipeline (not implementer):
   extend the `lead-ship` spec paragraph (`workflow-skills.md:1339`) and the
   `release-boundary` line (`:1210`, "a subsequent release-gate capability keys
   its mandatory review off" → now realized), add a `review.gate` contract
   anchor, and update `review-watermark-ledger.md` with the enumeration/ancestry
   modification guideline.

## Verification Plan

- **Go unit (`internal/wsreview`, post-impl):** ancestry selection —
  contained-Head excluded, post-tag Head included, orphaned Head included
  (over-review); routed-stem pairing (own `Ref`; legacy un-routed + later
  `routed`); clearing — `.done`/`.dropped` clears, open blocks, no-stem blocks
  (condition c); no-tag → all block entries in scope; `release-boundary: absent`
  → not-applicable.
- **MCP integration (`internal/mcp`, post-impl):** `review.gate` returns
  blocking/clear verdict + reasons over a fixture repo (fixture ledger + fixture
  ticket tree); root-aware `mandatory_session_key`/`unknown_session` behavior;
  raw `tools()` schema strips `root`; JSON + text formatter coverage.
- **Manual dry-run (devenv):** ship Pre-flight refuses to promote when the range
  review raises a blocking finding, when a ledger `block` since the last release
  points at an open routed ticket, or when such an entry has no routed ticket;
  proceeds when the range is clean and every finding's routed ticket is terminal;
  the reviewed through-SHA is re-asserted before `git merge --ff-only` (a moved
  tip forces re-review); a no-boundary project's ship path is unchanged
  (exercise by temporarily reading `release-boundary: absent`).
- **wsflow:** `runtime.json` contract test + skill-shim drift test green after
  the mirror regen.

## Escalations

Research resolved the mechanism; the following require **lead ratification**
before execution, and the two marked **USER SIGN-OFF** are new public surface /
canonical-flow changes the lead must escalate to the user:

1. **R2 — new `review.gate` MCP tool. [USER SIGN-OFF]** New public MCP API
   surface. Confidence: high that Go/MCP is the right split (Phase 1 precedent +
   single-source `entryLineRE` + `mcp-runtime.md` recipe); the tool *name*,
   *schema granularity* (one composite gate call, as recommended, vs. separate
   enumerate/resolve-tag primitives the playbook composes), and whether it is
   lead-only are the ratifiable knobs.
2. **R5 — mandatory gate as a generic `lead-ship` playbook branch. [USER
   SIGN-OFF]** Observable canonical lead-ship flow change; contradicts and must
   amend the "ship config is the single source of truth" Invariant. Rejected
   alternative (config-only Pre-flight bullet) is defeatable by omission — the
   lead must decide whether un-omittability is worth the invariant change.
3. **R5 sub-decision — host-neutral no-tag fallback.** Recommended: keep
   `main..develop` in the devenv *config* (mechanism stays shape-free).
   Alternative: a host-neutral marker-Head / review-track-merge-base fallback in
   the mechanism. Lead picks.
4. **R6 sub-decision — wsflow condition-(a) parity.** The b/c Go tool registers
   cleanly in wsflow; the *fresh range review* (a) has no subagent path in
   no-agent mode. Lead decides: inline review, or wsflow gate enforces b/c only
   with (a) degraded to the single-reviewer floor.
5. **R1, R3, R4** — resolved with high confidence and no new public surface
   (ancestry-based enumeration; `git describe` vs `.release_tag` reconciled as
   different questions; pin-and-re-assert as prose). Listed for the lead's
   awareness; no sign-off needed, but R1's over-review bias and R3's
   "don't reuse `.release_tag` as the gate base" are load-bearing correctness
   constraints the executor must not invert.

Confidence overall: high on mechanism; the residual risk is entirely in the two
sign-off items (public surface + flow change), which are policy calls, not
codebase unknowns.

## Lead Adjudications (30-1751)

Research is sound and the mechanism is executor-ready **modulo two user
sign-offs**. Goal-run posture: I resolve the lead-ratifiable items now; I do
**not** self-authorize the two items that expand public API / change a canonical
flow — those are AGENTS.md "Always ask" and the goal run's user-away autonomy
does not reach them. Phase 2 is therefore **paused pending user sign-off on R2
and R5**; the ticket carries a `## Blocked` note so the drain selector skips it
until the user decides. The independent ready ticket ⑤ `260828` continues.

Ratified now (no user needed):

- **R1 — ACCEPTED.** Git-ancestry, over-review-biased enumeration. An entry is
  excluded only when its Head is a *proven* ancestor of the last release tag;
  orphaned/unresolvable Heads are INCLUDED. This is the only rule consistent
  with the ledger mental-model's fail-safe-over-review invariant. Load-bearing —
  the executor must not invert the orphan→include bias.
- **R3 — ACCEPTED.** Last release tag via `git describe --tags --match
  <ReleaseTagGlob> --abbrev=0`. Do **NOT** reuse `ai-docs/ship/ws.md`'s
  `.release_tag`/`runtime.json` as the gate base — that is the *upcoming*
  just-bumped tag, a different question. Load-bearing correctness constraint.
- **R4 — ACCEPTED.** Pin-and-re-assert as pure playbook prose (`git rev-parse
  develop` at gate time, re-assert before `git merge --ff-only`); no Go.

Recommended to the user (SIGN-OFF REQUIRED — not executed until answered):

- **R2 (new `review.gate` MCP tool) — lead recommends APPROVE.** The Go/MCP
  split is right: bash-parsing the ledger grammar in `lead-ship` prose would
  fork the single-source `entryLineRE` contract into fragile shell, against the
  Phase 1 precedent (a dedicated Go parser for a *simpler* format). One
  composite `review.gate` call (root-aware, non-lead-only, mirroring
  `review.marker`/`review.stamp`) reporting gate base+Head + a b/c
  block|clear verdict + per-entry reasons. It is nonetheless **new public MCP
  API surface** → user decides the name, schema granularity (one composite call
  vs. separate enumerate/tag primitives), and lead-only-ness.
- **R5 (mandatory gate as a generic `lead-ship` playbook branch) — lead
  recommends APPROVE.** The ticket's forcing-function ethos ("mandatory,"
  "un-clearable," "cannot be dropped on the floor") is genuinely defeated by a
  config-only Pre-flight bullet, which any boundary project can silently omit. A
  playbook-level `if release-boundary: present` branch keyed off
  `ReadAgentsReviewPolicy` makes the gate un-omittable and host-neutral. Cost:
  it **amends the lead-ship "ship config is the single source of truth"
  Invariant** and is an observable canonical-flow change → user sign-off.
- **R5-sub (no-tag fallback) — lead recommends** keeping devenv's `main..develop`
  fallback in the *config* (`ai-docs/ship/ws.md`), so the mechanism stays
  shape-free. Contingent on R5.
- **R6-sub (wsflow condition-(a) parity) — lead recommends** the wsflow gate
  enforce **b/c only** (the deterministic ledger forcing-function), with
  condition (a)'s fresh range review degrading to wsflow's single-reviewer floor
  in no-agent mode — consistent with wsflow's agentless design rather than
  inventing an inline-review path. The b/c Go tool registers cleanly in both
  runtime.json (R6 accepted). Contingent on R2.

If the user rejects R2 and/or R5, the fallback direction is the recorded
rejected alternative (R2→playbook-prose orchestration of raw git/tickets.status;
R5→config-only Pre-flight bullet), which changes the executor sequencing but not
the ratified R1/R3/R4 correctness constraints.
