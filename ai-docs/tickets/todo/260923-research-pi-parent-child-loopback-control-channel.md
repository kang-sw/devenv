---
title: Investigate a Pi parent-child control channel with a pluggable transport
related:
  260923-bug-pi-execute-approval-accepted-worker-hangs: repaired after and on top of this channel; its incident defects stay independently diagnosable
  260924-research-pi-root-single-authority-durable-state: split out; root-authored durable state is not in this scope
  260924-bug-pi-retention-fork-prune-and-cross-owner-checkpoint: TCP-independent defect found by the inventory; its checkpoint part waits for this research's usage design
  260924-bug-pi-durable-write-hygiene: TCP-independent defects found by the inventory
  260920-bug-pi-nested-subagent-terminal-delivery-stall: cause unknown; this channel is not claimed to fix it
  260924-feat-pi-agent-channel-transport: derived child - transport abstraction, backends, bootstrap, readiness hello
  260924-feat-pi-agent-channel-approval-decisions: derived child - approval decisions (A5)
  260924-feat-pi-agent-channel-subtree-state: derived child - subtree snapshot (A6)
  260924-feat-pi-agent-channel-usage-rollup: derived child - cumulative usage (A9)
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

Other OS users on the same machine remain in scope, and the credential is what stops them. Loopback TCP is reachable by any local user. The evidence round measured the pipe backends' default access; see `#### Evidence round (2026-09-24)` under Verified Findings.

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

#### Evidence round (2026-09-24)

A delegated, provider-free prototype ran in a separate worktree. Its code is spike commit `339f9c20` on the local, never-merged branch `spike/pi-channel-evidence`, under `agents-plugin-pi/spike/channel-evidence/`. The README there gives the rerun command for each experiment.

- **Setup.**
  - Platforms: macOS (Node 25.9, libuv 1.52.1); Linux in a non-root `node:24-bookworm` Docker container (Node 24.21, libuv 1.52.1); native Windows 10.0.26200 on the smoke host (Node 24.15, libuv 1.51.0).
  - Children were real Pi 0.84.4 `RpcClient` processes, direct and nested.
  - Pi RPC mode exits when no model is configured, so the prototype registered Pi-ai's in-process faux provider. That involves no network and no LLM. The faux model also drives the real bash tool through the agent loop.
  - No evidence contradicted a Confirmed Decision.
- **Backend contract.**
  - One NDJSON-over-`net.Socket` wrapper served both the pipe and TCP backends, each forced explicitly.
  - The shared suite passed on all three platforms. It covered:
    - both directions;
    - embedded newline, U+2028, and non-ASCII payloads;
    - 20k messages each way, in order and exactly once;
    - a 512 KiB frame split across chunks;
    - close reaching the peer;
    - send-after-close throwing.
- **Direct child.** A real Pi child completed the authenticated hello and ping/pong on all three platforms, with bootstrap values passed through `RpcClientOptions.env`. Measured start-to-ready time was about 0.3–0.4 s on macOS and Linux and 1.2–1.3 s on Windows.
  - `RpcClient.start()` resolves after a fixed 100 ms wait, not at readiness (`dist/modes/rpc/rpc-client.js`). The parent must await the hello itself.
  - Any extension-factory throw makes Pi exit with code 1 at startup (`dist/main.js`, the `Failed to load extension` path). A child whose hello fails is therefore stopped at startup.
- **Nested hop.** A child bound its own channel and launched a grandchild. The grandchild's ready and pong messages reached the root through the child on all three platforms. The grandchild saw only the child's bootstrap values, and deleted them.
- **Failure injection.**
  - A failed pipe bind fell back to TCP. Tested failures were a squatted name on every platform, plus a missing directory and an over-long path on Unix. When both backends failed, the channel failed closed, and the diagnostic named each backend's error.
  - Connection policy results:
    - a second authenticated connection while one was live got `busy`, and the first kept working;
    - reconnect after close with the same credential and generation succeeded;
    - a wrong credential was rejected with `auth`, a stale generation with `generation`, and a bad version with `version`;
    - an unauthenticated connection did not occupy the live slot;
    - a real Pi child reconnected with its in-memory credential.
  - After a child SIGKILL, the public `RpcClient.getState()` rejected with `Agent process exited`. That took 2–4 ms on macOS and Linux and 10 ms on Windows, where it reports `code=1` with no signal. Inside the child, the same observation of a grandchild kill took 3–10 ms.
  - Freeing the live slot for a reconnect needed a connection-end signal. The prototype used an advisory backend `onEnd` event.
- **Bootstrap env read-then-delete.**
  - Source path: `RpcClient.bash`, the agent bash tool, and the adapter's `pi.exec` all build the child environment from the live `process.env` at execution time (`core/tools/bash.js`, `utils/shell.js`, `core/exec.js`). Extension factories are awaited inside runtime creation, before `runRpcMode` attaches the stdin reader (`main.js`, `rpc-mode.js`), so no RPC command or tool can run before load completes.
  - Measured on all three platforms: the values were absent from `RpcClient.bash`, from the faux-driven bash tool, and from a Pi launched inside that bash. In a negative control with the delete disabled, the values leaked, and the parent rejected the nested Pi as `busy`. With the hello delayed by 1.5 s, `bash` and `getState` calls sent right after `start()` returned only after the hello was accepted.
  - Two caveats:
    - The original process environment block still shows deleted values to same-user readers: `ps -E` on macOS and `/proc/<pid>/environ` on Linux. This is outside the threat model.
    - A process spawned before the delete inherits the values. The adapter's ws-mcp stdio client, for example, spreads `process.env` (`agents-plugin-pi/src/mcp-stdio-client.ts`).
