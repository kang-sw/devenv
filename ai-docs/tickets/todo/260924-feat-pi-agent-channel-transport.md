---
title: Pi parent-child control channel transport abstraction with pipe and TCP backends
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority, and its evidence round precedes promotion
  260924-feat-pi-agent-channel-approval-decisions: dependent migration
  260924-feat-pi-agent-channel-subtree-state: dependent migration
  260924-feat-pi-agent-channel-usage-rollup: dependent migration
  260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env: precedent for the accidental env-inheritance class the bootstrap removal guards against
---

# Pi parent-child control channel transport abstraction with pipe and TCP backends

## Background

The Pi adapter passes parent-child state through several ad hoc filesystem protocols, because Pi's public `RpcClient` offers no generic extension-data exchange. `260923-research-pi-parent-child-loopback-control-channel` settled on a dedicated per-direct-child control channel. The adapter owns it, and Pi itself and `RpcClient` stay unmodified. This ticket builds that channel and moves the two per-launch readiness handshakes onto it. The approval, subtree, and usage migrations are separate dependent tickets.

Before promoting this ticket to `ready/`, read the research ticket's Outcome Ledger, including the results of its evidence round. That round is a provider-free prototype run in a separate worktree. Reconcile this ticket against those results.

## Decisions

- **Message-level backend contract.**
  - The contract is `send(msg)`, `onMessage`, and `close()`, with ordered, at-most-once delivery within one connection generation. Framing lives inside each backend.
  - Disconnect is judged by process lifecycle, not by the backend: the parent observes child exit through `RpcClient`, and the child exits on stdin end. Acknowledgment and deduplication belong to the protocol layer, not the backend.
  - The contract must not assume stream-socket properties such as immediate close detection, so that a weaker backend can be added later without changing the contract.
- **Backends and selection.**
  - Backends are tried in this order: pipe (a Unix domain socket on macOS/Linux, a named pipe on Windows), then loopback TCP on `127.0.0.1`.
  - The parent binds in that order before spawning the child. It passes the resulting endpoint descriptor, a per-launch credential, and the generation through `RpcClientOptions.env`. The child only connects; there is no negotiation.
  - File backing is reserved in the abstraction but not implemented.
  - If every implemented backend fails, the channel fails closed, and the diagnostic names each backend's failure.
  - Tests can force a specific backend.
- **Connection policy.** This baseline is normative; weakening any part of it is a defect:
  - per-launch credential and generation checked in the handshake;
  - one live connection per direct child; a new connection is rejected while one is live, never allowed to replace it;
  - reconnect only after the previous connection closed, with the same credential and generation;
  - the child reads the bootstrap values and removes them from its environment before any tool runs. This guards against accidental inheritance, for example a Pi process started from the worker's own shell.
- **Threat model.** Same-user processes are outside the threat model. Do not escalate same-user forgery into a blocking finding, and do not add hardening beyond the baseline. Other OS users remain in scope, and the credential is what stops them. The research ticket's `## Threat Model (Scope Note)` is the full statement.
- **Readiness absorbed.** Fork readiness (`ready.json` in the fork launch envelope) and explore web-tools readiness (`web-tools-ready.json`) move into the authenticated channel hello, and their file handshakes are retired. The fork *context* envelope that carries input into the child is not a readiness handshake and is not retired here.
- **Escalation.** If verification contradicts a decision above, the worker stops and escalates instead of choosing an alternative. Example: removing the bootstrap values from `process.env` does not keep them out of the child's bash tool.

## Phases

### Phase 1: Channel transport, bootstrap, and readiness hello

Build the backend contract, the pipe and TCP backends, the parent-side ordered bind and child-side connect, the authenticated versioned hello, the connection policy, and the read-then-delete of bootstrap values. Route fork and web-tools readiness through the hello, and remove their files.

Verification:

- One protocol contract test suite passes against both the pipe and the TCP backends, each forced explicitly.
- A provider-free direct-child round trip works. A nested hop works too: a child that is itself a parent establishes its own channel to a grandchild.
- Backend fallback: a failed pipe bind falls back to TCP. When both fail, the channel fails closed, and the diagnostic names both failures.
- The connection policy is enforced: a second connection is rejected while one is live, a reconnect after close with a matching credential and generation succeeds, and a mismatched credential or stale generation is rejected.
- Bootstrap values are absent from the environment that the child's bash tool sees.
- Fork and web-tools readiness succeed and fail as today, but over the hello, and no readiness files are written.
- Record these findings in the Result: the named-pipe default access on Windows and whether remote clients are rejected, and the Unix-socket path location and length handling, including stale-socket cleanup after a crash.
- Native Windows coverage, or an explicitly recorded unverified gap.
