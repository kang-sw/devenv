---
title: "Shared dashboard daemon restarts without an explicit swap"
---

# Shared dashboard daemon restarts without an explicit swap

## Background

Dogfood surprise, 2026-11-13. The shared dashboard daemon of this checkout
(`~/.cache/ws@kang-sw-devenv/dashboard`, port 29417) was replaced twice
without anyone running `python3 scripts/dev.py rerun daemon`:

- 15:04:15Z: pid 98387 -> 63011. `dashboard.log` was recreated, so the old
  daemon's log was lost.
- 15:19:51Z: pid 63011 -> 8932, build `content_hash=e7c6e270d283`. A later
  explicit swap from develop `1ed33f98d` reported "running daemon has
  identical content", so the auto restart had already built the root
  checkout's current source.

Every lead and worker involved (MAIN, UX, EXEC workers) states it ran no
daemon start or swap command. Each restart came 1-3 minutes after a commit
and a `go test` finished in the root checkout.

Impact: a MAIN `whiteboard_update` carrying about 74 KB of panels failed
twice with "dashboard daemon not reachable; the request did not complete"
(`agents-plugin/bin/ws-mcp-launcher.py:1639`, `:1677`) at about 15:03Z,
just before the first restart. Small updates sent at the same time
succeeded. Live sessions also lose their stream for the restart window.

## Hypotheses to check

1. The launcher (local-devenv source build) sees source or metadata identity
   newer than the running daemon and replaces it on its own, for example
   when an MCP session reconnects after the root's source changes.
2. A test leaks onto the shared state directory (`WS_CACHE_HOME` unset in
   some test) and starts or stops the real daemon.
3. The large-request failure is independent: a size or timeout limit on the
   launcher -> daemon proxy path. Reproduce it on a stable daemon before
   ruling this out.

The answer decides whether this is a bug (an unannounced replacement, or a
test touching shared state) or intended behaviour that needs a notice. The
project's rule is that only the EXEC lead swaps the shared daemon, with mail
to the peer leads before and after.

## Evidence (2026-11-14, EXEC lead)

One unannounced replacement was traced end to end with `ps`. It supports
hypothesis 1.

- 01:22:41 KST: holder 4809 starts a hosted `pi` (4846), and its
  `ws-mcp-launcher.py serve --stdio` (5051) starts at 01:22:42.
- 01:22:43: a new shared daemon, 5776, appears (parent 1). It runs
  `ws-mcp-dashboard-f493f998ed0c90bd... dashboard --port 29417 --build-time
  1794586962277`. That build time is a fresh local-devenv build, made at
  that moment (VERIFIED: `dev.py rerun daemon` later reported "daemon before:
  ... build_time=1794586962277 content_hash=f493f998ed0c").
- The previous daemon (93316) is gone.
- The EXEC lead had just merged three branches into the root checkout's
  `develop` between about 01:21 and 01:22 (ef7940b8a, c49ae5a11,
  3ee1d448b), so the root's source changed seconds before (INFERRED link:
  the launcher saw newer source identity and rebuilt and replaced the
  daemon).
