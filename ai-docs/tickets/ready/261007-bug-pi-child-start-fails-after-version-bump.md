---
title: "Pi children fail to start after a ws version bump in a pre-bump lead"
related:
  261004-research-pi-post-bump-publish-worker-startup: absorbed evidence capture
  260922-bug-ws-ship-prime-pre-release-pi-runtime: absorbed earlier capture (0.46.17, 0.46.28 recurrences)
  260921-bug-pi-child-bootstrap-error-loses-diagnostic: absorbed into Phase 2
  260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap: lead-only local-devenv build this bug sits under
  260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env: origin of the child bootstrap-env blanking policy
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: af7a63b4229eea5d
sage-review-completeness-reviewed: af7a63b4229eea5d
---

# Pi children fail to start after a ws version bump in a pre-bump lead

## Background

Every ws release commits `chore(release): bump ws to X` on `develop` before
CI publishes the release assets (`ai-docs/ship/ws.md#L9-L19`; commit subject
form per `git log --grep="chore(release): bump"`, e.g. f5820fa5f). In this repository the
Pi lead runs from the working tree under the local-devenv marker
(`agents-plugin-pi/.local-devenv-runtime`). The user reports (2026-10-07) that
shipping breaks Pi-side sessions on every release; the v0.46.29 capture in
`261004-research-pi-post-bump-publish-worker-startup` (absorbed here) is one
instance, and that ship succeeded only after the lead was reloaded. The
earlier captures absorbed here record the same 404 at 0.46.17
(`260922-bug-ws-ship-prime-pre-release-pi-runtime`) and at 0.46.28, when the
Pi marker was absent and the mailbox waiter's launcher path also hit the
pending asset; and the silent child exit at 0.46.15
(`260921-bug-pi-child-bootstrap-error-loses-diagnostic`).

Mechanism, traced from source on 2026-10-07:

- The bump rewrites `agents-plugin-pi/runtime.json` (`bump-ws-version.sh`).
  The launcher addresses its binary as
  `ws-mcp-<plugin_version>-<sha256(runtime.json)[:12]>`, so the bump points at
  a path that does not exist yet.
- The marker is consulted only by `buildLocalDevenvBootstrap`, which the bridge
  runs once, at lead/fork session start (`bridge.ts`, `isLeadOrFork` gate). It
  builds ws-mcp from source with the exact version stamped and injects it
  through `WS_MCP_BOOTSTRAP_BINARY`, so the launcher installs it at the
  contract path of the `runtime.json` read at that moment.
- Children never build and get the bootstrap overrides blanked (`spawner.ts`
  `CHILD_BOOTSTRAP_OVERRIDE_ENVS`, commit bf811a0b7); per
  `260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap` they reuse whatever
  the lead's launch installed.
- A child loads `runtime.json` fresh from the working tree. When the running
  lead itself commits the bump, nothing has installed a runtime for the new
  contract, so the child's launcher falls through to the release download,
  which 404s until CI publishes. The launcher has no other fallback.
- The lead's own long-lived ws-mcp keeps working: `runtime.json` is read once
  per session start and nothing re-resolves it per call. What breaks is every
  child spawn or resume after the bump (inferred from code; the capture
  observed the lead's channel staying up).
- The capture's hand-built runtime reported `0.46.29-dev`. The launcher
  accepts a `-dev` suffix, but the bridge's `assertVersionPin`
  (`version-check.ts`) requires an exact match, so children kept failing.
- That failure was silent: `bootstrapOrFailLoud` (`index.ts`) reports through
  `ui.notify`, which in RPC mode goes to stdout as an `extension_ui_request`,
  then calls `process.exit(1)`. The parent's exit error carries only the
  child's stderr, which is blank, and no transcript turn runs (inferred from
  code; not reproduced).

## Decisions

- **Lead re-bootstraps on contract drift before a child launch.** When the
  lead is about to spawn or resume a child and the `runtime.json` contract
  differs from the one its bridge started with, the lead re-runs the
  local-devenv build and has the launcher install and stamp that binary for
  the new contract. The existing "children reuse the installed binary" path
  then works unchanged. Rejected: (a) a ship-procedure step telling the
  operator to `/reload` the Pi lead after the bump, which fails whenever it
  is forgotten and does not cover Pi sessions in other worktrees; (b) letting
  children build when the marker is present, which reverses the 260907
  "Lead-only" decision; (c) passing the lead's freshly built binary to the
  child as a bootstrap override, which reverses bf811a0b7's blanking policy
  for child launches; (d) priming the exact runtime in ship pre-flight, as
  `260922-bug-ws-ship-prime-pre-release-pi-runtime` had recorded, which
  covers only the worktree running ship and no other mid-session
  `runtime.json` drift. Building in the lead, not the child, still honors
  that ticket's worker-build concern.
