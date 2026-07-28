---
title: Inline alias (rename) for workspace and workroot nav labels
sage-review-design: required
---

# Inline alias (rename) for workspace and workroot nav labels

## Background

Dashboard nav labels for workspaces and workroots are server-derived: the daemon
computes each label from the directory basename (`discovery.rs:546`
`label_for_path`) and the frontend renders `view.label` verbatim. There is no way
for a user to give a workspace/workroot a friendlier display name. This ticket
adds a per-entity, user-editable **alias** that overrides the displayed label in
the left nav only.

Aliases are a pure presentation concern (no daemon round-trip): the override is
applied at the frontend render site, following the existing client-only
per-entity prefs already in the tree (`workNavOrder` order/hidden state,
`terminalPrefs`). This ticket also introduces the small **scope-tagged prefs
registry** those existing stores fold into, because the alias store is the third
such store and this is the cheap moment to standardize before more accumulate.

## Decisions

Settled during discussion:

- **Storage: client-side localStorage.** Aliases are presentation-only; the
  daemon already computes labels from basenames and the frontend renders them
  verbatim, so no daemon change is needed. Follows the `terminalPrefs` /
  `workNavOrder` precedent (`load/saveNamespacedPrefs`, versioned
  `{version, value}` envelope in `settingsStore.ts`).
  - *Rejected:* server-side alias storage. Only justified if aliases must sync
    across browsers/devices; not required for the current single-host dogfood.
    Cross-device sync is deferred to the separate prefs-portability idea ticket.
- **Key: `serverScopedIdentity(serverId, entityId)`.** Both workspaces and
  workroots have stable path-hash ids (`workspace-local-{hash}` /
  `root-local-{hash}`, `discovery.rs`). Keying on `serverScopedIdentity` exactly
  matches the existing `workNavOrder` / `hiddenWorktreesByWorkspace` convention
  (`workNavOrder.ts:30`, `App.tsx:7139`), so aliases stay consistent with
  reorder/hide state and are naturally per-server. Rename-tolerance is identical
  to the rest of the nav (a directory move re-keys everything uniformly).
- **Two maps:** `workspaceAliasByKey` and `workRootAliasByKey`. Both workspace
  and workroot labels are independently aliasable (user requirement).
- **Apply at the nav-row render sites ONLY, never by mutating the model label.**
  Override sites are the three `ResourceRow` `title` expressions in
  `WorkspaceRows`: `compactWorkspaceWorkRootTitle` (`App.tsx:7167`),
  `workspace.label` (`App.tsx:7207`), `root.label` (`App.tsx:7236`). Server rows
  are out of scope (user requested workspace/workroot only).
  - **Hard constraint — breadcrumb excluded.** The top breadcrumb
    (`WorkbenchToolbar`, `App.tsx:6452-6456`) reads model `.label` inline and
    shares no label helper with the nav rows, so applying the override at the
    nav-row `title` prop excludes the breadcrumb for free. Mutating the model
    `.label` would leak the alias into the breadcrumb and is therefore forbidden.
- **Edit UX: inline rename via the existing kebab ("...") menu, plus a
  right-click redirect.**
  - Add a **"Rename"** item to the existing per-row overflow menus (workspace
    menu `App.tsx:7544-7636`; worktree menu `App.tsx:7637-7699`).
  - Redirect right-click on the nav row to open that same menu:
    `onContextMenu` on the `ResourceRow` root div (`App.tsx:7431`) calls
    `preventDefault()` + `setMenuOpen(true)`. No existing `onContextMenu` handler
    exists anywhere in the frontend, so there is no collision; today right-click
    falls through to the browser/system menu, which this replaces.
  - Selecting Rename turns the row label into an inline text input seeded with
    the current effective label. Commit on **Enter or blur**, cancel on
    **Escape** (reuse `useDismissableMenu` for Escape). An **empty** committed
    value clears the alias (reverts to the derived label). No inline-edit
    component exists yet; build a small one.
  - *Rejected:* a Settings-panel "Aliases" section listing all entities. Inline
    rename is more discoverable and edits at the target row. *Rejected for v1:*
    cursor-coordinate menu anchoring — the kebab menu is CSS-anchored to the
    button (`top:100%; right:0`), so a right-click opens the menu at the button,
    not the pointer; acceptable for v1, coordinate anchoring is extra work.
- **Compact-row rename target:** in a single-root (compact) workspace row, rename
  targets the **workRoot** alias (the physical directory the row represents).

## Constraints

- Do not mutate view-model `.label` (breadcrumb-leak; see above).
- Aliases are host-scoped: keyed on local path-derived, server-scoped ids. They
  are meaningless on another machine and must be treated as `scope: "host"` in
  the prefs registry (below), so future export/sync ignores or host-gates them.
- Preserve existing kebab-menu behavior and `useDismissableMenu` dismissal.

## Prior Art

- `workNavOrder.ts` — closest structural precedent: per-entity state
  (`Record<string, ...>`) keyed by `serverScopedIdentity`, loaded via a
  `useState` initializer (`App.tsx:512`), threaded through
  `ResourceNavigation`/`ServerRows`/`WorkspaceRows`. The alias store mirrors this
  shape and threading.
- `terminalPrefs.ts` — versioned localStorage store with custom parse/defaults
  and a live-fanout React context; template for the registry descriptor.
- `settingsStore.ts` — `loadNamespacedPrefs` / `saveNamespacedPrefs` (versioned
  envelope) and the `SettingsSectionDescriptor` registry pattern the prefs
  registry parallels.

## Phases

### Phase 1: Scope-tagged prefs registry

Introduce a small registry where each client pref store declares
`{ key, version, scope: "portable" | "host", parse, defaults }` and is registered
in one place (sibling to `SettingsSectionDescriptor`). Fold the two existing
stores into it: `terminalPrefs` → `scope: "portable"` (machine-independent,
sync-worthy), `workNavOrder` → `scope: "host"` (path-derived server-scoped keys,
PC-bound).

This phase is internal restructuring with no intended caller-visible behavior
change: existing prefs continue to load/save identically (same localStorage keys,
same versions). The registry exists to (a) give the alias store a home and
(b) unblock the deferred export/import + sync idea, which serializes registered
stores and gates `host`-scoped ones to the matching machine.

Verification boundary: existing terminal-style and nav-order/hidden prefs still
persist and rehydrate across reload exactly as before; no visual/behavioral diff.

### Phase 2: Alias store + inline rename UI

Add the `ws-dashboard.workNavAliases.v1` store (`scope: "host"`) holding
`{ workspaceAliasByKey, workRootAliasByKey }`, loaded via a `useState`
initializer like `workNavOrder` and threaded to `WorkspaceRows`. Apply the
`alias ?? derivedLabel` override at the three nav-row `title` sites only. Add the
"Rename" kebab item + right-click redirect + inline-edit affordance
(Enter/blur commit, Escape cancel, empty clears). Confirm the breadcrumb is
unaffected.

Depends on Phase 1 (uses the registry to register the alias store).

Verification boundary: setting an alias renames only the left nav row (both
workspace and workroot cases, including compact rows); the top breadcrumb still
shows the derived label; alias persists across reload; empty commit reverts to
derived label; reorder/hide state remains keyed consistently.

## Spec Impact

Target spec area: none in the workflow spec set — this is downstream ws-dashboard
UI behavior with no workflow-system contract. Expected caller-visible change is
confined to the dashboard nav render (alias override) and a new client-only
localStorage key. Closeout documentation (mental-model `ws-web-dashboard`) may be
updated post-implementation if the nav-prefs surface grows.

Contract-first spec: no.
