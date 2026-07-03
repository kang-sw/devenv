---
title: "Inline lead-prefer-subagent and lead-verify-discussion; add substitution-mirrored skill generation"
related:
  260630-epic-skill-playbook-diet: adjacent skill/playbook-boundary work, opposite direction (inlining vs dieting) on the same skill-authoring surface
  260610-chore-wsflow-explore-playbook-mirroring: prior deferred wsflow-mirroring-mechanism discussion, related precedent for narrow generated-skill carve-outs
sage-review: required
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
  `SKILL.md` text.
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
  - The curated list of skills opted into this category must live in a
    well-known, discoverable location — not buried in test code or an
    obscure config. Candidate: a new explicit section in
    `ai-docs/ref/wsflow-mirroring.md` (which already documents "Shipped
    wsflow Skills" and the rsrc "Generated-sameness carve-out"), or a small
    manifest file the generation script reads. Exact location is an open
    implementation decision for Phase 2, not resolved here.
  - Scope is narrow: only skills **explicitly and deliberately migrated out
    of `playbook.print`** are eligible (currently just these two) — this is
    not a blanket auto-mirror for all skills. `ai-docs/ref/wsflow-mirroring.md`
    must document this as an explicit, narrow carve-out from its existing
    doctrine that skill mirroring is curated/human-reviewed, not
    auto-generated — analogous in spirit to the existing rsrc
    "Generated-sameness carve-out" section, but kept separate and narrower:
    skills in general remain curated; only this registered subset is
    generated.

## Out of Scope

- Whether other prose/behavior-mode skills should eventually migrate into
  this same substitution-mirrored category — not decided. This ticket covers
  only the `lead-prefer-subagent` + `lead-verify-discussion` pilot and the
  mechanism needed to support it.
- Changing `lead-implement` or other sequence-strict procedural skills' use
  of `playbook.print`.

## Phases

### Phase 1: Inline both skill bodies

Rewrite `agents-plugin/skills/lead-prefer-subagent/SKILL.md`,
`agents-plugin/skills/lead-verify-discussion/SKILL.md`, and their
`agents-plugin-wsflow/skills/...` counterparts with the full procedure body
inlined as prose (sourced from the current
`agents-plugin/rsrc/lead-prefer-subagent/lead-prefer-subagent.md` and
`agents-plugin/rsrc/lead-verify-discussion/lead-verify-discussion.md`
content), applying the harness-conditional prose and per-product continuity
line described in Decisions. Remove the now-orphaned rsrc playbooks and any
`wsflow-mirroring.md`/package-test references to them as `playbook.print`
shims (check `EXPECTED_INLINE_SKILLS`-style exemption sets and the shipped
wsflow skill list in `ai-docs/ref/wsflow-mirroring.md`).

### Phase 2: Substitution-mirrored skill generation mechanism

Implement the generation script, the hard-gate eligibility guard, and the
drift test described in Decisions. Register `lead-prefer-subagent` and
`lead-verify-discussion` as the initial (and currently only) entries in the
curated list, placed in a well-known discoverable location. Document the
narrow carve-out in `ai-docs/ref/wsflow-mirroring.md`.
