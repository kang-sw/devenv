# Brief: 260524-feat-ws-dashboard-document-viewer-editor-substrate Phase 3

## Intent

Add raw-text edit mode, optimistic save, same-document pane fan-out, and
document freshness events for dashboard document panes. A Markdown document pane
should remain one workbench attachment with local `view | edit` state: view mode
uses the existing formatted Markdown viewer, while edit mode is a raw text
editor that can save through daemon-owned workRoot-relative write routes.

## Scope Boundary

Implement only Phase 3: raw text edit mode, save fan-out, and document events.

In scope:

- document read/write route support with content hashes;
- raw-text edit mode inside existing read-only/document file panes;
- draft, dirty, save, revert, saving, saved, stale/conflict, and error states;
- optimistic concurrency through base content hash;
- same-source fan-out to all clean panes for `workRootId + path`;
- dirty pane stale marking without silent overwrite;
- invalidation of translation overlays/block identity after content hash
  changes;
- per-workRoot document freshness stream or bounded invalidation mechanism;
- focus/visibility re-read fallback for open document panes;
- browser evidence that view/edit switching does not create duplicate tabs or
  break workbench placement.

Deferred:

- collaborative editing, merge UI, overwrite workflows, rename/delete/move
  operations, create-file UI, provider-specific rich editors, broad workspace
  filesystem watching, and persisted translation preferences.

## Caller-Visible Contract

Editable workRoot text files expose an in-pane `view | edit` mode control.
Switching to edit mode does not open a second tab. Edit mode displays raw text,
tracks unsaved draft changes, and offers Save/Revert controls. Save succeeds
only when the daemon sees the caller's base content hash as current. A content
hash mismatch returns a conflict/stale state instead of overwriting another
writer.

When one pane saves a document, clean panes for the same `workRootId + path`
update to the new content and content hash. Dirty panes for that source remain
dirty and are marked stale or conflicted so the user does not lose edits.
Translation overlays and derived block ids become stale after the source
content hash changes.

Document events are scoped by workRoot. They may be watcher-backed or
invalidation-backed, but correctness must not depend only on watcher delivery:
focus/visibility re-read plus content-hash checks remain the fallback.

## Contract Instructions

Backend files/modules:

- Extend `ws-dashboard/crates/daemon/src/work_root_files.rs` or add a focused
  document module for write support. Preserve existing read route behavior.
- Add owner-authenticated write route under
  `/api/dashboard/work-roots/{workRootId}/files/write` or equivalent.
- Write requests must include workRoot-relative path, base content hash, and
  new content. Route validation must reject traversal, absolute paths,
  directories, binary/unsupported files, unavailable/offline roots, oversized
  content, and content-hash mismatches with bounded errors.
- Successful writes return new content hash, size, and saved timestamp.
- Add per-workRoot document events route, preferably SSE:
  `/api/dashboard/work-roots/{workRootId}/documents/events`.
  It may emit content-change/invalidation events from saves first and add
  watcher-backed external changes only if safe. Do not implement broad
  workspace watching if it widens scope.
- Keep absolute host paths, cache paths, watcher paths, and private diagnostics
  out of browser-visible errors/log-like response bodies.

Frontend files/modules:

- Extend `ReadOnlyFilePane` or introduce a compatible document pane state in
  `ws-dashboard/frontend/src/workRootFiles.ts`. Preserve existing logical keys,
  pane ids, preview/pinned behavior, restore descriptors, and placement.
- Extend `ReadOnlyMarkdownPane`, `ReadOnlyTextPane`, or shared document pane
  components in `ws-dashboard/frontend/src/App.tsx` so each pane owns local
  `mode: "view" | "edit"`, draft text, base content hash, dirty/saving/error,
  and stale/conflict state.
- Use CodeMirror 6 or an equivalent browser-native text editor foundation for
  edit mode if practical. A plain `<textarea>` is acceptable only if it is
  deliberately used as the smallest raw-text editor substrate and tests lock
  the same public edit/save/fan-out behavior.
- Route visible mode/save/revert controls through stable dashboard command ids
  where they mutate dashboard-visible state.
- Subscribe to one document event stream per visible/active workRoot, not one
  stream per pane. Tear it down when the workRoot is no longer active/visible.
- On window focus or visibility return, re-read open clean panes or otherwise
  reconcile content hashes so stale watcher delivery cannot be the only
  correctness path.

Forbidden temporary wiring:

- Do not mutate file content from the frontend without the daemon write route.
- Do not silently overwrite dirty panes during fan-out.
- Do not create duplicate workbench tabs for view/edit mode.
- Do not store file contents in persistent pane restore state.
- Do not add rename/delete/move/create or broad file-manager behavior.

## Integration Test Instructions

Backend tests:

