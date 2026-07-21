// Phase-1 route/model test for
// `260711-feat-ws-dashboard-agent-activity-chat-ui` — asserts:
//   1. Tab creation always produces an empty pane (`session: null`), never
//      blocking on a harness/session picker.
//   2. The resume-popup history list stub renders entries scoped to the
//      requesting `workRootId`.
//   3. A tile click actually invokes the stub `activity.session.create`/
//      `start` call path with the expected request shape, and the resulting
//      pane reflects the synthetic session it returns.
//   4. (Phase 3 review-fix-cycle addition) `appendUserTranscriptBlock` -
//      the pure base send-input primitive - mints a unique, non-colliding
//      cursor, returns a new session without mutating the input, and
//      appends a correctly-shaped `role: "user"`/`renderKind: "markdown"`
//      block carrying the exact sent text.

import {
  agentChatPaneId,
  agentChatPaneLogicalKey,
  appendUserTranscriptBlock,
  attachAgentChatSession,
  createEmptyAgentChatPane,
  markAgentChatPaneError,
  markAgentChatPaneStarting,
  reconcileOptimisticUserCursors,
  removeAgentChatPane,
  removeAgentChatPanesForWorkRoot,
  type AgentChatPaneState,
} from "./agentChatSessions.js";
import type { AgentChatSessionView } from "./agentChatSessions.js";
import type { TranscriptBlock } from "./workRootActivity.js";
import {
  stubActivityHistoryList,
  stubResumeAgentChatSession,
  stubStartNewAgentChatSession,
} from "./activitySessionStub.js";
import { LOCAL_DASHBOARD_SERVER_ROUTE } from "./resourceModel.js";

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

// --- 1b. Stub create/start rejection transitions the pane to error state ---
// Mirrors App.tsx's `.catch` handlers on the stub
// `activity.session.create`/`start` call (~line 4929 and ~4964): on
// rejection the pane must stop being "starting" and must carry a non-empty
// error message, without fabricating a session.

const erroredPane = markAgentChatPaneError(
  startingPane,
  "agent chat session failed to start",
);
assertEqual(
  erroredPane.starting,
  false,
  "a stub create/start rejection clears the in-flight starting flag",
);
assertEqual(
  erroredPane.error,
  "agent chat session failed to start",
  "a stub create/start rejection records the rejection's error message on the pane",
);
assertEqual(
  erroredPane.session,
  null,
  "a stub create/start rejection does not fabricate a session",
);

