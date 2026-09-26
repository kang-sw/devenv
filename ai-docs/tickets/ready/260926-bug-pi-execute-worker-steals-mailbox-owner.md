---
title: Pi execute-worker steals the lead's named-inbox mailbox ownership
related:
  260917-feat-ws-pi-mailbox-waiter-slug-wake: origin — the slug-armed waiter whose startup owner check surfaces this bug
  260913-feat-cross-session-mailbox-core: source — Decision 3 ferrule owner binding, narrowed here
  260926-bug-pi-children-inherit-mailbox-identity: sibling — removes the child's presence contest at its source
  260926-bug-mailbox-idle-owner-presence-goes-stale: sibling — idle-owner staleness that let the child reclaim the record
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a6e6f5d7796a9af9
sage-review-completeness-reviewed: a6e6f5d7796a9af9
---

# Pi execute-worker steals the lead's named-inbox mailbox ownership

## Background

A Pi lead with `WS_MAILBOX=exec@worktree` repeatedly shows:

```text
Warning: [ws-mailbox] ws-mcp mailbox wait: warning: named inbox exec@worktree is not currently owned by this --session-key; falling back to a reply-id-only wait
```

The warning is the visible half; the real damage is that the lead can no longer
`mailbox.recv` its named inbox, because the owner pointer now names another key.

Root cause, verified on-machine 2026-09-26 (gunpowder-odyssey worktree store:
presence `exec` had `owner: scared-number-obituary`, `pid: 37270` (dead),
`last_seen` equal to the creation time of key `scared-number-obituary`, whose
record is `scope: leaf` with **no parent**; the live lead key was
`visitor-oxidation-crumb`):

1. `ws-execute` (`agents-plugin-pi/src/execute-gateway.ts`, its `spawnAgent`
   call with `toolGroup: "execute-worker"`) passes no `parentPolicy`, unlike
   the `ws-agent-spawn` and `explore` spawn sites in `spawner.ts` that pass
   `{ ...callerDelegationPolicy(bridge.wsToolNames), sessionKey: bridge.defaultSessionKeyRef.current }`
   (`agents-plugin-pi/src/spawner.ts#L4141`, `#L4333`); the fork spawn sites
   pass no `parentPolicy` either, only `ctx.parentSessionKey`, which feeds the
   `WS_PI_PARENT_SESSION_KEY` env rather than the policy
   (`agents-plugin-pi/src/fork.ts#L208-L233`, `agents-plugin-pi/src/ask.ts#L1535-L1552`,
   `agents-plugin-pi/src/spawner.ts#L2475`).
   `childPolicy` therefore omits `parentSessionKey`, and the child bridge
   bootstraps with a **parent-less** `ferrule` carrying `capability: "leaf"`.
2. The child inherits `WS_MAILBOX`, so its own MCP server registers the same
   named-inbox presence. The lead's `LastSeen` had gone stale (>10 minutes; the
   heartbeat refreshes only opportunistically on mailbox tool calls, not while
   `mailbox wait` blocks), so the child overwrote the record.
3. `rebindMailboxOwnerAtFerrule` (`agents-plugin-tool/internal/mcp/mailbox_runtime.go`)
   gates only on an empty parent key and ignores capability, so the leaf
   key became the owner. The worker then exited, leaving a dead PID and a
   foreign owner.

`-r` resume is not a required condition; any long-lived lead that runs
`ws-execute` can hit it.

## Decisions

- **A (pi): the execute-worker spawn carries the lead's session key.**
  `ws-execute`'s spawn passes a `parentPolicy` with
  `sessionKey: bridge.defaultSessionKeyRef.current`, matching the other lead
  spawn sites, so the child's bootstrap `ferrule` carries
  `parent_session_key`.
- **B (ws-mcp, protocol semantics change, user-approved 2026-09-26): only a
  parent-less *lead-capability* ferrule rebinds the named-inbox owner.** A
  parent-less `ferrule` whose capability resolves to `delegate` or `leaf`
  mints its key but never touches the owner pointer. Omitted capability still
  resolves to lead (`parseCapabilityScope`), so existing top-lead logins,
  including the `workflow_manual` fresh-bootstrap mint, keep rebinding.
  - When a lead rebind lands on an existing presence record whose PID is not
    this process (and is not a live holder, which the existing in-lock guard
    already refuses), the rebind also sets the record's PID to this process.
    Otherwise `refreshMailboxPresenceHeartbeat` keeps skipping the record on
    its PID check and the reclaimed inbox stays stale.
  - Rejected: fixing only the pi spawn site (A alone). Any harness path that
    mints a parent-less non-lead key would steal ownership again; B is the
    defense in depth.
