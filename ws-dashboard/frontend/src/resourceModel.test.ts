import {
  compactWorkspaceWorkRoot,
  compactWorkspaceWorkRootTitle,
  flattenEntities,
  mergeResourcesByServer,
  preferredSelection,
  reconcileSelectedId,
  removeResourcesByServer,
  resolveActiveResources,
  resolveStickyWorkbenchSelection,
  withLastNonNullResourcesByServer,
  workRootActivationEndpoint,
  workspaceBaseWorkRoot,
  workspaceEndpoint,
  type DashboardResourcesView,
  type InstanceView,
  type LastMatchedSelectionByServer,
  type ResourcesByServer,
  type ViewState,
  type WorkRootView,
} from "./resourceModel.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertTrue(value: boolean, label: string) {
  if (!value) {
    throw new Error(`${label}: expected true`);
  }
}

function assertThrows(fn: () => unknown, label: string) {
  let threw = false;
  try {
    fn();
  } catch {
    threw = true;
  }
  if (!threw) {
    throw new Error(`${label}: expected a thrown error`);
  }
}

assertEqual(
  workRootActivationEndpoint("root-local-abc"),
  "/api/dashboard/work-roots/root-local-abc/activation",
  "activation endpoint defaults to the server-local compat route",
);
assertEqual(
  workRootActivationEndpoint("root-local-abc", "server-local"),
  "/api/dashboard/work-roots/root-local-abc/activation",
  "activation endpoint uses the server-local compat route explicitly",
);
assertEqual(
  workRootActivationEndpoint("root-a", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activation",
  "activation endpoint uses the canonical server-scoped route for a remote server",
);
assertThrows(
  () => workRootActivationEndpoint("root-a", "server.remote"),
  "activation endpoint rejects a dotted server route segment",
);

assertEqual(
  workspaceEndpoint("workspace-local-abc"),
  "/api/dashboard/workspaces/workspace-local-abc",
  "workspace endpoint defaults to the server-local compat route",
);
assertEqual(
  workspaceEndpoint("workspace-local-abc", "server-local"),
  "/api/dashboard/workspaces/workspace-local-abc",
  "workspace endpoint uses the server-local compat route explicitly",
);
assertEqual(
  workspaceEndpoint("workspace-a", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/workspaces/workspace-a",
  "workspace endpoint uses the canonical server-scoped route for a remote server",
);
assertThrows(
  () => workspaceEndpoint("workspace-a", "server.remote"),
  "workspace endpoint rejects a dotted server route segment",
);

const readyState: ViewState = { status: "ready", loading: false, stale: false, error: null };

function instance(id: string, workspaceId: string, workRootId: string): InstanceView {
  return {
    id,
    resourcePath: { serverId: "server-local", workspaceId, workRootId, instanceId: id },
    role: "main",
    kind: "harness",
    interactionMode: "direct",
    label: id,
    state: readyState,
    subInstances: [],
    actions: [],
  };
}

function workRoot(
  id: string,
  workspaceId: string,
  label: string,
  mainInstances: InstanceView[] = [],
): WorkRootView {
  return {
    id,
    resourcePath: { serverId: "server-local", workspaceId, workRootId: id, instanceId: null },
    label,
    kind: "plainDirectory",
    activation: "online",
    availability: "available",
    status: "online",
    state: readyState,
    compactable: false,
    mainInstances,
    actions: [],
  };
}

// Mock-style view: the static fixture-shaped workspace the daemon used to
// return before the live resource authority fix.
const mockView: DashboardResourcesView = {
  server: { id: "server-local", label: "Mock dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-devenv",
      label: "devenv",
      state: readyState,
      compactable: false,
      workRoots: [workRoot("root-devenv-primary", "workspace-devenv", "devenv")],
      actions: [],
    },
  ],
};

// Live view: a real opened workRoot discovered from daemon state.
const liveView: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-local-abc",
      label: "devenv",
      state: readyState,
      compactable: true,
      workRoots: [workRoot("root-local-abc", "workspace-local-abc", "devenv")],
      actions: [],
    },
  ],
};

