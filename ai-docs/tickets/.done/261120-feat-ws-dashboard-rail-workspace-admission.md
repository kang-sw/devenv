---
title: 세션 레일 명시적 워크스페이스 및 하단 미등록 승인/거부 목록
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 96c18e354b8f4dc3
sage-review-completeness-reviewed: 96c18e354b8f4dc3
completed: 2026-11-21
---

# Explicit-Only Session Rail Workspaces & Bottom Collapsible Unregistered Workspace Admission

## Background

In `agents-plugin-tool/internal/wsdashboard/pageview_rail.go` (`buildRailView`, lines 347-565), the session rail currently populates `RailView.Workspaces` with **both** explicitly stored workspaces from `workspaces.json` (`Stored == true`) **and** every implicitly discovered session root from `d.index.Sessions()` (`Stored == false`). Each implicit root renders as a top-level `.rws` section in `#rlist` (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`, line 458).

When tests, temporary scripts, or transient sessions register under ephemeral directories (such as `/private/var/folders/.../T/Test*`), 30-100+ unregistered `.rws` sections flood the top of `#rlist` and push explicitly added workspaces out of view.

This ticket restricts top-level `#rlist` to explicitly added workspaces (`Stored == true`) and moves all implicitly discovered session roots into a single, **always-collapsed-by-default** admission panel at the bottom of `#rlist`, reusing the worktree admission grammar (`rail_worktree_admission_route.go`, the worktree-admission block in `dashboard.css` starting at line 404) and matching the tracked mockup:

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
   - **Per-row draft toggles**: `Approve (✓)` (`.tg.ap`) and `Reject (✕)` (`.tg.rj`), mutually exclusive, same-click clears, persisted in `localStorage` (`ws.dashboard.workspace-admission.drafts`) across reloads/SSE swaps until explicitly cleared or applied (a draft whose row is not currently rendered stays stored but is left out of counts and Apply; Phase 2.3).
   - **Header `Reject all` shortcut** (`.unreg-reject-all`): toggles all currently visible (filtered) pending roots to `reject` draft (or clears their `reject` draft when all visible rows are already drafted `reject`).
   - **Inline filter input** (`#unreg-filter`): shown when `len(PendingWorkspaces) > 10`; filters pending rows case-insensitively by folder name or full path while preserving scroll and input focus.
   - **Bounded scrollable pending list** (`.unreg-list`, `max-height: 260px; overflow-y: auto`): bounds panel height regardless of temp-dir flood size.
   - **Clickable session-count pill** (`.unreg-scount`): expands inline `/s/<id>` session links (`.unreg-sessions` / `.unreg-slink`) with small harness badge (`.hb.sm`), session label/ID, and relative timestamp so users can open unregistered sessions without approving the root.
   - **Nested collapsed rejected fold** (`details.rrej[data-key="rail-unregistered-rejected"]`): displays `Rejected (N)` with per-row `Undo` (`.unreg-undo`, styled like the worktree `.admission-undo`; see Phase 2.1).
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

### Promotion-Settled Decisions

6. **(46) Partition presentation only; keep the full rail inventory for facts and layout** (announced — prescribed by 261113-feat-ws-dashboard-session-rail-tree, "Ops are validated against the rail as the daemon renders it (railBaseView -> railFactsOf)", and the worktree-admission precedent 679b9b9b, "Preserve ... full backend inventory; partition only presentation"):
   - `buildRailView` and `railBaseView` keep building every workspace (stored and implicit) into `view.Workspaces` exactly as today. `railFactsOf`, `orderRailView`, `splitRailArchived`, `classifyRailAdmissions`, the layout route's `pruneRailLayout`, session hire (`session_hire.go`, `session_hire_routes.go`), and `handleRailAdmission` all keep seeing pending/rejected roots' sessions, so a session in an unregistered root keeps its hire roster, stars, order, and children.
   - The partition is a final presentation step at the end of `railView` (after ordering, archive split, admission classification, and memo counts).
   - Rejected: partitioning inside `buildRailView` / `railBaseView`. It would shrink `railFactsOf`, emptying hire rosters for pending-root requesters and letting `pruneRailLayout` delete their layout state.
7. **(47) Keep the lister's `wanted` set unchanged** (technical): leaving it unchanged is chosen over limiting `wanted` to stored repos.
   - Reason: implicit repo roots are already read today, so the change adds no cost. A custom `worktree_pool` is only known from a completed read, so keeping the reads preserves pooled-session hiding. `l.admission.observe` and the full facts of (46) also depend on those reads. Plain-folder temp roots (`in.Repo == ""`) never enter `wanted`.
   - Rejected: limiting `wanted` to stored repos. It would leak custom-pool sessions into pending rows and counts, and stop worktree-admission observation for unregistered repos.
8. **(48) Apply semantics: per-endpoint all-or-nothing batches, submitted sequentially, stopping at the first refusal** (technical; per-endpoint atomicity follows 261115-feat-ws-dashboard-worktree-detection-approval (1103) "A stale, invalid or unverifiable selection refuses the whole Apply batch without admission writes; retain drafts", and (1100) "revalidate them at Apply"):
   - `POST /rail/workspaces/admission` validates every selection before any write. If any selection is invalid or stale, the whole batch is refused with no writes. The precise checks are in Phase 1.2.
   - When workspace and worktree drafts coexist, the client submits the workspace batch first. It submits the worktree batch only if the workspace batch answered `applied: true`.
   - A refused batch keeps its drafts and every unsent draft. The toast names which group applied and which was refused.
   - Rejected: a combined cross-store endpoint. Two independent stores cannot commit atomically together, and a combined endpoint would fork the shipped worktree-admission contract.
   - Rejected: parallel submission. It clears drafts unpredictably on a partial failure.

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
   - `agents-plugin-tool/internal/wsdashboard/assets/rail.js` (1048 lines) already exceeds the default 1000-line `ws-size-guard` soft limit (default hard limit 2000; `agents-plugin-tool/internal/mcp/git_merge_size.go:25,227-241`; `git check-attr` reports `unspecified` for it), and `assets/dashboard-rail.js` (1054 lines) carries its own per-path limit `ws-size-guard=1300` (`.gitattributes:27`), which a 1054-line file has not yet crossed. Place new workspace-admission JS logic in a dedicated module file (e.g., `assets/dashboard-workspace-admission.js`) registered in `dashboardScriptFiles` (`pages.go:44-66`) and `TestDashboardSourceAssembly` (`asset_source_test.go:20`), keeping edits to `rail.js` / `dashboard-rail.js` minimal or splitting clean hooks rather than growing those files further.
   - Any new CSS classes in `assets/dashboard-rail.css` (or a new `assets/dashboard-workspace-admission.css` registered in `dashboardStyleFiles` at `pages.go:68-78` and `asset_source_test.go:21`) must pass `TestCSSClassesDoNotCollideAcrossModules` (`css_collision_test.go:272`) across `dashboard`, `memo`, `tracker`, and `whiteboard`.
   - Every `{{t ...}}` / `wsT(...)` / `wsi18n.T(...)` key must exist in all 11 tables under `agents-plugin-tool/internal/wsi18n/tables/*.json` with matching placeholders and zero unused/duplicate keys (`internal/wsi18n/keys_test.go`).
   - Layout width constraint: the production rail is `340px` wide (`dashboard-rail.css:20,31`), narrowing only through `max-width: calc(100vw - 24px)` on narrow viewports; the panel must have zero horizontal overflow (`scrollWidth <= clientWidth`) at `340px` and at a forced `280px` rail width as the narrow bound.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

## Prior Art

- `agents-plugin-tool/internal/wsdashboard/workspaces.go`: `workspaceStore`, `WorkspacesFilePath`, `writePrivateFile`, `locateWorkspace`, `sweep`.
- `agents-plugin-tool/internal/wsdashboard/rail_worktree_admission.go` & `rail_worktree_admission_route.go` & `rail_worktree_admission_view.go`: unknown-field preservation (`extra`), atomic `writePrivateFile`, `fs.ErrNotExist`-only pruning, `sameOrigin` + `workspaceBody` + `{applied, gen, html}` reply contract.
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css` (worktree-admission block from line 404) & `dashboard-rail.js:206-402` & `rail.js:35-49`: `.tg.ap` / `.tg.rj`, `.rrej`, `.admission-undo`, `.rcap`, `.rtoast`, `localStorage` draft persistence.

## Prior Decisions

- 261113-feat-ws-dashboard-session-rail-tree (2026-11-14, commit 51429395): "Ops are validated against the rail as the daemon renders it (railBaseView -> railFactsOf): a row's group is the RailWorktree it sits under, or the plain folder." — bearing: contradiction-candidate
- 261115-feat-ws-dashboard-worktree-detection-approval (2026-11-15, Decisions): "(1103) A stale, invalid or unverifiable selection refuses the whole Apply batch without admission writes; retain drafts ... Rejected: partial application with easily missed per-row outcomes." — bearing: constrains
- 261115-feat-ws-dashboard-worktree-detection-approval (2026-11-15, Decisions): "(1100) Persist unsubmitted selections in browser-local storage across reload and rail refresh, separately from shared admission; revalidate them at Apply." — bearing: supports
- 679b9b9b (2026-11-15, commit): "Preserve the existing admitted worktree/session/tree markup and full backend inventory; partition only presentation with the supplied PendingWorktrees, AdmittedWorktrees and RejectedWorktrees helpers." — bearing: constrains
- 261110-feat-ws-dashboard-workspaces (2026-11-10, Decisions): "A session joins a stored git workspace when its resolved root worktree equals the stored root. A session whose root does not resolve to a repository joins a stored plain folder ..." — bearing: supports
- 261110-feat-ws-dashboard-workspaces (2026-11-10, Decisions): "Stored workspaces join the worktree reread set ((114) ok). A stored workspace with no session gets its worktree list on the rail's cadence" — bearing: supports
- 261112-feat-ws-dashboard-rail-memo-pad (2026-11-12, Decisions): "Scope is the rail workspace: memos belong to a rail workspace entry (the Workspaces the rail template renders, keyed by its Path)" — bearing: constrains
- 261110-feat-ws-dashboard-session-rail (2026-11-10, fda6c7a1): "worktree-list changes arrive by broker wake (railListRepoKey), so no lister stamp is needed." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdashboard/workspace_admission.go (new), workspace_routes.go, daemon.go, rail.go, pageview_rail.go, assets/dashboard.tmpl, dashboard-rail.css, dashboard-rail.js, dashboard-workspace-admission.js (new), pages.go, wsi18n/tables/*.json (11), tests |
| scope.surface | public-interface | new exported WorkspaceAdmissionFilePath, new HTTP route POST /rail/workspaces/admission, new RailView fields; railFactsOf consumers beyond the ticket's list (session_hire.go:143,161,444, session_hire_routes.go:225, rail_layout_route.go:105, rail_worktree_admission_route.go:160) keep reading the unpartitioned railBaseView ((46)) |
| scope.new_public_symbol | yes | WorkspaceAdmissionFilePath; RailView.PendingWorkspaces, RailView.RejectedWorkspaces |
| scope.new_type_contract | yes | workspace-admission.json schema v1, admission request/reply JSON, RailWorkspace display helpers |
| scope.test_surface | new-files | workspace_admission_test.go and rail_workspace_admission_browser_test.go are new (neither exists); rail_test.go, rail_admission_test.go, rail_lifecycle_test.go, railworktreedelete_test.go call sites also change |
| complexity.reuse_points | confirmed | railLayoutDoc extra-preserving Unmarshal/Marshal (rail_layout.go:136-205), writePrivateFile (endpoint.go:99), workspaceBody and sameOrigin, handleRailAdmission reply shape, dashboard.js openState/restoreOpen, existing .rrej/.admission-undo/.rcap/.rtoast CSS and JS |
| complexity.side_effect_risk | moderate | the partition is presentation-only at the end of railView ((46)), so railFactsOf, session hire, layout ops/prune and worktree admission keep the full inventory; the wanted set is unchanged ((47)); handleWorktreePage resolves against the pre-partition view |
| risk.correctness | moderate | layout-generation vs admission-generation separation for railMinGen (1.2 reply); whole-batch stale validation; row-less draft retention and exclusion from counts and Apply |
| risk.fit | low | follows the worktree admission grammar and its presentation-only partition precedent; a separate T0 store file beside workspaces.json |
| risk.test | moderate | buildRailView call sites stay unchanged; railView/render-level tests and a new Playwright fixture that must construct wsAdmission itself; browser verification needs WS_PLAYWRIGHT_MODULE |
| risk.security_or_contract | moderate | additive T0 file and route, sameOrigin guard reused; approve adds a client-named directory to workspaces.json through the existing add path |

## Phases

### Phase 1: Backend Store (`dashboard/workspace-admission.json`), Route (`POST /rail/workspaces/admission`), & Rail Partitioning

**Goal:** Persist workspace admission rejections beside `workspaces.json`, expose the batch admission HTTP endpoint, and partition `RailView` so top-level `Workspaces` holds only explicit (`Stored == true`) workspaces while implicitly discovered roots route into `PendingWorkspaces` and `RejectedWorkspaces`.

#### 1.1 Store (`agents-plugin-tool/internal/wsdashboard/workspace_admission.go`)
- Derive `WorkspaceAdmissionFilePath(workspacesFile string) string`: when `workspacesFile == ""`, return `""` (store is `nil`, feature disabled); otherwise return `filepath.Join(filepath.Dir(workspacesFile), "workspace-admission.json")`.
  - In production (`cmd/ws-mcp/dashboard.go:126`), `WorkspacesFile` is `<configDir>/dashboard/workspaces.json`, so this places `workspace-admission.json` in `<configDir>/dashboard/workspace-admission.json`. In test fixtures (`rail_worktree_admission_browser_test.go:39`), it places it right beside the fixture's `workspaces.json` (that fixture builds a `&Daemon{...}` literal with `openWorkspaceStore(filepath.Join(dir, "workspaces.json"))` and does not go through the `daemon.go:519` constructor path, so a new fixture must set `wsAdmission` explicitly).
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
  - Track `mu sync.Mutex`, `file string`, `doc`, `seen railFileStamp`, `rejectedSet map[string]bool` (cleaned paths), and monotonic `gen uint64` (initialized from `time.Now().UnixMicro()` like `railLayoutStore` at `rail_layout.go:346`).
  - `rejected() []string` and `isRejected(path string) bool` / `generation() uint64`: read strictly from in-memory map/slice under `mu` — **zero disk I/O** on render/stamp paths.
  - `mutate(fn)`: under `mu`, stat/reload if file changed on disk (preserving foreign-process writes), apply transformation, encode, and write atomically via `writePrivateFile` only if bytes changed; bump `gen` on change.
  - `reject(paths []string) (uint64, error)` and `undo(paths []string) (uint64, error)` and `clearRejected(paths []string) (uint64, error)`.
  - `sweep() bool`: reload from disk if `statRailFile(s.file)` moved; check `os.Lstat(r.Path)` for each rejected record and drop only entries returning `errors.Is(err, fs.ErrNotExist)` (never on `EACCES`/transient errors); write only if entries were removed; return `true` if `rejectedSet` changed (because of foreign write or missing-dir prune) so `railLister` wakes `railListRepoKey`.
- Wire `wsAdmission *workspaceAdmissionStore` into `Daemon` (`daemon.go:293,519-525`):
  - Initialize `d.wsAdmission = openWorkspaceAdmissionStore(WorkspaceAdmissionFilePath(opts.WorkspacesFile))`.
  - Compose `d.rail.sweep` so each full sweep runs both `d.workspaces.sweep()` and `d.wsAdmission.sweep()` (short-circuit-free OR: `c1 := d.workspaces.sweep(); c2 := d.wsAdmission.sweep(); return c1 || c2`).
  - Include `d.wsAdmission.generation()` in `railSource.stamp` (`rail.go:228-235`) so a foreign write picked up by `sweep` is never skipped as unchanged; this daemon's own mutations reach every rail stream through the route's `d.broker.wake(railListRepoKey)` (1.2), since the stamp is only checked on refresh ticks.

#### 1.2 Route (`POST /rail/workspaces/admission` in `workspace_routes.go`)
- Register `workspaceAdmissionRoute = "/rail/workspaces/admission"` in `Daemon.workspaceRoutes(mux)` (`workspace_routes.go:57-66`, active whenever `d.workspaces != nil`).
- Request/response types:
  - Request: `{ "selections": [ { "path": "/abs/path", "action": "approve" | "reject" | "undo" } ] }`
  - Body: the same `POST` + `sameOrigin(r)` + decode checks as `workspaceBody`, but with a route-specific `workspaceAdmissionBodyLimit = 256 << 10`. A 100-row temp-dir `Reject all` alone approaches the shared `workspaceBodyLimit` (`16 << 10`).
    - Add the limit as a parameter to a small shared helper that `workspaceBody` delegates to. Existing routes keep `16 << 10`.
  - Selections: `1..500` (the worktree route's bound, `rail_worktree_admission_route.go:154`). Outside it → `400` `{ "applied": false, "message": ... }`.
- Whole-batch validation before any write ((48)). Any failure refuses the batch, writes neither store, and returns `{ "applied": false, "message": <localized> }`.
  - `400` (malformed): an empty, relative, or unclean path; a duplicate path in the batch; an `action` outside `{"approve", "reject", "undo"}`.
  - `409` (stale, retain drafts and refresh):
    - `approve` of a path that is not an existing directory (`readWorkspaceFolder`) or is already stored.
    - `reject` of a path already stored in `d.workspaces`. (A `reject` of an already-rejected path is an idempotent no-op that passes validation: the store write is byte-unchanged.)
    - `undo` of a path not currently rejected.
    - `approve` / `reject` of a path the latest `railView` does not show as a pending or rejected root.
- Apply, after validation passes:
  - `approve`: `d.workspaces.add(path, false)` (stores the repository root or plain folder that `locateWorkspace` resolves), then `d.wsAdmission.clearRejected([path, add.Root])`.
  - `reject`: `d.wsAdmission.reject(paths)`. Nothing on disk outside `workspace-admission.json` is touched.
  - `undo`: `d.wsAdmission.undo(paths)`.
- On success call `d.broker.wake(railListRepoKey)` (as workspace add/forget at `workspace_routes.go:225,251` and worktree admission at `rail_worktree_admission_route.go:199` do), so other tabs and the `/rail/events` iframe re-render at once; approve changes `workspaces.json`, which the stamp does not cover.
- Reply: re-render `view := d.railView(d.pagesIn(lang))` and `region, err := renderRailRegion(view, lang)`, then return `200` JSON `{ "applied": true, "gen": view.LayoutGen, "html": string(region.HTML) }` (with `Cache-Control: no-store`).
  - `gen` is the **layout** generation only. The client feeds it into `railMinGen`, which `WsRail.staleRail` (`rail.js:195-198`) compares against the render's `data-lgen`.
  - Mixing in the admission generation would mark every later SSE rail render stale.
  - Admission freshness for SSE renders comes from `railSource.stamp` (1.1).

#### 1.3 Rail Partitioning & Consumer Interactions (`pageview_rail.go`, `rail.go`, `rail_layout.go`, `rail_rows.go`, `workspace_routes.go`, `worktree_page.go`)
- Extend `RailView` (`pageview_rail.go:42-69`):
  - After the partition, `Workspaces []RailWorkspace` holds only **explicitly stored** workspaces (`Stored == true` from `workspaces.json`). As today, a stored workspace with 0 sessions still renders as an empty `.rws` bar.
  - Add `PendingWorkspaces []RailWorkspace` and `RejectedWorkspaces []RailWorkspace`.
- `buildRailView` (`pageview_rail.go:347-565`) and `railBaseView` (`rail.go:59-134`) stay unchanged in membership: every workspace, stored and implicit, is built into `view.Workspaces` ((46)).
- Add a pure helper `partitionRailWorkspaces(view *RailView, rejected []string)` in `pageview_rail.go`. `railView` (`rail.go:36-51`) calls it as its last step, after `orderRailView`, `splitRailArchived`, `classifyRailAdmissions`, and memo counts. It takes `rejected` from `d.wsAdmission.rejected()` (in memory, nil-safe).
  - **Global counts (`Live`, `CountPolicy`, `CountApproval`, `CountSleep`)**: unchanged. `buildRailView` already sums them across all non-pooled listed sessions (`pageview_rail.go:397-406`), matching `workspace-admission-v1.js:342-345`, so the header live/sleep badges stay accurate.
  - **Plain-folder grouping (`pageview_rail.go:385-395`)**: unchanged. A session matches the longest `Stored` plain folder (`folders`); an unmatched plain-folder session forms an implicit root.
  - **Partitioning**:
    - `Stored == true` -> stays in `view.Workspaces`.
    - `Stored == false` and the path is in the rejected set -> `view.RejectedWorkspaces`.
    - A rejected path that `view.Workspaces` does not hold and that is not stored also goes into `view.RejectedWorkspaces`, as a session-less entry (`Name: lastElem(p)`, `Path: p`). It stays visible under `Rejected (N)` for `Undo` until `sweep` prunes it because its directory is gone.
    - Any other `Stored == false` -> `view.PendingWorkspaces`.
  - **Ordering of `PendingWorkspaces` / `RejectedWorkspaces`**:
    - Order `PendingWorkspaces` by the newest member session's `Registration.RegisteredAt` descending, tie-broken by `Path` ascending (registration time never moves with heartbeats or activity). `RegisteredAt` is an RFC3339Nano string (`registry.go:92`): parse it as `pageview_hover.go:24` does, and have `buildRailView` record the newest parsed time on each `RailWorkspace` (membership unchanged) so the pure partition can sort without registrations; order `RejectedWorkspaces` by `Path` ascending.
    - An unstored root with zero visible sessions (a repository whose every session is pooled, kept as `byWS[key] = nil` at `pageview_rail.go:376-384`) is not listed in `PendingWorkspaces`; a rejected one still lists under `RejectedWorkspaces` for `Undo`.
  - **Display helpers on `RailWorkspace`**:
    - Add helper methods (e.g. `ParentPath() string` returning directory prefix with trailing separator for `.unreg-parent`, `SessionCount() int` flattening live + ended + archived sessions across plain-folder or repo worktrees, and `AllSessions() []RailRow` for inline `/s/<id>` links in the unregistered panel).
- **Consumer verification & boundary checks**:
  - Unchanged, because they read the unpartitioned view ((46)):
    - `railFactsOf`, `orderRailView`, `splitRailArchived`, `classifyRailAdmissions`.
    - The layout route (`rail_layout_route.go:105`) and `pruneRailLayout` (`rail_layout.go:1039`).
    - Session hire (`session_hire.go:143,161,444`, `session_hire_routes.go:225`).
    - `handleRailAdmission` (`rail_worktree_admission_route.go:160`).
    - Layout state (stars, order, children) of a session in a pending root survives exactly as today.
  - **Declared consequences of the presentation-only partition ((46))**:
    - A session in a pending or rejected root has no `.rs` row, so it shows no row-level rail cue: no hire chip, no tree nesting, no star. This includes the requester-row hire chip and nesting that 261115-feat-ws-session-hire-approval Phase 2 plans. Hire arrival for such a requester relies on that ticket's top-bar badge, queue, and toast. A hire chip on `.unreg-slink` is new scope, not part of this ticket.
    - The detected-worktree admission rows (`PendingWorktrees`, `RejectedWorktrees`) and the `RejectedPrompt` (`rail_worktree_admission_view.go:7-37`) of a pending or rejected root are hidden until the workspace is approved, because they loop over the partitioned `view.Workspaces`. A worktree draft stored earlier for such a row surfaces as the existing `js.admission.fix.missing` correction (`dashboard-rail.js:262`) and is retained, not lost. The 2.5 flood fixture asserts this degrades without a page error.
  - The lister's `wanted` set in `railBaseView` (`rail.go:113`, `rail.go:125-128`) is unchanged ((47)). Plain-folder temp roots never enter it, and implicit repo roots are read as they are today.
  - Split `railView` into an unexported `railOrderedView(pages)` (today's `railView` body: base view, ordering, archive split, admission classification, memo counts) and `railView` = `railOrderedView` + `partitionRailWorkspaces`. `handleWorktreePage` (`worktree_page.go:33-63`) iterates `d.railOrderedView(...).Workspaces`, so every worktree page link (`worktreePagePath`) that resolves today keeps resolving, including links into a pending, rejected, or zero-visible-session (all-pooled) unstored repository.
  - `railRepository` (`workspace_routes.go:257-280`) is unchanged. It allows stored repositories and listed sessions' repositories.
  - `splitRailRows` (`rail_rows.go:15-66`): scans for `.rs[data-rid]` elements. Pending/rejected inline session links inside `details.rws-unreg` MUST NOT use `.rs` (use `.unreg-slink`, Phase 2), so incremental row patching never touches or breaks on unregistered rows.
  - **Existing unit tests to update (`rail_test.go`, `rail_admission_test.go`, `rail_lifecycle_test.go`, `railworktreedelete_test.go`)**:
    - Existing tests that call `buildRailView(..., nil, now)` keep passing unchanged, because `buildRailView` membership is unchanged ((46)). The call sites are `rail_test.go:80,123,132,169,230,282,367,419,447,1130`, `rail_admission_test.go:92,114,157`, `rail_lifecycle_test.go:37`, and `railworktreedelete_test.go:617,642`.
    - Update only tests that render the rail region or call `railView` end to end with unstored session roots. Those roots now appear under `PendingWorkspaces` instead of the top-level `.rws` list.

#### 1.4 Phase 1 Tests (`workspace_admission_test.go`, `workspace_routes_test.go`, `rail_test.go`)
- Store round-trip, unknown-field (`extra`) preservation, `version > 1` write refusal, `fs.ErrNotExist` pruning on `sweep()` vs retention on `EACCES`/non-empty dirs, zero-I/O on `rejected()`/`isRejected()`/`generation()`.
- `POST /rail/workspaces/admission`: `sameOrigin` + `POST` guards, invalid/duplicate/relative path rejection (`400`), batch approve (`d.workspaces.add` + clears rejection), batch reject (persists without touching disk), undo (clears rejection), missing dir on approve (`409`), returned `{applied, gen, html}` updates `#rlist` immediately.
- Route additions: an idempotent `reject` of an already-rejected path is accepted with a byte-unchanged store; a successful batch wakes `railListRepoKey` (broker wake observed); a refused batch writes neither store.
- `handleWorktreePage` resolves a worktree page of a pending (unstored) repository and of an unstored repository whose sessions are all pooled.
- `partitionRailWorkspaces` / `railView` tests:
  - `PendingWorkspaces` order: newest member `RegisteredAt` descending, then `Path` ascending; an unstored root with zero visible (all-pooled) sessions is not pending.
  - Explicit-only `Workspaces`; `PendingWorkspaces` vs `RejectedWorkspaces`, including the session-less rejected entry.
  - The 0-explicit-workspace state and isolation of 30+ temp roots.
  - Unchanged global `Live`/`CountSleep` totals, and `railPooled` exclusion on non-stored repos.
  - (46) regression: `railFactsOf(d.railBaseView(...))` still contains a pending-root session's row, and a layout op on it is accepted, not pruned.

### Result (c6160d4b7) - 2026-11-21

- Added the separate schema-1 `workspace-admission.json` rejection store with unknown-field retention, in-memory rail reads, foreign-write reload, missing-directory pruning, and refusal to overwrite malformed or newer data. `workspaces.json` remains unchanged, preserving the T0 compatibility boundary.
- Added the guarded batch admission route with whole-batch selection validation, a 256 KiB body limit, immediate rail HTML and layout-generation reply, and rail wake. Store preflight now refuses malformed or newer external edits before any approval write.
- Kept the full rail inventory for facts, layout, admissions, and worktree pages; the final presentation step partitions stored, pending, and rejected workspaces. Storage-disabled daemons retain their existing visible session roots.
- Added the two server refusal keys in all 11 locales and regression coverage for store behavior, route validation, partitioning, no-store mode, and malformed-file preflight. Independent review raised two Important findings; both were fixed and covered by tests.
- Verification: the requested dashboard test selection, `go vet ./internal/wsdashboard/... ./internal/wsi18n/...`, `go build ./cmd/ws-mcp`, and the i18n tests excluding `TestEveryEnglishKeyIsUsed` passed. The full i18n suite still fails on four unused `wb.*` keys (`wb.banner`, `wb.closed.more`, `wb.doc`, `wb.title`) already present in `HEAD` before this phase and unrelated to it.
- Phase 2 handoff: the template, CSS, JavaScript, and browser fixture still need to render and operate the new pending/rejected fields. Until that panel lands, roots moved out of the top-level rail are not visible in storage-enabled mode; land both phases together.

### Phase 2: Bottom Collapsible Unregistered Panel UI, Batch JS, 11 Locales, & Playwright Visual Verification

**Depends on:** Phase 1 (`RailView.PendingWorkspaces`, `RailView.RejectedWorkspaces`, `POST /rail/workspaces/admission`).

**Landing order:** Phase 1 alone is an intermediate state (unstored roots leave the top-level rail before the bottom panel renders them). Phase 1 and Phase 2 merge to the integration branch together; Phase 1 may be committed and reviewed separately on the implementation branch only.

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
      - Nested rejected fold (`{{with .RejectedWorkspaces}}`): `<details class="rrej" id="unreg-rrej" data-key="rail-unregistered-rejected">` (collapsed by default), `.rrej-list`, `.rrej-row[data-ws-rej-path="{{.Path}}"]`, `.rrej-row-info`, and `button.unreg-undo[data-ws-undo="{{.Path}}"]`.
        - **Not `.admission-undo`**: `admissionClick` (`dashboard-rail.js:380-401`) matches any `.admission-undo` and reads `b.closest(".rwt")` evidence, which throws on a `.rrej-row` (or, on a `.rwt`, posts a ghost worktree undo and stores a stale worktree draft). `.unreg-undo` takes the `.admission-undo` look through a shared CSS selector list, and the workspace module handles it.
- Preserve `.rcap` and `.rtoast` outside `#rlist` so they float cleanly above `.rfoot` even when `#rlist` scrolls (`.rcap` is a template sibling of `#rlist` at `dashboard.tmpl:458`; `.rtoast` is not in the template, `dashboard-rail.js:312-334` creates it and appends it to the rail element at `:324`).

#### 2.2 CSS (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css`)
- Port all `.rail-empty-guide`, `details.rws-unreg`, `.unreg-*`, `.rrej-list`, `.rrej-row*` styles, and the mockup's Undo style as `.unreg-undo` (added to the existing `.admission-undo` rule's selector list), from `workspace-admission-v1.css:213-483` into `dashboard-rail.css` (or a new `dashboard-workspace-admission.css` added to `dashboardStyleFiles` in `pages.go:68-78` and `asset_source_test.go:21`), using `--wb-*` light/dark CSS variables.
- Ensure `#rlist` is `display: flex; flex-direction: column;` so `details.rws-unreg { margin-top: auto; flex: none; }` pins quietly to the bottom when explicit workspaces are short, and scrolls naturally at the foot when explicit workspaces overflow.
- Verify `TestCSSClassesDoNotCollideAcrossModules` (`css_collision_test.go:272`): every new class name (`rws-unreg`, `unreg-*`, `rail-empty-*`, `rrej-list`, `rrej-row*`) stays unique to the `dashboard` stylesheet module.
- Verify zero horizontal overflow (`scrollWidth <= clientWidth`) at both `280px` and `340px` rail widths across dark and light themes.

