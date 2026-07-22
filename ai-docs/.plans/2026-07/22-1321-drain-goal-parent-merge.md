# Plan: drain-goal-parent-merge

## Relevant Ticket Contract

- Encode the fork parent into the goal branch name (stateless naming
  convention): new shape `goal/<parent>/<slug>`, was `goal/<slug>`. `<slug>`
  stays the existing random word-word-word token (slash-free).
- CREATION: capture the current branch (= fork parent) before checkout, then
  `git checkout -b goal/<parent>/<slug>` off it. Detached-HEAD guard: if
  `git rev-parse --abbrev-ref HEAD` returns `HEAD`, abort staging-branch
  creation with a clear message instead of creating `goal/HEAD/<slug>`.
- PARSE rule at completion: strip `goal/` prefix; SLUG = final
  slash-delimited segment; PARENT = everything between (rsplit on the last
  `/`). A parent containing slashes (e.g. `feature/foo`) round-trips
  correctly: `goal/feature/foo/<slug>` -> parent `feature/foo`.
- COMPLETION MERGE: derive parent from the branch name, merge
  `goal/<parent>/<slug>` into `<parent>` (not hardcoded `main`). Keep the
  existing user-approval gate; never push.
- BACKWARD-COMPAT fallback: old-format `goal/<slug>` (exactly one segment
  after `goal/`, no parent segment) -> fall back to merging into `main`.
- Files in scope: `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`
  (canonical), `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md`
  (byte-identical mirror), `ai-docs/spec/workflow-skills.md`
  (`{#260707-drain-goal-branch-staging}`). The mental-model entry
  (`ai-docs/mental-model/workflow-skills.md`,
  `{#260707-drain-goal-branch-staging}`) needs the same update but is owned
  by the doc pre-pass / mental-model-updater stage, not this implementer —
  record the required change only, do not edit it here.
- Non-goals: do not touch `implement_resolver.go` (branch-agnostic already);
  no config keys/git-upstream/session-state for the parent; do not edit the
  `.done` design ticket's decision text; no push, merge stays
  user-approval-gated.

## Out of Scope

- `implement_resolver.go` / `deriveImplementBranchPlan` — confirmed
  branch-name-agnostic (only checks `impl/`/`implement/` prefixes), stays
  untouched.
- Mental-model file edit — required change is recorded below for the doc
  pre-pass stage, not performed by this implementer.
- The `.dropped` ticket `260707-research-drain-queue-default-branch-policy`
  (reuse+rename / auto-delete / shorter naming friction points) — adjacent
  but unrelated to this fix; not touched.
- `agents-plugin/rsrc/` and `agents-plugin-wsflow/rsrc/` trees — unaffected;
  this change lives entirely under `skills/`.

## Codebase Findings

- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md#L15-L47` — full
  canonical prose. Three passages need edits:
  - L17-18 (creation-context check): "the current branch not already
    `goal/*`" wording is prefix-based already and needs no change, but the
    branch-creation sentence at L29-33 (`git checkout -b goal/<slug>`, slug
    generation) must add the "capture current branch as parent first" step,
    the new `goal/<parent>/<slug>` naming, and the detached-HEAD guard.
  - L15-24 (empty-queue completion branch): currently says "When it is
    `goal/<slug>`... merge `goal/<slug>` into `main`" — must become
    parent-derivation logic (rsplit on last `/`) with the old-format
    (single-segment) fallback to `main`, and the merge command example
    (`git checkout main && git merge --no-ff goal/<slug>`) must reflect
    `git checkout <parent> && git merge --no-ff goal/<parent>/<slug>`.
  - L26-38 (staging-branch creation on ticket dispatch) and L41-47
    (hand-off with `merge_confirm: skip`) both reference `goal/<slug>` in
    prose and must be updated to the new naming; the `merge_confirm: skip`
    /no-`merge_target`-override behavior itself is unaffected (confirmed
    below) and needs no logic change, only the branch-name literal in text.
