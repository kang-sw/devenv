import { browserStorage } from "../workRootFiles.js";

// CONTRACT: Persists a bounded, debounced browser-local snapshot of each
// terminal pane's serialized visual buffer (scrollback, cursor, styles) plus
// scroll viewport offset, keyed by `logicalKey`
// (`terminals.ts#terminalPaneLogicalKey`, already collision-safe across
// serverRoute + workRootId + terminal id). Mirrors `layoutRestore.ts`'s
// shape/defensiveness: a versioned `localStorage` blob, a pure load/save
// module independent of React. Unlike layout restore, this is a bounded
// cache tied to one pane's lifecycle - not a general raw-output store - so
// this module enforces the ticket's size bound directly at persist time:
// an entry whose serialized text exceeds the bound is dropped entirely
// (never truncated, which would risk cutting an entry mid-escape-sequence).

export type TerminalVisualRestoreEntry = {
  logicalKey: string;
  serialized: string;
  nextSequence: number;
  viewportY: number;
  capturedAtMs: number;
};

export type TerminalVisualRestoreSnapshot = Record<
  string,
  TerminalVisualRestoreEntry
>;

const terminalVisualRestoreStorageKey = "ws-dashboard.terminalVisual.v1";

// Hard cap on a single entry's serialized text length. Chosen generously
// above typical terminal-pane serialize output (a handful of screens of
// styled text) while still bounding worst-case localStorage growth across
// many open terminal panes.
export const terminalVisualRestoreMaxSerializedLength = 200_000;

// Number of scrollback lines requested from `SerializeAddon.serialize()` at
// capture time, so the addon itself never walks/serializes unlimited xterm
// buffer history for a long-lived terminal.
export const terminalVisualRestoreScrollbackLines = 2000;

// Debounce window (ms) the caller should use between qualifying pane.output
// changes and firing a capture, per the ticket's "writes debounced"
// constraint - terminal output can arrive many times per second, unlike
// Phase 5's infrequent discrete layout changes.
export const terminalVisualRestoreDebounceMs = 900;

export function loadTerminalVisualRestoreSnapshot(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): TerminalVisualRestoreSnapshot {
  if (!storage) {
    return {};
  }
  try {
    const raw = storage.getItem(terminalVisualRestoreStorageKey);
    if (!raw) {
      return {};
    }
    const parsed = JSON.parse(raw) as {
      version?: unknown;
      entries?: unknown;
    };
    if (parsed.version !== 1 || !Array.isArray(parsed.entries)) {
      return {};
    }
    const snapshot: TerminalVisualRestoreSnapshot = {};
    for (const value of parsed.entries) {
      const entry = parseTerminalVisualRestoreEntry(value);
      if (entry) {
        snapshot[entry.logicalKey] = entry;
      }
    }
    return snapshot;
  } catch {
    return {};
  }
}

export function saveTerminalVisualRestoreSnapshot(
  entries: readonly TerminalVisualRestoreEntry[],
  storage: Pick<Storage, "setItem" | "removeItem"> | null = browserStorage(),
) {
  if (!storage) {
    return;
  }
  const bounded = entries.filter(
    (entry) => entry.serialized.length <= terminalVisualRestoreMaxSerializedLength,
  );
  try {
    if (bounded.length === 0) {
      storage.removeItem(terminalVisualRestoreStorageKey);
      return;
    }
    storage.setItem(
      terminalVisualRestoreStorageKey,
      JSON.stringify({
        version: 1,
        entries: bounded.map((entry) => ({
          logicalKey: entry.logicalKey,
          serialized: entry.serialized,
          nextSequence: entry.nextSequence,
          viewportY: entry.viewportY,
          capturedAtMs: entry.capturedAtMs,
        })),
      }),
    );
  } catch {
    // Browser persistence is best-effort; live terminal state remains canonical.
  }
}

// Replace (or add) a single logical key's entry in an in-memory snapshot,
// leaving every other entry untouched. Pure, storage-independent upsert-by-
// key contract shared by every writer of a `TerminalVisualRestoreSnapshot`
// (Phase 7 review Test-partition finding): the localStorage-backed
// `upsertTerminalVisualRestoreEntry` below and App.tsx's live
// `terminalVisualRestoreRef` mirror write both call this instead of each
// re-implementing "overwrite by logicalKey" independently, so the two stay
// tied to one contract and cannot silently diverge.
export function upsertTerminalVisualRestoreEntryInSnapshot(
  snapshot: TerminalVisualRestoreSnapshot,
  entry: TerminalVisualRestoreEntry,
): TerminalVisualRestoreSnapshot {
  return { ...snapshot, [entry.logicalKey]: entry };
}

// Replace (or add) a single logical key's entry in the persisted snapshot,
// leaving every other entry untouched. Used by the per-pane debounced
// capture path so one terminal's capture never clobbers another's.
export function upsertTerminalVisualRestoreEntry(
  entry: TerminalVisualRestoreEntry,
  storage: (Pick<Storage, "getItem"> & Pick<Storage, "setItem" | "removeItem">) | null = browserStorage(),
) {
  const current = loadTerminalVisualRestoreSnapshot(storage);
  const merged = upsertTerminalVisualRestoreEntryInSnapshot(current, entry);
  saveTerminalVisualRestoreSnapshot(Object.values(merged), storage);
}

