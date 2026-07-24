import {
  loadTerminalVisualRestoreSnapshot,
  saveTerminalVisualRestoreSnapshot,
  terminalVisualRestoreMaxSerializedLength,
  upsertTerminalVisualRestoreEntry,
  upsertTerminalVisualRestoreEntryInSnapshot,
  resolveTerminalDeltaWrite,
  resolveTerminalMountWrite,
  type TerminalVisualRestoreEntry,
  type TerminalVisualRestoreSnapshot,
} from "./terminalVisualRestore.js";
import {
  appendTerminalOutput,
  terminalOutputCharacterBudget,
  terminalPaneFromSession,
  type TerminalOutputView,
  type TerminalSessionView,
} from "../terminals.js";

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

// resolveTerminalDeltaWrite: the four deterministic branch cases in
// isolation, independent of any real pane/append history.
{
  assertDeepEqual(
    resolveTerminalDeltaWrite({ output: "abc", outputTrimOffset: 0 }, 3),
    { kind: "noop", nextWrittenAbsolute: 3 },
    "currentEnd === writtenAbsolute is a no-op",
  );
  assertDeepEqual(
    resolveTerminalDeltaWrite({ output: "abcdef", outputTrimOffset: 0 }, 3),
    { kind: "tail", text: "def", nextWrittenAbsolute: 6 },
    "ordinary growth with no trim writes just the new tail slice",
  );
  assertDeepEqual(
    resolveTerminalDeltaWrite({ output: "ab", outputTrimOffset: 0 }, 5),
    { kind: "reset", text: "ab", nextWrittenAbsolute: 2 },
    "currentEnd < writtenAbsolute (defensive parity with the pre-260723 shrink branch) forces a clear+redump",
  );
  // Clamped/reset scenario (260723 Phase 1 load-bearing case): writtenAbsolute
  // stayed behind while `outputTrimOffset` advanced well past it (the effect
  // fell far enough behind that a trim evicted characters before they were
  // ever written) - `localStart` would go negative, so the whole
  // currently-retained buffer must be redumped via a full clear+write rather
  // than slicing into content that no longer exists in `pane.output`.
  assertDeepEqual(
    resolveTerminalDeltaWrite({ output: "TAIL-CONTENT", outputTrimOffset: 500 }, 0),
    { kind: "reset", text: "TAIL-CONTENT", nextWrittenAbsolute: 512 },
    "a writtenAbsolute stuck before the current outputTrimOffset triggers a full clear+redump, never a negative/garbage slice",
  );
}

// 260723 Phase 1 - the load-bearing trim-boundary regression: drive a real
// pane through `appendTerminalOutput` far enough to trim multiple times
// (small, realistic per-poll chunk sizes against the actual production
// `terminalOutputCharacterBudget`), simulating the delta-write effect firing
// on every single append (the common case - React commits the state update
// and the effect runs before the next append arrives). The simulated
// "emulator" - built purely from `resolveTerminalDeltaWrite`'s decisions,
// exactly mirroring what `terminal.write`/`terminal.clear` would do - is
// compared against `fullStream`, the true never-trimmed source of everything
// ever appended, NOT against `pane.output`: a real xterm buffer keeps
// growing forever (bounded separately by its own line-based scrollback, not
// by this character budget), so it must still hold every character `pane`
// ever saw even after `pane.output` itself has been front-trimmed down to a
// bounded tail. Asserting `emulator === fullStream` at every step - through
// several trims - is exactly "no silent gap, no duplication": a bug that
// diffs against a stale/shifted index (the pre-260723 raw-length
// comparison) would either skip characters (a gap, emulator falls behind
// fullStream) or repeat them (emulator overtakes/duplicates fullStream).
{
  const trimBoundarySession: TerminalSessionView = {
    terminalId: "term_trim_boundary",
    workRootId: "root-trim-boundary",
    title: "Trim boundary",
    status: "running",
    columns: 80,
    rows: 24,
    createdAtMs: 1,
    cwdHint: null,
  };
  const chunkSize = 4_000;
  // Comfortably more chunks than needed to trim at least once, and to keep
  // trimming several times past that first trim.
  const totalChunks = Math.ceil((terminalOutputCharacterBudget * 1.5) / chunkSize);

  let pane = terminalPaneFromSession(trimBoundarySession);
  let writtenAbsolute = 0;
  let emulator = "";
  let fullStream = "";
  let resetCount = 0;

  for (let index = 0; index < totalChunks; index += 1) {
    // Deterministic, distinguishable-per-chunk filler so a duplicated or
    // skipped chunk would show up as a content mismatch, not just a length
    // mismatch.
    const marker = `|C${index}|`;
    const chunkData = (marker + "x".repeat(chunkSize)).slice(0, chunkSize);
    fullStream += chunkData;
    const output: TerminalOutputView = {
      terminalId: trimBoundarySession.terminalId,
      status: "running",
      nextSequence: index + 1,
      chunks: [{ sequence: index, data: chunkData, stream: "pty" }],
    };
    pane = appendTerminalOutput(pane, output);

    const resolved = resolveTerminalDeltaWrite(pane, writtenAbsolute);
    if (resolved.kind === "reset") {
      resetCount += 1;
      emulator = resolved.text;
    } else if (resolved.kind === "tail") {
      emulator += resolved.text;
    }
    writtenAbsolute = resolved.nextWrittenAbsolute;

    assertEqual(
      emulator,
      fullStream,
      `emulator content must exactly match the full never-trimmed stream after append ${index} (no gap, no duplication)`,
    );
  }

  assertEqual(
    pane.outputTrimOffset > 0,
    true,
    "the drive loop actually caused at least one trim (test precondition)",
  );
  assertEqual(
    resetCount,
    0,
    "an ordinary per-append trim (effect never falls behind) always resolves to the cheap tail write, never a spurious full clear+redump",
  );
  assertEqual(
    fullStream.endsWith(pane.output),
    true,
    "the retained pane.output is exactly the tail of the full untrimmed stream - nothing skipped, nothing corrupted",
  );
}
