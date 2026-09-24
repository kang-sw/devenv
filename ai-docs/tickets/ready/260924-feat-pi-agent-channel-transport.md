---
title: Pi parent-child control channel transport abstraction with pipe and TCP backends
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority, and its evidence round precedes promotion
  260924-feat-pi-agent-channel-approval-decisions: dependent migration
  260924-feat-pi-agent-channel-subtree-state: dependent migration
  260924-feat-pi-agent-channel-usage-rollup: dependent migration
  260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env: precedent for the accidental env-inheritance class the bootstrap removal guards against
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 7f4f8beeab2c063b
sage-review-design-reviewed: 7f4f8beeab2c063b
---

# Pi parent-child control channel transport abstraction with pipe and TCP backends

## Background

The Pi adapter passes parent-child state through several ad hoc filesystem protocols, because Pi's public `RpcClient` offers no generic extension-data exchange. `260923-research-pi-parent-child-loopback-control-channel` settled on a dedicated per-direct-child control channel. The adapter owns it, and Pi itself and `RpcClient` stay unmodified. This ticket builds that channel and moves the two per-launch readiness handshakes onto it. The approval, subtree, and usage migrations are separate dependent tickets.

The research's evidence round has been reconciled into the decisions below. It was a provider-free prototype on macOS, Linux, and native Windows, and its findings are under `#### Evidence round (2026-09-24)` in the research ticket's Outcome Ledger. The prototype code is on the unmerged local branch `spike/pi-channel-evidence` (`339f9c20`, `agents-plugin-pi/spike/channel-evidence/`). It is reference only; do not merge it.

## Decisions

- **Message-level backend contract.**
  - The contract is `send(msg)`, `onMessage`, and `close()`, with ordered, at-most-once delivery within one connection generation. Framing lives inside each backend.
  - Disconnect is judged by process lifecycle, not by the backend: the parent observes child exit through `RpcClient`, and the child exits on stdin end. Acknowledgment and deduplication belong to the protocol layer, not the backend.
  - The contract must not assume stream-socket properties such as immediate close detection, so that a weaker backend can be added later without changing the contract.
  - Every backend must eventually report that its connection ended. That event frees the live slot so a reconnect can be accepted. A backend without native close detection meets this obligation internally with a lease, so the protocol layer sees only the end event. The pipe and TCP backends report it through socket close. The reserved file backend's lease design is recorded in the research ledger and is not implemented here.
- **Backends and selection.**
  - Backends are tried in this order: pipe (a Unix domain socket on macOS/Linux, a named pipe on Windows), then loopback TCP on `127.0.0.1`.
  - The parent binds in that order before spawning the child. It passes the resulting endpoint descriptor, a per-launch credential, and the generation through `RpcClientOptions.env`. The child only connects; there is no negotiation.
  - File backing is reserved in the abstraction but not implemented.
  - If every implemented backend fails, the channel fails closed, and the diagnostic names each backend's failure.
  - Tests force a backend through an option injected into the parent-side bind. There is no environment knob, so production launches cannot be steered by an inherited value.
  - Unix domain sockets live in a short per-user directory under `os.tmpdir()`. A path too long for `sun_path` fails the bind with `EINVAL`, and the parent then falls back to TCP. Each socket is closed in a process `exit` hook, because Pi's RPC shutdown ends in `process.exit`. Before binding, the parent sweeps that directory: it probes each socket with connect and unlinks it on `ECONNREFUSED`. Windows named pipes need neither step.
- **Launch identity.** Every launch of a child gets a fresh endpoint and a fresh credential: spawn, resume, and relaunch alike.
  - The generation is the record's existing `launchGeneration` (`agents-plugin-pi/src/spawner.ts`). It is not secret. The hello carries it, and so does every protocol message, so dependents can scope ordering state to one launch. For example, subtree revisions restart when a new launch begins.
  - The parent accepts only the current launch's generation.
  - A reconnect within one launch reuses the endpoint, the credential, and the generation.
- **Child reconnect and resume payload.** This ticket owns the reconnect path that the dependent tickets rely on.
  - The parent keeps the child's listener open for the child's whole lifetime, and closes it when the child exits or is stopped.
  - While the child process lives, it reconnects after any connection-end event. It uses its in-memory endpoint, credential, and generation. The backoff caps the delay between attempts, not their number: the child keeps retrying until it exits on stdin end. The worker chooses the cap and records it in the Result.
  - The versioned hello has an extensible resume section. Dependent tickets fill it with their per-feature state, for example the pending approval `cmd_id` or the latest subtree snapshot. The parent hands that state to the owning feature when it accepts the reconnect.
