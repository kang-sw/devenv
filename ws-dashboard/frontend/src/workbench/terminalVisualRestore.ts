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

// Replace (or add) a single logical key's entry in the persisted snapshot,
// leaving every other entry untouched. Used by the per-pane debounced
// capture path so one terminal's capture never clobbers another's.
export function upsertTerminalVisualRestoreEntry(
  entry: TerminalVisualRestoreEntry,
  storage: (Pick<Storage, "getItem"> & Pick<Storage, "setItem" | "removeItem">) | null = browserStorage(),
) {
  const current = loadTerminalVisualRestoreSnapshot(storage);
  saveTerminalVisualRestoreSnapshot(
    [
      ...Object.values(current).filter(
        (existing) => existing.logicalKey !== entry.logicalKey,
      ),
      entry,
    ],
    storage,
  );
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
