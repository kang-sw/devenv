# Implementation Plan: 260516-feat-ws-web-workroot-file-navigation Phase 2

## Scope

Implement only the frontend left-nav file explorer draft for the selected `workRoot`, consuming the existing Phase 1 route:

```text
GET /api/dashboard/work-roots/{work_root_id}/files?path=<relative-path>
```

Do not add file content reads, workbench text panes, terminal surfaces, persistence/restore, or file mutation operations.

## Likely Files

- `ws-dashboard/frontend/src/App.tsx#L152-L260` — current top-level resource fetch, selected id, command log, and nav/workbench composition; add explorer state/fetch orchestration here or pass a small hook into the nav.
- `ws-dashboard/frontend/src/App.tsx#L325-L378` — `ResourceNavigation` currently owns the left nav body; split it into a resource-list upper region plus a lower `WorkRootFileExplorer` region.
- `ws-dashboard/frontend/src/App.tsx#L1201-L1246` and `#L1259-L1288` — existing resource flattening and workbench selection already identify the selected workRoot; reuse this rather than deriving identity from browser routes.
- `ws-dashboard/frontend/src/workRootFiles.ts` (new) — frontend API types, endpoint builder, response parsing, and pure reducer/helper functions for expansion state.
- `ws-dashboard/frontend/src/workRootFiles.test.ts` (new, if using current no-runner test style) — pure tests for endpoint encoding, tree flattening, expansion toggling, and workRoot switch reset/retention behavior.
- `ws-dashboard/frontend/src/styles.css#L195-L199` and `#L887-L893` — adjust left-nav layout and add dense file-explorer styles using existing semantic tokens.
- `ws-dashboard/frontend/package.json#L6-L11` — add a `test:work-root-files` script only if a new standalone TS test file is added.

## API/Data Shape

Mirror Phase 1 JSON exactly, with camelCase fields:

```ts
type WorkRootFileListView = {
  workRootId: string;
  path: string; // "" for root, otherwise workRoot-relative
  status: "ok" | string;
  entries: WorkRootFileEntryView[];
};

type WorkRootFileEntryView = {
  name: string;
  path: string;
  kind: "directory" | "file" | "other";
  status: "ok" | "unavailable" | "unsupported" | string;
  readable: boolean;
  previewEligible: boolean;
};
```

Recommended component state:

```ts
type DirectoryLoadState = {
  status: "idle" | "loading" | "loaded" | "error";
  entries: WorkRootFileEntryView[];
  error: string | null;
};

type WorkRootExplorerState = {
  workRootId: string | null;
  expandedPaths: Set<string>;      // directory paths; root can be ""
  directories: Record<string, DirectoryLoadState>; // keyed by relative path
  selectedPath: string | null;     // row highlight only, no file open behavior
};
```

State rules:

- On selected workRoot change, ensure root path `""` is expanded/loaded for the new `workRootId`.
- Cache loaded directories per selected workRoot during the component lifetime; a manual refresh should reload the current directory or root plus currently expanded directories if that stays simple.
- Use `encodeURIComponent(workRootId)` and `URLSearchParams({ path })`; never include raw host paths.
- Treat failed listing responses as bounded UI errors from `{ error }` or `HTTP <status>`.

## UI Placement

- Keep the existing server/workspace/workRoot navigation visible at the top of the left nav.
- Convert the nav body to a vertical layout: upper resource identity area, lower file explorer area. The lower explorer should have a compact header (`Files`, selected workRoot label, refresh button) and a scrollable tree body.
- Render the explorer only when a workRoot selection is available from `resolveWorkbenchSelection`; otherwise show a subordinate empty state such as `Select a workRoot`.
- Rows:
  - directory rows get an expand/collapse button and display `directory`/status metadata;
  - file rows get a disabled/stubbed open button only when useful; label it honestly (for example `Open pending`) and keep it disabled until the read-only text pane ticket exists;
  - unsupported/unavailable rows remain visible with muted status.
- Required command ids:
  - `fileExplorer.refresh`
  - `fileExplorer.toggleDirectory`
  - `fileExplorer.selectEntry`
  - `fileExplorer.openFile` for the disabled/stubbed file-open affordance
- Keep all text truncating/ellipsis-friendly in the narrow nav and use existing dark tokens (`--ws-color-*`, `--ws-space-*`, hairline borders).

## Implementation Steps

1. Add `workRootFiles.ts` with API types, `workRootFilesEndpoint(workRootId, path)`, `fetchWorkRootFiles`, and pure helpers for toggling/flattening tree rows.
2. In `App`, derive `selectedWorkRoot` from `workbenchSelection?.root` and pass it into `ResourceNavigation`.
3. Add `WorkRootFileExplorer` in `App.tsx` or a small adjacent module; keep route-independent logic in `workRootFiles.ts` for testability.
4. Load root listing on selected workRoot change; load a directory when expanding it if not already loaded; show per-directory loading/error states.
5. Add refresh behavior that reloads the selected/current directory or all expanded directories, with `data-command-id="fileExplorer.refresh"`.
6. Add CSS for `.nav-stack`, `.resource-list-region`, `.file-explorer`, `.file-explorer-row`, status/meta chips, and narrow-height scrolling without changing workbench pane behavior.
7. Add pure TypeScript tests if helpers are introduced; avoid adding a browser test framework in this slice unless already trivial.

## Tests and Build Commands

From `ws-dashboard/frontend`:

```sh
npm run test:routes
npm run test:workbench
npm run build
```

If a new pure helper test is added:

```sh
npm run test:work-root-files
```

From repo root, only if frontend consumption requires daemon changes:

```sh
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon routes --test routes
```

## Screenshot / Dev-Server Verification Suggestions

- Run `npm run dev -- --host 127.0.0.1` from `ws-dashboard/frontend` for a quick Vite render check; if API calls need live auth/API, prefer a production build served by the daemon instead.
- Production-path smoke: run `npm run build`, then start the daemon with `--static-dir ws-dashboard/frontend/dist`, open the pairing URL, open a workRoot, and verify the lower-left explorer loads the root listing.
- Capture/review screenshots at desktop width and narrow width around `960px`/`560px` breakpoints, checking that resource identity remains visible above the file explorer and rows truncate instead of overflowing.
- Manual interaction checks: expand a directory, collapse it, refresh, select an unavailable/unsupported row, and confirm disabled file-open UI does not imply editing.
