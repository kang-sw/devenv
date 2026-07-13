// Phase-1 route/request-shape test for
// `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` — asserts each
// `activitySessionClient.ts` function issues the correct REST-nested
// method/URL/body for a Codex and a Claude session, in both the
// server-local and `/servers/{serverRoute}/...` forms (mirroring
// `workRootActivity.test.ts`'s existing `serverRoute`-threading pattern).
// This is a request-shape test only — Phase 1's verification bar does not
// require a live daemon; see the plan's Verification Plan.

import {
  activityHistoryList,
  beginRealStreamingTurn,
  forkActivitySession,
  resumeAgentChatSession,
  sendAgentChatPrompt,
  startNewAgentChatSession,
  steerActivitySession,
} from "./activitySessionClient.js";
import type { ActivityItem, TranscriptBlock } from "./workRootActivity.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assert(condition: unknown, label: string) {
  if (!condition) {
    throw new Error(label);
  }
}

// `body` is parsed JSON (a fresh object per call), so a `===` comparison
// would never match an equivalent object literal — compare serialized form.
function assertBodyEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

type RecordedCall = { url: string; method: string; body: unknown };

const calls: RecordedCall[] = [];
let nextResponses: unknown[] = [];

// A queued response of this shape makes the mock return a non-ok HTTP
// response instead of a 200, so error-path assertions (e.g.
// `beginRealStreamingTurn`'s `onError` branch) can be exercised without a
// live daemon.
type MockErrorResponse = { readonly __httpError: number; readonly error: string };

function isMockErrorResponse(value: unknown): value is MockErrorResponse {
  return typeof value === "object" && value !== null && "__httpError" in value;
}

globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  const method = init?.method ?? "GET";
  const body = init?.body ? JSON.parse(String(init.body)) : undefined;
  calls.push({ url: String(url), method, body });
  const payload = nextResponses.shift();
  if (isMockErrorResponse(payload)) {
    return new Response(JSON.stringify({ error: payload.error }), {
      status: payload.__httpError,
      headers: { "Content-Type": "application/json" },
    });
  }
  return new Response(JSON.stringify(payload ?? {}), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}) as typeof fetch;

const transcriptFixture = {
  workRootId: "root-a",
  activityId: "agent.codex:1",
  status: "available",
  sourceStatus: "ok",
  live: true,
  source: { kind: "agent.codex", label: "Codex", backend: "codex", harness: "codex", tier: "core", model: null },
  blocks: [
    {
      cursor: "0",
      timestamp: null,
      renderKind: "markdown",
      title: null,
      text: "hello",
      data: null,
      degraded: false,
    },
  ],
  nextCursor: null,
  hasMore: false,
  diagnostics: [],
};

// --- startNewAgentChatSession: create (POST) then transcript (GET) ---------

calls.length = 0;
nextResponses = [{ activityId: "agent.codex:1" }, transcriptFixture];
const codexSession = await startNewAgentChatSession("root-a", "codex", "server-local");
assertEqual(calls.length, 2, "startNewAgentChatSession issues exactly a create + transcript call");
assertEqual(calls[0]!.method, "POST", "create call is a POST");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/work-roots/root-a/activity/codex-sessions",
  "codex create hits the local codex-sessions collection route",
);
assertEqual(calls[1]!.method, "GET", "transcript call is a GET");
assertEqual(
  calls[1]!.url,
  "/api/dashboard/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/transcript",
  "codex transcript hits the local per-activity transcript route",
);
assertEqual(codexSession.harness, "codex", "returned session carries the codex harness");
assertEqual(codexSession.activityId, "agent.codex:1", "returned session carries the created activityId");
assertEqual(
  codexSession.transcript.blocks.length,
  1,
  "returned session's transcript is hydrated from the transcript fetch",
);
assertEqual(codexSession.capabilities.steer, true, "codex session capabilities enable steer");

calls.length = 0;
nextResponses = [{ activityId: "agent.claude:1" }, { ...transcriptFixture, activityId: "agent.claude:1" }];
const claudeSession = await startNewAgentChatSession("root-a", "claude", "server-remote-1");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/claude-sessions",
  "claude create hits the server-scoped claude-sessions collection route for a non-local serverRoute",
);
assertEqual(
  calls[1]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/claude-sessions/agent.claude%3A1/transcript",
  "claude transcript hits the server-scoped per-activity transcript route",
);
assertEqual(claudeSession.harness, "claude", "returned session carries the claude harness");
assertEqual(claudeSession.capabilities.steer, false, "claude session capabilities disable steer");

// --- resumeAgentChatSession: transcript fetch only (no create) -------------

