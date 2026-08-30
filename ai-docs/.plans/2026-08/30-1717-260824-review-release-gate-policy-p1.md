# Plan: 260824-feat-review-release-gate-policy — Phase 1: Policy config surface (AGENTS.md + _review.local.md) + workflow_manual nudge

## Relevant Ticket Contract

- Define `AGENTS.md` fields: review-track branch, release-boundary
  declaration (present/absent), and rendezvous backend (`platform` |
  `canary`, `260829`). When `platform`, document the recommended GitHub
  branch-protection set (require-up-to-date + dismiss-stale-approvals +
  required-checks + disable squash/rebase; merge queue at scale).
- Release-tag identification: Phase 2's `<tag>..HEAD` range needs the last
  release tag, identified via a tag-glob (default `v*`, resolved with
  `git describe --tags --match '<glob>'`); a boundary project declares its
  own glob alongside the boundary declaration if its tags use a different
  namespace.
- Define/confirm the `_review.local.md` review-mechanics home (already
  exists) and note the three-config-homes split so nothing double-owns:
  `AGENTS.md` (tracked, per-track structural facts) vs. `ai-docs/` ledger
  (marker+verdict state, already implemented) vs. `_review.local.md`
  (machine-local, gitignored mechanics).
- `workflow_manual` scans for the review-track config and emits a
  **non-blocking**, **session-scoped (at most once per session)** nudge
  when it is unset — not a hard block, and not a per-checkpoint repeat.
- Verification boundary (from the ticket): a repo with the field set
  exposes the review-track to the sweep; an unset repo gets exactly one
  scoped nudge, never a block; the three config homes have no overlapping
  ownership.
