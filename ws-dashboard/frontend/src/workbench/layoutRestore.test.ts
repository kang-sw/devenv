import {
  loadWorkbenchLayoutRestoreSnapshot,
  mergeReadOnlyAndTerminalPaneOrder,
  mergeWorkbenchLayoutRestoreEntries,
  pruneWorkbenchLayoutOrder,
  removePanesFromOrder,
  resolveRootLayout,
  revalidateWorkbenchLayoutForRoot,
  saveWorkbenchLayoutRestoreSnapshot,
  workbenchLayoutRestoreRootKey,
  type WorkbenchLayoutRestoreEntry,
  type WorkbenchLayoutRestoreSnapshot,
} from "./layoutRestore.js";

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

function fakeStorage() {
  const backing = new Map<string, string>();
  return {
    backing,
    storage: {
      getItem: (key: string) => backing.get(key) ?? null,
      setItem: (key: string, value: string) => {
        backing.set(key, value);
      },
      removeItem: (key: string) => {
        backing.delete(key);
      },
    },
  };
}

function entry(
  overrides: Partial<WorkbenchLayoutRestoreEntry> = {},
): WorkbenchLayoutRestoreEntry {
  return {
    serverRoute: "server-local",
    workRootId: "root-a",
    groups: [{ id: "group-1", label: "Group 1" }],
    paneOrderByGroup: { "group-1": ["pane-a", "pane-b"] },
    activePaneByGroup: { "group-1": "pane-a" },
    ...overrides,
  };
}

// Missing/absent storage degrades to an empty snapshot rather than throwing.
assertDeepEqual(
  loadWorkbenchLayoutRestoreSnapshot(null),
  {},
  "no storage available loads an empty snapshot",
);

// Round-trip: save then load returns an equivalent entry, keyed by
// serverScopedIdentity(serverRoute, workRootId).
{
  const { storage } = fakeStorage();
  const original = entry();
  saveWorkbenchLayoutRestoreSnapshot([original], storage);
  const loaded = loadWorkbenchLayoutRestoreSnapshot(storage);
  const rootKey = workbenchLayoutRestoreRootKey(original);
  assertEqual(rootKey, "server-local/root-a", "root key uses scoped identity");
  assertDeepEqual(
    loaded[rootKey],
    original,
    "layout entry round-trips through storage",
  );
}

// Collision safety: same workRootId on two different serverRoute values save
// and load independently without clobbering each other.
{
  const { storage } = fakeStorage();
  const entryA = entry({ serverRoute: "server-a", workRootId: "root-same" });
  const entryB = entry({
    serverRoute: "server-b",
    workRootId: "root-same",
    paneOrderByGroup: { "group-1": ["pane-c"] },
    activePaneByGroup: { "group-1": "pane-c" },
  });
  saveWorkbenchLayoutRestoreSnapshot([entryA, entryB], storage);
  const loaded = loadWorkbenchLayoutRestoreSnapshot(storage);
  assertEqual(
    Object.keys(loaded).length,
    2,
    "two serverRoute-scoped entries for the same workRootId both persist",
  );
  assertDeepEqual(
    loaded[workbenchLayoutRestoreRootKey(entryA)],
    entryA,
    "server-a entry is unaffected by the server-b entry",
  );
  assertDeepEqual(
    loaded[workbenchLayoutRestoreRootKey(entryB)],
    entryB,
    "server-b entry is unaffected by the server-a entry",
  );
}

// Saving an empty entry list clears any persisted snapshot.
{
  const { storage, backing } = fakeStorage();
  saveWorkbenchLayoutRestoreSnapshot([entry()], storage);
  assertEqual(backing.size, 1, "a non-empty save writes a storage entry");
  saveWorkbenchLayoutRestoreSnapshot([], storage);
  assertEqual(backing.size, 0, "saving no entries clears storage");
}

// Corrupt JSON degrades to an empty snapshot instead of throwing.
{
  const { storage, backing } = fakeStorage();
  backing.set("ws-dashboard.workbenchLayout.v1", "not json");
  assertDeepEqual(
    loadWorkbenchLayoutRestoreSnapshot(storage),
    {},
    "malformed layout restore storage degrades to empty",
  );
}

// Wrong version / malformed shape degrades to an empty snapshot.
{
  const { storage, backing } = fakeStorage();
  backing.set(
    "ws-dashboard.workbenchLayout.v1",
    JSON.stringify({ version: 2, entries: [entry()] }),
  );
  assertDeepEqual(
    loadWorkbenchLayoutRestoreSnapshot(storage),
    {},
    "unexpected version degrades to empty",
  );
}

