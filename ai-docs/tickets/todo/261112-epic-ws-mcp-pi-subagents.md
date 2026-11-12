---
title: "ws MCP spawns Pi subagents directly (pi.spawn and friends) with a tune-pi playbook"
sage-review-design: required
related:
  261110-epic-ws-dashboard-agent-host: wsagent PiBackend already runs Pi in RPC mode from Go
  260908-epic-ws-pi-subagent-conversation-view: Pi extension child view this may overlap
  261112-bug-ws-hosted-pi-subagents-missing-from-dashboard: hosted Pi child visibility
  261112-feat-pi-subagent-long-shell-auto-background: Pi subagent shell behaviour
---

# ws MCP spawns Pi subagents directly (pi.spawn and friends) with a tune-pi playbook

## Background

User request (2026-11-12, verbatim intent):

- Add to the ws/wsflow MCP a way to call a Pi agent as a subagent:
  `ws/pi.spawn(tier: "small|medium|large|xlarge")`, `pi.send`, and the other
  calls it needs. It must not depend on the user's local Pi setup.
- A new playbook `tune-pi` sets Pi's model and effort for each tier (Pi's
  tiers differ from Claude's).
- Motivation: the user drives Pi far more often as a subagent than through
  the Pi CLI, and running Pi as a co-lead has been frustrating; folding Pi
  spawning into the ws MCP lets any lead use Pi as a worker.
- Pi now ships MCP support (1.0, user-reported, unverified here), so the
  feature could live in an independent form rather than in the Pi extension,
  which is mostly rendering.
- Problem the user named: if Pi sessions then see the ws MCP, the Pi CLI
  would also be offered Pi spawning; the Pi agent may need it removed.

Evidence read while drafting:

- `agents-plugin-tool/internal/wsagent/pi.go` `PiBackend` runs Pi in RPC
  mode, one process per conversation, with ambient extensions, skills,
  context files, and trust-gated project config off and `--tools` as the
  whole tool surface; `piEfforts` lists Pi's thinking levels
  (off..max).
- `agents-plugin-tool/internal/wsconfig/config.go` already resolves
  `agents.tier` per harness, including a `pi` bucket (model, effort).
- `agents-plugin-pi/src/delegation-policy.ts` names the Pi extension's own
  child-management tools: `ws-agent-spawn`, `ws-agent-send`,
  `ws-agent-list`, `ws-agent-stop`, `ws-agent-transcript`, `explore`.

## Scope

- ws MCP tools that let a lead on any harness spawn, steer, and collect a Pi
  subagent: `pi.spawn(tier, prompt, cwd?)`, `pi.send`, `pi.list`, `pi.stop`,
  `pi.transcript`, `pi.wait`.
- Per-tier Pi model and effort, set through a new `tune-pi` playbook.
- Pi-harness sessions do not see `pi.*`.

## Non-Scope

- Moving the Pi extension's own ws tool access onto Pi's native MCP (the
  extension keeping only rendering) is a separate epic (467).
- Retiring the Pi extension's `ws-agent-*` child tools in favour of `pi.*`
  is not part of this epic; see `## Post-Landing`.

## Child Tickets

Planned, in this order (470):

- Planned: Claude-lead path - the `pi.*` tools, tier resolution, mailbox
  report plus `pi.wait`, dashboard registration, the Pi version gate, and the
  wsflow knob.
- Planned: `tune-pi` playbook (ws and wsflow mirrors), with `lead-tune`
  pointing to it.
- Planned: hide `pi.*` from Pi-harness sessions.

## Cross-Child Decisions

1. Placement: an epic in `todo/`, children cut later. The work spans MCP
   tools, a playbook, config, and the Pi package. (459)
2. Tier resolution reuses the existing `agents.tier` `pi` harness bucket
   (model + effort per tier); `tune-pi` writes that bucket. Rejected: a new
   Pi-only tier table. (460)
3. The tools ship in both ws and wsflow from the one MCP binary. (461)
4. The ws MCP server launches Pi children through the wsagent `PiBackend`
   (RPC mode, ambient extensions, skills, context files, and project config
   off), so the feature does not depend on the user's local Pi setup.
   Rejected: driving the Pi extension's `ws-agent-spawn`. (462)
5. Tool set: `pi.spawn(tier, prompt, cwd?)`, `pi.send`, `pi.list`,
   `pi.stop`, `pi.transcript`, `pi.wait`, mirroring the extension's
   lifecycle set. Rejected: a new verb set. (463)
6. A finished or blocked child is announced through the calling lead's
   mailbox, which already wakes an idle lead, plus a blocking `pi.wait` for
   synchronous use. Rejected: a new push channel (an MCP call cannot push).
   (464)
7. Children launched this way register in the dashboard agent store and show
   in the agents panel. The only UI variant: where an agent row shows the
   blinking blue circle, a Pi child shows a blinking `pi` badge in the same
   colour. Rejected: a separate, invisible registry. (465)
8. Pi-harness sessions do not see `pi.*`; Pi leads keep the extension's
   `ws-agent-*` for now. Rejected: Pi leads switching to `pi.*` now (kept as
   a post-landing nudge); hiding only from Pi subagents. (466)
9. Exposure: on by default in ws; in wsflow off by default behind a config
   knob. In either package the tools run only when a Pi of version 1.0 or
   later is detected. Rejected: on in both packages unconditionally. (468)
10. `tune-pi` is a new playbook (Approval Protocol "ask first", approved
    2026-11-12); `lead-tune` points to it. Rejected: a Pi section inside
    `lead-tune`. (469)
11. The first child is the Claude-lead path alone; `tune-pi` and Pi-session
    hiding follow as their own children. Rejected: one child for all three.
    (470)

## Completion Criteria

- Done: a lead on a non-Pi harness can spawn, steer, and collect a Pi
  subagent through ws MCP tools, with per-tier model and effort set through
  `tune-pi`, and Pi-harness sessions do not see `pi.*`.
- Dropped: Pi's RPC mode makes a ws-hosted runner unworkable.
- Deferred: the Pi extension's move to native MCP (separate epic, (467));
  Pi leads moving to `pi.*` (`## Post-Landing`).

## Post-Landing

- Nudge: once `pi.*` has proven out, propose that Pi leads switch to `pi.*`
  and the extension retire its `ws-agent-*` spawn tools, so one runner serves
  every harness ((466) alternative b, user 2026-11-12).
