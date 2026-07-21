import {
  applySiblingOrder,
  emptyWorkNavSiblingOrder,
  loadWorkNavOrderSnapshot,
  reorderSiblingIds,
  saveWorkNavOrderSnapshot,
  type WorkNavSiblingOrder,
} from "./workNavOrder.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const a = JSON.stringify(actual);
  const e = JSON.stringify(expected);
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

const items = [{ id: "a" }, { id: "b" }, { id: "c" }];

// No stored order (undefined) -> identity, natural/server-supplied order.
assertDeepEqual(
  applySiblingOrder(items, undefined),
  items,
  "undefined order returns items unchanged (identity)",
);

// Empty stored order -> identity as well (treated the same as "no custom
// order set").
assertDeepEqual(
  applySiblingOrder(items, []),
  items,
  "empty order array returns items unchanged (identity)",
);

// Partial order: named ids come first in the given sequence, the rest are
// appended in original order.
assertDeepEqual(
  applySiblingOrder(items, ["c", "a"]).map((item) => item.id),
  ["c", "a", "b"],
  "partial order places named ids first, then remaining items in original order",
);

// Order containing a stale/removed id: the dangling id is skipped, not
// inserted as a phantom entry.
assertDeepEqual(
  applySiblingOrder(items, ["z", "b", "a"]).map((item) => item.id),
  ["b", "a", "c"],
  "a stale/removed id in the stored order is skipped",
);

// Order missing a newly-added item: items not present in `order` are
// appended at the end in their original relative order.
assertDeepEqual(
  applySiblingOrder(items, ["b"]).map((item) => item.id),
  ["b", "a", "c"],
  "an item missing from the stored order is appended at the end",
);

// --- reorderSiblingIds ---

// Move forward (later in the list).
assertDeepEqual(
  reorderSiblingIds(["a", "b", "c"], "a", "c"),
  ["b", "a", "c"],
  "reorderSiblingIds moves an id forward, before the target id",
);

// Move backward (earlier in the list).
assertDeepEqual(
  reorderSiblingIds(["a", "b", "c"], "c", "a"),
  ["c", "a", "b"],
  "reorderSiblingIds moves an id backward, before the target id",
);

// Move to end (no beforeId).
assertDeepEqual(
  reorderSiblingIds(["a", "b", "c"], "a", undefined),
  ["b", "c", "a"],
  "reorderSiblingIds with no beforeId moves the id to the end",
);

// Move to end (beforeId not found in the list, e.g. a stale reference).
assertDeepEqual(
  reorderSiblingIds(["a", "b", "c"], "a", "does-not-exist"),
  ["b", "c", "a"],
  "reorderSiblingIds with an unresolvable beforeId moves the id to the end",
);

// Dropping onto itself is a no-op.
assertDeepEqual(
  reorderSiblingIds(["a", "b", "c"], "a", "a"),
  ["a", "b", "c"],
  "reorderSiblingIds is a no-op when sourceId === beforeId",
);

// sourceId not present in the effective order at all: returns the list
// unchanged (minus nothing, since it wasn't there), never inserting a
// phantom id.
assertDeepEqual(
  reorderSiblingIds(["a", "b", "c"], "z", "b"),
  ["a", "b", "c"],
  "reorderSiblingIds ignores a sourceId that is not in the effective order",
);

// --- load/save persistence round-trip ---

// Missing/absent storage degrades to the empty order rather than throwing.
assertDeepEqual(
  loadWorkNavOrderSnapshot(null),
  emptyWorkNavSiblingOrder,
  "no storage available loads the empty order",
);

{
  const { storage } = fakeStorage();
  const order: WorkNavSiblingOrder = {
    workspaceOrderByServer: { "server-local": ["ws-b", "ws-a"] },
    worktreeOrderByWorkspace: {
      "server-local/ws-a": ["root-c", "root-b", "root-a"],
    },
  };
  saveWorkNavOrderSnapshot(order, storage);
  const loaded = loadWorkNavOrderSnapshot(storage);
  assertDeepEqual(loaded, order, "order round-trips through storage");
}

// Saving an all-empty order removes the storage key rather than persisting
// an empty blob.
{
  const { storage, backing } = fakeStorage();
  saveWorkNavOrderSnapshot(
    {
      workspaceOrderByServer: { "server-local": ["ws-a"] },
      worktreeOrderByWorkspace: {},
    },
    storage,
  );
  assertTrue(backing.size === 1, "a non-empty order is persisted");
  saveWorkNavOrderSnapshot(emptyWorkNavSiblingOrder, storage);
  assertTrue(backing.size === 0, "saving the empty order clears storage");
}

function assertTrue(value: boolean, label: string) {
  if (!value) {
    throw new Error(`${label}: expected true`);
  }
}

// Back-compat: malformed/absent/wrong-version JSON resolves to the empty
// order, which `applySiblingOrder` then treats as "no custom order" -
// exactly the path an older store (written before this feature existed)
// takes.
{
  const { storage } = fakeStorage();
  storage.setItem("ws-dashboard.workNavOrder.v1", "not json");
  assertDeepEqual(
    loadWorkNavOrderSnapshot(storage),
    emptyWorkNavSiblingOrder,
    "malformed JSON loads the empty order",
  );
}
{
  const { storage } = fakeStorage();
  storage.setItem(
    "ws-dashboard.workNavOrder.v1",
    JSON.stringify({ version: 2, workspaceOrderByServer: {} }),
  );
  assertDeepEqual(
    loadWorkNavOrderSnapshot(storage),
    emptyWorkNavSiblingOrder,
    "a mismatched version loads the empty order",
  );
}
{
  const { storage } = fakeStorage();
  assertDeepEqual(
    loadWorkNavOrderSnapshot(storage),
    emptyWorkNavSiblingOrder,
    "absent storage entry loads the empty order",
  );
}

console.log("workNavOrder.test.ts passed");
