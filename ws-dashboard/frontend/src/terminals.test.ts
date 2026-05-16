import {
  appendTerminalOutput,
  mergeListedTerminalSessions,
  removeClosedTerminalPane,
  terminalCloseEndpoint,
  terminalInputEndpoint,
  terminalOutputEndpoint,
  terminalPaneFromSession,
  terminalPaneId,
  terminalPaneLogicalKey,
  terminalResizeEndpoint,
  validateTerminalSize,
  workRootTerminalsEndpoint,
  type TerminalSessionView,
} from "./terminals.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) throw new Error(`${label}: expected ${expected}, got ${actual}`);
}
function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const a = JSON.stringify(actual), e = JSON.stringify(expected);
  if (a !== e) throw new Error(`${label}: expected ${e}, got ${a}`);
}
function assertThrows(action: () => unknown, pattern: RegExp, label: string) {
  try { action(); } catch (error) { const message = error instanceof Error ? error.message : String(error); if (pattern.test(message)) return; throw new Error(`${label}: ${message}`); }
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
};

assertEqual(workRootTerminalsEndpoint("root/local abc"), "/api/dashboard/work-roots/root%2Flocal%20abc/terminals", "workRoot terminal endpoint encodes id");
assertEqual(terminalOutputEndpoint("term/abc", 12), "/api/dashboard/terminals/term%2Fabc/output?after=12", "output endpoint encodes cursor");
assertEqual(terminalInputEndpoint("term/abc"), "/api/dashboard/terminals/term%2Fabc/input", "input endpoint encodes id");
assertEqual(terminalResizeEndpoint("term/abc"), "/api/dashboard/terminals/term%2Fabc/resize", "resize endpoint encodes id");
assertEqual(terminalCloseEndpoint("term/abc"), "/api/dashboard/terminals/term%2Fabc", "close endpoint encodes id");
assertEqual(terminalPaneLogicalKey("root-local-abc", "term_abc"), "persistentTerminal/root-local-abc/term_abc", "logical key uses workRoot and terminal id");
assertEqual(terminalPaneId("term/abc"), "terminal:term%2Fabc", "pane id encodes terminal id");
assertEqual(String(terminalPaneLogicalKey("root-local-abc", "term_abc")).includes("/Users/"), false, "logical key omits host paths");

const pane = terminalPaneFromSession(session);
assertEqual(pane.nextSequence, 0, "new pane starts at cursor zero");
const merged = mergeListedTerminalSessions({}, [session]);
assertEqual(Boolean(merged[pane.logicalKey]), true, "listed live session reconstructs pane state");
const withOutput = appendTerminalOutput(pane, { terminalId: "term_abc", status: "running", nextSequence: 3, chunks: [{ sequence: 1, data: "hi", stream: "pty" }] });
assertEqual(withOutput.output, "hi", "output appends chunk data");
assertEqual(withOutput.nextSequence, 3, "output advances cursor");
assertDeepEqual(removeClosedTerminalPane(merged, pane.logicalKey), {}, "close success removes pane state");
assertDeepEqual(validateTerminalSize(100, 30), { columns: 100, rows: 30 }, "valid resize accepted");
assertThrows(() => validateTerminalSize(0, 30), /invalid terminal size/, "non-positive columns rejected");
assertThrows(() => validateTerminalSize(1000, 30), /invalid terminal size/, "oversized columns rejected");
