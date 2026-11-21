---
title: "데몬 버전 중재: 시작기 사전 점검 및 기동 후 데몬 자체 경합"
related:
  261115-feat-werkschaft-electron-daemon-override: "Werkschaft start/replace paths; its 2026-11-18 Decisions entry records (1343)=a, the 09:27 cross-minor replace and (1346)=b that this ticket stays consistent with"
  261114-bug-ws-shared-daemon-restarts-without-a-swap: "ordinary connection never swaps an equal core; dev.py owns the explicit build and swap"
  261112-feat-ws-mixed-version-compat-guards: "tier vocabulary and the remaining skew-notice mechanisms; this ticket adds no notice"
  261115-feat-ws-dev-daemon-accepted-build-recovery: "dead-daemon recovery stays separate from replacing a running daemon"
  261115-feat-ws-daemon-release-auto-update: "a user-approved Electron update swap is an explicit replace under this ticket"
  261113-bug-ws-daemon-replace-stops-hosted-sessions: "session consequences of a replacement; not changed here"
  261113-refactor-ws-dashboard-drop-lan-serving: "removes the LAN-mode change, one of today's equal-core swap triggers"
---

# Daemon version arbitration: starter pre-check plus a daemon self-contest after start

## Background

Today only the starter decides whether a daemon starts, and only the starter
drains an incumbent. A started daemon never compares versions: it waits for
`dashboard.lock` and, if the lock is still held after its lock wait, exits,
whatever either side's version is (`internal/wsdashboard/daemon.go:351-365`,
`cmd/ws-mcp/dashboard.go:125-129`). The owner reports contention and clutter
from this, for example an installed Werkschaft release and a dev build
redeploying over each other. The owner wants traffic control after start too,
not only before it.

All paths below are relative to `agents-plugin-tool/` unless they start with
`agents-plugin/`, `scripts/` or `ai-docs/`.

Current start and replace paths:

- **Go `Ensure`** (used by `daemon-start`, `ws-mcp serve` and a hosted LAN
  change): reads `endpoint.json`, asks `/health`, and keeps the incumbent when
  `keepRunning` holds (`internal/wsdashboard/ensure.go:142-157,210-217`).
  Otherwise it sends the token-authenticated `POST /shutdown` and then spawns
  `ws-mcp dashboard` (`ensure.go:167-176,193-202`). With
  `SpawnBeforeShutdown` (the Windows Werkschaft strict starter) it spawns
  first and drains second, and a refused drain reports `unavailable`
  (`ensure.go:159-172`). It starts at most two daemons, so it can replace a
  concurrently started older daemon once (`ensure.go:38-41,113-114`). The
  explicit `--replace` flag swaps an equal core once and never downgrades
  (`ensure.go:54-56`, `cmd/ws-mcp/proxy.go:380`).
- **`daemon-start`** (`cmd/ws-mcp/proxy.go:362-447`): resolves settings, then
  runs `Ensure`. With `--require-independent-lifetime` (Windows only) it spawns
  strictly with `--lock-wait 8s` (`proxy.go:381-385,418-424`).
- **The daemon process** (`cmd/ws-mcp/dashboard.go:31-140`,
  `internal/wsdashboard/daemon.go:344-560`): takes `dashboard.lock` first,
  then binds the preferred port, falling back to the last port and then to an
  OS port (`daemon.go:367`, `daemon.go:574-599`). It writes `endpoint.json`
  (schema 1: port, pid, version, token, started_at;
  `internal/wsdashboard/endpoint.go:48-59`) and serves. `Close` drains hosted
  sessions within `proxyDrainBudget` (lock wait minus 1.8 s,
  `internal/wsdashboard/proxy.go:43-48,305-358`). It closes the listener and
  removes its own endpoint file before it releases the lock
  (`daemon.go:690-738`).
- **The Python launcher** (`agents-plugin/bin/ws-mcp-launcher.py`, byte-identical
  in `agents-plugin-wsflow/bin/` and `agents-plugin-pi/bin/`): `needs_start`
  allows a start only for an absent daemon or an older numeric core
  (`ws-mcp-launcher.py:1645-1652,1720-1722`). Starters serialize on
  `launcher-start.lock`, and losers only poll (`ws-mcp-launcher.py:1737-1768`).
  Since 0.50.6, a newer-minor daemon makes the launcher fail at once with an
  "update this plugin" error (`ws-mcp-launcher.py:1770-1778`).