- `diff` of `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` vs
  `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` — exit 0
  (currently byte-identical). No `ws:`/`ws/` namespace tokens present in
  this file, so `GenerateWsflowSkillBody` is a substitution no-op here;
  after editing, the mirror must be re-produced from the same edited text
  (recommended via the regen test below, not a manual copy, to stay on the
  documented generation path).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-L19` —
  `substitutionMirroredSkills` explicitly lists `lead-drain-ready-queue`.
  This means `TestWsflowSkillsMirrorUpToDate` (L38-85) DOES cover this
  file and will fail once the canonical SKILL.md is edited but the wsflow
  mirror is not regenerated. Regenerate with
  `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
  (from `agents-plugin-tool/`), then confirm with
  `go test ./internal/wsrsrc -run TestWsflowSkillsMirrorUpToDate`. Also
  still run manual `diff` after regen — expect exit 0, since this file has
  no substitutable tokens (byte-identical mirror is the same invariant the
  contract asks for).
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L26-L68` — a
  second, independent mechanism: `agents-plugin/skills/manifest.json`
  hashes the `agents-plugin/skills/` tree (confirmed entry:
  `lead-drain-ready-queue/SKILL.md` ->
  `2249a0bae82586454aed4a48a414dd85953cb71691d77f4981967347957c1026` in the
  current manifest). Editing the canonical SKILL.md changes its hash, so
  `TestSkillsManifestDriftIsVisible` will fail until regenerated:
  `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`,
  then confirm with `go test ./internal/wsrsrc -run TestSkillsManifestDriftIsVisible`.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L11-L44` —
  `TestShippedManifestUpToDate` / shipped rsrc manifest operates only on
  `agents-plugin/rsrc` (`shippedRsrcRoot()`), NOT `agents-plugin/skills`.
  Confirmed out of scope for this change (no edit under `rsrc/`).
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L12-L54` —
  `TestWsflowRsrcMirrorUpToDate` compares `agents-plugin/rsrc` vs
  `agents-plugin-wsflow/rsrc` byte trees only. Not the skills tree; not
  affected by this change.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L46-L88` —
  `TestRetiredAPIGuidanceNotShipped` does walk `agents-plugin/skills`
  (among other roots) but only greps for retired `ws/api.ask*` guidance
  strings; the new prose introduces none of those, so this test is
  unaffected but will still run as part of the package test pass.
