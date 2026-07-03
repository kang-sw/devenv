# Plan: 260703-chore-prefer-subagent-verify-discussion-inline-mirror — Phase 1: Inline both skill bodies, repoint the manual-embed consumer

## Relevant Ticket Contract

- Rewrite `agents-plugin/skills/lead-prefer-subagent/SKILL.md`,
  `agents-plugin/skills/lead-verify-discussion/SKILL.md`, and their
  `agents-plugin-wsflow/skills/...` counterparts with the full procedure body
  inlined as prose, sourced from the current rsrc `.md` files.
- `lead-prefer-subagent`: drop the `<!-- ws:override:PreferSubagentInvocationGuidance -->`
  marker block entirely; replace with static host-conditional prose ("if your
  host provides a fork-style subagent that inherits current context, use that;
  otherwise use a fresh-context spawn primitive such as Codex's `spawn_agent`").
  Accepted regression: no runtime override surface for this point anymore.
- Delete now-dead Go symbols: `preferSubagentInvocationGuidancePointID`,
  `preferSubagentCodexInvocationGuidancePrompt`, and their coverage in
  `prompt_override_test.go`.
- `delegates: true` continuity tip (only `lead-verify-discussion` has
  `delegates: true`; `lead-prefer-subagent` does not) becomes a hardcoded line
  per product (full ws includes the mercenary-path line; wsflow omits it).
- **Go-plumbing sub-step** (prerequisite for deleting rsrc playbooks):
  - `wsrsrc/loader.go`: add `ResolveSkillsRoot()` parallel to `ResolveRoot()`
    — checks `WS_SKILLS_ROOT` env var, falls back to a derived path sibling to
    the resolved rsrc root.
  - Add `LoadSkillBody(root, name string) (string, error)` — resolves
    `<root>/<name>/SKILL.md`, reads it, reuses `parseFrontmatter()` to strip
    frontmatter, returns body only. No override-marker pass applies.
  - `mcp/playbook_tools.go` `printPlaybook()`: repoint the
    `lead-prefer-subagent` manual-embed call site (~line 806,
    `renderPlaybookBody(..., preferSubagentPlaybookName, ...)`) to
    `LoadSkillBody(skillsRoot, "lead-prefer-subagent")`.
  - Add `agents-plugin/skills/manifest.json`, generated via
    `wsrsrc.GenerateManifest` pointed at `skills/` instead of `rsrc/` — a
    parallel, independent mechanism, not an extension of the rsrc manifest
    schema. Needs its own regen entrypoint (mirror `TestGenerateRealManifest`)
    and its own drift test.
  - Only after the embed call site is repointed and passing: delete
    `agents-plugin/rsrc/lead-prefer-subagent/`,
    `agents-plugin/rsrc/lead-verify-discussion/`, and their
    `agents-plugin-wsflow/rsrc/...` mirrors; remove references to them as
    `playbook.print` shims from `ai-docs/ref/wsflow-mirroring.md` and any
    package-test exemption sets.
  - Update `ai-docs/spec/workflow-skills.md` `{#260505-workflow-primitive-reference}`
    to describe the new embed source (static `SKILL.md` body via
    `LoadSkillBody`, no override-marker pass, no per-harness runtime branch)
    and drop the now-inaccurate "harness-scoped rendering" claim.
- **Verification / acceptance for Phase 1**:
  - `grep` confirms no remaining references to the deleted rsrc playbooks or
    deleted override-marker Go symbols anywhere in the tree; `go build` and
    `go vet` pass on `agents-plugin-tool/`.
  - With global `workflow.prefer_subagent: on`, loading `lead-workflow-manual`
    still embeds the (now static) prefer-subagent text end-to-end, exercised
    via the actual `printPlaybook()` path, not just reading source.
  - `python3 -m unittest discover agents-plugin-wsflow/tests` still passes
    after the wsflow skill files change shape.

## Out of Scope

- Phase 2 (substitution-mirrored skill generation script, hard-gate
  eligibility guard, drift test, `ai-docs/ref/wsflow-mirroring.md` curated
  list registration) — explicitly a separate phase.
- Whether other prose/behavior-mode skills migrate into this category.
- Changes to `lead-implement` or other sequence-strict `playbook.print`-backed
  skills.

## Codebase Findings

- `agents-plugin/skills/lead-prefer-subagent/SKILL.md` and
  `agents-plugin/skills/lead-verify-discussion/SKILL.md` — current thin shims
  (single `ws/playbook.print(name: "...")` call + "execute inline" line).
  Same shape in `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md`
  and `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` but with
  `wsflow/playbook.print` and an extra "If the playbook cannot be loaded, stop
  and report that blocker." sentence (full-ws prefer-subagent shim lacks that
  sentence; verify-discussion shim in full-ws also lacks it — wsflow shims are
  slightly more verbose already).