- **A covers every Pi lead spawn path, not only `ws-execute`.** Audit each
  `spawnAgent` caller; any non-fork path that launches a child without the
  lead key in its policy gets the same fix. Rejected: fixing only
  `ws-execute` and leaving the rest to B, because B protects mailbox
  ownership only, not the child's missing lineage.
  - **Fork children are exempt.** `ws-fork` and the ask discussion fork stay
    lateral leads (authority `lead`, no policy parent key; the parent key
    travels only as `WS_PI_PARENT_SESSION_KEY`). Their parent-less lead
    `ferrule` is kept from rebinding by the sibling
    `260926-bug-pi-children-inherit-mailbox-identity`, which clears
    `WS_MAILBOX` for every child so a fork's MCP server has no mailbox
    identity. Rejected: giving forks a policy `parentSessionKey`, which
    changes a lateral lead's lineage and key semantics.
  - **Dormant-child relaunch re-stamps the parent key.** Relaunch
    (`ws-agent-send` auto-resume) reuses the spawn-time `record.delegation`;
    for a non-fork record whose stored policy carries no pre-minted
    `sessionKey`, the relaunch
    sets its `parentSessionKey` to the current
    `bridge.defaultSessionKeyRef.current`. This covers children spawned
    before the fix, a stored parent key pruned by the 30-day key retention
    (which would fail `ferrule` with "not a known session key"), and lineage
    after a lead re-login. Fork records relaunch through the same path and
    keep their parent-less policy, per the fork exemption above. Rejected:
    keeping the stored policy as-is.

