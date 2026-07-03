---
title: "Inline lead-prefer-subagent and lead-verify-discussion; add substitution-mirrored skill generation"
related:
  260630-epic-skill-playbook-diet: adjacent skill/playbook-boundary work, opposite direction (inlining vs dieting) on the same skill-authoring surface
  260610-chore-wsflow-explore-playbook-mirroring: prior deferred wsflow-mirroring-mechanism discussion, related precedent for narrow generated-skill carve-outs
spec:
  - 260505-workflow-primitive-reference
sage-review: completed
---

# Inline lead-prefer-subagent and lead-verify-discussion; add substitution-mirrored skill generation

## Background

`lead-prefer-subagent` and `lead-verify-discussion` are currently thin
`SKILL.md` shims that call `ws/playbook.print` (or `wsflow/playbook.print`)
and execute the returned rsrc procedure inline. Both are "behavior-mode
reminder" prose — posture/checkpoint guidance — not sequence-strict
procedural skills like `lead-implement`, which must stay MCP/playbook-backed.

Their current shim is a single unconditional call with no eager/parallel
fallback: "Call `ws/playbook.print(name: ...)` and execute the returned
procedure inline." This shape is structurally prone to being skipped by the
model, especially for reminder-style content the model may believe it
"already knows" — a risk distinct from, and higher than, deterministic
procedural skills the model doesn't believe it can already predict. This
mirrors a documented failure pattern in
`ai-docs/mental-model/workflow-skills.md` (`{#260505-workflow-primitive-reference}`):
"Skipping the manual load causes notation drift and wrong agent-call forms,"
which is why `lead-discuss`/`lead-sprint` already load `workflow_manual`
eagerly in parallel in their entry shim rather than relying on an in-body
call. Inlining the full procedure into `SKILL.md` removes the skip
opportunity entirely, since Skill-tool invocation loads the `SKILL.md` body
into context directly with no separate tool-call step to omit.

This is explicitly **not** motivated by reducing duplicate-maintenance
burden: the existing rsrc mirroring mechanism (`WS_REGEN_WSFLOW_RSRC`,
byte-identical body copy, guarded by `TestWsflowRsrcMirrorUpToDate`) already
gives playbook-print-backed content a single edit point with automated
mirroring. Removing `playbook.print` for these two skills trades that
existing automation away in exchange for call-skip reliability, so a new,
narrower mirroring mechanism is needed to avoid reintroducing hand-duplicated
maintenance at the `SKILL.md` layer.

Sage design review (first pass) blocked this ticket on a missed consumer:
`lead-workflow-manual`'s keyless rendering path embeds the
`lead-prefer-subagent` rsrc playbook body into its own output when global
`workflow.prefer_subagent` is `on` (`agents-plugin-tool/internal/mcp/playbook_tools.go`,
the `printPlaybook()` call site around line 806 using
`preferSubagentPlaybookName`/`renderPlaybookBody`; documented behavior in
`ai-docs/spec/workflow-skills.md` `{#260505-workflow-primitive-reference}` and
`ai-docs/mental-model/workflow-skills.md` line 37). Deleting the rsrc playbook
in Phase 1 without repointing this consumer would silently break that embed.
Resolved by switching the embed's source to the new `SKILL.md`-based skills
tree instead of keeping the rsrc file alive as a second copy — see the
**Go-plumbing sub-step** under Phase 1. Confirmed via source check: unlike
`lead-prefer-subagent`, `lead-verify-discussion` has no analogous
keyless-embed consumer anywhere in `agents-plugin-tool/internal/mcp/` (no
`verify-discussion`/`VerifyDiscussion` references outside its own playbook
file), so this repointing work applies only to `lead-prefer-subagent`; Phase 1
is asymmetric between the two skills for this sub-step.

## Decisions

- Move both skill bodies out of `ws/playbook.print` indirection into inline
  prose directly in `SKILL.md`, in both `agents-plugin/skills/<name>/` and
  `agents-plugin-wsflow/skills/<name>/`.