// Empty live view: the honest pre-open live state.
const emptyLiveView: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [],
};

// flattenEntities keeps server/workspace/workRoot rows and omits instances.
const mockEntities = flattenEntities(mockView);
assertEqual(mockEntities.length, 3, "mock view flattens to server + workspace + workRoot");
assertEqual(
  preferredSelection(mockEntities),
  "root-devenv-primary",
  "mock preferred selection is the mock workRoot",
);

// flattenEntities returns an empty list for a null view (the initial-load
// production path before resources are fetched).
assertEqual(flattenEntities(null).length, 0, "null resources flatten to no entities");

// Main/sub instances are workbench projections, not left-nav rows: they are
// omitted from the flattened entities even when present, while the owning
// workRoot row still reports their count.
const instanceView: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-local-xyz",
      label: "devenv",
      state: readyState,
      compactable: false,
      workRoots: [
        workRoot("root-local-xyz", "workspace-local-xyz", "devenv", [
          instance("instance-main-a", "workspace-local-xyz", "root-local-xyz"),
          instance("instance-main-b", "workspace-local-xyz", "root-local-xyz"),
        ]),
      ],
      actions: [],
    },
  ],
};
const instanceEntities = flattenEntities(instanceView);
assertEqual(
  instanceEntities.length,
  3,
  "view with main instances still flattens to server + workspace + workRoot only",
);
assertTrue(
  !instanceEntities.some((entity) => entity.type === "instance"),
  "instance rows are omitted from the flattened entities",
);
const workRootEntity = instanceEntities.find((entity) => entity.type === "workRoot");
assertEqual(
  workRootEntity?.type === "workRoot" ? workRootEntity.instanceCount : -1,
  2,
  "workRoot entity reports its main instance count",
);
assertEqual(
  workRootEntity?.type === "workRoot" ? workRootEntity.activation : "missing",
  "online",
  "workRoot entity carries activation distinctly",
);
assertEqual(
  workRootEntity?.type === "workRoot" ? workRootEntity.availability : "missing",
  "available",
  "workRoot entity carries availability distinctly",
);

// A caller selects the mock workRoot...
const mockSelectedId = preferredSelection(mockEntities) ?? null;

// ...then the tree turns live. The mock workRoot left the entity set, so the
// selection must reconcile to the live opened workRoot.
const liveEntities = flattenEntities(liveView);
const reconciled = reconcileSelectedId(liveEntities, mockSelectedId);
assertEqual(reconciled, "root-local-abc", "selection reconciles to the live opened workRoot");
assertTrue(reconciled !== "root-devenv-primary", "mock workRoot is not retained as active selection");
assertTrue(reconciled !== "workspace-devenv", "mock workspace is not retained as active selection");
assertTrue(
  !liveEntities.some((entity) => entity.id === "workspace-devenv"),
  "live entity set drops the mock workspace",
);

// A still-present selection survives a resource refresh.
assertEqual(
  reconcileSelectedId(liveEntities, "root-local-abc"),
  "root-local-abc",
  "a still-present selection is kept",
);

// Initial-load default selection: a null selection against a non-empty entity
// set resolves to the first workRoot.
assertEqual(
  reconcileSelectedId(liveEntities, null),
  "root-local-abc",
  "a null selection defaults to the first live workRoot",
);

// Empty live view (pre-open): only the server row, and a stale mock selection
// falls back to the server row instead of staying on the dropped workspace.
const emptyEntities = flattenEntities(emptyLiveView);
assertEqual(emptyEntities.length, 1, "empty live view flattens to the server row only");
assertEqual(
  reconcileSelectedId(emptyEntities, "root-devenv-primary"),
  "server-local",
  "empty live view reconciles a stale mock selection to the server row",
);

// No entities yet (initial load): reconcile leaves the prior selection alone.
assertEqual(
  reconcileSelectedId([], "root-devenv-primary"),
  "root-devenv-primary",
  "no entities keeps the prior selection",
);