## Constraints

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md` (all
  `agents-plugin-tool/` output) and `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`).
- Update drifted comments on contact, notably the ferrule call-site comment
  in `server.go` that says "a parent-less mint is a top-lead (re-)login", and the
  `rebindMailboxOwnerAtFerrule` doc comment that says a worker/delegate mint
  "always carries a parent".
- Out of scope: auto-reclaiming ownership that was already stolen before the
  fix. Recovery stays a lead re-login (`/reload`, which re-runs the
  parent-less lead `ferrule`); no waiter-side non-ownership detection.
- No hard ordering with `260926-bug-pi-children-inherit-mailbox-identity`:
  if this ticket lands first, fork children keep today's rebind behavior
  until the sibling clears their `WS_MAILBOX`, which is no regression.
- Both this ticket and `260926-bug-mailbox-idle-owner-presence-goes-stale`
  touch `mailbox_runtime.go`: whichever lands second rebases onto the other,
  and if this ticket lands second it adds a test that the background
  presence ticker keeps a reclaimed, PID-rewritten record fresh.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260913-feat-cross-session-mailbox-core (2026-09-13, Decisions): "The trigger is a parent-less ferrule (top-lead login / unknown_session → re-login recovery), never 'any lead-capability key' — worker mini-leads hold lead-capability child keys and must not steal ownership." — bearing: constrains
- 260913-research-cross-session-mailbox (2026-09-13, Confirmed Decisions): "rebind owner on each ferrule call where WS_MAILBOX is set and parent_session_key == null; worker/delegate ferrule calls (parent present) never bind. ... Trigger is parent-less ferrule, never 'any lead-capability key.'" — bearing: constrains
- 18d50953 (2026-09-13, Result of 260913-feat-cross-session-mailbox-core): "Owner-rebind is double-guarded: mailboxHasConflict() checked before the rebind attempt, then re-verified under the same write lock (fresh liveness/PID check) to close the registration-to-rebind race window." — bearing: constrains
- 6d786057 (2026-09-13, commit): "Chose to keep mailboxPresenceLive's dead-PID check asymmetric (dead PID overrides recency to dead; alive PID does not override recency to alive) rather than a full OS-liveness-based redesign" — bearing: constrains
- c0e0816a (2026-09-13, commit): "Mailbox storage is process-scoped (keyed off Server.root) ... Decision 8 scopes one endpoint per harness root session per WS_MAILBOX, and there is exactly one server root per process lifetime." — bearing: supports
- f5c06b38 (2026-09-13, commit): "named-inbox registration is a ferrule-time catch (not server-start), which is why the pre-ferrule recovered key saw itself as reply-id only." — bearing: supports
- 7f0a0d35 (2026-09-15, 260914-feat-ws-pi-mailbox-native-steer-push commit): "Owner-lead gate (readSpawnRole === undefined): a fork/worker/explore child has no cross-session inbox worth an extra background subprocess." — bearing: supports
- 35c143d7 (2026-09-18, 260917-feat-ws-pi-mailbox-waiter-slug-wake commit): "Adapter-only: shared mailbox Envelope/contract untouched; CLI owner-gate keeps an unowned/stale slug safe by construction." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/execute-gateway.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-tool/internal/mcp/mailbox_runtime.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/workflow_manual.go, plus the audited fork spawn sites in agents-plugin-pi/src/fork.ts and agents-plugin-pi/src/ask.ts |
| scope.surface | cross-module | Pi adapter spawn policy plus ws-mcp ferrule owner-binding semantics; rebindMailboxOwnerAtFerrule is unexported |
| scope.new_public_symbol | no | none; the rebind gate needs the resolved capability, an internal signature change only |
| scope.new_type_contract | no | none; DelegationPolicy already carries sessionKey and parentSessionKey in agents-plugin-pi/src/delegation-policy.ts#L25-L26 |
| scope.test_surface | existing | agents-plugin-pi/test/execute-gateway.test.ts, agents-plugin-pi/test/spawner.test.ts, agents-plugin-tool/internal/mcp/mailbox_tools_test.go |
| complexity.reuse_points | confirmed | parentPolicy pattern at agents-plugin-pi/src/spawner.ts#L4141; exported spawnAdmission seam at spawner.ts#L3240; live-PID refusal fixture TestRebindMailboxOwnerRefusesLiveDifferentPIDHolder in mailbox_tools_test.go |
| complexity.side_effect_risk | moderate | child ferrule now carries parent_session_key, which server.go#L1883-L1887 rejects when unknown, and the rebind now rewrites the shared presence PID |
| risk.correctness | moderate | PID takeover on rebind must stay behind the in-lock live-holder guard; fork children resolve to lead authority at spawner.ts#L3232 and still pass a parent-less lead ferrule |
| risk.fit | moderate | Decision A's audit reaches fork and discussion-fork sites whose lateral-lead lineage is a separate design question |
| risk.test | moderate | dead-PID and heartbeat-throttle fixtures needed; ambient WS_MAILBOX env leaks into mailbox tests per 260913-bug-mailbox-tests-read-ambient-ws-mailbox-env |
| risk.security_or_contract | high | user-approved protocol semantics change to ferrule owner binding, the mailbox hijack guard |

## Phases

### Phase 1: Keep mailbox ownership with the lead

Implement Decisions A and B, including A's spawn-path audit, fork
exemption, and dormant-relaunch re-stamp.

Verification:

- Pi test (`agents-plugin-pi/test/execute-gateway.test.ts` or the spawner
  tests): the execute-worker child's delegation policy carries
  `parentSessionKey` equal to the lead key.
- Go tests in `agents-plugin-tool/internal/mcp/`:
  - a parent-less `ferrule` with `capability: "leaf"` (and `"delegate"`) does
    not change the presence owner;
  - a parent-less lead `ferrule` still rebinds, including with capability
    omitted;
  - a lead rebind over a record left by a dead different PID sets the PID to
    this process, and a subsequent heartbeat refresh updates `LastSeen`;
  - the existing live-different-PID refusal test still passes.
- Pi test: relaunching a dormant child whose stored policy has no
  pre-minted `sessionKey` launches it with `parentSessionKey` equal to the
  current lead key; relaunching a fork record keeps its policy without a
  `parentSessionKey`.
- The worker's Result lists each audited `spawnAgent` caller and its outcome
  (already compliant, fixed, or fork-exempt).
- Existing mailbox and pi test suites pass.
