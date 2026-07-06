import {
  loadTerminalVisualRestoreSnapshot,
  saveTerminalVisualRestoreSnapshot,
  terminalVisualRestoreMaxSerializedLength,
  upsertTerminalVisualRestoreEntry,
  upsertTerminalVisualRestoreEntryInSnapshot,
  resolveTerminalMountWrite,
  type TerminalVisualRestoreEntry,
  type TerminalVisualRestoreSnapshot,
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

// Exact-boundary sizes: at maxLength the entry is kept, one char above is
// dropped (already covered above); one char below maxLength is also kept.
// Locks in the off-by-one in both directions around the `<=`/`>` comparisons.
{
  const { storage } = fakeStorage();
  const atMaxEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    logicalKey: "persistentTerminal/server-local/root-a/term-at-max",
    serialized: "x".repeat(terminalVisualRestoreMaxSerializedLength),
  };
  const belowMaxEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    logicalKey: "persistentTerminal/server-local/root-a/term-below-max",
    serialized: "x".repeat(terminalVisualRestoreMaxSerializedLength - 1),
  };
  saveTerminalVisualRestoreSnapshot([atMaxEntry, belowMaxEntry], storage);
  const loaded = loadTerminalVisualRestoreSnapshot(storage);
  assertEqual(
    Boolean(loaded[atMaxEntry.logicalKey]),
    true,
    "an entry exactly at the max serialized length is kept, not dropped",
  );
  assertEqual(
    Boolean(loaded[belowMaxEntry.logicalKey]),
    true,
    "an entry one char below the max serialized length is kept",
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

// `upsertTerminalVisualRestoreEntryInSnapshot` is the shared pure upsert-by-
// key contract both `upsertTerminalVisualRestoreEntry` (localStorage path,
// exercised above) and App.tsx's `terminalVisualRestoreRef` mirror write
// call, extracted so the two stay tied to one implementation (Phase 7 review
// Test-partition Minor finding). This test ties both call sites to the same
// observed behavior: overwrite-by-logicalKey, leave every other key
// untouched, no mutation of the input snapshot.
{
  const priorSnapshot: TerminalVisualRestoreSnapshot = {
    [smallEntry.logicalKey]: smallEntry,
  };
  const updatedEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    serialized: "ref-mirror-updated",
    nextSequence: 42,
  };
  const nextSnapshot = upsertTerminalVisualRestoreEntryInSnapshot(
    priorSnapshot,
    updatedEntry,
  );
  assertDeepEqual(
    nextSnapshot[smallEntry.logicalKey],
    updatedEntry,
    "the matching logical key's entry is replaced in the returned snapshot",
  );
  assertDeepEqual(
    priorSnapshot[smallEntry.logicalKey],
    smallEntry,
    "the input snapshot is left unmutated (a new object is returned)",
  );

  // Same helper, exercised against a snapshot holding an unrelated key, to
  // confirm the "leaves every other entry untouched" half of the contract
  // that App.tsx's ref-mirror write and the localStorage-backed upsert must
  // both preserve identically.
  const otherEntry: TerminalVisualRestoreEntry = {
    ...smallEntry,
    logicalKey: "persistentTerminal/server-local/root-b/term-b",
    serialized: "other",
  };
  const withOther = upsertTerminalVisualRestoreEntryInSnapshot(
    { [otherEntry.logicalKey]: otherEntry },
    updatedEntry,
  );
  assertDeepEqual(
    withOther,
    {
      [otherEntry.logicalKey]: otherEntry,
      [updatedEntry.logicalKey]: updatedEntry,
    },
    "an unrelated logical key's entry is preserved alongside the newly upserted one",
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

// resolveTerminalMountWrite: a matching restore entry takes priority over
// pane.output and yields the "restore" kind with the entry's serialized
// text/viewportY, regardless of pane.output's contents.
{
  const write = resolveTerminalMountWrite(
    { output: "should be ignored" },
    smallEntry,
  );
  assertDeepEqual(
    write,
    {
      kind: "restore",
      serialized: smallEntry.serialized,
      viewportY: smallEntry.viewportY,
    },
    "a matching restore entry yields the restore kind with its serialized/viewportY",
  );
}

// resolveTerminalMountWrite: no restore entry but non-empty pane.output
// yields the "replay" kind carrying the output text.
{
  const write = resolveTerminalMountWrite({ output: "hello world" }, undefined);
  assertDeepEqual(
    write,
    { kind: "replay", text: "hello world" },
    "no restore entry with non-empty pane.output yields the replay kind",
  );
}

// resolveTerminalMountWrite: no restore entry and empty pane.output yields
// the "none" kind (nothing to write).
{
  const write = resolveTerminalMountWrite({ output: "" }, null);
  assertDeepEqual(
    write,
    { kind: "none" },
    "no restore entry and empty pane.output yields the none kind",
  );
}
