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
- The hook only needs to fire for the main/lead session. Confirmed
  `SessionStart` never fires for subagents, so no subagent-side
  suppression logic is needed on the Claude side.

## Open question: Codex-side equivalent

`ai-docs/ref/codex-integration.md` already documents a Codex `SessionStart`
hook ("fires on session start and resume... developer context injection"),
gated behind `-c features.codex_hooks=true`. Unresolved before implementation:

- Does Codex's `SessionStart` distinguish a post-compaction resume from a
  plain fresh start/resume the way Claude Code's `compact` matcher does, or
  is compaction invisible at the hook layer on Codex?
- Can `features.codex_hooks=true` and the hook config be bundled inside a
  Codex-facing plugin artifact (`agents-plugin`'s `.codex-plugin/` tree,
  or `agents-plugin-tool`'s Codex adapter), or does it require a
  per-invocation `-c` flag the ws Codex launcher would need to inject on
  every `codex exec` call?
- Whether Codex subagents (however Codex models sub-work — exec
  sub-processes, not necessarily a Task-tool equivalent) could receive a
  `SessionStart`-fired reminder unintentionally, and how to detect/suppress
  that if so.

If no clean Codex-side equivalent exists, decide whether to (a) keep a
thin host-neutral fallback skill for Codex only while fully retiring
`lead-revive` on Claude, or (b) accept a temporary host asymmetry and track
the Codex gap as a follow-up.

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

### Phase 2: Codex-side research and parity decision

- Resolve the open questions above against the actual Codex CLI behavior
  (test `-c features.codex_hooks=true` plus a `SessionStart` hook against a
  real compaction-equivalent resume, if Codex has one).
- Implement the Codex-side equivalent if feasible, or document the
  accepted asymmetry and close the gap as a tracked follow-up if not.

