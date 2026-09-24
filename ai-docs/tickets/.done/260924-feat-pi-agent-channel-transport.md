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
completed: 2026-09-24
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

### Result (185452a2) - 2026-09-24

Implemented in `agents-plugin-pi` as `src/agent-channel.ts` (backend contract, NDJSON pipe and loopback-TCP backends over `node:net`, ordered bind pipe -> tcp with fail-closed diagnostics, `ParentChannel` / `ChildChannel` protocol layer) wired into `spawner.ts` (bind before spawn, hello and readiness waits, per-launch credential and `launchGeneration`), `index.ts` (read-then-delete bootstrap as the extension factory's first statement; the channel lives in the factory closure and closes only on a `quit` shutdown), `fork-context.ts` and `web-readiness.ts` / `web-tools.ts` (readiness published as the stage-2 message; `ready.json`, `web-tools-ready.json`, the envelope nonce and `readinessPath`, and `WS_PI_FORK_READY_PATH` / `WS_PI_FORK_READY_NONCE` / `WS_PI_WEB_READY_NONCE` are gone). Worker choices: reconnect backoff 50 ms doubling to a cap of 1000 ms with no attempt limit; parent hello and readiness waits 30 s each (`CHANNEL_HELLO_TIMEOUT_MS`, `CHANNEL_READINESS_TIMEOUT_MS`, matching `RpcClient`'s own request timeout) paired with a 500 ms lifecycle probe that counts an exit only when `RpcClient` recorded one; child-side hello wait 10 s; parent pre-hello socket bound 5 s. Readiness re-sent after a reconnect rides the reconnect hello's `resume.readiness` (a real state carrier, which is also what the "test-only resume payload" check observes). The Unix socket directory is `os.tmpdir()/ws-pi-<uid>`, created 0700 and refused (falling back to TCP) unless it is a directory owned by this uid with no group/other bits; sockets are unlinked on close, on a `process.exit` hook, and stale ones (connect refused) are swept before each bind. Tests force a backend through `ctx.channel.bind` (`force`, `socketDir`, `helloLineTimeoutMs`); no environment knob exists. A launch is claimed on the record before its first await (`record.client`, plus `record.launching` for sends that land mid-launch) so a stop or a second send during the bind can neither start a second process nor prompt an unstarted one; the spawn guards and registration run in one synchronous step after the bind. An Explore child publishes a proof failure as `{error}` so the parent fails at once with today's text.

Verification: `npm test` in `agents-plugin-pi` on macOS: 1692 tests, 1690 pass, 0 fail, 2 skipped. Contract suite (`test/agent-channel.test.ts`) runs the same cases against forced pipe and TCP backends; `test/agent-channel-launch.test.ts` drives `spawnAgent` and the `sendToAgent` relaunch with an in-process child through every readiness and hello failure (today's error texts), the cannot-reconnect case, and the stop / second-send / concurrent-alias races; `test/agent-channel.integration.test.ts` runs real Pi children for stop, resume (new endpoint, credential, generation; old generation rejected; busy), concurrent siblings, forced-drop reconnect with `resume.readiness`, drop between hello and readiness, a rejected hello exiting the child (parent reports the failed launch via the lifecycle probe), stale-socket sweep, bash-env scrubbing, and the nested hop (grandchild spawned from inside the child's bash gets its own channel). Native Windows (Node 24.15.0): the contract suite passes on named pipes and TCP (26 pass, 4 Unix-only skips); the launch and integration suites are unverified there: the launch suite hits the host's missing web-search provider and the pre-existing fork owned-home path-separator check, and the integration suite needs a provider for Explore launches, both failing identically on the pre-ticket baseline. Review: two partitioned rounds (correctness, fit, test); round one's 3 Important correctness and 2 Important test findings were fixed in 156f12be; round two found fit and test clean and two further Important correctness findings in that fix (guards no longer atomic with registration; a send during a resume's bind failing both sends), which 185452a2 fixes with tests but without a third review round. Open minors: two framers in `handleSocket`, the bootstrap reaching the child two ways (`buildRpcClientOptions` vs. env merge after bind), the stdio client's env only indirectly covered, post-reconnect checks over RPC rather than the channel, and a stop during a bind marking itself failed and pushing `spawn-failed`.
