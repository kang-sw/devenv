import {
  appendTerminalOutput,
  appendTerminalWebSocketMessage,
  canApplyTerminalOutputPoll,
  clampTerminalSize,
  markTerminalPaneCloseError,
  markTerminalSocketStatus,
  mergeListedTerminalSessions,
  loadTerminalRestoreIntents,
  reconcileListedTerminalSessions,
  removeClosedTerminalPane,
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
  type TerminalSessionView,
} from "./terminals.js";

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
  workRootTerminalsEndpoint("root/local abc", "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/work-roots/root%2Flocal%20abc/terminals",
  "server-scoped terminal create/list endpoint encodes server id",
);
assertEqual(
  terminalOutputEndpoint("term/abc", 12, "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/terminals/term%2Fabc/output?after=12",
  "server-scoped terminal output endpoint encodes server id",
);
assertEqual(
  terminalInputEndpoint("term/abc", "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/terminals/term%2Fabc/input",
  "server-scoped terminal input endpoint encodes server id",
);
assertEqual(
  terminalResizeEndpoint("term/abc", "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/terminals/term%2Fabc/resize",
  "server-scoped terminal resize endpoint encodes server id",
);
assertEqual(
  terminalCloseEndpoint("term/abc", "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/terminals/term%2Fabc",
  "server-scoped terminal close endpoint encodes server id",
);
assertEqual(
  terminalWebSocketEndpoint("term/abc", 12, "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/terminals/term%2Fabc/socket?after=12",
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
