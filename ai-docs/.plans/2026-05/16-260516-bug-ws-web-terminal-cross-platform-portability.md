# Implementation Plan: 260516-bug-ws-web-terminal-cross-platform-portability

## Scope

Implement the full ready-ticket slice from Phase 1 through Phase 5:

1. Deterministic daemon endpoint harness for spawned and external daemon modes.
2. Explicit/testable terminal shell selection in the Rust daemon.
3. Platform-aware terminal command helpers for backend route tests and Playwright acceptance.
4. Browser harness hardening for cross-platform startup/shutdown, diagnostics, and fixed endpoints.
5. Durable local and native-Windows portability evidence, with explicit gap recording when Windows is unavailable.

Preserve existing terminal product behavior: daemon-owned PTY sessions, owner auth, opaque terminal ids, WebSocket primary transport, HTTP fallback/backfill, bounded resize, close-as-terminate, and the current visible terminal UI.

## Current Insertion Points

- `ws-dashboard/frontend/e2e/daemonHarness.ts#L9-L36` — skeleton public harness types already name spawned vs external modes and the handle shape.
- `ws-dashboard/frontend/e2e/daemonHarness.ts#L59-L91` — environment parsing currently covers the new variables, but needs stricter validation/diagnostics and tests through the browser gate path.
- `ws-dashboard/frontend/e2e/daemonHarness.ts#L139-L172` — external mode waits on `/healthz` and returns a no-op stop handle; this is the attach/readiness insertion point.
- `ws-dashboard/frontend/e2e/daemonHarness.ts#L175-L236` — spawned mode builds `ws-dashboard serve --static-dir ...` and scrapes the pairing URL; extend here for command capture, fixed endpoint diagnostics, readiness, early-exit detail, and Windows binary behavior.
- `ws-dashboard/frontend/e2e/terminalPortabilityEvidence.ts#L4-L34` — skeleton evidence schema; extend only if implementation needs fields for commands/fixtures and OS-scoped limitations.
- `ws-dashboard/frontend/src/terminalCommandPlan.ts#L1-L13` — command intent API for browser tests.
- `ws-dashboard/frontend/src/terminalCommandPlan.ts#L45-L88` — current plan maps Unix, `cmd.exe`, and PowerShell but still leaves browser acceptance hardcoded to POSIX strings.
- `ws-dashboard/frontend/src/terminalCommandPlan.test.ts#L15-L46` — skeleton tests for the helper profiles; expand to executable coverage for quoting, unsupported/guarded behaviors, and all acceptance intents.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L21-L62` — evidence collection/writing currently emits text notes; replace or supplement with structured portability evidence while keeping `.artifacts/` ignored.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L217-L363` and `#L387-L420` — shared acceptance path currently embeds POSIX commands (`printf`, `sleep`, `clear`, `seq`, `sed`, `stty`, `awk`, shell arithmetic) and must call helper intent commands or explicit guarded paths.
- `ws-dashboard/crates/daemon/src/terminal.rs#L33-L78` — skeleton shell-selection types/function.
- `ws-dashboard/crates/daemon/src/terminal.rs#L384-L406` — PTY spawn uses `CommandBuilder::new(default_shell())` and `command.cwd(root_path)`; keep behavior but route diagnostics through selected shell information where possible.
- `ws-dashboard/crates/daemon/src/terminal.rs#L737-L746` — compile-time `default_shell()` bridge to `select_terminal_shell`.
- `ws-dashboard/crates/daemon/src/terminal.rs#L748-L788` — current shell-selection unit tests; expand around empty/invalid env and fallback semantics.
- `ws-dashboard/crates/daemon/tests/routes.rs#L42-L98` — backend command helper skeleton for Unix/cmd/PowerShell route tests.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1598-L1648` and `#L1778-L1814` — POSIX route-test command strings still need helper replacement.
- `ws-dashboard/crates/daemon/src/cli.rs#L20-L40`, `config.rs#L33-L44`, and `server.rs#L25-L49` — product serve contract already supports `--host`, `--bind-mode`, `--port`, and `--static-dir`; do not add another public command.

