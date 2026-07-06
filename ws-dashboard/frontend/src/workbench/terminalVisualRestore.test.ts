import {
  loadTerminalVisualRestoreSnapshot,
  saveTerminalVisualRestoreSnapshot,
  terminalVisualRestoreMaxSerializedLength,
  upsertTerminalVisualRestoreEntry,
  type TerminalVisualRestoreEntry,
} from "./terminalVisualRestore.js";

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

const smallEntry: TerminalVisualRestoreEntry = {
  logicalKey: "persistentTerminal/server-local/root-a/term-a",
  serialized: "\x1b[31mhello\x1b[0m",
  nextSequence: 12,
  viewportY: 3,
  capturedAtMs: 1000,
};

// Round-trip a small entry through save/load.
{
  const { storage } = fakeStorage();
  saveTerminalVisualRestoreSnapshot([smallEntry], storage);
  const loaded = loadTerminalVisualRestoreSnapshot(storage);
  assertDeepEqual(
    loaded[smallEntry.logicalKey],
    smallEntry,
    "a small entry round-trips through save/load unchanged",
  );
}

// Oversized entries are dropped, not truncated.
{
  const { storage } = fakeStorage();
  const oversizedEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    logicalKey: "persistentTerminal/server-local/root-a/term-oversized",
    serialized: "x".repeat(terminalVisualRestoreMaxSerializedLength + 1),
  };
  saveTerminalVisualRestoreSnapshot([smallEntry, oversizedEntry], storage);
  const loaded = loadTerminalVisualRestoreSnapshot(storage);
  assertEqual(
    Boolean(loaded[oversizedEntry.logicalKey]),
    false,
    "an oversized entry is dropped from persistence rather than truncated",
  );
  assertEqual(
    Boolean(loaded[smallEntry.logicalKey]),
    true,
    "a normal-sized sibling entry is still persisted alongside a dropped oversized one",
  );
}

// Saving an empty list clears any persisted snapshot.
{
  const { storage, backing } = fakeStorage();
  saveTerminalVisualRestoreSnapshot([smallEntry], storage);
  assertEqual(backing.size, 1, "a save with entries writes storage");
  saveTerminalVisualRestoreSnapshot([], storage);
  assertEqual(backing.size, 0, "saving an empty entry list clears storage");
}

// Malformed storage degrades to an empty snapshot.
{
  const { storage, backing } = fakeStorage();
  backing.set("ws-dashboard.terminalVisual.v1", "not json");
  assertDeepEqual(
    loadTerminalVisualRestoreSnapshot(storage),
    {},
    "malformed visual-restore storage degrades to empty",
  );
}

// upsertTerminalVisualRestoreEntry replaces only the matching logical key.
{
  const { storage } = fakeStorage();
  const otherEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    logicalKey: "persistentTerminal/server-local/root-b/term-b",
    serialized: "other",
  };
  saveTerminalVisualRestoreSnapshot([smallEntry, otherEntry], storage);
  const updatedEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    serialized: "updated",
    nextSequence: 99,
  };
  upsertTerminalVisualRestoreEntry(updatedEntry, storage);
  const loaded = loadTerminalVisualRestoreSnapshot(storage);
  assertDeepEqual(
    loaded[smallEntry.logicalKey],
    updatedEntry,
    "upsert replaces the matching logical key's entry",
  );
  assertDeepEqual(
    loaded[otherEntry.logicalKey],
    otherEntry,
    "upsert leaves an unrelated logical key's entry untouched",
  );
}

// A null storage (no window) is a safe no-op / empty read.
{
  saveTerminalVisualRestoreSnapshot([smallEntry], null);
  assertDeepEqual(
    loadTerminalVisualRestoreSnapshot(null),
    {},
    "a null storage handle degrades to an empty snapshot without throwing",
  );
}