- **Connection policy.** This baseline is normative; weakening any part of it is a defect:
  - per-launch credential and generation checked in the handshake;
  - one live connection per direct child; a new connection is rejected while one is live, never allowed to replace it;
  - reconnect only after the previous connection closed, with the same credential and generation;
  - the child reads the bootstrap values and removes them from its environment before any tool runs. This guards against accidental inheritance, for example a Pi process started from the worker's own shell. The removal is the first action of the adapter's extension factory. It precedes every process spawn, including the ws-mcp stdio client, which copies `process.env` (`agents-plugin-pi/src/mcp-stdio-client.ts`).
- **Readiness boundary: authenticate first, prove readiness second.**
  - Stage 1, hello. The extension factory's hello authenticates the connection with credential, generation, and version, and gates startup. The child awaits the accepted hello inside the factory and throws on failure, so Pi exits at startup: the child fails closed.
  - Stage 2, readiness message. Fork and web-tools readiness data does not exist when the factory runs, because both are produced in `session_start` (`agents-plugin-pi/src/index.ts`, `src/web-tools.ts`). The child sends it later as a separate readiness message over the authenticated connection, with the same payload the readiness files carry today.
  - The parent validates that payload with today's checks (`validateForkReadiness`, `verifyWebReadiness`), at today's points in the launch and relaunch paths in `spawner.ts`. It waits for the message there instead of reading a file.
  - A launch is ready only after the hello is accepted and, for fork and Explore children, the readiness message is validated. `RpcClient.start()` resolves after a fixed 100-ms wait, so its resolution is not readiness.
  - If the connection ends after the hello is accepted but before the readiness message is validated, the child resends the readiness message after reconnecting. The parent's readiness timeout still bounds the whole wait.
  - The parent bounds both waits with timeouts, paired with lifecycle observation. A timeout or rejection fails the launch through today's readiness-failure errors, for example `ws-pi-agent: fork did not publish readiness` and `web-search-tool-unavailable: …`. The worker chooses the bounds and records them in the Result.
  - The channel is a required foundation for subagents. There is no alternative readiness path when it fails.
- **Threat model.** Same-user processes are outside the threat model. Do not escalate same-user forgery into a blocking finding, and do not add hardening beyond the baseline. Other OS users remain in scope, and the credential is what stops them. The research ticket's `## Threat Model (Scope Note)` is the full statement.
- **Readiness absorbed.** Fork readiness (`ready.json` in the fork launch envelope) and Explore web-tools readiness (`web-tools-ready.json`) move to the stage-2 readiness message, and their file handshakes are retired. Retired with them:
  - the envelope's `nonce` and `readinessPath` fields;
  - the `WS_PI_FORK_READY_PATH`, `WS_PI_FORK_READY_NONCE`, and `WS_PI_WEB_READY_NONCE` (`WEB_NONCE_ENV`) environment variables;
  - the `web-tools-ready.json` writer and reader.

  The fork *context* envelope that carries input into the child is not a readiness handshake and is not retired here. Web readiness stops depending on `SubtreeChannel.nonce`, including `WEB_NONCE_ENV` and `verifyWebReadiness`. `SubtreeChannel` itself is retired later, by `260924-feat-pi-agent-channel-subtree-state`.
- **Escalation.** If verification contradicts a decision above, the worker stops and escalates instead of choosing an alternative. Example: removing the bootstrap values from `process.env` does not keep them out of the child's bash tool.

## Prior Decisions