// Singleton left-nav compaction is a workspace/workRoot presentation rule. It
// selects the concrete workRoot, does not require main instances, and does not
// hide degraded metadata.
const singletonNoMainWorkspace = liveView.workspaces[0];
assertEqual(
  compactWorkspaceWorkRoot(singletonNoMainWorkspace)?.id,
  "root-local-abc",
  "single workspace + single workRoot compacts to the workRoot id without main instances",
);
assertEqual(
  compactWorkspaceWorkRootTitle(singletonNoMainWorkspace, singletonNoMainWorkspace.workRoots[0]),
  "devenv",
  "compact workspace/workRoot title deduplicates matching labels",
);
assertEqual(
  compactWorkspaceWorkRootTitle(
    singletonNoMainWorkspace,
    workRoot("root-distinct", "workspace-local", "distinct"),
  ),
  "devenv / distinct",
  "compact workspace/workRoot title preserves distinct labels",
);

const multiRootWorkspace: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-multi",
      label: "multi",
      state: readyState,
      compactable: true,
      workRoots: [
        workRoot("root-multi-a", "workspace-multi", "multi-a"),
        workRoot("root-multi-b", "workspace-multi", "multi-b"),
      ],
      actions: [],
    },
  ],
};
assertEqual(
  compactWorkspaceWorkRoot(multiRootWorkspace.workspaces[0]),
  null,
  "multi-workRoot workspace remains expanded instead of compacting",
);

// workspaceBaseWorkRoot resolves the primary/base root among a workspace's
// roots regardless of array position, so the "workspace"-presentation row
// (base root + >=1 linked worktree) can target the right work root for its
// close affordance instead of the workspace id.
const primaryPlusWorktree = workRoot("root-primary", "workspace-git", "primary");
primaryPlusWorktree.kind = "gitPrimaryRoot";
const linkedWorktree = workRoot("root-worktree", "workspace-git", "worktree");
linkedWorktree.kind = "gitLinkedWorktree";
const gitWorkspacePrimaryFirst: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-git",
      label: "git",
      state: readyState,
      compactable: false,
      workRoots: [primaryPlusWorktree, linkedWorktree],
      actions: [],
    },
  ],
};
assertEqual(
  workspaceBaseWorkRoot(gitWorkspacePrimaryFirst.workspaces[0])?.id,
  "root-primary",
  "workspaceBaseWorkRoot resolves the primary root among primary + linked-worktree roots",
);

const gitWorkspaceWorktreeFirst: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-git-reversed",
      label: "git-reversed",
      state: readyState,
      compactable: false,
      workRoots: [linkedWorktree, primaryPlusWorktree],
      actions: [],
    },
  ],
};
assertEqual(
  workspaceBaseWorkRoot(gitWorkspaceWorktreeFirst.workspaces[0])?.id,
  "root-primary",
  "workspaceBaseWorkRoot finds the primary root regardless of array position",
);

const offlineUnavailableRoot = workRoot("root-offline", "workspace-offline", "offline");
offlineUnavailableRoot.activation = "offline";
offlineUnavailableRoot.availability = "inaccessible";
offlineUnavailableRoot.status = "inaccessible";
offlineUnavailableRoot.state = {
  status: "degraded",
  loading: false,
  stale: true,
  error: "permission denied",
};
const offlineWorkspace: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-offline",
      label: "offline",
      state: readyState,
      compactable: false,
      workRoots: [offlineUnavailableRoot],
      actions: [],
    },
  ],
};
const compactOfflineRoot = compactWorkspaceWorkRoot(offlineWorkspace.workspaces[0]);
assertEqual(compactOfflineRoot?.id, "root-offline", "offline single root still compacts");
assertEqual(
  compactOfflineRoot?.availability,
  "inaccessible",
  "compact offline row keeps availability metadata",
);
assertEqual(
  compactOfflineRoot?.activation,
  "offline",
  "compact offline row keeps activation metadata",
);

