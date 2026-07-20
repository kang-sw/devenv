import {
  resolveActiveResources,
  resolveStickyWorkbenchSelection,
  serverScopedIdentity,
  type DashboardResourcesView,
  type InstanceView,
  type LastMatchedSelectionByServer,
  type ResourcesByServer,
  type WorkbenchSelection,
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

// The exact inputs that produced a given `resolveStickyWorkbenchSelection`
// result, compared by reference/value equality in `driveStickyWorkbenchSelection`
// below to detect a redundant re-invocation over the SAME poll input (see
// that function's doc comment). `activeResources` is compared by object
// reference deliberately: `mergeResourcesByServer` (resourceModel.ts)
// allocates a fresh `DashboardResourcesView` object per poll response, so an
// unchanged reference reliably means "no new poll landed since the last
// invocation", not merely "structurally equal".
export type StickySelectionRenderKey = {
  activeResources: DashboardResourcesView | null;
  selectedId: string | null;
  selectedServerId: string;
};

// Carries the sticky cache PLUS the last render key/selection it was
// computed from, across renders (owned by a single ref at the `App.tsx` call
// site - see `driveStickyWorkbenchSelection`'s doc comment for why this must
// be one bundle rather than a bare `LastMatchedSelectionByServer`).
export type StickySelectionDriverState = {
  lastMatchedSelectionByServer: LastMatchedSelectionByServer;
  lastRenderKey: StickySelectionRenderKey | null;
  lastSelection: WorkbenchSelection | null;
};

export const initialStickySelectionDriverState: StickySelectionDriverState = {
  lastMatchedSelectionByServer: {},
  lastRenderKey: null,
  lastSelection: null,
};

// 260714 Phase 2 Prong 1 correctness fix. `resolveStickyWorkbenchSelection`
// is a NON-idempotent read-and-advance state machine over
// `LastMatchedSelectionByServer`: a first miss bridges (`bridged: false ->
// true`), a second consecutive miss with the SAME inputs drops the entry and
// falls through to the fallback. That transition is meant to count in POLL
// cycles (bridge for one omitted poll, expire on a second), but React can
// invoke the component body that owns the driving ref more than once for the
// exact same poll input - deterministically under StrictMode's double-invoke
// in development (the ref persists across both invocations), and in
// production whenever an unrelated App-level state change triggers a
// re-render before the next poll response lands. Calling
// `resolveStickyWorkbenchSelection` directly once per render invocation
// therefore advances the bridge/drop transition once per RENDER rather than
// once per POLL, collapsing the intended one-poll bridge to zero protection
// under StrictMode (inv1 bridges, inv2 immediately sees the already-bridged
// cache and treats it as a second miss) - unlike the D5 pattern
// (`withLastNonNullResourcesByServer`/`resolveActiveResources`) this is
// modeled on, which is exactly idempotent under repeat renders because
// re-storing the same last-good value any number of times yields the same
// cache.
//
// This driver restores that idempotency: it only calls
// `resolveStickyWorkbenchSelection` (and only commits its
// `nextLastMatchedSelectionByServer`) when `renderKey` differs from the key
// the driver state was last advanced with. A repeat invocation with the
// identical `renderKey` (same `activeResources` object reference, same
// `selectedId`, same `selectedServerId`) returns the previously memoized
// `selection` untouched - safe to call any number of times per poll,
// including from a genuinely new render triggered by an unrelated state
// change, as long as none of the three key fields changed.
export function driveStickyWorkbenchSelection(
  renderKey: StickySelectionRenderKey,
  driverState: StickySelectionDriverState,
): {
  selection: WorkbenchSelection | null;
  nextDriverState: StickySelectionDriverState;
} {
  const { lastRenderKey } = driverState;
  if (
    lastRenderKey &&
    lastRenderKey.activeResources === renderKey.activeResources &&
    lastRenderKey.selectedId === renderKey.selectedId &&
    lastRenderKey.selectedServerId === renderKey.selectedServerId
  ) {
    return { selection: driverState.lastSelection, nextDriverState: driverState };
  }

  const { selection, nextLastMatchedSelectionByServer } =
    resolveStickyWorkbenchSelection(
      renderKey.activeResources,
      renderKey.selectedId,
      renderKey.selectedServerId,
      driverState.lastMatchedSelectionByServer,
    );
  return {
    selection,
    nextDriverState: {
      lastMatchedSelectionByServer: nextLastMatchedSelectionByServer,
      lastRenderKey: renderKey,
      lastSelection: selection,
    },
  };
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
// - `selection` is NOT recomputed here (260714 Phase 2 Prong 1 correctness
//   fix): unlike `activeResources`/`resolveActiveResources`, resolving the
//   sticky selection is a non-idempotent operation across repeat render
//   invocations (see `driveStickyWorkbenchSelection`'s doc comment) and so
//   must be driven exactly once per render at the single ref-owning call
//   site (`App.tsx`) and passed in here as an already-resolved value, rather
//   than re-derived from a cache that this function has no safe way to
//   avoid re-advancing. This is the one exception to the "no cross-render
//   refs" contract line above: the exception is pushed entirely onto the
//   caller, which is the only place a ref can safely live.
// - `selectedRootKey` is `selection`'s root key, or `null` when no selection
//   resolves.
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
  selection: WorkbenchSelection | null;
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
  const selection = state.selection;
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
