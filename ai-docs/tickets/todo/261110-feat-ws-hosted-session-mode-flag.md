---
title: "Hosted-session mode: one env flag that hands input delivery and the dashboard feed to the daemon in the Claude mod and the Pi extension"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-headless-lead-sessions: "the consumer; the daemon sets this flag on the sessions it spawns"
  261110-research-ws-dashboard-agent-host-foundations: "Verified Findings on what the Claude mod and the Pi extension do and which parts conflict"
---

# Hosted-session mode flag

## Background

A daemon-hosted lead (`261110-feat-ws-dashboard-headless-lead-sessions`)
receives dashboard messages and ws mailbox mail from the daemon, which
injects them as stream-json user turns ((16)). The Claude mod and the Pi
extension already deliver the same input inside the session, so both would
claim the same messages.

What each part does (Explore, 2026-11-10; recorded in the foundations
research's Verified Findings):

| Feature | Claude mod | Pi extension | With a daemon host |
|---|---|---|---|
| Mailbox push, idle wake, mid-turn steer | wait process that recvs (`hooks/claude-mod/register.ts`, `waitArgv`) | `armMailbox` arms whenever the role is unset and a key exists, with no RPC-mode gate (`agents-plugin-pi/src/index.ts`, `shouldArmMailboxWaiter` in `mailbox-waiter.ts`) | conflicts: recv claims a message, so daemon and session race for it |
| Dashboard messages and interrupt | the same wait drains the board inbox | the same waiter drains and calls `abort()` | conflicts: claim race, double interrupt |
| Dashboard feed | `claude.dashboard_enabled` | present | duplicates: the daemon reads the stream-json output itself |
| Session key minting, workflow manual, key adoption on revive | `manual.ts` (probed under `-p`) | bridge | needed by a ws lead; no conflict |
| Preload skill expansion | `preload.ts` | n/a | needed; no conflict |
| `request_compaction` | works under `-p` through `$.command.run('compact')` | n/a | no conflict |

No per-process switch exists today: the mod's knobs are read from project
and global config only, the mod reads no disabling env var, and the Pi
waiter has no disable switch.

## Decisions

1. **The cut line is input delivery and the feed (technical, (20); user's
   concern, (16) note).** Under one env flag that the daemon sets, both the
   Claude mod and the Pi extension turn off: the mailbox wait and its
   recv/drain, the dashboard board-inbox drain, interrupt handling, and the
   dashboard feed. They keep: key minting or acceptance, workflow manual
   injection, key adoption on revive, preload expansion, and the compaction
   lever. One flag shared by both hosts puts the cut at the same place on
   each.
   Rejected: excluding the whole mod or extension the way `wsagent` isolates
   the support agent (`--setting-sources ""`, `--no-extensions`), because a
   lead without the workflow manual and preload expansion is not a ws lead.
2. **The daemon mints the session key and passes it in the environment
   (technical, (21)).** The mod or extension adopts that key instead of
   minting one (`manual.ts` `ensureKey` today). When a revive switches the
   key (the existing `adopt` path), the mod or extension reports the new key
   to the ws-mcp session registry, and the daemon follows it there.
   Rejected: discovering the key after the session mints it (a race between
   session start and the daemon's first delivery).

## Constraints

- A session without the flag behaves exactly as today. CLI-created sessions
  never see it.
- The Claude mod is mirrored byte-identically into `agents-plugin-wsflow`
  (`internal/wsrsrc/wsflow_hooks_mirror_test.go`).
- Behaviors the flag disables that have no headless use stay as they are:
  captions are already off headless (`captions.tsx`), and
  `request_mcp_reconnect` fails under `-p` (`reconnect.ts`
  `commandRefusal`); daemon-side recovery replaces it ((22), in the headless
  lead child).
- Promotion waits for the foundations research's verifications, like
  `261110-feat-ws-dashboard-headless-lead-sessions` (user, (91) a): the
  open items below depend on how `-p` and `--mode rpc` behave, and Decision
  2 needs a lead-scope key mint path in the daemon (`AgentKeyStore` mints
  delegate-scope keys only).
- To settle at promotion: the env names; whether the status band and the
  compaction advisory change under the flag; how the key change reaches the
  registry; Pi parity for each kept feature in `--mode rpc`.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/skill-authoring.md`
  if any shipped prompt text changes.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-10, Decisions): "The daemon injects all input (technical, (16)). Dashboard messages and ws mailbox mail for a daemon session reach it as stream-json user turns written by the daemon ... The session runs with the hosted-session flag" — bearing: supports
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-10, Decisions): "MCP recovery by restart and resume (technical, (22)). `/mcp reconnect` fails under `-p` (`agents-plugin/hooks/claude-mod/reconnect.ts` `commandRefusal`)." — bearing: constrains
- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "Daemon owns input for its sessions ... the Claude mod and the Pi extension stop their own delivery and feed under one hosted-session flag and keep the rest. Rejected: excluding the mod or extension entirely." — bearing: supports
- 259226c0 (2026-11-10, commit): "The cut line for the hosted-session flag (20) followed an Explore pass over the Claude mod and Pi extension: only recv/drain/interrupt and the feed conflict with daemon delivery" — bearing: supports
- 261110-feat-ws-mcp-daemon-hosted-proxy-launcher (2026-11-10, Decisions): "The full client environment travels. The hello carries the launcher's whole process environment, not only `WS_*`" — bearing: constrains
- 261110-feat-ws-mcp-daemon-hosted-proxy-launcher (2026-11-10, Result b9f4fb8): "`WS_MCP_NO_DASHBOARD` now means no registration/prune only, the daemon is the launcher's business" — bearing: constrains
- 261110-feat-ws-mcp-daemon-hosted-proxy-launcher (2026-11-10, Decisions): "`mailbox wait` also goes through the daemon (technical, (649)). The Claude mod's mailbox and dashboard-message push spawns `python3 <launcher> mailbox wait ...` on every wait cycle" — bearing: constrains
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Result 85b9da5fd): "`WS_MCP_NO_DASHBOARD=1` stops it ensuring a dashboard and registering a session. `WS_MAILBOX`/`WS_MAILBOX_AUTO` are set blank for it." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, agents-plugin/hooks/claude-mod/manual.ts, agents-plugin/hooks/claude-mod/dashboard.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/mailbox-waiter.ts, agents-plugin-pi/src/bridge.ts, agents-plugin-wsflow/hooks/claude-mod/ mirror |
| scope.surface | cross-module | Claude mod and Pi extension both read a new daemon-set env contract; key-change reporting may touch ws-mcp internal/mcp/dashboard_sessions.go |
| scope.new_public_symbol | yes | the hosted-session env flag and the daemon-minted key env, names unsettled |
| scope.new_type_contract | unknown | how a revive's key change reaches the registry is unsettled; Registration in internal/wsdashboard/registry.go has no session-key or lineage field today |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/*.test.ts, agents-plugin-pi/test/mailbox-waiter.test.ts, agents-plugin-pi/test/dashboard-feed.test.ts, agents-plugin-pi/test/bridge.test.ts, agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | envFlagOn and WS_MCP_NO_DASHBOARD in agents-plugin-tool/internal/mcp/process_env.go; relogin_session_key in internal/mcp/server.go handleLeadRelogin; shouldArmMailboxWaiter gate in agents-plugin-pi/src/mailbox-waiter.ts |
| complexity.side_effect_risk | moderate | gates mailbox wait, board drain, interrupt and feed; the unflagged path must stay byte-for-byte as today |
| risk.correctness | moderate | key adoption from env must coexist with resume relogin and revive adopt paths in both hosts |
| risk.fit | moderate | four Constraints items are still open, so the cut inside band, advisory and registry is undecided |
| risk.test | moderate | headless -p and Pi --mode rpc behavior is hard to exercise in unit tests; the mirror test must stay green |
| risk.security_or_contract | moderate | an env-supplied lead key relogs in and takes the mailbox owner pointer through rebindMailboxOwnerAtFerrule |

## Phases

### Phase 1: flag in the Claude mod and the Pi extension

Scope and verification are settled at promotion, after the Constraints'
open items.
