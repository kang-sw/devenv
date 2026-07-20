// Pure dashboard resource view-model: the JSON contract returned by
// GET /api/dashboard/resources plus the flattened navigation entity model.
//
// This module is intentionally free of React and DOM dependencies so the
// mock-fixture -> live opened-workRoot transition is testable as pure logic.

export type ViewState = {
  status: string;
  loading: boolean;
  stale: boolean;
  error: string | null;
};

export type ActionHint = {
  id: string;
  label: string;
  enabled: boolean;
};

export const LOCAL_DASHBOARD_SERVER_ROUTE = "server-local";

// `serverId` is the ResourcePath wire field name; it carries the selected
// Server Route, so this structural target reads it to resolve a route.
export type ServerScopedRouteTarget = { readonly serverId?: string | null };

export function dashboardServerRoute(
  target:
    | string
    | ServerScopedRouteTarget
    | null
    | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
): string {
  if (typeof target === "string") {
    return target.trim() || LOCAL_DASHBOARD_SERVER_ROUTE;
  }
  return target?.serverId?.trim() || LOCAL_DASHBOARD_SERVER_ROUTE;
}

export function isLocalDashboardServerRoute(
  serverRoute: string | null | undefined,
): boolean {
  return dashboardServerRoute(serverRoute) === LOCAL_DASHBOARD_SERVER_ROUTE;
}

export function dashboardApiRoute(segments: readonly string[]): string {
  return `/api/dashboard/${segments.map((segment) => encodeURIComponent(segment)).join("/")}`;
}

// Dot is reserved as a future hop separator, so a single Server Route segment
// must be dot-free. `server-local` and generated slugs already satisfy this;
// this guard rejects dotted routes reaching the canonical builder.
const SERVER_ROUTE_SEGMENT = /^[A-Za-z0-9_-]+$/;

export function isValidServerRouteSegment(serverRoute: string): boolean {
  return SERVER_ROUTE_SEGMENT.test(serverRoute);
}

export function dashboardServerApiRoute(
  serverRoute: string,
  segments: readonly string[],
): string {
  const route = dashboardServerRoute(serverRoute);
  if (!isValidServerRouteSegment(route)) {
    throw new Error(
      `invalid server route segment: ${route} (dot is reserved as a hop separator)`,
    );
  }
  return dashboardApiRoute(["servers", route, ...segments]);
}

export function localCompatibleDashboardApiRoute(
  serverRoute: string | null | undefined,
  localSegments: readonly string[],
  serverSegments: readonly string[] = localSegments,
): string {
  return isLocalDashboardServerRoute(serverRoute)
    ? dashboardApiRoute(localSegments)
    : dashboardServerApiRoute(dashboardServerRoute(serverRoute), serverSegments);
}

export function serverScopedIdentity(
  serverRoute: string | null | undefined,
  ...parts: readonly string[]
): string {
  return [dashboardServerRoute(serverRoute), ...parts].join("/");
}

export function workRootActivationEndpoint(
  workRootId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
): string {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "work-roots",
    workRootId,
    "activation",
  ]);
}

export function workspaceEndpoint(
  workspaceId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
): string {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "workspaces",
    workspaceId,
  ]);
}

export type ResourcePath = {
  // Wire field name retained for compatibility; the value is a Server Route.
  serverId: string;
  workspaceId: string;
  workRootId: string;
  instanceId: string | null;
};

export type ServerView = {
  id: string;
  label: string;
  state: ViewState;
  actions: ActionHint[];
};

export type ServerConnectionView = {
  id: string;
  label: string;
  kind: "local" | "sshRemote" | "wsl" | "manual";
  status:
    | "connected"
    | "authRequired"
    | "unreachable"
    | "starting"
    | "staleEndpoint"
    | "tunnelRequired";
  state: ViewState;
  actions: ActionHint[];
};

export type WorkspaceView = {
  id: string;
  label: string;
  state: ViewState;
  compactable: boolean;
  workRoots: WorkRootView[];
  actions: ActionHint[];
};

export type WorkRootView = {
  id: string;
  resourcePath: ResourcePath;
  label: string;
  kind: "plainDirectory" | "gitPrimaryRoot" | "gitLinkedWorktree";
  activation: "online" | "offline";
  availability: "available" | "missing" | "moved" | "inaccessible" | "unknown";
  status: "online" | "offline" | "moved" | "inaccessible";
  state: ViewState;
  compactable: boolean;
  mainInstances: InstanceView[];
  actions: ActionHint[];
};

export type InstanceView = {
  id: string;
  resourcePath: ResourcePath;
  role: "main" | "sub";
  kind:
    | "harness"
    | "agent"
    | "terminal"
    | "editor"
    | "viewer"
    | "exec"
    | "translation"
    | "task";
  interactionMode: "direct" | "delegated" | "passive";
  label: string;
  state: ViewState;
  subInstances: InstanceView[];
  actions: ActionHint[];
};

