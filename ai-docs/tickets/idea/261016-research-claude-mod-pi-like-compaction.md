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
- The mod API is documented as early access that "moves between releases";
  the declaration file, not the reference note, is the authority per build.
- The only current ws Claude compaction integration is the observe-only
  `SessionStart(compact)` revive nudge in `agents-plugin/hooks/hooks.json`.

### Confirmed Decisions

- Research only for now; no actionable child until the Claude mailbox mod
  (`261016-research-claude-mod-mailbox-active-push`) has
  validated the mod path in `ws`/`wsflow`. Compaction is tracked separately
  from the mailbox mod because a compaction hook mutates the conversation, so
  a mod-API break has a larger blast radius than a lost mailbox push.

### Proposals

- An agent-called lever tool registered by the mod records a compaction
  request; the mod runs `$.session.compact` at the next `turn.complete`/idle
  and then `$.prompt.submit`s the declared continuation.
- A `session.compact` hook vetoes `auto` compaction below a hard ceiling so
  compaction happens at agent-chosen boundaries, mirroring pi's
  advisory-then-hard thresholds; an advisory nudge is driven from
  `$.session.usage()` at turn end.
- Instructions injection on every trigger, so automatic compaction also
  receives ws-specific summary guidance (pi's auto compaction never received
  custom instructions; see `261002-feat-pi-lead-ws-owned-compaction`).

### Open Questions

- Does a `session.compact` hook that returns its own `{ messages }` without
  `next` produce a valid compaction, and is a fully custom summary worth it
  over instructions rewriting?
- Does vetoing `auto` hold when the engine compacts on "prompt too long"
  (also trigger `auto`)? A veto there may fail the request instead of
  deferring compaction; the hard-ceiling fallback must not deadlock.
- How does the existing `SessionStart(compact)` revive nudge interact with a
  mod-injected continuation (double revive prompts)?
- What is the failure mode when the mod API breaks on a Claude Code update:
  does Claude's native auto-compaction resume untouched, and how is the
  degradation detected (for example gating on `$.session.version()`)?
- Can the mod and the existing command hooks share one
  `agents-plugin/hooks/hooks.json` (`hooks` plus `modules`)? Shared with the
  mailbox mod probe.

### Rejected Alternatives
