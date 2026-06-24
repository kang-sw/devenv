---
title: Claude Code teammate-mode reshapes native-subagent result collection; manual under-describes it
related:
  260605-epic-ws-playbook-factory-pivot: delegation guidance lives in the pivot's playbook manual
  260605-research-ws-native-subagent-pivot: native-subagent delegation direction anchor
related-mental-model:
  - workflow-skills
  - named-agent-runtime
---

# Claude Code teammate-mode reshapes native-subagent result collection; manual under-describes it

## Background

The lead workflow manual teaches two delegation surfaces: **Scoped Exploration
(native Explore)** — "spawn a host-native exploration worker directly ... collect
the deferred result. For parallel dispatch, spawn multiple concurrent subagents in
a single turn and collect all before synthesizing"
(`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:61-68`) — and
**Persistent agents (`ws.mercenary.*`)**. The manual is intentionally host-neutral
and never uses the word "teammate".

A recent Claude Code harness update surfaces Agent-tool subagents as **"teammates"**:
named, persistent, addressable via `SendMessage`, emitting `idle_notification`
messages. This ticket captures the dogfood gap between the manual's fire-and-collect
native-Explore pattern and the teammate-mode behavior actually observed.

## Observed gap (live dogfood evidence, 2026-06-19)

During a parallel staleness audit, five subagents were spawned in one turn (named,
so they ran as background teammates). The manual's "collect the deferred result"
assumption did not hold cleanly:

- The Agent tool returned only `"Spawned successfully"` (background), not the result.
- Agents emitted `idle_notification` and persisted as addressable teammates after
  finishing, rather than terminating on return.
- Several agents went idle **without auto-delivering their report**; the lead had to
  `SendMessage` to pull each report explicitly (`audit-delegation-bugs`,
  `audit-research-epics`, `audit-git-worktree-bugs`).
- Result delivery was **inconsistent**: some agents pushed their final report to
  `main` automatically; others did not.

Honest caveat: part of this stems from spawning with a `name:` (which selects the
background/teammate path). Anonymous spawns return the final message directly as the
tool result. The manual's "spawn ... and collect" does **not distinguish
named-vs-anonymous** spawns or describe teammate-mode collection mechanics — that
undocumented fork is the core gap.

## Coupling: the ws installer enables teammate mode

This is not incidental to Claude Code. The ws installer explicitly "configures
teammate mode" in Claude settings
(`ai-docs/spec/developer-environment-tools.md:104`). Because ws itself turns the
feature on, ws guidance has standing to address its interaction semantics — at least
as a Claude adapter note.

## Design tension: team-free orchestration history

`ai-docs/tickets/.done/260424-refactor-team-free-orchestration.md` records a
deliberate move to "team-free" orchestration (fire-and-collect, no persistent team).
The teammate model reintroduces persistent addressable agents plus a mailbox, which
is in mild tension with that intent. This is a host UI/UX layer, not a ws contract
change, but the manual's guidance should not silently assume the older fire-and-collect
shape if the enabled host mode behaves differently.

## Disposition options (to decide)

- **(a) Host-neutral robustness instruction.** Amend the native-Explore pattern so the
  worker prompt requires the agent to emit its final report as its terminal message
  (and, for parallel dispatch, to deliver results explicitly), making collection robust
  regardless of host spawn mode. Keeps the manual host-neutral.
- **(b) Claude adapter note.** Add a Claude-specific note describing teammate-mode
  collection (background spawn, idle notifications, `SendMessage` pull, named-vs-anonymous
  difference) as adapter/fallback text, per the host-neutral-first architecture rule.
- **(c) Both** — (a) as the host-neutral default, (b) as a short adapter footnote.

Manual/convention edits are "Ask first" and require `lead-skill-authoring` review, so
this ticket only captures the gap and options; it does not pre-commit a doc change.

## Open questions

- Should the manual track named-vs-anonymous spawn semantics at all, or push that to an
  adapter doc?
- Does `ws.mercenary.*` (MCP-based) avoid this entirely, making native-Explore the only
  affected surface? Confirm mercenary result delivery is unaffected by teammate mode.
- Is "configures teammate mode" in the installer still desired given the team-free
  orchestration intent, or should it be reconsidered?

## Drop Note

Dropped 2026-06-24. Claude teammate-mode API surface is in active flux; the
subagent result-collection contract it describes may change before stabilizing.
Re-evaluate after the feature stabilizes in Claude Code.
