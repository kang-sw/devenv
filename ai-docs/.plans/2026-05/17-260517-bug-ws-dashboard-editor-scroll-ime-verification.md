# Survey: 260517-bug-ws-dashboard-editor-scroll-ime-verification

## Reusable Components
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L44-L75` — browser-gate fixture setup already creates `gate-long-readonly.txt` with 220 lines; use this existing fixture for scroll containment instead of adding new files.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L339-L345` — `documentScrolls(page)`: existing top-level document overflow oracle used by explorer and terminal checks; reuse for read-only pane containment.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L348-L368` — WebSocket frame/output-poll listeners: existing acceptance instrumentation records terminal socket URLs, sent input/resize frames, and HTTP output poll count.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L674-L847` — live terminal browser-gate step already drives the daemon-served xterm/WebSocket path and contains the new `ctrl-u`, `ctrl-w`, and composition guard skeleton assertions.
- `ws-dashboard/frontend/src/terminalCommandPlan.ts#L53-L101` — `terminalCommandPlanForPlatform`: portable command helper for shell-visible assertions; avoid inline POSIX-only commands for new terminal checks.
- `ws-dashboard/frontend/src/terminals.ts#L26-L42` — terminal WebSocket message contracts: `input` frames carry raw data and `resize` frames carry PTY size; useful when checking browser frame evidence.
- `ws-dashboard/frontend/e2e/terminalPortabilityEvidence.ts#L4-L38` — ignored machine-readable terminal evidence shape; extend only if manual IME evidence needs structured recording beyond `evidence.txt`.
- `ws-dashboard/frontend/e2e/daemonHarness.ts#L190-L220` — acceptance harness supports spawned/external daemon modes and env overrides; use it for any local or fixed-endpoint verification rather than a Vite dev server.

## Existing Patterns
- Read-only pane rendering is localized: see `ws-dashboard/frontend/src/App.tsx#L2535-L2580` — `ReadOnlyTextPane` renders a header plus `.readonly-text-content` `<pre>`; scroll fixes should be CSS/layout-local unless stale pane data is involved.
- Editor surface CSS already strips outer pane chrome: see `ws-dashboard/frontend/src/styles.css#L1264-L1292` — existing selectors make editor panes flex children, hide redundant detail text, and set `.readonly-text-content` as the scroll candidate.
- Terminal input should prefer xterm `onData`: see `ws-dashboard/frontend/src/App.tsx#L2096-L2108` — focused emulator data sends raw bytes over the open WebSocket before falling back to HTTP input.
- Fallback keyboard handling is deliberately narrower: see `ws-dashboard/frontend/src/App.tsx#L2125-L2175` — it only runs for the active pane when focus is outside xterm, skips composition, and currently maps only `ctrl-c`, `ctrl-l`, and `ctrl-a`.
- Focus ownership is pane-scoped: see `ws-dashboard/frontend/src/App.tsx#L1048-L1108` and `ws-dashboard/frontend/src/App.tsx#L1270-L1296` — `focusedTerminalPaneId` gates fallback input and is set once for newly opened terminals.
- Browser acceptance evidence notes are accumulated into ignored artifacts: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L34-L40` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L840-L847` — add concise IME/manual status here if automation cannot cover real Korean IME commits.

## Relevant Interfaces
- `ws-dashboard/frontend/src/App.tsx#L1988-L2011` — `TerminalPaneActions`: terminal body can send data, report focus, update WebSocket state, and query active-pane status; fallback changes should stay inside this interface.
- `ws-dashboard/frontend/src/App.tsx#L2040-L2286` — `TerminalPaneBody`: xterm construction, composition state, fallback keydown mapping, fit/resize forwarding, and cleanup live in one effect.
- `ws-dashboard/frontend/src/App.tsx#L2288-L2338` — terminal WebSocket effect: opened socket feeds output directly to xterm and forwards status to pane state; input frame evidence depends on this socket remaining open.
- `ws-dashboard/frontend/src/styles.css#L960-L1020` — base read-only pane/content styles: `.readonly-text-pane` is flex-column and `.readonly-text-content` has `overflow: auto`; likely missing piece is bounded height/flex shrink in the full Dockview chain.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L592-L625` — binding Phase 1 assertion: content must have `scrollHeight > clientHeight`, wheel must increase `scrollTop`, and document scroll must remain false.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L766-L838` — binding Phase 2 assertion: terminal must show `ctrl-u`/`ctrl-w` shell effects and record `\u0015`/`\u0017` input frames without forwarding synthetic composition-in-progress keys.
- `ws-dashboard/frontend/package.json#L6-L14` — verification scripts: `test:terminals`, `build`, and `test:browser` are the requested frontend commands.

