---
title: "Replace lead-revive skill with a plugin-bundled post-compaction hook"
related:
  260708-research-lead-revive-low-salience: prerequisite
spec: 260626-post-compaction-session-restoration
sage-review-design: completed
sage-review-completeness: completed
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
- No pre-compaction ("remember the session key") hook is needed on the
  Claude side. Confirmed against the hooks reference: `PreCompact` fires
  before compaction (matchers `manual`/`auto`, plugin-bundlable) but its
  output is limited to the top-level `decision: block`/allow pattern — it
  is not in the documented set of events that support
  `additionalContext`, so it cannot inject a "preserve this key" reminder
  into the compaction process. This is moot for *correctness*: the hook
  design does not depend on the compaction summary reliably preserving the
  session key — an unrecoverable key already falls back to the
  `obsidian-latch` sentinel, which `workflow_manual` accepts to bootstrap
  safely without leaking state. It is not moot for *continuity*, though:
  that fallback mints a fresh key and discards the prior session's
  agenda/todo state rather than truly restoring it, so reducing how often
  the key is actually lost is still worth doing — see Phase 0, which is a
  continuity improvement layered on top of an already-safe fallback, not a
  fix for a safety gap.

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

### Phase 0: Session-key preservation tip on `git.commit` responses

Complements the hook-based detection in Phases 1-2 by improving the odds the
session key itself survives into the compaction summary, reducing reliance on
the `obsidian-latch` sentinel fallback — which mints a fresh key and discards
the prior session's agenda/todo state rather than truly restoring it.
`workflow_manual`'s existing `injectSessionKeyLine` (`agents-plugin-tool/internal/mcp/workflow_manual.go:137-144`)
places the "preserve verbatim" reminder once, near the top of a manual reload;
that single placement loses attention salience as the transcript grows past it.

- Add a small shared helper (e.g. `appendSessionKeyTip(text, sessionKey
  string) string`) in `agents-plugin-tool/internal/mcp`, mirroring
  `injectSessionKeyLine`'s phrasing, that appends a trailer line — e.g. `tip:
  preserve this session key: <key> during compaction` — to a tool response.
- Wire it into the `git.commit` handler only (`server.go`, the `"git.commit"`
  case around line 895-919, where `commitKey` is already extracted in scope
  for the existing TODO-summary trailer). `git.commit` is a high-frequency,
  lead-scoped call that tends to land near the natural end of a working
  turn, keeping the key recent in the transcript at the point compaction is
  likely to trigger.
- Do not wire this into other lead-scoped tools (`tickets.move`,
  `agenda.set`, etc.) in this phase; there is no existing shared
  post-processing hook across tool formatters (each has its own `format*`
  function), so broader adoption is a separate, explicitly deferred
  follow-up rather than an implicit expansion of this phase's scope.
- Verify: a test asserting `git.commit`'s returned text contains the tip
  line with the correct session key (a `session_key` is always present at
  this point — `resolveToolRoot` already makes it mandatory before the
  handler produces commit text, so there is no reachable no-key path
  through `git.commit` itself). Unit-test `appendSessionKeyTip`'s own
  empty-key no-op behavior directly instead of through the `git.commit`
  handler.

### Phase 1: Claude-side hook + lead-revive removal

- Add `agents-plugin/hooks/hooks.json` (or the correct plugin-level hook
  manifest location) with a `SessionStart` hook, matcher `compact`, that
  reminds the lead to call `ws/workflow_manual(session_key: <preserved
  key>)` before continuing.
- Delete `agents-plugin/skills/lead-revive/` and its `agents-plugin-wsflow`
  mirror; regenerate `agents-plugin/skills/manifest.json` and the wsflow
  mirrors afterward.
- Update every actual reference site (verified by design review; the
  Background's original "`ai-docs/spec/workflow-skills.md`" guess was wrong —
  that file has zero mentions). Reword each to point at the hook mechanism
  or at `ws/workflow_manual(session_key: ...)` directly instead of naming
  `lead-revive`:
  - `agents-plugin-tool/internal/mcp/workflow_manual.go` (the fail-loud
    no-restore notice text, e.g. around lines 173 and 210, currently tells
    the user to "re-run lead-revive to restore your session").
  - `agents-plugin-tool/internal/mcp/server.go` (the `workflow_state` tool
    description, around line 3647).
  - Tests `session_state_test.go` and `playbook_tools_test.go` (string
    assertions on the notice text above).
  - `ai-docs/spec/mcp-tools.md` and `ai-docs/spec/plugin-runtime.md` (the
    `260626-post-compaction-session-restoration` anchor and the sibling
    `workflow_state` spec entry both name `lead-revive` as the recovery
    mechanism).
  - `ai-docs/mental-model/mcp-runtime.md` and
    `ai-docs/mental-model/workflow-skills.md`.
  - `ai-docs/_index.md`.
  - The six rsrc skill-prose files that mention it: `lead-proceed`,
    `lead-discuss`, `lead-salvage`, `lead-sprint`, `lead-forge-mental-model`,
    `lead-forge-spec`.
  - Run a final `grep -ri lead-revive` sweep across the repo before closing
    this phase to confirm no reference site was missed.
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

