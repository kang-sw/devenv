import {
  LOCAL_DASHBOARD_SERVER_ROUTE,
  localCompatibleDashboardApiRoute,
  serverScopedIdentity,
} from "./resourceModel.js";
import { defaultPtyLogicalSize } from "./workbench/policy.js";
import type { TerminalVisualRestoreEntry } from "./workbench/terminalVisualRestore.js";

export type TerminalSessionView = {
  serverRoute?: string;
  terminalId: string;
  workRootId: string;
  title: string;
  status: "running" | "exited" | "terminated" | "error" | string;
  columns: number;
  rows: number;
  createdAtMs: number;
  cwdHint: string | null;
};

export type TerminalOutputChunk = {
  sequence: number;
  data: string;
  stream: "pty" | string;
};

export type TerminalOutputView = {
  terminalId: string;
  status: TerminalSessionView["status"];
  nextSequence: number;
  chunks: TerminalOutputChunk[];
};

// CONTRACT: Browser terminal panes use this message shape for live daemon to
// xterm traffic once WebSocket transport is connected. Output frames feed
// terminal.write(...) directly; HTTP output remains only replay/backfill/fallback.
export type TerminalWebSocketServerMessage =
  | { type: "output"; terminalId: string; chunk: TerminalOutputChunk }
  | {
      type: "status" | "exit";
      terminalId: string;
      status: TerminalSessionView["status"];
      nextSequence: number;
      truncated: boolean;
    };

// CONTRACT: xterm onData traffic is forwarded as raw input data over the live
// WebSocket, and resize messages preserve the daemon's bounded PTY size contract.
export type TerminalWebSocketClientMessage =
  | { type: "input"; data: string }
  | { type: "resize"; columns: number; rows: number };

export type TerminalPaneState = {
  session: TerminalSessionView;
  logicalKey: string;
  paneId: string;
  output: string;
  nextSequence: number;
  error: string | null;
  localCreatedAtMs: number;
  socketStatus: "disconnected" | "connecting" | "connected" | "fallback";
  // True while the pane's WebSocket is closed solely because the pane is not
  // currently visible (background work root, or a backgrounded dockview
  // tab within the active root) - not a real disconnect/error. Kept distinct
  // from `socketStatus` so the HTTP output-poll fallback can tell "closed on
  // purpose because hidden" apart from "closed for a real reason, needs
  // polling" (see `shouldPollTerminalOutput`).
  visibilityGated: boolean;
};

export type TerminalCreateOptions = {
  title?: string;
  cwdHint?: string | null;
};

export type TerminalRestoreIntent = {
  serverRoute?: string;
  workRootId: string;
  title: string;
  cwdHint: string | null;
  updatedAtMs: number;
};

// PTY logical size contract, mirrored from the daemon terminal registry
// (crates/daemon/src/terminal.rs MIN/MAX columns/rows). Resize forwarding must
// stay inside these bounds or the daemon rejects the request.
export const terminalSizeBounds = Object.freeze({
  minColumns: 1,
  maxColumns: 300,
  minRows: 1,
  maxRows: 120,
});

export function clampTerminalSize(columns: number, rows: number) {
  return {
    columns: Math.min(
      Math.max(Math.trunc(columns), terminalSizeBounds.minColumns),
      terminalSizeBounds.maxColumns,
    ),
    rows: Math.min(
      Math.max(Math.trunc(rows), terminalSizeBounds.minRows),
      terminalSizeBounds.maxRows,
    ),
  };
}

export function workRootTerminalsEndpoint(
  workRootId: string,
  serverRoute?: string | null,
) {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "work-roots",
    workRootId,
    "terminals",
  ]);
}

export function terminalOutputEndpoint(
  terminalId: string,
  after = 0,
  serverRoute?: string | null,
) {
  const query = new URLSearchParams({ after: String(after) });
  return `${localCompatibleDashboardApiRoute(serverRoute, ["terminals", terminalId, "output"])}?${query.toString()}`;
}

export function terminalInputEndpoint(
  terminalId: string,
  serverRoute?: string | null,
) {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "terminals",
    terminalId,
    "input",
  ]);
}

export function terminalResizeEndpoint(
  terminalId: string,
  serverRoute?: string | null,
) {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "terminals",
    terminalId,
    "resize",
  ]);
}

