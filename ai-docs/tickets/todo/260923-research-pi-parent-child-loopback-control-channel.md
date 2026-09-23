---
title: Investigate a Pi parent-child control channel with a pluggable transport
related:
  260923-bug-pi-execute-approval-accepted-worker-hangs: repaired after and on top of this channel; its incident defects stay independently diagnosable
  260924-research-pi-root-single-authority-durable-state: split out; root-authored durable state is not in this scope
  260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint: TCP-independent defect found by the inventory; its checkpoint part waits for this research's usage design
  260924-bug-pi-durable-write-hygiene: TCP-independent defects found by the inventory
  260920-bug-pi-nested-subagent-terminal-delivery-stall: cause unknown; this channel is not claimed to fix it
---

# Investigate a Pi parent-child control channel with a pluggable transport

## Background

The Pi adapter has an RPC connection to each direct child, but its public `RpcClient` does not expose a generic extension-data exchange. The adapter therefore routes parent-child state through filesystem workarounds:

- in-flight execute approval uses a decision file and polling;
- recursive subtree state uses a private JSON snapshot and watcher;
- descendant usage does not roll up into the lead footer at all.

This research investigates a dedicated, transient control channel for each direct parent-child pair. The adapter owns the channel and does not modify Pi itself or its `RpcClient` implementation. The core deliverable is a source-level **transport abstraction**. The concrete transports behind it are replaceable backends.

The approved-but-unconsumed decision in `260923-bug-pi-execute-approval-accepted-worker-hangs` motivates the work. Its incident-specific filename/runtime mismatch and ownership-ID validation are separate defects and must be diagnosed on their own merits. A new transport does not by itself prove that bug fixed.

## Investigation Boundary

- **Transport abstraction and backends.** Define a message-level backend contract, then implement and verify the pipe and TCP backends behind it. Validate Windows, macOS, and Linux assumptions by measurement; Node API availability alone does not establish platform acceptance.
- **Bootstrap and lifecycle.**
  - The parent binds the endpoint before child launch and passes the endpoint descriptor, a per-launch credential, and the generation through supported RPC child environment options.
  - The child must complete authenticated readiness before its first work prompt.
  - Check concurrent and nested launches, stop, resume, reconnect, stale environment, and endpoint reuse or cleanup.
- **Protocol needs**, kept separate from each other:
  - a sparse, command-ID-bound parent-to-child decision while a tool is pending;
  - child-to-parent subtree state that keeps the pre-dispatch busy fence and the conservative unknown-state behavior;
  - replay-safe cumulative per-hop descendant usage that does not restore recursive footer polling.
  - The existing Pi RPC event path for reports and approval requests is preserved, as are durable ownership and session records for restart recovery.
- **Evidence required before recommending actionable implementation tickets:**
  - a harmless, provider-free direct-child round trip;
  - a nested-hop test;
  - one protocol contract test suite run against every implemented backend;
  - a failure-injection matrix for decision delivery and child or parent restart;
  - native Windows coverage, or an explicitly recorded unverified gap. A Windows smoke host is recorded in the ws notes.

## Threat Model (Scope Note)

The channel's credential and connection policy exist to prevent accidental cross-talk. The chief case is a Pi process started from a worker's own shell that inherits the channel bootstrap environment. The policy does not defend against a hostile process running as the same OS user.

Same-user processes are outside this channel's threat model. They can already read and write the adapter's owned homes, and the file-based approval path this work replaces consumes any well-formed decision file without authentication. Do not escalate same-user forgery into a blocking finding, and do not add hardening beyond the baseline below.

This note does not relax the baseline. A change that weakens it is a defect:
- per-launch credential and generation checked in the handshake;
- one live connection per direct child; a new connection is rejected while one is live, never allowed to replace it;
- reconnect only after the previous connection closed, with the same credential and generation;
- the child reads the bootstrap values and removes them from its environment before any tool runs.

Other OS users on the same machine remain in scope, and the credential is what stops them. Loopback TCP is reachable by any local user, and named-pipe default access is still unverified.

## Side-Channel Inventory

An Explore inventory of `agents-plugin-pi/src/` on 2026-09-24 found 18 mechanisms that carry state or signals across a process boundary outside the Pi RPC event stream. Entries marked (verified) in the Outcome Ledger were re-read by the lead; the rest are supporting context.

