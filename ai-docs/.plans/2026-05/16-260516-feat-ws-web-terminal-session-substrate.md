# Implementation Plan: 260516-feat-ws-web-terminal-session-substrate

## Scope

Implement all terminal substrate phases:

1. Daemon terminal registry and PTY spawn.
2. Terminal I/O transport.
3. Frontend terminal pane.
4. Explicit close terminates the daemon session.

Do not add agent presets, named-agent controls, detached restore UX, full multiplexing, terminal search, rich scrollback persistence, foreground-process confirmation, or custom keybinding UI.

## Dependency / PTY Guidance

- Prefer `portable-pty` for Rust PTY spawning: it provides a cross-platform PTY API and examples for spawn/read/write/resize. Add it narrowly to `ws-dashboard/Cargo.toml` workspace deps and `ws-dashboard/crates/daemon/Cargo.toml`.
- Expect `portable-pty` master reader/writer to be blocking; run PTY reader/writer work on dedicated threads or `tokio::task::spawn_blocking`, and bridge to daemon state with bounded channels/buffers.
- If `portable-pty` does not fit after a spike, fallback to Unix-only `pty-process`/`tokio-pty-process` only with an explicit note because cross-platform behavior is preferred for this dashboard.
- Frontend: prefer `@xterm/xterm` if adding one npm dependency is acceptable. Official xterm.js docs recommend package-manager install via `npm install @xterm/xterm`. If dependency install is blocked, use a basic monospace `<pre>` output + input box substrate for this ticket and record the blocker.

## Phase 1: Daemon Terminal Registry And PTY Spawn

### Likely Files

- `ws-dashboard/crates/daemon/src/terminal.rs` (new) — terminal registry, session model, PTY spawn, reader task/thread, input/resize/close methods, route handlers.
- `ws-dashboard/crates/daemon/src/router.rs#L19-L47` — add `TerminalRegistry` to `AppState` and protected terminal route family.
- `ws-dashboard/crates/daemon/src/server.rs` — initialize `TerminalRegistry::default()` alongside `OpenedWorkRoots`.
- `ws-dashboard/crates/daemon/src/lib.rs` — export new `terminal` module.
- `ws-dashboard/crates/daemon/tests/routes.rs#L36-L52` — update `AppState` test constructors and add terminal route tests near workRoot route tests.
- `ws-dashboard/Cargo.toml` and `ws-dashboard/crates/daemon/Cargo.toml` — add PTY/dependency entries.

### Daemon State Shape

```rust
TerminalRegistry {
  sessions: Arc<RwLock<HashMap<TerminalId, TerminalSession>>>,
}

TerminalSession {
  id: TerminalId,              // opaque, random/browser-safe, not pid
  work_root_id: WorkRootId,
  title: String,
  status: TerminalStatus,      // starting | running | exited | terminated | error
  created_at_ms: u64,
  size: TerminalSize,          // cols/rows only; no continuous visual churn
  output: VecDeque<TerminalOutputChunk>, // bounded ring buffer
  next_sequence: u64,
  input_tx: Sender<Vec<u8>> or writer handle,
  child/session handle for kill/wait,
}

TerminalOutputChunk {
  sequence: u64,
  data: String,                // UTF-8 lossy is acceptable for first substrate
  stream: "pty",
}
```

Use `OpenedWorkRoots` from `work_root_files.rs#L15-L35` to resolve `workRootId` to a daemon-private working directory before spawn. Creation must fail for unknown or unopened workRoots.

### Spawn Behavior

- Shell command: use `$SHELL` on Unix when set, otherwise `/bin/sh`; on Windows prefer `pwsh` if available, else `cmd.exe`.
- Spawn cwd is the opened workRoot path. Do not expose cwd in public terminal JSON.
- Default size should match frontend policy (`80x24`) unless client provides bounded positive `columns`/`rows`.
- Bound session count and output buffer per daemon process with constants to avoid runaway memory.

## Phase 2: Terminal I/O Transport