// An entry with no workRootId is dropped rather than crashing the parse.
{
  const { storage, backing } = fakeStorage();
  backing.set(
    "ws-dashboard.workbenchLayout.v1",
    JSON.stringify({
      version: 1,
      entries: [{ serverRoute: "server-local", groups: [] }, entry()],
    }),
  );
  const loaded = loadWorkbenchLayoutRestoreSnapshot(storage);
  assertEqual(
    Object.keys(loaded).length,
    1,
    "entries missing a workRootId are dropped, valid entries still load",
  );
}

// Group size restore data survives a round-trip when present.
{
  const { storage } = fakeStorage();
  const withSize = entry({
    groupSizeById: { "group-1": { width: 480, height: 320 } },
  });
  saveWorkbenchLayoutRestoreSnapshot([withSize], storage);
  const loaded = loadWorkbenchLayoutRestoreSnapshot(storage);
  assertDeepEqual(
    loaded[workbenchLayoutRestoreRootKey(withSize)].groupSizeById,
    { "group-1": { width: 480, height: 320 } },
    "group size hints round-trip through storage",
  );
}

// `pruneWorkbenchLayoutOrder` drops pane ids that are not in the live set and
// drops any group left empty by that filter — the same degrade-by-drop rule
// as the read-only-file-pane restore path.
assertDeepEqual(
  pruneWorkbenchLayoutOrder(
    { "group-1": ["pane-a", "pane-missing"], "group-2": ["pane-gone"] },
    new Set(["pane-a"]),
  ),
  { "group-1": ["pane-a"] },
  "prune drops missing pane ids and empties out groups left with none live",
);
assertDeepEqual(
  pruneWorkbenchLayoutOrder({}, new Set(["pane-a"])),
  {},
  "prune of an empty order map stays empty",
);

// A persisted entry referencing a group id no longer present in `groups[]`
// (e.g. a group removed in a later session, or a corrupted single field
// rather than the whole payload) is dropped from `paneOrderByGroup`,
// `activePaneByGroup`, and `groupSizeById` at parse time, instead of
// resurrecting a group that no longer exists.
{
  const { storage } = fakeStorage();
  storage.setItem(
    "ws-dashboard.workbenchLayout.v1",
    JSON.stringify({
      version: 1,
      entries: [
        {
          serverRoute: "server-local",
          workRootId: "root-a",
          groups: [{ id: "group-1", label: "Group 1" }],
          paneOrderByGroup: {
            "group-1": ["pane-a"],
            "group-gone": ["pane-b"],
          },
          activePaneByGroup: {
            "group-1": "pane-a",
            "group-gone": "pane-b",
          },
          groupSizeById: {
            "group-1": { width: 480 },
            "group-gone": { width: 200 },
          },
        },
      ],
    }),
  );
  const loaded = loadWorkbenchLayoutRestoreSnapshot(storage);
  const rootKey = workbenchLayoutRestoreRootKey({
    serverRoute: "server-local",
    workRootId: "root-a",
  });
  assertDeepEqual(
    loaded[rootKey].paneOrderByGroup,
    { "group-1": ["pane-a"] },
    "paneOrderByGroup drops references to a group id absent from groups[]",
  );
  assertDeepEqual(
    loaded[rootKey].activePaneByGroup,
    { "group-1": "pane-a" },
    "activePaneByGroup drops references to a group id absent from groups[]",
  );
  assertDeepEqual(
    loaded[rootKey].groupSizeById,
    { "group-1": { width: 480 } },
    "groupSizeById drops references to a group id absent from groups[]",
  );
}

