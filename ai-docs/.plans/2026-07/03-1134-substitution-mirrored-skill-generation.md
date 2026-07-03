# Plan: 260703-chore-prefer-subagent-verify-discussion-inline-mirror — Phase 2: Substitution-mirrored skill generation mechanism

## Relevant Ticket Contract

- New "substitution-mirrored skill" category: source of truth is
  `agents-plugin/skills/<name>/SKILL.md`; a generation script derives
  `agents-plugin-wsflow/skills/<name>/SKILL.md` via literal namespace
  substitution (`ws:` → `wsflow:`, `ws/` → `wsflow/`).
- **Hard-gate eligibility guard, effective immediately**: generation fails
  loudly if the source body contains anything beyond namespace-only tokens —
  e.g. the word "mercenary", `<!-- ws:full-only:... -->`/`<!-- ws:wsflow-only:...
  -->` markers, literal names of wsflow-excluded skills (`lead-write-code`,
  `lead-write-skeleton`, `lead-salvage`, `lead-skill-authoring`), or other
  product-specific content a blind substitution would mishandle. Guard should
  default to strict/conservative, not a curated exception list.
- A drift test analogous to `TestWsflowRsrcMirrorUpToDate` (in
  `internal/wsrsrc`), wired into the same regen-check pattern including the
  `-count=1` test-cache gotcha (see `ai-docs/ref/wsflow-mirroring.md`).
- The curated list of eligible skills lives in a new explicit section in
  `ai-docs/ref/wsflow-mirroring.md`, phrased as an additive bounded exception
  parallel to (but a separate section from) the existing rsrc
  "Generated-sameness carve-out" section — it must not read as a general
  loosening of the skill-mirroring rule.
- Scope: only skills explicitly and deliberately migrated out of
  `playbook.print` are eligible — currently exactly `lead-prefer-subagent` and
  `lead-verify-discussion`. Not a blanket auto-mirror mechanism.
- **Verification / acceptance for Phase 2**:
  - The drift test passes against the checked-in generated
    `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` and
    `.../lead-verify-discussion/SKILL.md`.
  - The eligibility guard's negative path is actually exercised: a
    deliberately-broken fixture (containing "mercenary" or a `ws:full-only`
    marker) must make the guard test fail.

## Out of Scope

- Whether other prose/behavior-mode skills should migrate into this category
  later (ticket "Out of Scope").
- Changes to `lead-implement` or other `playbook.print`-backed sequence-strict
  skills.
- Phase 1 work (already implemented/merged: inline bodies, `LoadSkillBody`,
  `ResolveSkillsRoot`, independent `agents-plugin/skills/manifest.json` +
  drift test, override-marker Go symbol deletion). Phase 2 only adds the
  generation/guard/drift mechanism and the curated-list doc section.

## Codebase Findings

- `agents-plugin/skills/lead-verify-discussion/SKILL.md#L36-L38` — **Contract
  conflict (blocking).** The actual, current, already-merged (Phase 1) source
  file contains an unmarked, literal paragraph: `**Mercenary path (always
  available):** A ws-managed external subprocess agent (mercenary) is always
  reachable on request via \`ws.mercenary.call\`... a self-contained prompt
  from \`ws/playbook.render\`...`. This is plain prose, not wrapped in any
  `<!-- ws:full-only:... -->`/`<!-- ws:wsflow-only:... -->` marker. The Decisions
  section names literally this content ("the word 'mercenary'") as
  guard-disqualifying. Applying the guard as specified to the real file as it
  exists today would make generation **fail loudly for the very skill the
  ticket says is one of the two initial, currently-passing entries** — a
  direct contradiction with the Phase 2 acceptance criterion that the drift
  test must pass against the checked-in wsflow counterpart.
- `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` (diffed
  against the full-ws source) — currently a **hand-authored** divergent body:
  identical except the mercenary paragraph (lines 36-38 in the source) is
  omitted entirely. This was produced by a human/agent edit in Phase 1
  (commit `f9b8e0c7`), not by any substitution script. There is no marker in
  the source recording *why* or *which lines* were dropped — a blind
  line-for-line namespace-substitution generator has no signal to reproduce
  this omission automatically.
