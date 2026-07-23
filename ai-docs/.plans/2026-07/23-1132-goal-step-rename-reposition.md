# Plan: 260723-feat-goal-step-rename-and-goal-loop-completion — Phase 1: Rename + identity reposition

## Relevant Ticket Contract

- New name **`lead-goal-step`**. Reposition, not a bare swap: promote the
  goal-pursuit-step identity to primary in the SKILL.md body opening, spec
  (`workflow-skills.md`), and mental-model (`workflow-skills.md`); demote the
  single-cycle shim to a degenerate case; state in the body's first line that
  `ready/` is the sole progress gate.
- **Verified rename blast radius** (ticket Constraints — authoritative
  touch-point inventory, all confirmed present in this survey): both skill
  directory names, `name:` frontmatter in both, `agents-plugin/skills/manifest.json`
  path key, `substitutionMirroredSkills` in `skills_mirror_test.go`, Python
  tests (`test_wsflow_skill_bundle.py` EXPECTED_SKILLS/EXPECTED_INLINE_SKILLS;
  `test_skill_dispatch_contracts.py` method name + path + inlined-body
  assertion string), spec + mental-model prose, and
  `ai-docs/ref/wsflow-mirroring.md`.
- **Date-keyed anchors stay unchanged**: `{#260703-drain-ready-queue-skill}`,
  `{#260707-drain-goal-branch-staging}` (spec + mental-model).
- Historical `CHANGELOG.md` and `.done/`/`.dropped/` ticket references are
  left as-is (immutable history) — confirmed present, not touch points.
- **wsflow substitution mirror**: `lead-drain-ready-queue` is a
  substitution-mirrored inline-body wsflow skill (confirmed: both SKILL.md
  files are currently byte-identical). Edit canonical source only, regenerate
  the mirror via `WS_REGEN_WSFLOW_SKILLS=1`, keep drift guard + wsflow package
  tests green.
- Skill-authoring invariant checklist (`agents-plugin/skills/lead-skill-authoring/SKILL.md`
  → `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`) applies
  to every changed Invariants/Constraints/behavior line.
- Verification boundary (ticket): skill-authoring invariant checklist passes;
  drift guard + wsflow package tests + both Python test suites green (fresh
  regen / `-count=1`, not a cached pass); `go test ./...` green (test-list
  update only, no production Go change).
- Phase 2 spec content for the sibling anchors already exists as `🚧 Planned`
  prose in `ai-docs/spec/workflow-skills.md` (contract-first, authored at
  ready-promotion) — Phase 1 only resolves the rename-reposition anchor's 🚧
  marker; the ticket-curation-authority anchor's 🚧 content is Phase 2 and its
  wording must not change, only its blockquote placement (see Codebase
  Findings).

## Out of Scope

- Phase 2 body posture: the lead ticket-curation-authority clause, the
  blocked-progress-conclusion term, and bounded autonomous bug capture. Their
  drafted `🚧 Planned` spec text (anchors
  `{#260723-goal-step-ticket-curation-authority}` and
  `{#260723-goal-step-blocked-progress-conclusion}`) must survive this phase
  verbatim in content — only mechanical containment/placement changes if the
  blockquote split (below) requires it.
- Removing the non-goal defensive branches in the skill body (ticket
  Constraints: explicitly deferred to a separate ticket; do not touch).
- `ai-docs/_index.md` ticket-focus entry — references the old name in past
  description prose but is not in the ticket's verified touch-point list;
  leave untouched.
- Version bump (`bump-ws-version.sh`) — governed by AGENTS.md's "bump per
  dev-merge" rule, not by this phase's own verification list; apply only if/when
  this phase actually lands as a dev-merge, not as a plan step gated on Phase 1
  content correctness.
- `260722-feat-goal-run-autonomy-posture` Phase 2 sequencing decision (land
  before/after, re-point file target) — a ready/todo-promotion-time judgment
  per the ticket's "Sequencing with 260722" section, not this phase's job to
  resolve; note the file-path move only if landing first.

## Codebase Findings

- `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` (75 lines) — canonical
  source. Frontmatter `name: lead-drain-ready-queue` (L2), `description:` (L3)
  does not use "drain" or "goal" wording so is not test-constrained but is a
  reposition candidate. Body opens `# Drain Ready Queue` (L6) then
  `**Goal-run posture.**` (L8) — the ticket's required "body's first line states
  `ready/` is the sole progress gate" is currently absent; the single-cycle-shim
  framing ("pulls one item... hands to lead-proceed") lives only in the spec,
  not the SKILL.md body itself, which today opens straight into goal-run
  posture without stating the shim/gate framing at all.