const historyItem: ActivityItem = {
  id: "agent.codex:history-1",
  kind: "agent.codex",
  label: "refactor auth module",
  status: "idle",
  live: false,
  attention: false,
  startedAt: null,
  updatedAt: null,
  finishedAt: null,
  source: { kind: "agent.codex", label: "Codex", backend: "codex", harness: "codex", tier: "core", model: null },
  transcript: { status: "available", available: true, cursor: null },
  diagnostics: [],
  metadata: {},
};

calls.length = 0;
nextResponses = [{ ...transcriptFixture, activityId: "agent.codex:history-1" }];
const resumedSession = await resumeAgentChatSession(historyItem, "codex", "root-a");
assertEqual(calls.length, 1, "resumeAgentChatSession issues exactly one transcript fetch, no create call");
assertEqual(calls[0]!.method, "GET", "resume issues a GET");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/work-roots/root-a/activity/codex-sessions/agent.codex%3Ahistory-1/transcript",
  "resume hits the local per-activity transcript route for the existing activityId",
);
assertEqual(resumedSession.title, historyItem.label, "resumed session title carries the history item's label");

calls.length = 0;
nextResponses = [{ ...transcriptFixture, activityId: "agent.codex:history-1" }];
await resumeAgentChatSession(historyItem, "codex", "root-a", "server-remote-1");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/codex-sessions/agent.codex%3Ahistory-1/transcript",
  "resume hits the server-scoped per-activity transcript route for a non-local serverRoute",
);

// --- activityHistoryList: GET both codex-sessions and claude-sessions ------

calls.length = 0;
nextResponses = [
  { sessions: [{ activityId: "agent.codex:1", label: "Codex session", status: "running", updatedAt: null }] },
  { sessions: [{ activityId: "agent.claude:1", label: "Claude session", status: "idle", updatedAt: "2026-07-13T00:00:00Z" }] },
];
const history = await activityHistoryList({ workRootId: "root-a" });
assertEqual(calls.length, 2, "activityHistoryList issues one list call per real harness");
assert(
  calls.some((call) => call.url === "/api/dashboard/work-roots/root-a/activity/codex-sessions" && call.method === "GET"),
  "activityHistoryList GETs the codex-sessions collection",
);
assert(
  calls.some((call) => call.url === "/api/dashboard/work-roots/root-a/activity/claude-sessions" && call.method === "GET"),
  "activityHistoryList GETs the claude-sessions collection",
);
assertEqual(history.items.length, 2, "activityHistoryList merges both harnesses' session lists");
assert(
  history.items.some((item) => item.kind === "agent.codex" && item.live === true),
  "a running codex session summary maps to a live ActivityItem",
);
assert(
  history.items.some((item) => item.kind === "agent.claude" && item.live === false),
  "an idle claude session summary maps to a non-live ActivityItem",
);

calls.length = 0;
nextResponses = [
  { sessions: [{ activityId: "agent.codex:1", label: "Codex session", status: "running", updatedAt: null }] },
  { sessions: [{ activityId: "agent.claude:1", label: "Claude session", status: "idle", updatedAt: "2026-07-13T00:00:00Z" }] },
];
await activityHistoryList({ workRootId: "root-a", serverRoute: "server-remote-1" });
assert(
  calls.some(
    (call) =>
      call.url === "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/codex-sessions" &&
      call.method === "GET",
  ),
  "activityHistoryList GETs the server-scoped codex-sessions collection for a non-local serverRoute",
);
assert(
  calls.some(
    (call) =>
      call.url === "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/claude-sessions" &&
      call.method === "GET",
  ),
  "activityHistoryList GETs the server-scoped claude-sessions collection for a non-local serverRoute",
);

// --- steerActivitySession: POST control with action=steer (Codex-only) ----

calls.length = 0;
nextResponses = [{ applied: true }];
await steerActivitySession("root-a", "agent.codex:1", "focus here", "server-local");
assertEqual(calls.length, 1, "steerActivitySession issues exactly one control call");
assertEqual(calls[0]!.method, "POST", "steer is a POST");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/control",
  "steer hits the local codex control route",
);
assertBodyEqual(
  calls[0]!.body,
  { action: "steer", text: "focus here" },
  "steer control body matches CodexControlRequest::Steer's tagged shape",
);

calls.length = 0;
nextResponses = [{ applied: true }];
await steerActivitySession("root-a", "agent.codex:1", "focus here", "server-remote-1");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/control",
  "steer hits the server-scoped codex control route for a non-local serverRoute",
);

// --- forkActivitySession: POST control with action=fork, cutCursor --------

