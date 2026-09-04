# Plan: 260824-feat-lead-review-range-scenario — Phase 2: Landing lens as a review-config required-check

## Relevant Ticket Contract

- Add the **landing lens** (convention adherence + spec/mental-model update
  completeness — were docs authored per each doc's function) to the review
  config (`_review.local.md` template) as a **required-check that runs within
  the review phases**.
- **Scope to the range/watermark scenario only** (own integrated work) — the
  branch/PR scenario must never run it, because an external contributor's PR
  must not be flagged for spec/mental-model updates they were never expected
  to author.
- It is captured as a **folded** required-check (lives inside the existing
  Review Phases execution, not a separate posture/pipeline); whether it later
  splits into its own posture is explicitly **deferred** — do not build that.
- Depends on Phase 1 (landed at `01fd2fe3`, mental-model fix `3571116b`): the
  scenario-kind dispatch (branch vs range), the scenario-scoped config-load
  Invariant, and the range-scenario built-in-defaults fallback already exist
  and must not be re-litigated — only extended.
- Verification boundary (from ticket): a range review over a diff that changed
  behavior without a matching spec/mental-model update surfaces a
  landing-lens finding; a convention-conformant, doc-complete diff passes it.
- Spec Impact (ticket-level, this phase's slice): `lead-review` gains "a
  landing-lens required-check in its review config," described in the
  `lead-review` behavior area of `ai-docs/spec/workflow-skills.md`
  (`#260513-review-workflow-skill`). No change to verdict vocabulary.

## Out of Scope

- Any change to the branch scenario's own runtime behavior — it must never
  run the landing lens, and no config mechanism may re-enable it there.
- Re-touching Phase 1's scenario-kind dispatch, config-load Invariant wording,
  or the branch-scenario diff/commit calls (`agents-plugin/rsrc/lead-review/lead-review.md`
  Invariants "Config Load" group, `### 1. Load config` steps 2-3, `### 2.
  Identify branch`, `### 3. Prepare` steps 1-3) beyond the minimal additive
  cross-references this phase needs (adding "Landing Lens" to the
  present/absent section-name lists).
- Splitting the landing lens into its own review posture/pipeline (explicitly
  deferred per the ticket's Decisions).
- Changing `judge: is-large-diff`'s subagent-parallelization wording to cover
  the landing phase — not requested; leave it scoped to alignment/risk as
  today.
- `agents-plugin/skills/lead-review/SKILL.md` and
  `agents-plugin-wsflow/skills/lead-review/SKILL.md` — both are pure
  `playbook.print` shims with no scenario/phase text (confirmed already in
  Phase 1's survey); no edit needed.
- Any Go code change — no code parses `## Review Phases`/`## Deep Review`/etc.
  section names (`rg` over `agents-plugin-tool/internal` for those strings
  outside this plan returned nothing); this is pure playbook-prose and
  spec/mental-model prose work.

## Codebase Findings

- `agents-plugin/rsrc/lead-review/lead-review.md#L9-L19` — `## Invariants`,
  `Config Load` group (L11-14). Line 13 (range-scenario absent-config
  fallback) names "built-in Review Phases / Deep Review defaults" — must gain
  "Landing Lens" in that list. Line 14 ("A present config's Review Phases,
  Checklist, Blocked Paths, and Deep Review sections are honored by **both**
  scenarios") is the exact sentence the landing lens must NOT be folded into
  unqualified — Landing Lens is honored by the range scenario only, so it
  needs its own clause or a new adjacent invariant, not a silent addition to
  this "both scenarios" list.
- `agents-plugin/rsrc/lead-review/lead-review.md#L21-L23` — `## On: invoke
  [branch?] [range: <base>..<head>]` entry line already determines scenario
  kind up front (range arg → range scenario; branch arg/default → branch
  scenario, range precedence on both supplied). No change needed here; the new
  landing-lens branching rides this existing scenario determination.
- `agents-plugin/rsrc/lead-review/lead-review.md#L25-L30` — `### 1. Load
  config` H3 block. Step 3 (L29, range-absent fallback) and step 4 (L30,
  present-config section list: "Remote, Branch Naming, Review Phases,
  Checklist, Blocked Paths, Comment Method, Merge Approval Method,
  Notification Method, Contributor Workflow, Deep Review") both need "Landing
  Lens" added, and step 4 needs an explicit "branch scenario ignores Landing
  Lens" clause mirroring the existing "Range scenario ignores Remote, Branch
  Naming, ..." clause already there for the opposite direction.
- `agents-plugin/rsrc/lead-review/lead-review.md#L45-L54` — `### 4. Review` H3
  block, step 4 (L51): `Run review phases in order: intent, alignment, risk,
  then any custom phases from config.` This is the single line that must gain
  scenario-conditional landing execution — range scenario runs the required
  `landing` phase (last, after custom phases or immediately after risk —
  executor's call, not specified by the ticket), branch scenario never does.
  Steps 1-3, 5-6 (diff/log calls, judges, checklist, verdict aggregation)
  already handle both scenarios and need no change; the landing finding folds
  into the same aggregate-and-verdict path as any other phase (step 6, no new
  verdict state).
- `agents-plugin/rsrc/lead-review/lead-review.md#L76-L115` — `### Review
  Config Template` fenced block. `## Review Phases` (L87-93: `intent`,
  `alignment`, `risk` subsections) is the natural sibling location for a new
  `## Landing Lens` section — it is a built-in-default, review-substance
  section like Review Phases and Deep Review (L113-114, which already
  demonstrates the template's pattern for a section that is "optional to
  write but has an unconditional built-in default": `## Deep Review ←
  optional` / `threshold: 20 files / 500 lines`). Add `## Landing Lens`
  following that same pattern, annotated as range-scenario-only, with default
  text grounded in what a spec vs. mental-model doc's function actually is
  (see next finding) rather than generic "add docs" language.
- `ai-docs/spec/documentation-system.md#L121-L136` (`#260505-spec-document-system`)
  and `#L189-L211` (`#260505-mental-model-document-system`) — canonical
  per-doc-type function definitions to ground the landing lens's default
  check text: specs "describe caller-visible project behavior"; mental-model
  docs "capture modification-relevant operational knowledge for agents
  changing the workflow system." The landing lens should ask whether a
  behavior change updated the doc matching *that* function, not just "any
  doc was touched" — this is what "per each doc's function" in the ticket
  means.
- `agents-plugin/rsrc/lead-review/lead-review.md#L167-L212` — `## Judgments`
  section (`has-blocked-paths`, `follows-ws-workflow`, `is-large-diff`,
  `has-checklist`, `has-comment-method`, `has-merge-approval-method`,
  `has-notification-method`). No new `judge:` entry is needed for the landing
  lens: unlike `has-checklist`/`has-blocked-paths` (presence-gated on an
  *optional* section), landing-lens execution is scenario-gated, already
  resolved by the scenario-kind determination at `On: invoke`'s entry line;
  its pass/fail criterion lives as phase text the same way `intent` /
  `alignment` / `risk` carry their criteria inline with no judge.
- `agents-plugin/rsrc/lead-review/lead-review.md#L215-L221` — `## Doctrine`,
  exact sentence `Review optimizes for **maintainer decision quality with
  minimum friction**.` is asserted verbatim by
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2749-L2766`
  (`TestPlaybookPrintGoldenLeadReview`). Do not alter this sentence; the
  scoping rationale (branch/PR external-contributor friction avoidance)
  already fits the existing "minimum friction" doctrine without needing new
  doctrine text — no Doctrine edit is required for this phase, but do not
  contradict it either.
- `agents-plugin/rsrc/manifest.json#L28` — current
  `lead-review/lead-review.md` hash (post-Phase-1) is
  `ab19c08657ccab0ada630915ee2a31abf14cc553333bb5af67a287fb040f4c3a`; will go
  stale the moment this phase's body edit lands and must be regenerated (same
  mechanism Phase 1 used).
- `agents-plugin/rsrc/lead-review/lead-review.md` mirrors byte-identically
  into `agents-plugin-wsflow/rsrc/lead-review/lead-review.md` (confirmed
  unchanged mechanism from Phase 1's finding). Only hand-edit the canonical
  file; regenerate the wsflow mirror, never hand-edit it.
- `agents-plugin/skills/lead-review/SKILL.md` /
  `agents-plugin-wsflow/skills/lead-review/SKILL.md` — pure `playbook.print`
  shims, `lead-review` is in the wsflow shipped-skill list
  (`ai-docs/manuals/wsflow-mirroring.md` "Shipped wsflow Skills") and not a
  substitution-mirrored-inline exception — no skill-entry edit needed.
- `ai-docs/spec/workflow-skills.md#L1122-L1165` (anchor
  `#260513-review-workflow-skill`, current lines) — Phase 1 already updated
  this area for the range scenario and scenario-scoped config-load. The
  phase-order paragraph (L1149-1151: "Review phases run in order — intent,
  alignment, risk, and any configured custom phases — producing one of four
  verdicts...") and/or the config-load paragraph (L1131-1141, which currently
  says "both scenarios honor its review-substance sections") need a landing-
  lens addition that mirrors the phase's scoping decision precisely (range
  scenario only; not part of the "both scenarios" set), keeping the anchor id
  unchanged.
- `ai-docs/mental-model/workflow-skills.md#L97` (`{#260513-review-workflow-skill}`
  bullet) — states "A present config's review-substance sections are honored
  by both scenarios regardless of which one is answering the config-load
  question." This is the mental-model bullet Phase 1 corrected once already
  (per the ticket's Phase 1 Result: "the mental-model bullet was corrected —
  it had implied unconditional forced setup"); it is now the exact sentence a
  future editor would misread as covering Landing Lens too. Needs a new
  adjacent bullet (or an amendment) stating the landing-lens exception is
  range-scenario-only, under the same anchor — this is precisely
  "modification-relevant operational knowledge for agents changing the
  workflow system" per the mental-model doc's own function.
- `ai-docs/tickets/ready/260824-feat-lead-review-range-scenario.md#L82-L125`
  — Phase 1's `### Result` block; confirms Phase 1 landed clean (golden test,
  wsrsrc tests, wsflow package tests, `spec_index_verify`, `go build` all
  green) and explicitly hands off: "Phase 2 (landing lens as a review-config
  required-check) remains; it rides this range-scenario path." No blocking
  residue from Phase 1's one accepted-minor review finding (an unrelated
  wording nit at L23, non-blocking, out of scope here).

## Implementation Plan

1. Edit `agents-plugin/rsrc/lead-review/lead-review.md` `## Invariants`
   (L9-19): in the `Config Load` group, add "Landing Lens" to the L13
   range-absent built-in-defaults list, and split L14 so the "both scenarios"
   claim excludes Landing Lens explicitly (e.g. "...are honored by both
   scenarios; `## Landing Lens` is honored by the range scenario only —
   branch scenario ignores it even if present."). Add a new grouped-invariant
   block (per `ai-docs/manuals/skill-authoring.md`'s "Grouped invariant
   lists" format), e.g. a `Landing Lens` group, stating: range scenario's
   `landing` phase is a required, always-runs Review Phase using config
   text if present else the built-in default; branch scenario never runs it
   and no config can add it there.
2. Edit `### 1. Load config` (L25-30): add "Landing Lens" to step 3's
   (L29) built-in-defaults fallback list and to step 4's (L30) present-config
   section-name list; append a clause to step 4 mirroring its existing
   "Range scenario ignores Remote, Branch Naming, ..." sentence, for the
   opposite direction: branch scenario ignores Landing Lens.
3. Edit `### 4. Review` (L45-54) step 4 (L51): change "Run review phases in
   order: intent, alignment, risk, then any custom phases from config." to
   add scenario-conditional landing execution — range scenario also runs the
   required `landing` phase (place it after custom phases, or immediately
   after risk before custom phases — pick one placement and state it
   explicitly since the ticket does not fix the order); branch scenario never
   runs it. Keep step 6 (aggregate → verdict) unchanged; the landing finding
   folds into the existing aggregate-and-verdict path with no new verdict
   state.
4. Edit `### Review Config Template` (L76-115): add a new `## Landing Lens`
   section immediately after `## Review Phases` (after the `risk` subsection,
   before `## Checklist`), annotated `← optional to customize; range scenario
   always runs it (built-in default below if omitted)` matching the `##
   Deep Review` annotation pattern. Default text should ask whether changed
   behavior has a matching spec update (caller-visible behavior,
   `ai-docs/spec/documentation-system.md#L121-136` framing) and/or
   mental-model update (modification-relevant operational knowledge,
   `documentation-system.md#L189-211` framing) authored per that doc's own
   function, plus convention adherence (repo conventions per `AGENTS.md`,
   `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`
   where applicable) — do not invent new doc-taxonomy language, ground the
   check text in the existing spec/mental-model function definitions found
   above.
5. Do not touch `## On: setup`'s numbered interview questions (L59-69) — the
   setup interview is branch-scenario-only per Phase 1, and the landing lens
   never triggers it (range scenario never enters setup, per Phase 1's
   Invariant, unchanged by this phase).
6. Do not touch `## Judgments` (L167-212) — no new `judge:` entry, per the
   Codebase Findings rationale (scenario-gated, not presence-gated; criterion
   lives as phase text like `intent`/`alignment`/`risk`).
7. Update `ai-docs/spec/workflow-skills.md` in the `#260513-review-workflow-skill`
   area (current L1122-1165): add the landing-lens required-check description
   — scoped to the range scenario only, folded into review-phase execution,
   not honored by the branch scenario even with a present config — matching
   the ticket's Spec Impact wording ("a landing-lens required-check in its
   review config"). Keep the anchor id unchanged (no heading rename).
8. Update `ai-docs/mental-model/workflow-skills.md` L97 area
   (`{#260513-review-workflow-skill}`): add a bullet (or amend L97) recording
   that the landing lens is the one review-substance section NOT honored by
   both scenarios — range scenario only — so a future editor does not
   misread L97's "both scenarios" framing as universal.
9. Run `ai-docs/manuals/skill-authoring.md`'s **On: Fresh-Reader Audit** on
   `agents-plugin/rsrc/lead-review/lead-review.md` after edits (required for
   doctrine/routing/layout edits — this phase changes Invariants and an `On:`
   handler's step ordering). Also run **On: Downstream Consistency Sweep**
   since this is a shared rsrc playbook edit (step 2 requires reading
   `ai-docs/manuals/wsflow-mirroring.md` first — already done for this plan).
10. Regenerate generated artifacts per `ai-docs/manuals/wsflow-mirroring.md`
    "Rsrc Tree Provisioning" → "After-edit checklist" (both steps, in order):
    - `cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
    - `cd agents-plugin-tool && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
    Do not hand-edit `agents-plugin-wsflow/rsrc/lead-review/lead-review.md` —
    it must come out byte-identical to the canonical file via this regen.
    `agents-plugin/skills/lead-review/SKILL.md` is untouched, so no
    `WSRSRC_REGEN_SKILLS` run is needed for this phase.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestPlaybookPrintGoldenLeadReview` —
  confirms the doctrine substring still resolves and no continuity-tip leak,
  after the Invariants/`On: invoke`/template edits.
- `cd agents-plugin-tool && go test ./internal/wsrsrc/...` — confirms
  `TestShippedManifestUpToDate` and `TestWsflowRsrcMirrorUpToDate` are green
  after the regen steps in Implementation Plan step 10 (expect red before
  regen, green after).
- `python3 -m unittest discover agents-plugin-wsflow/tests` — confirms the
  wsflow distributed skill bundle checks (thin-shim shape, forbidden
  references, shared playbook stem coverage) still pass; `lead-review`'s
  SKILL.md is untouched but the manual requires running this whenever a
  shared lead playbook body changes.
- `ws/spec_index_verify` (or `wsflow/spec_index_verify`) after the
  `ai-docs/spec/workflow-skills.md` edit, to confirm anchor
  `#260513-review-workflow-skill` stays indexed and well-formed.
- `go build ./...` in `agents-plugin-tool` — sanity build check (matches
  Phase 1's verification set); no Go source changes are expected in this
  phase, so this should be a no-op pass.
- Manual-only (no automated runtime harness for playbook prose; matches the
  ticket's stated verification boundary) — walk the edited `On: invoke`
  handler by hand for both scenarios:
  1. Range scenario, landing-lens-relevant diff (behavior change with no
     matching spec/mental-model update) → confirm the walked procedure
     reaches the `landing` phase and would surface a finding there, folding
     into NEEDS FIX/OPEN via the existing aggregate-and-verdict path (no new
     verdict state).
  2. Range scenario, a convention-conformant, doc-complete diff → confirm the
     walked procedure's `landing` phase would pass, contributing to LGTM.
  3. Branch scenario (with or without a present `## Landing Lens` config
     section) → confirm the walked procedure never reaches the `landing`
     phase at all.
  4. Range scenario, absent `ai-docs/_review.local.md` → confirm the walked
     procedure still never enters `On: setup` (Phase 1's invariant,
     unaffected) and uses the new built-in Landing Lens default text.

## Escalations

- None.
