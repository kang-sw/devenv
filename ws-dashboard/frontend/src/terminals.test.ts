import {
  appendTerminalOutput,
  appendTerminalWebSocketMessage,
  canApplyTerminalOutputPoll,
  clampTerminalSize,
  closeTerminal,
  createTerminal,
  fetchTerminalOutput,
  flushPendingOutputCursors,
  listTerminals,
  createOutputCursorFlushScheduler,
  resizeTerminal,
  sendTerminalInput,
  markTerminalOutputCursor,
  markTerminalPaneCloseError,
  markTerminalPaneVisibilityGated,
  markTerminalSocketStatus,
  mergeListedTerminalSessions,
  loadTerminalRestoreIntents,
  reconcileListedTerminalSessions,
  removeClosedTerminalPane,
  removeTerminalPanesForWorkRoot,
  replaceTerminalRestoreIntentsForWorkRoot,
  saveTerminalRestoreIntents,
  terminalCloseEndpoint,
  terminalInputEndpoint,
  terminalOutputEndpoint,
  terminalOutputPollChangedState,
  terminalPaneFromSession,
  terminalPaneId,
  terminalPaneLogicalKey,
  terminalResizeEndpoint,
  terminalRestoreIntentsForWorkRoot,
  terminalRestoreIntentsFromPanes,
  terminalWebSocketEndpoint,
  terminalWebSocketUrl,
  terminalWebSocketCursor,
  shouldPollTerminalOutput,
  validateTerminalSize,
  workRootTerminalsEndpoint,
  type TerminalPaneState,
  type TerminalSessionView,
  type TerminalWebSocketServerMessage,
} from "./terminals.js";
import { LOCAL_DASHBOARD_SERVER_ROUTE } from "./resourceModel.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected)
    throw new Error(`${label}: expected ${expected}, got ${actual}`);
}
function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const a = JSON.stringify(actual),
    e = JSON.stringify(expected);
  if (a !== e) throw new Error(`${label}: expected ${e}, got ${a}`);
}
function assertThrows(action: () => unknown, pattern: RegExp, label: string) {
  try {
    action();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (pattern.test(message)) return;
    throw new Error(`${label}: ${message}`);
  }
  throw new Error(`${label}: expected throw`);
}

const session: TerminalSessionView = {
  terminalId: "term_abc",
  workRootId: "root-local-abc",
  title: "Terminal",
  status: "running",
  columns: 80,
  rows: 24,
  createdAtMs: 1,
  cwdHint: null,
};

