---
title: "Hosted-session mode: one env flag that hands input delivery and the dashboard feed to the daemon in the Claude mod and the Pi extension"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-headless-lead-sessions: "the consumer; the daemon sets this flag on the sessions it spawns"
  261110-research-ws-dashboard-agent-host-foundations: "Verified Findings on what the Claude mod and the Pi extension do and which parts conflict"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 564d6329db2f1b33
sage-review-completeness-reviewed: 564d6329db2f1b33
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
   minting one (`manual.ts` `ensureKey` today). The key stays fixed for the
   session's life (Decision 7).
   Rejected: discovering the key after the session mints it (a race between
   session start and the daemon's first delivery).

3. **Env names (technical, (161)).** The flag is `WS_HOSTED_SESSION=1`;
   the daemon-minted key travels as `WS_HOSTED_SESSION_KEY`.
   Rejected: reusing `WS_MCP_NO_DASHBOARD`, which already means "no
   registration or prune only" (proxy-launcher Result b9f4fb8), a narrower
   contract than the input-and-feed cut.
4. **The daemon owns the registration (technical, (162)).** Under the flag
   the in-session ws-mcp neither registers nor prunes; the daemon writes the
   session's registration itself at spawn, with harness and the reply-id derived from the key
   (`wsmailbox.ReplyID`; the key itself is never recorded, Decision 7). If a
   registration field only the in-session server knows turns out to be
   needed, the worker reports it.
   Rejected: letting the in-session server register, which gives the record
   two owners and races the row the daemon needs before the first turn.
5. **What the flag gates (announced, (163)).** The mailbox wait at its first
   arm and at every re-arm after key adoption, on both hosts, and
   `dashboard_intake`. Under the flag the key cannot change (Decision 7),
   so adoption is a same-key no-op and the re-arm gate is defence in
   depth; no key-switch test is needed. The board-inbox drain and interrupt handling live
   inside that wait on both hosts, and the dashboard feed is
   `dashboard_intake`, so this covers everything Decision 1 turns off. The
   compaction lever and the `SessionStart:compact` nudge stay. Source: Decision 1 and the 2026-11-11 probe (foundations
   Verified Findings): adoption re-arms the wait on the new key in both
   hosts.
   The flag alone decides the gating: with `WS_HOSTED_SESSION=1` set but
   `WS_HOSTED_SESSION_KEY` missing or empty, the gates stay on, the host
   mints a key as it does today, and it emits a visible warning naming the
   missing variable.
6. **Band and advisory unchanged (technical, (164)).** The status band
   renders only in the TUI, so it is inert headless, and the compaction
   advisory steers the model to the compaction lever, which works under
   `-p` (probe).
   Rejected: gating them under the flag.
7. **A hosted session's key never changes (user, (181) d).** The ws-mcp
   relogin and manual handlers (`ferrule` with `relogin_session_key`,
   reached by the adopt path in `hooks/claude-mod/manual.ts` and
   `agents-plugin-pi/src/bridge.ts`, handled in `handleLeadRelogin`; and
   `workflow_manual(session_key)`) check the connection environment: when it
   carries `WS_HOSTED_SESSION_KEY`, a different key is refused with a notice
   that names the hosted key and tells the model to use it, and a
   parent-less, lead-capability `ferrule` mint (the top-lead bootstrap,
   `internal/mcp/server.go` `handleLeadLogin`) returns the hosted key
   instead of minting. Mints that carry `parent_session_key` or a
   delegate or leaf capability mint child keys exactly as today, so a
   subagent on the same connection never receives the lead key. The same
   key is accepted as today. The check is server-side, so both hosts get it
   from one place, including under proxy-launcher Decision 5's direct
   `serve --stdio` fallback (the hello carries the full environment,
   proxy-launcher Decision 3). The Registration, the reply-id, and the
   session page URL therefore stay fixed, and the registry keeps recording
   no session key (ee0520fa7).
   Rejected: following a key change through a session-key field on the
   Registration ((165), superseded; it reverses ee0520fa7 and puts a bearer
   credential into `registration.json` and the page-facing index Session);
   a successor reply-id lineage pointer ((181) b); re-registering the row
   under the new reply-id, which moves the page URL ((181) c). Taking over
   another session's track becomes a daemon spawn with that key, in the
   consumer ticket if needed.
8. **Lead-scope mint in the daemon (technical, (166)).** A lead mint beside
   `AgentKeyStore`'s delegate mint writes the same key record `ferrule`
   writes.
   Rejected: shelling out to `ferrule`, a process per spawn when the daemon
   already owns the key directory.
9. **Pi parity (announced, (167)).** Every kept feature works in `--mode
   rpc` (probe 2026-11-11: key minting, manual injection, skill expansion,
   key adoption). A successful RPC compaction is unverified and does not
   block this ticket, since Decision 1 leaves compaction untouched.

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
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, `ai-docs/manuals/skill-authoring.md`
  if any shipped prompt text changes.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

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
| scope.new_public_symbol | yes | `WS_HOSTED_SESSION` and `WS_HOSTED_SESSION_KEY` (Decision 3); a lead-scope mint beside `AgentKeyStore` (Decision 8) |
| scope.new_type_contract | yes | the hosted-key refusal in handleLeadRelogin and workflow_manual (Decision 7); no Registration field change |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/*.test.ts, agents-plugin-pi/test/mailbox-waiter.test.ts, agents-plugin-pi/test/dashboard-feed.test.ts, agents-plugin-pi/test/bridge.test.ts, agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | envFlagOn and WS_MCP_NO_DASHBOARD in agents-plugin-tool/internal/mcp/process_env.go#L229; relogin_session_key in internal/mcp/server.go handleLeadRelogin#L2348; shouldArmMailboxWaiter gate in agents-plugin-pi/src/mailbox-waiter.ts#L171, re-verified 2026-11-11 |
| complexity.side_effect_risk | moderate | gates mailbox wait, board drain, interrupt and feed; the unflagged path must stay byte-for-byte as today |
| risk.correctness | moderate | env-key adoption and the server-side hosted-key refusal must coexist with resume relogin and the revive adopt path in both hosts |
| risk.fit | moderate | the cut is settled (Decisions 3-9) but spans two hosts, the wsflow mirror, ws-mcp and the dashboard registry |
| risk.test | moderate | headless -p and Pi --mode rpc behavior is hard to exercise in unit tests; the mirror test must stay green |
| risk.security_or_contract | moderate | an env-supplied lead key relogs in and takes the mailbox owner pointer through rebindMailboxOwnerAtFerrule |

## Phases

### Phase 1: flag in the Claude mod and the Pi extension

One phase (technical, (168)): the same cut on both hosts is the point of
Decision 1, so splitting the mod and Pi was rejected.

Scope:
- The `WS_HOSTED_SESSION` / `WS_HOSTED_SESSION_KEY` env contract and gate
  in the Claude mod (`hooks/claude-mod/register.ts`, `manual.ts`,
  `dashboard.ts`), mirrored byte-identically into `agents-plugin-wsflow`,
  and in the Pi extension (`src/index.ts`, `src/mailbox-waiter.ts`,
  `src/bridge.ts`): no mailbox wait at first arm or re-arm, no
  `dashboard_intake`, adopt the env key instead of minting.
- In-session ws-mcp: under the flag, no dashboard registration or prune.
- ws-mcp: the hosted-key check in the relogin and `workflow_manual`
  handlers (Decision 7).
- Daemon: a lead-scope key mint beside `AgentKeyStore`.
  Wiring the mint and the env into an actual daemon spawn belongs to
  `261110-feat-ws-dashboard-headless-lead-sessions`; this phase provides
  the pieces and tests them directly. The spawn-time Registration write,
  its heartbeat refresh, and keeping other ws-mcp processes from pruning a
  daemon-owned row also belong to that consumer ticket. Under the flag this
  server sends no whiteboard nudges (they need its own registration), so
  board refresh falls back to the 30 s poll; confirm that is the only loss.

Verification:
- Unflagged paths unchanged: existing claude-mod and Pi tests pass.
- Flagged paths: spawn and re-arm no wait, send no intake, adopt the env
  key (new unit tests on both hosts).
- `internal/wsrsrc` mirror test green.
- Go tests for the lead mint and the hosted-key check: a different key is
  refused with a notice naming the hosted key, the same key is accepted,
  a parent-less lead-capability mint returns the hosted key, a
  parent-carrying or delegate/leaf mint still mints a fresh child key,
  and a connection without the env is unchanged.
- One live `claude -p` and one `pi --mode rpc` run with the flag set,
  showing no `mailbox wait` child process.
