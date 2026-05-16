# Implementation Plan: 260516-feat-ws-web-readonly-text-pane

## Scope

Implement all ticket phases:

1. Authenticated read-only file API.
2. Workbench read-only text pane surface.
3. File-open placement/dedupe from the left-nav file explorer.

Keep file identity `workRootId + relative path`. Do not add editing, save, dirty state, file mutations, terminal sessions, restore persistence, formatting, LSP, or broad IDE behavior.

## Phase 1: Authenticated Read-Only File API

### Likely Files

- `ws-dashboard/crates/daemon/src/work_root_files.rs#L15-L35` — reuse `OpenedWorkRoots` and keep daemon-private root path lookup here.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L77-L108` — add sibling handler/types for read-only file content near listing behavior.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L181-L213` — make path validation/string helpers reusable inside the module for both listing and reading.
- `ws-dashboard/crates/daemon/src/router.rs#L30-L47` — add protected read route inside the existing owner-authenticated router.
- `ws-dashboard/crates/daemon/tests/routes.rs#L709-L894` — add read-route tests near listing tests and reuse `open_work_root_for_test`.

### API Shape

Preferred route:

```text
GET /api/dashboard/work-roots/{work_root_id}/files/read?path=<relative-path>
```

Successful response:

```json
{
  "workRootId": "root-local-...",
  "path": "src/main.rs",
  "name": "main.rs",
  "status": "ok",
  "readOnly": true,
  "content": "fn main() {}\n",
  "sizeBytes": 13,
  "languageHint": "rust",
  "extension": "rs"
}
```

Bounded errors should stay JSON and path-safe:

```json
{ "error": "file is too large" }
```

Status guidance: `401` via auth middleware, `404` for unknown workRoot or missing file, `400` for traversal/absolute path/directory target/unsupported binary/oversize, `403` for permission denied. Never include absolute host paths in success or error bodies.

### API Implementation Notes

- Use the same relative-path validation as listing: reject `..`, absolute paths, backslashes/Windows-drive forms, and empty path for read.
- Canonicalize root and target, then verify target remains under root before reading.
- Metadata-check before reading: must be a regular file and below a small explicit limit. Use a named constant such as `MAX_READ_ONLY_TEXT_BYTES` (for example 512 KiB or 1 MiB) so tests can target it.
- Detect unsupported binary cheaply before returning content. Accept UTF-8 text; reject invalid UTF-8 and likely binary content (for example NUL bytes) with bounded unavailable error.
- `languageHint` can be a small extension map (`rs`, `ts`, `tsx`, `js`, `json`, `md`, `css`, `html`, `toml`, `yaml`, `yml`, `sh`, `py`) plus fallback `null`/extension.

### Route Tests

Add focused route tests for:

- unauthenticated read route rejects before handler behavior;
- open workRoot then read a UTF-8 text file succeeds and returns `workRootId`, relative `path`, `name`, `content`, `readOnly: true`, `sizeBytes`, `extension`, `languageHint`;
- traversal attempt fails and response body does not contain outside filename/content or absolute parent path;
- unknown workRoot returns `404` bounded JSON;
- missing file returns `404` bounded JSON;
- directory target returns `400` bounded JSON;
- binary/invalid UTF-8 file returns non-OK bounded JSON;
- oversized file returns non-OK bounded JSON.

## Phase 2: Workbench Text Pane Surface

### Likely Files

- `ws-dashboard/frontend/src/workRootFiles.ts#L1-L17` — extend with read-file API types, endpoint builder, fetch helper, and bounded error parsing reuse.
- `ws-dashboard/frontend/src/App.tsx#L404-L660` — enable file explorer open actions for previewable files and call an app/workbench open handler.
- `ws-dashboard/frontend/src/App.tsx#L687-L763` — store opened file pane state in `WorkbenchShell` or pass it from `App` if explorer and workbench need a shared callback.
- `ws-dashboard/frontend/src/App.tsx#L851-L955` — include opened read-only file panes in `buildWorkbenchEditorGroups` output.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L1-L57` — either reuse `editor` as the surface kind for read-only text panes or add a narrow `readonlyText` kind with `rowPolicy: "opened"`, `lifecycleOwner: "documentProvider"`, `closePolicy: "deferToProvider"`.
- `ws-dashboard/frontend/src/styles.css#L728-L795` — add dense text-pane body/header/status styles using existing dark tokens.

