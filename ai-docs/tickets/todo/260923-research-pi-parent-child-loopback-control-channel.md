---
title: Investigate a Pi parent-child loopback control channel
related:
  260923-bug-pi-execute-approval-accepted-worker-hangs: approval failure to diagnose separately and revisit after the channel investigation
---

# Investigate a Pi parent-child loopback control channel

## Background

The Pi adapter has an RPC connection to each direct child, but its public `RpcClient` does not expose a generic extension-data exchange. In-flight execute approval uses a decision file and polling; recursive subtree state uses a private JSON snapshot and watcher; descendant usage does not yet roll up into the lead footer. Reusing Pi's extension slash commands and UI events as a data bus is possible in source analysis but conflates user commands/UI with internal transport. Investigate a dedicated transient, per-direct-parent-child loopback TCP channel owned by the adapter, without modifying Pi itself or its `RpcClient` implementation. This is a transport investigation, not authorization to migrate every existing channel.

The approved-but-unconsumed decision in `260923-bug-pi-execute-approval-accepted-worker-hangs` motivates the work, but its incident-specific filename/runtime mismatch and ownership-ID validation must be diagnosed on their own merits; introducing TCP would not by itself prove that bug fixed.

## Investigation Boundary

- Establish a provider-free, cross-platform bootstrap and lifecycle design: bind to `127.0.0.1` on port `0` before child launch; obtain the assigned port after listening; pass address, per-launch credential and generation through supported RPC child environment options; require authenticated child readiness before the first work prompt. Check concurrent and nested launches, stop, resume, reconnect, stale environment, port reuse, and IPv4 failure behavior. Validate Windows, macOS, and Linux assumptions rather than inferring platform acceptance from Node API availability alone.
- Separate protocol needs: a sparse, command-ID-bound parent-to-child decision while a tool is pending; child-to-parent subtree busy/completion state with the existing pre-dispatch busy fence and conservative unknown-state behavior; and replay-safe, cumulative per-hop descendant usage that does not restore recursive footer polling. Preserve the existing Pi RPC event path for reports and approval requests and durable ownership/session records for restart recovery.
- Compare dedicated TCP with Unix socket/Windows named pipe, append-only file tail, and existing atomic snapshots. Specify framing, bounds, authentication and token exposure through shell environments, acknowledgments, deduplication, timeout/backpressure, generation changes, error reporting, and the boundary between transient and durable state. A disconnected *unconsumed* approval can require a fresh user decision; an ambiguously started command must never be executed again automatically.
- Determine which simplifications actually outweigh new transport complexity. Produce evidence from a harmless, provider-free direct-child round trip and a nested-hop test before recommending actionable implementation tickets. Include a failure-injection matrix for decision delivery and child/parent restart, plus native Windows coverage or an explicit unverified gap.

## Outcome Ledger

### Verified Findings

- Source inspection of the project's Pi 0.84.4 dependency finds that `RpcClientOptions` carries `env` but not a `stdio` or spawn hook; its child spawn fixes the standard three pipes. An extra inherited Node IPC descriptor is unavailable without changing the client path. (`agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.d.ts`, `rpc-client.js`; `agents-plugin-pi/src/spawner.ts`.)
- Node's `net.Server.listen(0, "127.0.0.1")` obtains an OS-assigned port observable after `listening`; binding the listener before launching each child avoids a fixed-port allocation race. This establishes the API route, not successful end-to-end Pi adapter behavior. ([Node v22 net API](https://nodejs.org/download/release/v22.19.0/docs/api/net.html).)
- Existing adapter paths are distinct: approvals poll a decision file; subtree counts and identity use a watched private snapshot with a mandatory busy-before-grandchild publication; footer cost currently accounts for direct child records and deliberately avoids recursive 250-ms polling. (`agents-plugin-pi/src/execute-gateway.ts`, `src/subtree-lifecycle.ts`, `src/spawner.ts`, `src/agent-footer.ts`; `260921-bug-pi-subtree-publication-lifecycle-isolation`, `260913-bug-ws-pi-cost-footer-cpu-saturation`.)
- Incident artifacts show the approved preflight command did not start before the worker was stopped. The saved decision file contains an unencoded `|` in its ID while current source encodes that character, and ownership validation rejects a raw pending ID containing `|`. The loaded parent/child code versions and actual polled path remain unconfirmed; the transcript error is separately explained by ownership validation. (`260923-bug-pi-execute-approval-accepted-worker-hangs`; `agents-plugin-pi/src/execute-gateway.ts`, `src/agent-storage.ts`.)

### Confirmed Decisions

- Investigate a dedicated loopback TCP control channel between direct Pi parent and child extensions without changing Pi itself. The investigation may compare alternatives; it does not commit to TCP implementation before verification.
- Approval *waiting* state need not survive a disconnected child channel; a fresh approval request is acceptable. This does not authorize replay or automatic re-execution of a command whose start status is uncertain.

### Proposals

- Non-authoritative sequencing candidate: investigate and, if justified, implement the reusable channel first; then repair the approval bug against that infrastructure. Keep the incident's filename/version and ownership-ID defects independently diagnosable, because a transport replacement alone does not address both.
- Non-authoritative transport candidate: one bound listener and per-launch credential per direct child, with a versioned handshake and separate message types for transient approvals, subtree state, and cumulative usage snapshots.

### Open Questions

- Is loopback IPv4 and environment-variable bootstrapping sufficiently portable and secure for the supported hosts, including native Windows and shell-command inheritance of the channel credential?
- Which messages require acknowledged delivery or durable reconciliation, and how should a disconnect distinguish an unconsumed decision from an already-started command?
- Can subtree file/watch publication be retired without weakening the busy-before-dispatch fence, fail-closed settlement, or bounded descendant identity behavior?
- What attribution and checkpoint scheme lets per-hop cumulative usage survive parent/child restarts without double counting or returning to recursive JSONL scans?
- Does the value of sharing one transport across approvals, subtree state, and usage exceed its listener lifecycle, authentication, framing, and test burden?

### Rejected Alternatives

- None explicitly ruled out; Pi UI-event/slash-command reuse remains a comparison point, not the presumed infrastructure.