export type DashboardResourcesView = {
  server: ServerView;
  workspaces: WorkspaceView[];
};

// Per-server resources cache keyed by `server.id`, as accumulated across the
// multi-server keep-alive session (260714 Phase 1): each On server's last
// fetch stays in this map so a hidden server's already-mounted work-root
// panes keep resolving against its own cached view while a different server
// is being fetched/focused.
export type ResourcesByServer = Record<string, DashboardResourcesView>;

// Merge a freshly-fetched server's resources into the per-server cache.
//
// CONTRACT: accumulate, don't replace - only the fetched server's own entry
// is added/overwritten; every other cached server's entry must survive
// untouched. This is the crux mechanic that lets a focus switch keep hidden
// servers' mounted work-root panes resolvable instead of unmounting them.
// Extracted from the `applyResources` closure passed to
// `createResourceRefreshCoordinator` (mirrors `resolveClosedWorkRootRefs` in
// `workbench/openRootLookup.ts`) so this property is unit-testable without a
// DOM/React harness.
export function mergeResourcesByServer(
  current: ResourcesByServer,
  resources: DashboardResourcesView,
): ResourcesByServer {
  return { ...current, [resources.server.id]: resources };
}

// Removes a single server's entry from the per-server cache (260714 Phase 2:
// the "Off" deallocation gesture). Returns the same reference when the
// server has no entry, matching `mergeResourcesByServer`'s no-surprises
// style; every other cached server's entry is left untouched.
export function removeResourcesByServer(
  current: ResourcesByServer,
  serverId: string,
): ResourcesByServer {
  if (!(serverId in current)) {
    return current;
  }
  const next = { ...current };
  delete next[serverId];
  return next;
}

// Keeps the last non-null resources seen per server id, so a transient gap
// in `resourcesByServer` for the *currently selected* server (260714
// childroot-fix: e.g. one render between `selectedServerId` advancing and
// the matching entry landing in `resourcesByServer`) doesn't have to
// collapse the active selection to null. Only ever records an entry once
// that server has resolved at least once - a server that has never
// resolved still has nothing to fall back to, so this cannot resurrect a
// server ahead of its first real fetch or after `resourcesByServer` itself
// never gained an entry for it.
export function withLastNonNullResourcesByServer(
  current: ResourcesByServer,
  selectedServerId: string,
  freshActiveResources: DashboardResourcesView | null,
): ResourcesByServer {
  if (!freshActiveResources || current[selectedServerId] === freshActiveResources) {
    return current;
  }
  return { ...current, [selectedServerId]: freshActiveResources };
}

// Resolves the active server's resources, falling back to the last
// non-null resources cached for that server (see
// `withLastNonNullResourcesByServer`) when `resourcesByServer` itself has no
// entry for it on this render.
export function resolveActiveResources(
  resourcesByServer: ResourcesByServer,
  selectedServerId: string,
  lastNonNullResourcesByServer: ResourcesByServer,
): DashboardResourcesView | null {
  return (
    resourcesByServer[selectedServerId] ??
    lastNonNullResourcesByServer[selectedServerId] ??
    null
  );
}

// Server-scoped sticky-selection cache (260714 Phase 2 Prong 1), modeled
// directly on the vetted D5 `withLastNonNullResourcesByServer`/
// `resolveActiveResources` shape above. This bridges a narrower gap than D5:
// D5 covers "the whole resource tree for this server is momentarily absent";
// this covers "the tree is present, but the previously-selected root's own
// entry is momentarily missing from it" (e.g. a `gitLinkedWorktree` child
// dropped from one poll response). `bridged` tracks whether this entry has
// already been used to paper over one miss, so a second consecutive miss for
// the same `selectedId` is treated as a genuine removal rather than bridged
// again indefinitely.
export type LastMatchedSelectionByServer = Record<
  string,
  { selectedId: string; selection: WorkbenchSelection; bridged: boolean }
>;

