# Plan: prefer-subagent-delegate-continuation

## Relevant Ticket Contract

- Goal: allow continuation (reuse) of an already-spawned delegate's own session under the `lead-prefer-subagent` posture, and remove the current wording that forbids it unconditionally.
- Required edit 1 (verbatim, user-approved): in `agents-plugin/skills/lead-prefer-subagent/SKILL.md`, replace the paragraph beginning "Route every delegated task to a fresh spawn built from named artifacts" with the two-sentence replacement given in the contract (adds "**new**" and "**opens with**" emphasis).
- Required edit 2 (verbatim, user-approved): insert immediately after that paragraph, in the same file, a new paragraph stating the continue-vs-fresh-spawn criterion (same work item the delegate already owns -> continue; new work item or judgment that must not inherit the prior agent's conclusion -> fresh spawn).
- Required edit 3 (content-specified, not verbatim-mandated): in `ai-docs/spec/workflow-skills.md`, the paragraph anchored `{#260724-prefer-subagent-fresh-spawn-delegation-posture}` must drop its now-false closing sentence ("...leaving two clean delegation poles: the fresh spawn and this context-holder carve-out.") and instead describe the delegate-session continuation affordance and its criterion, while preserving the still-true statement that the context-inheriting fork delegate and its Codex `spawn_agent` fork-fallback wording were removed.
- Hard constraint: `agents-plugin/skills/lead-prefer-subagent/SKILL.md` is substitution-mirrored — edit only the canonical `agents-plugin/` copy; never hand-edit `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` or `agents-plugin/skills/manifest.json`; regenerate both via env-gated tests.
- Hard constraint: the SKILL.md body must stay mirror-eligible/host-neutral — no "mercenary", no "SendMessage", no `ws:full-only`/`wsflow-only` markers, no denylisted skill names (`lead-write-code`, `lead-write-skeleton`, `lead-salvage`, `lead-skill-authoring`), and no host-specific continuation idiom (the runtime Continuity tip already supplies that).
- Read `ai-docs/ref/skill-authoring.md` before editing and apply its invariant checklist to changed lines.
- Explicit non-goals: no agent-id tracking wording, no context-exhaustion retirement wording, no new ticket, no edits to `lead-implement`/`lead-goal-step`/other skills.
- Verification boundary: `go test ./...` in `agents-plugin-tool` must pass; the wsflow python suite (if it covers this skill) must pass; `git diff` must show the wsflow mirror + manifest.json changed only as regen output.
- Report-only (do not implement): `ai-docs/mental-model/workflow-skills.md` line ~38 goes stale with this change ("every delegated task routes to a fresh spawn" and "The sole inline-execution carve-out") — flag for the doc pass, do not edit in this commit.

## Exact Required Edits

Edits 1 and 2 are user-approved verbatim text. Copy them character-for-character,
including the bold markers and the `…` ellipsis character.

**Edit 1** — in `agents-plugin/skills/lead-prefer-subagent/SKILL.md`, replace the
whole paragraph currently beginning "Route every delegated task to a fresh spawn
built from named artifacts" with exactly:

> Route every **new** delegated task to a fresh spawn built from named artifacts plus general constraints, never from a copy of this conversation. A standing role (implementer, reviewer, …) **opens with** a fresh spawn — this is unconditional — and captures the conversation's decisions into its spec so the fresh spawn stays self-contained.

**Edit 2** — in the same file, insert immediately after that paragraph (blank line
between, before the existing "Central authoring/mutation whitelist" paragraph)
exactly:

> Continue an existing delegate's session when the instruction is the same work item that delegate already owns — a review finding relayed back to its implementer, a widened query to the explorer that ran it, a gap filled by the survey agent that produced it. Open a fresh spawn instead when the work item is new, or when the judgment must not inherit the prior agent's conclusion — an independent review verdict, or a re-check of a claim that agent itself made.

**Edit 3** — the spec sentence rewrite described in "Relevant Ticket Contract"
item 3 and "Implementation Plan" step 3. This one is content-specified, not
verbatim-mandated; word it consistently with Edits 1-2 in spec prose style.

## Out of Scope

- `ai-docs/mental-model/workflow-skills.md` — staleness noted for doc pass, not edited here (explicit contract instruction: report, don't fix now).
- Agent-id continuity tracking/recording wording (owned by the runtime Continuity tip; 260605 settled it as tip-only).
- Context-exhaustion / long-running-session retirement wording (belongs to ticket `260611-bug-agent-context-exhaustion-opaque-failure`).
- `lead-implement`, `lead-goal-step`, and any other skill.
- Ticket creation (explicitly out of scope; this is an accepted single-slice inline change).

## Codebase Findings

- `agents-plugin/skills/lead-prefer-subagent/SKILL.md#L12` — exact current paragraph to replace: "Route every delegated task to a fresh spawn built from named artifacts plus general constraints, never from a copy of this conversation. A standing role (implementer, reviewer, …) always takes a fresh spawn — this is unconditional — and captures the conversation's decisions into its spec so the fresh spawn stays self-contained." Confirmed single occurrence in the file (safe for exact-match replace).
- `agents-plugin/skills/lead-prefer-subagent/SKILL.md#L14` — existing whitelist sentence already sanctions "the delegated subagent's own continuing session"; the new paragraph must stay consistent with this line's phrasing, not duplicate or contradict it.
- `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md#L1-L17` — currently byte-identical to the canonical file body (no `ws:`/`ws.` tokens present in this skill's prose, so the whole body passes the namespace-substitution guard unchanged). Confirms this file must NOT be hand-edited; it is regenerated.
- `ai-docs/ref/wsflow-mirroring.md#L136-L182` — substitution-mirror mechanism: `lead-prefer-subagent` is one of exactly four skills (`lead-prefer-subagent`, `lead-verify-discussion`, `lead-goal-step`, `mcp-server-repair`) mirrored via `GenerateWsflowSkillBody`; regen command: `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror` (run from `agents-plugin-tool/`). Drift guard: `TestWsflowSkillsMirrorUpToDate`. Eligibility guard rejects "mercenary", `ws:full-only`/`wsflow-only` markers, and the four denylisted skill names — matches the hard constraint in the contract.
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L26-L57` — `agents-plugin/skills/manifest.json` hashes the `agents-plugin/skills/` tree (including this SKILL.md); regen command: `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -count=1` (run from `agents-plugin-tool/`). Drift guard: `TestSkillsManifestDriftIsVisible`.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L35-L94` — houses `TestWsflowSkillsMirrorUpToDate` and `TestRegenerateWsflowSkillsMirror`; confirms the two regen env vars (`WS_REGEN_WSFLOW_SKILLS`, `WSRSRC_REGEN_SKILLS`) are independent and both apply after this edit (skill body text changes + manifest hash changes).
- `ai-docs/spec/workflow-skills.md#L102-L111` — exact current paragraph anchored `{#260724-prefer-subagent-fresh-spawn-delegation-posture}`:
  > Under this maximum-delegation posture the lead delegates every payload to a fresh, self-contained subagent by default; the sole carve-out is that authoring or mutating a durable artifact (ticket, spec) stays with the session that already holds the authoritative context for the decision — the lead when it was settled in the lead conversation, or the delegated subagent's own continuing session when settled there — never a separate fresh spawn working only from an after-the-fact summary. The earlier context-inheriting fork delegate and its Codex `spawn_agent` fork-fallback wording were removed, leaving two clean delegation poles: the fresh spawn and this context-holder carve-out.
  Only the final sentence (starting "The earlier context-inheriting fork delegate...") needs rewriting; the anchor tag sits on its own line immediately after (`{#260724-prefer-subagent-fresh-spawn-delegation-posture}`) and must be preserved in place.
- `ai-docs/mental-model/workflow-skills.md#L38` — confirmed stale line: "`lead-prefer-subagent` is a maximum-delegation posture: every delegated task routes to a fresh spawn built from named artifacts plus general constraints... The sole inline-execution carve-out is a central authoring/mutation whitelist..." Matches the contract's report-only note; do not edit, just include in the final report.
- `ai-docs/ref/skill-authoring.md#L65-L67` — invariant checklist to apply to the changed paragraphs: Falsifiable, Actionable, One line (paragraph-form prose here, not a bulleted Invariants list — the file predates that convention as an inline-body exception; apply the checklist's spirit to each sentence rather than a literal one-line format), Context-free, Non-redundant, Doctrine-aligned. No formal `Invariants:`/`Constraints:` block exists in this file to retarget.
- Risk signal: none of the three required edits touch Layer 1/2 (no MCP tool call routing in this skill), so no Layer-ownership conflict. No public-interface or test-surface risk — this is prose-only.

## Implementation Plan

1. Read `ai-docs/ref/skill-authoring.md` (already surveyed above) before editing; apply its invariant checklist mentally to each new/changed sentence in steps 2-3.
2. Edit `agents-plugin/skills/lead-prefer-subagent/SKILL.md`: replace the paragraph at L12 with the exact verbatim replacement text from the contract, then insert the exact verbatim new paragraph immediately after it (both quoted in full in the "EXACT REQUIRED EDITS" section of the inline contract — copy them character-for-character, including the bold markers `**new**` / `**opens with**`). Do not touch any other line in the file. Verify no forbidden token ("mercenary", "SendMessage", `ws:full-only`, `wsflow-only`, `lead-write-code`, `lead-write-skeleton`, `lead-salvage`, `lead-skill-authoring") is introduced.
3. Edit `ai-docs/spec/workflow-skills.md`: within the paragraph anchored `{#260724-prefer-subagent-fresh-spawn-delegation-posture}` (L102-L111), replace only the final sentence ("The earlier context-inheriting fork delegate... this context-holder carve-out.") with new prose that (a) keeps the still-true clause that the context-inheriting fork delegate and its Codex `spawn_agent` fork-fallback wording were removed, and (b) states the continuation affordance and its criterion (same work item the delegate already owns -> continue; new work item, or judgment that must not inherit the prior agent's conclusion -> fresh spawn). Keep the anchor tag on its own trailing line, unchanged. This sentence is not verbatim-mandated — word it consistently with the SKILL.md wording from step 2, kept compact (spec prose style, not skill-imperative style).
4. From `agents-plugin-tool/`, run `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror` to regenerate `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md`. Do not hand-edit that file.
5. From `agents-plugin-tool/`, run `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -count=1` to regenerate `agents-plugin/skills/manifest.json`. Do not hand-edit that file.
6. Inspect `git diff` (or `git status` + targeted diffs) to confirm: `agents-plugin/skills/lead-prefer-subagent/SKILL.md` and `ai-docs/spec/workflow-skills.md` carry the hand edits; `agents-plugin-wsflow/skills/lead-prefer-subagent/SKILL.md` and `agents-plugin/skills/manifest.json` changed only as regen output (byte-identical to what the regen tests produce, no manual touch).
7. In the final report to the lead, flag the `ai-docs/mental-model/workflow-skills.md#L38` staleness per the contract's "NOTE FOR DOC PASS" — do not edit it in this commit.

## Verification Plan

- `cd agents-plugin-tool && go test ./...` — must pass; covers `TestWsflowSkillsMirrorUpToDate`, `TestSkillsManifestDriftIsVisible`, and other mirror/manifest drift guards.
- `python3 -m unittest discover agents-plugin-wsflow/tests` (from repo root) — wsflow package suite; checks the distributed skill bundle stays a valid thin/inline shim and free of forbidden references.
- `git diff --stat` review to confirm the mirror + manifest files changed only via the regen tests in steps 4-5, never by hand.

## Escalations

- None.