- **Werkschaft** (`desktop-electron/daemon-controller.js`): its decision table
  replaces any older numeric core automatically and swaps an equal core only
  through the explicit menu (`daemon-controller.js:90-98`). On Windows it runs
  `daemon-start --require-independent-lifetime [--replace]`
  (`daemon-controller.js:117-138`). On macOS it spawns `ws-mcp dashboard`
  directly as a preflight successor and then sends `/shutdown` itself
  (`daemon-controller.js:139-158`). With no daemon running, macOS spawns
  `dashboard` directly (`daemon-controller.js:163-180`).
- **`scripts/dev.py rerun daemon`**: runs the staged binary's
  `daemon-start --replace` (`scripts/dev.py:196-206`).

Version comparison today:

- `CompareVersions` orders `0.50.6-dev` above `0.50.6`, because a `-dev` build
  is post-release code (`internal/wsdashboard/version.go:8-49`).
- Every replace decision ignores the suffix. Equal numeric cores are kept
  regardless of suffix, build time or content hash. This holds in
  `keepRunning` (`ensure.go:206-217`), in the launcher's `needs_start` and
  `same_core` (`ws-mcp-launcher.py:1045-1048,1645-1652`), and in Werkschaft
  (`desktop-electron/daemon-version.js:2-15`). It is also the rule in
  `ai-docs/manuals/release-compatibility.md` row 5.
- Release builds are versioned by the release script
  (`scripts/build-release-assets.sh:44`). A source build reports the version
  from `cmd/ws-mcp/main.go:16`, currently `0.50.6-dev`.

## Decisions

- **(1352)=a, owner 2026-11-18 11:23: a daemon version arbitration "double
  guard".** It replaces the hosted-launcher no-start idea (1349), which is on
  hold and superseded. Rejected: starter-only arbitration (the status quo,
  where a started daemon never contests); (1349), the hosted launcher not
  starting a daemon, on its own.
- **Layer 1, starter pre-check: kept.** The plugin launcher and the other
  starters compare versions, and a starter whose bundled daemon would not win
  does not start it. Rejected: dropping the pre-check and starting a daemon on
  every connect for Layer 2 to sort out.
- **Layer 2, daemon self-check right after start, in this order:**
  1. a daemon starts;
  2. it tries to take the singleton;
  3. if a daemon already holds it,
  4. the two contest versions;
  5. a newcomer that loses or ties exits;
  6. a newcomer that wins takes the incumbent down (drain) and takes over at
     once;
  7. a later lower-version daemon is either blocked in advance by Layer 1, or
     starts, loses the contest and exits immediately.

  The owner said "takes the port". Here that means the singleton the port
  stands for; see (1359). Rejected: the incumbent always wins, as today, where
  the newcomer waits for the lock and exits whatever the versions
  (`daemon.go:351-365`).
- **Upgrade direction (owner, 2026-11-18): always upgrade to the newest core,
  across minors too, and never downgrade.** (1343)=a at 09:15; replace across
  minors at 09:27; never downgrade. Rejected: same-minor-only replacement, the
  cross-minor refusal that 96253c58e removed.
- **(1346)=b, owner 09:50: the launcher fails fast with an "update the
  plugin" error against a newer-minor daemon.** This landed in 0.50.6
  (9cc550402) and stays unchanged. Rejected: the 75 s per-call wait.
- **Target: the next patch, 0.50.7.** The change must be T0-T2. A T3 needs the
  owner's explicit approval first. The settled tier is (1365), T1.

Settled from the Open Decision Queue (owner 2026-11-18 12:40; (1356)=a and
(1357)=a, (1358)-(1369) accepted as proposed):

- **(1356)=a: starter-authorized swaps win at an equal core and over an older
  one, and never downgrade a newer incumbent.** The authorized swaps are
  `dev.py rerun daemon` (`daemon-start --replace`, `scripts/dev.py:206`),
  Werkschaft's menu replace (`daemon-controller.js:98,124,143-150`) and an
  equal-core LAN-mode change (`ensure.go:213-214`,
  `cmd/ws-mcp/proxy.go:184-226`). The LAN change counts until
  261113-refactor-ws-dashboard-drop-lan-serving removes it. When the incumbent
  is newer, the starter reports the refusal. This keeps today's `--replace`
  contract (`ensure.go:54-56`). Rejected: (b), an explicit replace winning
  regardless of version, downgrade included, because it conflicts with
  never-downgrade.
