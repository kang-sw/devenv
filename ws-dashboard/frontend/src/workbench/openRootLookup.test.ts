import {
  applySelectRoot,
  findOpenWorkRoot,
  resolveClosedWorkRootRefs,
  withOpenWorkRootKey,
  withOpenWorkRootRef,
} from "./openRootLookup.js";
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

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const a = JSON.stringify(actual),
    e = JSON.stringify(expected);
  if (a !== e) {
    throw new Error(`${label}: expected ${e}, got ${a}`);
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

// `resolveClosedWorkRootRefs` — pure key-diff step behind the close-triggered
// cleanup effect in `WorkbenchShell`. Given the previous render's
// `openWorkRootKeys`/`openWorkRootRefs` snapshot and the current
// `openWorkRootKeys`, it resolves which rootKeys just closed and their
// `{rootId, serverRoute}`.

const refA = { rootId: "root-a", serverRoute: "server-local" };
const refB = { rootId: "root-b", serverRoute: "server-remote-1" };
const previousRefs = { "key-a": refA, "key-b": refB };

// A key present in `previousKeys` but absent from `currentKeys` resolves via
// `previousRefs`.
assertDeepEqual(
  resolveClosedWorkRootRefs(["key-a", "key-b"], previousRefs, ["key-b"]),
  [{ rootKey: "key-a", rootId: refA.rootId, serverRoute: refA.serverRoute }],
  "a key dropped from currentKeys resolves via previousRefs",
);

// A key still present in `currentKeys` is not included, even though it is
// also in `previousKeys`.
assertDeepEqual(
  resolveClosedWorkRootRefs(
    ["key-a", "key-b"],
    previousRefs,
    ["key-a", "key-b"],
  ),
  [],
  "a key still present in currentKeys is not treated as closed",
);

// An already-missing `previousRefs` entry (e.g. cleared out of band) is
// skipped gracefully rather than throwing or producing a partial record.
assertDeepEqual(
  resolveClosedWorkRootRefs(["key-a", "key-missing"], { "key-a": refA }, []),
  [{ rootKey: "key-a", rootId: refA.rootId, serverRoute: refA.serverRoute }],
  "a closed key with no previousRefs entry is skipped, other closed keys still resolve",
);

// `withOpenWorkRootKey`/`withOpenWorkRootRef` - the shared "ensure open"
// helpers behind the 260714-select-mount-gap fix. Both the
// `workbenchSelection` effect in `App.tsx` and the `resource.select` command
// handler's server-switch fast path call these so a newly-selected work root
// is mounted with identical key/ref shape and idempotent add-only-if-absent
// semantics, no matter which of the two call sites mounts it first.

const refX = { rootId: "root-x", serverRoute: "server-local" };
const refY = { rootId: "root-y", serverRoute: "server-remote-1" };

// Adding a key that is absent appends it without reordering existing keys.
assertDeepEqual(
  withOpenWorkRootKey(["key-a", "key-b"], "key-c"),
  ["key-a", "key-b", "key-c"],
  "withOpenWorkRootKey appends an absent key without reordering existing keys",
);

// Adding a key that is already present is a no-op: same reference returned,
// no duplicate entry.
const existingKeys = ["key-a", "key-b"];
const sameKeysResult = withOpenWorkRootKey(existingKeys, "key-a");
assertEqual(
  sameKeysResult === existingKeys,
  true,
  "withOpenWorkRootKey returns the same array reference when the key is already present (idempotent, skips redundant setState)",
);
assertDeepEqual(
  sameKeysResult,
  ["key-a", "key-b"],
  "withOpenWorkRootKey does not duplicate an already-present key",
);

// Seeding a ref for an absent key adds it with the exact `{rootId,
// serverRoute}` shape.
assertDeepEqual(
  withOpenWorkRootRef({ "key-a": refX }, "key-b", refY),
  { "key-a": refX, "key-b": refY },
  "withOpenWorkRootRef seeds an absent key with the given ref, preserving existing entries",
);

// Seeding a ref for a key that already has one never clobbers it, even with
// a different candidate ref for the same key.
const existingRefs = { "key-a": refX };
const sameRefResult = withOpenWorkRootRef(existingRefs, "key-a", refY);
assertEqual(
  sameRefResult === existingRefs,
  true,
  "withOpenWorkRootRef returns the same object reference when the key already has a ref (idempotent, skips redundant setState)",
);
assertDeepEqual(
  sameRefResult,
  { "key-a": refX },
  "withOpenWorkRootRef does not clobber an already-open root's ref",
);

// `applySelectRoot` - the pure core of the `selectRoot(serverId, entityId)`
// atomic action (260714 Phase 2, D4). Asserts the `selectedServerId`/
// `selectedId` pair advances together (atomically) for a work-root-shaped
// `entityId`, and separately for a non-work-root `entityId` (server row /
// workspace row id) - `selectRoot` never resolves or validates `entityId`
// itself, so both cases return the exact same `{selectedServerId,
// selectedId}` shape regardless of what kind of id `entityId` is.

assertDeepEqual(
  applySelectRoot({ serverId: "server-remote-1", entityId: "root-a" }),
  { selectedServerId: "server-remote-1", selectedId: "root-a" },
  "applySelectRoot commits the selectedServerId/selectedId pair atomically for a work-root-shaped entityId",
);

assertDeepEqual(
  applySelectRoot({ serverId: "server-local", entityId: "server-local" }),
  { selectedServerId: "server-local", selectedId: "server-local" },
  "applySelectRoot commits selection unchanged for a non-work-root entityId (a server row), without inventing a mount",
);

assertDeepEqual(
  applySelectRoot({ serverId: "server-local", entityId: "workspace-a" }),
  { selectedServerId: "server-local", selectedId: "workspace-a" },
  "applySelectRoot commits selection unchanged for a non-work-root entityId (a workspace row), without inventing a mount",
);