## Sequenced Work

### Step 1: Lock harness configuration and readiness behavior

1. Add focused unit coverage for `daemonHarness.ts` if practical, or a route-test-style TypeScript script compiled outside `src` if `tsconfig.route-tests.json` cannot include `e2e` without widening rootDir.
2. Finish `parseDaemonHarnessConfig` validation:
   - `WS_DASHBOARD_DAEMON_MODE=spawn|external` only.
   - spawned mode accepts host, fixed port, bind mode, daemon binary, static dir, readiness timeout.
   - external mode requires either `WS_DASHBOARD_DAEMON_PAIRING_URL` or enough base/pairing data to pair.
   - errors name missing/invalid variable names.
3. Improve external attach readiness in `startDaemon`:
   - derive `baseUrl` from pairing URL when needed;
   - call `/healthz` or another real HTTP reachability probe before pairing;
   - distinguish unreachable endpoint/forward from missing pairing URL.
4. Improve spawned mode diagnostics:
   - capture the spawn command/args for evidence;
   - include startup buffer and exit code/signal in pairing scrape timeout/early-exit errors;
   - after scraping pairing URL, verify the base endpoint is reachable before returning.
5. Confirm `dashboardBinaryName` / `resolveDaemonBinary` behavior for `win32` and non-Windows.
6. Keep `stopDaemonProcess` conservative: graceful stop on Unix, Windows-compatible termination fallback, bounded timeout, and explicit forced-termination diagnostics if failures surface.

### Step 2: Finish Rust shell selection contract

1. Expand `terminal.rs` shell-selection tests around:
   - Unix `$SHELL` set and missing/empty fallback to `/bin/sh`;
   - Windows `%COMSPEC%` set and missing/empty fallback to `cmd.exe`;
   - source enum values (`ShellEnv`, `ComspecEnv`, `Fallback`).
2. Keep `select_terminal_shell(platform, env)` independent from compile-time cfg so tests cover both platforms on any host.
3. If adding spawn failure diagnostics, keep public API errors bounded. Authenticated route errors may name the selected candidate/platform generically but must not leak private host paths to unauthenticated callers.
4. Do not change shell preference policy unless implementation evidence forces it; the current brief says Unix configured shell or `/bin/sh`, Windows `%COMSPEC%` or `cmd.exe`.

### Step 3: Replace backend POSIX command strings

1. Expand `TerminalTestCommands` in `routes.rs` from `echo_and_exit`/`exit` to the route-test intents actually used by terminal HTTP and WebSocket tests.
2. Replace `printf ws-terminal-test\n; exit\n` at `routes.rs#L1607` with `terminal_test_commands_for_current_platform("ws-terminal-test").echo_and_exit`.
3. Replace the WebSocket input string at `routes.rs#L1780` with the same helper for `WS-SOCKET-TEST`.
4. Keep explicit profile tests for Unix/cmd/PowerShell helper output near `routes.rs#L100-L114`, so Windows-specific syntax is validated even when running tests locally on macOS/Linux.
5. Preserve the existing route assertions: created terminal JSON must stay opaque and must not expose the workRoot path; close/input/resize/output status contracts remain unchanged.

### Step 4: Expand frontend command-plan coverage

1. Extend `TerminalCommandPlan` only as needed for acceptance intents:
   - echo;
   - ANSI/green rendering where shell supports it;
   - scroll lines;
   - alternate-screen/fullscreen or an explicit guarded substitute;
   - clear-screen recovery;
   - long-running interrupt target;
   - paste/edit/history-visible command text.
2. Harden quoting helpers for markers used by tests. Add assertions that markers containing shell metacharacters are escaped for Unix, `cmd.exe`, and PowerShell.
3. Encode non-equivalent behavior explicitly. For example, if `cmd.exe` cannot reliably produce ANSI-colored output under the test PTY, return a guarded/limitation signal instead of claiming equal native-Windows ANSI evidence.
4. Keep helper output limited to deterministic built-in shell capabilities; do not add optional tools such as `btop`.