- **(1357)=a: at the same numeric core, a `-dev` newcomer ties with a release
  incumbent and exits.** Dev builds swap in only through `dev.py rerun daemon`.
  This matches every starter's current rule (`ensure.go:206-217`,
  `ws-mcp-launcher.py:1645-1652`, `daemon-version.js:2`) and
  release-compatibility row 5, and ends the redeploy ping-pong even though
  `CompareVersions` ranks `-dev` higher (`version.go:11-20`). Rejected: (b)
  `-dev` wins, which would let any dev-launcher start take over the installed
  release daemon; (c) the release wins.
- **(1358): "equal" means an equal numeric core, ignoring suffix, build time
  and content hash.** Rejected: equal content hash. The core rule matches
  `keepRunning`, `needs_start`, `daemon-version.js` and release-compatibility
  row 5, so Layer 1 and Layer 2 never disagree. `CompareVersions` is a total
  order (`version.go:22-23`), so replacement cannot flap.
- **(1359): the contest is keyed on `dashboard.lock` plus `endpoint.json`.**
  Rejected: the TCP port. A daemon takes the lock before it binds, and a failed
  bind falls back to another port (`daemon.go:355-371,574-599`), so a failed
  bind does not prove another daemon is running. The newcomer reads the
  holder's endpoint and `/health` version, as `Ensure` already does
  (`ensure.go:143-145`, `client.go:21-49`).
  - Port continuity: the winner binds only after it holds the lock
    (`daemon.go:357-367`). The incumbent's `Close` closes its listener
    (`daemon.go:705-713,721`) and its web-card listeners (`daemon.go:724`)
    before it releases the lock (`daemon.go:733-734`). So the old daemon never
    holds the port at the winner's bind, and the winner tries the same port
    first. Layer 2 keeps lock-then-bind; binding before the lock is rejected,
    because it would meet the incumbent's open listener and fall back to
    another port.
- **(1360): the newcomer daemon is the only drainer, using the existing token
  `POST /shutdown`.** `Ensure` stops sending its own `/shutdown` for a child it
  spawns, and keeps its Layer 1 pre-check, its spawn and its successor wait.
  Rejected: two drainers. The second request finds a closed listener and is
  reported as "drain uncertain" (`ensure.go:167-172`,
  `daemon-controller.js:150-157`). This also gives every platform the
  spawn-then-drain order that the Windows strict path needs
  (`ensure.go:57,159-166`).
- **(1361): an authorized tie win travels as an environment variable naming
  the observed incumbent (pid plus started_at).** Rejected: a new `dashboard`
  flag. `dashboard` parses with `flag.ExitOnError`
  (`cmd/ws-mcp/dashboard.go:32-37`), and a Werkschaft development selection
  checks only major.minor (`desktop-electron/daemon-selection.js:110-111`), so
  an older same-minor binary would die on an unknown flag. It ignores an
  unknown variable and waits for the lock as today. Binding the authority to
  one observed incumbent stops it from draining a different, newer winner. The
  macOS Go spawn passes `os.Environ()` into the launchd plist
  (`internal/wsdashboard/spawn_launchd_darwin.go:34`), so a per-spawn variable
  must reach that path too. The variable carries no secret.
- **(1362): after the drain is accepted, the newcomer waits for the lock within
  its existing `--lock-wait`** (5 s by default, 8 s for Werkschaft). Rejected:
  a new budget. The incumbent's `Close` fits inside it (`proxyDrainBudget`,
  `proxy.go:43-48`; `shutdownDeadline` 3 s, `daemon.go:37`), and `Ensure`'s
  successor wait is already the lock wait plus 5 s (`ensure.go:34-36`). If
  another newcomer takes the lock first, the newcomer contests that new holder
  once; the holder is at least as new, so the newcomer exits.
- **(1363): a losing or tying newcomer exits at once with exit code 3 and one
  stderr line.** It never retries and never writes `dashboard.log`. Rejected:
  today's full lock wait. No starter reads a child's exit code. `Ensure` never
  respawns against a winner (`ensure.go:38-41,150-157`), and the launcher's
  `needs_start` sees a winner that is at least as new, so nothing loops.