- **Install mechanism.** The lead runs the unchanged launcher once as a
  one-shot subprocess with `WS_MCP_BOOTSTRAP_BINARY` set to the fresh build;
  the launcher's existing install, verify, and stamp then place it at the new
  contract path (the launcher forwards its argv to the installed binary, as
  the capture's `tools git.status` probe did). The check runs at the shared
  child-launch seam that fresh spawns and dormant resumes both pass through
  (85b1f2db), with at most one in-flight rebuild per contract hash.
- **The bridge's exact `plugin_version` pin and the launcher's local-devenv
  gate stay unchanged.** The 260907 Decisions already rejected relaxing
  either.
- **Children still never build ws-mcp themselves** (260907 "Lead-only":
  child spawn latency, no N concurrent `go build`s).
- **Scope is the local-devenv marker path.** Downstream installs consume only
  published releases, so a contract whose release asset is missing cannot
  arise there; the absorbed capture's open question about downstream
  shipping executors closes as not applicable. Rejected: a generic launcher
  fallback for a missing asset (e.g. reuse the newest installed binary of the
  same minor), which changes shared launcher semantics for every downstream
  project to cover a case they cannot reach.
- **A Pi child's bootstrap failure reaches the parent through the RPC event
  channel, not stderr.** The child lets its error-level `ui.notify` (an
  `extension_ui_request` on RPC stdout) flush before `exit(1)`, and the
  parent's child-launch path keeps the last such notify from that child and
  appends it to the "Agent process exited" error it reports. This applies to
  every Pi child, not only the marker path. Rejected: writing the message to
  stderr, because `RpcClient` forwards child stderr to the parent's own
  `process.stderr` (`rpc-client.js` stderr `data` handler) and corrupts the
  parent Pi TUI; a breadcrumb file read on blank-stderr exits, which adds a
  second channel and file lifecycle for the same message.

## Constraints

- `agents-plugin-pi/bin/ws-mcp-launcher.py` and `version-check.ts` are not
  edited (the launcher's byte-identity with `agents-plugin/bin/` stays).
- With no marker, or no contract drift, child spawn and resume behave exactly
  as today: no build, no extra launcher run.
- Following the "Lead-only" rationale, several children launched after the
  same drift must not trigger concurrent duplicate builds.

## Prior Decisions

- 260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap (2026-09-07, commit cb0a89b7): "No fingerprint cache and no cache fallback on build failure ... every lead/fork session start rebuilds from source." — bearing: constrains
- 260922-bug-ws-ship-prime-pre-release-pi-runtime (2026-09-22, Decisions): "Ship pre-flight must make the exact post-bump Pi runtime available locally after the version bump and before tests and the R4 reviewed-through pin." — bearing: superseded (absorbed; pre-flight priming rejected in Decisions)
- bf811a0b (2026-09-09, commit): "Kept the policy at the two existing environment-builder seams so every child launch kind inherits one contract." — bearing: constrains
- 85b1f2db (2026-09-26, commit): "Cleared at the single shared builder every spawn role (including dormant resume and the ask discussion fork via spawnAgent) passes through" — bearing: constrains
- 55172110 (2026-09-09, commit): "Deliberate role split: `role !== undefined` (worker/explore/fork) exits; `role === undefined` (true host lead) only notifies+returns" — bearing: constrains
- 9dc3b078 (2026-09-21, commit): "Reviewer startup surfaced empty stderr rather than the bridge version mismatch; reproducing without the child role exposed the notification" — bearing: supports
- 2372d60d (2026-09-22, commit): "The version bump made child delegation depend on a runtime asset that the pending release had not published yet." — bearing: supports
- 261004-research-pi-post-bump-publish-worker-startup (2026-10-07, Resolution): "Absorbed into 261007-bug-pi-child-start-fails-after-version-bump, which carries the traced mechanism and the settled fix." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/bridge.ts, spawner.ts, local-devenv.ts, index.ts plus tests |
| scope.surface | internal | Pi adapter internals only; launcher and version-check.ts are not edited |
| scope.new_public_symbol | no | a shared async ensure helper awaited before `client.start()` (Phase 1); names are the implementer's |
| scope.new_type_contract | no | injectable build/launcher/flush dependencies for tests; no exported contract change |
| scope.test_surface | existing | agents-plugin-pi/test/local-devenv.test.ts, spawner.test.ts, session-bootstrap-guard.test.ts |
| complexity.reuse_points | confirmed | buildLocalDevenvBootstrap and runGoBuild for the build; the launcher's existing WS_MCP_BOOTSTRAP_BINARY install path via a one-shot run (Decisions, Install mechanism); client.onEvent for the parent-side notify capture |
| complexity.side_effect_risk | moderate | adds a build and runtime install to the child spawn/resume path, with concurrent-launch dedupe |
| risk.correctness | moderate | drift detection plus shared single rebuild across concurrently launched children |
| risk.fit | moderate | introduces lead-side mutable state across spawn and resume against the 260907 Lead-only and no-cache decisions |
| risk.test | moderate | needs fake build and launcher, a concurrency case, and the env-blanking assertion |
| risk.security_or_contract | moderate | must keep the child WS_MCP_BOOTSTRAP_BINARY/URL blanking contract and the exact version pin unchanged |

## Phases

### Phase 1: Lead installs a runtime for the current contract before child launch

Implement the first decision in the lead's child-launch path, covering both
fresh spawns and resumes of dormant children.

- **Where.** `buildRpcClientOptions` (the 85b1f2db seam) is a synchronous
  builder, so the ensure step is a shared async helper awaited before
  `client.start()` at both `new RpcClient` sites: `spawnAgent` and the
  dormant resume in `sendToAgent`. Every other launch kind (ask, execute,
  explore, fork) routes through these two.
- **Who.** Gate on `isLeadOrFork`, the same gate as the startup build. A
  worker that spawns a grandchild stays uncovered, consistent with
  "Lead-only".
- **Contract identity.** sha256 of the `runtime.json` bytes, the same key as
  the launcher's binary name, so a tool-inventory change without a version
  bump is caught too. The baseline starts at the bridge's startup contract;
  after a successful re-bootstrap the settled hash is memoized so later
  sequential launches under the same contract do nothing, and concurrent
  launches share the one in-flight rebuild. A failed rebuild is not memoized.
- **Build inputs.** The rebuild re-reads `plugin_version` from the current
  `runtime.json` for the `-X main.version` ldflag; the bridge's startup
  `runtime` object is stale.
- **One-shot launcher run.** A bare launcher run execs the stdio server and
  blocks on stdin, so the run forwards a subcommand that exits promptly
  (e.g. `version`). Each re-bootstrap build artifact under
  `.runtime/local-devenv/` is removed after the launcher has copied it into
  the contract path.
- **Failure.** If the rebuild or the one-shot install fails, the child
  launch fails loudly with that error instead of launching a child that
  would hit the pending-release download, following 260907's no-fallback
  rule for build failures.

Verification:

- Unit tests with a fake build and launcher: drift before a spawn triggers
  one rebuild and install for the new contract; a resume after drift does
  the same; no drift triggers nothing; no marker triggers nothing; a worker
  role triggers nothing; concurrent launches after the same drift share one
  rebuild and later sequential launches reuse the settled result; a failed
  rebuild fails the launch with its error and a later launch retries; the
  rebuild stamps the current `plugin_version`; the child launch env still
  blanks `WS_MCP_BOOTSTRAP_BINARY`/`WS_MCP_BOOTSTRAP_URL`.
- Existing `agents-plugin-pi` test suite passes.

### Phase 2: Carry Pi child bootstrap failures to the parent

Implement the last decision in `bootstrapOrFailLoud` (child side) and the
child-launch path (parent side). The `ui.notify` call and the role split stay
(55172110): a child role still exits fail-closed, and the host lead still only
notifies. No child bootstrap path writes to stderr.

- **Child flush.** On macOS, Node pipe writes are asynchronous, so an
  immediate `process.exit` can drop the notify. Pi exposes no public flush
  primitive: in RPC mode `takeOverStdout()` reroutes `process.stdout.write` to
  stderr while `ui.notify` writes through the original stdout, so
  `process.stdout.write("", cb)` waits on the wrong stream. Use a bounded wait
  on the underlying stdout stream (yield, then `writableLength`/`'drain'` with
  a timeout) before `exitProcess(1)`.
- **Parent capture.** The child's channel hello completes before the
  session_start bootstrap fails, so the exit can surface at several later
  launch stages with different error text. Subscribe with `client.onEvent`
  before `client.start()`, keep the most recent error-level notify, and
  decorate whatever launch error the path rethrows or pushes through
  `pushSpawnFailed`, not one specific message shape.

Verification:

- Regression: a child whose bootstrap fails exits immediately and
  fail-closed, leaves no tool-less child running, and the parent's reported
  launch error carries the failure message.
- Unit test: the child's exit waits for a delayed notify write to drain
  before `exitProcess(1)`, and gives up after the bound.
- Unit test: the parent decorates the launch error whichever stage
  surfaces the exit, using the most recent error-level notify.
- Unit test: the no-role (host lead) path keeps its current notify-and-return
  behavior; nothing is written to stderr on either path.
- Existing `agents-plugin-pi` test suite passes.