export function terminalWebSocketEndpoint(
  terminalId: string,
  after = 0,
  serverRoute?: string | null,
) {
  const query = new URLSearchParams({ after: String(after) });
  return `${localCompatibleDashboardApiRoute(serverRoute, ["terminals", terminalId, "socket"])}?${query.toString()}`;
}

export function terminalWebSocketUrl(
  terminalId: string,
  after = 0,
  locationLike = window.location,
  serverRoute?: string | null,
) {
  // HINT: Implementation should use this helper when attaching xterm panes and
  // tests should assert live panes do not continue periodic output polling once
  // this socket is open.
  const protocol = locationLike.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${locationLike.host}${terminalWebSocketEndpoint(terminalId, after, serverRoute)}`;
}

export function terminalWebSocketCursor(pane: TerminalPaneState) {
  return Math.max(0, pane.nextSequence - 1);
}

export function terminalCloseEndpoint(
  terminalId: string,
  serverRoute?: string | null,
) {
  return localCompatibleDashboardApiRoute(serverRoute, ["terminals", terminalId]);
}

export async function createTerminal(
  workRootId: string,
  options: TerminalCreateOptions = {},
  serverRoute?: string | null,
) {
  const response = await fetch(
    workRootTerminalsEndpoint(workRootId, serverRoute),
    {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        columns: defaultPtyLogicalSize.columns,
        rows: defaultPtyLogicalSize.rows,
        title: options.title ?? "Terminal",
        cwdHint: options.cwdHint ?? null,
      }),
    },
  );
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return {
    ...((await response.json()) as TerminalSessionView),
    serverRoute: serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE,
  };
}

export async function listTerminals(
  workRootId: string,
  serverRoute?: string | null,
) {
  const response = await fetch(
    workRootTerminalsEndpoint(workRootId, serverRoute),
    {
      headers: { Accept: "application/json" },
    },
  );
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return ((await response.json()) as TerminalSessionView[]).map((session) => ({
    ...session,
    serverRoute: session.serverRoute ?? serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE,
  }));
}

export async function fetchTerminalOutput(
  terminalId: string,
  after: number,
  serverRoute?: string | null,
) {
  const response = await fetch(
    terminalOutputEndpoint(terminalId, after, serverRoute),
    {
      headers: { Accept: "application/json" },
    },
  );
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return (await response.json()) as TerminalOutputView;
}

export async function sendTerminalInput(
  terminalId: string,
  data: string,
  serverRoute?: string | null,
) {
  const response = await fetch(terminalInputEndpoint(terminalId, serverRoute), {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify({ data }),
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
}

export async function resizeTerminal(
  terminalId: string,
  columns: number,
  rows: number,
  serverRoute?: string | null,
) {
  const size = validateTerminalSize(columns, rows);
  const response = await fetch(terminalResizeEndpoint(terminalId, serverRoute), {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(size),
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  const session = (await response.json()) as TerminalSessionView;
  return {
    ...session,
    serverRoute:
      session.serverRoute ?? serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE,
  };
}

export async function closeTerminal(
  terminalId: string,
  serverRoute?: string | null,
) {
  const response = await fetch(terminalCloseEndpoint(terminalId, serverRoute), {
    method: "DELETE",
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
}

export function terminalPaneLogicalKey(
  workRootId: string,
  terminalId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
) {
  return [
    "persistentTerminal",
    serverScopedIdentity(serverRoute, workRootId),
    terminalId,
  ].join("/");
}

export function terminalPaneId(
  terminalId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
) {
  return `terminal:${encodeURIComponent(serverScopedIdentity(serverRoute, terminalId))}`;
}

// Lookup consulted by `terminalPaneFromSession` (and its callers below) to
// seed a freshly reattached pane's `nextSequence` from a matching persisted
// visual-restore entry instead of `0`. Only `nextSequence` is needed here -
// the rest of `TerminalVisualRestoreEntry` (serialized buffer, viewport)
// is consumed directly by the xterm mount effect in App.tsx, not by this
// pane-state construction.
export type TerminalVisualRestoreLookup = Record<
  string,
  Pick<TerminalVisualRestoreEntry, "nextSequence">
>;

export function terminalPaneFromSession(
  session: TerminalSessionView,
  visualRestoreByLogicalKey?: TerminalVisualRestoreLookup,
): TerminalPaneState {
  const logicalKey = terminalPaneLogicalKey(
    session.workRootId,
    session.terminalId,
    session.serverRoute,
  );
  return {
    session,
    logicalKey,
    paneId: terminalPaneId(session.terminalId, session.serverRoute),
    output: "",
    // A reattached pane with a matching visual-restore entry resumes the
    // WebSocket cursor (`terminalWebSocketCursor`) from the sequence
    // captured alongside that entry's serialized buffer, instead of 0 - this
    // is what makes Phase 4's delta-cursor catch-up line up exactly against
    // a restored snapshot with no new socket-path code.
    nextSequence: visualRestoreByLogicalKey?.[logicalKey]?.nextSequence ?? 0,
    error: null,
    localCreatedAtMs: Date.now(),
    socketStatus: "disconnected",
    visibilityGated: false,
  };
}

export function terminalRestoreIntentsFromPanes(
  panes: TerminalPaneState[],
  nowMs = Date.now(),
): TerminalRestoreIntent[] {
  return panes
    .filter((pane) => pane.session.status === "running")
    .map((pane) => ({
      serverRoute: pane.session.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE,
      workRootId: pane.session.workRootId,
      title: pane.session.title,
      cwdHint: pane.session.cwdHint,
      updatedAtMs: nowMs,
    }));
}

export function terminalRestoreIntentsForWorkRoot(
  intents: TerminalRestoreIntent[],
  workRootId: string,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
): TerminalRestoreIntent[] {
  return intents.filter(
    (intent) =>
      (intent.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE) ===
        (serverRoute || LOCAL_DASHBOARD_SERVER_ROUTE) &&
      intent.workRootId === workRootId,
  );
}

export function replaceTerminalRestoreIntentsForWorkRoot(
  current: TerminalRestoreIntent[],
  workRootId: string,
  nextForRoot: TerminalRestoreIntent[],
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
): TerminalRestoreIntent[] {
  const scopedServerId = serverRoute || LOCAL_DASHBOARD_SERVER_ROUTE;
  return [
    ...current.filter(
      (intent) =>
        (intent.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE) !== scopedServerId ||
        intent.workRootId !== workRootId,
    ),
    ...nextForRoot.filter(
      (intent) =>
        (intent.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE) === scopedServerId &&
        intent.workRootId === workRootId,
    ),
  ];
}

export function loadTerminalRestoreIntents(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): TerminalRestoreIntent[] {
  if (!storage) {
    return [];
  }
  try {
    const raw = storage.getItem(terminalRestoreStorageKey);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw) as {
      version?: unknown;
      terminals?: unknown;
    };
    if (parsed.version !== 1 || !Array.isArray(parsed.terminals)) {
      return [];
    }
    return parsed.terminals.flatMap((value): TerminalRestoreIntent[] => {
      if (!value || typeof value !== "object") {
        return [];
      }
      const record = value as Record<string, unknown>;
      if (
        typeof record.workRootId !== "string" ||
        typeof record.title !== "string"
      ) {
        return [];
      }
      const cwdHint =
        typeof record.cwdHint === "string"
          ? record.cwdHint
          : record.cwdHint === null || record.cwdHint === undefined
            ? null
            : undefined;
      if (cwdHint === undefined) {
        return [];
      }
      return [
        {
          serverRoute:
            typeof record.serverRoute === "string"
              ? record.serverRoute.trim() || LOCAL_DASHBOARD_SERVER_ROUTE
              : LOCAL_DASHBOARD_SERVER_ROUTE,
          workRootId: record.workRootId,
          title: record.title.trim() || "Terminal",
          cwdHint: cwdHint?.trim() ? cwdHint.trim() : null,
          updatedAtMs:
            typeof record.updatedAtMs === "number" &&
            Number.isFinite(record.updatedAtMs)
              ? record.updatedAtMs
              : 0,
        },
      ];
    });
  } catch {
    return [];
  }
}

export function saveTerminalRestoreIntents(
  intents: TerminalRestoreIntent[],
  storage: Pick<Storage, "setItem" | "removeItem"> | null = browserStorage(),
) {
  if (!storage) {
    return;
  }
  const terminals = intents.filter((intent) => intent.workRootId.trim());
  try {
    if (terminals.length === 0) {
      storage.removeItem(terminalRestoreStorageKey);
      return;
    }
    storage.setItem(
      terminalRestoreStorageKey,
      JSON.stringify({ version: 1, terminals }),
    );
  } catch {
    // Browser persistence is best-effort; live terminal state remains canonical.
  }
}

export function reconcileListedTerminalSessions(
  current: Record<string, TerminalPaneState>,
  workRootId: string,
  sessions: TerminalSessionView[],
  pruneStartedAtMs = Number.POSITIVE_INFINITY,
  serverRoute: string | null | undefined = LOCAL_DASHBOARD_SERVER_ROUTE,
  visualRestoreByLogicalKey?: TerminalVisualRestoreLookup,
) {
  const liveKeys = new Set(
    sessions.map((session) =>
      terminalPaneLogicalKey(
        session.workRootId,
        session.terminalId,
        session.serverRoute,
      ),
    ),
  );
  const retained = Object.fromEntries(
    Object.entries(current).filter(
      ([key, pane]) =>
        pane.session.workRootId !== workRootId ||
        (pane.session.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE) !==
          (serverRoute || LOCAL_DASHBOARD_SERVER_ROUTE) ||
        liveKeys.has(key) ||
        pane.localCreatedAtMs > pruneStartedAtMs,
    ),
  );
  return mergeListedTerminalSessions(
    retained,
    sessions,
    visualRestoreByLogicalKey,
  );
}

export function mergeListedTerminalSessions(
  current: Record<string, TerminalPaneState>,
  sessions: TerminalSessionView[],
  visualRestoreByLogicalKey?: TerminalVisualRestoreLookup,
) {
  const next = { ...current };
  for (const session of sessions) {
    const key = terminalPaneLogicalKey(
      session.workRootId,
      session.terminalId,
      session.serverRoute,
    );
    next[key] = next[key]
      ? { ...next[key], session }
      : terminalPaneFromSession(session, visualRestoreByLogicalKey);
  }
  return next;
}

export function appendTerminalOutput(
  pane: TerminalPaneState,
  output: TerminalOutputView,
) {
  return {
    ...pane,
    session: { ...pane.session, status: output.status },
    output: pane.output + output.chunks.map((chunk) => chunk.data).join(""),
    nextSequence: output.nextSequence,
    error: null,
    localCreatedAtMs: Date.now(),
  };
}

export function markTerminalSocketStatus(
  pane: TerminalPaneState,
  socketStatus: TerminalPaneState["socketStatus"],
  error: string | null = pane.error,
) {
  return { ...pane, socketStatus, error, localCreatedAtMs: Date.now() };
}

export function markTerminalPaneVisibilityGated(
  pane: TerminalPaneState,
  visibilityGated: boolean,
) {
  if (pane.visibilityGated === visibilityGated) return pane;
  return { ...pane, visibilityGated };
}

// Advances pane.nextSequence from an individual output frame's own chunk
// sequence, mirroring the cursor math in appendTerminalWebSocketMessage's
// output branch. Needed because the live socket's message listener
// direct-writes "output" frames to xterm and never routes them through
// appendTerminalWebSocketMessage (see App.tsx applyTerminalSocketMessage),
// so without this the cursor only advanced on trailing status/exit frames -
// a race where a socket closed mid-batch left the cursor stale and caused
// duplicate output on resume.
export function markTerminalOutputCursor(
  pane: TerminalPaneState,
  chunkSequence: number,
): TerminalPaneState {
  const nextSequence = Math.max(pane.nextSequence, chunkSequence + 1);
  if (nextSequence === pane.nextSequence) return pane;
  return { ...pane, nextSequence, localCreatedAtMs: Date.now() };
}

// Applies a batch of pending per-logicalKey output-cursor advances
// (260723 Phase 1) in one pass, mirroring markTerminalOutputCursor's own
// no-op contract: if nothing in `pending` actually advances any pane's
// cursor (missing key, duplicate/already-covered sequence), the same
// `panes` object reference is returned so an all-no-op flush never forces
// an extra render. `pending` maps logicalKey -> the max chunkSequence seen
// for that key since the last flush (callers are responsible for
// collapsing duplicate/out-of-order chunks per key before calling this).
export function flushPendingOutputCursors(
  panes: Record<string, TerminalPaneState>,
  pending: ReadonlyMap<string, number>,
): Record<string, TerminalPaneState> {
  if (pending.size === 0) return panes;
  let next = panes;
  for (const [logicalKey, chunkSequence] of pending) {
    const pane = next[logicalKey];
    if (!pane) continue;
    const advanced = markTerminalOutputCursor(pane, chunkSequence);
    if (advanced === pane) continue;
    if (next === panes) next = { ...panes };
    next[logicalKey] = advanced;
  }
  return next;
}

// Extracted rAF output-cursor flush scheduler (260723 Phase 1, fix-cycle 1
// test-partition finding): owns the same "accumulate per-chunk advances,
// coalesce into at most one animation frame, flush synchronously at
// correctness-critical call sites, never re-apply an already-consumed batch"
// lifecycle App.tsx previously inlined as component-local
// (pendingOutputCursorRef / pendingOutputCursorFrameRef /
// flushPendingOutputCursorsNow / the unmount-cleanup effect). Extracting it
// here - mirroring how flushPendingOutputCursors itself was already pulled
// out of the per-chunk call site - lets a unit test drive the exact shipped
// scheduling logic (with a fake requestAnimationFrame/cancelAnimationFrame
// pair) instead of only a hand-written mirror of the algorithm. Deliberately
// does not import flushPendingOutputCursors itself: committing the batch is
// the caller's concern (App.tsx wires `applyBatch` to
// `setTerminalPanes((current) => flushPendingOutputCursors(current, pending))`),
// keeping this factory framework-agnostic (no React, no TerminalPaneState
// dependency on the write side).
export type OutputCursorFlushSchedulerDeps = {
  requestAnimationFrame: (callback: () => void) => number;
  cancelAnimationFrame: (handle: number) => void;
  // Receives the accumulated (logicalKey -> max chunkSequence) batch and
  // commits it. Called at most once per accumulate()-triggered frame, or
  // synchronously from flushNow().
  applyBatch: (pending: ReadonlyMap<string, number>) => void;
};

export type OutputCursorFlushScheduler = {
  // Records an "output" chunk's sequence for `logicalKey`, collapsing
  // duplicate/out-of-order sequences via Math.max (mirrors
  // flushPendingOutputCursors's own per-key contract), and schedules exactly
  // one animation frame per batch - a no-op if a frame is already scheduled.
  accumulate: (logicalKey: string, chunkSequence: number) => void;
  // Cancels any scheduled frame (a no-op if none is scheduled) and, only if
  // the pending batch is non-empty, hands it to applyBatch synchronously -
  // swapping in a fresh empty Map first so a stale/already-fired frame
  // callback can never re-apply an already-consumed batch. Safe to pass
  // directly as the requestAnimationFrame callback.
  flushNow: () => void;
  // Cancels a scheduled frame (if any) WITHOUT flushing - a defensive
  // teardown/unmount backstop so no batched flush ever fires against a
  // torn-down tree.
  cancel: () => void;
  // Read-side counterpart for a pane that may have an un-flushed pending
  // advance, mirroring markTerminalOutputCursor's chunkSequence + 1 math.
  pendingNextSequenceFor: (pane: TerminalPaneState) => number;
};

export function createOutputCursorFlushScheduler(
  deps: OutputCursorFlushSchedulerDeps,
): OutputCursorFlushScheduler {
  let pending = new Map<string, number>();
  let frameId: number | null = null;

  function flushNow() {
    if (frameId !== null) {
      deps.cancelAnimationFrame(frameId);
      frameId = null;
    }
    if (pending.size === 0) {
      return;
    }
    const batch = pending;
    pending = new Map();
    deps.applyBatch(batch);
  }

  return {
    accumulate(logicalKey, chunkSequence) {
      const existing = pending.get(logicalKey);
      pending.set(
        logicalKey,
        existing === undefined
          ? chunkSequence
          : Math.max(existing, chunkSequence),
      );
      if (frameId === null) {
        frameId = deps.requestAnimationFrame(flushNow);
      }
    },
    flushNow,
    cancel() {
      if (frameId !== null) {
        deps.cancelAnimationFrame(frameId);
        frameId = null;
      }
    },
    pendingNextSequenceFor(pane) {
      const value = pending.get(pane.logicalKey);
      return value === undefined
        ? pane.nextSequence
        : Math.max(pane.nextSequence, value + 1);
    },
  };
}

export function appendTerminalWebSocketMessage(
  pane: TerminalPaneState,
  message: TerminalWebSocketServerMessage,
): TerminalPaneState {
  if (message.terminalId !== pane.session.terminalId) {
    return pane;
  }
  if (message.type === "output") {
    return {
      ...pane,
      output: pane.output + message.chunk.data,
      nextSequence: Math.max(pane.nextSequence, message.chunk.sequence + 1),
      error: null,
      localCreatedAtMs: Date.now(),
    };
  }
  return {
    ...pane,
    session: { ...pane.session, status: message.status },
    output: message.truncated
      ? pane.output +
        "\r\n[terminal output gap: some history was not retained]\r\n"
      : pane.output,
    nextSequence: Math.max(pane.nextSequence, message.nextSequence),
    socketStatus: message.type === "exit" ? "fallback" : pane.socketStatus,
    error: null,
    localCreatedAtMs: Date.now(),
  };
}

export function shouldPollTerminalOutput(pane: TerminalPaneState) {
  return (
    pane.session.status === "running" &&
    !pane.visibilityGated &&
    pane.socketStatus !== "connecting" &&
    pane.socketStatus !== "connected"
  );
}

export function canApplyTerminalOutputPoll(
  pane: TerminalPaneState,
  requestedAfter: number,
) {
  return shouldPollTerminalOutput(pane) && pane.nextSequence === requestedAfter;
}

/**
 * Decide whether a successful terminal output poll changed anything worth a
 * React state update. A truly idle poll - no new chunks, unchanged status, no
 * cursor advancement, and no stale error to clear - is skipped so the
 * workbench does not re-render while terminals are quiet. A non-null
 * `pane.error` always counts as a change so a transient output failure is
 * cleared by the next successful poll instead of lingering.
 */
export function terminalOutputPollChangedState(
  pane: TerminalPaneState,
  output: TerminalOutputView,
): boolean {
  return (
    output.chunks.length > 0 ||
    output.status !== pane.session.status ||
    output.nextSequence !== pane.nextSequence ||
    pane.error !== null
  );
}

export function markTerminalPaneCloseError(
  current: Record<string, TerminalPaneState>,
  logicalKey: string,
  error: string,
) {
  const pane = current[logicalKey];
  return pane ? { ...current, [logicalKey]: { ...pane, error } } : current;
}

export function removeClosedTerminalPane(
  current: Record<string, TerminalPaneState>,
  logicalKey: string,
) {
  const next = { ...current };
  delete next[logicalKey];
  return next;
}

/**
 * Drop every terminal pane belonging to a specific work root/server pair.
 * Used when a work root is closed from the left panel: the browser-side
 * pane map entries are cleared even though the daemon terminal session
 * itself is left running (no `closeTerminal()` call here) so a future
 * reopen can reattach by id.
 */
export function removeTerminalPanesForWorkRoot(
  current: Record<string, TerminalPaneState>,
  rootId: string,
  serverRoute: string | undefined,
): Record<string, TerminalPaneState> {
  const normalizedServerRoute = serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE;
  const next: Record<string, TerminalPaneState> = {};
  for (const [key, pane] of Object.entries(current)) {
    const matches =
      pane.session.workRootId === rootId &&
      (pane.session.serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE) ===
        normalizedServerRoute;
    if (!matches) {
      next[key] = pane;
    }
  }
  return next;
}

export function validateTerminalSize(columns: number, rows: number) {
  if (
    !Number.isInteger(columns) ||
    !Number.isInteger(rows) ||
    columns < terminalSizeBounds.minColumns ||
    rows < terminalSizeBounds.minRows ||
    columns > terminalSizeBounds.maxColumns ||
    rows > terminalSizeBounds.maxRows
  ) {
    throw new Error("invalid terminal size");
  }
  return { columns, rows };
}

async function terminalErrorMessage(response: Response) {
  try {
    const value = (await response.json()) as { error?: unknown };
    if (typeof value.error === "string" && value.error.trim()) {
      return value.error;
    }
  } catch {
    // Fall through.
  }
  return `HTTP ${response.status}`;
}

const terminalRestoreStorageKey = "ws-dashboard.terminalRestore.v1";

function browserStorage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}
