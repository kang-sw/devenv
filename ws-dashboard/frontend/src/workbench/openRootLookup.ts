import type {
  DashboardResourcesView,
  InstanceView,
  WorkRootView,
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

// Pure decision behind the 260714 childroot-fix safety net in
// `WorkbenchShell`: which mounted rootKey (if any) should render as "active"
// this frame. When the current selection genuinely matches a mounted
// instance, that wins outright. Otherwise fall back to the last rootKey that
// *did* genuinely match — but only when that remembered root belongs to the
// currently selected server. Without the server-scope guard, selecting a
// remote server that has never resolved (so `resources`/`selection` collapse
// to `null` and nothing reloads) would re-pin the *previous* server's
// last-active root, leaking its mounted (keep-alive) panes under the new
// server's header instead of showing the empty-state watermark.
export function resolveEffectiveActiveRootKey(params: {
  selectedRootKey: string | null;
  selectedRootIsMounted: boolean;
  lastActiveRootKey: string | null;
  lastActiveRootServerId: string | null;
  selectedServerId: string;
}): string | null {
  if (params.selectedRootKey && params.selectedRootIsMounted) {
    return params.selectedRootKey;
  }
  if (params.lastActiveRootServerId === params.selectedServerId) {
    return params.lastActiveRootKey;
  }
  return null;
}
