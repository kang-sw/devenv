import {
  loadWorkbenchLayoutRestoreSnapshot,
  mergeWorkbenchLayoutRestoreEntries,
  pruneWorkbenchLayoutOrder,
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