// Three-way selection of what `TerminalPaneBody`'s mount effect should write
// into a freshly opened xterm instance, extracted as a pure function so the
// branch selection (restore-snapshot vs. plain-text replay vs. no-op) is unit
// testable without a React/xterm harness. The actual `terminal.write(...)`/
// `terminal.scrollToLine(...)`/`writtenLengthRef` side effects stay at the
// call site - this only decides which of the three applies.
export type TerminalMountWrite =
  | { kind: "restore"; serialized: string; viewportY: number }
  | { kind: "replay"; text: string }
  | { kind: "none" };

export function resolveTerminalMountWrite(
  pane: { output: string },
  restoreEntry: Pick<TerminalVisualRestoreEntry, "serialized" | "viewportY"> | null | undefined,
): TerminalMountWrite {
  if (restoreEntry) {
    return {
      kind: "restore",
      serialized: restoreEntry.serialized,
      viewportY: restoreEntry.viewportY,
    };
  }
  if (pane.output.length > 0) {
    return { kind: "replay", text: pane.output };
  }
  return { kind: "none" };
}

// 260723 Phase 1 (load-bearing correctness fix): what `TerminalPaneBody`'s
// delta-write effect should do about `pane.output` on a given render, now
// that `pane.output` can be front-trimmed (see
// `TerminalPaneState.outputTrimOffset` in terminals.ts). Extracted as a pure
// function - same rationale as `resolveTerminalMountWrite` above - so the
// currentEnd/localStart/clamp arithmetic is unit testable without a React/
// xterm harness; the actual `terminal.clear()`/`terminal.write()`/ref-update
// side effects stay at the call site.
//
// `writtenAbsolute` is the absolute stream position (NOT a raw string
// length) of the last character already written to the emulator - the same
// absolute coordinate space `pane.outputTrimOffset` lives in, so
// `pane.outputTrimOffset + pane.output.length` (`currentEnd` below) is always
// directly comparable to it, independent of how many times `pane.output` has
// been front-trimmed since.
//
// - `currentEnd > writtenAbsolute`: new output arrived since the last write.
//   `localStart = writtenAbsolute - pane.outputTrimOffset` is where the
//   not-yet-written tail starts *within the current (possibly trimmed)*
//   `pane.output`. If `localStart` is still `>= 0`, every not-yet-written
//   character is still present in `pane.output` - write just that slice
//   (`kind: "tail"`), no clear needed, so no already-rendered content is
//   ever re-drawn or lost.
//   If `localStart` would be negative, a trim has evicted some characters
//   that were never written to the emulator (the pane fell far enough behind
//   that a front-trim outran the delta-write effect) - what is on screen can
//   no longer be trusted as a valid prefix of the *current* `pane.output`, so
//   this clears and redumps the entire retained `pane.output` (`kind:
//   "reset"`) rather than risk a silent gap.
// - `currentEnd < writtenAbsolute`: defensive parity with the pre-260723
//   effect's own "output got shorter" clear+redump branch - not reachable via
//   any current append/trim path (both only grow `currentEnd`), kept for the
//   same reason the original branch existed.
// - `currentEnd === writtenAbsolute`: nothing new - no-op.
export type TerminalDeltaWrite =
  | { kind: "noop"; nextWrittenAbsolute: number }
  | { kind: "tail"; text: string; nextWrittenAbsolute: number }
  | { kind: "reset"; text: string; nextWrittenAbsolute: number };

export function resolveTerminalDeltaWrite(
  pane: { output: string; outputTrimOffset: number },
  writtenAbsolute: number,
): TerminalDeltaWrite {
  const currentEnd = pane.outputTrimOffset + pane.output.length;
  if (currentEnd > writtenAbsolute) {
    const localStart = writtenAbsolute - pane.outputTrimOffset;
    if (localStart < 0) {
      return { kind: "reset", text: pane.output, nextWrittenAbsolute: currentEnd };
    }
    return {
      kind: "tail",
      text: pane.output.slice(localStart),
      nextWrittenAbsolute: currentEnd,
    };
  }
  if (currentEnd < writtenAbsolute) {
    return { kind: "reset", text: pane.output, nextWrittenAbsolute: currentEnd };
  }
  return { kind: "noop", nextWrittenAbsolute: writtenAbsolute };
}

function parseTerminalVisualRestoreEntry(
  value: unknown,
): TerminalVisualRestoreEntry | null {
  if (!value || typeof value !== "object") {
    return null;
  }
  const record = value as Record<string, unknown>;
  const logicalKey =
    typeof record.logicalKey === "string" ? record.logicalKey.trim() : "";
  if (!logicalKey) {
    return null;
  }
  const serialized =
    typeof record.serialized === "string" ? record.serialized : "";
  const nextSequence =
    typeof record.nextSequence === "number" &&
    Number.isFinite(record.nextSequence)
      ? record.nextSequence
      : 0;
  const viewportY =
    typeof record.viewportY === "number" && Number.isFinite(record.viewportY)
      ? record.viewportY
      : 0;
  const capturedAtMs =
    typeof record.capturedAtMs === "number" &&
    Number.isFinite(record.capturedAtMs)
      ? record.capturedAtMs
      : 0;
  if (serialized.length > terminalVisualRestoreMaxSerializedLength) {
    return null;
  }
  return { logicalKey, serialized, nextSequence, viewportY, capturedAtMs };
}