- **(1364): the drained incumbent's sessions behave as on any replacement
  today.** Idle streams close and launchers reconnect to the winner. Calls in
  flight finish within the drain budget or keep their stream until they end.
  Already-sent calls are not replayed. Previous-minor launchers get the 0.50.6
  fail-fast (`proxy.go:305-358`, `ws-mcp-launcher.py:1770-1778`,
  release-compatibility rows 3 and 4a).
- **(1365): tier T1 overall, a patch.** It is T0 on every production path. It
  is T1 only where old Werkschaft JS drives a newer binary on macOS through a
  development selection. Rejected: any T3 boundary. Directional evidence:
  - New newcomer against an old incumbent (any 0.50.x): it reads schema-1
    `endpoint.json` (readers ignore unknown fields, `endpoint.go:48-51`), then
    `/health` and the bearer `/shutdown`, both present at v0.50.0 and v0.50.6
    (`v0.50.6:internal/wsdashboard/daemon.go:767-768,874`,
    `v0.50.0:.../daemon.go:807`). Nothing is written or changes shape: T0.
  - Old newcomer against a new incumbent: the old daemon has no contest. It
    waits 5 s and exits 3 (`v0.50.6:.../daemon.go:33,364`,
    `v0.50.6:cmd/ws-mcp/dashboard.go:125-128`). Old starters never spawn
    against a newer core. Behavior is unchanged: T0.
  - New Werkschaft JS against an older same-minor binary: the binary ignores
    the (1361) variable, and (1367) keeps the JS drain: T0.
  - Old Werkschaft JS against a newer binary on macOS, development selection
    only:
    - On an upgrade, the old JS's own `/shutdown` fails after the preflight
      daemon has drained, which shows the "outcome uncertain" notice or error
      (`daemon-controller.js:150-157`).
    - On an equal-core menu replace, the new preflight ties and exits, and no
      successor comes (`daemon-controller.js:232-234`).
    - No data is touched, and the next start recovers: T1.
  - No hello, MCP tool, store or manifest surface changes.
- **(1366): Layer 1 stays unchanged in the Python launcher and in Werkschaft's
  decision table; only Go `Ensure` changes, per (1360).** Rejected:
  re-implementing the pre-check. Every starter already pre-checks
  (`ws-mcp-launcher.py:1645-1652,1720-1722`, `ensure.go:210-217`,
  `daemon-controller.js:90-98`). No launcher mirror edit and no Pi bundle or
  payload regeneration is needed.
- **(1367): Werkschaft on macOS keeps its own `/shutdown` only when the
  selected binary's `metadata.version` predates Layer 2.** Otherwise the
  preflight daemon drains, and the JS passes the (1361) authority for the menu
  replace. Rejected: removing the JS drain outright, because a development
  selection can pick an older same-minor binary (`daemon-selection.js:110-111`).
  The Windows JS needs no change, because the binary's own `Ensure` handles it
  (`daemon-controller.js:117-124`).
- **(1368): Layer 2 applies to every daemon start.** That includes a hand-run
  `ws-mcp dashboard` used for diagnosis or live dev assets, and Werkschaft's
  direct macOS spawns. Source: owner (1352) step 1, "a daemon starts";
  `cmd/ws-mcp/dashboard.go:26-31`; `daemon-controller.js:143-145,170-172`.
