---
title: 세션 레일 명시적 워크스페이스 및 하단 미등록 승인/거부 목록
---

# Explicit-Only Session Rail Workspaces & Bottom Collapsible Unregistered Workspace Admission

## Background

In `agents-plugin-tool/internal/wsdashboard/pageview_rail.go` (`buildRailView`, lines 347-565), the session rail currently populates `RailView.Workspaces` with **both** explicitly stored workspaces from `workspaces.json` (`Stored == true`) **and** every implicitly discovered session root from `d.index.Sessions()` (`Stored == false`). Each implicit root renders as a top-level `.rws` section in `#rlist` (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`, line 458).

When tests, temporary scripts, or transient sessions register under ephemeral directories (such as `/private/var/folders/.../T/Test*`), 30-100+ unregistered `.rws` sections flood the top of `#rlist` and push explicitly added workspaces out of view.

This ticket restricts top-level `#rlist` to explicitly added workspaces (`Stored == true`) and moves all implicitly discovered session roots into a single, **always-collapsed-by-default** admission panel at the bottom of `#rlist`, reusing the worktree admission grammar (`rail_worktree_admission_route.go`, `dashboard.css:406-500`) and matching the tracked mockup:

- `ai-docs/assets/mockups/261120/rail-workspace-admission/workspace-admission-v1.html`
- `ai-docs/assets/mockups/261120/rail-workspace-admission/workspace-admission-v1.css`
- `ai-docs/assets/mockups/261120/rail-workspace-admission/workspace-admission-v1.js`

## Decisions

### Settled Owner Decisions (2026-11-20 23:01 KST, `(43)=a`, `(45)=ok`)

1. **Explicit-only top-level `#rlist`**:
   - Top-level `RailView.Workspaces` (`.rws` sections in `#rlist`) renders **only** explicitly added workspaces (`workspaces.json`, `Stored == true`).
   - When 0 explicit workspaces exist, `#rlist` renders a subtle empty-rail guidance card (`.rail-empty-guide`) with a `+` add-workspace button (`data-add="ws"`) and, when unregistered pending roots exist, a hint pointing down to the still-collapsed bottom bar.
2. **Single bottom collapsible unregistered panel (`details.rws-unreg[data-key="rail-unregistered-workspaces"]`)**:
   - All implicitly discovered roots (registered non-pooled session roots whose resolved workspace is not in `workspaces.json`) go into one collapsible `<details class="rws-unreg" data-key="rail-unregistered-workspaces">` at the bottom of `#rlist`.
   - **Always collapsed by default**, even when 30-100+ temp roots flood in and even when 0 explicit workspaces exist. Open state persists only after intentional user toggle via `openState` (`data-key` / `restoreOpen` in `dashboard.js`) across SSE swaps.
3. **Mockup & worktree-admission UX parity**:
   - **Per-row draft toggles**: `Approve (✓)` (`.tg.ap`) and `Reject (✕)` (`.tg.rj`), mutually exclusive, same-click clears, persisted in `localStorage` (`ws.dashboard.workspace-admission.drafts`) across reloads/SSE swaps until explicitly cleared or applied.
   - **Header `Reject all` shortcut** (`.unreg-reject-all`): toggles all currently visible (filtered) pending roots to `reject` draft (or clears their `reject` draft when all visible rows are already drafted `reject`).
   - **Inline filter input** (`#unreg-filter`): shown when `len(PendingWorkspaces) > 10`; filters pending rows case-insensitively by folder name or full path while preserving scroll and input focus.
   - **Bounded scrollable pending list** (`.unreg-list`, `max-height: 260px; overflow-y: auto`): bounds panel height regardless of temp-dir flood size.
   - **Clickable session-count pill** (`.unreg-scount`): expands inline `/s/<id>` session links (`.unreg-sessions` / `.unreg-slink`) with small harness badge (`.hb.sm`), session label/ID, and relative timestamp so users can open unregistered sessions without approving the root.
   - **Nested collapsed rejected fold** (`details.rrej[data-key="rail-unregistered-rejected"]`): displays `Rejected (N)` with per-row `Undo` (`.admission-undo`).
   - **Floating Apply capsule (`.rcap`) & rail toast (`.rtoast`)**: shows Clear (`초기화`), draft counts (`승인 N` / `거부 N`), and Apply (`적용`), coordinated cleanly with any worktree-admission drafts, and surfaces `.rtoast` feedback on Apply/Undo.
4. **Admission state semantics & auto-prune**:
   - **Approve**: calls `d.workspaces.add(path, false)` to persist the root in `workspaces.json` (promoting it to top-level `RailView.Workspaces`) and deletes any rejected record for that path.
   - **Reject**: persists `"rejected"` in `dashboard/workspace-admission.json` and moves the root into `거부됨 (N)` without touching any files or sessions on disk and without generating an agent investigation prompt (unlike worktree rejection, workspace rejection is a clean UI dismissal of transient/unwanted roots).
   - **Undo**: removes the rejected entry from `dashboard/workspace-admission.json`; if listed sessions for that root still exist, it returns to pending, otherwise it disappears cleanly.
   - **Missing directory prune**: rejected paths whose directory no longer exists (`os.Lstat(path)` returns `fs.ErrNotExist`) are auto-pruned during `sweep` / full scan without pruning on transient I/O errors.
5. **Compatibility Tier: `T0 transparent`** (see `ai-docs/manuals/release-compatibility.md`):
   - **Rejected alternative — storing rejections inside `workspaces.json`**:
     - `workspaces.go:77-105` (`workspaceStore.reload`) unmarshals into fixed struct `workspacesDoc` (`workspaces.go:33-40`, `Version` + `Workspaces []workspaceRecord{Path}`), dropping all unknown JSON keys.
     - `workspaces.go:108-122` (`workspaceStore.save`) marshals `workspacesDoc` and overwrites the file atomically via `writePrivateFile`.
     - Any older binary running `add`, `forget`, or `sweep` (`workspaces.go:171-254`) would silently strip unknown fields from `workspaces.json` with no recovery path (Scenario 10 in `release-compatibility.md:114` — NOT T2 and not patch-safe).
   - **Rejected alternative — storing workspace rejections inside `rail-layout.json`**:
     - `rail-layout.json` lives in `DaemonOptions.Dir` (cache root, `rail_layout.go:346`), which is wiped when daemon caches are cleared. User-curated workspace admission choices belong in config home beside `workspaces.json` (`workspaces.go:15-17`), where cache clears preserve them.
   - **Chosen design — separate additive file `dashboard/workspace-admission.json` beside `workspaces.json` + additive route `POST /rail/workspaces/admission`**:
     - Older binaries never read or write `dashboard/workspace-admission.json` and continue reading/writing `workspaces.json` schema v1 untouched.
     - Reopening with the new binary preserves all rejected entries in `dashboard/workspace-admission.json`, while older binaries simply treat unregistered roots as before (or never overwrite `workspace-admission.json`).
     - Both root and record structs in `workspace-admission.json` retain unknown JSON fields via `extra map[string]json.RawMessage` (matching `rail_layout.go:106-216`), keeping future schema-1 additions T0 as well.
     - Both `workspaces.json` and `workspace-admission.json` are empty/disabled (`nil`) when `DaemonOptions.WorkspacesFile == ""`.

## Constraints

1. **Shipped-surface boundary (`ai-docs/manuals/shipped-surface-boundary.md`)**:
   - No repository-internal ticket stems, commit hashes, or devenv-only paths in any user-facing dashboard strings, embedded templates, JS, or MCP tool messages.
2. **File-touch budget & memory-first reads (`ai-docs/manuals/performance.md`)**:
   - `railView` / `railBaseView` / `railSource.stamp` MUST read `workspaceAdmissionStore` state strictly from memory (zero `os.ReadFile` or `os.Stat` calls on the interactive render path).
   - Disk reads occur only at `openWorkspaceAdmissionStore` startup, under the store mutex before a mutation write, and during background `sweep` (using stat-before-read or reload-on-write only when missing directories are pruned, writing nothing when unchanged).
   - Pruning uses `os.Lstat(path)` only for stored rejected entries during background `sweep` (never per-session or per-render), and deletes an entry only on `errors.Is(err, fs.ErrNotExist)`.
3. **Compatibility (`ai-docs/manuals/release-compatibility.md`)**:
   - Keep `workspaces.json` schema v1 untouched (`WorkspacesFile` remains `workspacesDoc{Version, Workspaces}`).
   - `dashboard/workspace-admission.json` must tolerate and preserve unknown JSON keys (`extra map[string]json.RawMessage`).
4. **Asset assembly, CSS isolation, i18n, and `ws-size-guard`**:
   - `agents-plugin-tool/internal/wsdashboard/assets/rail.js` (1048 lines) and `assets/dashboard-rail.js` (1054 lines) already exceed the 1000-line `ws-size-guard` soft limit (`agents-plugin-tool/internal/mcp/git_merge_size.go:25,227-241`). Place new workspace-admission JS logic in a dedicated module file (e.g., `assets/dashboard-workspace-admission.js`) registered in `dashboardScriptFiles` (`pages.go:44-66`) and `TestDashboardSourceAssembly` (`asset_source_test.go:20`), keeping edits to `rail.js` / `dashboard-rail.js` minimal or splitting clean hooks rather than growing those files further.
   - Any new CSS classes in `assets/dashboard-rail.css` (or a new `assets/dashboard-workspace-admission.css` registered in `dashboardStyleFiles` at `pages.go:68-78` and `asset_source_test.go:21`) must pass `TestCSSClassesDoNotCollideAcrossModules` (`css_collision_test.go:272`) across `dashboard`, `memo`, `tracker`, and `whiteboard`.
   - Every `{{t ...}}` / `wsT(...)` / `wsi18n.T(...)` key must exist in all 11 tables under `agents-plugin-tool/internal/wsi18n/tables/*.json` with matching placeholders and zero unused/duplicate keys (`internal/wsi18n/keys_test.go`).
   - Layout width constraint: the production rail supports `280px`-`340px` (`dashboard-rail.css:20,31`) and must have zero horizontal overflow (`scrollWidth <= clientWidth`).

## Prior Art

- `agents-plugin-tool/internal/wsdashboard/workspaces.go`: `workspaceStore`, `WorkspacesFilePath`, `writePrivateFile`, `locateWorkspace`, `sweep`.
- `agents-plugin-tool/internal/wsdashboard/rail_worktree_admission.go` & `rail_worktree_admission_route.go` & `rail_worktree_admission_view.go`: unknown-field preservation (`extra`), atomic `writePrivateFile`, `fs.ErrNotExist`-only pruning, `sameOrigin` + `workspaceBody` + `{applied, gen, html}` reply contract.
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css:406-500` & `dashboard-rail.js:206-402` & `rail.js:35-49`: `.tg.ap` / `.tg.rj`, `.rrej`, `.admission-undo`, `.rcap`, `.rtoast`, `localStorage` draft persistence.

## Phases

### Phase 1: Backend Store (`dashboard/workspace-admission.json`), Route (`POST /rail/workspaces/admission`), & Rail Partitioning

**Goal:** Persist workspace admission rejections beside `workspaces.json`, expose the batch admission HTTP endpoint, and partition `RailView` so top-level `Workspaces` holds only explicit (`Stored == true`) workspaces while implicitly discovered roots route into `PendingWorkspaces` and `RejectedWorkspaces`.

#### 1.1 Store (`agents-plugin-tool/internal/wsdashboard/workspace_admission.go`)
- Derive `WorkspaceAdmissionFilePath(workspacesFile string) string`: when `workspacesFile == ""`, return `""` (store is `nil`, feature disabled); otherwise return `filepath.Join(filepath.Dir(workspacesFile), "workspace-admission.json")`.
  - In production (`cmd/ws-mcp/dashboard.go:126`), `WorkspacesFile` is `<configDir>/dashboard/workspaces.json`, so this places `workspace-admission.json` in `<configDir>/dashboard/workspace-admission.json`. In test fixtures (`rail_worktree_admission_browser_test.go:39`), it places it right beside the fixture's `workspaces.json`.
- Schema (`version: 1`):
  ```json
  {
    "version": 1,
    "rejected": [
      { "path": "/abs/clean/path" }
    ]
  }
  ```
  Implement custom `UnmarshalJSON`/`MarshalJSON` on both root doc and record structs to retain unknown fields in `extra map[string]json.RawMessage` (matching `rail_layout.go:106-216`), rejecting `version > 1` on write without overwriting newer files.
- In-memory state (`workspaceAdmissionStore`):
  - `openWorkspaceAdmissionStore(file string) *workspaceAdmissionStore`: returns `nil` when `file == ""`; loads once at startup (`fs.ErrNotExist` => empty list; malformed JSON => keep safe/refuse writes).
  - Track `mu sync.Mutex`, `file string`, `doc`, `seen railFileStamp`, `rejectedSet map[string]bool` (cleaned paths), and monotonic `gen uint64` (initialized from `time.Now().UnixMicro()` like `railLayoutStore` at `rail_layout.go:347`).
  - `rejected() []string` and `isRejected(path string) bool` / `generation() uint64`: read strictly from in-memory map/slice under `mu` — **zero disk I/O** on render/stamp paths.
  - `mutate(fn)`: under `mu`, stat/reload if file changed on disk (preserving foreign-process writes), apply transformation, encode, and write atomically via `writePrivateFile` only if bytes changed; bump `gen` on change.
  - `reject(paths []string) (uint64, error)` and `undo(paths []string) (uint64, error)` and `clearRejected(paths []string) (uint64, error)`.
  - `sweep() bool`: reload from disk if `statRailFile(s.file)` moved; check `os.Lstat(r.Path)` for each rejected record and drop only entries returning `errors.Is(err, fs.ErrNotExist)` (never on `EACCES`/transient errors); write only if entries were removed; return `true` if `rejectedSet` changed (because of foreign write or missing-dir prune) so `railLister` wakes `railListRepoKey`.
- Wire `wsAdmission *workspaceAdmissionStore` into `Daemon` (`daemon.go:293,519-525`):
  - Initialize `d.wsAdmission = openWorkspaceAdmissionStore(WorkspaceAdmissionFilePath(opts.WorkspacesFile))`.
  - Compose `d.rail.sweep` so each full sweep runs both `d.workspaces.sweep()` and `d.wsAdmission.sweep()` (short-circuit-free OR: `c1 := d.workspaces.sweep(); c2 := d.wsAdmission.sweep(); return c1 || c2`).
  - Include `d.wsAdmission.generation()` in `railSource.stamp` (`rail.go:228-235`) so admission mutations immediately trigger a fresh rail render.

#### 1.2 Route (`POST /rail/workspaces/admission` in `workspace_routes.go`)
- Register `workspaceAdmissionRoute = "/rail/workspaces/admission"` in `Daemon.workspaceRoutes(mux)` (`workspace_routes.go:57-66`, active whenever `d.workspaces != nil`).
- Request/response types:
  - Request: `{ "selections": [ { "path": "/abs/path", "action": "approve" | "reject" | "undo" } ] }`
  - Enforce `workspaceBody(w, r, &req)` (`POST`, `sameOrigin(r)`, `workspaceBodyLimit` — bump or bound `1..500` selections cleanly, e.g. `64 << 10` if batch of 500 paths exceeds `16 << 10`, or document explicit limit).
  - Validate every selection before mutating either store: non-empty clean absolute path, no duplicate paths in one batch, `action` in `{"approve", "reject", "undo"}`. Reject invalid requests with `400` JSON `{ "applied": false, "message": ... }`.
  - For `approve`: check `os.Stat(path)` / `readWorkspaceFolder` (if missing or not a dir, fail cleanly with `409` or partial-free refusal before mutating, or return localized refusal message); call `d.workspaces.add(path, false)` (which promotes to `workspaces.json` as root repo or plain folder) and clear both `path` and `a.Root` from `d.wsAdmission`.
  - For `reject`: skip or disallow if already `Stored` in `d.workspaces`; persist in `d.wsAdmission.reject(...)`.
  - For `undo`: remove from `d.wsAdmission.undo(...)`.
- Wake `d.broker.wake(railListRepoKey)`, re-render `view := d.railView(d.pagesIn(lang))` and `region, err := renderRailRegion(view, lang)`, and return `200` JSON:
  `{ "applied": true, "gen": <max(layoutGen, wsAdmissionGen)>, "html": string(region.HTML) }` (with `Cache-Control: no-store`).

#### 1.3 Rail Partitioning & Consumer Interactions (`pageview_rail.go`, `rail.go`, `rail_layout.go`, `rail_rows.go`, `workspace_routes.go`, `worktree_page.go`)
- Extend `RailView` (`pageview_rail.go:42-69`):
  - Keep `Workspaces []RailWorkspace` strictly for **explicitly stored** workspaces (`Stored == true` from `workspaces.json`), preserving 0-session stored workspaces as empty `.rws` bars as today.
  - Add `PendingWorkspaces []RailWorkspace` and `RejectedWorkspaces []RailWorkspace`.
- Extend `buildRailView` (`pageview_rail.go:347-565`) — or apply an explicit partition helper accepting `rejected []string` (kept clean so `buildRailView` remains pure and testable without a server):
  - **Global counts (`Live`, `CountPolicy`, `CountApproval`, `CountSleep`)**: keep summing across ALL non-pooled listed sessions (`pageview_rail.go:397-406`), matching `workspace-admission-v1.js:342-345` so header live/sleep badges remain accurate.
  - **Plain-folder grouping (`pageview_rail.go:385-395`)**: continues matching against longest `Stored` plain folder (`folders`); unmatched plain-folder sessions form implicit roots.
  - **Partitioning**:
    - Keys in `storedAt` -> `view.Workspaces` (`Stored: true`).
    - Keys NOT in `storedAt`:
      - If in `rejectedSet` -> `view.RejectedWorkspaces` (plus any rejected path in `rejected` that is not in `storedAt` even if it currently has 0 active sessions, so it remains visible under `Rejected (N)` for `Undo` until its directory disappears from disk and is pruned by `sweep`; compute `Name: lastElem(p)` and `ParentPath` helper for display).
      - Else (has ≥ 1 listed non-pooled session, not `Stored`, not `rejected`) -> `view.PendingWorkspaces`.
  - **Ordering of `PendingWorkspaces` / `RejectedWorkspaces`**:
    - Order `PendingWorkspaces` deterministically (newest session activity descending, tie-broken by `Path` ascending) so fresh temp/project roots appear consistently without jumping on unrelated heartbeats; order `RejectedWorkspaces` by `Path` ascending (or persisted order).
  - **Display helpers on `RailWorkspace`**:
    - Add helper methods (e.g. `ParentPath() string` returning directory prefix with trailing separator for `.unreg-parent`, `SessionCount() int` flattening live + ended + archived sessions across plain-folder or repo worktrees, and `AllSessions() []RailRow` for inline `/s/<id>` links in the unregistered panel).
- **Consumer verification & boundary checks**:
  - `railFactsOf`, `orderRailView`, `splitRailArchived`, `classifyRailAdmissions`: operate on `view.Workspaces` (explicit workspaces only). Unregistered roots never pollute `rail-layout.json`'s `workspace_order` or `workspaces` map (`pruneRailLayout` keeps explicit layout clean). Note: `railBaseView` still includes `in.Repo` of pending/rejected sessions in `wanted` if needed or leaves them light — wait: verify `railBaseView` (`rail.go:112-129`) does NOT spawn unnecessary git worktree reads for 100+ temp dirs unless already explicit, OR keep `wanted` limited to `Stored` repos (check: `railPooled` already checks default pool roots without `repos[key]`, and pending rows only show session count + `/s/<id>` links, not worktree rows! Leaving non-stored repos out of `wanted` avoids 100+ `git worktree list` subprocesses on temp floods while preserving pooled-worktree exclusion via `worktreePoolRoots(nil, key)` in `railPooled`!). Document this explicitly!
  - `splitRailRows` (`rail_rows.go:15-66`): scans for `.rs[data-rid]` elements. Pending/rejected inline session links inside `details.rws-unreg` MUST NOT use `.rs` (use `.unreg-slink`, Phase 2), so incremental row patching never touches or breaks on unregistered rows.
  - `railRepository` (`workspace_routes.go:257-280`) and `handleWorktreePage` (`worktree_page.go:39-63`): document exact behavior — `handleWorktreePage` iterates `railView.Workspaces` (explicit workspaces); `railRepository` continues allowing stored repos or listed session repos for `/rail/worktrees/*` unless restricted.
  - **Existing unit tests to update (`rail_test.go`, `rail_admission_test.go`, `rail_lifecycle_test.go`, `railworktreedelete_test.go`)**:
    - Existing tests calling `buildRailView(..., nil, now)` (`rail_test.go:80,123,132,169,282,367,419,447,1130`, `rail_admission_test.go:92,157`, `rail_lifecycle_test.go:37`, `railworktreedelete_test.go:617,642`) previously expected implicit roots in `view.Workspaces`. Pass `stored` where explicit-workspace grouping/layout is tested, and assert `PendingWorkspaces` where unregistered discovery is tested (including `TestRailViewWithoutAListShowsTheSessionRoots` at `rail_test.go:121` and `TestRailViewShowsStoredWorkspaces` at `rail_test.go:207`).

#### 1.4 Phase 1 Tests (`workspace_admission_test.go`, `workspace_routes_test.go`, `rail_test.go`)
- Store round-trip, unknown-field (`extra`) preservation, `version > 1` write refusal, `fs.ErrNotExist` pruning on `sweep()` vs retention on `EACCES`/non-empty dirs, zero-I/O on `rejected()`/`isRejected()`/`generation()`.
- `POST /rail/workspaces/admission`: `sameOrigin` + `POST` guards, invalid/duplicate/relative path rejection (`400`), batch approve (`d.workspaces.add` + clears rejection), batch reject (persists without touching disk), undo (clears rejection), missing dir on approve (`409`), returned `{applied, gen, html}` updates `#rlist` immediately.
- `buildRailView` partitioning tests: explicit-only `Workspaces`, `PendingWorkspaces` vs `RejectedWorkspaces`, 0-explicit-workspace state, 30+ temp roots isolation, global `Live`/`CountSleep` totals, and `railPooled` exclusion on non-stored repos.

### Phase 2: Bottom Collapsible Unregistered Panel UI, Batch JS, 11 Locales, & Playwright Visual Verification

**Depends on:** Phase 1 (`RailView.PendingWorkspaces`, `RailView.RejectedWorkspaces`, `POST /rail/workspaces/admission`).

#### 2.1 Template (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`)
- Update `{{define "r-rail"}}` (`dashboard.tmpl:458`):
  - **0-explicit-workspace guidance card** (`{{else}}` branch of `{{range $ws := .Workspaces}}`): replace bare `<div class="empty">{{t "index.empty"}}</div>` with `.rail-empty-guide` matching `workspace-admission-v1.html` / `workspace-admission-v1.js:206-218`: folder icon (`{{icon "wsfolder"}}`), title, description, `+ Workspace` button (`class="rail-empty-add" data-add="ws"` when `$.CanAdd`), and conditional hint (`{{with .PendingWorkspaces}}...{{end}}`) pointing down to the still-collapsed bottom bar.
  - **Bottom collapsible panel** (at the very end of `#rlist`, after `.Workspaces` / `.rail-empty-guide`, rendered whenever `$.CanAdd` or pending/rejected roots exist; **never** carrying `open` by default):
    - `<details class="rws-unreg" data-key="rail-unregistered-workspaces">`
    - `<summary class="unreg-sum">`: chevron (`#l-chevron-right`), `.unreg-dot` (`.zero` when `len .PendingWorkspaces == 0`), localized label, client-filled `.unreg-draft-pill` (`hidden` by default), `.unreg-count` (`{{len .PendingWorkspaces}}`).
    - `.unreg-body`:
      - `.unreg-toolbar`: pending stat + `#unreg-reject-all` (`disabled` when `len .PendingWorkspaces == 0`).
      - Conditional `.unreg-filter-wrap` (`{{if gt (len .PendingWorkspaces) 10}}`): search icon (`#l-search`) + `<input type="search" class="unreg-filter" id="unreg-filter" ...>`.
      - `.unreg-list#unreg-list`: pending rows (`div.rwt.d-pend.unreg-row` with `data-unreg-path="{{.Path}}"` — intentionally **without** `data-admission` / `data-repository` so worktree-admission selectors in `dashboard-rail.js:231` never misread workspace rows, and **without** `.wthd` so `openWorktree` / `rdStart` drag handlers never hijack clicks/drags on pending workspace rows!).
        - Each row contains `.tgs` (`button.tg.ap[data-ws-admission-action="approve"]`, `button.tg.rj[data-ws-admission-action="reject"]`), `.unreg-info` (`title="{{.Path}}"`, `.unreg-name`, `.unreg-parent`), and `button.unreg-scount[data-tog-sess="{{.Path}}"]` (`aria-expanded="false"`).
        - Inline `.unreg-sessions` (initially `hidden`): `<a class="unreg-slink" href="/s/{{.ReplyID}}">` with `<span class="hb sm" data-h="{{.Harness}}"></span>`, `.unreg-stitle`, and `<time class="unreg-stime t" ...>`. Crucially **no `.rs` class** so `splitRailRows` (`rail_rows.go:37`) and `WsRail.markCurrent` remain isolated.
      - Nested rejected fold (`{{with .RejectedWorkspaces}}`): `<details class="rrej" id="unreg-rrej" data-key="rail-unregistered-rejected">` (collapsed by default), `.rrej-list`, `.rrej-row[data-ws-rej-path="{{.Path}}"]`, `.rrej-row-info`, and `button.admission-undo[data-ws-undo="{{.Path}}"]`.
- Preserve `.rcap` and `.rtoast` outside `#rlist` so they float cleanly above `.rfoot` even when `#rlist` scrolls (`dashboard.tmpl:458`, `dashboard-rail.js:324`).

#### 2.2 CSS (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css`)
- Port all `.rail-empty-guide`, `details.rws-unreg`, `.unreg-*`, `.rrej-list`, `.rrej-row*`, `.admission-undo` styles from `workspace-admission-v1.css:213-452` into `dashboard-rail.css` (or a new `dashboard-workspace-admission.css` added to `dashboardStyleFiles` in `pages.go:68-78` and `asset_source_test.go:21`), using `--wb-*` light/dark CSS variables.
- Ensure `#rlist` is `display: flex; flex-direction: column;` so `details.rws-unreg { margin-top: auto; flex: none; }` pins quietly to the bottom when explicit workspaces are short, and scrolls naturally at the foot when explicit workspaces overflow.
- Verify `TestCSSClassesDoNotCollideAcrossModules` (`css_collision_test.go:272`): every new class name (`rws-unreg`, `unreg-*`, `rail-empty-*`, `rrej-list`, `rrej-row*`) stays unique to the `dashboard` stylesheet module.
- Verify zero horizontal overflow (`scrollWidth <= clientWidth`) at both `280px` and `340px` rail widths across dark and light themes.

#### 2.3 JavaScript & `.rcap` Coordination (`assets/dashboard-workspace-admission.js`, `dashboard-rail.js`, `rail.js`)
- Create `agents-plugin-tool/internal/wsdashboard/assets/dashboard-workspace-admission.js` and register it right after `dashboard-rail.js` in `dashboardScriptFiles` (`pages.go:44-66`) and `TestDashboardSourceAssembly` (`asset_source_test.go:20`), keeping `rail.js` (1048 lines) and `dashboard-rail.js` (1054 lines) from growing further under `ws-size-guard`.
- **Draft persistence**: store workspace admission drafts in `localStorage` under `ws.dashboard.workspace-admission.drafts` (`[{ path, action }]`, `action` in `{"approve","reject"}`), restored after reload and every SSE rail swap.
- **Filter, inline sessions, & open-state retention across SSE swaps**:
  - Maintain in-memory UI state across SSE swaps: `wsUnregFilter` string (filter `#unreg-filter` value, focus, and cursor `selectionStart`/`selectionEnd`, plus `.unreg-list` `scrollTop`) and `wsUnregExpanded` `Set` of paths whose inline `.unreg-sessions` links are expanded.
  - `details.rws-unreg[data-key="rail-unregistered-workspaces"]` and `details.rrej[data-key="rail-unregistered-rejected"]` reuse `dashboard.js`'s existing `openState` + `restoreOpen` (`dashboard.js:277-291`), keeping both **collapsed by default** on initial load and retaining intentional user expand/collapse across SSE region swaps.
  - If the currently viewed session `rid` lives inside an expanded `.unreg-sessions`, mark its `.unreg-slink[href="/s/<rid>"]` with `.cur` / `aria-current="page"` during sync (without touching `.rs`).
  - `.rail-empty-add[data-add="ws"]` delegates seamlessly through the existing `.radd` click handler in `dashboard-rail.js:532-537` (simply ensure `railEl` delegates `[data-add="ws"]` whether `.radd` or `.rail-empty-add`).
- **Coordinated `.rcap` & Batch Submission**:
  - Expose a minimal coordination hook between `dashboard-rail.js` (worktree admission) and `dashboard-workspace-admission.js` (workspace admission) so `.rcap` reflects combined pending drafts (`wtApprove + wsApprove`, `wtReject + wsReject`), `capon` toggles when `total > 0`, and `.unreg-draft-pill` shows `선택 N` on the collapsed summary bar when workspace drafts exist.
  - `[data-admission-clear]` clears both draft stores.
  - `[data-admission-apply]` submits:
    - If only workspace drafts exist: `POST /rail/workspaces/admission` with `{ selections: [{ path, action }] }`, applies returned `html` via `swap("rail", ...)`, clears workspace drafts, updates `railMinGen`, and shows `.rtoast.ok` (`적용 완료: 승인 N · 거부 M` or localized equivalent).
    - If only worktree drafts exist: existing `POST /rail/worktrees/admission` flow runs unchanged.
    - If both exist simultaneously: submits each non-empty batch to its respective endpoint, clears only batches that returned `applied: true`, applies latest `html`, and shows combined `.rtoast` / error feedback so a partial failure never loses unapplied drafts.
  - `.admission-undo[data-ws-undo]`: immediately submits `{ selections: [{ path, action: "undo" }] }` to `POST /rail/workspaces/admission` and displays `.rtoast.ok`.

#### 2.4 Localization (All 11 `agents-plugin-tool/internal/wsi18n/tables/*.json` Files)
- Add new `rail.ws.unreg.*` and `js.ws.admission.*` keys to `en.json` and translate into all 10 sibling tables (`de.json`, `es.json`, `fr.json`, `it.json`, `ja.json`, `ko.json`, `pt-BR.json`, `ru.json`, `zh-Hans.json`, `zh-Hant.json`):
  - Empty guidance (`rail.ws.empty.title`, `rail.ws.empty.sub`, `rail.ws.empty.add`, `rail.ws.empty.hint`), bottom summary (`rail.ws.unreg.title`, `rail.ws.unreg.pending`, `rail.ws.unreg.rejectall`, `rail.ws.unreg.rejectall.clear`, `rail.ws.unreg.filter`, `rail.ws.unreg.filter.empty`, `rail.ws.unreg.empty`, `rail.ws.unreg.approve`, `rail.ws.unreg.approve.title`, `rail.ws.unreg.reject`, `rail.ws.unreg.reject.title`, `rail.ws.unreg.sessions.title`, `rail.ws.unreg.rejected`, `rail.ws.unreg.undo`, `rail.ws.unreg.undo.title`, `js.ws.admission.drafts`, `js.ws.admission.toast.applied`, `js.ws.admission.toast.approved`, `js.ws.admission.toast.rejected`, `js.ws.admission.toast.undone`, `js.ws.admission.failed`).
- Verify zero missing/extra/empty/placeholder-mismatched keys via `go test ./internal/wsi18n/...`.

#### 2.5 Playwright Visual & Behavioral Verification (`rail_workspace_admission_browser_test.go` + `testdata/rail-workspace-admission.browser.cjs`)
- Build an isolated `httptest` fixture mirroring `rail_worktree_admission_browser_test.go` (no real daemon/provider) covering:
  1. **30-junk-root flood**: verify `#rlist` renders only explicit `.rws` sections at top, `details.rws-unreg` stays collapsed by default (`open == null`), and zero horizontal overflow (`scrollWidth <= clientWidth`) at both `1440x1000` and `700x1000` in dark and light themes.
  2. **0-explicit-workspace empty state**: verify `.rail-empty-guide` renders `+` button and bottom-bar hint while `details.rws-unreg` remains collapsed by default.
  3. **Intentional expand, filter (`> 10`), `모두 거부` toggle, inline `/s/<id>` expansion & navigation**: verify filtering retains focus/scroll, toggling session pill reveals `/s/<id>` links and navigates cleanly.
  4. **`.rcap` batch approve / reject / clear & `.admission-undo` & SSE open-state retention**: verify approve promotes root to top-level `.rws`, reject moves it to `details.rrej`, undo returns it to pending (when sessions exist), `.rtoast` sits cleanly above `.rfoot`/`.rcap`, and SSE rail swap preserves `details.rws-unreg` open state when intentionally expanded.
  5. **Visual comparison**: capture production light/dark captures side-by-side against `ai-docs/assets/mockups/261120/rail-workspace-admission/workspace-admission-v1.html`.

## Verification

```sh
# Phase 1 & Phase 2 unit + i18n + CSS + asset assembly tests
cd agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsi18n/...

# Targeted Playwright acceptance test (requires WS_PLAYWRIGHT_MODULE)
cd agents-plugin-tool && WS_PLAYWRIGHT_MODULE="$(node -e "console.log(require.resolve('playwright'))")" \
  go test -v ./internal/wsdashboard/ -run TestRailWorkspaceAdmissionBrowser
```
