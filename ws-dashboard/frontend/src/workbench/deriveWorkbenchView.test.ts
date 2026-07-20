import {
  deriveWorkbenchView,
  driveStickyWorkbenchSelection,
  initialStickySelectionDriverState,
  type StickySelectionDriverState,
} from "./openRootLookup.js";
import {
  resolveActiveResources,
  resolveWorkbenchSelection,
  serverScopedIdentity,
  type DashboardResourcesView,
  type InstanceView,
  type ResourcesByServer,
  type ViewState,
  type WorkbenchSelection,
  type WorkRootView,
} from "../resourceModel.js";

// Pure-logic coverage for `deriveWorkbenchView` (260714 active-root
// derivation refactor Phase 1, D1/D2/D3) and `driveStickyWorkbenchSelection`
// (260714 Phase 2 Prong 1 + its correctness-review idempotency fix). Oracle
// is tsc + node only - no jsdom/RTL/Playwright (D6). Each case feeds the
// exact single-render committed-state slice a real render would see and
// asserts the derived `openInstanceKeys`/`effectiveActiveRootKey` do not
// collapse on the intervening render.

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

// Test-only stand-in for the non-sticky selection `deriveWorkbenchView`
// takes as an input since the correctness fix (260714 Phase 2 Prong 1):
// mirrors exactly what a caller with no transient-omission concern (cases
// (a)-(d) below) would compute and pass in - `resolveActiveResources` then a
// plain `resolveWorkbenchSelection`, no sticky cache involved.
function plainSelectionFor(
  resourcesByServer: ResourcesByServer,
  lastNonNullResourcesByServer: ResourcesByServer,
  selectedServerId: string,
  selectedId: string | null,
): WorkbenchSelection | null {
  const activeResources = resolveActiveResources(
    resourcesByServer,
    selectedServerId,
    lastNonNullResourcesByServer,
  );
  return resolveWorkbenchSelection(activeResources, selectedId);
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
    selection: plainSelectionFor(
      { "server-local": localResources },
      {},
      "server-local",
      "root-a",
    ),
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
    selection: plainSelectionFor(
      {},
      { "server-local": localResources },
      "server-local",
      "instance-main-a",
    ),
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
    selection: plainSelectionFor({}, {}, "server-remote-2", "root-x"),
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
    selection: plainSelectionFor(
      { "server-local": localResources },
      {},
      "server-local",
      "root-a",
    ),
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
    selection: plainSelectionFor(
      { "server-local": localResources },
      {},
      "server-local",
      "root-b",
    ),
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
    selection: plainSelectionFor(
      { "server-local": localResources },
      {},
      "server-local",
      "root-b",
    ),
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
    selection: plainSelectionFor(
      { "server-local": localResources },
      {},
      "server-local",
      "root-a",
    ),
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

// Fixtures for the sticky-selection wiring/idempotency cases below: a
// workspace with two work roots - root-b stands in for the reported bug's
// `gitLinkedWorktree` child, whose entry can momentarily vanish from an
// otherwise-present resource tree. Each "poll" is modeled as a FRESH object
// (a new `resources(...)` call), matching how `mergeResourcesByServer`
// allocates a new `DashboardResourcesView` per real poll response; re-using
// the SAME object reference across two `driveStickyWorkbenchSelection` calls
// models two render invocations over the SAME poll (e.g. StrictMode's
// deterministic double-invoke, or an unrelated App-level re-render before
// the next poll lands).
function resourcesWithBothRoots(): DashboardResourcesView {
  return resources("server-local", [
    workRoot("root-a", "workspace-a", "server-local"),
    workRoot("root-b", "workspace-a", "server-local"),
  ]);
}
function resourcesMissingRootB(): DashboardResourcesView {
  return resources("server-local", [
    workRoot("root-a", "workspace-a", "server-local"),
  ]);
}

// (e) 260714 Phase 2 Prong 1 wiring: `deriveWorkbenchView` must be fed the
// already-driven sticky selection (via `driveStickyWorkbenchSelection`), not
// re-derive it from a cache itself, so a transient single-poll omission of
// the previously-selected root's own entry does not collapse
// `effectiveActiveRootKey` to the natural fallback (root-a). One
// `driveStickyWorkbenchSelection` call per poll here (the "happy path" of
// exactly one render per poll); case (f) below is the same scenario but
// exercises >=2 render invocations per poll.
{
  let driverState: StickySelectionDriverState = initialStickySelectionDriverState;

  // Poll 1: both roots present, root-b freshly selected.
  const poll1 = resourcesWithBothRoots();
  const activeResourcesPoll1 = resolveActiveResources(
    { "server-local": poll1 },
    "server-local",
    {},
  );
  const driven1 = driveStickyWorkbenchSelection(
    {
      activeResources: activeResourcesPoll1,
      selectedId: "root-b",
      selectedServerId: "server-local",
    },
    driverState,
  );
  driverState = driven1.nextDriverState;
  const stateRenderN = deriveWorkbenchView({
    resourcesByServer: { "server-local": poll1 },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-b",
    openWorkRootKeys: [],
    openWorkRootRefs: {},
    selection: driven1.selection,
  });
  assertEqual(
    stateRenderN.effectiveActiveRootKey,
    rootBKey,
    "(e) render N resolves the freshly-selected root-b normally",
  );

  // Poll 2: root-b's own entry is momentarily missing from an otherwise-
  // present tree (root-a still there). Without Prong 1 this would collapse
  // to the fallback (root-a); with it, the sticky cache bridges the one-poll
  // omission and `effectiveActiveRootKey` stays pinned to root-b.
  const poll2 = resourcesMissingRootB();
  const activeResourcesPoll2 = resolveActiveResources(
    { "server-local": poll2 },
    "server-local",
    {},
  );
  const driven2 = driveStickyWorkbenchSelection(
    {
      activeResources: activeResourcesPoll2,
      selectedId: "root-b",
      selectedServerId: "server-local",
    },
    driverState,
  );
  driverState = driven2.nextDriverState;
  const stateRenderNPlus1 = deriveWorkbenchView({
    resourcesByServer: { "server-local": poll2 },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-b",
    openWorkRootKeys: stateRenderN.openInstanceKeys,
    openWorkRootRefs: {
      [rootBKey]: { rootId: "root-b", serverRoute: "server-local" },
    },
    selection: driven2.selection,
  });
  assertEqual(
    stateRenderNPlus1.effectiveActiveRootKey,
    rootBKey,
    "(e) a transient single-poll omission of the selected root's own entry does not collapse the active root to the natural fallback",
  );

  // Poll 3: the omission repeats on a genuinely NEW poll (a fresh object,
  // still missing root-b) - a real removal, not a transient blip. The
  // sticky cache must expire, and the active root is now allowed to fall
  // through to the natural fallback (root-a).
  const poll3 = resourcesMissingRootB();
  const activeResourcesPoll3 = resolveActiveResources(
    { "server-local": poll3 },
    "server-local",
    {},
  );
  const driven3 = driveStickyWorkbenchSelection(
    {
      activeResources: activeResourcesPoll3,
      selectedId: "root-b",
      selectedServerId: "server-local",
    },
    driverState,
  );
  const stateRenderNPlus2 = deriveWorkbenchView({
    resourcesByServer: { "server-local": poll3 },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-b",
    openWorkRootKeys: stateRenderNPlus1.openInstanceKeys,
    openWorkRootRefs: {
      [rootBKey]: { rootId: "root-b", serverRoute: "server-local" },
    },
    selection: driven3.selection,
  });
  assertEqual(
    stateRenderNPlus2.effectiveActiveRootKey,
    rootAKey,
    "(e) a second consecutive omitting POLL is treated as a genuine removal and the active root is allowed to fall through to the natural fallback",
  );
}

// (f) 260714 Phase 2 Prong 1 correctness-review regression: the miss-counter
// must advance at most once per distinct POLL, not once per render
// invocation. This is the axis the pre-fix implementation got wrong (caught
// by correctness review, not by the per-call tests above or in
// resourceModel.test.ts, since those advance exactly once per call/assertion
// and never re-invoke over the identical input). Simulates >=2 render
// invocations over the SAME poll input - standing in for React StrictMode's
// deterministic double-invoke in development, or any unrelated App-level
// re-render firing before the next poll response lands in production - and
// asserts the bridge SURVIVES every repeat invocation, only expiring once a
// GENUINELY NEW poll still omits the root.
{
  let driverState: StickySelectionDriverState = initialStickySelectionDriverState;

  // Poll 1 (both roots present, root-b selected) - two render invocations
  // over the exact same poll object (simulates StrictMode's double-invoke of
  // the component body holding the driver ref). Both invocations see the
  // matched case, so this pair mostly documents that the matched branch is
  // already idempotent (it was never the buggy branch).
  const poll1 = resourcesWithBothRoots();
  const activeResourcesPoll1 = resolveActiveResources(
    { "server-local": poll1 },
    "server-local",
    {},
  );
  const renderKeyPoll1 = {
    activeResources: activeResourcesPoll1,
    selectedId: "root-b",
    selectedServerId: "server-local",
  };
  const poll1Invocation1 = driveStickyWorkbenchSelection(
    renderKeyPoll1,
    driverState,
  );
  driverState = poll1Invocation1.nextDriverState;
  const poll1Invocation2 = driveStickyWorkbenchSelection(
    renderKeyPoll1,
    driverState,
  );
  driverState = poll1Invocation2.nextDriverState;
  assertEqual(
    poll1Invocation2.selection?.root.id,
    "root-b",
    "(f) poll 1, invocation 2 (repeat render, same poll): still resolves the freshly-matched root-b",
  );

  // Poll 2 (root-b's own entry omitted) - TWO render invocations over the
  // exact same (identical object reference) omitting poll, modeling
  // StrictMode's double-invoke landing on the very poll that introduces the
  // omission. Pre-fix, invocation 1 would bridge (bridged: true) and
  // invocation 2 would immediately read that already-bridged cache and treat
  // it as a second consecutive miss - dropping to the fallback (root-a)
  // within the SAME poll, i.e. zero bridging. Post-fix, invocation 2 must
  // reuse invocation 1's memoized result untouched.
  const poll2 = resourcesMissingRootB();
  const activeResourcesPoll2 = resolveActiveResources(
    { "server-local": poll2 },
    "server-local",
    {},
  );
  const renderKeyPoll2 = {
    activeResources: activeResourcesPoll2,
    selectedId: "root-b",
    selectedServerId: "server-local",
  };
  const poll2Invocation1 = driveStickyWorkbenchSelection(
    renderKeyPoll2,
    driverState,
  );
  assertEqual(
    poll2Invocation1.selection?.root.id,
    "root-b",
    "(f) poll 2, invocation 1: the first omitting poll bridges to root-b",
  );
  const poll2Invocation2 = driveStickyWorkbenchSelection(
    renderKeyPoll2,
    poll2Invocation1.nextDriverState,
  );
  assertEqual(
    poll2Invocation2.selection?.root.id,
    "root-b",
    "(f) poll 2, invocation 2 (repeat render, SAME omitting poll): the bridge survives - this is the regression the correctness review caught",
  );
  // A third, fourth, ... repeat invocation over the same poll must also stay
  // pinned - the memoized result never degrades on its own from repeats.
  const poll2Invocation3 = driveStickyWorkbenchSelection(
    renderKeyPoll2,
    poll2Invocation2.nextDriverState,
  );
  assertEqual(
    poll2Invocation3.selection?.root.id,
    "root-b",
    "(f) poll 2, invocation 3 (yet another repeat render, SAME omitting poll): still bridged",
  );
  driverState = poll2Invocation3.nextDriverState;

  // deriveWorkbenchView, fed the driven selection from the repeat-invocation
  // path above, must agree - no divergence introduced by the idempotency
  // guard.
  const stateAfterPoll2Repeats = deriveWorkbenchView({
    resourcesByServer: { "server-local": poll2 },
    lastNonNullResourcesByServer: {},
    selectedServerId: "server-local",
    selectedId: "root-b",
    openWorkRootKeys: [],
    openWorkRootRefs: {},
    selection: poll2Invocation3.selection,
  });
  assertEqual(
    stateAfterPoll2Repeats.effectiveActiveRootKey,
    rootBKey,
    "(f) deriveWorkbenchView agrees with the driven selection after repeat invocations over the same omitting poll",
  );

  // Poll 3: a GENUINELY NEW poll (fresh object reference), still omitting
  // root-b - a real second consecutive miss across actual polls. The bridge
  // must now expire and fall through to the natural fallback (root-a). One
  // invocation is enough here since the point is the cross-POLL transition,
  // not another repeat-invocation check.
  const poll3 = resourcesMissingRootB();
  const activeResourcesPoll3 = resolveActiveResources(
    { "server-local": poll3 },
    "server-local",
    {},
  );
  const poll3Invocation1 = driveStickyWorkbenchSelection(
    {
      activeResources: activeResourcesPoll3,
      selectedId: "root-b",
      selectedServerId: "server-local",
    },
    driverState,
  );
  assertEqual(
    poll3Invocation1.selection?.root.id,
    "root-a",
    "(f) poll 3 (a genuinely new poll still omitting root-b) is the real second consecutive miss and falls through to the fallback",
  );
}

// Type-only guard: `ResourcesByServer` is the committed-state map type the
// derivation consumes; referenced here so a signature drift surfaces in this
// suite too.
const _typeGuard: ResourcesByServer = { "server-local": localResources };
void _typeGuard;
