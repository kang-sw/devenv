import {
  ATTENTION_SOURCE_CLOSED_READY_STATE,
  attentionEventsEndpoint,
  parseAgentAttentionEntry,
  parseAgentAttentionSnapshot,
  shouldReplaceAttentionSourceOnError,
  type AgentAttentionEntry,
} from "./agentAttention.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

// --- attentionEventsEndpoint -------------------------------------------

assertEqual(
  attentionEventsEndpoint(),
  "/api/dashboard/terminals/attention/events",
  "local attention endpoint has no work-root or server-route segment",
);
assertEqual(
  attentionEventsEndpoint("server-local"),
  "/api/dashboard/terminals/attention/events",
  "the local server route is treated as local, not server-scoped",
);
assertEqual(
  attentionEventsEndpoint("server-windows"),
  "/api/dashboard/servers/server-windows/terminals/attention/events",
  "a non-local server route addresses the server-scoped sibling",
);
assertEqual(
  attentionEventsEndpoint(null),
  "/api/dashboard/terminals/attention/events",
  "a null server route falls back to the local endpoint",
);

// --- parseAgentAttentionEntry -------------------------------------------

const validEntry: AgentAttentionEntry = {
  terminalId: "term_abc",
  workRootId: "root-abc",
  state: "working",
  updatedAtMs: 1_700_000_000_000,
};

assertDeepEqual(
  parseAgentAttentionEntry({
    type: "terminal.attentionChanged",
    terminalId: "term_abc",
    workRootId: "root-abc",
    state: "working",
    updatedAtMs: 1_700_000_000_000,
  }),
  validEntry,
  "a well-formed attention frame parses, ignoring the extra 'type' field",
);

for (const state of ["working", "ready", "idle"] as const) {
  assertEqual(
    parseAgentAttentionEntry({ ...validEntry, state })?.state,
    state,
    `parseAgentAttentionEntry accepts the '${state}' vocabulary value`,
  );
}

assertEqual(
  parseAgentAttentionEntry({ ...validEntry, state: "blocked" }),
  null,
  "an unrecognized state value is rejected, not silently coerced",
);
assertEqual(
  parseAgentAttentionEntry({ ...validEntry, terminalId: 42 }),
  null,
  "a non-string terminalId is rejected",
);
assertEqual(
  parseAgentAttentionEntry({ ...validEntry, updatedAtMs: "not-a-number" }),
  null,
  "a non-number updatedAtMs is rejected",
);
assertEqual(
  parseAgentAttentionEntry(null),
  null,
  "a non-object payload is rejected",
);
assertEqual(
  parseAgentAttentionEntry("term_abc"),
  null,
  "a bare string payload is rejected",
);

// --- parseAgentAttentionSnapshot -----------------------------------------

assertDeepEqual(
  parseAgentAttentionSnapshot({ items: [] }),
  [],
  "an empty snapshot parses to an empty list, not null",
);
assertDeepEqual(
  parseAgentAttentionSnapshot({
    items: [
      {
        terminalId: "term_a",
        workRootId: "root-a",
        state: "working",
        updatedAtMs: 1,
      },
      {
        terminalId: "term_b",
        workRootId: "root-b",
        state: "ready",
        updatedAtMs: 2,
      },
    ],
  }),
  [
    { terminalId: "term_a", workRootId: "root-a", state: "working", updatedAtMs: 1 },
    { terminalId: "term_b", workRootId: "root-b", state: "ready", updatedAtMs: 2 },
  ],
  "a snapshot with multiple pending entries parses every item in order",
);
assertEqual(
  parseAgentAttentionSnapshot({
    items: [
      { terminalId: "term_a", workRootId: "root-a", state: "working", updatedAtMs: 1 },
      { terminalId: "term_b", workRootId: "root-b", state: "not-a-real-state", updatedAtMs: 2 },
    ],
  }),
  null,
  "one malformed item invalidates the whole snapshot rather than dropping it silently",
);
assertEqual(
  parseAgentAttentionSnapshot({ items: "not-an-array" }),
  null,
  "a non-array items field is rejected",
);
assertEqual(
  parseAgentAttentionSnapshot({}),
  null,
  "a payload with no items field is rejected",
);
assertEqual(
  parseAgentAttentionSnapshot(null),
  null,
  "a non-object snapshot payload is rejected",
);

// --- shouldReplaceAttentionSourceOnError (review finding C) --------------

assertEqual(
  ATTENTION_SOURCE_CLOSED_READY_STATE,
  2,
  "must match the DOM EventSource.CLOSED numeric value this module deliberately avoids importing",
);
assertEqual(
  shouldReplaceAttentionSourceOnError(ATTENTION_SOURCE_CLOSED_READY_STATE),
  true,
  "a permanently-failed (CLOSED) EventSource must be replaced - this is the only resync path once a 401/502 kills the connection",
);
assertEqual(
  shouldReplaceAttentionSourceOnError(0),
  false,
  "a CONNECTING EventSource is already auto-retrying with its own backoff and must be left alone",
);
assertEqual(
  shouldReplaceAttentionSourceOnError(1),
  false,
  "an OPEN EventSource has not failed and must be left alone",
);