### Step 5: Convert Playwright acceptance to intent helpers

1. Import `terminalCommandPlanForPlatform` into `dashboard-acceptance.spec.ts`; choose the command profile from the target daemon platform/shell rather than blindly from the local Playwright platform when external Windows mode is active.
2. Add a small local profile resolver from environment, for example an optional `WS_DASHBOARD_TERMINAL_SHELL_PROFILE`/shell hint used only by tests/evidence.
3. Replace hardcoded commands:
   - `printf 'GATEOUT-%s\n' 12345` with `plan.echo("GATEOUT-12345")`.
   - Backspace/edit/paste command text with plan-provided echo commands that still exercise raw keyboard editing.
   - `sleep 5` with `plan.longRunningCommand()` before Ctrl-C.
   - `clear; printf ...` with `plan.clearAndEcho(...)` or guarded clear behavior.
   - ANSI `printf` with `plan.ansiGreen(...)` and a profile-specific assertion; if only text can be asserted for `cmd.exe`, record the limitation.
   - `seq | sed` with `plan.scrollLines("SCROLL-LINE-", 80)`.
   - alternate-screen `stty|awk|printf|sleep` script with `plan.alternateScreenBottomRow(...)` or an explicit POSIX-only guarded branch that does not count as native-Windows evidence.
   - second-terminal `printf` with `plan.echo("SECOND-MARKER")`.
4. Preserve existing browser behavior assertions: owner pairing, real workRoot open, no mock terminal, WebSocket connected/no output polling while connected, xterm input fidelity, resize frames, tab isolation, close-as-terminate, reload reconstruction, viewport containment, and screenshots.
5. Update evidence notes to include command profile and any guarded/limited behaviors.

### Step 6: Emit structured evidence

1. Convert the current `.artifacts/evidence.txt` emission into structured JSON plus optional readable text. Keep both under `ws-dashboard/frontend/e2e/.artifacts/` so private run data remains ignored.
2. Populate at minimum:
   - OS/platform;
   - shell profile;
   - daemon mode (`spawned` or `external`);
   - base URL and pairing URL source without committing tokens;
   - host/port/bind mode/static-dir when non-secret;
   - readiness signal/result/detail;
   - browser gate result;
   - command profile and limitations;
   - forwarding kind/endpoints when used, without private hostnames/usernames.
3. On failed browser gate, still write an evidence object in `afterAll` with failure/skipped status if the harness reached evidence initialization.
4. Add a tracked dogfood summary, likely `ai-docs/.plans/2026-05/16-260516-bug-ws-web-terminal-cross-platform-portability.dogfood.md`, summarizing local POSIX results and native-Windows result or explicit gap. Do not include private host/user/path/token details.

### Step 7: Native-Windows fixed-endpoint run, if reachable

1. Use the machine-local SSH host from ignored `ai-docs/_index.local.md`; do not copy private details into tracked files.
2. On Windows host, build frontend/dist and daemon as needed, then run the daemon in the foreground with the existing product CLI:

   ```text
   ws-dashboard.exe serve --host 127.0.0.1 --bind-mode tunnel --port 47173 --static-dir <dist>
   ```

3. Keep that remote daemon foreground/supervised; do not depend on a short-lived background SSH setup command.
4. From the local machine, create SSH local forwarding:

   ```text
   ssh -L 47173:127.0.0.1:47173 <windows-ssh-host>
   ```

5. Drive Playwright locally in external mode through the forwarded endpoint, passing either the full pairing URL scraped from the remote foreground daemon or the base URL plus any required pairing URL input.
6. If any layer fails, record the exact layer as an evidence gap: SSH unreachable, remote build failure, daemon startup/port conflict, forwarding unreachable, missing pairing URL, owner pairing failure, or browser assertions.

## Verification Commands

Run locally from repo root unless noted:

```sh
cd ws-dashboard && cargo test
cd ws-dashboard/frontend && npm run test:terminals
cd ws-dashboard/frontend && npm run test:browser
cd ws-dashboard/frontend && npm run build
git diff --check
```

