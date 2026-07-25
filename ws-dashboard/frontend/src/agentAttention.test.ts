import {
  ATTENTION_SOURCE_CLOSED_READY_STATE,
  acknowledgeAttentionEntry,
  attentionEventsEndpoint,
  parseAgentAttentionEntry,
  parseAgentAttentionSnapshot,
  pendingAttentionStateFor,
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

// --- pendingAttentionStateFor (260725 Phase 6 tab indicator) -------------

const readyEntry: AgentAttentionEntry = {
  terminalId: "term_abc",
  workRootId: "root-abc",
  state: "ready",
  updatedAtMs: 1_700_000_000_000,
};

assertEqual(
  pendingAttentionStateFor(readyEntry, undefined, "running"),
  "ready",
  "an unacknowledged non-idle entry on a running session shows its state",
);
assertEqual(
  pendingAttentionStateFor(
    { ...readyEntry, state: "working" },
    undefined,
    "running",
  ),
  "working",
  "the working state is surfaced too, not only ready",
);
assertEqual(
  pendingAttentionStateFor({ ...readyEntry, state: "idle" }, undefined, "running"),
  null,
  "idle is the explicit nothing-to-show state, never a badge",
);
assertEqual(
  pendingAttentionStateFor(undefined, undefined, "running"),
  null,
  "a terminal with no attention entry at all shows nothing",
);
assertEqual(
  pendingAttentionStateFor(readyEntry, readyEntry.updatedAtMs, "running"),
  null,
  "an acknowledged revision clears the indicator",
);
assertEqual(
  pendingAttentionStateFor(readyEntry, readyEntry.updatedAtMs - 1, "running"),
  "ready",
  "an ack of an OLDER revision must not suppress a newer turn boundary",
);
// The stale-indicator fix (plan step 2): the daemon keeps listing a
// dead-but-in-grace-window terminal for up to 30s with a non-running status,
// and its AttentionHub entry outlives even that - so liveness is gated here,
// at render time, rather than by a daemon-side reaper.
for (const deadStatus of ["exited", "terminated", "error", "starting"]) {
  assertEqual(
    pendingAttentionStateFor(readyEntry, undefined, deadStatus),
    null,
    `a '${deadStatus}' session never shows a stale attention indicator`,
  );
}

// --- acknowledgeAttentionEntry ------------------------------------------

assertDeepEqual(
  acknowledgeAttentionEntry({}, "server-local/term_abc", readyEntry),
  { "server-local/term_abc": readyEntry.updatedAtMs },
  "acknowledging records the entry's current revision under its key",
);
{
  const acknowledged = { "server-local/term_abc": readyEntry.updatedAtMs };
  assertEqual(
    acknowledgeAttentionEntry(acknowledged, "server-local/term_abc", readyEntry),
    acknowledged,
    "re-acknowledging the same revision returns the same object identity (no re-render)",
  );
  assertEqual(
    acknowledgeAttentionEntry(acknowledged, "server-local/term_abc", undefined),
    acknowledged,
    "selecting a terminal with no attention entry is a no-op",
  );
  assertDeepEqual(
    acknowledgeAttentionEntry(acknowledged, "server-local/term_abc", {
      ...readyEntry,
      state: "working",
      updatedAtMs: readyEntry.updatedAtMs + 5,
    }),
    { "server-local/term_abc": readyEntry.updatedAtMs + 5 },
    "a newer revision overwrites the watermark rather than being ignored",
  );
}
