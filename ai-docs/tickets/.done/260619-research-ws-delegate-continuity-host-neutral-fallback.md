---
title: Playbook delegate-continuity tip assumes SendMessage; needs host-neutral fallback
related:
  260605-research-ws-native-subagent-pivot: delegate/adapter-boundary direction this surfaces under
related-mental-model:
  - workflow-skills
sage-review: required
completed: 2026-06-30
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

## Finding: a fully host-neutral stateful continuation path already exists (2026-06-19)

Investigated the premise "is continuation impossible when the experimental
feature is off?" Answer: NO. Separate three layers:

1. **Native subagent same-agent resume** — the ONLY path gated by the
   experimental feature. Confirmed absent in the default harness (`SendMessage`
   does not resolve even as a deferred tool). Fallback = fresh-spawn with
   self-contained artifacts: correct, but re-pays cold context.
2. **Mercenary delegates** — host-neutral AND death-resilient, independent of any
   harness feature. The backend runner persists the model session id and resumes
   via the BACKEND CLI's own resume, not the harness:
   - `internal/wsagent/claude.go:82-84` — first call `--session-id <id>`, resume
     `--resume <sessionID>`; id persisted via `OnSessionID` (`claude.go:35-36`).
   - `internal/wsagent/codex.go:144-166` — `codex resume <thread_id>`; thread id
     captured from `thread.started` (`codex.go:227-228`).
   - `internal/wsagent/agent.go:51` — "ws will resume the stored session when the
     backend supports it."
   - Mid-flight steering = `ws.mercenary.interrupt` durable inbox file (not OS
     signal), delivered at hook/check-inbox boundaries.
   - Registry + session store persist on disk (SQLite + files), so worker death,
     lead restart, and MCP server restart all recover (`mcp-runtime.md:41`,
     `named-agent-runtime.md:33`).
3. **Lead itself** — continuation = compaction/transcript replay (host-native).

Implication: the design is NOT built on the experimental feature. The
load-bearing continuation primitive is the mercenary path (layer 2), which is
host-neutral. `prefer_mercenary` mode is precisely the lever that routes
delegates onto it (`mcp-runtime.md:40`) — i.e. the continuation guarantee is
already a first-class, soon-to-be-tunable config item under this epic.

The real gap is narrower than originally framed: the playbook's DEFAULT delegate
dispatch is native-subagent, whose same-agent resume needs the experimental
feature. The fix is twofold and both halves are docs/routing, not new runtime:

- State the fresh-spawn fallback inline in the relay/re-review prompts so an
  operator never dead-ends on `SendMessage`.
- Note that routing fix-cycle delegates through mercenaries (or running under
  `prefer_mercenary`) eliminates the cold-context re-pay entirely, because the
  mercenary backend-session resume IS a host-neutral stateful continuation.

## Notes

- This is an adapter-boundary item under epic `260605-epic-ws-playbook-factory-pivot`;
  the stateless-delegate design already makes the fresh-spawn fallback correct,
  so this is mostly a docs/guidance fix plus deciding whether to probe capability.
- Original framing undersold layer 2: it treated fresh-spawn as the only fallback
  and missed that a fully host-neutral *stateful* resume (mercenary backend
  session) already ships. The guidance fix should mention both.

## Decision (260629 sweep)

Fix: Replace the SendMessage-assuming delegate-continuity tip with host-neutral guidance. The tip currently tells callers to use SendMessage to resume a subagent; this assumes Claude Code harness features. Replace with: "To continue a delegate, send a follow-up prompt to the same agent using the host's native continuation mechanism (e.g. SendMessage on Claude Code). If no such mechanism exists, re-spawn with a recap of the prior exchange." Docs-only change; no runtime behavior change.


## Resolution (2026-06-30)

Replaced SendMessage-assuming continuity tip in lead-implement with host-neutral guidance: use host's native continuation mechanism; re-spawn with recap if unavailable. Added note that mercenary delegates provide host-neutral stateful resume.