- `agents-plugin/skills/lead-prefer-subagent/SKILL.md` vs.
  `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` — confirmed
  **byte-identical today** (`diff` exit 0). This file contains no `ws:`/`ws/`
  tokens at all (the harness-conditional prose in Decisions was already
  written host-neutral in Phase 1), so plain substitution is a true no-op for
  this skill and poses no guard conflict.
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L538-L586` —
  `selectProductModeBlocks` is the **existing, precedented marker mechanism**
  (`ws:full-only:start/end`, `ws:wsflow-only:start/end`,
  `ws:mercenary-on:start/end`) used by the *runtime* `playbook.print`/`render`
  pipeline to select product-mode content from a shared rsrc body. This is the
  natural mechanism to reuse/mirror if the source `SKILL.md` is updated to
  wrap the mercenary paragraph in `<!-- ws:wsflow-only:... -->`-style markers
  (inverted: strip on wsflow output) so a script can strip it deterministically
  — but per Decisions, *presence* of such markers is itself named as
  guard-disqualifying content, not an instruction for the generator to handle.
  This is a second reading ambiguity: are markers (a) forbidden entirely for
  eligible sources (guard rejects them, meaning content must be marker-free
  and namespace-token-only), or (b) the *sanctioned exclusion mechanism* the
  generator is expected to strip (guard rejects only *unmarked*
  product-specific content)? The ticket text reads as (a) — markers are listed
  as an example of disqualifying content alongside the word "mercenary" itself
  — which would mean eligible sources may contain **zero** product-specific
  content, full stop, and the existing mercenary line would need to be
  deleted or reworded out of the shared source (a source-text change beyond
  Phase 2's stated scope of "generation script, guard, drift test").
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L114` — direct
  structural precedent to mirror for the new drift test: `collectTreeBytes`
  walk-and-hash-map helper, `TestWsflowRsrcMirrorUpToDate` (fails loud with a
  sorted diff list and a copy-pasteable regen command), and
  `TestRegenerateWsflowRsrcMirror` gated by an env var, skipped by default.
  New test needs a **substitution-aware** comparison, not `bytes.Equal` — the
  wsflow file is `ws:`→`wsflow:`/`ws/`→`wsflow/`-substituted, not identical.
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L1-L69` — direct
  precedent for the "independent parallel manifest" pattern and its own
  distinct regen-env-var convention (`WSRSRC_REGEN_SKILLS`, distinct from the
  rsrc `WSRSRC_REGEN`/`WS_REGEN_WSFLOW_RSRC`). The new Phase 2 mechanism should
  follow the same pattern: a distinct, non-colliding env var name for its own
  regen entrypoint (e.g. `WS_REGEN_WSFLOW_SKILLS` or similar), never reusing
  `WS_REGEN_WSFLOW_RSRC` (that name is already the rsrc-tree regen and reusing
  it would silently couple two independent generation surfaces).
- `agents-plugin-tool/internal/wsrsrc/loader.go#L60-L72` — `LoadSkillBody`
  already strips frontmatter via `parseFrontmatter`; unclear from the ticket
  whether the substitution-mirrored generator should substitute over the
  frontmatter block too (the `name:`/`description:` fields are namespace-free
  in both current files, so this has not yet mattered, but a generator that
  operates on the full raw file including frontmatter vs. body-only is an
  implementation choice not settled by the ticket text).
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L31-L52` —
  `EXPECTED_INLINE_SKILLS = {"lead-revive", "lead-prefer-subagent",
  "lead-verify-discussion"}` and `FORBIDDEN_PATTERNS` (regexes for `ws/`,
  `ws:`, `ws.`, `subquery`, `agents.`, and the four excluded skill names) is
  the **existing, separately-run Python static verification** that both
  wsflow `SKILL.md` files must already pass today (they do, since
  `lead-verify-discussion`'s wsflow copy omits the mercenary line and
  `lead-prefer-subagent` never had ws-tokens). Phase 2's new Go drift test is
  additive to this, not a replacement — this file has no awareness of the new
  generation mechanism and needs no change for Phase 2 unless the curated-list
  doc update implies inventory changes (it does not; the skill lists are
  unaffected).
- `ai-docs/ref/wsflow-mirroring.md#L74-L79` — existing "Exception (temporary,
  ahead of a formal curated category)" paragraph already documents that these
  two skills ship inline bodies; Phase 2 must add the new curated-list section
  described in Decisions (separate from, not a rewrite of, the rsrc
  "Generated-sameness carve-out" section at lines 151-159) and should likely
  retire or update this "temporary" exception wording once the formal category
  exists, though the ticket does not explicitly require deleting the old
  paragraph.

