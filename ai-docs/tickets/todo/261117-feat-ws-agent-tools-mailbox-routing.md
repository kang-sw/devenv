---
title: "ws/agent.* delegation tools with unified mailbox routing, relation from lineage, and native subagent removal in hosted leads"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "steer-capability lookup for parent-to-child delivery reads the catalog's mid-turn steer item"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
  261115-feat-ws-worker-profile-session-spawn: "worker-profile caps apply to agent.spawn"
  261115-feat-ws-session-hire-approval: "hire stays a separate peer-creation path; lineage authority was rejected there"
  261115-feat-ws-permission-profile: "common permission intent; tool-whitelist based spawn depends on it"
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch constraint; not edited from here"
  261112-epic-ws-mcp-pi-subagents: "done; pi.* is the prior art"
---

# ws/agent.* tools with unified mailbox routing

## Background

Delegation is always `ws/agent.*` regardless of harness (06): hosted leads no
longer use the harness's own spawn tool, so subagent tracking comes from the
daemon. Today `pi.*` is the daemon-owned spawn path (children, depth cap
`MaxSpawnDepth = 2` in `agents-plugin-tool/internal/wsagent/children.go`), and
hosted Claude leads are launched without a tool whitelist, so the native Agent
tool is available. This ticket introduces the final `agent.*` surface and
unifies parent/child/peer communication on a single `mailbox.send`.

The surface below supersedes an earlier draft that had `agent.transcript` and
`agent.wait`.

## Decisions

1. **Surface (06)=a, (09)=a.**
   - `agent.spawn` (prompt, profile, harness?, tier/model/effort?,
     cwd/worktree?, label?) returns the child's mailbox address and status.
   - `agent.stop` closes deepest-first (existing behavior kept).
   - `agent.list` returns child states plus the last report summary (a superset
     of `pi.list`).
   - No `wait` and no `transcript`: completion arrives by push (a system mail
     starts a new turn); humans read transcripts in the dashboard.
   - No `agent.send`: a single `mailbox.send(to: <address>)` serves children,
     peers and other leads.
2. **In-memory routing for daemon-owned addresses (12).** The daemon Runner
   routes daemon-owned agent addresses in memory, not through mailbox files;
   persist only what restore needs. The mailbox file path stays only for
   external sessions and CLI leads. Reason: the release gate on daemon file
   touches.
3. **Relation is computed, not stored (11).** Parent/child/peer is derived from
   lineage at delivery. Rejected: a stored Envelope relation field, because of
   whole-file loss of a new Envelope field on an old binary (11).
4. **Delivery timing by relation.** Parent to child: steer if the catalog
   supports mid-turn steer for that harness@version, else next turn. Child to
   parent: report as today's Pi worker report. Peer: next turn.
5. **Report-hold rule kept.** A child's final report is an automatic system mail
   to its parent, held while the child's own children are open.
6. **Native subagent removal in hosted leads (06)=a, (11).** Hosted Claude leads
   launch with `--disallowedTools Agent` and `Task` (init reports Agent as
   `Task`). Fix the existing tests that assert the flag's absence
   (`claude_lead_plugins_test.go:90`, `lead_claude_test.go:1182`).
7. **No depth cap (10)=c.** Remove `MaxSpawnDepth` (`wsagent/children.go`, the
   constant and its enforcement). Runaway is bounded by a per-root-tree
   concurrent-agent cap exposed as a config key. The default is chosen by
   measurement in this ticket (a guess of about 8 was floated and is uncertain)
   (12).
8. **Hire stays separate (11).** `session.request_hire` (peer, no lineage) is not
   merged into `agent.spawn` (child, lineage); the hire ticket rejected lineage
   authority (768=c, 775=a).
9. **Worker-profile caps apply.** A child never exceeds its parent's tier or
   permission (1014); this holds for `agent.spawn`.
10. **`pi.*` stays as a behavior-preserving alias (11)**; `agent.list` is a
    superset of `pi.list`.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If playbooks, skills, `agents-plugin/rsrc/` or
  `agents-plugin-wsflow/` text is touched (for example spawn-idiom routing), also
  read `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; no per-event O(N) rereads; interactive paths never wait on
  file I/O; judged by measured syscalls/s on the owner's PC. Daemon-owned agent
  routing must not add per-message file writes.
- Shipped playbooks must not name this repository's specifics
  (`ai-docs/manuals/shipped-surface-boundary.md`); route per-tier spawn idiom
  through generic hooks.
- Removing `MaxSpawnDepth` and blocking native tools in hosted leads are
  observable behavior changes, already owner-confirmed by (06), (10), (18).
- `pi.*` and old tools remain as aliases; the Claude CLI tier is frozen and keeps
  its native spawn.

## Compatibility

New `agent.*` tools are additive (T0). `pi.*` stays as an alias (T0). Relation
computed at delivery rather than stored is T0 (no new Envelope field, so no
whole-file loss on an old binary). The concrete verdict cites source lines for
which version writes and reads what, per
`ai-docs/manuals/release-compatibility.md`, recorded in Phase 1's Result. Removal
of the depth cap and native-tool blocking in hosted leads are observable
behavior changes owner-confirmed as above.

## Phases

### Phase 1: agent.spawn/stop/list, mailbox routing and relation delivery

Add `agent.spawn`, `agent.stop`, `agent.list` over the existing Runner spawn path
generalized to harness-selected backends with profile/tier resolution; route
daemon-owned addresses in memory via the Runner; accept `mailbox.send` to a
daemon-owned address; compute relation from lineage at delivery with the timing
rules and the report-hold rule; apply worker-profile caps; keep `pi.*` as an
alias; remove `MaxSpawnDepth` and add the per-root-tree concurrent-agent cap
config key with a default chosen by measurement (record the measurement).

Deferred: exec tools, single holder, any stage-3 harness backend.

Verification: Go tests on remote w5090 covering spawn/stop deepest-first/list,
mail between parent, child and peer with each timing, the report-hold rule, cap
enforcement and caps inheritance, `pi.*` alias parity, and no mailbox file write
for daemon-owned addresses; the recorded cap-default measurement.

### Phase 2: Native subagent removal in hosted Claude leads

Launch hosted Claude leads with `--disallowedTools Agent` and `Task`; fix the
two tests that assert the flag's absence; confirm via the init event that the
native tool is absent and that delegation works through `agent.spawn`.

Verification: updated tests on remote w5090; a recorded hosted-lead init stream
showing the tools removed; a hosted lead delegating through `agent.spawn` end to
end.
