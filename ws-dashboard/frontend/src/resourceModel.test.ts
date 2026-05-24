import {
  compactWorkspaceWorkRoot,
  flattenEntities,
  preferredSelection,
  reconcileSelectedId,
  type DashboardResourcesView,
  type InstanceView,
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