const mainWithSub = instance("instance-main-with-sub", "workspace-main", "root-main");
mainWithSub.subInstances = [
  {
    ...instance("instance-sub", "workspace-main", "root-main"),
    role: "sub",
  },
];
const workspaceWithInstances: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-main",
      label: "main",
      state: readyState,
      compactable: false,
      workRoots: [workRoot("root-main", "workspace-main", "main", [mainWithSub])],
      actions: [],
    },
  ],
};
assertEqual(
  compactWorkspaceWorkRoot(workspaceWithInstances.workspaces[0])?.id,
  "root-main",
  "main/sub instance presence does not block workspace/workRoot compaction",
);
assertTrue(
  !flattenEntities(workspaceWithInstances).some((entity) => entity.type === "instance"),
  "main/sub instances do not reappear as left-nav rows when compactable",
);

const twoWorkspaceView: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    liveView.workspaces[0],
    {
      id: "workspace-second",
      label: "second",
      state: readyState,
      compactable: true,
      workRoots: [workRoot("root-second", "workspace-second", "second")],
      actions: [],
    },
  ],
};
assertEqual(
  compactWorkspaceWorkRoot(twoWorkspaceView.workspaces[0])?.id,
  "root-local-abc",
  "single-root workspaces compact independently even when the dashboard has multiple workspaces",
);
assertEqual(
  compactWorkspaceWorkRoot(twoWorkspaceView.workspaces[1])?.id,
  "root-second",
  "each single-root workspace gets its own compact workRoot row",
);

// mergeResourcesByServer accumulates rather than replaces: the multi-server
// keep-alive cache (260714 Phase 1) must retain every previously-fetched
// server's entry when a different server's resources arrive.
const serverBView: DashboardResourcesView = {
  server: { id: "server-remote-b", label: "Remote B", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-remote-b",
      label: "remote-b",
      state: readyState,
      compactable: true,
      workRoots: [workRoot("root-remote-b", "workspace-remote-b", "remote-b")],
      actions: [],
    },
  ],
};

const cacheWithA: ResourcesByServer = mergeResourcesByServer({}, liveView);
assertEqual(
  cacheWithA["server-local"],
  liveView,
  "merging server A's resources into an empty cache adds its entry",
);

const cacheWithAAndB = mergeResourcesByServer(cacheWithA, serverBView);
assertEqual(
  cacheWithAAndB["server-local"],
  liveView,
  "fetching server B's resources leaves server A's cached entry intact",
);
assertEqual(
  cacheWithAAndB["server-remote-b"],
  serverBView,
  "fetching server B's resources adds server B's entry",
);
assertEqual(
  Object.keys(cacheWithAAndB).length,
  2,
  "both servers' entries coexist in the accumulated cache",
);

// A later refetch of server A (e.g. a poll or explicit refresh) replaces only
// that server's own entry, still without disturbing server B's.
const refreshedServerAView: DashboardResourcesView = {
  ...liveView,
  workspaces: [],
};
const cacheAfterARefresh = mergeResourcesByServer(cacheWithAAndB, refreshedServerAView);
assertEqual(
  cacheAfterARefresh["server-local"],
  refreshedServerAView,
  "refetching server A replaces only its own entry",
);
assertEqual(
  cacheAfterARefresh["server-remote-b"],
  serverBView,
  "refetching server A does not disturb server B's cached entry",
);

// withLastNonNullResourcesByServer / resolveActiveResources (260714
// childroot-fix): a server that has never resolved still falls through to
// null - the cache must not resurrect it ahead of its first real fetch.
assertEqual(
  resolveActiveResources({}, "server-remote-b", {}),
  null,
  "a server with no resourcesByServer entry and no cached fallback resolves to null",
);

// Once a server has resolved at least once, a later render where
// `resourcesByServer` transiently has no entry for it (e.g. `selectedServerId`
// advanced ahead of the matching resources landing) falls back to the last
// non-null resources cached for that exact server id.
const lastNonNullAfterA = withLastNonNullResourcesByServer({}, "server-local", liveView);
assertEqual(
  lastNonNullAfterA["server-local"],
  liveView,
  "caches the first non-null resources seen for a server",
);
assertEqual(
  resolveActiveResources({}, "server-local", lastNonNullAfterA),
  liveView,
  "falls back to the cached resources when resourcesByServer has no entry for the selected server",
);
assertEqual(
  resolveActiveResources({ "server-local": serverBView }, "server-local", lastNonNullAfterA),
  serverBView,
  "prefers a fresh resourcesByServer entry over the cached fallback",
);