- 260923-research-pi-parent-child-loopback-control-channel (2026-09-24, Confirmed Decisions): "Evidence reconciliation (2026-09-24). The following settle the evidence round's findings and are carried into 260924-feat-pi-agent-channel-transport: Connection-end obligation..." — bearing: supports
- 424d4af3 (2026-09-24, commit): "User declared the channel a required foundation for subagents with no alternative readiness path." — bearing: supports
- dc857cf3 (2026-09-24, commit): "Owner made the transport abstraction, not TCP, the core deliverable: backend order pipe/UDS -> loopback TCP, file backing reserved but unimplemented, fail closed with per-backend diagnostics." — bearing: supports
- 260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env (2026-09-09, Result 649ca5cf): "Direct child environments delete the inherited bootstrap binary/URL overrides; RPC options explicitly empty them so the SDK's parent-environment merge cannot restore stale values." — bearing: constrains
- 260924-feat-pi-agent-channel-subtree-state (2026-09-24, Decisions): "A disconnected or not-yet-connected channel maps to "waiting", preserving today's fail-closed semantics." — bearing: constrains
- 260924-feat-pi-agent-channel-approval-decisions (2026-09-24, Decisions): "The parent sends each decision to the child as a channel message bound to its `cmd_id`. The decision file, `WS_PI_APPROVAL_DIR`, and the 200-ms poll are retired." — bearing: constrains
- 32687830 (2026-09-12, commit): "Isolated capture plus per-launch facade provenance/nonce readiness fails before prompting and repeats on dormant/restarted launch" — bearing: constrains
- 55172110 (2026-09-09, commit): "Reuses RpcClient's existing exit-rejection path instead of inventing a new file-based readiness/IPC protocol" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | new channel module under agents-plugin-pi/src/, agents-plugin-pi/src/spawner.ts#L2127-L2135 prepareForkLaunch ready.json, agents-plugin-pi/src/fork-context.ts#L201-L211 readForkLaunchContext, agents-plugin-pi/src/web-readiness.ts, agents-plugin-pi/src/index.ts#L386 extension factory, agents-plugin-pi/src/mcp-stdio-client.ts#L183 env spread ordering |
| scope.surface | cross-module | spawner launch path, fork and explore web-tools readiness, extension factory startup, and a new parent-child env bootstrap contract; no ws MCP or shipped-skill surface |
| scope.new_public_symbol | yes | adapter-internal exported channel backend contract, bind and connect entry points, and new bootstrap env names; exact names not yet chosen |
| scope.new_type_contract | yes | backend contract send msg, onMessage, close plus connection-end event; versioned authenticated hello carrying credential and generation |
| scope.test_surface | new-files | new backend contract suite; existing agents-plugin-pi/test/web-readiness.test.ts, fork.test.ts, spawner.test.ts, fork-context.test.ts, mcp-stdio-client.test.ts cover the retired readiness files and env paths |
| complexity.reuse_points | confirmed | RpcClientOptions.env in node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.d.ts#L20; Node net listen and connect; reference prototype on unmerged spike 339f9c20 agents-plugin-pi/spike/channel-evidence/ |
| complexity.side_effect_risk | high | deletes bootstrap values from process.env, creates and sweeps socket files under os.tmpdir, adds process exit hooks, and retires readiness files every fork and explore launch uses |
| risk.correctness | high | connection policy, generation and reconnect semantics, hello timeout paired with lifecycle, and a fail-closed readiness gate on every subagent launch |
| risk.fit | moderate | wsPiBridgeExtension at agents-plugin-pi/src/index.ts#L386 is synchronous today and must await the hello before any spawn including the ws-mcp stdio client |
| risk.test | high | needs real Pi children across macOS, Linux, and native Windows plus adapter-level stop, resume, relaunch, sibling, and SIGKILL cleanup cases the evidence round left open |
| risk.security_or_contract | high | per-launch credential is the only barrier against other OS users on loopback TCP and Windows pipes; bootstrap env removal is a normative baseline |

## Phases

### Phase 1: Channel transport, bootstrap, and readiness message

Build these pieces:

- the backend contract and the pipe and TCP backends;
- the parent-side ordered bind and the child-side connect;
- the authenticated versioned hello with its extensible resume section;
- the child reconnect loop;
- the connection policy;
- the read-then-delete of bootstrap values.

Move fork and web-tools readiness to the stage-2 readiness message, and remove their files.

Verification:

- One protocol contract test suite passes against both the pipe and the TCP backends, each forced explicitly.
- A provider-free direct-child round trip works. A nested hop works too: a child that is itself a parent establishes its own channel to a grandchild.
- Backend fallback: a failed pipe bind falls back to TCP. When both fail, the channel fails closed, and the diagnostic names both failures.
- The connection policy is enforced. A second connection is rejected while one is live. The connection-end event frees the slot, so a reconnect after close with a matching credential and generation succeeds. A mismatched credential or stale generation is rejected.
- Bootstrap values are absent from the environment of the child's bash tool and of the ws-mcp stdio client.
- A child whose hello fails or times out exits at startup. The parent reports it as a failed launch and does not treat the child as ready.
- Fork and web-tools readiness succeed and fail as today, but through the stage-2 readiness message, and no readiness files are written. A missing or invalid readiness message fails the launch with today's errors.
- Unix socket cleanup: no socket file remains after a normal child shutdown. After a SIGKILL, the next bind sweeps the stale socket and succeeds.
- Child reconnect: after the parent forcibly drops a live connection, a real Pi child reconnects on its own. A test-only resume payload in the hello reaches the parent, and the child keeps working.
- A connection dropped between the accepted hello and the readiness message recovers: the child resends readiness after reconnecting, and a child that cannot reconnect before the readiness timeout fails the launch.
- Run these through the ws adapter, not only at the policy level. The evidence round left them as gaps.
  - Stop: the channel closes, the listener is closed, the Unix socket file is removed, and no later connection is accepted.
  - Resume or relaunch: the new launch gets a new endpoint, a new credential, and the next `launchGeneration`. A hello carrying the previous launch's generation is rejected.
  - Concurrent sibling launches: each sibling gets its own endpoint and credential. One sibling's credential presented at another sibling's endpoint is rejected.
- Native Windows coverage, or an explicitly recorded unverified gap.