- **Unix domain sockets.**
  - Node binds without silent truncation. Over-long paths fail with `EINVAL` at 110 bytes and above; 103 and 104 bytes bind. `sun_path` is 104 bytes on macOS and 108 on Linux. A missing directory reports `EACCES`, because libuv maps `ENOENT` to it.
  - `os.tmpdir()` is a usable location. On macOS it is per-user with mode 0700, and a default path is 73 bytes. On Linux it is `/tmp`, mode 1777, and the socket is created 0755. Another user's connect fails with `EACCES` in Docker, while loopback TCP from that user connects.
  - `server.close()` unlinks the socket. SIGKILL or `process.exit()` without close leaves the file, and then `listen` fails with `EADDRINUSE` while `connect` gets `ECONNREFUSED`. Probing with connect and unlinking on `ECONNREFUSED` restores bind. Pi's RPC shutdown ends in `process.exit`, so a child that is also a parent needs a `process.once("exit")` close hook. With that hook, the macOS run left no files.
- **Windows named pipes.**
  - libuv creates the pipe with a NULL security descriptor and without `PIPE_REJECT_REMOTE_CLIENTS`.
  - Measured SDDL: `O:BA D:(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;BA)(A;;FR;;;WD)(A;;FR;;;AN)`. System and Administrators get full access; Everyone and Anonymous get read-only. The owner was elevated, so a non-elevated owner's ACE is unverified.
  - A read-only client cannot write a hello.
  - Connections through `\\127.0.0.1\pipe\…`, `\\<hostname>\pipe\…`, and `\\localhost\pipe\…` were all accepted. Remote clients are not rejected, so the credential is the barrier, as the threat model assumes. Pipe names are listable by any local process.
  - Killing the listener frees the name with no stale-file cleanup. Binding a name twice fails with `EADDRINUSE`.
- **Acknowledgment round trip** (p50/p99):

  | setting | pipe | TCP |
  |---|---|---|
  | in-process, macOS / Linux / Windows | 8/17 · 7/20 · 33/69 µs | 22/43 · 8/16 · 42/69 µs |
  | Pi child, macOS / Linux / Windows | 18/53 · 21/246 · 80/267 µs | 60/85 · 21/86 · 80/181 µs |

  The worst single sample was 0.75 ms, about three orders of magnitude below the time to spawn a grandchild (0.3–1.3 s).
- **Gaps.**
  - No second real OS user was tested on macOS or Windows; those conclusions rest on the measured directory mode and SDDL.
  - The Windows remote-client test used SMB loopback as the same user, not a separate machine.
  - Linux ran in Docker, not on bare metal.
  - Stop, resume, a generation bump across a real relaunch, and concurrent sibling launches were covered only at the policy level, not through the ws adapter.
  - The readiness hello replacing the fork and web-tools readiness handshakes was not prototyped.

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
- **Derived children (2026-09-24).** The work is split into four `todo/` feature tickets:
  - `260924-feat-pi-agent-channel-transport`: the transport and the readiness hello;
  - three migrations, each `blocked-by` the transport ticket and independent of one another: `260924-feat-pi-agent-channel-approval-decisions`, `260924-feat-pi-agent-channel-subtree-state`, and `260924-feat-pi-agent-channel-usage-rollup`.
  - Each open question below is assigned to one child's verification. A child whose verification contradicts a decision in this ledger stops and escalates.
  - `260923-bug-pi-execute-approval-accepted-worker-hangs` is `blocked-by` the approval child.
- **Evidence round before promotion.** The evidence-first boundary in `## Investigation Boundary` stands. The evidence is gathered now by a delegated, provider-free prototype in a separate worktree, not folded into a child's phase. Its results are recorded in this ledger and reconciled into `260924-feat-pi-agent-channel-transport` before that ticket is promoted to `ready/`.

### Proposals

- Non-authoritative: a versioned hello handshake, with message types kept separate for transient approvals, subtree snapshots, and cumulative usage snapshots.
- Non-authoritative, from the evidence round; to be settled before `260924-feat-pi-agent-channel-transport` is promoted:
  - **Connection-end signal.** The policy "reconnect only after the previous connection closed" needs a signal that frees the live slot. Candidate: every backend must eventually report connection end, and a backend without native close detection meets that obligation internally, for example with a lease. Process lifecycle still decides child death.
  - **Readiness boundary.** The parent treats a child as ready only after an accepted hello. It pairs the hello timeout with lifecycle observation, because `start()` resolution is not readiness. The child fails closed by throwing from its extension factory.
  - **Unix socket cleanup.** Use a short per-user directory under `os.tmpdir()`. Close in an exit hook, and before binding, sweep that directory with probe-then-unlink for stale sockets. Windows pipes need neither.
  - **Delete ordering.** Deleting the bootstrap values is the adapter extension factory's first action, before any process is spawned, including the ws-mcp stdio client.

### Open Questions

- How is an already-started command distinguished from an unconsumed approval across a disconnect? What form should a "started" marker take?
- How should eviction fold a child's cumulative subtree usage into the owner checkpoint's evicted baseline without double counting?
- Answered by the evidence round: named-pipe default access and remote clients, Unix socket path length, location, and cleanup, bash-tool env timing and extension-load ordering, and acknowledgment latency before grandchild dispatch. See `#### Evidence round (2026-09-24)`.

### Rejected Alternatives

- Reusing Pi's `extension_ui_request` sub-protocol or its slash commands as the transport. There is no public response path, and the approach conflates user UI with internal transport.
- Keeping the existing file protocols as a parallel fallback next to the channel. That would mean maintaining two paths. A reserved file backend behind the same abstraction replaces this option.
- Moving `ownership.json` onto the channel or funnelling it to the root within this research. It is durable restart and deletion-protection state, and its lock guards against cross-session retention, which an intra-tree channel cannot reach.
