---
title: "Pi mini-lead profile: pi.spawn profile argument and subset no-escalation"
related:
  261112-research-ws-hosted-pi-missing-workflow-tools: research and evidence behind these decisions
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: d82d403e0f9d8f19
sage-review-design-reviewed: d82d403e0f9d8f19
---

# Pi mini-lead profile

## Background

`pi.spawn` children always run the builtin `pi-worker` profile, whose 11 ws
tools cover commit and spawning but not the ticket workflow, so a Pi worker
cannot run `ticket-worker`-style flows. Owner answers 2026-11-13 settle a
profile-based fix; evidence is in the related research ticket.

## Decisions

Compatibility tier: T0 (additive optional argument and a new builtin profile
key; `pi-worker` unchanged; running children pick up changes on restart).
The resolved-definition snapshot is an additive optional field on the session
record; records written before it lack the field, so their first launch
re-resolves once and snapshots, which keeps the record store change T0.

- Keep builtin `pi-worker` unchanged; add builtin `pi-mini-lead`; `pi.spawn`
  gains an optional `profile` argument. Rejected: widening `pi-worker` alone;
  also shipping a lead-privilege profile now.
- `pi-mini-lead` = `pi-worker` tools plus `tickets.query/acquire/release/
  close/move/verify/checklist`, read-only `note.query`, `playbook.read`,
  `playbook.render`, `path.generate`, `route.resolve_implement`. No note,
  whiteboard, or tracker writes. `workflow_manual`, `ferrule`,
  `workflow_state`, `git.merge`, `worktree.*`, `tickets.sage_stamp` stay
  excluded.
- Selection is by existing `agent.profile.<name>` name; no separate
  permission enum. Rejected: an enum (duplicates `ws_tools`, bypasses user
  overrides, splits enforcement).
- Builtin set shipped now: `pi-worker` + `pi-mini-lead`. Rejected: read-only
  `pi-reader`; lead-privilege profile.
- A Pi child holding lead-scope keys is deferred (security; needs a key-scope
  change; no playbook needs it).
- Omitted `profile` always means `pi-worker`. Rejected: caller-relative
  defaults.
- `profile` is a string naming any resolvable non-lead Pi profile, not an
  enum.
- No-escalation is a subset check "child tools within parent tools" over
  resolved definitions (effective `ws_tools`, `tools`, `mcp_servers`); a lead
  parent is unrestricted. The parent's comparison set is its launched
  snapshot, not its current profile by name. At each Pi child launch the
  resolved profile definition is snapshotted on the child's record; the check
  runs against that snapshot and the same snapshot is what launches (no
  re-resolution between validation and start). On a later restart or
  re-resolution the check re-applies against the parent's snapshot and refuses
  if the re-resolved child exceeds it. Rejected: checking only in
  `Runner.Spawn` against current profile definitions (re-resolution at
  `runner.go:1877-1892` lets a widened parent or a changed child profile
  diverge from what launched; `record.go` stores only profile names). Rejected:
  a fixed rank (breaks when an override shrinks `pi-mini-lead`).
- An escalation request refuses the spawn; the error names both profiles and
  the excess tools. Rejected: silently intersecting.
- No new playbook-write tool; read, render, `path.generate` suffice.
  Rejected: a worker override-write tool; deferring to a lead profile.
- Depth cap stays 2 (`wsagent/children.go:63-65`): a mini-lead's children are
  leaves.

## Constraints

- Out of scope: lead-privilege profile, `pi-reader`, lead-scope Pi keys,
  playbook text fixes (see
  `261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent`).
- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`, `ai-docs/manuals/release-compatibility.md`
  (AGENTS.md Implementation Conventions for `agents-plugin-tool/internal/` and
  `internal/mcp/`).
- T0 only: no existing tool, argument, store, or manifest field may change
  meaning.
- Update the `PiWorkerWSTools` comment, the `pi-worker` paragraph of
  `ws-mcp.md`, the `pi.spawn` descriptions (`mcp/pi_tools.go`,
  `mcp/pi_forward.go`), and `builtin_test.go` in the same change.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261112-feat-ws-mcp-pi-spawn-recursion (2026-11-13, commit 1d3fe308): "The forward check runs before requireLeadSessionKey: the worker's key is a delegate key by design (ticket Decision 6), so the lead-key check would refuse every forwarded call." — bearing: constrains
- 261112-feat-ws-mcp-pi-spawn-recursion (2026-11-13, ticket Result): "capability token (`WS_MCP_PI_TOKEN`, `WS_MCP_PI_ENDPOINT`) for a `Runner.Spawn` conversation with a parent, a recorded tier, and a `pi.*` profile tool, drawn per process start, never persisted" — bearing: constrains
- 261112-feat-ws-mcp-pi-spawn-claude-lead-path (2026-11-13, ticket Result): "Implemented in 7f40e8110 (runner spawn spec, parent link, turn reports, waits, `pi-worker` profile, `agentKind` in live state)" — bearing: supports
- 184452a1 (2026-11-13, commit 261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart): "Lead privileges remain independent of holder selection; Claude and other non-lead backends keep their existing behavior." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsagent/builtin.go, wsagent/profile.go, wsagent/children.go, wsagent/spawn.go, wsagent/record.go, wsagent/runner.go, mcp/pi_tools.go, mcp/pi_forward.go, ws-mcp.md |
| scope.surface | cross-module | internal/wsagent and internal/mcp packages; pi.spawn gains an MCP argument |
| scope.new_public_symbol | yes | pi-mini-lead builtin profile key, optional pi.spawn profile argument |
| scope.new_type_contract | yes | ChildSpawn (children.go#L252) gains a profile field; session record (record.go) gains an optional resolved-definition snapshot field (`ws_tools`, `tools`, `mcp_servers`); Runner.Spawn gains a subset-check refusal against the parent's snapshot |
| scope.test_surface | existing | wsagent/builtin_test.go, wsagent/children_test.go, mcp/pi_tools_test.go, mcp/pi_forward_test.go |
| complexity.reuse_points | confirmed | ResolveProfile and BuiltinProfiles (builtin.go#L106), PiWorkerWSTools pattern, delegate role already allows tickets.* (mcp/server.go#L5682-L5688) |
| complexity.side_effect_risk | moderate | changes which tools a spawned Pi child can reach; pi-worker default must stay unchanged |
| risk.correctness | moderate | subset check must use resolved definitions (ws_tools, tools, mcp_servers) including user overrides, and the launched snapshot rather than re-resolved names (runner.go:1877-1892 re-resolves per start) |
| risk.fit | moderate | follows the agent.profile family, but the spawn path is shared by the forwarded worker route (pi_forward.go) |
| risk.test | moderate | needs worker-parent refusal and override cases; Go tests run remotely |
| risk.security_or_contract | high | privilege no-escalation boundary between parent and child; new MCP argument on a shipped tool |

## Phases

### Phase 1: pi-mini-lead profile + pi.spawn profile argument + subset no-escalation check

Code sites:
- `agents-plugin-tool/internal/wsagent/builtin.go` (`BuiltinProfiles` ~:106,
  `PiWorkerProfile` :64, `PiWorkerWSTools` :75): add the `pi-mini-lead`
  profile.
- `wsagent/profile.go`: profile resolution and validation (`ws_tools` refusal
  of `ferrule`/`workflow_manual`).
- `mcp/pi_tools.go:307` (hardcoded `wsagent.PiWorkerProfile` in
  `handlePiSpawn`) and `mcp/pi_forward.go` schema: add the optional `profile`
  string.
- `wsagent/children.go:310` (`spawnFor` hardcoded profile) and `ChildSpawn`;
  depth cap at `:63-65` unchanged.
- `wsagent/record.go`: snapshot the child's resolved profile definition
  (`ws_tools`, `tools`, `mcp_servers`) on the record at each launch; store the
  parent's launched snapshot for comparison (names alone are insufficient).
- `Runner.Spawn` (`wsagent/spawn.go:117`) and the re-resolution path at
  `wsagent/runner.go:1877-1892`: subset check of the child's snapshotted
  resolved definition against the conversation parent's launched snapshot;
  lead parent unrestricted. The launch uses the same snapshot (no second
  resolve by name).
- `mcp/agent_keys.go`: keys stay delegate-scope (no change expected).

Verification (Go tests run on remote w5090 per the repo note):
- Omitted `profile` resolves to `pi-worker`.
- A `pi-mini-lead` spawn exposes the tool set above (Pi `--tools` and
  `WS_MCP_ALLOWED_TOOLS`).
- A worker parent requesting `pi-mini-lead` is refused; the error names both
  profiles and the excess tools.
- A lead parent may request any resolvable non-lead Pi profile.
- A user-overridden `agent.profile.pi-mini-lead` / `pi-worker` is honored and
  the subset check uses the resolved definitions.
- Widening the parent's profile after its launch does not let it spawn a child
  beyond its launched tool set (the comparison uses the parent's snapshot).
- A child whose profile is widened between spawn and (re)start is refused at
  start when the re-resolved definition exceeds the parent's snapshot.
