---
title: Claude mod mailbox push stays on a key the lead never saw when the manual section does not reach the model
related:
  261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind: reverses its "sentinel is never adopted" decision
  261018-feat-claude-mod-ws-manual-injection: origin of the minted key and the system prompt section
---

# Claude mod mailbox push stays on a key the lead never saw when the manual section does not reach the model

## Background

In a ws project the Claude mod mints a lead session key at `session.start`,
binds the mailbox push to it through `keyChanged`, and carries the key in the
`ws:workflow-manual` system prompt section (`manual.ts` `setup`,
`register.ts` `prompt.compose`). The bind happens whether or not the section
reaches the model.

On an organization Claude account the security policy sometimes keeps the
section from reaching the model. The lead then sees no key, calls
`workflow_manual(session_key: "obsidian-latch", root)`, and ws-mcp mints a
second key and returns it under `## Session Key`
(`agents-plugin-tool/internal/mcp/workflow_manual.go`, search
`freshBootstrapKey`). `revivedKeyToAdopt` skips the sentinel, so the push
stays bound to the first key, which the lead never saw; only a later
`workflow_manual` call with the second key adopts it. In a ws project the
mod registers no mailbox watch tool, so the lead has no way to bind the push
itself.

The lead also often starts a session with `ws:lead-use-mailbox`, whose skill
shim loads only its playbook and never bootstraps a key.

## Decisions

- **Adopt the sentinel's minted key.** A main-thread `workflow_manual` call
  with `obsidian-latch` whose result carries a `## Session Key` line adopts
  that key as the conversation's key, and the push rebinds to it. This
  reverses `261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind`'s
  "The sentinel is never adopted" on the user's direction (2026-10-23).
  A sentinel result without a `## Session Key` line adopts nothing.
- **Through the existing adoption flow.** The minted key goes through the
  same re-login, key switch, key entry rewrite, and cache rebuild a revive's
  key does. Rejected: rewriting sentinel calls to the conversation key as pi
  does (a second code path, and it has no key to rewrite to when the mod's
  own setup failed).
- **`ferrule` is not adopted.** It starts an independent track by contract,
  so a main-thread `ferrule` key leaves the push where it is. Rejected:
  adopting a `ferrule` key while the model has not yet used the current key
  (an extra heuristic for a case the sentinel already covers).
- **The adoption notice names the push.** When the push runs here, every
  adoption notice, a revive's included, gains a sentence that the mailbox
  push is bound to the new key and no mailbox wait is needed. Without it, a
  lead whose system prompt lacks the mailbox line falls through
  `arm-the-wait.claude.md` to a redundant background wait. Rejected: the
  sentence on sentinel adoptions only (two notice shapes for one fact).
- **`lead-use-mailbox` bootstraps like `lead-discuss`.** Both the ws and the
  wsflow `lead-use-mailbox` skill shims take the `lead-discuss` two-call
  parallel form: `playbook.read` together with
  `workflow_manual(session_key: <your key or "obsidian-latch" if fresh>,
  root: <absolute worktree path if fresh>)`, then the returned procedure. The
  wsflow shim keeps its `wsflow/` prefix and its blocker sentence.
- **The CLI wait is a one-line fallback.** `arm-the-wait.claude.md` step 3
  shrinks to one line for a Claude Code without the mod (the only Claude host
  where neither the push nor the watch tool exists). Steps 1, 2 and 4 stay.
  Rejected: deleting step 3 (a Claude Code without the mod would have no
  wake path).

## Constraints

- Main thread only, in a ws project, through the existing `rewriteRead`
  filter: a subagent's sentinel call adopts nothing.
- Python-less environments are not a target; the push probe's `python3`
  requirement stays as it is, and the fallback line does not mention it.
- Every copy stays identical: `agents-plugin/hooks/claude-mod/` with its
  wsflow copy, and `arm-the-wait.claude.md` under `agents-plugin/rsrc/`,
  `agents-plugin-wsflow/rsrc/`, and `agents-plugin-pi/rsrc/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Phases

### Phase 1: Adopt the sentinel key and bootstrap from lead-use-mailbox

Implement the Decisions in `agents-plugin/hooks/claude-mod/` (`manual.ts`,
its tests) with the byte-identical wsflow copy, the two `lead-use-mailbox`
skill shims, and every `arm-the-wait.claude.md` copy.

Verification, by unit tests rather than a live organization-account probe
(the policy cannot be reproduced on this machine), in `manual.test.ts` /
`manual-engine.test.ts`: a main-thread sentinel result is adopted and the key
listener is told the new key; a `ferrule` result, a subagent's sentinel call,
and a sentinel result without `## Session Key` adopt nothing; the push
sentence appears only when the push runs. The claude-mod suites and the
skill-shim drift tests pass in every copy.
