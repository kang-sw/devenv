---
title: Pi-extension-like lead compaction on Claude Code via a plugin mod
related:
  261016-research-claude-mod-mailbox-active-push: Claude mailbox mod is the first mod-path validation this research waits on
  261002-feat-pi-lead-ws-owned-compaction: pi ws-owned compaction being considered for porting
  261006-feat-pi-compaction-advisory-standing-intent-milestones: pi compaction timing rules being considered for porting
  261004-feat-pi-explicit-post-compaction-continuation: pi post-compaction continuation being considered for porting
---

# Pi-extension-like lead compaction on Claude Code via a plugin mod

## Background

On pi, the adapter owns lead compaction timing: the lead is nudged at an
advisory threshold, chooses its own compaction boundary, compacts through a
ws-owned lever, and states an explicit post-compaction continuation
(`261002-feat-pi-lead-ws-owned-compaction`,
`261006-feat-pi-compaction-advisory-standing-intent-milestones`,
`261004-feat-pi-explicit-post-compaction-continuation`). In dogfooding this
custom compaction timing is the pi feature with the largest observed effect.

Claude Code had no comparable control: ws only observed Claude compaction
(the `SessionStart(compact)` command hook in `agents-plugin/hooks/hooks.json`
nudges a compacted lead to run `lead-revive`). Claude Code now ships plugin
mods (plugins of function hooks: a TS/JS module named under `modules` in a
plugin's `hooks/hooks.json`, exporting `register(on, options)`), whose engine
API exposes compaction. This research asks whether, and how much of, pi's
compaction timing can be reproduced on Claude through such a mod.

## Claude mod compaction surface

Read from the engine's own declaration file for Claude Code 2.1.293
(`plugin-authoring` skill, `types/claude-code.d.ts`) and the skill's
`reference.md`; nothing below was exercised live.

- `$.session.usage()` returns `context.tokens`, `context.window`, and
  `context.percent` as the status line has them; the plain call costs
  nothing. The declaration's own example is
  `if ((context.percent ?? 0) >= 85) await $.session.compact()`.
- `$.session.compact({ instructions })` compacts on the plugin's initiative
  (trigger `plugin`). It rejects while a turn runs, so a plugin-chosen
  boundary has to be scheduled at turn end (`turn.complete`) or idle.
- The `session.compact` hook sees every trigger, `manual | auto | plugin |
  precompute`. A hook can veto with `{ skip: reason }` (the conversation stays
  as is, one line says why), rewrite `instructions` (what the summarizer is
  told), or rewrite `messages` (what is summarized). Its result type is the
  compaction itself, `{ messages, tokensBefore?, tokensAfter?, usage? }` ("there
  is no summary string: the summary is a message"), so a hook answering its
  own compaction without `next` appears type-legal but is untested.
- `$.prompt.submit({ text })` queues a prompt that starts its own turn once the
  session is idle, so a post-compaction continuation (for example a revive
  with the preserved session key) can be injected after the compaction lands.
- `$.tool.register` declares a model-callable tool served by the mod's own
  `tool.call` hook (named `mcp__<plugin>__<name>`, not an MCP server tool), so
  an agent-invoked compaction lever is expressible.
- `$.model.fork({ prompt })` asks one tool-less question over the session's own
  transcript, prompt-cache-served, which is a possible building block for a
  custom summary.

## Outcome Ledger

### Verified Findings

- The Claude Code 2.1.293 mod API declares `$.session.usage()`,
  `$.session.compact()`, the `session.compact` hook with `manual | auto |
  plugin | precompute` triggers and a `{ skip }` veto, `$.prompt.submit`, and
  `$.tool.register`, with the semantics listed under
  "Claude mod compaction surface" (source: that build's declaration file and
  `reference.md`).
- `$.session.compact()` rejects while a turn runs (declaration doc comment).
- `$.session.compact()` takes only `instructions`; it starts a compaction that
  the engine's summarizer writes. The only declared path for supplying the
  compacted content ourselves is a `session.compact` hook returning its own
  `{ messages }` without `next`, which is untested.
- The mod API is documented as early access that "moves between releases";
  the declaration file, not the reference note, is the authority per build.
- The only current ws Claude compaction integration is the observe-only
  `SessionStart(compact)` revive nudge in `agents-plugin/hooks/hooks.json`.
- Pi's advisory and milestone nudges already cost a turn each: they open a
  boundary at `agent_end` and tell the lead to end the turn without replying
  when the boundary does not fit (`agents-plugin-pi/src/lead-compaction.ts`,
  the milestone message text). A `$.prompt.submit` nudge on Claude therefore
  matches pi's cost.
- The boundary-steer loss of
  `261015-bug-pi-compact-lever-drops-boundary-steers` comes from Pi's
  `AgentSession.compact()` calling `abort()` first without clearing the
  steering queue. Claude's `$.session.compact()` rejects during a turn instead
  of aborting it, so that loss path does not carry over by construction.

### Confirmed Decisions

- Research only for now; no actionable child until the Claude mailbox mod
  (`261016-research-claude-mod-mailbox-active-push`) has
  validated the mod path in `ws`/`wsflow`. Compaction is tracked separately
  from the mailbox mod because a compaction hook mutates the conversation, so
  a mod-API break has a larger blast radius than a lost mailbox push.
- First-step scope mirrors pi's timing, not pi's summary ownership:
  - Advisory (50%) and milestone nudges are `$.prompt.submit`ted at
    `turn.complete`, driven from `$.session.usage()`.
  - A mod-registered lever tool records a compaction request and a one-line
    continuation (for example "continue Phase 2 implementation"). After the
    turn ends the mod runs `$.session.compact()`, then submits the
    continuation, since an idle session after compaction has nothing else to
    resume it.
  - The compacted content is left to the engine's summarizer. The lead
    prepares no summary content, because `compact()` starts a summarization of
    its own and a prepared body would be summarized a second time. No fixed
    custom instructions are injected in the first step either.
  - Pi's hard cut maps to Claude's native auto compaction; the mod does not
    veto `auto`.

### Proposals

- Instructions injection on every trigger, so automatic compaction also
  receives ws-specific summary guidance (pi's auto compaction never received
  custom instructions; see `261002-feat-pi-lead-ws-owned-compaction`).
  Deferred past the first step; revisit only on evidence that the plain
  engine summary loses ws state.
- A fully custom summary through a `session.compact` hook returning its own
  `{ messages }`, which would avoid double summarization of prepared content.
  Revisit only if the engine summary proves insufficient.

### Open Questions

- Ordering against the mailbox mod (to measure live): when a compaction is
  scheduled for idle and a mailbox `$.prompt.submit` is already queued, which
  runs first? If the submit wins, `compact()` rejects and retries at the next
  `turn.complete`; under steady mail, check that compaction is not starved
  until native auto fires. Check for lost prompts as well.
- Does a `session.compact` hook that returns its own `{ messages }` without
  `next` produce a valid compaction? Relevant only to the deferred custom
  summary proposal.
- How does the existing `SessionStart(compact)` revive nudge interact with a
  mod-injected continuation (double revive prompts)?
- What is the failure mode when the mod API breaks on a Claude Code update:
  does Claude's native auto-compaction resume untouched, and how is the
  degradation detected (for example gating on `$.session.version()`)?
- Do the mod and the existing command hooks both run at runtime from one
  `agents-plugin/hooks/hooks.json` (`hooks` plus `modules`)? The mailbox mod
  probe showed `claude plugin validate` accepts the combined file; runtime is
  unexercised there too.

### Rejected Alternatives

- **Vetoing `auto` compaction below a hard ceiling.** Preempting at an
  agent-chosen boundary needs no veto, and leaving native auto untouched as
  the hard cut avoids the "prompt too long" deadlock risk (also trigger
  `auto`) and degrades to stock Claude behavior if the mod breaks.
- **Lead-prepared summary content passed through `compact()`.** `compact()`
  starts its own summarization, so the prepared body would be summarized
  again: double cost and an extra lossy pass.