## Implementation Plan

**Lead-resolved escalation, revised (2026-07-03, user-confirmed direction:
remove the disqualifying content instead of carving a marker exception.)**
On further review, the `delegates:true`-driven continuity-tip and
mercenary-path paragraphs were a poor fit for `lead-verify-discussion` in the
first place: its own delegation is conditional ("when investigation is
useful"), unlike `lead-verify-design`'s unconditional "isolate a fresh deep
reviewer" delegation; the sibling checkpoint `lead-check-blockers` (same
"compact, lightweight checkpoint" framing) never carried `delegates:true` at
all. Both paragraphs have been removed outright from
`agents-plugin/skills/lead-verify-discussion/SKILL.md` and
`agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` (which are now
byte-identical). This dissolves the guard conflict entirely — the source
contains no product-specific content — so the marker-exception mechanism
described in the prior edition of this plan is **not needed**. Phase 2
proceeds exactly per the ticket's original Decisions: a pure namespace
substitution generator with no marker-stripping step, no exception carve-out.

1. **Generator** (new, `agents-plugin-tool/internal/wsrsrc/`, standalone —
   do NOT import `internal/mcp` to avoid a reversed package-dependency
   direction):
   - Read the full raw `SKILL.md` (frontmatter included — the generator
     transforms the whole file, unlike `LoadSkillBody` which strips
     frontmatter for the runtime print path).
   - Apply literal substitution `ws:` → `wsflow:`, `ws/` → `wsflow/` over the
     text.
   - Guard: scan the pre-substitution text for disqualifying tokens
     (`mercenary`, `<!-- ws:full-only:` / `<!-- ws:wsflow-only:` markers,
     `ws.`, literal names of `lead-write-code`/`lead-write-skeleton`/
     `lead-salvage`/`lead-skill-authoring`). Any hit → fail loudly, do not
     write output. No exception path for marked content — presence of any of
     these tokens is disqualifying, full stop, per the ticket's original
     Decisions wording.
2. **Env var**: `WS_REGEN_WSFLOW_SKILLS` for the regen entrypoint — distinct
   from `WSRSRC_REGEN_SKILLS` (manifest regen) and `WS_REGEN_WSFLOW_RSRC`
   (rsrc mirror regen); never reuse either.
3. **Validation of the resolution itself**: run the new generator against
   `lead-prefer-subagent/SKILL.md` and `lead-verify-discussion/SKILL.md` and
   diff its output against the currently checked-in
   `agents-plugin-wsflow/skills/.../SKILL.md` counterparts. Expect
   byte-identical (confirms the mechanism reproduces the checked-in files);
   if not identical, treat generator output as canonical and note the diff
   explicitly as a finding, do not silently keep the old hand file.
4. **Drift test**: `TestWsflowSkillsMirrorUpToDate` in
   `agents-plugin-tool/internal/wsrsrc/`, substitution-aware (not
   `bytes.Equal`), parallel to `TestWsflowRsrcMirrorUpToDate`'s fail-loud/
   sorted-diff/copy-pasteable-regen-command shape; a
   `TestRegenerateWsflowSkillsMirror` counterpart gated by
   `WS_REGEN_WSFLOW_SKILLS`, skipped by default.