// `revalidateWorkbenchLayoutForRoot` combines prune + active-pane
// reconciliation into one pure (groups, orderForRoot, activePaneByGroup,
// livePaneIds, terminalsReady) -> {prunedOrder, reconciledActivePane}
// transformation, extracted from `WorkbenchShell`'s revalidation effect in
// App.tsx so the highest-restore-correctness-risk glue logic is unit
// testable.
{
  const groups = [
    { id: "group-1", label: "Group 1" },
    { id: "group-2", label: "Group 2" },
  ];
  const orderForRoot = {
    "group-1": ["readonly:pane-a", "readonly:pane-gone"],
    "group-2": ["terminal:pane-b"],
  };
  const activePaneByGroup = {
    "group-1": "readonly:pane-gone",
    "group-2": "terminal:pane-b",
  };

  // With terminals ready, a live-pane-id set missing both `pane-gone` and
  // the terminal pane prunes both: the file pane because it's genuinely
  // unavailable, and the terminal pane because its listing has resolved and
  // it is genuinely gone too. The active pane falls back to the remaining
  // live pane in each group.
  {
    const result = revalidateWorkbenchLayoutForRoot(
      groups,
      orderForRoot,
      activePaneByGroup,
      new Set(["readonly:pane-a"]),
      true,
    );
    assertDeepEqual(
      result.prunedOrder,
      { "group-1": ["readonly:pane-a"] },
      "terminals-ready: a genuinely-gone terminal pane is pruned like any other",
    );
    assertDeepEqual(
      result.reconciledActivePane,
      { "group-1": "readonly:pane-a" },
      "terminals-ready: active pane falls back off a pruned reference, group-2 has no live pane left",
    );
  }

  // With terminals NOT ready, the same live-pane-id set (which has no
  // terminal ids yet, since `listTerminals` hasn't resolved) must not prune
  // the restored terminal pane reference - only the file pane (seeded
  // synchronously at mount) is prunable immediately.
  {
    const result = revalidateWorkbenchLayoutForRoot(
      groups,
      orderForRoot,
      activePaneByGroup,
      new Set(["readonly:pane-a"]),
      false,
    );
    assertDeepEqual(
      result.prunedOrder,
      {
        "group-1": ["readonly:pane-a"],
        "group-2": ["terminal:pane-b"],
      },
      "terminals not ready: restored terminal pane reference survives the grace window",
    );
    assertDeepEqual(
      result.reconciledActivePane,
      { "group-1": "readonly:pane-a", "group-2": "terminal:pane-b" },
      "terminals not ready: active terminal pane reference is preserved, not reset",
    );
  }
}

// `mergeReadOnlyAndTerminalPaneOrder` merges a root's agent/activity pane
// order (`orderForRoot`) with the flat, cross-root readonly-file and
// terminal pane-order maps, each filtered to its own live-id set — the
// Phase 2 (260707-bug-dashboard-e2e-multi-root-locator-leakage) fix for a
// group whose live panes are only readonly-file/terminal panes looking
// pane-less to `revalidateWorkbenchLayoutForRoot`.
{
  const groups = [{ id: "group-1" }, { id: "group-2" }];

  // A group with no readonly/terminal entries keeps exactly its
  // `orderForRoot` order, unchanged.
  {
    const result = mergeReadOnlyAndTerminalPaneOrder(
      groups,
      { "group-1": ["agent-pane"] },
      {},
      new Set(),
      {},
      new Set(),
    );
    assertDeepEqual(
      result,
      { "group-1": ["agent-pane"], "group-2": [] },
      "no readonly/terminal entries: orderForRoot order is preserved as-is",
    );
  }

  // Live readonly and terminal panes for a group are appended (readonly
  // before terminal), on top of that group's own `orderForRoot` entries —
  // this is what makes a readonly/terminal-only group non-pane-less.
  {
    const result = mergeReadOnlyAndTerminalPaneOrder(
      groups,
      { "group-1": ["agent-pane"] },
      { "group-1": ["readonly:a"], "group-2": ["readonly:b"] },
      new Set(["readonly:a", "readonly:b"]),
      { "group-1": ["terminal:c"] },
      new Set(["terminal:c"]),
    );
    assertDeepEqual(
      result,
      {
        "group-1": ["agent-pane", "readonly:a", "terminal:c"],
        "group-2": ["readonly:b"],
      },
      "live readonly/terminal panes are merged in per group, readonly before terminal",
    );
  }

  // A stale readonly/terminal pane id present in the flat order map but
  // absent from the corresponding live-id set is dropped, not resurrected —
  // a closed pane's leftover order-map entry must never reappear in a live
  // group's pane list.
  {
    const result = mergeReadOnlyAndTerminalPaneOrder(
      groups,
      {},
      { "group-1": ["readonly:closed", "readonly:live"] },
      new Set(["readonly:live"]),
      { "group-1": ["terminal:closed"] },
      new Set(),
    );
    assertDeepEqual(
      result,
      { "group-1": ["readonly:live"], "group-2": [] },
      "stale readonly/terminal pane ids absent from the live set are dropped, not resurrected",
    );
  }
}