// A subsequent retry that starts again must clear any stale error.
const retryingPane = markAgentChatPaneStarting(erroredPane);
assertEqual(
  retryingPane.error,
  null,
  "retrying after a failure clears the previous error before the next stub call resolves",
);
assertEqual(
  retryingPane.starting,
  true,
  "retrying after a failure re-enters the in-flight starting state",
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

// --- serverRoute threading (Phase 4, 260711) --------------------------------
// Mirrors the precedent pattern at `workRootActivity.test.ts#L1365-L1422`:
// (a) omitting `serverRoute` falls back to `LOCAL_DASHBOARD_SERVER_ROUTE`
// consistently, and (b) distinct server routes produce distinct identity
// keys for the same workRootId/tabId.

assertEqual(
  agentChatPaneLogicalKey("root-shared", "tab-x") ===
    agentChatPaneLogicalKey("root-shared", "tab-x", LOCAL_DASHBOARD_SERVER_ROUTE),
  true,
  "omitting serverRoute in agentChatPaneLogicalKey falls back to LOCAL_DASHBOARD_SERVER_ROUTE consistently",
);
assertEqual(
  agentChatPaneId("tab-x") === agentChatPaneId("tab-x", LOCAL_DASHBOARD_SERVER_ROUTE),
  true,
  "omitting serverRoute in agentChatPaneId falls back to LOCAL_DASHBOARD_SERVER_ROUTE consistently",
);
assertEqual(
  createEmptyAgentChatPane("root-shared").serverRoute,
  LOCAL_DASHBOARD_SERVER_ROUTE,
  "omitting serverRoute in createEmptyAgentChatPane falls back to LOCAL_DASHBOARD_SERVER_ROUTE consistently",
);

const distinctServerRoutes = ["server-remote-1", "server-remote-2", LOCAL_DASHBOARD_SERVER_ROUTE];
const distinctLogicalKeys = new Set(
  distinctServerRoutes.map((serverRoute) =>
    agentChatPaneLogicalKey("root-shared", "tab-shared", serverRoute),
  ),
);
assertEqual(
  distinctLogicalKeys.size,
  distinctServerRoutes.length,
  "agentChatPaneLogicalKey produces a distinct key per distinct serverRoute for the same workRootId/tabId",
);
const distinctPaneIds = new Set(
  distinctServerRoutes.map((serverRoute) => agentChatPaneId("tab-shared", serverRoute)),
);
assertEqual(
  distinctPaneIds.size,
  distinctServerRoutes.length,
  "agentChatPaneId produces a distinct id per distinct serverRoute for the same tabId",
);

// removeAgentChatPanesForWorkRoot must scope by (workRootId, serverRoute)
// together — two panes sharing a workRootId but on different serverRoutes
// must not cross-remove each other.
const localPaneForShared = createEmptyAgentChatPane("root-cross-server");
const remotePaneForShared = createEmptyAgentChatPane(
  "root-cross-server",
  "server-remote-1",
);
const crossServerPanes: Record<string, AgentChatPaneState> = {
  [localPaneForShared.logicalKey]: localPaneForShared,
  [remotePaneForShared.logicalKey]: remotePaneForShared,
};
const afterLocalRootClose = removeAgentChatPanesForWorkRoot(
  crossServerPanes,
  "root-cross-server",
  LOCAL_DASHBOARD_SERVER_ROUTE,
);
assert(
  !(localPaneForShared.logicalKey in afterLocalRootClose),
  "closing a work root on the local serverRoute drops the matching local pane",
);
assert(
  remotePaneForShared.logicalKey in afterLocalRootClose,
  "closing a work root on the local serverRoute leaves the same workRootId's remote-serverRoute pane untouched",
);

// --- appendUserTranscriptBlock (Phase 3 base send-input primitive) --------
// Review-fix-cycle addition (260711 Phase 3, cycle 1): this pure function
// previously had zero direct unit coverage - only indirect e2e coverage via
// the "renders a real bubble" browser assertions. Exercises cursor
// uniqueness, non-mutation of the input session, and block-shape
// correctness directly against `codexSession` (the same stub session
// fixture already built above).

const blockCountBeforeAppend = codexSession.transcript.blocks.length;
const afterFirstAppend = appendUserTranscriptBlock(
  codexSession,
  "hello from a real user send",
);
assertEqual(
  codexSession.transcript.blocks.length,
  blockCountBeforeAppend,
  "appendUserTranscriptBlock does not mutate the input session's block array",
);
assertEqual(
  afterFirstAppend.transcript.blocks.length,
  blockCountBeforeAppend + 1,
  "appendUserTranscriptBlock returns a new session with exactly one additional block",
);
assert(
  afterFirstAppend !== codexSession,
  "appendUserTranscriptBlock returns a distinct session object rather than the input reference",
);
assertEqual(
  afterFirstAppend.activityId,
  codexSession.activityId,
  "appendUserTranscriptBlock preserves the session's other fields (activityId) unchanged",
);

const firstAppendedBlock =
  afterFirstAppend.transcript.blocks[afterFirstAppend.transcript.blocks.length - 1]!;
assertEqual(
  firstAppendedBlock.role,
  "user",
  "the appended block is tagged with the user role",
);
assertEqual(
  firstAppendedBlock.renderKind,
  "markdown",
  "the appended block renders as markdown, mirroring stubTranscriptBlock's shape",
);
assertEqual(
  firstAppendedBlock.text,
  "hello from a real user send",
  "the appended block carries the exact sent text",
);
assert(
  firstAppendedBlock.cursor.length > 0,
  "the appended block carries a non-empty cursor",
);
assert(
  !codexSession.transcript.blocks.some(
    (block) => block.cursor === firstAppendedBlock.cursor,
  ),
  "the appended block's cursor does not collide with any pre-existing block's cursor",
);

// A second, independent call (mirroring two separate sends, e.g. a real
// send followed by a later FIFO-dequeued send) mints a distinct cursor -
// this is the exact uniqueness property the mid-turn queue's dequeue path
// depends on to avoid overwriting/colliding streaming-overlay entries.
const afterSecondAppend = appendUserTranscriptBlock(
  codexSession,
  "a second, independent send",
);
const secondAppendedBlock =
  afterSecondAppend.transcript.blocks[afterSecondAppend.transcript.blocks.length - 1]!;
assert(
  secondAppendedBlock.cursor !== firstAppendedBlock.cursor,
  "two separate calls to appendUserTranscriptBlock mint distinct cursors, never colliding with each other",
);
assertEqual(
  secondAppendedBlock.text,
  "a second, independent send",
  "the second call's appended block carries its own text, independent of the first call",
);

// --- reconcileOptimisticUserCursors (260720 Phase 1) ------------------------
// Reproduces the ticket's confirmed bug mechanism directly: a canonical
// `session.transcript.blocks` entry created by `appendUserTranscriptBlock`
// keeps its client-only `user-sent-...` cursor forever unless something
// rewrites it once the daemon confirms the same logical message with a real
// sequential cursor ("0", "1", "2", ...). `forkAgentChatFromBubble`
// (`App.tsx`) reads a bubble's *last* block's cursor straight off this
// canonical array, so this is the exact site "fork from here" depends on.

function makeBlock(overrides: Partial<TranscriptBlock>): TranscriptBlock {
  return {
    cursor: "0",
    timestamp: new Date().toISOString(),
    renderKind: "markdown",
    title: null,
    text: null,
    data: null,
    degraded: false,
    role: "agent",
    turnId: undefined,
    ...overrides,
  };
}

function sessionWithBlocks(blocks: TranscriptBlock[]): AgentChatSessionView {
  return {
    ...codexSession,
    transcript: { ...codexSession.transcript, blocks },
  };
}

// 1. A single live-sent optimistic user block gets replaced wholesale by the
//    daemon-confirmed real-cursor block once the poll observes it.
const optimisticOnly = sessionWithBlocks([
  makeBlock({ cursor: "user-sent-abc-1", role: "user", text: "hi" }),
]);
const confirmedDelta = [
  makeBlock({ cursor: "0", role: "user", text: "hi" }),
  makeBlock({ cursor: "1", role: "agent", text: "hello back" }),
];
const reconciledOnce = reconcileOptimisticUserCursors(optimisticOnly, confirmedDelta);
assertEqual(
  reconciledOnce.transcript.blocks.length,
  1,
  "reconciling a single optimistic user block does not change the canonical block count",
);
assertEqual(
  reconciledOnce.transcript.blocks[0]!.cursor,
  "0",
  "a live user-sent block's optimistic cursor is replaced by the daemon-confirmed sequential cursor once the poll observes it",
);
assert(
  !reconciledOnce.transcript.blocks[0]!.cursor.startsWith("user-sent-"),
  "the reconciled block no longer carries a client-only optimistic cursor - forkAgentChatFromBubble can now send a daemon-resolvable cutCursor",
);

// 2. No matching confirmed user block in the delta (e.g. only agent blocks
//    polled so far) leaves the optimistic block, and the session reference,
//    untouched (pure no-op, not a wasted re-render).
const onlyAgentDelta = [makeBlock({ cursor: "5", role: "agent", text: "..." })];
const untouched = reconcileOptimisticUserCursors(optimisticOnly, onlyAgentDelta);
assert(
  untouched === optimisticOnly,
  "a poll delta with no confirmed user blocks is a pure no-op, returning the same session reference",
);

// 3. A resume/hydrated bubble's already-real cursor is never touched, even
//    when the delta also carries confirmed user blocks - this is the "should
//    already work" bubble class the ticket calls out as unaffected.
const alreadyResolved = sessionWithBlocks([
  makeBlock({ cursor: "0", role: "user", text: "already real" }),
]);
const noOpOnResolved = reconcileOptimisticUserCursors(alreadyResolved, confirmedDelta);
assert(
  noOpOnResolved === alreadyResolved,
  "a block that already carries a daemon-confirmed (non-optimistic) cursor is left alone by reconciliation",
);

// 4. Multiple pending live sends (fast typing/queued dequeue) reconcile in
//    position/turn order against the poll's confirmed user blocks in the same
//    order - the ticket's suggested "match by position, not identity or
//    content" direction, so this deliberately uses non-matching text to prove
//    the match is positional.
const twoOptimistic = sessionWithBlocks([
  makeBlock({ cursor: "user-sent-a-1", role: "user", text: "first send" }),
  makeBlock({ cursor: "1", role: "agent", text: "reply to first" }),
  makeBlock({ cursor: "user-sent-b-2", role: "user", text: "second send" }),
]);
const twoConfirmedDelta = [
  makeBlock({ cursor: "2", role: "user", text: "first send (normalized)" }),
  makeBlock({ cursor: "3", role: "agent", text: "reply to first" }),
  makeBlock({ cursor: "4", role: "user", text: "second send (normalized)" }),
];
const reconciledTwo = reconcileOptimisticUserCursors(twoOptimistic, twoConfirmedDelta);
assertEqual(
  reconciledTwo.transcript.blocks[0]!.cursor,
  "2",
  "the first pending optimistic user block reconciles to the first confirmed user block in the poll delta, in order",
);
assertEqual(
  reconciledTwo.transcript.blocks[2]!.cursor,
  "4",
  "the second pending optimistic user block reconciles to the second confirmed user block in the poll delta, in order",
);
assertEqual(
  reconciledTwo.transcript.blocks[1]!.cursor,
  "1",
  "a non-optimistic (already-real) block between two pending sends is left untouched by reconciliation",
);

// 5. Reconciliation never mutates the input session/block array.
assertEqual(
  optimisticOnly.transcript.blocks[0]!.cursor,
  "user-sent-abc-1",
  "reconcileOptimisticUserCursors does not mutate the input session's blocks in place",
);

// 6. Reviewer-confirmed regression: `beginRealStreamingTurn` resets its
//    `lastSeenLength` cursor to 0 at the start of EVERY new turn, so the
//    first poll of the 2nd+ user message in a session redelivers the ENTIRE
//    transcript (index 0 onward) as the "delta" - not just the newly-sent
//    block. A session with prior resolved blocks ["0" user "first", "1"
//    agent "reply1"] plus one new optimistic block ("user-sent-x-2" ->
//    "second") must reconcile the optimistic block to the NEWEST confirmed
//    cursor ("2"), never to the oldest ("0") that positional matching alone
//    would wrongly pick when the delta is a full-transcript redelivery.
const priorHistoryPlusOnePending = sessionWithBlocks([
  makeBlock({ cursor: "0", role: "user", text: "first" }),
  makeBlock({ cursor: "1", role: "agent", text: "reply1" }),
  makeBlock({ cursor: "user-sent-x-2", role: "user", text: "second" }),
]);
const fullTranscriptRedeliveryDelta = [
  makeBlock({ cursor: "0", role: "user", text: "first" }),
  makeBlock({ cursor: "1", role: "agent", text: "reply1" }),
  makeBlock({ cursor: "2", role: "user", text: "second" }),
];
const reconciledAfterRedelivery = reconcileOptimisticUserCursors(
  priorHistoryPlusOnePending,
  fullTranscriptRedeliveryDelta,
);
assertEqual(
  reconciledAfterRedelivery.transcript.blocks[2]!.cursor,
  "2",
  "on a full-transcript-redelivery poll delta, the newest optimistic block reconciles to the NEWEST confirmed cursor, not the oldest already-resolved one",
);
assertEqual(
  reconciledAfterRedelivery.transcript.blocks[2]!.text,
  "second",
  "the reconciled block's text is not corrupted by an unrelated earlier message",
);
assertEqual(
  reconciledAfterRedelivery.transcript.blocks[0]!.cursor,
  "0",
  "the pre-existing resolved block at cursor 0 is left untouched by the redelivery",
);
assert(
  reconciledAfterRedelivery.transcript.blocks[0]!.text === "first",
  "cursor 0's text is not overwritten as a side effect of reconciling the newer optimistic block",
);
const cursorsAfterRedelivery = reconciledAfterRedelivery.transcript.blocks.map(
  (block) => block.cursor,
);
assertEqual(
  new Set(cursorsAfterRedelivery).size,
  cursorsAfterRedelivery.length,
  "no cursor is duplicated across the reconciled transcript after a full-transcript-redelivery poll",
);
