import {
  flattenEntities,
  preferredSelection,
  reconcileSelectedId,
  type DashboardResourcesView,
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

function workRoot(id: string, workspaceId: string, label: string): WorkRootView {
  return {
    id,
    resourcePath: { serverId: "server-local", workspaceId, workRootId: id, instanceId: null },
    label,
    kind: "plainDirectory",
    status: "online",
    state: readyState,
    compactable: false,
    mainInstances: [],
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

// A caller selects the mock workRoot...
const mockSelectedId = preferredSelection(mockEntities);

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