// Resolves `selectedId` against `resources`, bridging a single transient
// omission of the previously-matched selection using a server-scoped cache
// (see `LastMatchedSelectionByServer` above). Pure - no `useRef`/`useState`;
// callers own advancing the returned `nextLastMatchedSelectionByServer` into
// whatever storage they use across calls (mirrors how `resolveActiveResources`
// callers own advancing `lastNonNullResourcesByServer`).
//
// Deliberately does NOT engage when `resources` itself is `null` - that case
// is already fully owned by D2 (slot-level fallback)/D5 (`resolveActiveResources`)
// upstream of this function; bridging here would double up on stickiness
// already applied to the tree itself.
export function resolveStickyWorkbenchSelection(
  resources: DashboardResourcesView | null,
  selectedId: string | null,
  selectedServerId: string,
  lastMatchedSelectionByServer: LastMatchedSelectionByServer,
): {
  selection: WorkbenchSelection | null;
  nextLastMatchedSelectionByServer: LastMatchedSelectionByServer;
} {
  if (!resources) {
    return {
      selection: null,
      nextLastMatchedSelectionByServer: lastMatchedSelectionByServer,
    };
  }

  const { selection, matched } = resolveWorkbenchSelectionWithMatch(
    resources,
    selectedId,
  );

  if (matched && selectedId && selection) {
    return {
      selection,
      nextLastMatchedSelectionByServer: {
        ...lastMatchedSelectionByServer,
        [selectedServerId]: { selectedId, selection, bridged: false },
      },
    };
  }

  const cached = lastMatchedSelectionByServer[selectedServerId];
  if (cached && selectedId && cached.selectedId === selectedId) {
    if (!cached.bridged) {
      // First consecutive miss for this selection: bridge over it with the
      // last-matched selection, and mark the entry bridged so a second
      // consecutive miss (below) is treated as a genuine removal.
      return {
        selection: cached.selection,
        nextLastMatchedSelectionByServer: {
          ...lastMatchedSelectionByServer,
          [selectedServerId]: { ...cached, bridged: true },
        },
      };
    }

    // Second consecutive miss: genuine removal, not a transient omission.
    // Drop the cache entry and fall through to the fresh (non-sticky)
    // selection.
    const next = { ...lastMatchedSelectionByServer };
    delete next[selectedServerId];
    return { selection, nextLastMatchedSelectionByServer: next };
  }

  // No cache entry for this server, or `selectedId` itself changed (a real
  // user re-selection) - nothing to bridge, cache unchanged.
  return { selection, nextLastMatchedSelectionByServer: lastMatchedSelectionByServer };
}

export type DashboardServersView = {
  servers: ServerConnectionView[];
};

export type ResourceEntity =
  | {
      id: string;
      type: "server";
      label: string;
      state: ViewState;
      actions: ActionHint[];
    }
  | {
      id: string;
      type: "workspace";
      label: string;
      state: ViewState;
      actions: ActionHint[];
      compactable: boolean;
      workRootCount: number;
    }
  | {
      id: string;
      type: "workRoot";
      label: string;
      state: ViewState;
      actions: ActionHint[];
      compactable: boolean;
      path: ResourcePath;
      kind: WorkRootView["kind"];
      activation: WorkRootView["activation"];
      availability: WorkRootView["availability"];
      status: WorkRootView["status"];
      instanceCount: number;
    }
  | {
      id: string;
      type: "instance";
      label: string;
      state: ViewState;
      actions: ActionHint[];
      path: ResourcePath;
      role: InstanceView["role"];
      kind: InstanceView["kind"];
      interactionMode: InstanceView["interactionMode"];
      subInstanceCount: number;
    };

// A workspace containing exactly one workRoot is rendered as one compact
// workRoot row in the browser left nav. The selected location remains the
// concrete workRoot id; main/sub instances are workbench surfaces and do not
// participate in this presentation decision.
export function compactWorkspaceWorkRoot(
  workspace: WorkspaceView,
): WorkRootView | null {
  if (workspace.workRoots.length !== 1) {
    return null;
  }

  return workspace.workRoots[0];
}

export function compactWorkspaceWorkRootTitle(
  workspace: WorkspaceView,
  workRoot: WorkRootView,
): string {
  if (workspace.label === workRoot.label) {
    return workspace.label;
  }

  return `${workspace.label} / ${workRoot.label}`;
}

// Flatten the resource hierarchy into the left-nav entity rows.
//
// Main and sub instances are workbench surfaces/projections, not default
// left-nav rows, so they are intentionally omitted here.
export function flattenEntities(
  resources: DashboardResourcesView | null,
): ResourceEntity[] {
  if (!resources) {
    return [];
  }

  const entities: ResourceEntity[] = [
    {
      id: resources.server.id,
      type: "server",
      label: resources.server.label,
      state: resources.server.state,
      actions: resources.server.actions,
    },
  ];

  for (const workspace of resources.workspaces) {
    entities.push({
      id: workspace.id,
      type: "workspace",
      label: workspace.label,
      state: workspace.state,
      actions: workspace.actions,
      compactable: workspace.compactable,
      workRootCount: workspace.workRoots.length,
    });

    for (const root of workspace.workRoots) {
      entities.push({
        id: root.id,
        type: "workRoot",
        label: root.label,
        state: root.state,
        actions: root.actions,
        compactable: root.compactable,
        path: root.resourcePath,
        kind: root.kind,
        activation: root.activation,
        availability: root.availability,
        status: root.status,
        instanceCount: root.mainInstances.length,
      });
    }
  }

  return entities;
}