#### 2.3 JavaScript & `.rcap` Coordination (`assets/dashboard-workspace-admission.js`, `dashboard-rail.js`, `rail.js`)
- Create `agents-plugin-tool/internal/wsdashboard/assets/dashboard-workspace-admission.js` and register it right after `dashboard-rail.js` in `dashboardScriptFiles` (`pages.go:44-66`) and `TestDashboardSourceAssembly` (`asset_source_test.go:20`), keeping `rail.js` (1048 lines) and `dashboard-rail.js` (1054 lines) from growing further under `ws-size-guard`.
- **Draft persistence**: store workspace admission drafts in `localStorage` under `ws.dashboard.workspace-admission.drafts` (`[{ path, action }]`, `action` in `{"approve","reject"}`), restored after reload and every SSE rail swap.
- **Filter, inline sessions, & open-state retention across SSE swaps**:
  - Maintain in-memory UI state across SSE swaps: `wsUnregFilter` string (filter `#unreg-filter` value, focus, and cursor `selectionStart`/`selectionEnd`, plus `.unreg-list` `scrollTop`) and `wsUnregExpanded` `Set` of paths whose inline `.unreg-sessions` links are expanded.
  - `details.rws-unreg[data-key="rail-unregistered-workspaces"]` and `details.rrej[data-key="rail-unregistered-rejected"]` reuse `dashboard.js`'s existing `openState` + `restoreOpen` (`openState` declared at `dashboard.js:220`, `restoreOpen` at `dashboard.js:288`), keeping both **collapsed by default** on initial load and retaining intentional user expand/collapse across SSE region swaps.
  - If the currently viewed session `rid` lives inside an expanded `.unreg-sessions`, mark its `.unreg-slink[href="/s/<rid>"]` with `.cur` / `aria-current="page"` during sync (without touching `.rs`).
  - `.rail-empty-add[data-add="ws"]` delegates seamlessly through the existing `.radd` click handler in `dashboard-rail.js:532-537` (simply ensure `railEl` delegates `[data-add="ws"]` whether `.radd` or `.rail-empty-add`).
