# Implementation Plan: 260516-feat-ws-web-workroot-file-navigation Phase 1

## Scope

Implement Phase 1 only: an owner-authenticated daemon API that lists directory entries below an already opened `workRoot` by opaque `workRootId`. Do not add frontend explorer UI, file-open/read APIs, workbench panes, or filesystem mutation.

## Likely Files

- `ws-dashboard/crates/daemon/src/router.rs#L18-L46` — extend `AppState` with opened workRoot registry state and add the protected `GET /api/dashboard/work-roots/{work_root_id}/files` route beside the existing dashboard API routes.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L86-L110` — after a successful online `open_work_root`, register the returned `workRootId` with the daemon-private path used to open it.
- `ws-dashboard/crates/daemon/src/discovery.rs#L102-L131` — current workRoot id generation happens inside discovery; expose or mirror the minimum helper needed so registration stores the same id returned in the open response.
- `ws-dashboard/crates/daemon/src/work_root_files.rs` (new) — keep listing request/response structs, registry type, path normalization, directory reading, error mapping, and handler here rather than expanding `router.rs`.
- `ws-dashboard/crates/daemon/src/lib.rs#L1-L10` — export the new daemon module.
- `ws-dashboard/crates/daemon/tests/routes.rs#L36-L51` and `#L523-L703` — update test `AppState` constructors and add route tests near existing root-picker/open-workRoot tests.

## Public API Shape

Route:

```text
GET /api/dashboard/work-roots/{workRootId}/files?path=<workRoot-relative-location>
```

Use absent or empty `path` for the workRoot root. Treat `path` as workRoot-relative only; reject absolute paths, parent traversal, prefixes, Windows drive/root components, and empty path components that would escape the root.

Recommended success response, camelCase and dashboard-local:

```json
{
  "workRootId": "root-local-...",
  "path": "src",
  "status": "ok",
  "entries": [
    {
      "name": "main.rs",
      "path": "src/main.rs",
      "kind": "file",
      "status": "ok",
      "readable": true,
      "previewEligible": true
    },
    {
      "name": "nested",
      "path": "src/nested",
      "kind": "directory",
      "status": "ok",
      "readable": true,
      "previewEligible": false
    }
  ]
}
```

Recommended bounded error response:

```json
{ "error": "unknown workRoot" }
```

Status guidance: `200 OK` for successful directory listings, `401 Unauthorized` from existing auth middleware, `404 Not Found` for unknown `workRootId` or missing relative target, `400 Bad Request` for traversal/absolute path/non-directory target, and `403 Forbidden` or `400 Bad Request` for unreadable/inaccessible targets. Do not include daemon-private absolute host paths in responses or error text.

## Implementation Steps

1. Add an in-memory `OpenedWorkRoots` registry, likely `Arc<RwLock<HashMap<WorkRootId, PathBuf>>>`, with `register(work_root_id, root_path)` and `resolve(work_root_id) -> Option<PathBuf>` helpers.
2. Add the registry to `AppState` and all `AppState` test/server construction paths. Keep `OwnerAuthState` and `ServeConfig` behavior unchanged.
3. Refactor workRoot id derivation in `discovery.rs` just enough to avoid duplicating the hash/id format when registering the open result.
4. Change `open_work_root` to accept `State<AppState>` (or just registry state if split) and register only after the provider returns exactly one online workRoot in the response.
5. Implement `work_root_files` listing:
   - parse `{workRootId}` and optional `path` query;
   - resolve the id through the registry;
   - validate the relative path by components before joining;
   - join below the registered root and verify the final target remains under that root without following an escape outside it;
   - read only one directory level;
   - sort entries deterministically, preferably directories then files, then name;
   - classify `file` vs `directory`; surface other kinds as unavailable/unsupported if encountered;
   - compute cheap `readable`/`previewEligible` from file type/metadata only, without reading file contents.
6. Register the route inside the existing protected router so auth rejects before handler behavior.

## Route Tests

Add tests to `ws-dashboard/crates/daemon/tests/routes.rs`:

- `work_root_file_listing_route_is_owner_authenticated`: unauthenticated `GET /api/dashboard/work-roots/root-local-test/files` returns `401`.
- `work_root_file_listing_succeeds_after_opening_work_root`: create a temp fixture with a subdirectory and file, pair, POST `/api/dashboard/work-roots/open`, extract `workspaces[0].workRoots[0].id`, then GET the listing and assert `workRootId`, root `path`, sorted entries, `kind`, relative `path`, `readable`, and `previewEligible` fields.
- `work_root_file_listing_rejects_traversal`: after opening a temp root with an outside sibling file, request `?path=..` or `?path=../outside.txt`; assert non-OK and no outside filename/path appears in the JSON body.
- `work_root_file_listing_reports_unknown_work_root`: authenticated GET with an unregistered id returns `404` and bounded JSON error.
- `work_root_file_listing_reports_non_directory_target`: authenticated GET with `?path=file.txt` returns bounded non-OK without host path leakage.

## Verification Commands

From repository root:

```sh
cargo fmt --manifest-path ws-dashboard/Cargo.toml
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon routes --test routes
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon
cargo check --manifest-path ws-dashboard/Cargo.toml
```

If route shape or public structs move into `ws-dashboard-core`, also run:

```sh
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-core
```