Useful narrower checks during implementation:

```sh
cd ws-dashboard && cargo test -p ws-dashboard-daemon terminal_shell_selection_contract_targets
cd ws-dashboard && cargo test -p ws-dashboard-daemon terminal_test_command_profiles_have_exit_sequences --test routes
cd ws-dashboard/frontend && npx tsc -p tsconfig.route-tests.json
cd ws-dashboard/frontend && node ./node_modules/.tmp/route-tests/terminalCommandPlan.test.js
```

External forwarded endpoint smoke shape after the remote daemon is running and the SSH tunnel is open:

```sh
cd ws-dashboard/frontend
WS_DASHBOARD_DAEMON_MODE=external \
WS_DASHBOARD_DAEMON_BASE_URL=http://127.0.0.1:47173 \
WS_DASHBOARD_DAEMON_PAIRING_URL='<scrubbed pairing URL from remote daemon>' \
WS_DASHBOARD_TERMINAL_SHELL_PROFILE=cmd-exe \
npx playwright test
```

Use the actual pairing URL only in the local shell/session; never write it into tracked files.

## Risks / Watchpoints

- `daemonHarness.ts` currently lives outside the route-test `rootDir`; adding tests for it may require a separate tsconfig or keeping coverage inside Playwright without widening unrelated compile inputs.
- `/healthz` is owner-auth protected after pairing rules, but the harness currently treats `200` or `401` as readiness. Preserve that intent while making diagnostics clear enough to separate reachable-but-unpaired from unreachable.
- Browser profile detection is easy to get wrong in forwarded Windows mode because Playwright runs locally while the shell runs remotely. Make target shell profile explicit in env/evidence.
- Some terminal behaviors are shell-specific. Do not force fake equivalence for ANSI color or alternate screen if `cmd.exe`/PowerShell does not provide the same observable behavior under `portable_pty`; guard and record limitations.
- Playwright key sequences differ by platform and shell. Keep assertions tied to shell-visible output so the gate still proves byte-stream behavior.
- Fixed port `47173` can conflict locally or remotely. Error messages should identify port conflict separately from owner pairing/browser timeouts.
- Stopping a spawned daemon on Windows may not honor POSIX signals. Keep shutdown bounded and avoid leaving orphaned child processes.
- Evidence artifacts may include pairing URLs, hostnames, usernames, absolute paths, or screenshots. Keep machine-readable artifacts ignored and scrub the tracked dogfood summary.
- Do not weaken owner auth, Host/Origin checks, bind-mode guardrails, terminal route auth, or tunnel-mode loopback assumptions while making the harness easier to attach remotely.

## Files Read

- `ai-docs/_index.md`
- `ai-docs/_index.local.md`
- `ai-docs/.plans/2026-05/16-260516-bug-ws-web-terminal-cross-platform-portability.brief.md`
- `ai-docs/tickets/ready/260516-bug-ws-web-terminal-cross-platform-portability.md`
- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ai-docs/.plans/2026-05/16-260516-bug-ws-web-terminal-websocket-transport.md`
- `ai-docs/.plans/2026-05/16-260516-feat-ws-web-terminal-session-substrate.md`
- `ws-dashboard/frontend/e2e/daemonHarness.ts`
- `ws-dashboard/frontend/e2e/terminalPortabilityEvidence.ts`
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`
- `ws-dashboard/frontend/src/terminalCommandPlan.ts`
- `ws-dashboard/frontend/src/terminalCommandPlan.test.ts`
- `ws-dashboard/frontend/package.json`
- `ws-dashboard/frontend/tsconfig.route-tests.json`
- `ws-dashboard/frontend/playwright.config.ts`
- `ws-dashboard/crates/daemon/src/terminal.rs`
- `ws-dashboard/crates/daemon/tests/routes.rs`
- `ws-dashboard/crates/daemon/src/cli.rs`
- `ws-dashboard/crates/daemon/src/config.rs`
- `ws-dashboard/crates/daemon/src/server.rs`
