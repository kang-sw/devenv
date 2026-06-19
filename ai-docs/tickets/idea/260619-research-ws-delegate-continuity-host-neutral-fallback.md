---
title: Playbook delegate-continuity tip assumes SendMessage; needs host-neutral fallback
related:
  260605-research-ws-native-subagent-pivot: delegate/adapter-boundary direction this surfaces under
related-mental-model:
  - workflow-skills
---

# Playbook delegate-continuity tip assumes SendMessage; needs host-neutral fallback

## Background

Dogfood surprise (2026-06-19, during lead-implement of
`260619-feat-ws-layered-config-scope-substrate`).

`lead-implement` and `lead-write-spec` both append a continuity tip:

> When the subagent returns an agent id, use `SendMessage(to: <agentId>)` to send
> follow-up messages to the same agent rather than spawning a new one.

And the partitioned-review path explicitly says to **relay to the implementer**
(`Review relay prompt`, `Re-review prompt`) across fix cycles — which presumes
the original delegate can be resumed with its context intact.

In a default Claude Code harness, `SendMessage` is NOT available; it is gated
behind an experimental "team" feature. When it is absent there is no documented
fallback, so the relay step has no resume path. The relevant delegate also may
be a native subagent (not a ws mercenary), so `ws.mercenary.interrupt` does not
apply either.

## Observed Impact

Worked around by spawning a FRESH fix-cycle agent and pointing it at the review
findings files + brief + plan + the committed diff. No information was lost
because those artifacts are self-contained (consistent with the stateless-
delegate intent). But:

- The fresh spawn re-pays cold-context cost each fix cycle.
- The playbook text instructs an idiom that silently does not exist on the
  default host, so an operator following it literally hits a dead end.

## Direction / Open Questions

- The continuity tip is a Claude-specific adapter behavior; host-neutral guidance
  should treat resume-same-delegate as an *optional optimization* with an
  explicit fallback: "if no same-agent resume channel is available, spawn a fresh
  delegate with the findings/brief/plan paths — these are self-contained."
- Should the relay prompts (lead-implement) state the fresh-spawn fallback
  inline so partitioned-review fix cycles never assume SendMessage?
- Is there a host-neutral capability probe (does this harness expose an
  agent-resume channel?) the playbook could key off, or is prose guidance enough?
- Confirm the mercenary path: for mercenary delegates the resume idiom is
  `ws.mercenary.call`/`interrupt` on the same name — that IS host-neutral and
  already works. The gap is specifically the NATIVE-subagent resume idiom.

## Notes

- This is an adapter-boundary item under epic `260605-epic-ws-playbook-factory-pivot`;
  the stateless-delegate design already makes the fresh-spawn fallback correct,
  so this is mostly a docs/guidance fix plus deciding whether to probe capability.