// Regression coverage for the caller-side `?? initialWorkbenchGroups`
// fallback bug this phase fixed: `revalidateWorkbenchLayoutForRoot`'s
// `groups` argument determines the `groupsWithPanes` reconstruction that
// `reconcileActiveWorkbenchPanes` reads — `prunedOrder` itself is
// group-list-independent (it only prunes by live pane id), but with an empty
// `groups` list (the old, buggy `workbenchGroupsByRoot[rootKey] ?? []`
// fallback) every group looks pane-less to the reconciliation step
// regardless of how many live panes actually exist in `orderForRoot`,
// silently dropping every group's active-pane entry. Passing the real
// default two-group list (the fixed `?? initialWorkbenchGroups` fallback)
// resolves correctly against the exact same order/live-id inputs.
{
  const orderForRoot = { "group-1": ["readonly:a"], "group-2": ["terminal:b"] };
  const activePaneByGroup = {
    "group-1": "readonly:a",
    "group-2": "terminal:b",
  };
  const livePaneIds = new Set(["readonly:a", "terminal:b"]);

  const buggyEmptyGroups = revalidateWorkbenchLayoutForRoot(
    [],
    orderForRoot,
    activePaneByGroup,
    livePaneIds,
    true,
  );
  assertDeepEqual(
    buggyEmptyGroups.reconciledActivePane,
    {},
    "old `?? []` fallback: every group's active-pane entry is silently dropped, even though prunedOrder still has live panes",
  );

  const fixedDefaultGroups = revalidateWorkbenchLayoutForRoot(
    [
      { id: "group-1", label: "Group 1" },
      { id: "group-2", label: "Group 2" },
    ],
    orderForRoot,
    activePaneByGroup,
    livePaneIds,
    true,
  );
  assertDeepEqual(
    fixedDefaultGroups.prunedOrder,
    orderForRoot,
    "fixed `?? initialWorkbenchGroups` fallback: live panes survive the prune",
  );
  assertDeepEqual(
    fixedDefaultGroups.reconciledActivePane,
    activePaneByGroup,
    "fixed `?? initialWorkbenchGroups` fallback: active-pane entries are preserved",
  );
}

// Composed regression coverage: a preferred active pane that exists only in
// the merged (readonly+terminal) order — not in the raw `orderForRoot` a
// caller might otherwise pass directly — must still be recognized as live by
// `revalidateWorkbenchLayoutForRoot`'s reconciliation, rather than falling
// back to `group.panes[0]`. This is the exact shape of the Phase 2 bug: a
// just-created second terminal pane's activation must resolve correctly once
// fed through the merge, not just when checked against `orderForRoot` alone.
{
  const groups = [{ id: "group-1", label: "Group 1" }];
  const mergedOrder = mergeReadOnlyAndTerminalPaneOrder(
    groups,
    {},
    {},
    new Set(),
    { "group-1": ["terminal:first", "terminal:second"] },
    new Set(["terminal:first", "terminal:second"]),
  );
  assertDeepEqual(
    mergedOrder,
    { "group-1": ["terminal:first", "terminal:second"] },
    "merged order includes both live terminal panes ahead of reconciliation",
  );
  const result = revalidateWorkbenchLayoutForRoot(
    groups,
    mergedOrder,
    { "group-1": "terminal:first" },
    new Set(["terminal:first", "terminal:second"]),
    true,
  );
  assertDeepEqual(
    result.reconciledActivePane,
    { "group-1": "terminal:first" },
    "a preferred pane present only via the merged order (not the caller's raw orderForRoot) is recognized as live and preserved",
  );
}

// `removePanesFromOrder` drops the given pane ids from every group's order,
// leaving unrelated pane ids and groups untouched. Used by the revalidation
// effect to strip readonly-file/terminal pane ids back out of a merged order
// before persisting into `paneOrderByRoot`, which must stay agnostic to
// those (owned separately by the flat order maps).
{
  const result = removePanesFromOrder(
    {
      "group-1": ["agent-a", "readonly:b", "terminal:c"],
      "group-2": ["readonly:b", "agent-d"],
    },
    ["readonly:b", "terminal:c"],
  );
  assertDeepEqual(
    result,
    { "group-1": ["agent-a"], "group-2": ["agent-d"] },
    "removePanesFromOrder drops the given ids from every group, keeping the rest in place",
  );

  // Removing every pane id from a group leaves an empty array for that
  // group rather than dropping the group key (unlike `pruneWorkbenchLayoutOrder`,
  // which drops empty groups) — callers that persist this result rely on
  // every original group key remaining present.
  const emptied = removePanesFromOrder(
    { "group-1": ["agent-a"] },
    ["agent-a"],
  );
  assertDeepEqual(
    emptied,
    { "group-1": [] },
    "removing every pane id from a group empties its array without dropping the group key",
  );
}

