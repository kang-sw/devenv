---
title: "Hosted child session entry: click any ws/agent.spawn child in the dashboard and enter its session, for every harness"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-agent-tools-mailbox-routing: "prerequisite; agent.spawn children and lineage are what the dashboard lists"
  261117-feat-ws-harness-capability-catalog: "per-harness steer and resume items decide how chat/steer behaves in an entered child"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
---

# Hosted child session entry

## Background

Owner 18:35 proposed, as part 4 of the "application layer" on top of the common
harness interface, that the dashboard lets a user click into any child agent's
session, as is possible today for `pi.spawn` children, for every harness's
subagents. (28)=a places it late in stage 2, after the agent tools it lists.

## Decisions

Confirmed:

1. **Scope (owner 18:35, (28)=a).** From the dashboard, click any child agent
   spawned through `ws/agent.spawn`, on any harness, and enter its hosted
   session: view and chat/steer as with `pi.spawn` children today. Late stage 2.

Mechanics (how a child session is addressed from the dashboard, per-harness
steer limits) are not decided here; they are phase-planning notes.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): interactive paths never wait
  on file I/O; no per-event O(N) rereads. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- Identity and authority are preserved: entering a child session does not grant
  approval authority the child's profile lacks.
- Any cross-version surface touched states its compatibility tier in
  `## Decisions` before `ready/`.

## Phases

### Phase 1: Enter a child's hosted session from the dashboard

Make every `ws/agent.spawn` child, regardless of harness, a clickable entry
that opens its hosted session with transcript view and chat/steer, matching the
`pi.spawn` child experience. Planning notes: how the existing `pi.spawn` child
view is generalized; how steer degrades where the catalog grades mid-turn steer
unsupported; how children across a compaction segment chain (see
`261117-feat-ws-hosted-compaction-lifecycle`) are stitched.

Verification: Go tests on remote w5090 for child session addressing and
chat/steer delivery for each harness available at that time; a dashboard
check entering a Claude child and a Pi child end to end.
