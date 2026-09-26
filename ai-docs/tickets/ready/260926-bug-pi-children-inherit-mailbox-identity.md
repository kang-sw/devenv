---
title: Pi child processes inherit the lead's mailbox identity env
related:
  260926-bug-pi-execute-worker-steals-mailbox-owner: sibling — ownership-theft fix; this ticket removes the contest at its source
  260926-bug-mailbox-idle-owner-presence-goes-stale: sibling — same mailbox dogfood investigation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 681fb14acbe21b2b
sage-review-completeness-reviewed: 681fb14acbe21b2b
---

# Pi child processes inherit the lead's mailbox identity env

## Background

Every child Pi process the extension spawns (explore, worker,
execute-worker, fork, ask) launches with `{...process.env, ...childEnv}`, so
it inherits the lead's `WS_MAILBOX` / `WS_MAILBOX_AUTO`. Under `WS_MAILBOX`,
each child's own `ws-mcp serve` then resolves the same named-inbox identity
and contests the lead's presence record in `ensureMailboxRegistered` (under
`WS_MAILBOX_AUTO` a child instead mints its own random stem and self-registers
a separate `<stem>@<scope>` presence rather than contesting the lead's record:
agents-plugin-tool/internal/mcp/mailbox_runtime.go#L88-L121,
agents-plugin-tool/internal/mcp/mailbox_runtime.go#L191-L227):

- while the lead's record is live, the child flags `Conflict` on the lead's
  own record, so `lookup_peers` reports a spurious conflict;
- once the lead's `LastSeen` is stale, the child overwrites the record
  outright (the precondition of
  `260926-bug-pi-execute-worker-steals-mailbox-owner`).
- under `WS_MAILBOX_AUTO`, every child registers its own random
  `<stem>@<scope>` presence, so `lookup_peers` lists a phantom peer per
  spawned child that no waiter ever reads.

No spawned child is a mailbox owner: `shouldArmMailboxWaiter` already
refuses every role-marked child, and children talk back through the reply-id
channel, which needs no identity env.

## Decisions

- **The extension clears `WS_MAILBOX` and `WS_MAILBOX_AUTO` for every child
  it spawns**, at the shared child-env construction point (the builder that
  sets `WS_PI_SPAWN_ROLE_ENV` and the other per-child markers in
  `spawner.ts`), so no spawn role can miss it. Clearing to an empty string is
  sufficient: `computeMailboxIdentity` treats a blank value as inert.
  - Rejected: clearing only for leaf children. Forks and ask children are no
    more an owner than a leaf.
  - Rejected: relying on the sibling ownership fix alone. It stops the owner
    theft but not the spurious conflict or the presence overwrite.

## Constraints

- Scope is the Pi extension's spawned children only; the lead process's own
  env and the host-neutral mailbox contract are untouched.
- Fork children must not be exempted from the clear: the sibling
  `260926-bug-pi-execute-worker-steals-mailbox-owner` keeps forks as
  parent-less lead mints and relies on this ticket to keep a fork's MCP
  server from holding a mailbox identity (and so from rebinding the owner).

## Prior Decisions

- 260913-feat-cross-session-mailbox-core (2026-09-13, Decisions): "Identity / opt-in (1): a launch-time env WS_MAILBOX=slug@scope, which the ws-mcp server inherits at spawn, names a durable, discoverable endpoint explicitly." — bearing: constrains
- 260913-feat-cross-session-mailbox-core (2026-09-13, Decisions): "Auto-identity (10): ... When set (and WS_MAILBOX is not), the server mints a random 3-word stem at startup ... and self-registers as <stem>@<scope>" — bearing: constrains
- 260917-feat-ws-pi-mailbox-waiter-slug-wake (2026-09-17, Decisions): "Resolve the self slug from mailbox.lookup_peers, never from env. ... It must not re-read WS_MAILBOX / WS_MAILBOX_AUTO from its own environment" — bearing: supports
- 260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap (2026-09-07, Decisions): "The variable is passed only on the launcher child's spawn env ..., never written to process.env, so it cannot leak into Pi child processes the spawner creates." — bearing: supports
- ad03ac41 (2026-09-24, commit): "No bootstrap means no channel regardless of role, so a Pi started from a worker's shell with an inherited WS_PI_SPAWN_ROLE still boots" — bearing: supports
- 260926-bug-pi-execute-worker-steals-mailbox-owner (2026-09-26, Decisions): "Fork children are exempt. ws-fork and the ask discussion fork stay lateral leads ... Their parent-less lead ferrule is kept from rebinding by the sibling 260926-bug-pi-children-inherit-mailbox-identity, which clears WS_MAILBOX for every child" — bearing: constrains
- 260926-bug-mailbox-idle-owner-presence-goes-stale (2026-09-26, Decisions): "ws-mcp serve runs a background presence heartbeat for its active mailbox identity, refreshing LastSeen while its PID holds the presence record." — bearing: supports
- eca4a3a0 (2026-09-04, commit): "The WS_PI_AGENT_CHILD_ENV marker is additive-only in both spawn call sites (RPC options previously had no env field at all ...) — no existing spawn behavior changes." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts buildRpcClientOptions L2450-L2498, agents-plugin-pi/test/spawner.test.ts |
| scope.surface | internal | buildRpcClientOptions is exported only for tests; signature unchanged; both RpcClient call sites spawner.ts L3391 and L3595 already use it |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts describe buildRpcClientOptions WS_PI_SPAWN_ROLE_ENV placement at L2295 |
| complexity.reuse_points | confirmed | explicit empty-string clears already in buildRpcClientOptions spawner.ts L2468-L2487 over RpcClient env overlay rpc-client.js L44 |
| complexity.side_effect_risk | low | only the child env overlay gains two blank keys; computeMailboxIdentity treats blank as inert mailbox_runtime.go L88-L121; no child is a waiter owner mailbox-waiter.ts L109-L111 |
| risk.correctness | low | two more keys in the same explicit-clear block every spawn role passes through |
| risk.fit | low | matches the existing inherited-env neutralization convention at the same site |
| risk.test | low | existing tests assert the built options.env directly without reading process.env |
| risk.security_or_contract | low | Pi-extension-only change; host-neutral mailbox contract and the lead env untouched |

## Phases

### Phase 1: Clear mailbox identity env for spawned children

In `buildRpcClientOptions` (`agents-plugin-pi/src/spawner.ts`), add
`WS_MAILBOX: ""` and `WS_MAILBOX_AUTO: ""` to the existing explicit
empty-string clear block that every spawn role passes through, including
forks and ask children.

Verification:

- Pi unit test in `agents-plugin-pi/test/spawner.test.ts` on
  `buildRpcClientOptions`: for each spawn role, the built env sets
  `WS_MAILBOX` and `WS_MAILBOX_AUTO` to empty even when the parent
  `process.env` carries them.
- Existing Pi test suite passes (`npm test` in `agents-plugin-pi/`).