calls.length = 0;
nextResponses = [{ applied: true }];
const forkResult = await forkActivitySession({
  workRootId: "root-a",
  activityId: "agent.codex:1",
  cutCursor: "3",
  serverRoute: "server-local",
});
assertEqual(calls.length, 1, "forkActivitySession issues exactly one control call");
assertEqual(calls[0]!.method, "POST", "fork is a POST");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/control",
  "fork hits the same local codex control route as steer (no dedicated fork route exists until Phase 3)",
);
assertBodyEqual(
  calls[0]!.body,
  { action: "fork", cutCursor: "3" },
  "fork control body carries the future Fork action tag plus the requested cut-point cursor",
);
assertEqual(forkResult.cutCursor, "3", "forkActivitySession echoes the requested cutCursor back");

calls.length = 0;
nextResponses = [{ applied: true }];
await forkActivitySession({
  workRootId: "root-a",
  activityId: "agent.codex:1",
  serverRoute: "server-remote-1",
});
assertEqual(
  calls[0]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/control",
  "fork hits the server-scoped codex control route for a non-local serverRoute",
);
assertBodyEqual(
  calls[0]!.body,
  { action: "fork", cutCursor: null },
  "an omitted cutCursor defaults to null (fork the entire transcript), not undefined-dropped",
);

// --- sendAgentChatPrompt: POST prompt with { text } ------------------------

calls.length = 0;
nextResponses = [{}];
await sendAgentChatPrompt("root-a", "claude", "agent.claude:1", "continue please", "server-local");
assertEqual(calls.length, 1, "sendAgentChatPrompt issues exactly one prompt call");
assertEqual(calls[0]!.method, "POST", "prompt send is a POST");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/work-roots/root-a/activity/claude-sessions/agent.claude%3A1/prompt",
  "prompt send hits the local claude prompt route",
);
assertBodyEqual(calls[0]!.body, { text: "continue please" }, "prompt send body carries the prompt text");

calls.length = 0;
nextResponses = [{}];
await sendAgentChatPrompt("root-a", "codex", "agent.codex:1", "continue please", "server-remote-1");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/prompt",
  "prompt send hits the server-scoped codex prompt route for a non-local serverRoute",
);

// --- beginRealStreamingTurn: poll loop (Phase 2) ---------------------------
// Rewritten from Phase 1's one-shot fetch to a poll loop driven by an
// injectable timer environment (mirroring `gitToolbar.ts`'s
// `GitRefreshSchedulerEnvironment`), so these tests manually tick the poll
// instead of waiting on real intervals.

function createDeferred<T = void>(): { promise: Promise<T>; resolve: (value: T) => void } {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function makeManualPollEnv() {
  let listener: (() => void) | null = null;
  let nextHandle = 0;
  const clearedHandles: number[] = [];
  return {
    env: {
      setInterval: (fn: () => void) => {
        listener = fn;
        nextHandle += 1;
        return nextHandle;
      },
      clearInterval: (handle: number) => {
        clearedHandles.push(handle);
      },
    },
    tick: () => {
      if (!listener) throw new Error("interval listener was not registered");
      listener();
    },
    clearedHandles,
  };
}

const pollPartialFixture = {
  ...transcriptFixture,
  live: true,
  blocks: [
    { cursor: "0", timestamp: null, renderKind: "markdown", title: null, text: "user prompt", data: null, degraded: false },
    {
      cursor: "1",
      timestamp: null,
      renderKind: "markdown",
      title: null,
      text: "agent reply, still streaming",
      data: null,
      degraded: false,
    },
  ],
};

const pollFinalFixture = {
  ...transcriptFixture,
  live: false,
  blocks: [
    { cursor: "0", timestamp: null, renderKind: "markdown", title: null, text: "user prompt", data: null, degraded: false },
    {
      cursor: "1",
      timestamp: null,
      renderKind: "markdown",
      title: null,
      text: "agent reply, now complete",
      data: null,
      degraded: false,
    },
    { cursor: "2", timestamp: null, renderKind: "markdown", title: null, text: "tool result", data: null, degraded: false },
  ],
};

// Two polls to completion: first poll is live (partial), manual tick drives
// the second poll which observes live:false and stops the loop.

calls.length = 0;
nextResponses = [pollPartialFixture, pollFinalFixture];
const twoPollEnv = makeManualPollEnv();
const onUpdateCalls: TranscriptBlock[][] = [];
const firstUpdate = createDeferred();
const completeAfterTwoPolls = createDeferred();
let updateCount = 0;
beginRealStreamingTurn(
  "root-a",
  "codex",
  "agent.codex:1",
  "server-local",
  (blocks) => {
    updateCount += 1;
    onUpdateCalls.push([...blocks]);
    if (updateCount === 1) firstUpdate.resolve();
  },
  () => completeAfterTwoPolls.resolve(),
  undefined,
  twoPollEnv.env,
);
await firstUpdate.promise;
assertEqual(calls.length, 1, "beginRealStreamingTurn fires one immediate poll at call time, before any interval tick");
assertEqual(calls[0]!.method, "GET", "the immediate poll's call is the transcript GET");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/transcript",
  "beginRealStreamingTurn fetches the local codex transcript route",
);
assertEqual(onUpdateCalls[0]!.length, 2, "the first poll (lastSeenLength 0) hands onUpdate the full initial block array");