// `mergeWorkbenchLayoutRestoreEntries` is the layout save effect's
// merge/clobber-fix transformation, extracted from App.tsx (Phase 7 review
// Test-partition finding). It is the crux of this phase's clobber-bug fix:
// callers must feed each run's `mergedSnapshot` back in as the next run's
// `priorSnapshot` so a closed root's fallback reflects its last live state
// instead of a stale earlier snapshot.
{
  const rootRefs = {
    "server-local/root-a": { rootId: "root-a", serverRoute: "server-local" },
  };
  const groupsByRoot = {
    "server-local/root-a": [{ id: "group-1", label: "Group 1" }],
  };
  const paneOrderByRoot = {
    "server-local/root-a": { "group-1": ["pane-a"] },
  };
  const activePaneByRoot = {
    "server-local/root-a": { "group-1": "pane-a" },
  };
  const groupSizeByRoot = {};

  // (a) A rootKey removed from `openRootKeys` falls into the untouched
  // fallback sourced from the *live* prior snapshot (the previous call's
  // `mergedSnapshot`), not a stale earlier snapshot, across two consecutive
  // calls - this is the clobber-bug fix itself.
  {
    const firstRun = mergeWorkbenchLayoutRestoreEntries(
      ["server-local/root-a"],
      rootRefs,
      groupsByRoot,
      paneOrderByRoot,
      activePaneByRoot,
      groupSizeByRoot,
      {},
    );
    assertDeepEqual(
      firstRun.mergedEntries,
      [
        {
          serverRoute: "server-local",
          workRootId: "root-a",
          groups: [{ id: "group-1", label: "Group 1" }],
          paneOrderByGroup: { "group-1": ["pane-a"] },
          activePaneByGroup: { "group-1": "pane-a" },
        },
      ],
      "first run: the open root's live state is the sole merged entry",
    );

    // Simulate a second effect run after the root closed (no longer in
    // `openRootKeys`) but with an intervening edit to the root's live state
    // that a stale, earlier prior snapshot would not have seen. Feeding
    // `firstRun.mergedSnapshot` back in (the fix) must carry that edited
    // entry forward untouched, rather than a snapshot from before the edit.
    const editedPriorSnapshot: WorkbenchLayoutRestoreSnapshot = {
      "server-local/root-a": {
        serverRoute: "server-local",
        workRootId: "root-a",
        groups: [{ id: "group-1", label: "Group 1" }],
        paneOrderByGroup: { "group-1": ["pane-a", "pane-b"] },
        activePaneByGroup: { "group-1": "pane-b" },
      },
    };
    const secondRun = mergeWorkbenchLayoutRestoreEntries(
      [],
      rootRefs,
      groupsByRoot,
      paneOrderByRoot,
      activePaneByRoot,
      groupSizeByRoot,
      editedPriorSnapshot,
    );
    assertDeepEqual(
      secondRun.mergedEntries,
      [editedPriorSnapshot["server-local/root-a"]],
      "closed root falls back to the live prior snapshot's edited entry, not a stale earlier one",
    );
  }

  // (b) `workbenchLayoutRestoreRootKey` round-trips correctly when rebuilding
  // `mergedSnapshot` from `mergedEntries`.
  {
    const result = mergeWorkbenchLayoutRestoreEntries(
      ["server-local/root-a"],
      rootRefs,
      groupsByRoot,
      paneOrderByRoot,
      activePaneByRoot,
      groupSizeByRoot,
      {},
    );
    const expectedKey = workbenchLayoutRestoreRootKey({
      serverRoute: "server-local",
      workRootId: "root-a",
    });
    assertEqual(
      expectedKey,
      "server-local/root-a",
      "sanity: expected key matches the scoped identity format",
    );
    assertDeepEqual(
      Object.keys(result.mergedSnapshot),
      [expectedKey],
      "mergedSnapshot is keyed by workbenchLayoutRestoreRootKey(entry)",
    );
    assertDeepEqual(
      result.mergedSnapshot[expectedKey],
      result.mergedEntries[0],
      "mergedSnapshot[key] round-trips to the same entry object present in mergedEntries",
    );
  }

  // (c) A rootKey present in both the live-open set and the prior snapshot is
  // deduplicated in favor of the live entry: the prior snapshot's stale copy
  // for an open root must not survive into `mergedEntries`/`mergedSnapshot`
  // alongside (or instead of) the live one.
  {
    const stalePriorSnapshot: WorkbenchLayoutRestoreSnapshot = {
      "server-local/root-a": {
        serverRoute: "server-local",
        workRootId: "root-a",
        groups: [{ id: "group-1", label: "Group 1" }],
        paneOrderByGroup: { "group-1": ["stale-pane"] },
        activePaneByGroup: { "group-1": "stale-pane" },
      },
    };
    const result = mergeWorkbenchLayoutRestoreEntries(
      ["server-local/root-a"],
      rootRefs,
      groupsByRoot,
      paneOrderByRoot,
      activePaneByRoot,
      groupSizeByRoot,
      stalePriorSnapshot,
    );
    assertEqual(
      result.mergedEntries.length,
      1,
      "an open root present in both live and prior snapshot yields exactly one merged entry",
    );
    assertDeepEqual(
      result.mergedEntries[0].paneOrderByGroup,
      { "group-1": ["pane-a"] },
      "the live entry wins over the prior snapshot's stale copy for an open root",
    );
    assertDeepEqual(
      result.mergedSnapshot["server-local/root-a"].paneOrderByGroup,
      { "group-1": ["pane-a"] },
      "mergedSnapshot also reflects the live entry, not the stale prior copy",
    );
  }
}

