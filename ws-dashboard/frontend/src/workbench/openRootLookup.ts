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

// Pure "ensure open" step - the mount-side counterpart to
// `resolveClosedWorkRootRefs` above. Given the current `openWorkRootKeys`
// snapshot, adds `rootKey` if it is not already present (append, never
// reorder or drop existing keys); a no-op returns the same array reference so
// callers can skip a redundant `setState`.
//
// Shared by two call sites that must never drift apart (260714
// select-mount-gap fix): the `workbenchSelection` effect in `App.tsx` (which
// mounts a newly-selected root the render *after* selection changes) and the
// `resource.select` command handler's server-switch fast path (which mounts
// the selected root *synchronously in the same commit* as the server switch,
// so the render that flips `selectedServerId`/`selectedId` together already
// has the root mounted - see `resolveEffectiveActiveRootKey` above, whose
// first branch requires `selectedRootIsMounted` to be true on that same
// render or it falls through to the server-scoped guard with a stale
// remembered server and hides every instance for one frame).
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