- `agents-plugin/rsrc/lead-prefer-subagent/lead-prefer-subagent.md` — full
  source body to inline (frontmatter `kind: print`, no `delegates`). Contains
  the `<!-- ws:override:PreferSubagentInvocationGuidance desc="..." -->` /
  `<!-- ws:/override:PreferSubagentInvocationGuidance -->` empty marker pair
  (Claude's slot is empty; only Codex gets injected text) — this pair must be
  replaced with static host-conditional prose per Decisions.
- `agents-plugin/rsrc/lead-verify-discussion/lead-verify-discussion.md` —
  full source body to inline (frontmatter `kind: print`, `delegates: true`).
  No override markers present.
- `agents-plugin-wsflow/rsrc/lead-prefer-subagent/lead-prefer-subagent.md` and
  `.../lead-verify-discussion/lead-verify-discussion.md` — byte-identical to
  the ws copies (verified). Confirms namespace difference is render-time only,
  not stored-file difference, so wsflow body text needs the same content with
  `ws/` → `wsflow/`, `ws:` → `wsflow:` substitution applied by hand for this
  phase (Phase 2 automates it later).
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L350-L369` — dead-code
  targets: `preferSubagentInvocationGuidancePointID` (const),
  `preferSubagentCodexInvocationGuidancePrompt` (const),
  `builtinPromptOverrideDefaults()` (currently only returns this one entry —
  becomes an empty map, but keep the function since `buildOverrideLookup`
  (`#L420-L430`) and `server.go#L328` (`builtinConfigAndPromptDefaults`) both
  call it generically for other override points; do not delete the function
  itself, only its `PreferSubagentInvocationGuidance` entry).
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L780-L812` —
  `printPlaybook()`: the `name != workflowManualPlaybookName` early-return
  guard and the `workflowPreferSubagentEnabled` check stay; only the append
  call at `#L806-L810` changes from `renderPlaybookBody(..., preferSubagentPlaybookName, ...)`
  to `LoadSkillBody(skillsRoot, "lead-prefer-subagent")`. Need a `skillsRoot`
  parameter threaded into `printPlaybook` (or resolved inside via
  `wsrsrc.ResolveSkillsRoot()` — check call sites of `printPlaybook` in
  `server.go` to decide whether to thread it as a param or resolve locally).
  `wrapRenderedPlaybookForConcatenation` (`#L761-L769`) is reused unchanged
  for the boundary wrap since it only takes name/title/body strings.
- `agents-plugin-tool/internal/wsrsrc/loader.go#L31-L40` — `ResolveRoot()`
  pattern to mirror for `ResolveSkillsRoot()`: env var first
  (`WS_RSRC_ROOT` → new `WS_SKILLS_ROOT`), then `os.Executable()`-derived
  sibling path. `agents-plugin-tool/internal/wsrsrc/frontmatter.go#L20-L86`
  — `parseFrontmatter()` is the reusable frontmatter/body splitter;
  `LoadSkillBody` should call this directly instead of going through the
  full manifest-verified `Load()` path (skills are not manifest/hash
  integrity-checked the way rsrc playbooks are — confirm this is acceptable;
  ticket text explicitly says "reuses the existing `parseFrontmatter()`... No
  override-marker pass applies," implying a lighter-weight read, not full
  `wsrsrc.Load`).
