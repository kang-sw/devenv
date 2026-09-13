---
title: "mailbox.lookup_peers lists the caller's own bound named inbox as a peer (self-leak)"
related:
  260913-feat-cross-session-mailbox-core: source — lookup_peers and the named-inbox self/peer projection live in this ticket's surface
  260913-feat-cross-session-mailbox-wake: source — ferrule-time named-inbox registration (the "catch") is this ticket's binding step
---

# mailbox.lookup_peers lists the caller's own bound named inbox as a peer

## Background

Dogfood finding (2026-09-13), reproduced live in a Claude session whose MCP
server holds `WS_MAILBOX=devenv@machine`.

Sequence:
1. Before `ferrule`, using a compaction-recovered session key minted in a prior
   era (before named-inbox registration existed for this session),
   `mailbox.lookup_peers(scope:"machine")` returned
   `self: {reply_id: ...}` (reply-id only, no durable inbox) and listed
   `devenv@machine` under `peers[]`.
2. Calling `ws/ferrule(root, capability:"lead")` minted a fresh session key and
   **registered the named inbox** — the "catch". With the new key,
   `lookup_peers(scope:"machine")` now returns
   `self: {"address":"devenv@machine","auto":false}` (correct: env `WS_MAILBOX`
   caught, `auto:false` = explicit not `WS_MAILBOX_AUTO`).
3. **Bug:** the same call STILL lists `devenv@machine` in `peers[]`, i.e. the
   caller's own bound address appears simultaneously as `self` and as a foreign
   peer, with no conflict marker.

Verified single-holder (rules out a genuine two-process conflict): `ps eww` over
every live `ws-mcp ... serve` process shows exactly one holder of
`WS_MAILBOX=devenv@machine` (pid 95497), which is this session's own server. So
this is not a missing conflict marker — the conflict marker correctly stays off
for a single holder. It is a self-exclusion failure: `lookup_peers` enumerates
the caller's own registered named inbox into `peers[]` instead of filtering it
out into `self` only.

Two adjacent facts confirmed correct in the same session (not bugs, recorded so
a fix does not "fix" them):
- Clone-scope isolation holds: `lookup_peers(scope:"clone")` returned
  `no live peers`; sibling sessions `main@clone` / `code-dev@clone` from OTHER
  repos did NOT leak in (they were only visible via direct `ps` inspection of
  other server processes, never via a mailbox call).
- The named-inbox binding is a `ferrule`-time action, not a server-start action:
  the server carries `WS_MAILBOX` in its env from launch, but the durable
  registration (and thus `self.address`) only materializes after `ferrule` runs.
  A session using a pre-`ferrule`/pre-registration key sees itself as reply-id
  only. (Whether a running server should retro-register on a later `ferrule`
  vs. warn that its intended identity was not yet caught is a separate UX
  question — see Open Questions.)

## Investigation

Needed: locate where `mailbox.lookup_peers` builds `peers[]` (likely
`agents-plugin-tool/internal/mcp/mailbox_tools.go` handler +
`internal/wsmailbox/listening.go` / `store.go` enumeration) and confirm the
self-exclusion predicate. The `self` resolution clearly knows the caller's bound
address; the peer enumeration must exclude the entry matching the caller's own
registered address (by owner identity / registered address, not by reply-id),
the same way a caller's own reply-id is not surfaced as a peer.

## Outcome Ledger

### Verified Findings

- With `self.address == devenv@machine` (single live holder, pid 95497 = the
  caller's own server), `lookup_peers(scope:"machine")` returns
  `devenv@machine` inside `peers[]` — the caller's own bound inbox listed as a
  peer.
- Single holder confirmed by `ps eww` over all live `ws-mcp serve` processes;
  the missing entry is a self-exclusion filter, not a conflict marker.
- `ferrule` correctly performs the named-inbox catch (`self` flips from
  reply-id-only to `{address: devenv@machine, auto:false}`).
- Clone-scope isolation is intact (no cross-repo peer leak via mailbox calls).

### Proposals

- Exclude the caller's own registered named-inbox address from `peers[]` in
  `lookup_peers`, mirroring how the caller's own reply-id is already excluded;
  keep it only under `self`. Add a test: a caller bound to `<name>@<scope>` sees
  its own name in `self.address` and NOT in `peers[]`; a genuinely-distinct
  second live holder still produces the conflict marker.

### Open Questions

- Should a running server whose env carries `WS_MAILBOX` but which has not yet
  registered (pre-`ferrule`) surface any signal that its intended identity is
  not yet caught, rather than silently reporting reply-id-only? (Separate UX
  concern; not required to fix the self-leak.)

### Rejected Alternatives

- Treating the double listing as a conflict-marker gap — rejected: only one live
  process holds the name, so conflict suppression is correct; the defect is
  self-exclusion.