### Frontend Data/State Shape

Add to `workRootFiles.ts`:

```ts
type WorkRootTextFileView = {
  workRootId: string;
  path: string;
  name: string;
  status: "ok" | string;
  readOnly: true;
  content: string;
  sizeBytes: number;
  languageHint: string | null;
  extension: string | null;
};

type ReadOnlyFilePane = {
  id: string;              // deterministic from workRootId + path
  logicalKey: string;      // same identity basis, no host paths
  workRootId: string;
  path: string;
  title: string;
  status: "loading" | "loaded" | "error";
  content: string;
  error: string | null;
  readOnly: true;
  sizeBytes: number | null;
  languageHint: string | null;
  extension: string | null;
};
```

Recommended state in `WorkbenchShell` or lifted to `App`:

```ts
type ReadOnlyPaneState = {
  panes: Readonly<Record<string, ReadOnlyFilePane>>; // keyed by logicalKey
  activePaneKeyByGroup: Record<string, string>;
};
```

The pane body should be a real preview surface: title/path/status metadata, read-only badge, and `<pre><code>` content with horizontal scrolling and whitespace preservation. For error/loading states, keep the pane open and show an honest bounded message.

## Phase 3: File-Open Placement Policy

### Placement/Dedupe Approach

- Change file explorer `Open pending` button to enabled only when `entry.kind === "file" && entry.previewEligible && entry.status === "ok"`.
- Add command id `fileExplorer.openFile` and keep `fileExplorer.selectEntry` separate.
- Build logical keys from stable scoped file identity, for example `surfaceLogicalKey("editor", workRootId, relativePath)` or `surfaceLogicalKey("readonlyText", workRootId, relativePath)`. Do not use display title or host path.
- Use existing workbench placement semantics from `decideSurfaceOpen` (`ws-dashboard/frontend/src/workbench/policy.ts#L98-L123`): duplicate logical keys focus existing; opened/support surfaces prefer the second split group (`#L171-L180`).
- Map the placement decision into the visible editor-group model:
  - `focusExisting`: set active pane in the returned group and do not add another pane.
  - `openNew`: insert/update pane state for the target group; prefer `support` group in the current two-group UI.
- Preserve current tab movement behavior: any opened-file pane must participate in `applyWorkbenchPaneOrder`, `commitWorkbenchPaneMove`, and `selectWorkbenchPane` without resetting existing moved tabs.
- Closing/detaching can remain browser-arrangement-only if close UI is not currently implemented; do not add lifecycle termination semantics for read-only files.

### Workbench Tests

Update `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L469-L515` or add helper tests to cover:

- read-only file logical key dedupes same `workRootId + path` to `focusExisting`;
- different paths open distinct panes;
- opened file panes prefer group 2 when two groups exist;
- single-group fallback goes to group 1;
- logical key does not include raw host path input.

## Frontend Helper Tests

Add/extend `ws-dashboard/frontend/src/workRootFiles.test.ts` for:

- read endpoint encoding with `workRootId` and `path` query;
- relative path with spaces/slashes encodes through query params;
- error response parsing returns `{ error }` text or `HTTP <status>` fallback;
- deterministic read-only pane id/logical-key helper for same file/different file cases if implemented there.

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
npm run build
```

If frontend served-through-daemon behavior changes, also run a manual smoke after `npm run build` by serving `ws-dashboard/frontend/dist` with the daemon, pairing, opening a workRoot, expanding files, and opening the same text file twice to verify focus/dedupe.

## Risks / Watchpoints

- Path safety: sharing helpers between list/read is good, but reading must reject empty path and directories while listing accepts root directories.
- Binary/oversize behavior: read bytes only after size guard; invalid UTF-8 and NUL-heavy content must not render as text.
- Host path leaks: error strings from `std::io::Error` or frontend titles must not include absolute daemon paths.
- Placement model mismatch: current workbench groups are derived fixtures plus browser order state; adding dynamic panes must not be overwritten by each rerender or break drag/drop reconciliation.
- Duplicate identity: dedupe by `workRootId + relative path`, not basename/title, otherwise files with same name in different directories collide.
- UI honesty: no save button, dirty indicator, editable textarea, rename/delete affordance, or wording that suggests write-back editing.
- Test determinism: permission-denied tests may be platform-sensitive; keep core coverage to traversal, directory, missing, binary, oversize, and success unless adding Unix-only guards.
