import {
  loadWorkbenchLayoutRestoreSnapshot,
  pruneWorkbenchLayoutOrder,
  saveWorkbenchLayoutRestoreSnapshot,
  workbenchLayoutRestoreRootKey,
  type WorkbenchLayoutRestoreEntry,
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
