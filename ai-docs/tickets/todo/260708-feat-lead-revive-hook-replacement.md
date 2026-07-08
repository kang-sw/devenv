---
title: "Replace lead-revive skill with a plugin-bundled post-compaction hook"
related:
  260708-research-lead-revive-low-salience: prerequisite
sage-review-design: required
---

# Replace lead-revive skill with a plugin-bundled post-compaction hook

## Background

`lead-revive` is a description-only, self-triggered skill: its `SKILL.md`
tells the model to invoke it "BEFORE any other ws lead skill" whenever a
session was compacted or continued. Dogfooded live (see
`260708-research-lead-revive-low-salience`): a session compacted, and the
lead jumped straight into `/ws:lead-ship` instead, silently running the
rest of the session — including a later `/ws:lead-tune` call — without
`workflow_manual` in context. A stored `UserPreferenceSection` override
went unenforced as a direct result.

Follow-up research confirmed a structural alternative exists on the Claude
Code side: a `SessionStart` hook with matcher `compact` fires exactly at
session-resume-after-compaction, independent of what skill the user's next
message names, and can be bundled at the plugin level via
`hooks/hooks.json`. It was also confirmed that `SessionStart` does not fire
for Task-tool subagents at all (subagents get `SubagentStart`/`SubagentStop`
instead), so a compact-triggered reminder cannot leak a lead-only
`workflow_manual` instruction into subagent context.

Direction: delete the `lead-revive` skill entirely and replace its job with
a hook-injected reminder (or direct primitive re-invocation) fired by the
harness itself at resume time, removing reliance on the model noticing and
complying with a plain-English trigger condition.

## Decisions

- Enforcement moves from "skill description asks the model to self-trigger"
  to "harness hook fires structurally at the compaction-resume boundary,"
  per this repo's general preference for structural correctness over
  attention-dependent conventions.
- The hook only needs to fire for the main/lead session on Claude.
  Confirmed `SessionStart` never fires for subagents there, so no
  subagent-side suppression logic is needed on the Claude side.
- **`workflow_manual`'s existing session-key gating already provides all
  the safety this design needs — no new "safe manual" or delegation-only
  variant is required.** Confirmed in
  `agents-plugin-tool/internal/mcp/workflow_manual.go` and `server.go`:
  a resolvable delegate/leaf key is rejected pre-handler as
  `workflow_manual` is on the lead-only tool list (`isLeadOnlyTool`); an
  unresolvable/unknown key gets a quiet "no restorable state" notice with
  no manual body; only a valid lead key or the `obsidian-latch` sentinel
  renders the real manual. This means the hook (or the model acting on the
  hook's reminder) can shape its call as `workflow_manual(<session-key-if-
  known>)` and simply let the existing gate decide the outcome — no branch
  logic needs to live in the hook itself.
- This closes the subagent-leak question for Codex too: since Codex uses
  the same `workflow_manual` handler, firing the hook unconditionally
  (main session or any Codex sub-process) is safe by construction — a
  subagent/child context either has no resolvable lead key (quiet notice)
  or a non-lead key (rejected), never a leaked manual.
- Codex-side hook does not need to distinguish "resumed after compaction"
  from a plain session start/resume. Install the `SessionStart` hook
  unconditionally on every Codex session start/resume regardless of
  cause; the worst case is a redundant reminder on ordinary starts, which
  is harmless, versus the real risk of missing the compaction case if
  Codex cannot signal it distinctly.

## Open question: Codex-side plugin bundling

`ai-docs/ref/codex-integration.md` documents a Codex `SessionStart` hook
("fires on session start and resume... developer context injection"),
gated behind `-c features.codex_hooks=true`. Unresolved before
implementation:

- Can `features.codex_hooks=true` and the `SessionStart` hook config be
  bundled inside a Codex-facing plugin artifact (`agents-plugin`'s
  `.codex-plugin/` tree, or `agents-plugin-tool`'s Codex adapter), or does
  it require a per-invocation `-c` flag the ws Codex launcher would need
  to inject on every `codex exec` call?
- Whether a "known session key" is resolvable at all from inside a hook's
  shell command context on either host (e.g. by querying ws's session
  store for the current root's most recent lead key), or whether the hook
  can only ever emit a generic reminder and must leave key resolution to
  the model's own next turn.

If no clean Codex-side plugin-bundled option exists, decide whether to
(a) require a per-invocation `-c` flag injected by the ws Codex launcher
wrapper instead, or (b) accept a temporary host asymmetry and track the
Codex gap as a follow-up.

## Phases

### Phase 1: Claude-side hook + lead-revive removal

- Add `agents-plugin/hooks/hooks.json` (or the correct plugin-level hook
  manifest location) with a `SessionStart` hook, matcher `compact`, that
  reminds the lead to call `ws/workflow_manual(session_key: <preserved
  key>)` before continuing.
- Delete `agents-plugin/skills/lead-revive/` and its `agents-plugin-wsflow`
  mirror; remove references to `lead-revive` from
  `ai-docs/spec/workflow-skills.md` and any skill routing docs that name it.
- Regenerate the wsflow skill/manifest mirrors as needed.
- Verify: trigger `/compact` in a live session and confirm the hook's
  reminder text actually surfaces before the next model turn, without
  manual invocation of any skill.

### Phase 2: Codex-side hook, installed unconditionally

- Resolve the plugin-bundling open question above against actual Codex CLI
  behavior (test `-c features.codex_hooks=true` plus a `SessionStart` hook
  either bundled in the Codex plugin artifact or injected by the ws Codex
  launcher wrapper).
- Wire the Codex `SessionStart` hook unconditionally (every start/resume,
  no compaction-only gating) to the same `workflow_manual(<session-key-if-
  known>)`-shaped call as Phase 1, relying on the existing session-key
  gate for safety rather than any new host-specific branch logic.
- Verify: a Codex session resume (with and without a resolvable lead key)
  produces the expected outcome for each case — full manual, quiet
  no-restore notice, or lead-only rejection — matching the behavior
  already confirmed for `workflow_manual` in Phase 1's research.

