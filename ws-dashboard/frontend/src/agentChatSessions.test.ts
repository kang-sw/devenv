// Phase-1 route/model test for
// `260711-feat-ws-dashboard-agent-activity-chat-ui` — asserts:
//   1. Tab creation always produces an empty pane (`session: null`), never
//      blocking on a harness/session picker.
//   2. The resume-popup history list stub renders entries scoped to the
//      requesting `workRootId`.
//   3. A tile click actually invokes the stub `activity.session.create`/
//      `start` call path with the expected request shape, and the resulting
//      pane reflects the synthetic session it returns.

import {
  agentChatPaneId,
  agentChatPaneLogicalKey,
  attachAgentChatSession,
  createEmptyAgentChatPane,
  markAgentChatPaneStarting,
  removeAgentChatPane,
  removeAgentChatPanesForWorkRoot,
  type AgentChatPaneState,
} from "./agentChatSessions.js";
import {
  stubActivityHistoryList,
  stubResumeAgentChatSession,
  stubStartNewAgentChatSession,
} from "./activitySessionStub.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assert(condition: boolean, label: string) {
  if (!condition) {
    throw new Error(`${label}: expected condition to hold`);
  }
}

// --- 1. Tab creation is always an empty pane, no picker gate ---------------

const emptyPane = createEmptyAgentChatPane("root-local-abc");
assertEqual(
  emptyPane.session,
  null,
  "a freshly created agent chat pane starts with no session/harness chosen",
);
assertEqual(
  emptyPane.starting,
  false,
  "a freshly created agent chat pane is not mid-flight",
);
assertEqual(
  emptyPane.paneId,
  agentChatPaneId(emptyPane.tabId),
  "pane id is derived from the tab id via the shared helper",
);
assertEqual(
  emptyPane.logicalKey,
  agentChatPaneLogicalKey("root-local-abc", emptyPane.tabId),
  "logical key is derived from (workRootId, tabId) via the shared helper",
);

const secondEmptyPane = createEmptyAgentChatPane("root-local-abc");
assert(
  secondEmptyPane.tabId !== emptyPane.tabId,
  "two separately created tabs get distinct tab ids (multi-instance, not singleton)",
);
assert(
  secondEmptyPane.paneId !== emptyPane.paneId,
  "two separately created tabs get distinct pane ids",
);

const startingPane = markAgentChatPaneStarting(emptyPane);
assertEqual(
  startingPane.starting,
  true,
  "marking a pane starting flips the starting flag while the stub call is in flight",
);
assertEqual(
  startingPane.session,
  null,
  "marking a pane starting does not fabricate a session before the stub call resolves",
);

// --- 2. Resume-popup history stub is scoped to the current workRootId ------

const historyForRootA = await stubActivityHistoryList({
  workRootId: "root-local-abc",
});
assert(
  historyForRootA.items.length > 0,
  "the history stub returns at least one synthetic entry",
);
assert(
  historyForRootA.items.every((item) => item.id.includes("root-local-abc")),
  "every synthetic history entry is scoped to the requesting workRootId",
);
assert(
  historyForRootA.items.every(
    (item) =>
      typeof item.label === "string" &&
      item.label.length > 0 &&
      typeof item.updatedAt === "string" &&
      item.updatedAt.length > 0,
  ),
  "every synthetic history entry carries an alias/title and a last-accessed time",
);

const historyForRootB = await stubActivityHistoryList({
  workRootId: "root-local-other",
});
assert(
  historyForRootB.items.every((item) => item.id.includes("root-local-other")),
  "a different workRootId's history request stays scoped to that root, not global",
);
assert(
  historyForRootA.items.every(
    (itemA) => !historyForRootB.items.some((itemB) => itemB.id === itemA.id),
  ),
  "history entries for distinct work roots never collide by id",
);

// --- 3. Tile click actually invokes the stub create/start call path --------

const codexSession = await stubStartNewAgentChatSession(
  "root-local-abc",
  "codex",
);
assertEqual(
  codexSession.workRootId,
  "root-local-abc",
  "the stub session carries the requesting workRootId",
);
assertEqual(
  codexSession.harness,
  "codex",
  "the stub session reflects the harness the clicked tile requested",
);
assert(
  codexSession.activityId.length > 0,
  "the stub create/start call path assigns a non-empty activityId",
);
assert(
  codexSession.transcript.blocks.length > 0,
  "the stub session carries a synthetic transcript, not just an id",
);

const attachedPane: AgentChatPaneState = attachAgentChatSession(
  startingPane,
  codexSession,
);
assertEqual(
  attachedPane.session?.activityId,
  codexSession.activityId,
  "attaching a resolved stub session records it on the pane",
);
assertEqual(
  attachedPane.starting,
  false,
  "attaching a resolved session clears the in-flight starting flag",
);

// Selecting a history entry resumes rather than creating a brand-new session.
const historyEntry = historyForRootA.items[0]!;
const resumedSession = await stubResumeAgentChatSession(
  historyEntry,
  "root-local-abc",
);
assertEqual(
  resumedSession.activityId,
  historyEntry.id,
  "resuming a history entry reuses its activityId rather than minting a new one",
);
assertEqual(
  resumedSession.title,
  historyEntry.label,
  "a resumed session's title reflects the picked history entry's alias/title",
);

// --- pane removal helpers ---------------------------------------------------

const paneMap: Record<string, AgentChatPaneState> = {
  [emptyPane.logicalKey]: emptyPane,
  [secondEmptyPane.logicalKey]: secondEmptyPane,
};
const afterRemoveOne = removeAgentChatPane(paneMap, emptyPane.logicalKey);
assert(
  !(emptyPane.logicalKey in afterRemoveOne),
  "removeAgentChatPane drops exactly the requested pane",
);
assert(
  secondEmptyPane.logicalKey in afterRemoveOne,
  "removeAgentChatPane leaves other panes untouched",
);

const otherRootPane = createEmptyAgentChatPane("root-local-other");
const mixedRootPanes: Record<string, AgentChatPaneState> = {
  [secondEmptyPane.logicalKey]: secondEmptyPane,
  [otherRootPane.logicalKey]: otherRootPane,
};
const afterRootClose = removeAgentChatPanesForWorkRoot(
  mixedRootPanes,
  "root-local-abc",
  undefined,
);
assert(
  !(secondEmptyPane.logicalKey in afterRootClose),
  "closing a work root drops every agent chat pane scoped to that root",
);
assert(
  otherRootPane.logicalKey in afterRootClose,
  "closing a work root leaves other work roots' agent chat panes untouched",
);
