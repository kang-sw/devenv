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

Resolved (2026-07-08) against the official Codex hooks doc
(`developers.openai.com/codex/hooks`, recorded in
`ai-docs/ref/codex-integration.md`): Codex hooks are enabled by default and
explicitly support plugin-bundled configuration via `hooks/hooks.json` inside
the plugin root, the same shape as Claude's plugin-level hook manifest. No
per-invocation `-c` flag injection by the ws Codex launcher is needed for
enablement or bundling. The prior `-c features.codex_hooks=true` requirement
recorded from a 2026-05-04 CLI 0.128.0 smoke test may be stale for current
CLI versions — re-verify the exact enablement state (default-on vs. flag-gated)
against the installed CLI version during Phase 2 implementation, since a wrong
assumption here would silently no-op the hook rather than error.

`SessionStart` fires at thread (main-session) scope; subagent/sub-process
starts fire a separate `SubagentStart` event instead, mirroring the
Claude-side split — so the existing "no subagent leak" conclusion holds
structurally on Codex too, not just via `workflow_manual`'s gating.

Still open before implementation:

- Whether a "known session key" is resolvable at all from inside a hook's
  shell command context on either host (e.g. by querying ws's session
  store for the current root's most recent lead key), or whether the hook
  can only ever emit a generic reminder and must leave key resolution to
  the model's own next turn.

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

- Bundle `hooks/hooks.json` in the Codex plugin artifact per the confirmed
  default-enabled, plugin-bundlable mechanism; re-verify the exact enablement
  state (default-on vs. flag-gated) against the installed Codex CLI version
  before relying on it, and resolve the still-open session-key-resolvability
  question above.
- Wire the Codex `SessionStart` hook unconditionally (every start/resume,
  no compaction-only gating) to the same `workflow_manual(<session-key-if-
  known>)`-shaped call as Phase 1, relying on the existing session-key
  gate for safety rather than any new host-specific branch logic.
- Verify: a Codex session resume (with and without a resolvable lead key)
  produces the expected outcome for each case — full manual, quiet
  no-restore notice, or lead-only rejection — matching the behavior
  already confirmed for `workflow_manual` in Phase 1's research.

