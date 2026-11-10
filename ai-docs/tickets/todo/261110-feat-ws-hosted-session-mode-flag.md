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
- To settle at promotion: the env names; whether the status band and the
  compaction advisory change under the flag; how the key change reaches the
  registry; Pi parity for each kept feature in `--mode rpc`.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/skill-authoring.md`
  if any shipped prompt text changes.

## Phases

### Phase 1: flag in the Claude mod and the Pi extension

Scope and verification are settled at promotion, after the Constraints'
open items.