- Side effect: 5776's environment had PATH starting with
  `/Users/kang-sw/devenv/agents-plugin-pi/node_modules/.bin` plus npm
  run-script entries, inherited through the hosted Pi's launcher. So the
  daemon resolved the repo's dev Pi 0.84.4, and `pi.spawn` and `pi.stop`
  refused for every session ("pi 0.84.4 is installed; Pi subagents need pi
  1.0 or later"; UI/UX EXEC saw it on `pi.stop`). Sibling ticket:
  `261114-bug-ws-pi-version-probe-never-refreshes`.
- Fixed by `python3 scripts/dev.py rerun daemon` from a clean shell (pid
  12261).

## Investigation evidence - 2026-11-14 (EXEC 3)

This addition records evidence only. It changes no runtime, replacement
policy, ticket status, or implementation authority. Source coordinates below
refer to `9854ff350fa1b0eccfb5c71649c5190c405925b9`, not an assumed live build.

### Verified source mechanism

- **VERIFIED:** `agents-plugin/bin/ws-mcp-launcher.py:1858-1862` resolves
  local-devenv mode, forced installation and the source fingerprint.
  At `:1892-1899`, the usable-daemon shortcut applies only without forced
  installation; a current compatibility stamp can avoid rebuilding, while
  forced local mode with a stale stamp requests a source rebuild. This is
  not a claim that every session start unconditionally rebuilds.
- **VERIFIED:** `needs_start` at `:1347-1371` requests replacement for an
  older version, or for a same-core daemon with an older build timestamp
  whose content hash differs or is unavailable. Equal known hashes suppress
  replacement. `connect` at `:1437-1444` invokes `start_daemon` automatically
  when `allow_start` is true and the daemon is absent or needs replacement.
  These cited checks contain no active-turn or owner swap-approval gate.
- **VERIFIED:** `start_daemon` at `:1394-1409` runs the staged binary's
  `daemon-start` command with an environment copied from the launcher.
  This supplies a source mechanism for inherited PATH; it does not alone
  prove the environment of any historical process.
- **VERIFIED separate Pi build path:**
  `agents-plugin-pi/src/bridge.ts:916-932` invokes
  `buildLocalDevenvBootstrap` for lead/fork roles at bridge startup.
  With a valid local-devenv marker, `local-devenv.ts:153-185` runs
  `go build` using source HEAD in the version stamp without a fingerprint
  cache; `:207-209` returns `WS_MCP_BOOTSTRAP_BINARY`. The Python launcher's
  bootstrap override forces installation at `:1887-1891`. Thus a local-dev
  Pi lead or fork bridge startup can build even with unchanged source.
- **VERIFIED worker/explore distinction:** `bridge.ts:916-924` skips that
  TypeScript startup build for these roles. Native child launch arguments
  load the parent's extension (`spawner.ts:2935-2939`) and clear inherited
  bootstrap overrides (`:2929-2931`). Their Python MCP launcher still has
  the stale-source-stamp rebuild path above. The lead's separate pre-child
  guard rebuilds on `runtime.json` contract drift and memoizes settled
  contracts (`local-devenv.ts:266-302`); it does not rebuild for every
  ordinary worker/explore spawn.
- **VERIFIED connection route:** the session bootstrap calls `startBridge`
  (`agents-plugin-pi/src/index.ts:875-888`), which creates the client
  (`bridge.ts:954-962`) as `python3 <launcher> serve --stdio`
  (`mcp-stdio-client.ts:346-360`). The same route is loaded by native child
  extension arguments above. Daemon-hosted leads load bundled extension
  entries, with an MCP bridge fallback when unavailable
  (`agents-plugin-tool/internal/wsagent/pi_lead.go:35-62`).
- **INFERRED operational consequence:** a develop merge that changes
  fingerprinted inputs can make the next MCP launcher build a fresh binary
  and replace an older/different daemon without any manual build command.
  No branch-name gate or background source watcher is present in the cited
  launcher checks: a merge alone is not an immediate replacement trigger.
  CLI-mode calls using a usable daemon bypass installation/replacement
  (`agents-plugin/bin/ws-mcp-launcher.py:1881-1885,1924-1926`), while the
  separate Pi lead/fork startup build has the behavior described above.

### Retained incident testimony and causal limits

Transcript reference R is
`/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/rebelrepugnantstonewareemcee/pi/session.jsonl`;
line numbers below are physical JSONL lines.

- **VERIFIED retained report:** R:39, received at 2026-11-14 01:07:37Z,
  records MAIN observing daemon PID 68420 without a swap notification.
  At that point MAIN explicitly left open whether EXEC had swapped it.
- **VERIFIED retained report:** R:59, received at 01:12:15Z, forwards EXEC's
  01:11:59Z account of a 01:05:40Z replacement: holder 67828 -> Pi 67831 ->
  installed launcher 67844, followed by daemon 68420 with content prefix
  `d2c34344`. The report attributes rebuilding to `.local-devenv-runtime`
  pointing at the primary checkout at `4804cf47f`. MAIN's appended note
  attributes the replacement to its own session start.
- **INFERRED historical cause:** that reported launcher/process sequence,
  the earlier process evidence above, and the verified source replacement
  path support automatic dev-source replacement as the cause of an
  unannounced restart. EXEC's historical wording was "ROOT CAUSE VERIFIED";
  this investigation verified the retained testimony and source mechanism,
  not the historical process tree independently. No new live reproduction,
  PID observation, environment capture or daemon operation was performed.
- **Still unverified:** the exact initiating process for each Nov 13
  15:04:15Z and 15:19:51Z restart, whether any test touched shared state,
  and whether the roughly 74 KB update failures have an independent size
  or timeout cause. Temporal proximity does not settle these questions.

### Related partial repair and policy hold

`261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work` already records
this launcher mechanism and a partial bounded-wait repair. Its
`## Result (partial)` explicitly leaves replacement policy and completion
mail durability open. That repair is not evidence that automatic replacement
has been disabled. Avoid a duplicate replacement-policy repair in this ticket.

MAIN subsequently relayed the owner's correction **(703)=a**, superseding
the earlier answer **b**: replace the shared daemon only at explicitly
coordinated, designated timing. Ordinary connection must not replace it
because of a newer same-core development build. The advance-notice design
from option **b** is withdrawn, not pending.

The owner subsequently confirmed **(706)=a**, **(709)=a**, and **(714)=a**.
The accepted design below supersedes the earlier narrower fingerprint-reuse
repair and the earlier exclusion of restart/rollback. No notice design remains.

## Accepted repair design - 2026-11-14

### Confirmed decisions

- **(703)=a:** shared development-daemon replacement happens only at explicitly
  coordinated, designated timing, not because an ordinary connection finds a
  newer same-core development build. Preserve released-version compatibility.
- **(714)=a:** ordinary launcher startup must not automatically build or replace
  a development runtime because its source fingerprint changed. Surface that a
  new build is available; build and swap only through an explicit command at
  designated timing. This also removes the unconditional local-dev Pi lead/fork
  startup build. Reuse the selected runtime rather than merely caching that
  unconditional build; stale source alone is not permission to rebuild.
- **(706)=a:** keep dead-daemon recovery separate from replacing a running
  daemon. Automatically restart a dead daemon using the last-known-good build,
  never an unaccepted newer candidate. Roll back a designated swap that fails
  its health checks, and record the recovery in the board log.
- **(709)=a:** a candidate earns last-known-good status after post-start checks
  covering hello, session lookup, whiteboard read and mailbox receive, followed
  by approximately 30 minutes of quiet observation before promotion. Startup
  liveness alone does not establish session-feature correctness.
- **(709)=a:** emergency rollback must be available through a launcher CLI
  command even when the dashboard is broken, plus a dashboard button visible
  only for development mode identified by `.local-devenv-runtime`.
- Compatibility target **T0**: preserve existing release/protocol/store meanings;
  use additive recovery state and command surfaces if needed. Do not remove or
  reinterpret an existing protocol, manifest or tracker field. Any broader
  cross-version change stops for MAIN's approval before implementation.

### Verified development-only source-build boundary

The source-fingerprint auto-build path requires a valid local development
contract: `local_devenv_source_fingerprint` returns no fingerprint without it
(`agents-plugin/bin/ws-mcp-launcher.py:546-561`), and
`install_local_devenv_runtime` returns false without that same contract
(`:648-652`). Main computes the fingerprint only when `local_enabled`
(`:1858-1862`), and the forced source-rebuild branch requires both forced
installation and local mode (`:1895-1899`). Non-development runtime repair
continues to use the released download path (`:721-726`). Explicit
`WS_MCP_BOOTSTRAP_BINARY` or `WS_MCP_BOOTSTRAP_URL` overrides can force an
installation without that marker (`:534-541,679-720`); those overrides are not
themselves Go source-build commands. Do not describe every runtime install or
released-version replacement as development-only.

## Constraints

- This capture authorizes design text only under the current load throttle:
  no daemon operation, new build/test, process kill or installation change.
  Execution and shared-daemon activation remain separately coordinated.
- Preserve first-install and released-version compatibility behavior while
  changing development auto-build/replacement policy. A missing, dead, hung,
  incompatible or newer running daemon must be distinguished explicitly.
- Recovery self-checks must not consume real unread user mail or overwrite
  user/session/board state. Specify safe probe ownership and cleanup before
  implementing the mailbox-receive check.
- Keep previous accepted binaries available for rollback; current staged-binary
  pruning is not a last-known-good retention guarantee. Recovery state must
  distinguish an accepted build from a candidate and a merely healthy process.
- Do not claim that a quiet interval proves every session feature correct.
  Technical design review must define failed-check handling, promotion timing,
  concurrent launcher ownership, interrupted swaps and bounded restart behavior.
- Mail durability and large-request failures remain separate investigations.
- Before launcher/tooling edits read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, and `ai-docs/manuals/ws-mcp.md` where
  applicable. Keep launcher copies in ws, wsflow and Pi consistent; read
  `ai-docs/manuals/wsflow-mirroring.md` for mirrored plugin edits. Consult Pi SDK
  integration documentation before changing Pi bootstrap behavior.

## Phases

### Phase 1: Separate ordinary connection from explicit development build and swap

Implement the confirmed (703)/(714) boundary across the shared launcher copies
and native Pi bootstrap. Ordinary startup reuses the selected runtime and reports
source drift without building or replacing a live daemon. An explicit command
owns build and coordinated swap. Preserve normal released-runtime compatibility
and installation. Keep dead/hung-daemon handling distinct rather than disabling
all start or repair paths.

Verify with isolated launcher/bootstrap fixtures: unchanged and stale development
fingerprints, lead/fork/worker startup, live/dead/hung daemon distinctions, explicit
build/swap entry, and ordinary released-plugin first install and reconnect. Assert
that ordinary development startup executes neither Go build nor live replacement.

### Phase 2: Add accepted-build recovery and guarded promotion

Implement (706)/(709): retain a last-known-good runtime, check a designated
candidate through the approved session-facing operations, observe approximately
30 quiet minutes before promotion, recover a dead daemon with the accepted build,
and roll back failed candidate activation. Preserve accepted binaries through
staging/pruning and record recovery in the board log. Define internal recovery
state, safe probes and concurrent-operation behavior through technical design
review before promotion to ready; do not infer these mechanisms from old liveness
checks or claim they already exist.

Verify deterministically with isolated state, disposable probes and controlled
time: successful checks, failed checks, interrupted startup, rejected promotion,
failed candidate, retained accepted binary, dead-daemon recovery, and concurrent
callers. No test may stop or replace the shared dogfood daemon.

### Phase 3: Expose emergency rollback outside the dashboard

Provide the approved launcher CLI rollback command and development-only dashboard
button. Both target the retained accepted build through the same guarded recovery
operation; the CLI must work while the dashboard is unavailable. Preserve a usable
recovery path when the candidate UI or session controls regress.

Verify CLI behavior with a broken/unreachable dashboard, development-marker UI
visibility, rejected unsafe/missing rollback targets, recovery log visibility and
consistent operation from both controls. Run independent design/completeness and
implementation reviews before integration or activation; record exact commands,
results, compatibility boundaries and remaining acceptance gaps.