// `resolveRootLayout` - render-time layout resolver (260714 Phase 1, D7).
// Test (e): a root mounted this render by the D1 union has no live state entry
// yet (the async seeding effects run one render later), so the restore
// snapshot must supply groups/paneOrder/activePane at render time. Precedence
// is live-state -> restore-snapshot -> caller default.
{
  const restoreKey = "server-local/root-a";
  const restored: WorkbenchLayoutRestoreEntry = entry({
    groups: [{ id: "group-1", label: "Group 1" }],
    paneOrderByGroup: { "group-1": ["pane-a", "pane-b"] },
    activePaneByGroup: { "group-1": "pane-b" },
  });
  const snapshot: WorkbenchLayoutRestoreSnapshot = { [restoreKey]: restored };

  // (e) All three live maps are still empty for this key on the mount render;
  // resolveRootLayout falls back to the restore snapshot for every field.
  const freshlyMounted = resolveRootLayout(restoreKey, {}, {}, {}, snapshot);
  assertDeepEqual(
    freshlyMounted.groups,
    restored.groups,
    "resolveRootLayout returns the restored groups when live state is empty",
  );
  assertDeepEqual(
    freshlyMounted.paneOrderByGroup,
    restored.paneOrderByGroup,
    "resolveRootLayout returns the restored paneOrderByGroup when live state is empty",
  );
  assertDeepEqual(
    freshlyMounted.activePaneByGroup,
    restored.activePaneByGroup,
    "resolveRootLayout returns the restored activePaneByGroup when live state is empty",
  );

  // Live state wins over the restore snapshot once it has an entry.
  const liveGroups = [{ id: "group-9", label: "Group 9" }];
  const livePaneOrder = { "group-9": ["pane-live"] };
  const liveActivePane = { "group-9": "pane-live" };
  const live = resolveRootLayout(
    restoreKey,
    { [restoreKey]: liveGroups },
    { [restoreKey]: livePaneOrder },
    { [restoreKey]: liveActivePane },
    snapshot,
  );
  assertDeepEqual(
    live.groups,
    liveGroups,
    "resolveRootLayout prefers live groups over the restore snapshot",
  );
  assertDeepEqual(
    live.paneOrderByGroup,
    livePaneOrder,
    "resolveRootLayout prefers live paneOrder over the restore snapshot",
  );
  assertDeepEqual(
    live.activePaneByGroup,
    liveActivePane,
    "resolveRootLayout prefers live activePane over the restore snapshot",
  );

  // No live entry and no restore entry: groups is null (caller applies its own
  // ultimate default), the two maps degrade to empty objects.
  const bare = resolveRootLayout("server-local/root-unknown", {}, {}, {}, {});
  assertEqual(
    bare.groups,
    null,
    "resolveRootLayout returns null groups when nothing live and nothing restored",
  );
  assertDeepEqual(
    bare.paneOrderByGroup,
    {},
    "resolveRootLayout returns an empty paneOrderByGroup when nothing live and nothing restored",
  );
  assertDeepEqual(
    bare.activePaneByGroup,
    {},
    "resolveRootLayout returns an empty activePaneByGroup when nothing live and nothing restored",
  );
}