| Id | Channel | Disposition |
|---|---|---|
| A1/A2 | launch env and argv (`buildRpcClientOptions`), `prompt.md` | stays; the channel bootstrap itself rides on env |
| A3 | fork launch envelope and `ready.json` handshake | absorbed into the authenticated channel hello |
| A4 | explore web-tools readiness proof | absorbed into the authenticated channel hello |
| A5 | execute-approval decision file, 200-ms poll | moves to the channel |
| A6 | `subtree.json` snapshot plus `fs.watch`: counts, busy-before-dispatch fence, `descendants[]` | moves to the channel as a whole |
| A7 | `ownership.json` plus lock directory | stays on the filesystem |
| A8 | session-start retention and cross-owner checkpoint fold | out of scope; `260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint` |
| A9 | parent reads the child's `session.jsonl` for telemetry | live and durable cumulative usage move to the channel; the Pi-owned file itself stays |
| A10 | cost checkpoint | stays; its evicted-baseline fold follows the usage design here |
| A11/A12 | shutdown sidecar and thread registry | self-owned, not parent-child; see `260921-bug-pi-subagent-registry-checkpoint` and `260924-bug-pi-durable-write-hygiene` |
| A13 | `--fork` source session | stays; Pi CLI contract |
| A14/A15 | shared generated skills tree, ws-mcp launcher binary and stamp | stays; not parent-child |
| A16 | ws-mcp session-key and mailbox stores | stays; Go-owned and cross-session |
| A17 | RPC stop, abort, and `getState` | stays; already native RPC |
| A18 | read-only shared configuration | stays |

## Outcome Ledger

### Verified Findings

- Source inspection of the project's Pi 0.84.4 dependency finds that `RpcClientOptions` carries `env` but not a `stdio` or spawn hook, and its child spawn fixes the standard three pipes. An extra inherited Node IPC descriptor is therefore unavailable without changing the client path. (`agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.d.ts`, `rpc-client.js`; `agents-plugin-pi/src/spawner.ts`.)
- `RpcClient` has no public method for sending an `extension_ui_response`; `send` is private (`rpc-client.d.ts`). Pi's `extension_ui_request` sub-protocol therefore cannot carry adapter data without bypassing the client's private API.
- In RPC mode the child listens for stdin `end` (`dist/modes/rpc/rpc-mode.js`). A child's channel lifetime is bounded by its parent's process lifetime.
- Node's `net.Server.listen(0, "127.0.0.1")` obtains an OS-assigned port observable after `listening`. Binding before launching each child avoids a fixed-port allocation race. This establishes the API route, not end-to-end Pi adapter behavior. ([Node v22 net API](https://nodejs.org/download/release/v22.19.0/docs/api/net.html).) The same `net` module serves Unix domain sockets and Windows named pipes through `listen(path)`/`connect(path)` with the same `net.Socket` stream API.
- The existing adapter paths are distinct:
  - approvals poll a decision file;
  - subtree counts and identity use a watched private snapshot with a mandatory busy-before-grandchild publication;
  - footer cost accounts only for direct child records and deliberately avoids recursive 250-ms polling. The footer arms only in a TUI lead or fork (`shouldArmAgentFooter`), so grandchild cost never reaches the root footer.
  - Sources: `agents-plugin-pi/src/execute-gateway.ts`, `src/subtree-lifecycle.ts`, `src/spawner.ts`, `src/agent-footer.ts`; `260921-bug-pi-subtree-publication-lifecycle-isolation`, `260913-bug-ws-pi-cost-footer-cpu-saturation`.
- (verified) `waitForDecisionFile` consumes any parseable JSON at the decision path, with no authentication, then unlinks it and resolves (`agents-plugin-pi/src/execute-gateway.ts`). The parent writes that file with a plain, non-atomic `writeFileSync`. The path lives under the child's own home, so a child's shell command can approve its own pending command today.
- (verified) `pruneStaleAgentHomes` scans every owner namespace under `<agentDir>/ws-agents/`, including other Pi sessions' namespaces (`agents-plugin-pi/src/agent-storage.ts`). An intra-tree channel cannot reach this cross-session contention.
- Incident artifacts show that the approved preflight command did not start before the worker was stopped. The saved decision file contains an unencoded `|` in its ID, while current source encodes that character, and ownership validation rejects a raw pending ID containing `|`. The loaded parent and child code versions and the actual polled path remain unconfirmed. The transcript error is separately explained by ownership validation. (`260923-bug-pi-execute-approval-accepted-worker-hangs`; `agents-plugin-pi/src/execute-gateway.ts`, `src/agent-storage.ts`.)

