---
title: "Hosted prompt assembly: daemon-owned workflow manual, playbook body, ambient injection and renderable agent-facing playbooks"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "prerequisite for the native-subagent render variable (the catalog's native subagent removability item per harness); also skill list/injection items"
  261117-feat-ws-agent-tools-mailbox-routing: "agent.spawn surface (candidate C: spawn delivers a system prompt and resolves tier; rendering stays in playbook.render); Decision 6 pairs native-tool removal with this ticket's Never-use steer"
  261117-feat-ws-hosted-compaction-lifecycle: "sibling stage-2 child; shares the daemon injection path (nudge and restore bundle ride the same assembly)"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
---

# Hosted prompt assembly

## Background

Owner 18:35 proposed an "application layer" on top of the common harness
interface: the daemon, not the harness plugin, assembles what a hosted session
reads. This ticket carries four of its parts, placed in stage 2 ("hosted prompt
assembly") by (28)=a:

- (2) the daemon owns the workflow manual;
- (3) the daemon owns the skill body: it inserts the playbook body directly,
  removing today's double pointer (skill invocation, then `playbook.read`);
- B (lead addition, accepted as a candidate by (28)=a): per-turn ambient
  injection by the daemon replaces plugin hooks;
- C (same, revised by owner 18:56): `agent.spawn` is playbook-unaware; it
  delivers `system_prompt_path`/`system_prompt` as an appended system prompt
  ((39), (44)) and resolves tier server-side ((38)=d). Rendering stays only in
  `playbook.render`, which returns the path plus the recommended tier.

It also adds the `{{NativeToolScheme}}` render variable from (29)=ok's owner
addition. Today the shipped playbooks steer delegation toward whatever spawn
tool the CLI plugin offers; once hosted workers lose their native subagent tool
too, the playbooks must still steer harnesses whose native subagent tool cannot
be removed (Codex and Antigravity removability is unprobed, (08)=c, stage 3)
toward `ws/agent.*`.

## Decisions

Confirmed (cite IDs; no other behavior is decided here). Decisions 4 and 5 were
queued and acknowledged by the owner on 2026-11-17 ("32..37=ok").

1. **Application layer parts 2, 3, B, C are stage-2 children (28)=a**, owner
   proposal 18:35. Parts 2 and 3 are the daemon-owned manual and daemon-owned
   skill body; B and C were lead additions accepted as candidates by (28)=a.
2. **Native subagent tool removal extends from hosted leads to hosted workers
   (29)=ok**, a consequence of (06) "delegation is always `ws/agent.*`
   regardless of harness". The removal itself is delivered in
   `261117-feat-ws-agent-tools-mailbox-routing` (Decision 6, Phase 2).
3. **Playbooks state, per harness, "Never use {{NativeToolScheme}}" (owner
   addition with (29)).** `{{NativeToolScheme}}` is a render variable naming
   that harness's native subagent tool, so harnesses whose native subagent
   tools cannot be removed are still steered to `ws/agent.*`. Its source is the
   catalog's native-subagent item per harness; the catalog ticket is therefore a
   prerequisite.
4. **(32), owner ack 2026-11-17: the "Never use {{NativeToolScheme}}" line
   renders only in hosted sessions; the CLI tier renders it empty.** Rejected:
   rendering it everywhere, because the CLI plugin is frozen and CLI-tier leads
   still delegate through the native Agent tool on the shared playbooks.
5. **(33), owner ack 2026-11-17: (28)=a's single "hosted prompt assembly" child
   is split into two tickets:** prompt assembly (2, 3, B, C; this ticket) and
   compaction lifecycle (1, A, ws/compact;
   `261117-feat-ws-hosted-compaction-lifecycle`). Rejected: one ticket, because
   ws/compact is a separate runtime mechanism (session replacement, segment
   chain, record-shape compatibility tier) with different verification.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Shipped playbooks stay harness-neutral: per-harness differences are render
  variables resolved by the daemon (`{{NativeToolScheme}}`), never harness names
  in playbook text. Shipped text must not depend on anything only this
  repository has.
- Read before editing playbooks, skills, `agents-plugin/rsrc/` or
  `agents-plugin-wsflow/`: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md` (apply its invariant checklist to every
  changed Invariants/Constraints line) and `ai-docs/manuals/wsflow-mirroring.md`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; no per-turn file writes or per-event O(N) rereads for injection;
  interactive paths never wait on file I/O. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- The Claude CLI tier is frozen: keep current features, add none. Hosted-only
  behavior must not change what the CLI plugin renders.
- Any cross-version surface touched (an MCP tool or argument, the `/mcp` hello,
  a render-variable manifest field) states its compatibility tier in
  `## Decisions` before `ready/` (AGENTS.md `### Compatibility Tiers`).

## Phases

Plausible breakdown for phase planning; phase text is refined at `ready/`
promotion.

### Phase 1: Render variable and daemon-owned playbook body

Add the `{{NativeToolScheme}}` render variable, sourced from the catalog's
native-subagent item per harness (prerequisite: the catalog ticket's Claude
values; other harnesses fill it as their stage-3 values land), rendering empty
in the CLI tier (32). Add the "Never use {{NativeToolScheme}}" line to
the shared delegation playbooks. The daemon inserts the playbook body directly
into a hosted session's prompt when a skill/command is invoked, removing the
skill-invocation-to-`playbook.read` double pointer.

Deferred: ambient injection, `agent.spawn` system prompt delivery.

Verification: Go tests on remote w5090 for variable resolution per harness and
tier (hosted renders the tool name, CLI renders empty), and for body insertion
without a `playbook.read` round trip; playbook mirroring/shim drift tests per
`wsflow-mirroring.md`; a recorded hosted turn showing the inserted body.

### Phase 2: Daemon-owned workflow manual

The daemon delivers the workflow manual to hosted sessions itself (MCP
initialize instructions and/or injection), replacing the plugin's manual
injection and preload for hosted sessions. The injected routing-table size
budget (research Open Question 10) is settled in this phase's planning.

Dogfood evidence (2026-11-18, track/agent-v2): a hosted lead starts with no
session key and no workflow manual. On a scratch daemon a hosted Claude lead's
`agent.spawn` failed with "session_key is required", and the agy Phase 2 live
lead recovered only by calling `ferrule` itself
(`261117-feat-ws-hosted-antigravity-adapter` Phase 2 Result, Forward). The
delivered manual must carry the lead's session key, so ws tools that require
one work from the first turn.

Verification: tests on remote w5090 that a hosted session of each available
harness receives the manual without any plugin hook; size-budget check; a
hosted lead's first-turn `agent.spawn` succeeds without a `ferrule` call.

### Phase 3: Per-turn ambient injection replacing plugin hooks

The daemon injects the per-turn ambient context that plugin hooks supply today,
driven from stream events, so a hosted session needs no plugin hooks. Which
hooks are replaced is enumerated in planning from the plugin's hook inventory.

Verification: tests on remote w5090 per replaced hook (injected content
equivalent, no duplicate when a plugin is also loaded); measured syscalls/s
shows no new per-turn file writes.

### Phase 4: Renderable agent-facing playbooks for `agent.spawn`

No rendering happens inside `agent.spawn` (playbook-unaware, owner 18:56).
`agent.spawn` delivers `system_prompt_path`/`system_prompt` as an appended system
prompt and resolves tier server-side ((39), (44), (38)=d); that surface is owned
by `261117-feat-ws-agent-tools-mailbox-routing` (phase 1). This phase makes the
agent-facing builtin profile instructions (explore.md) renderable playbooks so
`playbook.render` can supply their path and recommended tier ((52)=a), including
the `{{NativeToolScheme}}` line for the target harness. Recommendations use bare
tiers only (49). Depends on `261117-feat-ws-agent-tools-mailbox-routing#phase1`.

Verification: tests on remote w5090 that `playbook.render` for each agent-facing
playbook returns a path and bare-tier recommendation, that a child spawned with
that path and tier receives the rendered text (including the
`{{NativeToolScheme}}` line for its harness) as appended system prompt, with
tier-rank and authority caps still applied.