### Route / API Shape

Keep all routes inside the existing protected router:

```text
POST   /api/dashboard/work-roots/{workRootId}/terminals
GET    /api/dashboard/work-roots/{workRootId}/terminals
GET    /api/dashboard/terminals/{terminalId}/output?after=<sequence>
POST   /api/dashboard/terminals/{terminalId}/input
POST   /api/dashboard/terminals/{terminalId}/resize
DELETE /api/dashboard/terminals/{terminalId}
```

Request/response sketches:

```json
// POST create
{ "columns": 80, "rows": 24, "title": "Terminal" }

// TerminalSessionView
{
  "terminalId": "term_opaque",
  "workRootId": "root-local-...",
  "title": "Terminal",
  "status": "running",
  "columns": 80,
  "rows": 24,
  "createdAtMs": 1778880000000
}

// GET output
{
  "terminalId": "term_opaque",
  "status": "running",
  "nextSequence": 12,
  "chunks": [{ "sequence": 10, "data": "...", "stream": "pty" }]
}

// POST input
{ "data": "ls\r" }

// POST resize
{ "columns": 100, "rows": 30 }
```

Output transport can be long-polling for the first substrate: return immediately when chunks after `after` exist, otherwise wait up to a short timeout. SSE/WebSocket is acceptable only if it stays testable and authenticated before upgrade/stream acceptance.

### Daemon Tests

Add route tests for:

- all terminal routes reject unauthenticated callers;
- create fails for unknown/unopened workRoot;
- open workRoot then create terminal returns opaque id, workRootId, status, and no pid/path;
- list returns live sessions after create and still returns them after a fresh request/router clone (daemon-owned, not component-owned);
- output route returns bounded chunks/cursor shape;
- input route accepts deterministic input and output eventually reflects a deterministic command where feasible;
- resize validates positive bounded dimensions and updates session view;
- delete/close terminates session and removes or marks it closed so list no longer shows it as live;
- closed terminal rejects further input/resize/output with bounded non-OK.

For deterministic tests, avoid prompts. Spawn a shell and send `printf ws-terminal-test\\n; exit\\n` if portable across chosen shell, or add a test-only command/spawn adapter seam so route behavior can be tested without relying on a user shell.

## Phase 3: Frontend Terminal Pane

### Likely Files

- `ws-dashboard/frontend/src/terminals.ts` (new) — API types, endpoint builders, fetch helpers, polling helper decisions, terminal pane ids/logical keys.
- `ws-dashboard/frontend/src/App.tsx#L776-L895` — add terminal session state, create/list restore, polling, and pane rendering into `WorkbenchShell`.
- `ws-dashboard/frontend/src/App.tsx#L897-L965` — add toolbar action `terminal.create` or visible workRoot-local create button.
- `ws-dashboard/frontend/src/App.tsx#L980+` / pane rendering area — render terminal pane body and close/terminate action.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L36-L50` — existing `persistentTerminal` is already `pinned`, daemon-owned, and detach-on-close; for this ticket override close flow so terminal close terminates instead of detaches, or add a terminal-specific close command before registry close behavior is used.
- `ws-dashboard/frontend/src/workbench/policy.ts#L155-L168` — reuse `defaultPtyLogicalSize` and `preservePtyLogicalSize`; resize only on explicit/bounded events, not every split drag.
- `ws-dashboard/frontend/src/styles.css` — add terminal pane styles using dark semantic tokens.
- `ws-dashboard/frontend/package.json` — add `@xterm/xterm` dependency and terminal test script if helper tests are added.

### Frontend State Shape

```ts
type TerminalSessionView = {
  terminalId: string;
  workRootId: string;
  title: string;
  status: "starting" | "running" | "exited" | "terminated" | "error" | string;
  columns: number;
  rows: number;
  createdAtMs: number;
};

type TerminalPaneState = {
  session: TerminalSessionView;
  logicalKey: string;        // persistentTerminal/<workRootId>/<terminalId>
  paneId: string;            // deterministic from terminalId
  output: string;
  nextSequence: number;
  polling: boolean;
  inputDraft: string;        // only for fallback non-xterm UI
  error: string | null;
};
```