## Likely Fix Areas
- `ws-dashboard/frontend/src/styles.css#L1264-L1292` — read-only containment likely needs stricter flex bounds (`height: 100%`/`min-height: 0`/`overflow: hidden` on `.readonly-text-pane` or `.workbench-pane-content`) so `.readonly-text-content` becomes the only vertical scroller.
- `ws-dashboard/frontend/src/styles.css#L960-L1020` — consider `flex: 1 1 0` or equivalent on `.readonly-text-content` and `overflow: hidden` on `.readonly-text-pane`; avoid adding an editor library.
- `ws-dashboard/frontend/src/App.tsx#L2150-L2155` — if `ctrl-u`/`ctrl-w` fail only when xterm focus is lost, add fallback mappings for `u -> \x15` and `w -> \x17`; if they fail while `.xterm-helper-textarea` is focused, investigate why xterm `onData` is not receiving/forwarding them instead of duplicating xterm behavior.
- `ws-dashboard/frontend/src/App.tsx#L2120-L2134` — synthetic composition guard currently listens on `container`; if skeleton fails, composition events dispatched on `.xterm-helper-textarea` may need to bubble to the container or be listened for on the helper/terminal textarea path.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L771-L788` — the current `ctrl-w` assertion types a full command before deleting one word, then types only `CTRL-W-OK`; if shell behavior differs by profile, a small command-plan helper may be safer than changing production input code.

## Tests and Verification Commands
- `cd ws-dashboard/frontend && npm run test:terminals` — checks TypeScript terminal helpers and daemon harness tests.
- `cd ws-dashboard/frontend && npm run build` — required frontend typecheck/Vite build.
- `cd ws-dashboard/frontend && npm run test:browser` — daemon-served production browser acceptance gate; required evidence source for both phases.
- Optional targeted debug: `cd ws-dashboard/frontend && npx playwright test e2e/dashboard-acceptance.spec.ts --grep "dashboard workRoot UI browser acceptance" --headed` when inspecting scroll/IME focus behavior interactively.
- Evidence locations: `ws-dashboard/frontend/e2e/.artifacts/evidence.txt` and `ws-dashboard/frontend/e2e/.artifacts/terminal-portability-evidence.json`; record whether Korean IME commit is automated or manual.

## Constraints and Risks
- Do not mask failures by switching to Vite dev server or fixture-only tests; the contract requires the daemon-served production frontend (`ai-docs/spec/ws-web-dashboard/index.md#L232-L263`).
- Do not treat ASCII typing or synthetic keypresses as real Korean IME commit evidence; synthetic composition only proves the fallback guard (`ai-docs/spec/ws-web-dashboard/index.md#L508-L509`).
- Keep terminal commands platform-aware through `terminalCommandPlanForPlatform`; external daemon gates require explicit shell profile/platform env (`ai-docs/spec/ws-web-dashboard/index.md#L523-L532`).
- Avoid broad CSS changes around terminal fit containers; xterm row measurement and bottom-row visibility are sensitive to extra chrome/padding (`ai-docs/mental-model/ws-web-dashboard.md#L71-L72`).
- Preserve Dockview as workbench owner and tab polish behavior; editor/terminal fixes should not reintroduce custom pane headers or duplicate tab shells.
- Separate native-Windows `Ctrl-C` investigation remains out of scope; do not claim native-Windows control-key coverage from a local POSIX browser pass.

## Opinion
- The likely implementation is small: one CSS containment adjustment for read-only panes plus either a narrow fallback mapping for `ctrl-u`/`ctrl-w` or an xterm focus-path fix if `onData` is not firing. The main risk is over-correcting terminal fallback input and accidentally duplicating xterm's focused helper textarea path, which would regress IME composition.
- The skeleton test is already the best implementation map. Run `test:browser` early after any CSS/input tweak because the failures are likely layout/focus-sensitive and not caught by pure TypeScript tests.
