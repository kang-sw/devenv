import { deriveWorkbenchView } from "./openRootLookup.js";
import {
  serverScopedIdentity,
  type DashboardResourcesView,
  type InstanceView,
  type ResourcesByServer,
  type ViewState,
  type WorkRootView,
} from "../resourceModel.js";

// Pure-logic coverage for `deriveWorkbenchView` (260714 active-root
// derivation refactor Phase 1, D1/D2/D3). Oracle is tsc + node only - no
// jsdom/RTL/Playwright (D6). Each case feeds the exact single-render committed
// -state slice a real render would see and asserts the derived
// `openInstanceKeys`/`effectiveActiveRootKey` do not collapse on the
// intervening render.

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

function resources(
  serverId: string,
  workRoots: WorkRootView[],
): DashboardResourcesView {
  return {
    server: { id: serverId, label: serverId, state: readyState, actions: [] },
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

function isPrefix<T>(prefix: readonly T[], full: readonly T[]): boolean {
  if (prefix.length > full.length) {
    return false;
  }
  return prefix.every((value, index) => value === full[index]);
}

const localResources = resources("server-local", [
  workRoot("root-a", "workspace-a", "server-local", [
    instance("instance-main-a", "workspace-a", "root-a", "server-local"),
  ]),
  workRoot("root-b", "workspace-a", "server-local", [
    instance("instance-main-b", "workspace-a", "root-b", "server-local"),
  ]),
]);
const rootAKey = serverScopedIdentity("server-local", "root-a");
const rootBKey = serverScopedIdentity("server-local", "root-b");

// (a) Mount-by-construction: the selected root resolves straight off
// `resourcesByServer`, and even though `openWorkRootKeys` does not yet contain
// its key (the selection-seed effect runs one render later), the union folds
// it in this render and `effectiveActiveRootKey` points at it - no collapse.
{
  const view = deriveWorkbenchView({
    resourcesByServer: { "server-local": localResources },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-a",
    openWorkRootKeys: [],
    openWorkRootRefs: {},
  });
  assertEqual(
    view.effectiveActiveRootKey,
    rootAKey,
    "(a) selected root resolves to its state key on the same render",
  );
  assertDeepEqual(
    view.openInstanceKeys,
    [rootAKey],
    "(a) the selected root is folded into openInstanceKeys by construction even before the seed effect runs",
  );
}

// (b) D2 transient-gap fallback: `resourcesByServer` has no entry for the
// selected server on this render (e.g. one render after `selectedServerId`
// advanced), but `lastNonNullResourcesByServer` still does. The selection
// resolves through the same fallback path as `activeResources`, so the active
// root does NOT collapse to null for that render.
{
  const view = deriveWorkbenchView({
    resourcesByServer: {},
    lastNonNullResourcesByServer: { "server-local": localResources },
    selectedServerId: "server-local",
    selectedId: "instance-main-a",
    openWorkRootKeys: [rootAKey],
    openWorkRootRefs: {
      [rootAKey]: { rootId: "root-a", serverRoute: "server-local" },
    },
  });
  assertEqual(
    view.effectiveActiveRootKey,
    rootAKey,
    "(b) a transient resourcesByServer gap falls back to lastNonNull and does not collapse the active root",
  );
  assertDeepEqual(
    view.openInstanceKeys,
    [rootAKey],
    "(b) openInstanceKeys keeps the selected root through the transient gap",
  );
}

// (b) unresolved-server case: neither `resourcesByServer` nor
// `lastNonNullResourcesByServer` has an entry for the selected server (a
// server that has never resolved). Selection is null, so
// `effectiveActiveRootKey` is null and `openInstanceKeys` is exactly the
// persisted keys (nothing appended, nothing dropped) - the render falls
// through to the empty-state watermark without pinning a stale root.
{
  const openWorkRootKeys = [rootAKey];
  const view = deriveWorkbenchView({
    resourcesByServer: {},
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-remote-2",
    selectedId: "root-x",
    openWorkRootKeys,
    openWorkRootRefs: {
      [rootAKey]: { rootId: "root-a", serverRoute: "server-local" },
    },
  });
  assertEqual(
    view.effectiveActiveRootKey,
    null,
    "(b) an unresolved server resolves the active root to null",
  );
  assertDeepEqual(
    view.openInstanceKeys,
    [rootAKey],
    "(b) an unresolved server leaves the persisted keep-alive keys untouched",
  );
}

// (c) Keep-alive: a non-selected member already in `openWorkRootKeys` survives
// in `openInstanceKeys` and does not reorder when a different root is
// selected - the selected root is appended after it.
{
  const view = deriveWorkbenchView({
    resourcesByServer: { "server-local": localResources },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-a",
    openWorkRootKeys: [rootBKey],
    openWorkRootRefs: {
      [rootBKey]: { rootId: "root-b", serverRoute: "server-local" },
    },
  });
  assertDeepEqual(
    view.openInstanceKeys,
    [rootBKey, rootAKey],
    "(c) a keep-alive member survives and stays first; the newly selected root is appended",
  );
}

// (c) Re-selecting an already-open root is a no-op on ordering: no duplicate,
// no reorder.
{
  const view = deriveWorkbenchView({
    resourcesByServer: { "server-local": localResources },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-b",
    openWorkRootKeys: [rootBKey, rootAKey],
    openWorkRootRefs: {
      [rootBKey]: { rootId: "root-b", serverRoute: "server-local" },
      [rootAKey]: { rootId: "root-a", serverRoute: "server-local" },
    },
  });
  assertDeepEqual(
    view.openInstanceKeys,
    [rootBKey, rootAKey],
    "(c) re-selecting an already-open root neither duplicates nor reorders the union",
  );
}

// (d) Cross-render position-preserving-prefix proxy: render N selects root-b
// with no keys open yet; the seed effect then adds root-b to
// `openWorkRootKeys`; render N+1 selects root-a. render N's `openInstanceKeys`
// must be a position-preserving prefix of render N+1's.
{
  const stateN = deriveWorkbenchView({
    resourcesByServer: { "server-local": localResources },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-b",
    openWorkRootKeys: [],
    openWorkRootRefs: {},
  });
  const stateNPlus1 = deriveWorkbenchView({
    resourcesByServer: { "server-local": localResources },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-a",
    openWorkRootKeys: stateN.openInstanceKeys,
    openWorkRootRefs: {
      [rootBKey]: { rootId: "root-b", serverRoute: "server-local" },
    },
  });
  assertDeepEqual(
    stateN.openInstanceKeys,
    [rootBKey],
    "(d) render N mounts the selected root by construction",
  );
  assertDeepEqual(
    stateNPlus1.openInstanceKeys,
    [rootBKey, rootAKey],
    "(d) render N+1 preserves the prior member and appends the new selection",
  );
  assertEqual(
    isPrefix(stateN.openInstanceKeys, stateNPlus1.openInstanceKeys),
    true,
    "(d) render N's openInstanceKeys is a position-preserving prefix of render N+1's across a selection change",
  );
}

// Type-only guard: `ResourcesByServer` is the committed-state map type the
// derivation consumes; referenced here so a signature drift surfaces in this
// suite too.
const _typeGuard: ResourcesByServer = { "server-local": localResources };
void _typeGuard;