// The cache is scoped per server id - a cached fallback for one server must
// never leak into another server's transient gap.
assertEqual(
  resolveActiveResources({}, "server-remote-b", lastNonNullAfterA),
  null,
  "a cached fallback for one server does not resolve a different server's gap",
);

// A no-op update (same resources object, or a null fresh value) must not
// allocate a new cache object, so it is safe to call unconditionally on
// every render.
const lastNonNullUnchanged = withLastNonNullResourcesByServer(
  lastNonNullAfterA,
  "server-local",
  liveView,
);
assertTrue(
  lastNonNullUnchanged === lastNonNullAfterA,
  "re-caching the same resources object for the same server is a no-op",
);
assertTrue(
  withLastNonNullResourcesByServer(lastNonNullAfterA, "server-remote-b", null) ===
    lastNonNullAfterA,
  "a null fresh value never overwrites or extends the cache",
);

// removeResourcesByServer (260714 Phase 2: the Off deallocation gesture)
// deletes only the target server's entry and leaves every other cached
// server's entry untouched.
const cacheAfterRemovingB = removeResourcesByServer(cacheWithAAndB, "server-remote-b");
assertEqual(
  cacheAfterRemovingB["server-local"],
  liveView,
  "removing server B's entry leaves server A's cached entry intact",
);
assertTrue(
  !("server-remote-b" in cacheAfterRemovingB),
  "removing server B's entry deletes it from the cache",
);
assertEqual(
  Object.keys(cacheAfterRemovingB).length,
  1,
  "only the target server's entry is removed from the accumulated cache",
);

// Removing a server id that has no entry is a no-op that returns the same
// reference, so it is safe to call unconditionally.
assertTrue(
  removeResourcesByServer(cacheWithAAndB, "server-never-cached") === cacheWithAAndB,
  "removing an absent server id is a no-op that returns the same reference",
);

// resolveStickyWorkbenchSelection (260714 Phase 2 Prong 1): the server-scoped
// D5 pattern applied to selection. A workspace with two work roots - the
// second (`root-b`) stands in for the reported bug's `gitLinkedWorktree`
// child, whose entry can momentarily vanish from an otherwise-present
// resource tree.
const stickyWorkspaceWithBoth: DashboardResourcesView = {
  server: { id: "server-local", label: "Local ws dashboard", state: readyState, actions: [] },
  workspaces: [
    {
      id: "workspace-sticky",
      label: "sticky",
      state: readyState,
      compactable: false,
      workRoots: [
        workRoot("root-a", "workspace-sticky", "root-a"),
        workRoot("root-b", "workspace-sticky", "root-b"),
      ],
      actions: [],
    },
  ],
};
// The one-poll omission: `root-b`'s own entry is missing from the tree, but
// the tree itself (and `root-a`) are still present - the D2/D5 slot-level
// fallback does not engage, since `resources` itself is non-null.
const stickyWorkspaceMissingB: DashboardResourcesView = {
  ...stickyWorkspaceWithBoth,
  workspaces: [
    {
      ...stickyWorkspaceWithBoth.workspaces[0],
      workRoots: [workRoot("root-a", "workspace-sticky", "root-a")],
    },
  ],
};

const emptyStickyCache: LastMatchedSelectionByServer = {};

// A fresh exact match seeds the cache with `bridged: false` and returns the
// freshly-resolved selection unchanged.
const stickyAfterMatch = resolveStickyWorkbenchSelection(
  stickyWorkspaceWithBoth,
  "root-b",
  "server-local",
  emptyStickyCache,
);
assertEqual(
  stickyAfterMatch.selection?.root.id,
  "root-b",
  "an exact match resolves to the matched root, not the fallback",
);
assertEqual(
  stickyAfterMatch.nextLastMatchedSelectionByServer["server-local"]?.bridged,
  false,
  "a fresh exact match seeds the cache entry as not-yet-bridged",
);

