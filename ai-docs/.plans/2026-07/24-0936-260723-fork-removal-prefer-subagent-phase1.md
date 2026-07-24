# Plan: Delete the fork delegation construct; reshape lead-prefer-subagent to fresh-spawn + central authoring whitelist — Phase 1: Remove fork from prefer-subagent and reshape to fresh + central whitelist

## Relevant Ticket Contract

- Delete the fork construct entirely from `lead-prefer-subagent`; do not
  retain it behind a flag or narrow it.
- Rewrite the opening invariant (SKILL.md:8), not just excise fork sentences —
  it is a semantic reversal: admit a context-holder-authors carve-out instead
  of asserting the lead "cannot act inline."
- Reshape to two clean delegation poles: fresh spawn (default) and
  lead-inline authoring for durable-artifact mutation via a **central
  authoring/mutation whitelist owned by `lead-prefer-subagent` as an
  overlay** — individual skills (e.g. `lead-write-ticket`) stay agnostic.
  Rule: authoring/mutation of durable artifacts (tickets, specs) stays with
  the session that already holds the authoritative context (lead if decided
  in lead conversation; the delegated subagent continuing in its own session
  if decided there) — never handed to a fresh, context-less agent for
  after-the-fact authoring from a summary.
- Remove the posture-gated-on-fork-availability clause (the "ask user to
  suspend posture if no fork mechanism" block) — no longer needed once fork
  is gone.
- Do NOT touch `native-spawn-binding.codex.md:7`'s `fork_turns:"none"` — that
  is the surviving fresh-spawn idiom, not a deletion target. Precision: the
  idiom to delete is `spawn_agent(fork_context:true, ...)` full-history fork;
  `fork_turns:"none"` is unrelated and must stay everywhere it appears.
- No override to delete: `builtinPromptOverrideDefaults()` in
  `playbook_tools.go` already returns `map[string]string{}` — its doc comment
  already states the sole prior entry (`PreferSubagentInvocationGuidance`)
  was retired when the skill body was inlined. Nothing to change in that
  function.
- Drop the `lead-goal-step` fork pointer (locate by content — "forked
  subagent" — not by the ticket's stale `:124` line number, which has
  drifted). Leave the unrelated git "fork point" term (at `lead-goal-step`
  SKILL.md around line 93) untouched.
- Tests: remove the `fork_context:true` assertions in
  `prefer_mercenary_phase2_test.go` and `playbook_tools_test.go`'s
  `spawn_agent(fork_context:true, ...)` check; **keep** the
  `fork_turns: "none"` assertions in `playbook_tools_test.go` (fresh-spawn
  binding, unrelated to fork deletion).
- Note in tickets `260625-research-fork-posture-leak-system-guarantee` and
  `260629-research-fork-worker-persona-bleed` that they are resolved by this
  deletion.
- **Skill-authoring discipline** (AGENTS.md + `lead-skill-authoring`): apply
  the six-point invariant checklist (Falsifiable / Actionable / One line /
  Context-free / Non-redundant / Doctrine-aligned) to every changed
  invariant line in the reshaped SKILL.md, then run the "Downstream
  Consistency Sweep" (doctrine/routing edit) covering mirrored surfaces,
  specs, and agent prompts that reference this posture.
- Spec Impact (in scope for Phase 1, closeout not a new contract): update
  `ai-docs/spec/workflow-skills.md` (prefer-subagent delegation posture /
  fresh-vs-fork routing description) and correct any stale
  `PreferSubagentInvocationGuidance.codex` fork-seeding description in
  `mcp-tools.md`. Mental-model docs `workflow-skills.md` and
  `prompt-bundle.md` are also named as relevant.
- Acceptance check: Go test suite passes with updated string assertions; a
  `fork` scan over rendered `lead-prefer-subagent` SKILL.md, the codex
  `native-spawn-binding` source, and the `PreferSubagentInvocationGuidance.codex`
  builtin default returns zero remaining fork-delegation references
  (excluding unrelated git "fork point" senses); `lead-goal-step` pointer no
  longer names fork.

## Out of Scope

- Any phase beyond Phase 1 (none currently defined in the ticket beyond
  Phase 1).
- `native-spawn-binding.codex.md` content itself — confirmed unrelated
  (`fork_turns:"none"` fresh-spawn idiom), must not be edited.
- Any change to `PreferSubagentInvocationGuidance` override machinery in Go —
  already retired; nothing live to delete.
- Any redesign of the fresh-spawn / whitelist mechanism beyond what the
  ticket's Decisions section specifies — this phase reshapes prose, not
  runtime code (fork has zero Go implementation per ticket Background).
- `260626-bug-prefer-subagent-fork-executor-narration` and
  `260626-bug-prefer-subagent-recursive-delegate-escape` — already dropped
  tickets, not reopened by this phase.

## Codebase Findings

- `agents-plugin/skills/lead-prefer-subagent/SKILL.md#L1-L22` — full canonical
  skill body (22 lines total, not 8-22 as loosely described in the ticket
  background; the ticket's "~8-22" range is the primary-edit span within this
  22-line file, roughly accurate). Key sub-spans to edit:
  - `L8` — opening invariant: "Under this posture the lead cannot act
    inline, so route each task to one of two delegates." Contradicts the new
    whitelist; needs the semantic-reversal rewrite per ticket Phase 1 note.
  - `L12` — fresh-vs-fork decision rule ("For other work, ask whether correct
    execution depends on a decision... If so, use a fork...") — reshape to
    fresh-by-default + central whitelist carve-out language.
  - `L14` — fork-failure re-dispatch clause ("If a fork does not execute the
    task...").
  - `L16` — fork definition ("Treat a subagent as forked only when...").
  - `L18` — the `spawn_agent(fork_context:true, ...)` invocation guidance
    paragraph — delete.
  - `L20` — posture-gated-on-fork-availability clause ("If no forked
    mechanism exists... ask the user whether to suspend maximum-delegation
    posture...") — delete per Decisions ("Remove the
    posture-gated-on-fork-availability clause").
  - `L22` — fork-prompt-authoring guidance (word budget, exact boundary
    phrasing, required return format) — delete or fold any still-relevant
    parts (e.g. execution-constraint framing, stop condition, return format)
    into the fresh-spawn prompt guidance if the reshaped skill still needs to
    describe how to write a delegate prompt; ticket does not explicitly ask
    to keep this, treat as fork-specific and remove unless a fresh-spawn
    equivalent is clearly needed — flag as a judgment call for the
    implementer, not a strategy gap (low risk, reversible via review).
  - **Eligibility-guard constraint (critical, from task facts, not ticket
    text):** the reshaped body feeds
    `GenerateWsflowSkillBody` (see wsflow finding below), which fails loudly
    on the word "mercenary", `<!-- ws:full-only:... -->` /
    `<!-- ws:wsflow-only:... -->` markers, or literal names of
    `lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
    `lead-skill-authoring`. The new prose must not introduce any of these
    tokens.

- `agents-plugin/skills/lead-goal-step/SKILL.md#L125-L128` — fork pointer to
  drop: "Conserve lead context for the long-running goal this serves: beyond
  selection, delegate everything else too — including simple tasks like
  commits — to an appropriately tiered subagent or forked subagent, following
  `lead-prefer-subagent`." The word "forked" is on L127. This has drifted
  from the ticket's stale `:124` reference; locate by content, not line
  number. Leave `L93`'s unrelated git "fork point" term
  ("PARENT, the fork point this goal run branches from") untouched — do not
  conflate the two senses.

- `ai-docs/ref/wsflow-mirroring.md#L122-L164` ("Substitution-Mirrored Skill
  Generation") — **both edited skills are substitution-mirrored**:
  `lead-prefer-subagent` and `lead-goal-step` are both in the exhaustive
  3-skill list (with `lead-verify-discussion`, untouched here). Their
  `agents-plugin-wsflow/skills/<name>/SKILL.md` counterparts are GENERATED
  via literal namespace substitution (`ws:`→`wsflow:`, `ws/`→`wsflow/`) from
  the canonical `agents-plugin/skills/<name>/SKILL.md`, never hand-edited.
  After editing either canonical body, regenerate:
  1. `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
     (from `agents-plugin-tool/`) — regenerates
     `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` and
     `agents-plugin-wsflow/skills/lead-goal-step/SKILL.md`.
  2. `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestGenerateRealSkillsManifest`
     — regenerates the skills manifest.
  The `-count=1` flag is mandatory for both (env-gated regen test bodies with
  no changing input; go's test cache can return a stale green `ok` without
  the write side effect). Drift guards `TestWsflowSkillsMirrorUpToDate` and
  `TestSkillsManifestDriftIsVisible` will fail red if either regen step is
  skipped — run both as part of the verification pass, not just after
  manual edits.

- `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go#L318` and
  `#L385` — `"spawn_agent(fork_context:true, message:<prompt>)"` string
  assertions inside `wantSentence`-style slices in
  `TestWorkflowPreferSubagentWorkflowManualHarnessSwitch` (approx, around
  L295-342) and `TestWorkflowPreferSubagentWorkflowManualClaudeGetsStaticSkillBody`
  (L353-393). Both must be removed/replaced once the fork invocation
  sentence is deleted from the skill body; the surrounding test structure
  (loop asserting `Contains` for a list of `want` strings) stays intact —
  only the fork-specific string element is removed from each `want` list.

- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L592` and `#L605` —
  `` `fork_turns: "none"` `` assertions inside `TestPlaybookPrintWorkflowManualHarnessBranch`-style
  table test (~L570-619): L592 is in the Claude "forbidden" list (must NOT
  contain — Codex-only fresh-spawn binding leak check), L605 is in the Codex
  "want" list (must contain). **Keep both unchanged** — they pin the
  surviving `fork_turns:"none"` fresh-spawn binding in
  `native-spawn-binding.codex.md`, unrelated to fork-construct deletion.

- `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L832` — a third
  `"spawn_agent(fork_context:true, message:<prompt>)"` assertion, inside a
  wsflow-mode test (~L811-838, `TestPlaybookPrintWsflow...`-style, asserting
  `bodyOn` contains a `want` list including the fork sentence when
  `workflow.prefer_subagent` is on). Remove this list element too — same
  fork-invocation sentence, wsflow-rendered path.

- `agents-plugin-tool/internal/mcp/playbook_tools.go#L451-L467` —
  `builtinPromptOverrideDefaults()` already returns `map[string]string{}`
  with a doc comment stating the sole prior entry
  (`PreferSubagentInvocationGuidance`) was retired when
  `lead-prefer-subagent`'s body moved to a static inlined SKILL.md. **No Go
  code change needed here** — ticket's "already retired" claim confirmed
  live. Do not touch this function; only the doc comment is a candidate for
  a clarifying note if the implementer judges it stale, but it already
  correctly states the retirement.

- `ai-docs/spec/mcp-tools.md#L1387-L1392` (anchor
  `{#260619-delegation-section-override-point}`) — **stale**, contradicts
  live behavior and even contradicts this same spec file's own
  `#260505-workflow-primitive-reference` section (L93-102, which correctly
  says the body is "read directly via `LoadSkillBody` with no
  override-marker pass"). L1387-1391 still describes
  `PreferSubagentInvocationGuidance` as the live extension point and Codex's
  `prompt.PreferSubagentInvocationGuidance.codex` as seeding
  `spawn_agent(fork_context:true, ...)` guidance today. This must be
  corrected/removed as part of the ticket's named Spec Impact check ("check
  whether `prompt-bundle` / `mcp-tools` still describe... and correct it if
  stale"). The anchor `{#260619-delegation-section-override-point}` is
  cross-referenced once more, from
  `ai-docs/tickets/.done/260625-refactor-workflow-delegation-config.md:9`
  (a closed ticket's `related:` list) — that reference is historical and
  does not need updating, but do not delete the anchor if anything else
  might resolve it; confirm no other live doc links it before removing the
  anchor itself (only the mcp-tools.md file was found referencing it besides
  the closed ticket).

- `ai-docs/mental-model/prompt-bundle.md#L36` — also stale, same pattern:
  "Current use: the generic `prompt.PreferSubagentInvocationGuidance.codex`
  default fills an empty `lead-prefer-subagent` slot with Codex
  `spawn_agent(fork_context:true, ...)` invocation guidance while Claude
  renders the empty shared seed." This describes machinery that no longer
  exists in code (`playbook_tools.go` confirms the function returns an empty
  map). Needs correction alongside the mcp-tools.md fix — same underlying
  fact, two doc sites.

- `ai-docs/spec/workflow-skills.md#L93-L102` (anchor
  `{#260505-workflow-primitive-reference}`) — this section is already
  correct (describes the static-inlined-body, no-override-marker-pass
  behavior) and does not mention fork. No `fork` substring appears anywhere
  in this file except the unrelated `L129` (`fork_turns: "none"`, Codex
  native-spawn binding — leave as-is) and `L484` (git "fork point" sense —
  leave as-is). This spec file's prefer-subagent section needs no fork-path
  correction; the ticket's "target spec area: workflow-skills" concern is
  already satisfied by existing text, so this file likely needs no edit
  beyond a light pass confirming nothing describes the deleted fork branch
  as current behavior (a targeted re-check, not a rewrite).

- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go` — curated list
  gating substitution-mirror eligibility (referenced by
  `wsflow-mirroring.md#L140`); confirms the 3-skill list membership. Not
  edited by this phase (no new skill added to the list), but the
  implementer should be aware this is the enforcement point if the
  eligibility guard trips during regeneration.

## Implementation Plan

1. Rewrite `agents-plugin/skills/lead-prefer-subagent/SKILL.md` per the
   ticket's Decisions and Phase 1 instructions:
   - Rewrite the opening invariant (current L8) to admit the
     context-holder-authors carve-out instead of "cannot act inline."
   - Replace the fresh-vs-fork decision rule (current L12) with: fresh spawn
     by default for all delegated payloads; authoring/mutation of durable
     artifacts (tickets, specs) stays with whichever session already holds
     the authoritative context (lead-inline if decided in the lead
     conversation; the delegated subagent's own continuing session if
     decided there) — framed as a central whitelist overlay, not
     interwoven into per-skill text.
   - Delete the fork-failure re-dispatch clause (current L14), the fork
     definition (current L16), the `spawn_agent(fork_context:true, ...)`
     invocation guidance (current L18), and the
     posture-gated-on-fork-availability clause (current L20).
   - Decide whether any part of the fork-prompt-authoring guidance (current
     L22 — word budget, execution-constraint framing, required return
     format) has a fresh-spawn equivalent worth keeping; if kept, rewrite it
     as fresh-spawn prompt guidance, not fork guidance.
   - Apply the `lead-skill-authoring` six-point invariant checklist
     (Falsifiable / Actionable / One line / Context-free / Non-redundant /
     Doctrine-aligned) to every changed invariant line.
   - Verify the new body contains none of: the word "mercenary", `<!-- ws:full-only:... -->`
     / `<!-- ws:wsflow-only:... -->` markers, or the literal names
     `lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
     `lead-skill-authoring` (substitution-mirror eligibility guard).

2. Edit `agents-plugin/skills/lead-goal-step/SKILL.md` to drop the fork
   pointer at the paragraph containing "forked subagent" (content-located,
   currently ~L125-128), rewording to reference only
   `lead-prefer-subagent`/fresh-spawn delegation. Leave the unrelated git
   "fork point" term (~L93) untouched. Re-run the same eligibility-guard
   check on the edited body (this skill is also substitution-mirrored).

3. Regenerate the wsflow substitution mirrors and skills manifest (from
   `agents-plugin-tool/`):
   - `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
   - `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestGenerateRealSkillsManifest`
   Confirm the regenerated
   `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` and
   `agents-plugin-wsflow/skills/lead-goal-step/SKILL.md` reflect the new
   prose under `wsflow:`/`wsflow/` namespacing.

4. Update tests to match the new skill body:
   - `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go` —
     remove the `"spawn_agent(fork_context:true, message:<prompt>)"` element
     from the `want` lists at (currently) L318 and L385; if the reshaped
     skill body drops the phrase "Maximum-delegation posture for this
     session" or other asserted strings, update those too, but only if the
     rewrite actually changes that exact wording — confirm before editing
     unrelated assertions.
   - `agents-plugin-tool/internal/mcp/playbook_tools_test.go` — remove the
     `"spawn_agent(fork_context:true, message:<prompt>)"` element from the
     `want` list at (currently) L832. Do NOT touch L592/L605
     (`fork_turns: "none"`) — those pin the unrelated fresh-spawn binding.

5. Note ticket resolution: append a short "resolved by deletion" note (per
   ticket Background wording) to
   `ai-docs/tickets/idea/260625-research-fork-posture-leak-system-guarantee.md`
   and
   `ai-docs/tickets/idea/260629-research-fork-worker-persona-bleed.md`
   (confirm exact current path/status directory before editing — ticket
   Background implies `idea/` but verify with a listing since these may have
   moved).

6. Spec closeout:
   - `ai-docs/spec/mcp-tools.md#L1387-L1392` — correct or remove the stale
     `PreferSubagentInvocationGuidance`/Codex-fork-seeding description under
     anchor `{#260619-delegation-section-override-point}` so it matches the
     already-correct `#260505-workflow-primitive-reference` section
     (L93-102) and the live `builtinPromptOverrideDefaults()` empty-map
     behavior. Confirm no other live doc depends on the anchor before
     removing it outright (only the closed ticket
     `260625-refactor-workflow-delegation-config.md` references it, and that
     is historical).
   - `ai-docs/mental-model/prompt-bundle.md#L36` — correct the matching
     stale sentence about `prompt.PreferSubagentInvocationGuidance.codex`.
   - `ai-docs/spec/workflow-skills.md` — re-check the prefer-subagent
     section (L84-114) after the skill rewrite; it currently contains no
     fork-path description needing removal, but re-verify against the final
     reshaped SKILL.md wording before closing this off as "no edit needed."
   - Follow `AGENTS.md`'s commit convention: if a spec heading's `{#slug}`
     changes, include `renamed-spec: <old-stem> -> <new-stem>` in the
     commit; only needed if the anchor above is actually removed/renamed
     rather than corrected in place.

7. Run the Downstream Consistency Sweep required by
   `lead-skill-authoring` for doctrine/routing edits: scan for other rsrc
   playbooks, entry skills, agent prompts, or specs that reference fork
   delegation, `fork_context`, or the old "cannot act inline" framing beyond
   the sites already identified above, and classify any additional findings
   as fix / risk-accepted / intentional-difference / out-of-scope.

## Verification Plan

- `cd /home/swkang/devenv/agents-plugin-tool && go test ./... -count=1` — full
  suite, catches the edited assertions plus the two drift guards
  (`TestWsflowSkillsMirrorUpToDate`, `TestSkillsManifestDriftIsVisible`) and
  the wsflow byte-mirror check.
- `cd /home/swkang/devenv/agents-plugin-tool && go test ./internal/mcp/... -run "PreferSubagent|WorkflowManual" -count=1 -v` —
  focused re-check of the specific edited test functions.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — wsflow package
  test (per `ai-docs/ref/wsflow-mirroring.md`), checks shipped-skill
  forbidden-reference and shim-shape invariants on the regenerated mirror.
- Manual acceptance scan (per ticket's Phase 1 acceptance check): grep for
  `fork` (case-sensitive on the delegation sense) across the rendered
  `lead-prefer-subagent` SKILL.md (both `agents-plugin/` and
  `agents-plugin-wsflow/` copies), `native-spawn-binding.codex.md`, and the
  `PreferSubagentInvocationGuidance.codex` builtin default location; confirm
  zero remaining fork-delegation references excluding the unrelated git
  "fork point" sense in `lead-goal-step` L93 and `fork_turns:"none"` in
  `native-spawn-binding.codex.md`. Confirm the `lead-goal-step` fork pointer
  is gone.

## Escalations

- None.