- **(1369): testing uses in-process Go contest tests plus one real-process
  cross-version check on w5090.** Rejected: Windows-local runs. The contest
  needs no OS-specific code beyond the existing flock and spawn. Windows CI
  runs the Go suites.
  - The in-process tests cover:
    - an older incumbent drained;
    - a newer or equal incumbent, where the newcomer exits at once;
    - an authorized equal-core swap, and an authority naming a different
      incumbent being ignored;
    - an unanswering holder, which ends in `ErrLocked` after the wait;
    - two concurrent winners, where exactly one survives;
    - a loser that never writes `dashboard.log`;
    - a handoff that keeps the incumbent's port (see (1359)).
  - The existing `TestEnsure*` cases (`internal/wsdashboard/daemon_test.go:326-800`,
    `ensure_electron_test.go:14`) are adapted to the newcomer-drains order.
  - The cross-version check runs a v0.50.6 binary built from its tag against
    the candidate, in both directions, in an isolated `WS_CACHE_HOME`.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` if any launcher copy changes.
- Preserved behavior:
  - An unreachable or unanswering holder is never treated as dead. The
    newcomer waits for its lock wait and exits as today. Never kill by PID
    (`ensure.go:107-111`).
  - A daemon that does not hold the lock never writes `dashboard.log`. The log
    truncates on first write (`cmd/ws-mcp/dashboard.go:291-293`), so contest
    diagnostics go to stderr.
  - The shutdown token never appears in argv, an environment variable or a log.
  - The endpoint file is removed before the lock is released
    (`daemon.go:690-692`).
- No local Go tests, Go tools, ws executables, launcher or Electron runs on the
  owner PC. Go verification runs on the remote Linux host w5090. Windows
  behavior is verified in Windows CI or by an owner check.

## Open Decision Queue

Last ID used: (1369)

## Phases

### Phase 1: Daemon self-contest and Ensure as pre-check-and-spawn

Goal: Layer 2 in the Go daemon, with the newcomer as the only drainer, per
(1358)-(1363).

Scope:

- At its lock step, the starting daemon contests a holder that answers:
  - it loses or ties: it exits at once (1363);
  - it wins, or holds authority for this exact incumbent (1356, 1361): it
    drains the incumbent with the token `/shutdown`, then takes the lock within
    its lock wait (1362);
  - the holder does not answer: it waits for the lock and exits as today.
- The daemon still binds only after it holds the lock, so a winner binds after
  the incumbent's listener has closed and keeps the incumbent's port (1359).
- `Ensure` keeps its pre-check and successor wait, and passes the authority for
  explicit replace and LAN change. It stops sending its own `/shutdown` for its
  spawned child (1360).
- `daemon-start` flags and output are unchanged.
- Update `ai-docs/manuals/release-compatibility.md`: the "How Versions Meet"
  bullet, plus a scenario row with source lines for the settled tier (1365).
- Update `ai-docs/manuals/ws-mcp.md`, the replacement wording near the
  `daemon-start --replace` and `POST /shutdown` line.
- Update the AGENTS.md compatibility paragraph only if its wording becomes
  inaccurate.

Deferred: the Werkschaft JS (Phase 2). The Python launcher and its mirrors are
untouched (1366).

Verification:

- On w5090: `gofmt -l`, `go vet ./...` and `GOOS=windows go vet ./...` are
  clean, and `go test ./internal/wsdashboard ./cmd/ws-mcp` passes with the
  (1369) cases.
- A port-continuity test: an incumbent on a fixed preferred port is replaced
  through Layer 2, and the winner's endpoint names that same port. No handoff
  test asserts this today; `TestCloseReleasesThePort`
  (`internal/wsdashboard/daemon_test.go:188-206`) covers only Close followed by
  a bind.
- Also on w5090, the real-process cross-version check from (1369):
  - a v0.50.6 incumbent is drained by the candidate, which takes the same port;
  - a candidate incumbent survives a v0.50.6 newcomer, which exits after its
    lock wait;
  - an equal-core candidate newcomer exits at once;
  - `daemon-start --replace` still swaps an equal core once.
- The Windows CI Go jobs are green, including the port-continuity test.
  Windows binds without `SO_REUSEADDR` and needs its own evidence there.

### Phase 2: Werkschaft adaptation and owner check

Depends on Phase 1: the JS gate needs the first binary version that has
Layer 2.

Scope:

- Werkschaft on macOS: the version-gated drain and the authority for the menu
  replace (1367).
- The Windows JS stays unchanged, and so do the decision table (older core
  replaced, equal core attached, never downgrade) and the 0.50.6 launcher
  fail-fast.

Verification:

- `npm test` in `desktop-electron` on w5090 or in CI. The decision-table tests
  gain cases for a Layer 2 binary and a pre-Layer-2 binary on darwin.
- Owner check on Windows with installed Werkschaft and a dev build:
  - an ordinary dev start against the release daemon of the same core attaches
    and starts nothing;
  - `dev.py rerun daemon` swaps once;
  - a later Werkschaft start attaches to the dev daemon;
  - a stale older-core starter's daemon exits at once.