- `agents-plugin-tool/internal/wsrsrc/manifest.go#L35-L83` —
  `GenerateManifest(root)` / `WriteManifest(root, m)` are root-parametric
  already; reusable as-is pointed at `agents-plugin/skills/` for the new
  `skills/manifest.json`. Mirror pattern:
  `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L894-L911`
  (`TestGenerateRealManifest`, gated by `WSRSRC_REGEN=1`) for the new skills
  manifest regen entrypoint and drift test (`TestValidateRealTree`-equivalent
  at `#L884-L892` uses `Validate(root)` — check whether `Validate` assumes
  rsrc-specific structure before reusing it for `skills/`; if incompatible,
  a simpler own drift check comparing `GenerateManifest` output to committed
  `manifest.json` suffices).
- `agents-plugin-tool/internal/mcp/prompt_override_test.go#L976-L1113` —
  three tests reference the doomed marker: `TestConfigPromptListIncludesShippedUserPreferenceSection`
  (asserts `"PreferSubagentInvocationGuidance"` and its desc string appear in
  `config.prompt` output, and that `"PreferSubagentCodexBinding"` does NOT
  appear — this whole assertion block needs removal since the marker is
  gone), `TestConfigTuningShippedPromptKnobsOmitDelegationSection#L1043`
  (asserts `requireTuningKnob(t, catalog, "prompt.PreferSubagentInvocationGuidance")`
  — needs removal), and `TestLeadPreferSubagentInvocationGuidanceUsesCodexBuiltinPromptOverride#L1049-L1113`
  (the whole test function exercises Codex vs Claude override-marker
  rendering divergence for this exact point — must be deleted entirely, not
  just tweaked, since the behavior it tests no longer exists post-inlining).