- `lead-prefer-subagent` currently resolves a harness-axis (Claude vs Codex)
  override marker (`<!-- ws:override:PreferSubagentInvocationGuidance -->`)
  dynamically per harness via `config.prompt.<pointId>.<harness>` (Codex gets
  `spawn_agent(fork_context:true, ...)` guidance; Claude gets an empty,
  self-determined slot). This becomes static host-conditional prose in the
  inlined body (e.g. "if your host provides a fork-style subagent that
  inherits current context, use that; otherwise use a fresh-context spawn
  primitive such as Codex's `spawn_agent`"). Accepted as a known regression:
  the `config.prompt.set`-based per-session/per-project tuning of this point
  is lost once inlined — there is no runtime override surface for static
  `SKILL.md` text. The override-marker Go plumbing that only existed to serve
  this point — `preferSubagentInvocationGuidancePointID`,
  `preferSubagentCodexInvocationGuidancePrompt`, and their coverage in
  `agents-plugin-tool/internal/mcp/prompt_override_test.go` — becomes dead
  code once the marker is gone from the source and must be deleted in Phase 1,
  not left behind.
- Both skills currently get a `delegates:true`-driven continuity tip appended
  by `playbook.print`/`render`, whose content differs by product (full ws
  includes a mercenary-path line; wsflow omits it). Once inlined, this
  becomes a literal hardcoded line duplicated by hand across the two
  per-product `SKILL.md` files, matching the existing product-specific
  behavior.
- New "substitution-mirrored skill" generation category:
  - Source of truth: `agents-plugin/skills/<name>/SKILL.md`. A generation
    script derives `agents-plugin-wsflow/skills/<name>/SKILL.md` via literal
    namespace substitution (`ws:` → `wsflow:`, `ws/` → `wsflow/`).
  - **Hard-gate eligibility guard, effective immediately (not phased in)**:
    generation fails loudly if the source body contains anything beyond
    namespace-only tokens — e.g. the word "mercenary", `<!-- ws:full-only:...
    -->`/`<!-- ws:wsflow-only:... -->` markers, literal names of the
    wsflow-excluded skills (`lead-write-code`, `lead-write-skeleton`,
    `lead-salvage`, `lead-skill-authoring`), or other product-specific
    content a blind substitution would mishandle. False positives from this
    guard are fixed reactively as they appear, not pre-verified
    exhaustively up front, so the guard should default to strict/conservative
    rather than a curated exception list.
  - A drift test analogous to `TestWsflowRsrcMirrorUpToDate` (in
    `internal/wsrsrc`) verifying the generated wsflow `SKILL.md` is up to
    date with its source, wired into the same regen-check pattern (including
    the `-count=1` test-cache gotcha — see `ai-docs/ref/wsflow-mirroring.md`
    for the pattern to mirror).
  - **Resolved** (was left open in the first draft): the curated list of
    skills opted into this category lives directly in
    `ai-docs/ref/wsflow-mirroring.md` as a new explicit section, reusing the
    existing required-reading doc for anyone touching mirrored skills rather
    than adding a second manifest file for callers to discover.
  - Scope is narrow: only skills **explicitly and deliberately migrated out
    of `playbook.print`** are eligible (currently just these two) — this is
    not a blanket auto-mirror for all skills. The new
    `ai-docs/ref/wsflow-mirroring.md` section must state plainly that this
    category applies *only* to skills explicitly named in its curated list
    and does not relax or reinterpret the document's existing rule that all
    other skills remain curated/human-reviewed, not auto-mirrored. Phrase it
    as an additive, bounded exception (parallel in spirit to the existing
    rsrc "Generated-sameness carve-out" section, but a separate section, not
    an edit to that one) so a future reader cannot mistake it for a general
    loosening of the skill-mirroring rule.

## Spec Impact

`ai-docs/spec/workflow-skills.md` `{#260505-workflow-primitive-reference}`
already documents the current `lead-workflow-manual` embed behavior ("The
appended posture is rendered through the normal playbook pipeline so
harness-specific defaults, including Codex invocation guidance, remain
harness-scoped"). Phase 1 changes the embed mechanism (static `SKILL.md`
body via `LoadSkillBody`, no override-marker pass, no per-harness runtime
branch), so this existing stem is the addressed spec and must be updated in
the same logical change as Phase 1 to describe the new embed source and drop
the now-inaccurate harness-scoped-rendering claim. Contract-first spec: no —
this is a closeout update to an existing documented contract, not a new
planned contract authored ahead of implementation.

## Out of Scope

- Whether other prose/behavior-mode skills should eventually migrate into
  this same substitution-mirrored category — not decided. This ticket covers
  only the `lead-prefer-subagent` + `lead-verify-discussion` pilot and the
  mechanism needed to support it.
- Changing `lead-implement` or other sequence-strict procedural skills' use
  of `playbook.print`.

## Phases

### Phase 1: Inline both skill bodies, repoint the manual-embed consumer

Rewrite `agents-plugin/skills/lead-prefer-subagent/SKILL.md`,
`agents-plugin/skills/lead-verify-discussion/SKILL.md`, and their
`agents-plugin-wsflow/skills/...` counterparts with the full procedure body
inlined as prose (sourced from the current
`agents-plugin/rsrc/lead-prefer-subagent/lead-prefer-subagent.md` and
`agents-plugin/rsrc/lead-verify-discussion/lead-verify-discussion.md`
content), applying the harness-conditional prose and per-product continuity
line described in Decisions, and dropping the override-marker comment block
from the source entirely (its Go-side plumbing is deleted per Decisions).

**Go-plumbing sub-step** (prerequisite for safely deleting the rsrc
playbooks — not a separate phase):

- `wsrsrc/loader.go`: add `ResolveSkillsRoot()` (parallel to the existing
  `ResolveRoot()`) — checks a new `WS_SKILLS_ROOT` env var, falls back to a
  derived path sibling to the resolved rsrc root.
- `wsrsrc/loader.go` or a new `wsrsrc/skill.go`: add
  `LoadSkillBody(root, name string) (string, error)` — resolves
  `<root>/<name>/SKILL.md`, reads it, reuses the existing `parseFrontmatter()`
  (`wsrsrc/frontmatter.go`) to strip frontmatter, and returns the body only.
  No override-marker pass applies to this content going forward.
- `mcp/playbook_tools.go`: in `printPlaybook()`, change the
  `lead-prefer-subagent` manual-embed call site (currently
  `renderPlaybookBody(..., preferSubagentPlaybookName, ...)` around line 806)
  to call `LoadSkillBody(skillsRoot, "lead-prefer-subagent")` instead.
- Delete the now-dead override-marker symbols and their test coverage:
  `preferSubagentInvocationGuidancePointID`,
  `preferSubagentCodexInvocationGuidancePrompt`, and the now-inapplicable
  assertions in `agents-plugin-tool/internal/mcp/prompt_override_test.go`.
- **Integrity**: add a separate `agents-plugin/skills/manifest.json`,
  generated by reusing the existing `wsrsrc.GenerateManifest` tooling pointed
  at the `skills/` root instead of `rsrc/` — an independent, parallel
  mechanism mirroring the existing `rsrc/manifest.json` pattern, not an
  extension of the rsrc manifest schema. Needs its own regen entrypoint
  (analogous to `WSRSRC_REGEN=1 ... TestGenerateRealManifest`) and its own
  up-to-date drift test.
- Only after the embed call site is repointed and passing: delete
  `agents-plugin/rsrc/lead-prefer-subagent/` and
  `agents-plugin/rsrc/lead-verify-discussion/` (and their
  `agents-plugin-wsflow/rsrc/...` mirrors), and remove references to them as
  `playbook.print` shims from `ai-docs/ref/wsflow-mirroring.md` and any
  package-test exemption sets (e.g. check `EXPECTED_INLINE_SKILLS`-style
  sets and the shipped wsflow skill list).
- Update `ai-docs/spec/workflow-skills.md` `{#260505-workflow-primitive-reference}`
  per Spec Impact above.

**Verification / acceptance for Phase 1**:

- `grep` confirms no remaining references to the deleted rsrc playbooks or
  the deleted override-marker Go symbols anywhere in the tree; `go build` and
  `go vet` pass on `agents-plugin-tool/`.
- With global `workflow.prefer_subagent: on`, loading `lead-workflow-manual`
  still embeds the (now static) prefer-subagent text end-to-end, confirmed by
  exercising the actual `printPlaybook()` path, not just reading the new
  source.
- `python3 -m unittest discover agents-plugin-wsflow/tests` still passes
  after the wsflow skill files change shape.

### Result

Implemented on `implement/inline-skill-bodies-repoint-embed`
(`f9b8e0c7^..c8e824f4`, 5 commits). Both skill bodies inlined into
`agents-plugin/skills/<name>/SKILL.md` and `agents-plugin-wsflow/skills/<name>/SKILL.md`;
`wsrsrc.ResolveSkillsRoot()`/`LoadSkillBody()` added; `printPlaybook()`'s
manual-embed call site repointed from `renderPlaybookBody`(rsrc) to
`LoadSkillBody`(skills); override-marker Go symbols and their test coverage
deleted; independent `agents-plugin/skills/manifest.json` + drift test added;
rsrc trees for both skills deleted (ws and wsflow mirrors). Spec
(`{#260505-workflow-primitive-reference}`) and mental-model docs updated to
describe the static `LoadSkillBody` embed.

Partitioned review: correctness clean, fit clean, test non-clean (1 important
— `ResolveSkillsRoot()` executable-path fallback branch untested), fixed in
`a7996fed` with `TestResolveSkillsRootEnv`/`TestResolveSkillsRootFallsBackToExecutablePath`
mirroring the `ResolveRoot()` precedent. All verification/acceptance criteria
above independently re-confirmed by the test reviewer, including an
end-to-end `printPlaybook()` exercise and a negative-path drift-test check
(tampered manifest hash correctly caught).

Plugin version bumped 0.32.0 → 0.32.1 (dev-merge rule) in `c8e824f4`.

### Phase 2: Substitution-mirrored skill generation mechanism

**Edition (lead-resolved) - 2026-07-03**: Phase 2's survey found that the
already-merged Phase 1 `lead-verify-discussion/SKILL.md` contained an unmarked
"mercenary" paragraph, which the guard wording above would reject outright —
a direct conflict with this being one of exactly two initial eligible skills.
Superseded resolution: rather than carving a marker exception into the guard,
the user (on review) concluded the `delegates:true`-driven continuity-tip and
mercenary-path paragraphs were never a good fit for this skill in the first
place — `lead-verify-discussion`'s own delegation is conditional
("when investigation is useful"), unlike `lead-verify-design`'s unconditional
"isolate a fresh deep reviewer", and the sibling checkpoint
`lead-check-blockers` (same "compact, lightweight checkpoint" framing) never
carried `delegates:true` at all. Both paragraphs were removed outright from
`agents-plugin/skills/lead-verify-discussion/SKILL.md` and
`agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` (the latter's
existing hand-authored copy already omitted the mercenary line; it now also
drops the continuity-tip line, making the two files byte-identical). This
dissolves the guard conflict entirely: the source contains no
product-specific content, so no marker mechanism is needed and Phase 2
proceeds as originally specified (pure namespace substitution, no exception
carve-out). A separate `todo/` ticket
(`260703-chore-review-delegates-true-classification`) covers a broader review
of `delegates:true` usage across all rsrc playbooks, sage-review skipped.

Implement the generation script, the hard-gate eligibility guard, and the
drift test described in Decisions. Register `lead-prefer-subagent` and
`lead-verify-discussion` as the initial (and currently only) entries in the
curated list in `ai-docs/ref/wsflow-mirroring.md`, worded as a bounded
exception per Decisions.

**Verification / acceptance for Phase 2**:

- The drift test (analogous to `TestWsflowRsrcMirrorUpToDate`) passes against
  the checked-in generated `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md`
  and `.../lead-verify-discussion/SKILL.md`.
- The eligibility guard's negative path is actually exercised, not just
  asserted to exist: a deliberately-broken fixture (e.g. a registered source
  file containing the word "mercenary" or a `ws:full-only` marker) must make
  the guard test fail, proving the guard actually rejects disqualifying
  content rather than only running on already-clean input.

### Result

Implemented on `implement/substitution-mirrored-skill-generation`
(`864902a3..0aaacd01`, 3 commits). Added `GenerateWsflowSkillBody` in
`agents-plugin-tool/internal/wsrsrc/skills_mirror.go`: reads the full raw
`SKILL.md` (frontmatter included), guards against disqualifying tokens
(`mercenary`, `ws:full-only`/`ws:wsflow-only` markers, `ws.`, the four
wsflow-excluded skill names), then applies literal `ws:`→`wsflow:`/`ws/`→`wsflow/`
substitution — no marker-stripping exception path, per the revised (design
conflict resolved by removing the disqualifying content from the source
instead of exempting it). `skills_mirror_test.go` adds
`TestWsflowSkillsMirrorUpToDate` (substitution-aware drift guard, parallel to
`TestWsflowRsrcMirrorUpToDate`), `TestRegenerateWsflowSkillsMirror` (gated by
`WS_REGEN_WSFLOW_SKILLS`, skipped by default), 4 negative-path guard subtests,
and 1 positive-path substitution test. `ai-docs/ref/wsflow-mirroring.md` gets
a new "Substitution-Mirrored Skill Generation" section as a bounded exception
(curated list: exactly `lead-prefer-subagent` and `lead-verify-discussion`),
retiring the old "temporary exception" wording.

Design conflict resolved before implementation: Phase 2's survey found the
already-merged Phase 1 `lead-verify-discussion/SKILL.md` carried an unmarked
`delegates:true`-driven mercenary paragraph, which the guard would reject
outright. Rather than carving a marker exception into the guard, review
concluded the classification itself was wrong for this skill (conditional
"when investigation is useful" delegation, not the unconditional delegation
`lead-verify-design` has) and removed both the continuity-tip and mercenary
paragraphs from `lead-verify-discussion/SKILL.md` in both `agents-plugin` and
`agents-plugin-wsflow` trees (`864902a3`), making the two files
byte-identical and dissolving the guard conflict entirely. A follow-up
`todo/` ticket (`260703-chore-review-delegates-true-classification`, sage-review
skipped) covers a broader review of `delegates:true` usage across the rest
of the rsrc playbook set.

Partitioned review: test non-clean (1 important — the drift test's
substitution-aware comparison was unexercised by real data, since both
curated fixtures contain zero `ws:`/`ws/` tokens and are true no-ops), fixed
in `0aaacd01` by adding an inline synthetic namespace-token fixture inside
`TestWsflowSkillsMirrorUpToDate`. Fix verified empirically by both the fix
author and the original reviewer's methodology (break the substitution
`ReplaceAll` calls, confirm the new assertion fails, restore, confirm clean).
1 minor finding (guard's excluded-skill-name subtest only exercises one of
four names) left as-is per lead disposition (low-risk, shared code path).

`go build ./...` and `go test ./internal/wsrsrc/... ./internal/mcp/...`
(agents-plugin-tool) pass; `python3 -m unittest discover
agents-plugin-wsflow/tests` passes unchanged (9 tests).

Plugin version bumped 0.32.2 → 0.32.3 (dev-merge rule) in `198c0290`.

This was the ticket's final phase.
