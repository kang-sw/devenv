import {
  resolveActiveResources,
  resolveWorkbenchSelection,
  serverScopedIdentity,
  type DashboardResourcesView,
  type InstanceView,
  type ResourcesByServer,
  type WorkRootView,
} from "../resourceModel.js";

// Re-resolves a previously-opened work root against the current resources
// snapshot without depending on `selectedId`/tree-walk selection state. Used
// to keep every mounted per-root `DockviewWorkbenchLayout` instance in sync
// with live resource updates even while a different root is selected.
export function findOpenWorkRoot(
  resources: DashboardResourcesView | null,
  ref: { rootId: string; serverRoute: string },
): { root: WorkRootView; mainInstance: InstanceView | null } | null {
  if (!resources) {
    return null;
  }
  for (const workspace of resources.workspaces) {
    for (const root of workspace.workRoots) {
      if (
        root.id === ref.rootId &&
        root.resourcePath.serverId === ref.serverRoute
      ) {
        return { root, mainInstance: root.mainInstances[0] ?? null };
      }
    }
  }
  return null;
}

// Pure key-diff step at the center of the close-triggered cleanup effect in
// `WorkbenchShell`: given the previous render's `openWorkRootKeys` snapshot
// (and the `openWorkRootRefs` snapshot taken at the same time, since a close
// command removes an entry from both in the same React commit) plus the
// current `openWorkRootKeys`, resolve which rootKeys just closed and what
// `{rootId, serverRoute}` each one pointed at. Extracted so the snapshot-
// ordering fix (advance both refs together, diff against the *previous*
// refs) is unit-testable without a DOM/React-effect harness.
export function resolveClosedWorkRootRefs(
  previousKeys: readonly string[],
  previousRefs: Record<string, { rootId: string; serverRoute: string }>,
  currentKeys: readonly string[],
): Array<{ rootKey: string; rootId: string; serverRoute: string }> {
  const currentKeySet = new Set(currentKeys);
  const closedRefs: Array<{
    rootKey: string;
    rootId: string;
    serverRoute: string;
  }> = [];
  for (const rootKey of previousKeys) {
    if (currentKeySet.has(rootKey)) {
      continue;
    }
    const ref = previousRefs[rootKey];
    if (!ref) {
      continue;
    }
    closedRefs.push({ rootKey, rootId: ref.rootId, serverRoute: ref.serverRoute });
  }
  return closedRefs;
}

// Pure "ensure open" step - the mount-side counterpart to
// `resolveClosedWorkRootRefs` above. Given the current `openWorkRootKeys`
// snapshot, adds `rootKey` if it is not already present (append, never
// reorder or drop existing keys); a no-op returns the same array reference so
// callers can skip a redundant `setState`.
//
// Shared by the state-seeding call sites that must never drift apart (260714
// select-mount-gap fix): the `workbenchSelection` effect in `App.tsx` (which
// mounts a newly-selected root the render *after* selection changes) and the
// `resource.select` command handler's server-switch fast path (which mounts
// the selected root *synchronously in the same commit* as the server switch).
// Also the primitive `deriveWorkbenchView` (below) uses to fold the freshly-
// resolved selected root into `openWorkRootKeys` at render time, so the
// selected root is mounted by construction on the same render its selection
// resolves rather than one render later.
export function withOpenWorkRootKey(
  openWorkRootKeys: readonly string[],
  rootKey: string,
): string[] {
  return openWorkRootKeys.includes(rootKey)
    ? (openWorkRootKeys as string[])
    : [...openWorkRootKeys, rootKey];
}

// Pure "ensure open" step for the `openWorkRootRefs` side: seeds
// `{rootId, serverRoute}` for `rootKey` only if absent, never clobbering an
// already-open root's ref. Paired with `withOpenWorkRootKey` above at both
// shared call sites.
export function withOpenWorkRootRef(
  openWorkRootRefs: Readonly<Record<string, { rootId: string; serverRoute: string }>>,
  rootKey: string,
  ref: { rootId: string; serverRoute: string },
): Record<string, { rootId: string; serverRoute: string }> {
  return openWorkRootRefs[rootKey]
    ? (openWorkRootRefs as Record<string, { rootId: string; serverRoute: string }>)
    : { ...openWorkRootRefs, [rootKey]: ref };
}

// CONTRACT: the pure active-root derivation seam (260714 active-root
// derivation refactor Phase 1, D6). Given the exact committed-state slice a
// single render sees, derive everything the workbench active-root render
// needs, with NO dependency on cross-render refs. This subsumes the old
// `resolveEffectiveActiveRootKey` safety net (deleted with its
// `lastActiveRootKey*` refs): the render-time union below mounts the selected
// root by construction, so the transient "selected but not yet mounted" gap
// the safety net existed to paper over can no longer occur.
//
// - `activeResources` resolves through the same fallback-bearing
//   `resolveActiveResources` path as the live view (D2), so a one-render gap
//   in `resourcesByServer` for the selected server does not collapse the
//   selection.
// - `selectedRootKey` is the freshly-resolved selection's root key, or `null`
//   when no selection resolves.
// - `openInstanceKeys` folds the selected root's key into the persisted
//   `openWorkRootKeys` via `withOpenWorkRootKey` (append-if-absent,
//   position-preserving; never filter-then-append, D1), so keep-alive members
//   survive and never reorder.
// - `effectiveActiveRootKey` is pure: the selected root's key, else `null`
//   (D3).
//
// `openWorkRootRefs` is part of the committed-state slice (D6) for contract
// completeness; the selected entry resolves via `selection` rather than that
// lagging ref map, so it is not read here.
export function deriveWorkbenchView(state: {
  resourcesByServer: ResourcesByServer;
  lastNonNullResourcesByServer: ResourcesByServer;
  selectedServerId: string;
  selectedId: string | null;
  openWorkRootKeys: readonly string[];
  openWorkRootRefs: Record<string, { rootId: string; serverRoute: string }>;
}): {
  activeResources: DashboardResourcesView | null;
  selectedRootKey: string | null;
  openInstanceKeys: string[];
  effectiveActiveRootKey: string | null;
} {
  const activeResources = resolveActiveResources(
    state.resourcesByServer,
    state.selectedServerId,
    state.lastNonNullResourcesByServer,
  );
  const selection = resolveWorkbenchSelection(activeResources, state.selectedId);
  const selectedRootKey = selection
    ? serverScopedIdentity(
        selection.root.resourcePath.serverId,
        selection.root.id,
      )
    : null;
  const openInstanceKeys = selectedRootKey
    ? withOpenWorkRootKey(state.openWorkRootKeys, selectedRootKey)
    : [...state.openWorkRootKeys];
  return {
    activeResources,
    selectedRootKey,
    openInstanceKeys,
    effectiveActiveRootKey: selectedRootKey,
  };
}