- **Coordinated `.rcap` & Batch Submission**:
  - Expose a minimal coordination hook between `dashboard-rail.js` (worktree admission) and `dashboard-workspace-admission.js` (workspace admission) so `.rcap` reflects combined pending drafts (`wtApprove + wsApprove`, `wtReject + wsReject`), `capon` toggles when `total > 0`, and `.unreg-draft-pill` shows the localized `js.ws.admission.drafts` count (ko: `선택 N`) on the collapsed summary bar when workspace drafts exist. Every user-visible JS string goes through the 2.4 `js.ws.admission.*` keys via `wsT`; the Korean strings here are the ko table values, never literals.
  - **Row-less drafts are kept, not pruned**: a stored draft whose path has no `.unreg-row[data-unreg-path]` in the current render (approved elsewhere, session-less, pruned, or simply not listed yet while the index is still loading after a daemon restart) stays in `localStorage`, but is left out of the `.unreg-draft-pill` / `.rcap` counts and out of the Apply selections, so it can neither trigger a `409` nor be lost to a partial render; it re-activates when its row renders again (the user's earlier choice for that path still stands) and is removed only by Clear (`[data-admission-clear]`, which wipes the whole workspace draft store, row-less entries included) or by a successful Apply that submitted it. Row-less drafts get no visible cue of their own; this is accepted, since they can never act until their row is visible again. The filter only hides rows with `hidden` and never removes them from the DOM, so filtered-out rows count as rendered. The server's whole-batch `409` remains the backstop for a race between swap and Apply.
  - `[data-admission-clear]` clears both draft stores.
  - `[data-admission-apply]` submits:
    - If only workspace drafts exist: `POST /rail/workspaces/admission` with `{ selections: [{ path, action }] }`, applies returned `html` via `swap("rail", ...)`, clears exactly the workspace drafts it submitted (row-less drafts survive), raises `railMinGen` to the reply's `gen` (the layout generation, Phase 1.2), and shows `.rtoast.ok` with `js.ws.admission.toast.applied` (ko: `적용 완료: 승인 N · 거부 M`).
    - If only worktree drafts exist: existing `POST /rail/worktrees/admission` flow runs unchanged.
    - If both exist ((48)):
      - Submit the workspace batch first, and the worktree batch only after it answers `applied: true`.
      - A refusal stops the sequence. The refused and unsent batches keep their drafts.
      - Apply the latest returned `html`, and show one `.rtoast` naming the group that applied and the group that was refused.
    - A `409` refusal keeps every draft of that batch and shows the localized message (the warn `.rtoast` variant without `.ok`, `dashboard.css:491-493`).
  - `.unreg-undo[data-ws-undo]`: immediately submits `{ selections: [{ path, action: "undo" }] }` to `POST /rail/workspaces/admission` and displays `.rtoast.ok`.

#### 2.4 Localization (All 11 `agents-plugin-tool/internal/wsi18n/tables/*.json` Files)
- Add new `rail.ws.unreg.*` and `js.ws.admission.*` keys to `en.json` and translate into all 10 sibling tables (`de.json`, `es.json`, `fr.json`, `it.json`, `ja.json`, `ko.json`, `pt-BR.json`, `ru.json`, `zh-Hans.json`, `zh-Hant.json`):
  - Empty guidance (`rail.ws.empty.title`, `rail.ws.empty.sub`, `rail.ws.empty.add`, `rail.ws.empty.hint`), bottom summary (`rail.ws.unreg.title`, `rail.ws.unreg.pending`, `rail.ws.unreg.rejectall`, `rail.ws.unreg.rejectall.clear`, `rail.ws.unreg.filter`, `rail.ws.unreg.filter.empty`, `rail.ws.unreg.empty`, `rail.ws.unreg.approve`, `rail.ws.unreg.approve.title`, `rail.ws.unreg.reject`, `rail.ws.unreg.reject.title`, `rail.ws.unreg.sessions.title`, `rail.ws.unreg.rejected`, `rail.ws.unreg.undo`, `rail.ws.unreg.undo.title`, `js.ws.admission.drafts`, `js.ws.admission.toast.applied`, `js.ws.admission.toast.approved`, `js.ws.admission.toast.rejected`, `js.ws.admission.toast.undone`, `js.ws.admission.toast.partial` (mixed Apply: names the applied group and the refused group), `js.ws.admission.failed`), plus server-side refusal messages for 1.2 (`rail.ws.admission.invalid` for `400`, `rail.ws.admission.stale` for `409`), rendered via `wsi18n.T` in the request's language.
- Verify zero missing/extra/empty/placeholder-mismatched keys via `go test ./internal/wsi18n/...`.

#### 2.5 Playwright Visual & Behavioral Verification (`rail_workspace_admission_browser_test.go` + `testdata/rail-workspace-admission.browser.cjs`)
- Build an isolated `httptest` fixture mirroring `rail_worktree_admission_browser_test.go` (no real daemon/provider) covering:
  1. **30-junk-root flood**: verify `#rlist` renders only explicit `.rws` sections at top, `details.rws-unreg` stays collapsed by default (`open == null`), and zero horizontal overflow (`scrollWidth <= clientWidth`) at both `1440x1000` and `700x1000` in dark and light themes, plus a forced `280px` rail width; a worktree-admission draft seeded for a worktree of a pending repository surfaces as a retained `js.admission.fix.missing` correction with no page error.
  2. **0-explicit-workspace empty state**: verify `.rail-empty-guide` renders `+` button and bottom-bar hint while `details.rws-unreg` remains collapsed by default.
  3. **Intentional expand, filter (`> 10`), `모두 거부` toggle, inline `/s/<id>` expansion & navigation**: verify filtering retains focus/scroll, toggling session pill reveals `/s/<id>` links and navigates cleanly.
  4. **`.rcap` batch approve / reject / clear & `.unreg-undo` & SSE open-state retention**: verify approve promotes root to top-level `.rws`, reject moves it to `details.rrej`, undo returns it to pending (when sessions exist) with no page error, a swap rendering without a drafted row keeps that draft stored but out of the `.rcap` counts and the Apply body, and the worktree-admission draft store (`admissionStore`, `dashboard-rail.js:208`) unchanged, `.rtoast` sits cleanly above `.rfoot`/`.rcap`, and SSE rail swap preserves `details.rws-unreg` open state when intentionally expanded.
  5. **Visual comparison**: capture production light/dark rail captures side-by-side with the same states rendered from `ai-docs/assets/mockups/261120/rail-workspace-admission/workspace-admission-v1.html` (collapsed flood, expanded panel with filter, rejected fold open, empty guide). Pass criterion: every mockup structural element (`.rail-empty-guide`, `summary.unreg-sum` parts, `.unreg-toolbar`, `.unreg-filter-wrap`, `.unreg-row` parts, `.unreg-sessions`, `details.rrej` rows, `.rcap`) is present in the same order and grouping, with zero horizontal overflow; pixel identity is not required. Attach the capture paths to the phase `### Result`.

### Result (127688fdc) - 2026-11-21

- Added the empty-rail guidance card and bottom collapsed unregistered-workspace panel with bounded pending rows, filtering, inline session links, a rejected fold, Undo, and theme-aware styling. Explicit workspaces remain the only top-level rail sections.
- Added browser-local workspace drafts and combined workspace/worktree capsule counts. Apply submits the workspace batch first, preserves refused and row-less drafts, then submits worktree drafts only after workspace success. Clear removes both draft stores. A timed workspace request releases the rail gate on failure.
- Added 11-locale labels and messages, asset assembly, and an isolated production-renderer Playwright fixture. Captures in `/tmp/rawshots-p2/`: `rail-pending-{dark,light}.png`, `rail-expanded-{dark,light}.png`, `rail-expanded-700-light.png`, `rail-expanded-280-light.png`, `rail-rejected-expanded-dark.png`, `rail-empty-{dark,light}.png`, and `mockup-{pending-dark,expanded-light}.png`. Visual inspection found no overlap or clipping; the panel matches the approved mockup's structure and grouping.
- Verification passed: the requested targeted dashboard Go tests, `TestCSSClassesDoNotCollideAcrossModules`, `TestDashboardSourceAssembly`, new and existing worktree-admission Playwright tests, `go vet ./...`, and `go build ./cmd/ws-mcp` (temporary binary removed). The full i18n run reports only four pre-existing unused keys (`wb.banner`, `wb.closed.more`, `wb.doc`, `wb.title`), all present on base `596109dbe`; all other i18n tests, including key parity and placeholders, pass.
- Independent `codex.medium` code-review-lite found two Important issues: false mixed-Apply feedback on workspace refusal and a workspace POST without a timeout. Both were fixed; browser acceptance covers workspace and worktree batch refusals, retained drafts, changed-hash SSE swaps, viewport bounds, and toast placement. A follow-up test assertion for the retained hidden worktree-draft correction landed in `a0ee9806a`.

## Verification

```sh
# Phase 1 & Phase 2 unit + i18n + CSS + asset assembly tests
cd agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsi18n/...

# Targeted Playwright acceptance test (requires WS_PLAYWRIGHT_MODULE)
cd agents-plugin-tool && WS_PLAYWRIGHT_MODULE="$(node -e "console.log(require.resolve('playwright'))")" \
  go test -v ./internal/wsdashboard/ -run TestRailWorkspaceAdmissionBrowser
```