- Host-neutral: gating is opt-in; absence of a declared boundary means
  advisory-only, not "no review" (this framing belongs to Phase 2's gate,
  but Phase 1's config surface must not itself imply a hard dependency).

## Out of Scope

- Phase 2 (the mandatory release gate itself, `lead-ship` pre-flight
  insertion, gate verdict logic, promotion atomicity) — explicitly excluded
  by the lead's authority selection.
- `260824-feat-lead-review-range-scenario` (②) and the ledger/marker
  mechanics of `260824-feat-review-watermark-ledger` (③) — both already
  landed in `.done/`; Phase 1 only *reads* their existing surface
  (`wsreview.ResolveTrack`, `wsreview.Read`), it does not modify ledger
  semantics.
- The multi-maintainer canary/banner/no-squash mechanics — already
  implemented in ③ Phase 3; Phase 1 only documents the `platform` vs.
  `canary` rendezvous-backend *choice* as an AGENTS.md field, it does not
  re-implement the canary.
- Whether/how to add a matching versioned-checklist entry to the two
  `AGENTS.template.md` bootstrap templates so downstream projects can
  adopt this field set through normal `lead-bootstrap` upgrades — flagged
  below under Escalations as a lead judgment call, not decided here.

## Codebase Findings

- `agents-plugin-tool/internal/wsreview/track.go#L11-L38` —
  `ResolveTrack` is explicitly documented as "the interim fallback" for
  the not-yet-existing `AGENTS.md` review-track declaration this ticket
  (④) owns. It resolves `origin/HEAD` → local `main` → local `master`,
  fails open to `("", err)`. Phase 1 should make this function prefer an
  `AGENTS.md`-declared branch first, falling back to the existing
  git-heuristic when unset — this is the direct mechanism that satisfies
  "a repo with the field set exposes the review-track to the sweep."
- `agents-plugin-tool/internal/wsreview/config.go#L1-L59` — `StalenessKnob`
  is the existing `_review.local.md` reader: regex-isolate a `## <Heading>`
  section, regex-match a `key: value` line inside it, fail open to a
  built-in default on any parse miss (missing file/section/malformed
  value). This is the reusable parsing shape for the new `AGENTS.md`
  fields (same fail-open contract, same section+line regex style) — a new
  sibling reader belongs in `wsreview` (e.g. `agents_config.go`), not a
  new ad-hoc parser.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L11-L17,79-137` —
  the existing `AGENTS.md`-marker-reading precedent:
  `templateVersionTag` regexp for `<!-- Template Version: vNNNN -->`, read
  via `readInstalledVersionState`, and `bootstrapStalenessWarning`'s
  fail-open silent-vs-firing case list. Confirms `AGENTS.md` is already a
  parsed (not just human-read) config surface in this codebase, with an
  established regex-marker convention. `wsdoc/doctor.go#L24` also treats
  `AGENTS.md` as an optional-but-checked root file.
- `agents-plugin-tool/internal/mcp/doc_coverage_alarm.go#L1-L47` — the
  cleanest small template to mirror for a new `warning()`/`inject...()`
  function pair: a pure `root(, resolver, sessionKey) string` warning
  computer plus a one-line inject wrapper delegating to
  `injectBootstrapStalenessWarning` (which is the shared
  prepend-if-nonempty helper — reuse it directly, do not write a new
  inject helper).
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L255-L327` — the two
  live branches (FRESH-with-root, CONTINUE) where every existing ambient
  nudge (`bootstrapStalenessWarning`, `docCoverageWarning`,
  `scopeAnnouncement`, `computeManuals`, `wsreview.CheckpointNudge`) is
  wired, always unconditionally recomputed and injected on **every** call.
  This is the wiring site for the new nudge, but note the shape mismatch
  below (risk signal).
- **Risk signal — no existing "fire at most once per session" nudge
  exists.** Every current `workflow_manual` nudge (`CheckpointNudge`,
  `docCoverageWarning`, `bootstrapStalenessWarning`, `scopeAnnouncement`)
  recomputes fresh on every call and has no suppression state; none of
  them match the ticket's "session-scoped, at most once per session"
  requirement, so this piece needs new session-state plumbing, not a
  copy-paste of an existing nudge. Found reusable primitives for it:
  - `agents-plugin-tool/internal/mcp/session_auth.go#L46-L67` —
    `sessionRecord` is explicitly documented as growing via **additive
    fields** (`Overrides`, `Agenda`, `Todos`, `Note` — each
    `omitempty`, each safe for older records to parse as zero-value). A
    new `ReviewTrackNudgeShown bool \`json:"review_track_nudge_shown,omitempty"\`` field follows this exact precedent.
  - `agents-plugin-tool/internal/mcp/session_auth.go#L305-L318` —
    `setNote(targetKey, text string) error` is the direct template for a
    new `setReviewTrackNudgeShown(targetKey string) error`, both built on:
  - `agents-plugin-tool/internal/mcp/session_state.go#L749-L764` —
    `mutateRecord(sessionKey, fn) error`, the shared atomic
    read-modify-write primitive (already used by `setNote`/`setOverride`)
    that both reads and writes the record under `s.mu`. This is the
    correct primitive to reuse; do not add a second ad-hoc
    read-modify-write path.
  - In the FRESH-with-root branch, `rec` does not pre-exist (a brand new
    key is minted via `s.sessions.mint` at
    `workflow_manual.go#L266`), so the flag is implicitly false on first
    render — the nudge fires unconditionally there (correct: first call in
    a fresh session) and must be written `true` immediately after via
    `mutateRecord`/`setReviewTrackNudgeShown` so a same-session CONTINUE
    call does not repeat it. In the CONTINUE branch, `rec` is the resolved
    existing record (`workflow_manual.go` CONTINUE branch, `rec.Root`
    guarded block at L310-326) — check `rec.ReviewTrackNudgeShown` before
    computing/injecting, and write it `true` via the same setter after a
    non-empty nudge fires.
- `ai-docs/spec/mcp-tools.md#L1880-L1970` (`{#260830-review-watermark-ledger-tools}`,
  `{#260830-review-watermark-checkpoint-nudge}`) — the spec-anchor style and
  prose shape to match for documenting the new nudge's caller-visible
  contract (silent/firing case enumeration, exact call sites, fail-open
  behavior).
- `ai-docs/spec/workflow-skills.md#L1122-L1170` (`{#260513-review-workflow-skills}`)
  and the ticket's own `## Spec Impact` — `ai-docs/spec/workflow-skills.md`
  is the named target for the `AGENTS.md` review-track/boundary config
  contract prose (host-neutral, no devenv-specific shape encoded into the
  mechanism description).
- `ai-docs/ship/ws.md#L1-L20` — confirms the `v<version>` tag scheme
  (`agents-plugin/runtime.json` `.plugin_version`/`.release_tag`) that
  motivates the default `v*` release-tag glob; devenv needs no override.
- `AGENTS.md` (this project's own root file, current content) — has no
  review-track/boundary/rendezvous-backend section yet. `## Workflow` →
  `### Branch Policy` / `### Commit Rules` is the closest existing
  subsection pattern to extend with a new `### Review Policy` subsection,
  since devenv is itself the boundary project the ticket's Background
  section names (`develop`→`main` ship).
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` and the wsflow
  mirror — carry a **versioned migration checklist** (`v0001`..`v0047+`)
  that both packages must bump in lockstep per the mental model's
  "Bootstrap template changes must check both ws and wsflow packages"
  coupling rule. Adding the new fields to the template (so downstream
  projects can adopt them via ordinary `lead-bootstrap` upgrade) would
  require a new checklist entry in **both** files with the same shared
  migration-ordinal counter — real work, not in Phase 1's stated
  verification boundary. Flagged under Escalations, not decided here.

## Implementation Plan

1. **`agents-plugin-tool/internal/wsreview/agents_config.go` (new file).**
   Add a fail-open reader for the `AGENTS.md` review-policy fields,
   mirroring `config.go`'s section+line regex shape: a `## Review Policy`
   (exact heading TBD by author, but must match what step 4 documents)
   section containing `review-track: <branch>`,
   `release-boundary: present|absent`,
   `rendezvous-backend: platform|canary`, and
   `release-tag-glob: <glob>` (default `v*` when absent/malformed) lines.
   Missing file/section/field all fail open to zero-value/absent, never an
   error — same contract as `StalenessKnob`. Add a `_test.go` alongside it
   covering: all fields present, file/section/field absent, malformed
   `release-boundary`/`rendezvous-backend` enum values (fail open to
   absent/`canary` respectively — pick `canary` as the built-in default
   since it needs no platform config).
2. **`agents-plugin-tool/internal/wsreview/track.go`.** Update
   `ResolveTrack` to check the new `AGENTS.md` `review-track` field first
   (via step 1's reader); only fall back to the existing
   `origin/HEAD`/`main`/`master` git heuristic when the field is absent.
   Update the doc comment to drop the "does not exist yet" framing (this
   ticket is what makes it exist) and describe the new precedence order.
   Extend `track_test.go` with a case asserting the `AGENTS.md`-declared
   branch wins over the git-default fallback even when they differ.
3. **New nudge function + `sessionRecord` field + setter (once-per-session
   shape, see Risk Signal above).**
   - Add `ReviewTrackNudgeShown bool \`json:"review_track_nudge_shown,omitempty"\`` to `sessionRecord` (`session_auth.go`).
   - Add `func (s *sessionStore) setReviewTrackNudgeShown(targetKey string) error` mirroring `setNote`, built on `mutateRecord`.
   - Add a new `agents-plugin-tool/internal/mcp/review_track_alarm.go` (or
     similarly named, matching `doc_coverage_alarm.go`'s file-per-alarm
     convention) exporting a pure `reviewTrackNudge(root string) string`
     that calls step 1's `AGENTS.md` reader and returns a fixed
     "configure a review-track branch in AGENTS.md first" advisory string
     when the `review-track` field is absent, else `""`. This function
     itself is *not* the once-per-session gate — it is stateless, like
     `docCoverageWarning`.
   - In `workflow_manual.go`'s FRESH-with-root branch (after the existing
     `wsreview.CheckpointNudge` injection at `L292`): compute
     `reviewTrackNudge(canonical)`; if non-empty, inject via
     `injectBootstrapStalenessWarning` and call
     `s.sessions.setReviewTrackNudgeShown(mintedKey)`.
   - In the CONTINUE branch's `rec.Root != ""` block (after `L325`): if
     `!rec.ReviewTrackNudgeShown`, compute `reviewTrackNudge(rec.Root)`;
     if non-empty, inject and call
     `s.sessions.setReviewTrackNudgeShown(key)`.
   - Add tests in `agents-plugin-tool/internal/mcp/` (mirroring the
     existing `review_watermark_checkpoint_test.go` shape) covering: unset
     config fires once on FRESH-with-root; a second CONTINUE call in the
     same session does not repeat it; a set config never fires; the nudge
     never blocks (response is always success text).
4. **Document the `AGENTS.md` fields.**
   - Add a `### Review Policy` subsection under this project's own
     `AGENTS.md` `## Workflow` heading (devenv is the boundary project
     named in the ticket's Background), stating devenv's own
     `review-track: develop`, `release-boundary: present`,
     `rendezvous-backend: canary` (devenv is single-maintainer serial
     today per the `260829` research ticket's open question — `canary`
     needs no GitHub config and is the safer default; flag this specific
     value choice for lead confirmation since it is a devenv-specific
     policy decision, not a mechanism decision), and
     `release-tag-glob: v*` (already devenv's real scheme per
     `ai-docs/ship/ws.md`).
   - Document the field syntax generically (heading + `key: value` lines)
     and the recommended GitHub branch-protection set for `platform`
     (require-up-to-date, dismiss-stale-approvals, required-checks,
     disable squash/rebase; merge queue at scale) — this generic
     documentation belongs in `ai-docs/spec/workflow-skills.md` per the
     ticket's `## Spec Impact`, not only in devenv's own `AGENTS.md`.
   - Add a `{#26083x-review-policy-config-surface}`-style new anchor to
     `ai-docs/spec/workflow-skills.md` (near `{#260513-review-workflow-skills}`)
     describing: the three config homes and their non-overlapping
     ownership (explicitly restate the "do not put review-track branch in
     `_review.local.md`, do not put the marker in `AGENTS.md`" constraint
     from the ticket's `## Decisions`), the four `AGENTS.md` fields and
     their defaults, the release-tag-glob/`git describe` resolution
     mechanism, and the `workflow_manual` nudge's non-blocking
     session-scoped contract (mirroring `{#260830-review-watermark-checkpoint-nudge}`'s
     documentation shape).
5. **Regenerate manifests / run build gates** per `agents-plugin-tool`
   convention (`go build ./...`, `go vet ./...`) — no `wsrsrc` manifest
   regen is expected since this phase touches no `SKILL.md`/rsrc playbook
   text, only Go source, `AGENTS.md`, and spec prose. Confirm with
   `spec_index.verify` after the spec edit.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./...` clean.
- `go test ./internal/wsreview/... ./internal/mcp/...` — new tests from
  steps 1-3 pass alongside the existing suite (22 `wsreview` tests + the
  existing `mcp` watermark integration tests must stay green).
- Manual/targeted check: with `AGENTS.md`'s `review-track` field set,
  `wsreview.ResolveTrack` returns the declared branch even when it differs
  from the git default branch (exercises the "field set exposes the
  review-track to the sweep" verification line directly).
- Manual/targeted check: a fresh session with no `review-track` field
  produces exactly one nudge across a FRESH-with-root call followed by a
  CONTINUE call on the same key (never two, never a block/error).
- `ws/spec_index_verify` (or the CLI mirror) after the spec edit — no
  duplicate-anchor or index drift.

## Escalations

- Confidence: high on the mechanical pieces (AGENTS.md field reader,
  `ResolveTrack` precedence change, spec/doc prose) — all have direct,
  strong precedent in the codebase (`config.go`, `bootstrap_alarm.go`,
  `doc_coverage_alarm.go`, `track.go`'s own "interim fallback" doc
  comment naming this ticket).
- Medium confidence, flagged for lead review (not full research
  escalation — a concrete, precedented mechanism is proposed, but it is
  genuinely new shape in this codebase and worth a second look before
  implementation):
  1. **Once-per-session nudge storage** (Implementation Plan step 3): no
     existing `workflow_manual` nudge is session-scoped-once today; the
     proposed `sessionRecord` additive-field + `mutateRecord` setter
     mirrors `setNote`/`Overrides`/`Agenda` precedent exactly, but the
     lead should confirm this is the intended mechanism before an
     implementer builds on it.
  2. **Whether Phase 1 also updates the two `AGENTS.template.md` bootstrap
     templates** (new versioned checklist entry, both packages in
     lockstep) so downstream projects can adopt this config through
     ordinary `lead-bootstrap` upgrades, or whether that is deliberately
     deferred to a follow-up ticket. The Phase 1 ticket text does not
     explicitly ask for it, and its stated verification boundary
     (devenv's own repo + an "unset repo gets exactly one nudge") does
     not require it, so this plan treats it as Out of Scope — but it is a
     real gap if left unaddressed indefinitely (no downstream project ever
     gets this field set via bootstrap).
  3. **devenv's own `rendezvous-backend` value** (`canary` vs. `platform`):
     proposed as `canary` (simplest, no GitHub config needed, matches
     devenv's current single-maintainer-serial reality per the `260829`
     research ticket's own open question "whether devenv itself adopts the
     multi-maintainer machinery"), but this is a policy choice for the
     lead/user, not a mechanism fact — flagging rather than deciding
     silently.

## Lead Adjudications (30-1717)

Goal-run posture: reversible local decisions resolved on stated
recommendation and recorded; only genuinely-unsettled or downstream-affecting
items held. All three survey flags resolved:

1. **Once-per-session nudge storage — APPROVED as proposed (step 3).** The
   additive `sessionRecord` bool field (`ReviewTrackNudgeShown`, `omitempty`)
   + `setReviewTrackNudgeShown` setter built on the existing `mutateRecord`
   RMW primitive is the correct mechanism. It is the *ticket-mandated*
   "session-scoped, at most once per session" behavior (ticket Phase 1 line),
   not new scope; and the additive-omitempty-field convention is documented
   and precedented (`Overrides`/`Agenda`/`Todos`/`Note`), so older records
   parse it as zero-value (false → fires on first call, correct). Not a public
   API or cross-module pattern change — an internal session-state field. The
   stateless `reviewTrackNudge(root) string` computer stays pure like
   `docCoverageWarning`; the once-fire gate lives only at the two
   `workflow_manual.go` wiring sites. Build it.

2. **`AGENTS.template.md` bootstrap templates — CONFIRMED Out of Scope for
   Phase 1.** Ticket Phase 1 text does not ask for it, and its verification
   boundary (devenv's own repo + "an unset repo gets exactly one nudge") does
   not require it. Independently, AGENTS.md workflow policy classes
   "template changes that affect downstream projects" as **Ask first** — a
   downstream-affecting template change must not be silently folded into an
   implementation phase. **Recorded as a forward gap**: no downstream project
   gets these fields via `lead-bootstrap` until a follow-up addresses the two
   templates (both packages, lockstep versioned checklist bump). Surface at
   the final gate for the user to route (follow-up child ticket under epic
   `260824-epic-review-watermark-model`, or fold into Phase 2 rollout). Do
   NOT touch the templates in this phase.

3. **devenv's config values — ADJUDICATED (reversible local policy).**
   `rendezvous-backend: canary` (single-maintainer-serial today; needs no
   GitHub branch-protection; the ③ ledger-conflict canary is already built).
   With it: `review-track: develop` (work lands and is reviewed on develop;
   ship is develop→main), `release-boundary: present` (develop→main ship
   exists — inert until Phase 2 builds the gate, so safe to declare now),
   `release-tag-glob: v*` (devenv's real scheme per `ai-docs/ship/ws.md`;
   no override needed). Populating devenv's own `AGENTS.md ### Review Policy`
   is pre-authorized by this ticket's sage-settled charter (Phase 1 IS the
   config-surface deliverable) and makes devenv the dogfooded "set" case; the
   "unset → one nudge" case is covered by the `mcp` tests, not by leaving
   devenv unset.

### Scope note carried to the implementer

- Implementer scope = Implementation Plan steps 1-3 (Go code + tests):
  new `agents_config.go` reader, `ResolveTrack` precedence change, and the
  once-per-session nudge (field + setter + `review_track_alarm.go` + two
  wiring sites + `mcp` tests). Plus step 4's **devenv `AGENTS.md ### Review
  Policy` population** with the adjudicated values above (it is source-adjacent
  config the implementer sets, using the exact field syntax the reader parses).
- The **generic spec prose** (step 4's `workflow-skills.md` anchor + the
  three-config-homes ownership + GitHub branch-protection recommendation) and
  any mental-model update move to the `{doc-pre-pass}` step, per the runbook's
  doc-pipeline ownership — NOT the implementer.