// The entity that should be selected by default: the first workRoot row, or
// the first entity (the server) when no workRoot exists yet. Returns undefined
// only for an empty entity list.
export function preferredSelection(
  entities: ResourceEntity[],
): string | undefined {
  return (
    entities.find((entity) => entity.type === "workRoot")?.id ?? entities[0]?.id
  );
}

// Reconcile the active selection after the resource tree changes.
//
// Keeps a still-present selection, otherwise falls back to `preferredSelection`
// so a selection that left the entity set (for example the mock workspace
// after the tree turns live) cannot remain active.
export function reconcileSelectedId(
  entities: ResourceEntity[],
  selectedId: string | null,
): string | null {
  if (entities.length === 0) {
    return selectedId;
  }

  if (selectedId && entities.some((entity) => entity.id === selectedId)) {
    return selectedId;
  }

  return preferredSelection(entities) ?? null;
}

// The workbench selection resolved against a resource tree: the workspace +
// work root + main/selected instance that the current `selectedId` points at.
// Relocated verbatim from `App.tsx` (260714 active-root derivation refactor
// Phase 1) so `deriveWorkbenchView` in `workbench/openRootLookup.ts` can reuse
// the same tree-walk without importing back from the React component module -
// that back-import would create a circular module dependency. Pure, free of
// React/DOM dependencies like the rest of this module.
export type WorkbenchSelection = {
  workspace: WorkspaceView;
  root: WorkRootView;
  mainInstance: InstanceView | null;
  selectedInstance: InstanceView | null;
};

// A `gitLinkedWorktree` work root is presented as a child row under its
// workspace in the left nav rather than as the workspace's default selection
// target, so it is skipped when resolving a workspace row's default root.
export function isWorkspaceNavChildWorkRoot(root: WorkRootView): boolean {
  return root.kind === "gitLinkedWorktree";
}

// Result of walking the resource tree for `selectedId`: `matched` is true
// only when `selectedId` was found exactly (workspace id, root id, or nested
// instance id); a `selection` returned with `matched: false` fell through to
// `fallback` (the first root walked). Callers that need to distinguish "hit"
// from "fallback" (260714 Phase 2 Prong 1 sticky selection) use
// `resolveWorkbenchSelectionWithMatch`; the plain `resolveWorkbenchSelection`
// below stays a thin wrapper so its existing callers/behavior are unaffected.
function resolveWorkbenchSelectionWithMatchInternal(
  resources: DashboardResourcesView | null,
  selectedId: string | null,
): { selection: WorkbenchSelection | null; matched: boolean } {
  if (!resources) {
    return { selection: null, matched: false };
  }

  let fallback: WorkbenchSelection | null = null;

  for (const workspace of resources.workspaces) {
    const workspaceRoot =
      workspace.workRoots.find((root) => !isWorkspaceNavChildWorkRoot(root)) ??
      workspace.workRoots[0] ??
      null;
    if (selectedId === workspace.id && workspaceRoot) {
      const mainInstance = workspaceRoot.mainInstances[0] ?? null;
      return {
        selection: {
          workspace,
          root: workspaceRoot,
          mainInstance,
          selectedInstance: mainInstance,
        },
        matched: true,
      };
    }

    for (const root of workspace.workRoots) {
      const mainInstance = root.mainInstances[0] ?? null;
      const rootSelection = {
        workspace,
        root,
        mainInstance,
        selectedInstance: mainInstance,
      };
      fallback ??= rootSelection;

      if (selectedId === root.id) {
        return { selection: rootSelection, matched: true };
      }

      for (const main of root.mainInstances) {
        const selectedInstance = findInstanceById(main, selectedId);
        if (selectedInstance) {
          return {
            selection: { workspace, root, mainInstance: main, selectedInstance },
            matched: true,
          };
        }
      }
    }
  }

  return { selection: fallback, matched: false };
}

export function resolveWorkbenchSelectionWithMatch(
  resources: DashboardResourcesView | null,
  selectedId: string | null,
): { selection: WorkbenchSelection | null; matched: boolean } {
  return resolveWorkbenchSelectionWithMatchInternal(resources, selectedId);
}

export function resolveWorkbenchSelection(
  resources: DashboardResourcesView | null,
  selectedId: string | null,
): WorkbenchSelection | null {
  return resolveWorkbenchSelectionWithMatchInternal(resources, selectedId).selection;
}

export function findInstanceById(
  instance: InstanceView,
  selectedId: string | null,
): InstanceView | null {
  if (!selectedId) {
    return null;
  }

  if (instance.id === selectedId) {
    return instance;
  }

  for (const subInstance of instance.subInstances) {
    const nested = findInstanceById(subInstance, selectedId);
    if (nested) {
      return nested;
    }
  }

  return null;
}