- Grep sweep for exact-shape goal-branch parsing (contract requirement):
  - `agents-plugin-tool/internal/**/*.go` (non-test): zero hits for
    `goal/` anywhere. No Go source code parses or special-cases goal
    branch names at all.
  - `agents-plugin-tool/internal/mcp/session_state_test.go#L307-L334` (and
    its `.worktree/ws-dashboard-dev/` copy) — the only Go hits: two test
    fixtures set `BranchPlan.CurrentBranch: "goal/drain-example"` as an
    opaque string to exercise `deriveImplementTodosFromVerdict`'s
    `merge_confirm: skip` wording. The function only does
    `strings.Contains`/formatting on the whole string, never splits it —
    confirmed no fixed-segment-count parsing. These tests are unaffected by
    the naming-shape change and do not need updating (the new shape
    `goal/<parent>/<slug>` would work identically as an opaque string here
    too, but changing the fixture is not required since nothing in this
    contract touches this file).
  - `agents-plugin-tool/internal/mcp/implement_resolver.go#L575-L707` —
    `deriveImplementBranchPlan`/`validObservedBranch` only checks
    `strings.HasPrefix(obs.CurrentBranch, "impl/")` and `"implement/"`;
    when the current branch has neither prefix it is used verbatim as
    `plan.MergeTarget` (L696-698). This is exactly the mechanism that lets
    each ticket's own `impl/<stem>` branch merge into whatever the current
    branch is named, including `goal/<parent>/<slug>` — fully
    branch-name-agnostic, confirmed untouched and unaffected by the rename.
  - No shell/glob goal-branch matching exists anywhere in code (`grep -rn
    "goal/\*"` across `*.go`/`*.md`/`*.sh` outside `.worktree` only matches
    prose in the SKILL.md files, the spec, the mental-model file, and
    ticket/plan history — never an actual `[[ $b == goal/* ]]` shell
    conditional or Go `strings.HasPrefix(x, "goal/")` call). The `goal/*`
    "current branch" check described in the skill is performed by the lead
    agent's own reasoning at runtime, not by code, so a 3-segment name
    matches trivially — no glob/prefix code exists to fail or need updating.
  - Prose-only hits elsewhere (no code, no change needed): `CHANGELOG.md`,
    `ai-docs/tickets/.done/260713-bug-lead-drain-ready-queue-goal-branch-slug-collision.md`,
    `ai-docs/tickets/.done/260707-feat-drain-goal-branch-staging.md`
    (design ticket, explicitly out of scope per contract non-goals),
    `ai-docs/tickets/.dropped/260707-research-drain-queue-default-branch-policy.md`,
    `ai-docs/.plans/2026-07/07-2045-merge-confirm-fact.md`,
    `ai-docs/.plans/2026-07/07-2059-drain-goal-aware.md`,
    `ai-docs/.plans/2026-07/13-1727-260630-bug-enter-proceed-status-report-dead-code.md`,
    `ai-docs/.plans/2026-07/14-1035-slug-cap-fix.md` — all historical
    plan/ticket artifacts, not live contract surfaces; left untouched.
- `ai-docs/spec/workflow-skills.md#L458-L485` — the exact spec paragraph
  under `{#260707-drain-goal-branch-staging}` (anchor line 485, attached to
  this paragraph). Currently states: "...derives a branch-safe slug from
  the goal text and creates/checks out `goal/<slug>`..." (L463-464) and
  "...then `git merge --no-ff goal/<slug>` into `main`..." (L476-477).
  Needs the parent-encoded naming, the detached-HEAD guard, and the
  parent-derived merge with old-format fallback folded in; keep the rest of
  the paragraph's structure (staging trigger conditions, `merge_confirm:
  skip` mechanism, per-ticket `impl/<stem>` merge-into-goal-branch
  behavior) unchanged since those are unaffected by this fix.
- `ai-docs/mental-model/workflow-skills.md#L69` — single-line mental-model
  entry under the same anchor; same two stale phrases (`goal/<slug>`
  naming, "merge `goal/<slug>` into `main`"). Per contract, record the
  required change here for the doc pre-pass / mental-model-updater stage —
  do not edit this file as part of implementation.

## Implementation Plan