- `agents-plugin-tool/internal/mcp/server.go#L319-L332` —
  `builtinConfigAndPromptDefaults()` merges `builtinConfigDefaults()` +
  `builtinPromptOverrideDefaults()`; no change needed here beyond the source
  map shrinking to empty for this point (function stays, used by other
  override points too — confirmed no other caller depends on this specific
  key).
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L371-L408` (`scanOverridePoints`)
  and `#L437-L520` (`applyOverrideMarkers`) — generic override-marker
  machinery that must NOT be deleted; it's shared with other override points
  (e.g. `UserPreferenceSection` per `prompt_override_test.go#L999-L1001`).
  Only the `PreferSubagentInvocationGuidance`-specific constants/wiring are
  dead after this change; the marker syntax scanner/applier stays live for
  other points.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L16-L37` —
  `EXPECTED_SKILLS` includes both `lead-prefer-subagent` and
  `lead-verify-discussion`. `EXPECTED_INLINE_SKILLS = {"lead-revive"}`
  (`#L40`) is the exemption set for `test_skill_files_are_thin_playbook_shims`
  (`#L93-L111`, regex-matches the exact thin-shim shape) and
  `test_skill_shims_point_to_shared_playbooks` (`#L135-L142`, checks a
  matching rsrc playbook dir/file exists for every non-inline skill). **Risk
  signal**: both tests will fail after Phase 1 unless
  `lead-prefer-subagent` and `lead-verify-discussion` are added to
  `EXPECTED_INLINE_SKILLS`, since (a) the shim regex no longer matches once
  bodies are inlined, and (b) their rsrc playbook dirs are deleted in this
  same phase. This Python test file is in scope for Phase 1 even though the
  ticket text doesn't explicitly name it — it directly enforces the "thin
  shim" invariant this phase overturns, and Phase 1's own acceptance
  criterion ("`python3 -m unittest discover agents-plugin-wsflow/tests` still
  passes") requires this edit.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L43-L53`
  (`FORBIDDEN_PATTERNS`) — inlined `lead-prefer-subagent` wsflow body must not
  contain `ws/`, `ws:`, `ws.`, `subquery`, `agents.`, or the four excluded
  skill names. Verified: current rsrc source text contains none of these
  literal tokens (checked via grep) — the word "mercenary" appears only in
  the ticket's Decisions text about the full-ws-only continuity line, not in
  the current rsrc body, so nothing to strip there for wsflow. When drafting
  the wsflow `SKILL.md` bodies, apply `ws/` → `wsflow/`, `ws:` → `wsflow:`
  substitution and omit the mercenary-path line per the existing
  `delegationTip` product-mode split (see next finding).
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L195-L215`
  (`delegationTip`) — confirms only `lead-verify-discussion` has
  `delegates: true` (checked both rsrc frontmatters: prefer-subagent has no
  `delegates` key, verify-discussion has `delegates: true`). The appended tip
  text (continuity-tip paragraph, always; mercenary-path paragraph, only when
  `!NoAgentMode()`) is what must become a hardcoded literal line in
  `lead-verify-discussion/SKILL.md` (full ws: both paragraphs; wsflow: only
  the continuity-tip paragraph, no mercenary paragraph) since wsflow runs in
  `NoAgentMode()`.
- `ai-docs/ref/wsflow-mirroring.md#L60-L69` — current rule states shipped
  `agents-plugin-wsflow/skills/lead-*` files must stay "thin
  `wsflow/playbook.print(name: "<lead-name>")` entry shims... do not place
  procedure bodies there," with `lead-revive` as the sole "inline-body
  exception." Phase 1 makes `lead-prefer-subagent`/`lead-verify-discussion`
  a second and third inline-body exception ahead of Phase 2's formal curated
  "substitution-mirrored" category. The doc's "Shipped wsflow Skills" list
  (`#L29-L58`) and inline-exception note (`#L67-L69`) need a Phase 1 update
  noting these two are now inline bodies too (even though the ticket frames
  the curated-list *naming* as Phase 2 work, the doc will misdescribe reality
  the moment Phase 1 lands unless this line is touched — flagged as a
  detail the ticket's Phase 1 prose doesn't explicitly call out, but Phase
  1's own acceptance bullet about `ai-docs/ref/wsflow-mirroring.md` references
  covers removing the stale "playbook.print shim" references for these two).
- `ai-docs/spec/workflow-skills.md#L87-L99` (`{#260505-workflow-primitive-reference}`)
  — exact spec prose to update per Spec Impact: sentence "The appended
  posture is rendered through the normal playbook pipeline so harness-specific
  defaults, including Codex invocation guidance, remain harness-scoped." must
  be replaced to describe the new static `LoadSkillBody`-sourced embed with no
  per-harness runtime branch.
- `ai-docs/mental-model/workflow-skills.md#L37-L38` — parallel mental-model
  lines describing the same embed/override-slot behavior; not explicitly
  named in ticket's Spec Impact (which only names the spec file), but
  contains the same now-inaccurate claim ("Codex gets a builtin
  `spawn_agent(fork_context:true, ...)` default for that slot while Claude
  keeps the empty shared seed behavior") — flag for lead judgment on whether
  "update drifted docs on contact" (AGENTS.md Context Window Discipline)
  requires touching this file in the same change; it is not in the ticket's
  explicit Spec Impact scope.

## Implementation Plan

1. Draft inlined bodies for all four `SKILL.md` files, sourcing prose from
   the two rsrc `.md` files, in a scratch location first for review:
   - `agents-plugin/skills/lead-prefer-subagent/SKILL.md` — inline body from
     `agents-plugin/rsrc/lead-prefer-subagent/lead-prefer-subagent.md`,
     replacing the override marker pair with static host-conditional prose
     (fork-style-if-available / Codex `spawn_agent` fallback wording).
   - `agents-plugin/skills/lead-verify-discussion/SKILL.md` — inline body
     from `agents-plugin/rsrc/lead-verify-discussion/lead-verify-discussion.md`,
     appending the full-ws continuity-tip + mercenary-path lines (mirroring
     `delegationTip(harness)` non-`NoAgentMode` output) as literal text.
   - `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` — same body,
     `ws/`→`wsflow/`, `ws:`→`wsflow:` substitution applied by hand.
   - `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` — same
     body, namespace-substituted, with only the continuity-tip paragraph
     appended (no mercenary-path paragraph, matching `NoAgentMode()` product
     behavior).
2. Go plumbing (`agents-plugin-tool/internal/wsrsrc/loader.go` or new
   `wsrsrc/skill.go`):
   - Add `ResolveSkillsRoot()` mirroring `ResolveRoot()`
     (`loader.go#L31-L40`): `WS_SKILLS_ROOT` env var, else
     `os.Executable()`-derived sibling `skills/` path.
   - Add `LoadSkillBody(root, name string) (string, error)`: read
     `<root>/<name>/SKILL.md`, call `parseFrontmatter()` (`frontmatter.go`),
     return body only, normalized per that function's existing LF behavior.
3. `agents-plugin-tool/internal/mcp/playbook_tools.go`:
   - Repoint the `printPlaybook()` append call (`#L806-L810`) from
     `renderPlaybookBody(..., preferSubagentPlaybookName, ...)` to
     `wsrsrc.LoadSkillBody(skillsRoot, "lead-prefer-subagent")`; resolve
     `skillsRoot` via `wsrsrc.ResolveSkillsRoot()` inside `printPlaybook` (no
     override_root seam requested by the ticket for skills, unlike rsrc root).
   - Remove `preferSubagentInvocationGuidancePointID` (`#L350`),
     `preferSubagentCodexInvocationGuidancePrompt` (`#L359-L363`), and their
     entry inside `builtinPromptOverrideDefaults()` (`#L365-L369`) — keep the
     function itself (used generically elsewhere), just drop this one map
     entry / the constants backing it.
4. `agents-plugin-tool/internal/mcp/prompt_override_test.go`:
   - Delete `TestLeadPreferSubagentInvocationGuidanceUsesCodexBuiltinPromptOverride`
     (`#L1049-L1113`) in full.
   - In `TestConfigPromptListIncludesShippedUserPreferenceSection`
     (`#L979-L1019`), remove the `"PreferSubagentInvocationGuidance"` /
     `"harness-specific forked subagent invocation guidance"` want-substrings
     and the `"PreferSubagentCodexBinding"` forbidden-check (the point no
     longer exists so neither assertion applies).
   - In `TestConfigTuningShippedPromptKnobsOmitDelegationSection`
     (`#L1021-L1047`), remove the
     `requireTuningKnob(t, catalog, "prompt.PreferSubagentInvocationGuidance")`
     line.
5. Add `agents-plugin/skills/manifest.json` generation:
   - New test (mirroring `TestGenerateRealManifest`,
     `wsrsrc_test.go#L894-L911`) gated by an env var (reuse `WSRSRC_REGEN=1`
     or introduce a skills-specific one if collision with the existing rsrc
     regen test on the same env var is undesirable — prefer a distinct var,
     e.g. `WSRSRC_REGEN_SKILLS=1`, to avoid accidentally regenerating both
     from one flag) that calls `wsrsrc.GenerateManifest(skillsRoot)` +
     `wsrsrc.WriteManifest(skillsRoot, m)` pointed at
     `agents-plugin/skills/`.
   - Add a drift test (mirroring `TestValidateRealTree`,
     `wsrsrc_test.go#L884-L892`) that fails if `skills/manifest.json` is
     stale; check whether `wsrsrc.Validate()` needs a rsrc-shape-agnostic
     path or whether a simpler direct `GenerateManifest` vs. on-disk-JSON
     comparison is more appropriate for the skills tree (skills tree has no
     harness-overlay files, includes, or `kind:`/`delegates:` frontmatter
     semantics the rsrc `Validate()` may assume — read `validate.go` before
     reusing wholesale).
   - Run the regen entrypoint once to produce the initial committed
     `agents-plugin/skills/manifest.json`.
6. Delete rsrc trees only after steps 2-3 build and the manual-embed
   integration check (Verification Plan) passes:
   - `agents-plugin/rsrc/lead-prefer-subagent/`
   - `agents-plugin/rsrc/lead-verify-discussion/`
   - `agents-plugin-wsflow/rsrc/lead-prefer-subagent/`
   - `agents-plugin-wsflow/rsrc/lead-verify-discussion/`
7. Update `ai-docs/ref/wsflow-mirroring.md`:
   - `#L29-L58` "Shipped wsflow Skills" list: no membership change (both
     names stay listed) but note near `#L67-L69` that
     `lead-prefer-subagent`/`lead-verify-discussion` are now inline-body
     skills too (temporary state ahead of Phase 2's formal curated
     "substitution-mirrored" category — do not pre-write the Phase 2 section
     here, only correct the now-false "thin shim" claim for these two).
   - Remove references treating these two as `playbook.print`-backed rsrc
     shims wherever the doc currently implies that (cross-check the
     "Rsrc Tree Provisioning" and "After-edit checklist" sections do not
     name these two specifically — spot-checked, they don't; no edit needed
     there).
8. Update `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`:
   - Add `"lead-prefer-subagent"` and `"lead-verify-discussion"` to
     `EXPECTED_INLINE_SKILLS` (`#L40`).
   - Confirm `test_full_skill_inventory_drift_is_visible` (`#L61-L80`) still
     passes: once these two are in `EXPECTED_INLINE_SKILLS`, they're excluded
     from `missing_full_counterparts` requiring an rsrc dir — correct, since
     their full-ws counterpart becomes `agents-plugin/skills/<name>/` (a
     skill dir, not an rsrc dir) which already exists structurally.
9. Update `ai-docs/spec/workflow-skills.md` `{#260505-workflow-primitive-reference}`
   (`#L93-L99`): replace the "rendered through the normal playbook pipeline
   so harness-specific defaults... remain harness-scoped" sentence with
   language describing the static `LoadSkillBody`-sourced embed and the loss
   of per-harness runtime branching for this specific append.
10. Consider (lead judgment, not strictly ticket-scoped) updating
    `ai-docs/mental-model/workflow-skills.md#L37-L38` to match — flagged in
    Codebase Findings; align with the spec edit if the lead decides
    AGENTS.md's "update drifted docs on contact" applies here.
11. `go build ./...` and `go vet ./...` in `agents-plugin-tool/`; `grep -rn`
    for `preferSubagentInvocationGuidancePointID`,
    `preferSubagentCodexInvocationGuidancePrompt`,
    `lead-prefer-subagent/lead-prefer-subagent.md`,
    `lead-verify-discussion/lead-verify-discussion.md` across the repo to
    confirm zero remaining references.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./...`
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... ./internal/mcp/...`
  (full suite, not just touched tests, to catch any other reference to the
  deleted symbols or rsrc paths)
- Exercise the actual embed path end-to-end (not just source reading): a Go
  test (new or reused from `prompt_override_test.go`'s harness) that sets
  `workflow.prefer_subagent: on`, calls `printPlaybook(s, rsrcRoot, "lead-workflow-manual", ...)`,
  and asserts the response contains the now-static `lead-prefer-subagent`
  skill body text wrapped in the `<playbook name="lead-prefer-subagent" ...>`
  boundary (reuses `wrapRenderedPlaybookForConcatenation` output shape,
  unchanged).
- `grep -rn "preferSubagentInvocationGuidancePointID\|preferSubagentCodexInvocationGuidancePrompt" agents-plugin-tool/` → no results.
- `grep -rln "rsrc/lead-prefer-subagent\|rsrc/lead-verify-discussion" .` (repo root) → no results after deletion + doc updates.
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- Manual read-through of all four new `SKILL.md` bodies against
  `FORBIDDEN_PATTERNS` in `test_wsflow_skill_bundle.py#L43-L53` for the two
  wsflow files (already covered by the unittest run, called out for
  visibility since it's the main mis-substitution risk).

## Escalations

- None — survey found concrete, prescriptive ticket text matching concrete
  codebase evidence at every decision point; no strategy or contract
  ambiguity remains for Phase 1. One judgment call is flagged (mental-model
  doc update, item 10) but it does not block safe execution — it can be
  decided inline by the lead/executor without a separate research pass.
