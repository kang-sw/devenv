import {
  findOpenWorkRoot,
  resolveClosedWorkRootRefs,
  resolveEffectiveActiveRootKey,
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

// `resolveEffectiveActiveRootKey` - the 260714 childroot-fix safety net's
// decision, extracted for unit coverage: which mounted rootKey (if any)
// should render active, given the current selection, whether it matched a
// mounted instance, and the remembered last-active root/server pair.

// A genuine match: the current selection matches a mounted instance, so it
// wins outright regardless of what is remembered.
assertEqual(
  resolveEffectiveActiveRootKey({
    selectedRootKey: "server-local/root-a",
    selectedRootIsMounted: true,
    lastActiveRootKey: "server-local/root-b",
    lastActiveRootServerId: "server-local",
    selectedServerId: "server-local",
  }),
  "server-local/root-a",
  "a genuinely-mounted selection wins outright over the remembered fallback",
);

// Same-server transient collapse: the selection matches no mounted instance
// right now, but the remembered last-active root belongs to the same server
// that is currently selected - fall back to it (the remote-child
// flash-then-hide bug this safety net exists to fix).
assertEqual(
  resolveEffectiveActiveRootKey({
    selectedRootKey: "server-local/root-a",
    selectedRootIsMounted: false,
    lastActiveRootKey: "server-local/root-b",
    lastActiveRootServerId: "server-local",
    selectedServerId: "server-local",
  }),
  "server-local/root-b",
  "a same-server transient selection collapse falls back to that server's last-active root",
);

// Cross-server switch to an unresolved server: the selection collapsed
// (never fetched/resolved) and the remembered last-active root belongs to a
// *different* server than the one currently selected - must NOT pin the
// previous server's root; resolves to null (render falls through to the
// empty-state watermark).
assertEqual(
  resolveEffectiveActiveRootKey({
    selectedRootKey: null,
    selectedRootIsMounted: false,
    lastActiveRootKey: "server-remote-1/root-a",
    lastActiveRootServerId: "server-remote-1",
    selectedServerId: "server-remote-2",
  }),
  null,
  "a cross-server switch to an unresolved server does not pin the previous server's root",
);

// No history yet (fresh session, nothing has ever genuinely matched): also
// resolves to null rather than throwing.
assertEqual(
  resolveEffectiveActiveRootKey({
    selectedRootKey: null,
    selectedRootIsMounted: false,
    lastActiveRootKey: null,
    lastActiveRootServerId: null,
    selectedServerId: "server-local",
  }),
  null,
  "no remembered last-active root resolves to null",
);
