# Dogfood Verification: 260516-bug-ws-web-dashboard-ui-acceptance-recovery

Date: 2026-05-16

Scope: browser-level acceptance of the dashboard workRoot UI recovery
(Phases 1-4). This artifact records the daemon-served browser flow after owner
pairing. Final frontend design verification and bounded tweaks are deferred to
the separate Phase 5 Sonnet pass before normal review.

## Browser Acceptance Gate

A persistent, runnable browser gate now exists and was executed:

```sh
cd ws-dashboard/frontend
npm run test:browser
```

`test:browser` builds the production frontend (`frontend/dist`), builds the
daemon binary, and runs Playwright (`@xterm`-backed frontend + Chromium) against
the **daemon-served production frontend**, not a Vite dev server.

- Gate config: `ws-dashboard/frontend/playwright.config.ts`.
- Daemon harness: `ws-dashboard/frontend/e2e/daemonHarness.ts` boots
  `target/debug/ws-dashboard serve --static-dir frontend/dist` on an ephemeral
  port and scrapes the one-time owner pairing URL from startup output.
- Gate spec: `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`.

Result: **1 passed** (`dashboard workRoot UI browser acceptance`).

## Production-Served Daemon Smoke

The gate launches the daemon equivalent to:

```sh
cd ws-dashboard
target/debug/ws-dashboard serve --static-dir frontend/dist
```

Daemon reported `owner pairing URL: http://127.0.0.1:<ephemeral-port>/pair?token=...`.
The gate paired through that one-time URL (owner cookie installed, redirect off
`/pair`) and drove the production browser frontend for all checks below.

## Browser Steps And Viewports

- Browser automation: Playwright + Chromium (headless).
- Desktop viewport: 1440x900.
- Narrow viewport: 480x900.
- Test workRoot: a temporary directory (`ws-dash-gate-*`) seeded with
  `gate-readme.txt`, `gate-subdir/`, and `gate-subdir/nested.txt`, opened
  through the raw path-input opener.
- Terminal commands used for output/color/control verification:
  - `printf 'GATEOUT-%s\n' 12345` — plain PTY output round trip.
  - `printf '\033[32mGATE-GREEN\033[0m\n'` — ANSI SGR color sequence.
  - `printf 'SECOND-%s\n' MARKER` — second-session isolation marker.

## Generated Evidence

Screenshots and traces are written to the gitignored
`ws-dashboard/frontend/e2e/.artifacts/` and `ws-dashboard/frontend/test-results/`
directories and are not committed (per ticket convention):

- `e2e/.artifacts/evidence.txt` — per-step pass notes.
- `e2e/.artifacts/desktop-workbench.png` — full desktop workbench after the flow.
- `e2e/.artifacts/narrow-workbench.png` — 480px narrow layout.
- `e2e/.artifacts/terminal-emulator.png` — terminal emulator with ANSI output.
- `e2e/.artifacts/file-explorer.png` — expanded conventional file explorer.
- Playwright traces under `test-results/` (retained on failure only).

Regenerate with `npm run test:browser`.

## Reported Failure Checks

| # | Reported failure | Result | Evidence |
|---|------------------|--------|----------|
| 1 | Terminal tabs were not selectable | PASS | Two terminals created; clicking each tab focuses only that session. |
| 2 | Mock/placeholder terminal shown after opening a real workRoot | PASS | After opening the workRoot and before `New terminal`, zero terminal tabs/surfaces exist; the static placeholder pane is removed. |
| 3 | Terminal not a real emulator (raw text, line input, no pane fill) | PASS | PTY output streams into the xterm emulator; ANSI green renders as `xterm-fg-2`, not raw `\033[` text; keyboard input flows via the emulator; surface measures 656px tall and fills the pane. |
| 4 | File explorer not conventional | PASS | Directory rows expand on whole-row click with disclosure triangle + trailing slash; previewable file rows open a read-only pane; refresh keeps entries. |

Additional acceptance checks exercised by the gate:

- Open a real workRoot through the raw path opener — PASS.
- File explorer expansion and explicit refresh — PASS.
- Open a previewable read-only file — PASS (content + read-only badge shown).
- Terminal input/output round trip — PASS.
- ANSI color rendering — PASS.
- Terminal pane fill — PASS (656px surface height at desktop viewport).
- Terminal tab selection with per-session input/output isolation — PASS.
- Close-as-terminate — PASS (tab removed; surviving session preserved).
- Reload reconstruction without mock surfaces — PASS (daemon-owned terminal
  reconstructs as a selectable tab; no mock workspace/terminal).
- Desktop and narrow (480px) layouts inspected — PASS (both recorded).

## Preserved Constraints

- Daemon-owned terminal lifecycle unchanged: create/list/output/input/resize/
  close remain daemon APIs; explicit close still terminates the session.
- Terminal resize forwarding stays bounded: the emulator debounces `fit()`
  output (250ms) before calling the daemon resize route, so logical PTY
  columns/rows are not rewritten on every visual drag frame.
- Owner pairing/auth flow unchanged: the gate uses the real one-time pairing
  URL and token-free paired navigation.
- Opaque command ids (`terminal.create`, `terminal.input`, `terminal.close`,
  `fileExplorer.refresh`, `fileExplorer.toggleDirectory`,
  `fileExplorer.selectEntry`, `fileExplorer.openFile`) preserved.
- Host paths are not used as browser route or resource identity; resource ids
  stay opaque (`root-local-*`).

## Notes

- The shell prompt rendered inside the PTY may display the shell's own working
  directory; that is terminal content from a real shell session, not a daemon
  resource-identity leak.
- Phase 5 (Sonnet frontend design verification and bounded tweak pass) is
  intentionally left to the lead and is not executed in this artifact.
