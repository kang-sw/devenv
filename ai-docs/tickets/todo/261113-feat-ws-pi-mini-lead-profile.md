---
title: "Pi mini-lead profile: pi.spawn profile argument and subset no-escalation"
related:
  261112-research-ws-hosted-pi-missing-workflow-tools: research and evidence behind these decisions
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
- No-escalation is a subset check "child tools within parent tools" in
  `Runner.Spawn`; a lead parent is unrestricted. Rejected: a fixed rank
  (breaks when an override shrinks `pi-mini-lead`).
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
- `Runner.Spawn` (`wsagent/spawn.go:117`): subset check of the child's
  resolved `ws_tools`, `tools`, `mcp_servers` against a conversation parent's;
  lead parent unrestricted.
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