- `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md` — byte-identical
  mirror of the file above (confirmed via `diff`, exit 0). No `ws:`/`ws/`
  substitution tokens present in this skill's body today, so regeneration is
  expected to stay a clean copy after the rename.
- `agents-plugin/skills/manifest.json#L10` — `"lead-drain-ready-queue/SKILL.md": "<hash>"`.
  Regenerated automatically by
  `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`
  (run from `agents-plugin-tool/`) once the directory is renamed and content
  edited — do not hand-edit the hash.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-19` —
  `substitutionMirroredSkills = []string{"lead-drain-ready-queue", "lead-prefer-subagent", "lead-verify-discussion"}`.
  This single string drives both source (`fullSkillsRoot()/<name>`) and
  destination (`wsflowSkillsRoot()/<name>`) mirror paths (L40-42, L97-99) — both
  directories must already be renamed to `lead-goal-step` before this list
  entry is updated, or the regen test will read/write the wrong path.
  Regenerate with
  `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
  after the rename (expected no-op content change, just confirms clean
  regeneration per the ticket's "avoid a cached pass" verification note).
- `agents-plugin/tests/test_skill_dispatch_contracts.py#L67-78` —
  `test_drain_ready_queue_is_inlined_static_body` reads
  `SKILLS_DIR / "lead-drain-ready-queue" / "SKILL.md"` and asserts
  `assertNotIn('ws/playbook.print(name: "lead-drain-ready-queue")', text)` plus
  four content-substring assertions (`"light-tier Explore-style subagent"`,
  `"(FIFO)"`, `"prerequisite"`, `"lead-prefer-subagent"`, `"Do not list"`) that
  must all still be true post-reposition (i.e. the reposition must preserve
  these phrases or the test needs matching updates — safest is to keep the
  phrases intact while restructuring surrounding prose). Ticket Constraints
  call out renaming the **method name** too.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L16-45` —
  `EXPECTED_SKILLS` (L20) and `EXPECTED_INLINE_SKILLS` (L44) both list
  `"lead-drain-ready-queue"`; rename both. `FORBIDDEN_PATTERNS` (L48-58) is
  unaffected (name change doesn't introduce forbidden tokens).
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go` — no literal
  skill-name string found (confirms manifest content is derived, not
  hardcoded); its regen command is the one above.
- `ai-docs/spec/workflow-skills.md`:
  - `L27` — namespace listing block, plain `lead-drain-ready-queue` line.
  - `L52-56` — "14 entry skills" enumeration listing `lead-drain-ready-queue`
    among directly-invocable `/ws:<name>` skills.
  - `L425-456` — the shim's base description under anchor
    `{#260703-drain-ready-queue-skill}`: opens with the single-cycle-shim
    framing ("pulls one item from the `ready/` ticket queue... single-cycle
    shim, not a loop"). Per the reposition decision this framing must become
    the *degenerate-case* description, not the lead sentence; anchor ID is
    unchanged.
  - `L458-477` — **one blockquote spanning two anchors**: `L459-464` is the
    already-drafted `🚧 Planned` text for
    `{#260723-lead-goal-step-rename-reposition}` (Phase 1's target semantics,
    written in announcement voice: "This shim is renamed `lead-goal-step`
    and repositioned so the goal-pursuit step is its primary identity; the
    standalone single-cycle drain becomes a degenerate case, with the `ready/`
    queue stated explicitly as the sole progress gate..."); `L466-477` is the
    Phase 2 `🚧 Planned` text for
    `{#260723-goal-step-ticket-curation-authority}` (ticket-curation authority
    + bug capture), sharing the same `>` blockquote container. **Risk signal**:
    landing Phase 1 requires splitting this single blockquote into (a) the
    rename-reposition content promoted out of `🚧 Planned` into permanent
    prose (folded into the `L425-456` base description, rewritten from
    announcement voice to settled descriptive voice) and (b) a new
    standalone `🚧 Planned` blockquote containing only the
    ticket-curation-authority paragraph, content byte-for-byte unchanged,
    anchor unchanged.
  - `L479-514` — goal-branch-staging description under
    `{#260707-drain-goal-branch-staging}`: pure token-rename target
    (`lead-drain-ready-queue` → `lead-goal-step`), content otherwise already
    goal-framed and consistent with the new identity — no restructuring
    needed here beyond the name swap.
  - `L516-530` — second `🚧 Planned` block, anchor
    `{#260723-goal-step-blocked-progress-conclusion}` (Phase 2, blocked-progress
    conclusion). Standalone block, not entangled with Phase 1's edits — leave
    untouched.
- `ai-docs/mental-model/workflow-skills.md#L68-69` — two single-line bullets
  under `{#260703-drain-ready-queue-skill}` and
  `{#260707-drain-goal-branch-staging}`. `L68` opens "`lead-drain-ready-queue`
  never lists `ready/` or reads ticket files itself..." — same
  shim-framing-first pattern as the spec; needs reposition (goal-step identity
  first, selection-mechanism/shim details demoted) plus token rename. `L69` is
  goal-branch-staging mechanism detail, already goal-framed — token rename
  only, no restructuring. No `🚧 Planned` convention used in mental-model
  files (spec-only pattern); nothing to split here.
- `ai-docs/ref/wsflow-mirroring.md#L50,L75,L80,L133` — four
  `lead-drain-ready-queue` list/prose mentions in the substitution-mirrored
  skills inventory; token rename only. Note (pre-existing, unrelated,
  out-of-scope drift): `L134` says "For these two skills only" but lists three
  skills (`lead-prefer-subagent`, `lead-verify-discussion`,
  `lead-drain-ready-queue`) above it — a pre-existing doc inaccuracy, not
  introduced by and not fixed by this rename; do not touch it as part of this
  phase.
- `agents-plugin/skills/lead-prefer-subagent/SKILL.md#L5`,
  `agents-plugin/skills/lead-verify-discussion/SKILL.md#L5`,
  `agents-plugin/skills/lead-revive/SKILL.md#L5` — sibling inline-body skills'
  H1 title convention: Title Case of the name minus `lead-` prefix ("Prefer
  Subagent", "Verify Discussion", "Revive"). New H1 for this skill should
  follow the same pattern: `# Goal Step`.
- Verification commands (confirmed from `ai-docs/ref/wsflow-mirroring.md#L231`
  and prior `.done` ticket precedent
  `ai-docs/tickets/.done/260703-feat-lead-drain-ready-queue-skill.md`):
  `python3 -m unittest discover agents-plugin-wsflow/tests`,
  `python3 -m unittest discover agents-plugin/tests`,
  `go test ./... -count=1` (from `agents-plugin-tool/`).

## Implementation Plan

1. `git mv agents-plugin/skills/lead-drain-ready-queue agents-plugin/skills/lead-goal-step`
   and `git mv agents-plugin-wsflow/skills/lead-drain-ready-queue agents-plugin-wsflow/skills/lead-goal-step`.
2. Edit `agents-plugin/skills/lead-goal-step/SKILL.md`:
   - `name: lead-drain-ready-queue` → `name: lead-goal-step`.
   - Reconsider `description:` for goal-step-primary framing (not a listed
     touch point, but consistent with the reposition intent; keep it short and
     factual — e.g. still mention the `ready/` selection + `lead-proceed`
     handoff, add the per-turn/goal-step framing).
   - Rewrite the H1 to `# Goal Step` (per sibling inline-skill convention).
   - Reposition the opening: the body's first prose line/paragraph must state
     that `ready/` is the sole progress gate and that this is the per-turn
     step of a goal-pursuit run; fold the existing single-cycle-shim framing
     in as the degenerate (non-`goal/*`) case rather than the lead framing.
     Preserve every literal phrase the dispatch-contract test asserts on
     (`"light-tier Explore-style subagent"`, `"(FIFO)"`, `"prerequisite"`,
     `"lead-prefer-subagent"`, `"Do not list"`) somewhere in the restructured
     body.
   - Apply the skill-authoring invariant checklist (Falsifiable / Actionable /
     One line / Context-free / Non-redundant / Doctrine-aligned) to every
     changed line per `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`.
   - Rename all internal `lead-drain-ready-queue` self-references (there are
     none besides the frontmatter `name:` in the current file — confirm none
     were introduced by the rewrite).
3. Regenerate the wsflow mirror:
   `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
   (from `agents-plugin-tool/`) — overwrites
   `agents-plugin-wsflow/skills/lead-goal-step/SKILL.md` from the canonical
   source; expect a clean/no-diff regen since the file has no `ws:`/`ws/`
   tokens.
4. `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L16` — change
   `"lead-drain-ready-queue"` → `"lead-goal-step"` in
   `substitutionMirroredSkills` (do this before step 3 if running the regen
   test, since the regen test reads/writes paths derived from this list —
   sequence steps 1 → 4 → 3 in practice, or re-run step 3 after step 4 to be
   safe).
5. Regenerate the manifest:
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`
   (from `agents-plugin-tool/`) — updates
   `agents-plugin/skills/manifest.json#L10` key + hash automatically.
6. `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` — rename
   `"lead-drain-ready-queue"` → `"lead-goal-step"` at `L20` (`EXPECTED_SKILLS`)
   and `L44` (`EXPECTED_INLINE_SKILLS`).
7. `agents-plugin/tests/test_skill_dispatch_contracts.py#L67-78` — rename the
   method `test_drain_ready_queue_is_inlined_static_body` →
   `test_goal_step_is_inlined_static_body` (or equivalent), the path segment
   `"lead-drain-ready-queue"` → `"lead-goal-step"` (L71), and the assertion
   string `'ws/playbook.print(name: "lead-drain-ready-queue")'` →
   `'ws/playbook.print(name: "lead-goal-step")'` (L73). Leave the four
   content-substring assertions as-is if step 2 preserved those phrases.
8. `ai-docs/spec/workflow-skills.md`:
   - `L27`, `L52-56` — rename the plain-text/listing occurrences of
     `lead-drain-ready-queue` → `lead-goal-step`.
   - `L425-456` (anchor `{#260703-drain-ready-queue-skill}`, unchanged ID) —
     rewrite the opening to lead with the goal-step identity, demoting the
     single-cycle-shim description to the degenerate-case framing; rename all
     `lead-drain-ready-queue` mentions (`L425`, `L452`) → `lead-goal-step`.
   - `L458-477` blockquote split: promote `L459-464`'s content out of
     `🚧 Planned` into the permanent prose (merged into the rewritten
     `L425-456` section, rewritten from announcement voice — "This shim is
     renamed... and repositioned..." — to settled descriptive voice matching
     the surrounding spec's tense), remove the
     `{#260723-lead-goal-step-rename-reposition}` `🚧` marker for this
     resolved content. Re-emit `L466-477`'s ticket-curation-authority
     paragraph as its own standalone `> [!note] Planned 🚧` blockquote with
     `{#260723-goal-step-ticket-curation-authority}`, content byte-identical,
     placed at the same relative position (immediately after the rewritten
     base description, before the `L479-514` goal-branch-staging section).
   - `L479-514` (anchor `{#260707-drain-goal-branch-staging}`, unchanged ID) —
     rename `lead-drain-ready-queue` (`L479`) → `lead-goal-step`; no other
     content change (already goal-framed).
   - `L516-530` (`{#260723-goal-step-blocked-progress-conclusion}`) — leave
     untouched (Phase 2, standalone block).
9. `ai-docs/mental-model/workflow-skills.md#L68-69` — rename
   `lead-drain-ready-queue` → `lead-goal-step` in both bullets; reword `L68`'s
   opening to lead with the goal-step identity per the same reposition
   principle as the spec (mental-model files carry no `🚧 Planned` convention,
   so this is a direct edit, not a promotion). `L69` is token-rename only.
10. `ai-docs/ref/wsflow-mirroring.md#L50,L75,L80,L133` — rename
    `lead-drain-ready-queue` → `lead-goal-step` at all four occurrences. Do
    not touch the unrelated pre-existing "these two skills" vs
    three-skills-listed inaccuracy near `L134`.
11. Re-run step 3's regen command once more after all source edits land, to
    guarantee the wsflow mirror reflects the final canonical body (idempotent
    if nothing changed since step 3/4).

## Verification Plan

- `cd agents-plugin-tool && go test ./... -count=1` (fresh run, not cached) —
  covers `TestWsflowSkillsMirrorUpToDate` (drift guard) and
  `TestSkillsManifestDriftIsVisible`, plus confirms no other Go breakage
  (test-list-only change expected).
- `python3 -m unittest discover agents-plugin-wsflow/tests` — covers
  `EXPECTED_SKILLS`/`EXPECTED_INLINE_SKILLS` membership, forbidden-pattern
  scan, thin-shim shape checks (this skill is exempt via
  `EXPECTED_INLINE_SKILLS`).
- `python3 -m unittest discover agents-plugin/tests` — covers the renamed
  `test_goal_step_is_inlined_static_body` and any other dispatch-contract
  assertions.
- Manual: grep the full verified blast-radius list once more
  (`grep -rn "lead-drain-ready-queue\|drain-ready-queue" agents-plugin agents-plugin-tool agents-plugin-wsflow ai-docs/spec ai-docs/mental-model ai-docs/ref`)
  to confirm zero remaining hits outside `CHANGELOG.md` and `.done/`/`.dropped/`
  tickets.
- Manual: confirm both date-keyed anchors
  (`{#260703-drain-ready-queue-skill}`, `{#260707-drain-goal-branch-staging}`)
  are byte-identical to before in the spec and mental-model files (anchor
  string only, not surrounding prose).
- Manual: confirm the Phase 2 `🚧 Planned` content
  (`{#260723-goal-step-ticket-curation-authority}`,
  `{#260723-goal-step-blocked-progress-conclusion}`) is unchanged in wording
  after the blockquote split.

## Escalations

None.
