import { findOpenWorkRoot } from "./openRootLookup.js";
import type {
  DashboardResourcesView,
  InstanceView,
  ViewState,
  WorkRootView,
} from "../resourceModel.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

const readyState: ViewState = {
  status: "ready",
  loading: false,
  stale: false,
  error: null,
};

function instance(
  id: string,
  workspaceId: string,
  workRootId: string,
  serverId: string,
): InstanceView {
  return {
    id,
    resourcePath: { serverId, workspaceId, workRootId, instanceId: id },
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
  serverId: string,
  mainInstances: InstanceView[] = [],
): WorkRootView {
  return {
    id,
    resourcePath: { serverId, workspaceId, workRootId: id, instanceId: null },
    label: id,
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

function resources(workRoots: WorkRootView[]): DashboardResourcesView {
  return {
    server: { id: "server-local", label: "Mock", state: readyState, actions: [] },
    workspaces: [
      {
        id: "workspace-a",
        label: "workspace-a",
        state: readyState,
        compactable: false,
        workRoots,
        actions: [],
      },
    ],
  };
}

// `resources: null` (resources not yet loaded).
assertEqual(
  findOpenWorkRoot(null, { rootId: "root-a", serverRoute: "server-local" }),
  null,
  "null resources snapshot resolves to no open root",
);

// No match: the tracked root id is stale/closed and no longer present in the
// current snapshot.
const singleRootResources = resources([
  workRoot("root-a", "workspace-a", "server-local", [
    instance("instance-main-a", "workspace-a", "root-a", "server-local"),
  ]),
]);
assertEqual(
  findOpenWorkRoot(singleRootResources, {
    rootId: "root-stale",
    serverRoute: "server-local",
  }),
  null,
  "a stale/closed root id that is absent from the snapshot resolves to no open root",
);

// `serverRoute` mismatch: same `rootId` exists, but on a different server
// route — must not be treated as a match (guards the identity-collision
// scenario the plan's Constraints section calls out for other identity
// keys).
assertEqual(
  findOpenWorkRoot(singleRootResources, {
    rootId: "root-a",
    serverRoute: "server-remote-1",
  }),
  null,
  "a rootId match on a different serverRoute is not treated as the same open root",
);

// Match: same `rootId` and `serverRoute` resolves the root and its first
// main instance.
const resolved = findOpenWorkRoot(singleRootResources, {
  rootId: "root-a",
  serverRoute: "server-local",
});
if (!resolved) {
  throw new Error(
    "matching rootId+serverRoute: expected a resolved open root, got null",
  );
}
assertEqual(resolved.root.id, "root-a", "resolved root has the matching id");
assertEqual(
  resolved.mainInstance?.id ?? null,
  "instance-main-a",
  "resolved mainInstance is the root's first main instance",
);

// A root with no main instances resolves `mainInstance` to `null` rather
// than throwing.
const noInstanceResources = resources([
  workRoot("root-b", "workspace-a", "server-local", []),
]);
const resolvedNoInstance = findOpenWorkRoot(noInstanceResources, {
  rootId: "root-b",
  serverRoute: "server-local",
});
assertEqual(
  resolvedNoInstance?.mainInstance ?? null,
  null,
  "a root with no main instances resolves mainInstance to null",
);