### Frontend Behavior

- On selected workRoot load/refresh, `GET /work-roots/{workRootId}/terminals` and reconstruct visible terminal panes for live sessions. This is refresh persistence: daemon session existence survives browser refresh.
- Create button posts to create route, inserts pane state, and opens/focuses the corresponding workbench terminal pane.
- Terminal logical key should be `surfaceLogicalKey("persistentTerminal", workRootId, terminalId)`; dedupe the same session to focus existing.
- Use `decideSurfaceOpen` for placement. Persistent terminals are pinned/durable and prefer focused group then first group; this matches current registry policy.
- Poll output by `nextSequence` while pane/session is live. Append chunks to xterm instance or fallback output text.
- Input: xterm `onData` posts raw data to input route. Fallback UI sends textbox content plus `\r` on Enter/Send.
- Resize: send bounded resize on explicit xterm resize/addon event or committed size change only. Do not wire continuous split drag directly to resize.

## Phase 4: Close Semantics And Verification

### Close Semantics

- Terminal panel close must call `DELETE /api/dashboard/terminals/{terminalId}` and treat success as session termination.
- After close, remove pane state and stop polling. Do not keep hidden detached terminal session or restore-list entry.
- If delete fails, keep pane visible with an error and do not pretend termination happened.
- Reserve a future confirmation hook, but first substrate may close immediately with clear button label such as `Terminate`/`Close terminal`.
- Ensure registry cleanup kills child process/session and reaps/waits where the PTY crate requires it.

### Frontend Tests

Add `ws-dashboard/frontend/src/terminals.test.ts` if helper logic exists:

- endpoint builders encode workRootId/terminalId and query cursor;
- logical key/pane id uses `workRootId + terminalId`, not process id/path/title;
- live session list maps to terminal panes;
- close success removes pane state, close failure preserves error state;
- resize helper rejects non-positive/oversized dimensions.

Update `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts` where useful:

- persistent terminal duplicate logical key focuses existing;
- pinned terminal opens to focused group and falls back to first group;
- close decision reserves terminate command for `persistentTerminal`.

## Verification Commands

From repo root:

```sh
cargo fmt --manifest-path ws-dashboard/Cargo.toml
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon routes --test routes
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon
cargo check --manifest-path ws-dashboard/Cargo.toml
```

From `ws-dashboard/frontend`:

```sh
npm run test:routes
npm run test:work-root-files
npm run test:workbench
npm run test:terminals   # if added
npm run build
```

Manual smoke:

1. `npm run build` in `ws-dashboard/frontend`.
2. Start daemon serving `ws-dashboard/frontend/dist`.
3. Pair in browser, open a workRoot, create terminal.
4. Run `pwd`, `printf hello`, and `exit`/close checks.
5. Refresh browser and confirm live terminal sessions reappear until explicitly closed.
6. Close terminal panel and confirm it no longer appears in list and no hidden detached restore UX exists.

## Risks / Watchpoints

- PTY crates use blocking IO and process handles; isolate blocking work from Axum/Tokio request tasks.
- Cross-platform shell behavior differs. Keep tests deterministic with an adapter seam or shell commands that do not rely on prompts.
- Process cleanup is easy to get wrong: delete must terminate the child/session and stop reader threads without deadlocks.
- Output buffers need bounds; do not let long-running terminals grow memory unbounded.
- Auth must wrap stream/long-poll/upgrade routes before accepting a stream or input.
- Do not leak pids, cwd, host paths, shell command internals, or environment in public JSON.
- Browser refresh persistence is daemon-memory persistence only; do not add durable disk restore in this ticket.
- xterm integration can be time-consuming; fallback UI is acceptable if it keeps create/list/input/output/close correct and documented.
- Continuous resize churn can break TUIs; send resize only on explicit/settled events.