assertEqual(
  workRootTerminalsEndpoint("root/local abc"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/terminals",
  "workRoot terminal endpoint encodes id",
);
assertEqual(
  terminalOutputEndpoint("term/abc", 12),
  "/api/dashboard/terminals/term%2Fabc/output?after=12",
  "output endpoint encodes cursor",
);
assertEqual(
  terminalInputEndpoint("term/abc"),
  "/api/dashboard/terminals/term%2Fabc/input",
  "input endpoint encodes id",
);
assertEqual(
  terminalResizeEndpoint("term/abc"),
  "/api/dashboard/terminals/term%2Fabc/resize",
  "resize endpoint encodes id",
);
assertEqual(
  terminalCloseEndpoint("term/abc"),
  "/api/dashboard/terminals/term%2Fabc",
  "close endpoint encodes id",
);
assertEqual(
  terminalWebSocketEndpoint("term/abc", 12),
  "/api/dashboard/terminals/term%2Fabc/socket?after=12",
  "websocket endpoint encodes id and cursor",
);
assertEqual(
  workRootTerminalsEndpoint("root/local abc", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Flocal%20abc/terminals",
  "server-scoped terminal create/list endpoint encodes server id",
);
assertEqual(
  terminalOutputEndpoint("term/abc", 12, "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/terminals/term%2Fabc/output?after=12",
  "server-scoped terminal output endpoint encodes server id",
);
assertEqual(
  terminalInputEndpoint("term/abc", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/terminals/term%2Fabc/input",
  "server-scoped terminal input endpoint encodes server id",
);
assertEqual(
  terminalResizeEndpoint("term/abc", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/terminals/term%2Fabc/resize",
  "server-scoped terminal resize endpoint encodes server id",
);
assertEqual(
  terminalCloseEndpoint("term/abc", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/terminals/term%2Fabc",
  "server-scoped terminal close endpoint encodes server id",
);
assertEqual(
  terminalWebSocketEndpoint("term/abc", 12, "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/terminals/term%2Fabc/socket?after=12",
  "server-scoped websocket endpoint encodes server id and cursor",
);
assertEqual(
  terminalWebSocketUrl("term/abc", 12, {
    protocol: "http:",
    host: "127.0.0.1:1234",
  } as Location),
  "ws://127.0.0.1:1234/api/dashboard/terminals/term%2Fabc/socket?after=12",
  "websocket URL uses ws for http",
);
assertEqual(
  terminalWebSocketUrl("term/abc", 0, {
    protocol: "https:",
    host: "example.test",
  } as Location),
  "wss://example.test/api/dashboard/terminals/term%2Fabc/socket?after=0",
  "websocket URL uses wss for https",
);
assertEqual(
  terminalPaneLogicalKey("root-local-abc", "term_abc"),
  "persistentTerminal/server-local/root-local-abc/term_abc",
  "logical key uses workRoot and terminal id",
);
assertEqual(
  terminalPaneId("term/abc"),
  "terminal:server-local%2Fterm%2Fabc",
  "pane id encodes terminal id",
);
assertEqual(
  String(terminalPaneLogicalKey("root-local-abc", "term_abc")).includes(
    "/Users/",
  ),
  false,
  "logical key omits host paths",
);
assertEqual(
  terminalPaneLogicalKey("root-same", "term-same", "server-a") ===
    terminalPaneLogicalKey("root-same", "term-same", "server-b"),
  false,
  "same bare terminal ids on different servers produce distinct logical keys",
);
assertEqual(
  terminalPaneId("term-same", "server-a") ===
    terminalPaneId("term-same", "server-b"),
  false,
  "same bare terminal ids on different servers produce distinct pane ids",
);

const pane = terminalPaneFromSession(session);

// Visual-restore nextSequence seeding: a reattached pane with a matching
// persisted entry resumes from the captured sequence instead of 0, and an
// unmatched/absent lookup leaves the existing 0-seeded behavior unchanged.
const restoredSessionPane = terminalPaneFromSession(session, {
  [terminalPaneLogicalKey(session.workRootId, session.terminalId, session.serverRoute)]:
    { nextSequence: 42 },
});
assertEqual(
  restoredSessionPane.nextSequence,
  42,
  "terminalPaneFromSession seeds nextSequence from a matching visual-restore entry",
);
assertEqual(
  terminalPaneFromSession(session, {}).nextSequence,
  0,
  "terminalPaneFromSession leaves nextSequence at 0 when no visual-restore entry matches",
);
assertEqual(
  terminalPaneFromSession(session, {
    "some-other-logical-key": { nextSequence: 99 },
  }).nextSequence,
  0,
  "terminalPaneFromSession ignores a visual-restore entry keyed to a different logical key",
);
const mergedWithVisualRestore = mergeListedTerminalSessions({}, [session], {
  [terminalPaneLogicalKey(session.workRootId, session.terminalId, session.serverRoute)]:
    { nextSequence: 7 },
});
assertEqual(
  mergedWithVisualRestore[pane.logicalKey]?.nextSequence,
  7,
  "mergeListedTerminalSessions threads the visual-restore lookup into freshly built panes",
);
assertEqual(
  mergeListedTerminalSessions(
    { [pane.logicalKey]: { ...pane, nextSequence: 3 } },
    [session],
    {
      [terminalPaneLogicalKey(session.workRootId, session.terminalId, session.serverRoute)]:
        { nextSequence: 7 },
    },
  )[pane.logicalKey]?.nextSequence,
  3,
  "mergeListedTerminalSessions does not re-seed nextSequence for an already-tracked pane",
);
const reconciledWithVisualRestore = reconcileListedTerminalSessions(
  {},
  session.workRootId,
  [session],
  Number.POSITIVE_INFINITY,
  undefined,
  {
    [terminalPaneLogicalKey(session.workRootId, session.terminalId, session.serverRoute)]:
      { nextSequence: 5 },
  },
);
assertEqual(
  reconciledWithVisualRestore[pane.logicalKey]?.nextSequence,
  5,
  "reconcileListedTerminalSessions threads the visual-restore lookup through to newly built panes",
);

assertEqual(pane.nextSequence, 0, "new pane starts at cursor zero");
assertEqual(
  pane.socketStatus,
  "disconnected",
  "new pane starts without a socket attachment",
);
assertEqual(
  shouldPollTerminalOutput(pane),
  true,
  "disconnected running panes remain eligible for HTTP fallback polling",
);
assertEqual(
  shouldPollTerminalOutput(markTerminalSocketStatus(pane, "connecting")),
  false,
  "connecting websocket panes suppress live HTTP output polling",
);
assertEqual(
  shouldPollTerminalOutput(markTerminalSocketStatus(pane, "connected")),
  false,
  "connected websocket panes suppress live HTTP output polling",
);
assertEqual(
  canApplyTerminalOutputPoll(pane, 0),
  true,
  "poll response can apply when pane cursor and polling state still match",
);
assertEqual(
  pane.visibilityGated,
  false,
  "new pane starts visible (not gated)",
);
assertEqual(
  shouldPollTerminalOutput(markTerminalPaneVisibilityGated(pane, true)),
  false,
  "a pane closed solely because it is hidden is excluded from HTTP fallback polling",
);
assertEqual(
  markTerminalPaneVisibilityGated(pane, true).visibilityGated,
  true,
  "marking a pane visibility-gated sets the flag",
);
assertEqual(
  markTerminalPaneVisibilityGated(pane, false) === pane,
  true,
  "marking a pane visibility-gated with no change returns the same reference",
);
assertEqual(
  shouldPollTerminalOutput(
    markTerminalSocketStatus(
      markTerminalPaneVisibilityGated(pane, false),
      "fallback",
      "terminal WebSocket failed",
    ),
  ),
  true,
  "a genuine socket error still polls once the pane becomes visible again (gate cleared)",
);
assertEqual(
  canApplyTerminalOutputPoll(markTerminalSocketStatus(pane, "connecting"), 0),
  false,
  "in-flight poll response is discarded once a socket starts connecting",
);
const merged = mergeListedTerminalSessions({}, [session]);
assertEqual(
  Boolean(merged[pane.logicalKey]),
  true,
  "listed live session reconstructs pane state",
);
assertDeepEqual(
  reconcileListedTerminalSessions(merged, "root-local-abc", []),
  {},
  "absent listed sessions for a workRoot are removed from pane state",
);
assertEqual(
  Boolean(
    reconcileListedTerminalSessions(
      { [pane.logicalKey]: { ...pane, localCreatedAtMs: 20 } },
      "root-local-abc",
      [],
      10,
    )[pane.logicalKey],
  ),
  true,
  "stale list responses preserve panes created after the list started",
);
const withOutput = appendTerminalOutput(pane, {
  terminalId: "term_abc",
  status: "running",
  nextSequence: 3,
  chunks: [{ sequence: 1, data: "hi", stream: "pty" }],
});
assertEqual(withOutput.output, "hi", "output appends chunk data");
assertEqual(withOutput.nextSequence, 3, "output advances cursor");
assertEqual(
  canApplyTerminalOutputPoll(withOutput, 0),
  false,
  "stale in-flight poll response is discarded after cursor advancement",
);
const withSocketOutput = appendTerminalWebSocketMessage(
  markTerminalSocketStatus(pane, "connected"),
  {
    type: "output",
    terminalId: "term_abc",
    chunk: { sequence: 4, data: " socket", stream: "pty" },
  },
);
assertEqual(
  withSocketOutput.output,
  " socket",
  "websocket output appends chunk data",
);
assertEqual(
  withSocketOutput.nextSequence,
  5,
  "websocket output advances cursor past chunk sequence",
);
assertEqual(
  terminalWebSocketCursor(withOutput),
  2,
  "socket cursor resumes from the last HTTP-observed sequence",
);
const withSocketExit = appendTerminalWebSocketMessage(withSocketOutput, {
  type: "exit",
  terminalId: "term_abc",
  status: "exited",
  nextSequence: 5,
  truncated: false,
});
assertEqual(
  withSocketExit.session.status,
  "exited",
  "websocket exit updates terminal status",
);
assertEqual(
  withSocketExit.socketStatus,
  "fallback",
  "websocket exit leaves pane in fallback state",
);
assertEqual(
  withSocketExit.output.includes("terminal output gap"),
  false,
  "non-truncated exit frame leaves output untouched",
);

const withTruncatedStatus = appendTerminalWebSocketMessage(withSocketOutput, {
  type: "status",
  terminalId: "term_abc",
  status: "running",
  nextSequence: 5,
  truncated: true,
});
assertEqual(
  withTruncatedStatus.output.includes(
    "[terminal output gap: some history was not retained]",
  ),
  true,
  "truncated status frame appends a visible gap marker to pane output",
);

const cursorAdvanced = markTerminalOutputCursor(pane, 4);
assertEqual(
  cursorAdvanced.nextSequence,
  5,
  "markTerminalOutputCursor advances the cursor past a higher chunk sequence",
);
assertEqual(
  markTerminalOutputCursor(cursorAdvanced, 1) === cursorAdvanced,
  true,
  "markTerminalOutputCursor is a no-op when the chunk sequence is already covered",
);
assertEqual(
  markTerminalOutputCursor(cursorAdvanced, 4) === cursorAdvanced,
  true,
  "markTerminalOutputCursor is a no-op at the exact boundary (chunkSequence === nextSequence - 1)",
);

// 260723 Phase 1 - flushPendingOutputCursors: App.tsx no longer calls
// setTerminalPanes synchronously per output chunk; it batches
// (logicalKey -> max chunkSequence seen) into a ref Map and flushes the
// whole batch through this helper once per animation frame (or
// synchronously at the correctness-critical call sites - see the
// close-mid-burst regression test below). These assertions pin the
// helper's own equivalence contract: N pending advances (including
// duplicate/out-of-order sequences for one key) collapse into a single
// patch, each pane's cursor lands at the max sequence seen for its key, an
// unrelated pane in the same batch advances independently, and an
// all-no-op flush (missing keys, or already-covered sequences) returns the
// exact same `panes` reference so it never forces an extra render.
const sessionB: TerminalSessionView = {
  terminalId: "term_def",
  workRootId: "root-local-def",
  title: "Terminal B",
  status: "running",
  columns: 80,
  rows: 24,
  createdAtMs: 1,
  cwdHint: null,
};
const paneA = terminalPaneFromSession(session);
const paneB = terminalPaneFromSession(sessionB);
const panesForFlush: Record<string, TerminalPaneState> = {
  [paneA.logicalKey]: paneA,
  [paneB.logicalKey]: paneB,
};

const pendingBatch = new Map<string, number>();
// Simulate several accumulate() calls landing within one animation frame -
// repeated, duplicate, and out-of-order chunk sequences for the same
// logicalKey - mirroring App.tsx's accumulation contract
// (`Math.max(existing ?? -1, chunkSequence)` per logicalKey).
for (const seq of [2, 0, 4, 1, 4]) {
  const existing = pendingBatch.get(paneA.logicalKey);
  pendingBatch.set(
    paneA.logicalKey,
    existing === undefined ? seq : Math.max(existing, seq),
  );
}
pendingBatch.set(paneB.logicalKey, 9);

const flushed = flushPendingOutputCursors(panesForFlush, pendingBatch);
assertEqual(
  flushed !== panesForFlush,
  true,
  "flushPendingOutputCursors returns a new panes object when any pending advance actually changes a cursor",
);
assertEqual(
  flushed[paneA.logicalKey].nextSequence,
  5,
  "flushPendingOutputCursors advances pane A to the max pending chunk sequence + 1, collapsing duplicate/out-of-order writes",
);
assertEqual(
  flushed[paneB.logicalKey].nextSequence,
  10,
  "flushPendingOutputCursors advances an unrelated pane B from the same batched flush independently",
);

assertEqual(
  flushPendingOutputCursors(panesForFlush, new Map()) === panesForFlush,
  true,
  "flushPendingOutputCursors short-circuits to the same panes reference for an empty pending map",
);
assertEqual(
  flushPendingOutputCursors(
    panesForFlush,
    new Map([["missing-logical-key", 3]]),
  ) === panesForFlush,
  true,
  "flushPendingOutputCursors is a no-op (same reference) when every pending key is missing from panes",
);
assertEqual(
  flushPendingOutputCursors(flushed, new Map([[paneA.logicalKey, 1]])) ===
    flushed,
  true,
  "flushPendingOutputCursors returns the same panes reference when every pending advance is already covered by the current cursor",
);

// Close-mid-burst regression (260723 Phase 1's correctness requirement):
// an "output" chunk (sequence 9) lands in the pending batch but has not
// yet been flushed into pane.nextSequence, and is immediately followed by
// an "exit" socket message whose own reported nextSequence (5) lags behind
// that already-delivered chunk - the same class of stale-trailing-frame
// race markTerminalOutputCursor's own doc comment documents.
// applyTerminalSocketMessage's non-"output" branch must flush the pending
// batch (flushPendingOutputCursorsNow) BEFORE merging the exit message via
// appendTerminalWebSocketMessage, so the final cursor is the true max
// across both sources, never regressing to the (possibly stale)
// message.nextSequence alone.
const burstPane = terminalPaneFromSession(session);
const burstPending = new Map<string, number>([[burstPane.logicalKey, 9]]);
const burstPanes: Record<string, TerminalPaneState> = {
  [burstPane.logicalKey]: burstPane,
};
const exitMessage: TerminalWebSocketServerMessage = {
  type: "exit",
  terminalId: burstPane.session.terminalId,
  status: "exited",
  nextSequence: 5,
  truncated: false,
};

const withoutFlush = appendTerminalWebSocketMessage(
  burstPanes[burstPane.logicalKey],
  exitMessage,
);
assertEqual(
  withoutFlush.nextSequence,
  5,
  "regression baseline: merging the exit message without first flushing the pending batch loses the already-delivered chunk 9's cursor advance",
);

const burstFlushed = flushPendingOutputCursors(burstPanes, burstPending);
const withFlush = appendTerminalWebSocketMessage(
  burstFlushed[burstPane.logicalKey],
  exitMessage,
);
assertEqual(
  withFlush.nextSequence,
  10,
  "flush-before-merge (App.tsx's non-output branch order) preserves the batched chunk's cursor advance across a mid-burst close/exit message",
);

// rAF scheduling-lifecycle contract (260723 Phase 1, fix-cycle 1 test-
// partition finding): the prior version of this block hand-reimplemented
// the scheduling algorithm against a fake rAF pair instead of driving the
// shipped closures, so a regression introduced only in App.tsx's real
// wiring (a broken cancel/null sequence, a dropped unmount-cleanup effect)
// would not have been caught by any test. createOutputCursorFlushScheduler
// (terminals.ts) is now the single implementation of that lifecycle - both
// App.tsx's outputCursorFlushSchedulerRef and this test construct it via the
// same factory, so this block exercises the actual shipped code, not a
// mirror of it. Uses a deterministic fake requestAnimationFrame/
// cancelAnimationFrame pair (no browser/jsdom dependency) to pin: (a) at
// most one frame scheduled per batch; (b) a synchronous flushNow() cancels
// the pending frame and nulls the internal frame id (observed indirectly:
// a subsequent accumulate() schedules a genuinely new frame, which is only
// possible if the internal id was reset to null - if flushNow forgot the
// cancel/null step, the internal id would stay set and no further frame
// would ever be requested); (c) cancel() (the unmount-cleanup path) cancels
// an in-flight frame without applying its batch; (d) a late/already-fired
// frame callback can never re-apply an already-consumed batch, because
// flushNow swaps in a fresh empty pending Map before calling applyBatch.
{
  let requestedFrameCount = 0;
  let nextFrameId = 1;
  const scheduled = new Map<number, () => void>();
  const cancelled = new Set<number>();
  function fakeRequestAnimationFrame(cb: () => void): number {
    requestedFrameCount += 1;
    const id = nextFrameId++;
    scheduled.set(id, cb);
    return id;
  }
  function fakeCancelAnimationFrame(id: number): void {
    cancelled.add(id);
    scheduled.delete(id);
  }

  const applied: Array<Map<string, number>> = [];
  const scheduler = createOutputCursorFlushScheduler({
    requestAnimationFrame: fakeRequestAnimationFrame,
    cancelAnimationFrame: fakeCancelAnimationFrame,
    applyBatch: (pending) => applied.push(new Map(pending)),
  });

  // (a) at most one frame scheduled per batch: repeated/duplicate/
  // out-of-order accumulates for the same and a second logicalKey, within
  // one un-flushed batch, must not request more than one frame.
  scheduler.accumulate("k1", 1);
  assertEqual(requestedFrameCount, 1, "the first accumulate schedules a frame");
  scheduler.accumulate("k1", 2);
  scheduler.accumulate("k2", 5);
  assertEqual(
    requestedFrameCount,
    1,
    "further accumulates within the same batch reuse the already-scheduled frame",
  );
  assertEqual(
    scheduled.size,
    1,
    "only one frame is actually pending in the fake scheduler",
  );

  // (b) synchronous flushNow() cancels the pending frame and nulls the
  // frame id. Nulling is not directly observable, so it is pinned via its
  // required consequence: the very next accumulate() must request a
  // genuinely new frame. If the cancel-or-null step were dropped, the
  // internal frame id would remain set forever and this next assertion
  // (requestedFrameCount advancing to 2) would fail.
  scheduler.flushNow();
  assertEqual(applied.length, 1, "the synchronous flush actually applied a batch");
  assertDeepEqual(
    Object.fromEntries(applied[0]),
    { k1: 2, k2: 5 },
    "the synchronous flush applies the accumulated max-per-key batch",
  );
  assertEqual(
    cancelled.has(1),
    true,
    "the synchronous flush cancels the previously-scheduled frame",
  );
  assertEqual(
    scheduled.size,
    0,
    "no frame remains scheduled after the synchronous flush",
  );

  scheduler.accumulate("k3", 7);
  assertEqual(
    requestedFrameCount,
    2,
    "a fresh frame is requested for the post-flush batch, proving the frame id was nulled by the prior flushNow (this fails if the cancel/null step is removed)",
  );

  // (c) cancel() (mirrors the App-level unmount-cleanup effect) cancels an
  // in-flight frame without applying its pending batch.
  assertEqual(scheduled.size, 1, "a frame is pending before cancel()");
  scheduler.cancel();
  assertEqual(
    cancelled.has(2),
    true,
    "cancel() cancels the scheduled frame",
  );
  assertEqual(scheduled.size, 0, "no frame remains scheduled after cancel()");
  assertEqual(
    applied.length,
    1,
    "cancel() does not apply the pending batch (only flushNow/the rAF callback does)",
  );

  // (d) a late/already-fired frame cannot re-apply an already-consumed
  // batch. Capture the exact callback requestAnimationFrame was given for a
  // fresh batch, flush it synchronously (as the real correctness call
  // sites do, ahead of the frame firing), then invoke the captured callback
  // directly - simulating the frame firing anyway despite being cancelled.
  // The fresh-empty-Map swap inside flushNow means this must be a no-op.
  scheduler.accumulate("k4", 3);
  const capturedCallback = scheduled.get(3);
  assertEqual(
    capturedCallback !== undefined,
    true,
    "the third requested frame's callback was captured for the late-fire simulation",
  );
  scheduler.flushNow();
  assertEqual(applied.length, 2, "the second synchronous flush applied its own batch");
  assertDeepEqual(
    Object.fromEntries(applied[1]),
    { k3: 7, k4: 3 },
    // cancel() only cancels the scheduled frame - it deliberately does not
    // discard the pending Map (only flushNow's fresh-empty-Map swap does),
    // so k3's un-flushed advance (accumulated before cancel()) survives and
    // is included in this next flush alongside k4.
    "the second flush applies every advance accumulated since the last flush, including one accumulated before an intervening cancel()",
  );
  capturedCallback?.();
  assertEqual(
    applied.length,
    2,
    "a late/already-fired frame callback cannot re-apply an already-consumed batch",
  );

  // pendingNextSequenceFor: read-side counterpart, exercised against a
  // pane whose logicalKey has (and has not) an un-flushed pending advance.
  scheduler.accumulate("k5", 4);
  assertEqual(
    scheduler.pendingNextSequenceFor({
      ...paneA,
      logicalKey: "k5",
      nextSequence: 0,
    }),
    5,
    "pendingNextSequenceFor reflects an un-flushed pending advance (chunkSequence + 1)",
  );
  assertEqual(
    scheduler.pendingNextSequenceFor({
      ...paneA,
      logicalKey: "k5",
      nextSequence: 9,
    }),
    9,
    "pendingNextSequenceFor never regresses below the pane's own current cursor",
  );
  assertEqual(
    scheduler.pendingNextSequenceFor({
      ...paneA,
      logicalKey: "no-pending-entry",
      nextSequence: 3,
    }),
    3,
    "pendingNextSequenceFor falls back to pane.nextSequence when there is no pending entry",
  );
  scheduler.cancel();
}

const idleOutput = {
  terminalId: "term_abc",
  status: "running",
  nextSequence: 0,
  chunks: [],
};
assertEqual(
  terminalOutputPollChangedState(pane, idleOutput),
  false,
  "idle poll with no chunks, status, or cursor change is skipped",
);
assertEqual(
  terminalOutputPollChangedState(
    { ...pane, error: "terminal output failed" },
    idleOutput,
  ),
  true,
  "successful idle poll clears a stale error instead of leaving it",
);
assertEqual(
  terminalOutputPollChangedState(pane, {
    ...idleOutput,
    chunks: [{ sequence: 1, data: "x", stream: "pty" }],
  }),
  true,
  "new chunks count as a state change",
);
assertEqual(
  terminalOutputPollChangedState(pane, { ...idleOutput, status: "exited" }),
  true,
  "a status change counts as a state change",
);
assertEqual(
  terminalOutputPollChangedState(pane, { ...idleOutput, nextSequence: 5 }),
  true,
  "cursor advancement without chunks still counts as a state change",
);
assertDeepEqual(
  removeClosedTerminalPane(merged, pane.logicalKey),
  {},
  "close success removes pane state",
);
assertEqual(
  markTerminalPaneCloseError(merged, pane.logicalKey, "close failed")[
    pane.logicalKey
  ]?.error,
  "close failed",
  "close failure preserves pane state with error",
);

const otherRootSession: TerminalSessionView = {
  ...session,
  terminalId: "term_other_root",
  workRootId: "root-local-other",
};
const otherServerSession: TerminalSessionView = {
  ...session,
  terminalId: "term_other_server",
  serverRoute: "server-remote-1",
};
const multiRootPanes = mergeListedTerminalSessions({}, [
  session,
  otherRootSession,
  otherServerSession,
]);
assertDeepEqual(
  Object.keys(
    removeTerminalPanesForWorkRoot(
      multiRootPanes,
      session.workRootId,
      session.serverRoute,
    ),
  ).sort(),
  [
    terminalPaneFromSession(otherRootSession).logicalKey,
    terminalPaneFromSession(otherServerSession).logicalKey,
  ].sort(),
  "removeTerminalPanesForWorkRoot removes only matching-root, matching-server entries",
);
assertDeepEqual(
  removeTerminalPanesForWorkRoot(
    multiRootPanes,
    otherRootSession.workRootId,
    "server-remote-1",
  ),
  multiRootPanes,
  "removeTerminalPanesForWorkRoot leaves entries untouched when server route does not match",
);
assertDeepEqual(
  removeTerminalPanesForWorkRoot({}, session.workRootId, session.serverRoute),
  {},
  "removeTerminalPanesForWorkRoot is a no-op on an empty pane map",
);

const restoreIntents = terminalRestoreIntentsFromPanes(
  [
    pane,
    terminalPaneFromSession({
      ...session,
      terminalId: "term_nested",
      title: "Nested",
      cwdHint: "nested",
    }),
  ],
  123,
);
assertDeepEqual(
  restoreIntents.map((intent) => ({
    workRootId: intent.workRootId,
    title: intent.title,
    cwdHint: intent.cwdHint,
    updatedAtMs: intent.updatedAtMs,
  })),
  [
    {
      workRootId: "root-local-abc",
      title: "Terminal",
      cwdHint: null,
      updatedAtMs: 123,
    },
    {
      workRootId: "root-local-abc",
      title: "Nested",
      cwdHint: "nested",
      updatedAtMs: 123,
    },
  ],
  "restore intents capture browser-visible terminal tab context",
);
assertDeepEqual(
  terminalRestoreIntentsForWorkRoot(restoreIntents, "root-local-abc").map(
    (intent) => intent.title,
  ),
  ["Terminal", "Nested"],
  "restore intents filter by workRoot",
);
assertDeepEqual(
  replaceTerminalRestoreIntentsForWorkRoot(
    [{ workRootId: "other", title: "Other", cwdHint: null, updatedAtMs: 1 }],
    "root-local-abc",
    restoreIntents,
  ).map((intent) => intent.title),
  ["Other", "Terminal", "Nested"],
  "replace restore intents updates one workRoot without dropping others",
);

const fakeStorage = new Map<string, string>();
const storage = {
  getItem: (key: string) => fakeStorage.get(key) ?? null,
  setItem: (key: string, value: string) => {
    fakeStorage.set(key, value);
  },
  removeItem: (key: string) => {
    fakeStorage.delete(key);
  },
};
saveTerminalRestoreIntents(restoreIntents, storage);
assertDeepEqual(
  loadTerminalRestoreIntents(storage).map((intent) => ({
    workRootId: intent.workRootId,
    title: intent.title,
    cwdHint: intent.cwdHint,
  })),
  [
    { workRootId: "root-local-abc", title: "Terminal", cwdHint: null },
    { workRootId: "root-local-abc", title: "Nested", cwdHint: "nested" },
  ],
  "terminal restore intents round-trip through storage",
);
fakeStorage.set("ws-dashboard.terminalRestore.v1", "not json");
assertDeepEqual(
  loadTerminalRestoreIntents(storage),
  [],
  "malformed restore storage degrades to empty",
);

assertDeepEqual(
  validateTerminalSize(100, 30),
  { columns: 100, rows: 30 },
  "valid resize accepted",
);
assertThrows(
  () => validateTerminalSize(0, 30),
  /invalid terminal size/,
  "non-positive columns rejected",
);
assertThrows(
  () => validateTerminalSize(1000, 30),
  /invalid terminal size/,
  "oversized columns rejected",
);

assertDeepEqual(
  clampTerminalSize(0, 0),
  { columns: 1, rows: 1 },
  "clamp raises below-min columns and rows to the minimum",
);
assertDeepEqual(
  clampTerminalSize(1000, 30),
  { columns: 300, rows: 30 },
  "clamp caps oversized columns at the max",
);
assertDeepEqual(
  clampTerminalSize(80, 1000),
  { columns: 80, rows: 120 },
  "clamp caps oversized rows at the max",
);
assertDeepEqual(
  clampTerminalSize(120.9, 40.7),
  { columns: 120, rows: 40 },
  "clamp truncates fractional dimensions toward zero",
);
assertDeepEqual(
  clampTerminalSize(100, 30),
  { columns: 100, rows: 30 },
  "clamp passes an in-bounds size through unchanged",
);

// Fetch-level coverage: every server-scoped terminal operation must hit the
// canonical Server Route URL with the right method and payload, and stamp the
// serverRoute onto returned session state so panes stay collision-safe.
type RecordedFetch = { url: string; method: string; body: unknown };
let recordedFetch: RecordedFetch | null = null;
function installFetchMock(responseBody: unknown, status = 200) {
  recordedFetch = null;
  (globalThis as { fetch: typeof fetch }).fetch = (async (
    input: RequestInfo | URL,
    init?: RequestInit,
  ) => {
    recordedFetch = {
      url: String(input),
      method: init?.method ?? "GET",
      body:
        typeof init?.body === "string" ? JSON.parse(init.body) : (init?.body ?? null),
    };
    return {
      ok: status >= 200 && status < 300,
      status,
      json: async () => responseBody,
    } as Response;
  }) as typeof fetch;
}
function recorded(): RecordedFetch {
  if (!recordedFetch) throw new Error("no fetch recorded");
  return recordedFetch;
}

const remote = "server-remote-1";
const remoteSession: TerminalSessionView = {
  terminalId: "term-remote",
  workRootId: "root-remote",
  title: "Remote",
  status: "running",
  columns: 80,
  rows: 24,
  createdAtMs: 1,
  cwdHint: null,
};

await (async () => {
  installFetchMock(remoteSession);
  const created = await createTerminal(
    "root-remote",
    { title: "Remote", cwdHint: "sub" },
    remote,
  );
  assertEqual(
    recorded().url,
    "/api/dashboard/servers/server-remote-1/work-roots/root-remote/terminals",
    "createTerminal posts to the server-scoped url",
  );
  assertEqual(recorded().method, "POST", "createTerminal uses POST");
  assertDeepEqual(
    recorded().body,
    { columns: 80, rows: 24, title: "Remote", cwdHint: "sub" },
    "createTerminal forwards the size/title/cwd payload",
  );
  assertEqual(
    created.serverRoute,
    remote,
    "createTerminal stamps the serverRoute onto the returned session",
  );

  installFetchMock([remoteSession]);
  const listed = await listTerminals("root-remote", remote);
  assertEqual(
    recorded().url,
    "/api/dashboard/servers/server-remote-1/work-roots/root-remote/terminals",
    "listTerminals gets the server-scoped url",
  );
  assertEqual(recorded().method, "GET", "listTerminals uses GET");
  assertEqual(
    listed[0]?.serverRoute,
    remote,
    "listTerminals stamps the serverRoute onto each session",
  );

  installFetchMock({
    terminalId: "term-remote",
    status: "running",
    nextSequence: 3,
    chunks: [],
  });
  await fetchTerminalOutput("term-remote", 7, remote);
  assertEqual(
    recorded().url,
    "/api/dashboard/servers/server-remote-1/terminals/term-remote/output?after=7",
    "fetchTerminalOutput gets the server-scoped url with cursor",
  );
  assertEqual(recorded().method, "GET", "fetchTerminalOutput uses GET");

  installFetchMock(null, 204);
  await sendTerminalInput("term-remote", "echo hi", remote);
  assertEqual(
    recorded().url,
    "/api/dashboard/servers/server-remote-1/terminals/term-remote/input",
    "sendTerminalInput posts to the server-scoped url",
  );
  assertEqual(recorded().method, "POST", "sendTerminalInput uses POST");
  assertDeepEqual(
    recorded().body,
    { data: "echo hi" },
    "sendTerminalInput forwards the input data payload",
  );

  installFetchMock(remoteSession);
  await resizeTerminal("term-remote", 100, 40, remote);
  assertEqual(
    recorded().url,
    "/api/dashboard/servers/server-remote-1/terminals/term-remote/resize",
    "resizeTerminal posts to the server-scoped url",
  );
  assertEqual(recorded().method, "POST", "resizeTerminal uses POST");
  assertDeepEqual(
    recorded().body,
    { columns: 100, rows: 40 },
    "resizeTerminal forwards the clamped size payload",
  );

  installFetchMock(null, 204);
  await closeTerminal("term-remote", remote);
  assertEqual(
    recorded().url,
    "/api/dashboard/servers/server-remote-1/terminals/term-remote",
    "closeTerminal deletes the server-scoped url",
  );
  assertEqual(recorded().method, "DELETE", "closeTerminal uses DELETE");
})().catch((error) => {
  console.error(error);
  throw error;
});

// Regression: resizeTerminal must stitch the caller's serverRoute onto the
// returned session when the daemon response omits the serverRoute field, so a
// resize never collapses a linked-terminal pane back onto the local route.
await (async () => {
  const sessionWithoutRoute: TerminalSessionView = {
    terminalId: "term-resize",
    workRootId: "root-resize",
    title: "Resize",
    status: "running",
    columns: 80,
    rows: 24,
    createdAtMs: 1,
    cwdHint: null,
  };

  installFetchMock(sessionWithoutRoute);
  const resizedRemote = await resizeTerminal("term-resize", 100, 40, "wsl-daemon");
  assertEqual(
    resizedRemote.serverRoute,
    "wsl-daemon",
    "resizeTerminal stitches the caller's serverRoute when the response omits it",
  );

  installFetchMock(sessionWithoutRoute);
  const resizedLocal = await resizeTerminal("term-resize", 100, 40, undefined);
  assertEqual(
    resizedLocal.serverRoute,
    LOCAL_DASHBOARD_SERVER_ROUTE,
    "resizeTerminal falls back to the local route when no route is provided",
  );
})().catch((error) => {
  console.error(error);
  throw error;
});