5. **Guard negative-path tests** (required, not optional — this is the
   ticket's explicit Phase 2 acceptance bar): separate fixtures/subtests each
   proving a genuine rejection: (a) "mercenary" word anywhere → guard fails;
   (b) a `ws:full-only`/`ws:wsflow-only` marker anywhere → guard fails; (c) an
   excluded-skill-name literal → guard fails.
6. **Curated-list doc**: add the new section to `ai-docs/ref/wsflow-mirroring.md`
   per ticket Decisions (bounded-exception framing, separate from the rsrc
   "Generated-sameness carve-out" section). Retire/update the existing
   "temporary exception" note at lines 74-79 to point at the new formal
   mechanism.
7. Do not change `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` —
   confirmed unaffected (Codebase Findings).

## Verification Plan

- Known verification boundary once research resolves the conflict: a new Go
  drift test in `agents-plugin-tool/internal/wsrsrc/` (parallel to
  `TestWsflowRsrcMirrorUpToDate`) must pass against the checked-in
  `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` and
  `.../lead-verify-discussion/SKILL.md`, and a guard test must demonstrate a
  deliberately-broken fixture (containing "mercenary" or a `ws:full-only`
  marker) actually fails generation. Both are blocked on resolving which
  source-file shape (marker-stripped vs. content-free) the guard is meant to
  accept.
- `python3 -m unittest discover agents-plugin-wsflow/tests` should continue to
  pass unchanged (no inventory/content expectations for these two skills
  change under any resolution path).

## Escalations

**Resolved (2026-07-03)** — see "Implementation Plan" above. The mercenary
paragraph and its `delegates:true`-driven continuity tip were removed from
`lead-verify-discussion/SKILL.md` outright (both full-ws and wsflow copies),
rather than kept and marker-exempted. The guard now runs against a source
containing zero disqualifying content, so none of the four numbered research
questions below apply anymore; retained for historical record of the
conflict that prompted the resolution.

- Confidence: low (historical, pre-resolution)
- Reason: The ticket's Decisions text specifies a hard-gate guard that rejects
  source content containing the word "mercenary" or `ws:full-only`/
  `ws:wsflow-only` markers, and names this as effective immediately against
  the two skills already selected as the mechanism's only two entries. But the
  real, already-merged `lead-verify-discussion/SKILL.md` source contains
  exactly that disqualifying content today (an unmarked mercenary-path
  paragraph), and the already-merged wsflow counterpart is a hand-divergent
  body missing that paragraph with no marker recording the divergence. A
  literal namespace-substitution generator, run against the guard as
  literally specified, cannot produce the currently-checked-in wsflow file
  from the currently-checked-in full-ws source — the guard would reject the
  source outright, or (if the guard is bypassed) plain substitution would
  reproduce the mercenary paragraph verbatim in the wsflow output, which is
  forbidden distributed-skill content per
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` `FORBIDDEN_PATTERNS`
  ("full ws dotted namespace" `ws\.` and "full ws MCP notation" `ws/`) and per
  the wsflow-mirroring doctrine itself. This is a genuine contract conflict,
  not an implementation detail — it cannot be resolved by picking an
  arbitrary interpretation without risking rework or a guard that is either
  too strict to ship (rejects the pilot skill) or too permissive to be the
  "effective immediately" gate the ticket demands.
- Research should decide:
  1. Whether `lead-verify-discussion/SKILL.md`'s mercenary paragraph must be
     wrapped in a marker (e.g. `<!-- ws:full-only:start/end -->`, inverted
     from the runtime-render semantics to mean "strip for wsflow
     substitution-mirror output") as an in-scope source-text edit for Phase
     2, and if so, whether the guard is defined to *require* well-formed
     markers around all non-namespace-token content (rejecting only
     *unmarked* disqualifying words) rather than rejecting marker presence
     itself.
  2. Whether the generator's marker-stripping behavior (if markers are the
     resolution) should reuse/import `selectProductModeBlocks`-style parsing
     from `agents-plugin-tool/internal/mcp/playbook_tools.go`, or is
     intentionally a separate, simpler line-filter local to the new
     generation code (avoiding an `internal/mcp` → `internal/wsrsrc`
     dependency direction that may not currently exist).
  3. Whether the existing hand-authored
     `agents-plugin-wsflow/skills/lead-verify-discussion/SKILL.md` is expected
     to change byte-for-byte as a result of adopting the generator (i.e. is
     the current file merely coincidentally compliant, or does it need
     regeneration/re-review once the real mechanism exists), given the
     drift-test acceptance criterion requires the test to pass against "the
     checked-in generated" file.
  4. Confirm the regen env-var name for the new mechanism (distinct from
     `WS_REGEN_WSFLOW_RSRC` and `WSRSRC_REGEN_SKILLS`) and the exact guard
     token list scope (does "namespace-only tokens" mean literally only
     `ws:`/`ws/` substrings are permitted to differ, with everything else
     required byte-identical pre-substitution — i.e. is the check "source
     minus namespace tokens equals wsflow output minus namespace tokens"
     rather than a keyword denylist at all?).