twoPollEnv.tick();
await completeAfterTwoPolls.promise;
assertEqual(calls.length, 2, "the manual interval tick issues a second transcript fetch");
assertEqual(onUpdateCalls.length, 2, "the second poll calls onUpdate again");
assertEqual(
  onUpdateCalls[1]!.length,
  2,
  "the second poll (lastSeenLength 2) hands onUpdate only the re-included tail block plus the newly appended block, not the full 3-block array",
);
assertEqual(onUpdateCalls[1]![0]!.cursor, "1", "the re-included tail block is cursor 1 (its text grew since the last poll)");
assertEqual(onUpdateCalls[1]![1]!.cursor, "2", "the newly appended block is cursor 2");
assertEqual(updateCount, 2, "onComplete fires exactly once, after the live:false poll, not before");
assertEqual(twoPollEnv.clearedHandles.length, 1, "the interval is cleared once the poll loop observes live:false");

// A poll whose immediate first fetch already observes live:false stops
// after just that one poll, no interval tick needed — also covers the
// server-scoped URL shape for a non-local serverRoute.

calls.length = 0;
nextResponses = [pollFinalFixture];
const immediateCompleteEnv = makeManualPollEnv();
const immediateComplete = createDeferred();
beginRealStreamingTurn(
  "root-a",
  "codex",
  "agent.codex:1",
  "server-remote-1",
  () => undefined,
  () => immediateComplete.resolve(),
  undefined,
  immediateCompleteEnv.env,
);
await immediateComplete.promise;
assertEqual(calls.length, 1, "a poll that immediately observes live:false stops after just the initial poll");
assertEqual(
  calls[0]!.url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-a/activity/codex-sessions/agent.codex%3A1/transcript",
  "beginRealStreamingTurn fetches the server-scoped codex transcript route for a non-local serverRoute",
);
assertEqual(immediateCompleteEnv.clearedHandles.length, 1, "the interval is cleared after the single completing poll");

// stop() called externally (e.g. simulating unmount) clears the interval so
// no further polls are scheduled.

calls.length = 0;
nextResponses = [pollPartialFixture, pollPartialFixture, pollPartialFixture];
const stopEnv = makeManualPollEnv();
const firstUpdateBeforeStop = createDeferred();
const stopHandle = beginRealStreamingTurn(
  "root-a",
  "codex",
  "agent.codex:1",
  "server-local",
  () => firstUpdateBeforeStop.resolve(),
  () => undefined,
  undefined,
  stopEnv.env,
);
await firstUpdateBeforeStop.promise;
assertEqual(calls.length, 1, "the immediate poll fires once before stop() is called");
stopHandle.stop();
assertEqual(stopEnv.clearedHandles.length, 1, "stop() clears the scheduled interval, matching the live:false completion path");

// --- beginRealStreamingTurn: transcript fetch failure -> onError ----------
// Error on the first (immediate) poll still calls onError, then onComplete,
// and clears the interval so no further polls are attempted.

calls.length = 0;
nextResponses = [{ __httpError: 500, error: "daemon unavailable" }];
let capturedError: unknown;
let updateCalledAfterError = false;
const errorEnv = makeManualPollEnv();
const completeAfterError = createDeferred();
beginRealStreamingTurn(
  "root-a",
  "codex",
  "agent.codex:1",
  "server-local",
  () => {
    updateCalledAfterError = true;
  },
  () => completeAfterError.resolve(),
  (error) => {
    capturedError = error;
  },
  errorEnv.env,
);
await completeAfterError.promise;
assert(capturedError instanceof Error, "beginRealStreamingTurn's onError receives the transcript fetch failure");
assertEqual(
  (capturedError as Error).message,
  "daemon unavailable",
  "onError receives the error message from the failed transcript response body",
);
assert(!updateCalledAfterError, "onUpdate must not be called when the transcript fetch fails");
assertEqual(errorEnv.clearedHandles.length, 1, "a poll error clears the interval so no further polls are attempted");