// First consecutive miss: `root-b` is momentarily absent from the tree.
// The cached selection bridges over it (still `root-b`, not the fallback
// `root-a`), and the cache entry advances to `bridged: true`.
const stickyAfterFirstMiss = resolveStickyWorkbenchSelection(
  stickyWorkspaceMissingB,
  "root-b",
  "server-local",
  stickyAfterMatch.nextLastMatchedSelectionByServer,
);
assertEqual(
  stickyAfterFirstMiss.selection?.root.id,
  "root-b",
  "a first consecutive miss bridges to the previously-matched selection, not the natural fallback",
);
assertEqual(
  stickyAfterFirstMiss.nextLastMatchedSelectionByServer["server-local"]?.bridged,
  true,
  "a bridged miss advances the cache entry so a second consecutive miss is not bridged again",
);

// Second consecutive miss for the same `selectedId`: treated as a genuine
// removal, not a transient omission. Falls through to the natural fallback
// (`root-a`, the first root walked) and drops the cache entry.
const stickyAfterSecondMiss = resolveStickyWorkbenchSelection(
  stickyWorkspaceMissingB,
  "root-b",
  "server-local",
  stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
);
assertEqual(
  stickyAfterSecondMiss.selection?.root.id,
  "root-a",
  "a second consecutive miss is treated as a genuine removal and falls through to the natural fallback",
);
assertTrue(
  !("server-local" in stickyAfterSecondMiss.nextLastMatchedSelectionByServer),
  "a genuine removal drops the now-stale cache entry",
);

// A real user re-selection to an id absent from the tree (so the fresh
// lookup itself does not match, falling through to the natural fallback)
// while a bridged cache entry exists for a DIFFERENT, unrelated `selectedId`
// does not consult that stale entry - the fresh (fallback) selection passes
// through untouched, and the cache is left as-is (nothing to advance for a
// selection the cache never saw). Note this "otherwise" branch is reached
// only when the fresh lookup itself does NOT match (see the matched-branch
// case above, which always reseeds the cache for the matched `selectedId`
// regardless of what was cached before).
const stickyOnRealReselect = resolveStickyWorkbenchSelection(
  stickyWorkspaceWithBoth,
  "root-nonexistent",
  "server-local",
  stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
);
assertEqual(
  stickyOnRealReselect.selection?.root.id,
  "root-a",
  "a re-selection to an absent id falls through to the natural fallback and ignores a stale cache entry for a different selectedId",
);
assertTrue(
  stickyOnRealReselect.nextLastMatchedSelectionByServer ===
    stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
  "a re-selection the cache never saw leaves the unrelated cache entry's reference untouched",
);

// `resources === null` is already fully owned by D2/D5 upstream of this
// function - the sticky cache must not engage on that case at all (only
// bridges "tree present, entry missing", never "tree itself absent").
const stickyOnNullResources = resolveStickyWorkbenchSelection(
  null,
  "root-b",
  "server-local",
  stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
);
assertEqual(
  stickyOnNullResources.selection,
  null,
  "a null resources tree resolves to a null selection, untouched by the sticky cache",
);
assertTrue(
  stickyOnNullResources.nextLastMatchedSelectionByServer ===
    stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
  "a null resources tree passes the cache through unchanged rather than bridging or dropping it",
);

// `selectedId === null` boundary (initial-load case, before anything has
// ever been selected): the fresh lookup never matches (no `selectedId` to
// find), so this always falls into the "otherwise" branch regardless of what
// the cache holds for this server - the natural fallback (the first root
// walked) passes through untouched and the cache is left exactly as-is, even
// if a stale bridged entry exists from a PRIOR (now-irrelevant) selection.
const stickyOnNullSelectedId = resolveStickyWorkbenchSelection(
  stickyWorkspaceWithBoth,
  null,
  "server-local",
  stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
);
assertEqual(
  stickyOnNullSelectedId.selection?.root.id,
  "root-a",
  "a null selectedId resolves to the natural fallback, ignoring any cached entry",
);
assertTrue(
  stickyOnNullSelectedId.nextLastMatchedSelectionByServer ===
    stickyAfterFirstMiss.nextLastMatchedSelectionByServer,
  "a null selectedId leaves the cache reference untouched",
);