### Confirmed Decisions

- **Scope.** The following move to the channel:
  - execute-approval decisions (A5);
  - the whole subtree snapshot (A6): counts, the busy-before-dispatch fence, and `descendants[]`;
  - descendant usage (A9), both live and durable cumulative;
  - fork and web-tools readiness (A3, A4), folded into the authenticated channel hello.
  `ownership.json` (A7) stays on the filesystem. Root single-authority authoring of durable state is a separate research, `260924-research-pi-root-single-authority-durable-state`. Defects independent of the channel are tracked in `260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint` and `260924-bug-pi-durable-write-hygiene`.
- **Core deliverable: a message-level transport abstraction.**
  - The backend contract is `send(msg)`, `onMessage`, and `close()`, with ordered, at-most-once delivery within one connection generation. Framing lives inside each backend.
  - Disconnect is judged by process lifecycle, not by the backend. Acknowledgment and deduplication belong to the protocol layer above the backend.
  - The contract must not assume stream-socket properties such as immediate close detection, so a weaker backend can be added later without changing it.
  - One protocol contract test suite runs against every implemented backend.
- **Backend order.**
  - Backends are tried in this order: pipe (a Unix domain socket on macOS/Linux, a named pipe on Windows), then loopback TCP.
  - The parent selects the backend before spawning, by attempting to bind in that order. It passes the resulting endpoint descriptor through env, and the child only connects; there is no negotiation.
  - File backing is reserved in the abstraction but not implemented in this round.
  - If every implemented backend fails, the channel fails closed, and the diagnostic names each backend's failure.
  - Tests can force a specific backend.
- **Subtree state over the channel.** The child sends full revisioned snapshots, and the last revision wins. The busy-before-dispatch fence waits for the parent's acknowledgment of the busy revision before a grandchild is dispatched. A disconnected channel maps to waiting, matching today's missing-snapshot fail-closed behavior.
- **Usage over the channel.** Each hop reports its cumulative usage, covering itself and all descendants, with last-wins semantics. The parent stores it in the child's ownership telemetry, where it is already the single writer. After a restart, each hop recomputes the value from its own session and its children's records. Deltas are not used, because they break replay safety.
- **Threat model.** The `## Threat Model (Scope Note)` section above is confirmed as written, including its connection policy and its read-then-delete handling of the bootstrap environment. Read-then-delete is justified by accidental inheritance, the `260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env` class, rather than by a same-user attacker.
- **Sequencing.**
  - The channel infrastructure lands first. `260923-bug-pi-execute-approval-accepted-worker-hangs` is repaired on top of it, and the incident's filename/version and ownership-ID defects stay independently diagnosable.
  - Fixes that the channel absorbs wait for it: the non-atomic approval and web-readiness writes, approval filename encoding, and the missing grandchild usage roll-up.
- Approval *waiting* state need not survive a disconnected child channel; a fresh approval request is acceptable. This does not authorize replay or automatic re-execution of a command whose start status is uncertain.

### Proposals

- Non-authoritative: a versioned hello handshake, with message types kept separate for transient approvals, subtree snapshots, and cumulative usage snapshots.

### Open Questions

- Windows named pipes: what is the default access for a pipe created by Node, and are remote (SMB) clients rejected? Unix domain sockets: what path-length limit applies, where should the socket live (a short temp path rather than the child home), and how are stale socket files cleaned up after a crash?
- Does Pi's bash tool read `process.env` at command execution time, and does extension load complete before the first tool runs? Read-then-delete depends on both.
- How is an already-started command distinguished from an unconsumed approval across a disconnect? What form should a "started" marker take?
- Is the extra acknowledgment round trip before grandchild dispatch acceptable in latency terms?
- How should eviction fold a child's cumulative subtree usage into the owner checkpoint's evicted baseline without double counting?

### Rejected Alternatives

- Reusing Pi's `extension_ui_request` sub-protocol or its slash commands as the transport. There is no public response path, and the approach conflates user UI with internal transport.
- Keeping the existing file protocols as a parallel fallback next to the channel. That would mean maintaining two paths. A reserved file backend behind the same abstraction replaces this option.
- Moving `ownership.json` onto the channel or funnelling it to the root within this research. It is durable restart and deletion-protection state, and its lock guards against cross-session retention, which an intra-tree channel cannot reach.