1. Edit `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`:
   - Completion paragraph (currently L15-24): replace the "`goal/<slug>`...
     merge `goal/<slug>` into `main`" logic with: detect current branch is
     `goal/*`; strip the `goal/` prefix; split on the LAST `/` to get
     PARENT (everything before) and SLUG (final segment); if there is no
     parent segment (old-format `goal/<slug>`), fall back to `main` as the
     merge target; ask the user for explicit approval to merge
     `goal/<parent>/<slug>` into `<parent>` (or `goal/<slug>` into `main`
     for the fallback case); on approval, run
     `git checkout <parent> && git merge --no-ff goal/<parent>/<slug>`
     (or the `main`-fallback equivalent). Keep the never-push constraint
     verbatim.
   - Creation paragraph (currently L26-38): before `git checkout -b
     goal/<slug>`, capture the current branch name (e.g. via `git
     rev-parse --abbrev-ref HEAD`) as the parent. Add the detached-HEAD
     guard: if that command yields literal `HEAD`, abort staging-branch
     creation with a clear message (do not create `goal/HEAD/<slug>`) and
     fall through to the non-staging path. Otherwise create/check out
     `goal/<parent>/<slug>` instead of `goal/<slug>`.
   - Update every other `goal/<slug>` literal in the file (the
     goal-detection sentence, the hand-off paragraph at L41-47) to the new
     `goal/<parent>/<slug>` shape or to parent-agnostic phrasing (e.g. "the
     checked-out goal branch") where the parent value itself is not
     relevant to that sentence.
2. Regenerate the wsflow mirror from the edited canonical file (do not
   hand-copy): from `agents-plugin-tool/`, run
   `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`.
   This rewrites
   `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md`.
3. Regenerate `agents-plugin/skills/manifest.json`: from
   `agents-plugin-tool/`, run
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`.
4. Edit `ai-docs/spec/workflow-skills.md` L458-485 (the paragraph under
   `{#260707-drain-goal-branch-staging}`): update the naming and merge
   description to match the new creation/completion behavior encoded in
   step 1, preserving the rest of the paragraph's content (staging
   trigger, `merge_confirm: skip`, per-ticket merge-into-goal-branch flow)
   unchanged. Keep the anchor `{#260707-drain-goal-branch-staging}` as-is
   (no heading rename, so no `renamed-spec` note needed).
5. Do NOT edit `ai-docs/mental-model/workflow-skills.md`. Leave this to the
   doc pre-pass / mental-model-updater stage. Required change to hand off:
   update L69's `goal/<slug>` naming and "merge `goal/<slug>` into `main`"
   phrasing to the parent-encoded naming and parent-derived merge with
   old-format fallback, mirroring the spec update in step 4.
6. Do not touch `implement_resolver.go`, config/session-state files, or the
   `.done` design ticket's decision text (non-goals, confirmed no incidental
   need to touch them).

## Verification Plan

- `diff agents-plugin/skills/lead-drain-ready-queue/SKILL.md agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` — expect exit 0 (byte-identical, both before manual comparison and after the regen step).
- From `agents-plugin-tool/`: `go test ./internal/wsrsrc/...` (full package) — must pass, in particular `TestWsflowSkillsMirrorUpToDate` and `TestSkillsManifestDriftIsVisible` (both cover this skill file per the findings above) and `TestRetiredAPIGuidanceNotShipped` (walks `agents-plugin/skills`, must stay clean of retired-API strings — the new prose introduces none).
- From `agents-plugin-tool/`: `go build ./...` — this is a prose-only skill/doc change with no Go source edits expected, but run the build as a cheap regression check per the contract's verification boundary ("if Go or test files end up touched, run the repo Go build + relevant tests"); if `git diff` after the full change touches no `.go` files, this step may be a quick confirmatory build rather than a required gate.
- Manual read-through of the edited `agents-plugin/skills/lead-drain-ready-queue/SKILL.md`: confirm every `goal/<slug>` literal was updated to `goal/<parent>/<slug>` (or generalized), the detached-HEAD guard reads clearly, and the old-format single-segment fallback to `main` is stated explicitly and unambiguously (this is prose an agent will execute at runtime — ambiguity here is a functional bug, not a style issue).
- Confirm `ai-docs/spec/workflow-skills.md` still has exactly one
  `{#260707-drain-goal-branch-staging}` anchor and its paragraph text is
  internally consistent with the updated SKILL.md (same naming shape, same
  fallback rule).
- `git status`/`git diff --stat` at the end: expect changes limited to the
  two SKILL.md files, `agents-plugin/skills/manifest.json`, and
  `ai-docs/spec/workflow-skills.md` — no changes under
  `agents-plugin-tool/internal/mcp/` (`implement_resolver.go` untouched),
  no changes under `agents-plugin/rsrc/` or `agents-plugin-wsflow/rsrc/`,
  no changes to `ai-docs/mental-model/workflow-skills.md` (left for the doc
  pre-pass stage), no changes to the `.done` design ticket.

## Escalations

- None.
