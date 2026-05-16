# Implementation / Verification Plan: 260516-feat-ws-web-workroot-io-workbench-integration

## Scope

Implement or verify all integration phases:

1. Cross-surface restore model.
2. Placement and command polish.
3. End-to-end daemon-served dogfood verification.

Do not add editing/save/dirty-state behavior, new terminal emulator depth, terminal multiplexing, agent presets, named-agent controls, detached terminal restore UX, or broad IDE features. Verify first; patch only gaps.

## Likely Files

- `ws-dashboard/frontend/src/App.tsx#L201-L360` — top-level read-only file pane state/open placement and props passed into `WorkbenchShell`.
- `ws-dashboard/frontend/src/App.tsx#L791-L980` — workbench restore, terminal listing/polling, active pane requests, create/send/close terminal handlers.
- `ws-dashboard/frontend/src/App.tsx#L1151-L1467` — editor group construction, terminal/read-only pane insertion, placement helper use, pane bodies.
- `ws-dashboard/frontend/src/terminals.ts` — terminal endpoint helpers, list/merge/close state helpers, pane logical keys.
- `ws-dashboard/frontend/src/workRootFiles.ts` — file listing/read helpers and read-only pane state helpers.
- `ws-dashboard/frontend/src/workbench/policy.ts#L98-L188` — placement/focus/dedupe policy; do not bypass it with hard-coded group appends.
- `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts` — placement/focus/close policy coverage.
- `ws-dashboard/frontend/src/terminals.test.ts` and `ws-dashboard/frontend/src/workRootFiles.test.ts` — frontend helper coverage for restore/close/open state.
- `ws-dashboard/crates/daemon/src/terminal.rs#L28-L70` and `#L165-L259` — terminal registry/list/create/close behavior if frontend dogfood exposes lifecycle bugs.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1172-L1481` — daemon terminal lifecycle route tests if backend behavior needs patching.
- `ws-dashboard/frontend/src/styles.css` — only if layout polish is needed for desktop/narrow dogfood checks.

## Phase 1: Cross-Surface Restore Model

### Restore Checks

Verify the implementation has these semantics; patch only failures:

- Terminal panes restore from daemon `GET /api/dashboard/work-roots/{workRootId}/terminals` for the selected opened workRoot after browser refresh/re-entry.
- Explicitly closed terminals do not reappear from stale frontend state or in-flight output polling.
- Browser arrangement remains presentation-only: moving tabs/splits does not create/delete daemon sessions.
- Read-only file panes are keyed by `workRootId + relative path`; duplicate file opens focus existing panes.
- Read-only file panes are browser-owned. If refreshed/reloaded state is absent, that is acceptable; if arrangement tries to restore a file, it must re-read through daemon file API and show an unavailable/error state if missing, binary, oversized, or unreadable.
- File panes render only under their owning workRoot; selecting another workRoot must not show stale file panes as if they belonged to it.

### Implementation Notes

- If restoring file panes is not already implemented, keep it minimal: no disk/localStorage persistence unless existing arrangement state already supports it. The contract allows file panes to remain browser-owned.
- For terminal restore, prefer `mergeListedTerminalSessions`-style reconciliation: listed live sessions add/update panes; absent terminal sessions should eventually be removed or marked unavailable unless a close/error flow is in progress.
- Ensure output polling checks current pane existence before appending data so stale polls cannot recreate closed panes.

## Phase 2: Placement And Command Polish

### Placement Checks

Verify/polish these behaviors:

- `fileExplorer.openFile` opens read-only files through `decideSurfaceOpen`, using logical key `editor/<workRootId>/<relativePath>` or equivalent stable scoped key.
- New read-only file panes prefer the support/second split when available.
- `terminal.create` or equivalent create command opens/focuses a persistent terminal pane using `persistentTerminal/<workRootId>/<terminalId>`.
- Terminal panes use pinned/durable placement semantics: focused group if valid, otherwise first group.
- Existing logical targets focus existing panes instead of duplicating file/terminal surfaces.
- Drag/reorder/move behavior still works after dynamic file and terminal panes are present.
- Terminal close calls daemon close and removes pane only on success; it is not a detach action.

### Command Id Checks

Ensure visible mouse actions carry stable `data-command-id` values:

- `fileExplorer.refresh`
- `fileExplorer.toggleDirectory`
- `fileExplorer.selectEntry`
- `fileExplorer.openFile`
- `terminal.create`
- `terminal.input` or terminal send equivalent
- `terminal.close`
- existing resource/workbench commands remain unchanged

Command log labels can be simple, but should not imply editing or detached terminal restore.

## Phase 3: End-To-End Dogfood Verification

### Preflight Commands

From repo root:

```sh
git status --short
cargo fmt --manifest-path ws-dashboard/Cargo.toml --check
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon routes --test routes
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon
cargo check --manifest-path ws-dashboard/Cargo.toml
```

From `ws-dashboard/frontend`:

```sh
npm run test:routes
npm run test:work-root-files
npm run test:terminals
npm run test:workbench
npm run build
```

If only frontend polish changed, still run the frontend commands and at least daemon route tests for terminal/file APIs before dogfood.

### Daemon-Served Dogfood Steps

Use production-served frontend, not only Vite, unless production serving is blocked:

```sh
cd ws-dashboard
./dev.sh run
```

Then in the browser:

1. Open the printed pairing URL and confirm authenticated shell loads.
2. Open/select a workRoot through the root picker/open flow.
3. Browse the lower-left file explorer: expand/collapse at least one directory and refresh the explorer.
4. Open a previewable text file. Confirm it appears in a read-only pane, in the support split when available, with no save/dirty/edit controls.
5. Open the same file again. Confirm the existing pane is focused rather than duplicated.
6. Create a terminal for the selected workRoot. Confirm terminal pane appears/focuses and uses workRoot-scoped identity only.
7. Send a simple command such as `pwd` or `printf ws-dashboard-terminal\n`; confirm output appears.
8. Refresh the browser. Confirm daemon-owned live terminal session reappears, while no closed terminal is resurrected.
9. Open another text file while terminal is active. Confirm terminal work is not displaced when support split exists.
10. Close the terminal pane. Confirm it terminates the daemon session and no restore/detached list appears after another refresh.
11. Inspect desktop width and narrow widths around the `960px` and `560px` breakpoints. Confirm left nav identity remains visible above file explorer, text panes are readable, and terminal UI does not overflow badly.

If screenshot tooling is available, capture desktop and narrow screenshots. If not, record manual inspection notes.

## Tests To Add/Patch If Gaps Are Found

- `terminals.test.ts`: listed live sessions merge into panes; absent/closed sessions do not reappear; close failure preserves pane with error; output append skips missing panes.
- `workRootFiles.test.ts`: duplicate file pane identity by `workRootId + path`; unavailable file read maps to pane error state.
- `workbenchModel.test.ts`: dynamic file panes prefer support split; persistent terminal placement/dedupe; terminal close reserves terminate behavior.
- Daemon route tests: terminal close removes from live listing and rejects later input/output/resize; file read/listing keep bounded path-safe errors.

## Blocker Recording

If any verification step cannot run, record an explicit blocker in the implementation result/ticket Result with:

```text
#### Blocker - YYYY-MM-DD
- Step: <exact command or dogfood step>
- Expected: <what should have happened>
- Actual: <error text, browser symptom, missing tool, or environment limitation>
- Evidence: <log path, command output excerpt, screenshot path, or "not captured">
- Impact: <blocks completion | dogfood partial | follow-up only>
- Follow-up: <ticket needed or patch area>
```

Use a new `idea/` or follow-up ticket only for blockers that imply separate work outside this ticket's scope. Do not mark dogfood complete if a required user-visible step was skipped without a blocker note.

## Risks / Watchpoints

- Treating browser layout as daemon lifecycle authority; terminal existence must come from daemon list/create/close.
- Recreating terminal panes from stale output polls after explicit close.
- Rendering file panes under the wrong selected workRoot.
- Duplicating surfaces because logical keys use title, pane id, or display labels instead of scoped resource identity.
- Accidentally changing close semantics from terminate to detach.
- Adding broad persistence or restore lists that become hidden detached terminal UX.
- Passing tests while skipping daemon-served frontend verification; this ticket is the dogfood gate.