- Extend `ws-dashboard/crates/daemon/tests/routes.rs` for write route auth,
  traversal rejection, unknown/offline/unavailable root rejection, successful
  write, content hash mismatch, size/binary safeguards where applicable, and
  no host-path leakage.
- Test document event route auth and at least save-triggered
  `document.contentChanged` or invalidation event delivery.

Frontend tests:

- Extend `ws-dashboard/frontend/src/workRootFiles.test.ts` or add
  document-edit helper tests for content hash handling, dirty/stale fan-out,
  clean-pane update, dirty-pane preservation, restore descriptor compatibility,
  and save/revert state transitions.
- Extend `ws-dashboard/frontend/src/commands.test.ts` if new document command
  ids/builders are added.

Browser acceptance:

- Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` to open a
  Markdown file, switch view/edit in the same pane, edit raw text, save,
  observe formatted view update, prove duplicate tabs were not created, and
  cover stale/conflict/fan-out behavior where practical.

Verification commands:

- `cargo test -p ws-dashboard-daemon`
- `npm run test:commands` if commands changed
- `npm run test:work-root-files`
- `npm run test:document-viewer`
- `npm run build`
- `npm run test:browser` unless blocked with exact blocker.

## Implementation Strategy Decisions

- Treat content hashes as daemon authority for writes. If the existing read
  route lacks a hash, add it there and update frontend types rather than using
  frontend-local hashes for save concurrency.
- Keep pane restore as descriptor-only: workRoot id, relative path, mode, and
  title are acceptable; file content, drafts, and dirty state are not.
- Save fan-out is source-keyed (`workRootId + path`) and pane-state aware:
  clean panes update, dirty panes mark stale/conflicted.
- Document events can start from daemon-triggered saves and bounded
  invalidation. Cross-platform file watching may be introduced only as a
  refresh hint if it is not broad or fragile.
- Translation overlays are invalidated when the source content hash changes.

## Rejected Alternatives

- Do not make Markdown view mode a rich text editor.
- Do not route editing through a terminal/nvim workflow.
- Do not let the browser decide save concurrency from local hashes only.
- Do not open one tab for view and a separate tab for edit.
- Do not implement overwrite/merge conflict workflows in this phase.

## Approach

- Add backend read content hash and write route with tests.
- Add document event service/route with save-triggered events and tests.
- Extend frontend pane state and API wrappers for write/save/events.
- Add raw edit UI and command builders.
- Implement fan-out and stale marking in App state.
- Add helper/browser tests and run the full verification contract.

## Constraints

- All routes remain owner-authenticated and workRoot-relative.
- Private host paths and daemon paths must not leak in response bodies, command
  payloads, browser-visible diagnostics, or copied pathrefs.
- Existing read-only preview/pinned lifecycle and workbench placement stay
  compatible.
- Browser-visible UI changes require daemon-served browser evidence.

## Out of scope

- Rich editing.
- Collaboration.
- Merge/overwrite UI.
- File create/rename/delete/move.
- Broad watcher correctness.
- General persistence of editor drafts or translation preferences.

## Details

Suggested write request/response:

```ts
type DocumentWriteRequest = {
  path: string;
  baseContentHash: string;
  content: string;
};

type DocumentWriteResponse = {
  contentHash: string;
  sizeBytes: number;
  savedAtMs: number;
};
```

Suggested event shape:

```ts
type DocumentEvent =
  | {
      type: "document.contentChanged";
      source: { workRootId: string; path: string };
      contentHash: string;
      changedAtMs: number;
      originPaneId?: string;
    }
  | {
      type: "document.watchInvalidated";
      source: { workRootId: string; path?: string };
      reason: "watcherError" | "tooManyFiles" | "workRootUnavailable";
    };
```

## Verification Contract

The phase is acceptable when:

- daemon tests prove read/write hash and conflict behavior;
- frontend tests prove mode state, fan-out, dirty stale preservation, and
  command builders where added;
- browser acceptance proves same-tab view/edit/save behavior against the
  daemon-served production frontend;
- document events or bounded invalidations are tested without relying on broad
  filesystem watchers;
- Phase 1 viewer and Phase 2 translation tests still pass.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md`:
  `260524-ws-dashboard-document-edit-save-fanout`,
  `260524-ws-dashboard-document-viewer-mode`,
  `260516-ws-web-dashboard-readonly-file-api`,
  `260516-ws-web-dashboard-readonly-text-pane`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260516-ws-web-dashboard-workroot-workbench-substrate`,
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`,
  `260516-ws-web-dashboard-instance-event-envelope-fixtures`,
  `260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold`
- [Must] `ai-docs/mental-model/ws-web-dashboard.md`
- [Must] `ai-docs/tickets/ready/260524-feat-ws-dashboard-document-viewer-editor-substrate.md`
