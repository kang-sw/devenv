import {
  ATTENTION_SOURCE_CLOSED_READY_STATE,
  acknowledgeAttentionEntry,
  aggregateNavAttentionCounts,
  aggregateNavAttentionTone,
  attentionEventsEndpoint,
  navAttentionCountsSignature,
  navAttentionTone,
  parseAgentAttentionEntry,
  parseAgentAttentionSnapshot,
  pendingAttentionStateFor,
  shouldReplaceAttentionSourceOnError,
  type AgentAttentionEntry,
  type NavAttentionPane,
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

// --- aggregateNavAttentionCounts (260725 Phase 7 nav-row counter) --------

function navPane(overrides: Partial<NavAttentionPane> = {}): NavAttentionPane {
  return {
    serverRoute: "server-local",
    terminalId: "term_1",
    workRootId: "root-a",
    status: "running",
    profileId: "claude",
    ...overrides,
  };
}

const ROOT_A = "server-local/root-a";
const ROOT_B = "server-local/root-b";

assertDeepEqual(
  aggregateNavAttentionCounts([], { attentionByKey: {}, acknowledgements: {} }),
  {},
  "no panes aggregate to an empty map",
);

// THE partition rule this counter is pinned to: the agent predicate is the
// pane-recorded spawn profile, not the presence of an attention entry. A
// shell terminal must never enter the agent map even when (impossibly) an
// entry exists for it, and an agent that has never finished a turn must
// still be counted.
assertDeepEqual(
  aggregateNavAttentionCounts(
    [
      navPane({ terminalId: "term_shell", profileId: null }),
      navPane({ terminalId: "term_agent" }),
    ],
    {
      attentionByKey: {
        "server-local/term_shell": {
          terminalId: "term_shell",
          workRootId: "root-a",
          state: "ready",
          updatedAtMs: 10,
        },
      },
      acknowledgements: {},
    },
  ),
  { [ROOT_A]: { agents: 1, working: 0, ready: 0 } },
  "profileId partitions agent from shell terminals; a shell terminal's entry is unreachable, and a fresh agent with no entry still counts as an agent",
);

assertDeepEqual(
  aggregateNavAttentionCounts(
    [
      navPane({ terminalId: "term_w" }),
      navPane({ terminalId: "term_r" }),
      navPane({ terminalId: "term_idle" }),
      navPane({ terminalId: "term_other", workRootId: "root-b" }),
    ],
    {
      attentionByKey: {
        "server-local/term_w": {
          terminalId: "term_w",
          workRootId: "root-a",
          state: "working",
          updatedAtMs: 1,
        },
        "server-local/term_r": {
          terminalId: "term_r",
          workRootId: "root-a",
          state: "ready",
          updatedAtMs: 2,
        },
        "server-local/term_idle": {
          terminalId: "term_idle",
          workRootId: "root-a",
          state: "idle",
          updatedAtMs: 3,
        },
        "server-local/term_other": {
          terminalId: "term_other",
          workRootId: "root-b",
          state: "ready",
          updatedAtMs: 4,
        },
      },
      acknowledgements: {},
    },
  ),
  {
    [ROOT_A]: { agents: 3, working: 1, ready: 1 },
    [ROOT_B]: { agents: 1, working: 0, ready: 1 },
  },
  "working and ready are counted as a split, idle contributes to neither, and the scope is per work root",
);

assertDeepEqual(
  aggregateNavAttentionCounts([navPane({ status: "exited" })], {
    attentionByKey: {
      "server-local/term_1": {
        terminalId: "term_1",
        workRootId: "root-a",
        state: "ready",
        updatedAtMs: 7,
      },
    },
    acknowledgements: {},
  }),
  { [ROOT_A]: { agents: 1, working: 0, ready: 0 } },
  "a non-running agent session contributes to neither working nor ready (the liveness gate), while still being a mounted agent pane",
);

assertDeepEqual(
  aggregateNavAttentionCounts([navPane()], {
    attentionByKey: {
      "server-local/term_1": {
        terminalId: "term_1",
        workRootId: "root-a",
        state: "ready",
        updatedAtMs: 7,
      },
    },
    acknowledgements: { "server-local/term_1": 7 },
  }),
  { [ROOT_A]: { agents: 1, working: 0, ready: 0 } },
  "an acknowledged entry contributes to neither half - the row badge reuses the tab indicator's single ack watermark, with no second watermark of its own",
);

assertDeepEqual(
  aggregateNavAttentionCounts([navPane()], {
    attentionByKey: {
      "server-local/term_1": {
        terminalId: "term_1",
        workRootId: "root-a",
        state: "ready",
        updatedAtMs: 8,
      },
    },
    acknowledgements: { "server-local/term_1": 7 },
  }),
  { [ROOT_A]: { agents: 1, working: 0, ready: 1 } },
  "an ack of an OLDER revision must not suppress the row badge for a newer turn boundary",
);

assertDeepEqual(
  aggregateNavAttentionCounts(
    [navPane({ serverRoute: "server-windows" })],
    {
      attentionByKey: {
        "server-windows/term_1": {
          terminalId: "term_1",
          workRootId: "root-a",
          state: "ready",
          updatedAtMs: 1,
        },
      },
      acknowledgements: {},
    },
  ),
  { "server-windows/root-a": { agents: 1, working: 0, ready: 1 } },
  "both the root key and the terminal join key are server-scoped, matching terminalAttentionKey's join exactly",
);

// --- navAttentionTone / aggregateNavAttentionTone ------------------------

assertEqual(navAttentionTone(undefined), null, "a root with no agents has no tone");
assertEqual(
  navAttentionTone({ agents: 2, working: 0, ready: 0 }),
  null,
  "agents present but nothing pending shows no tone",
);
assertEqual(
  navAttentionTone({ agents: 1, working: 1, ready: 0 }),
  "working",
  "working alone shows the working tone",
);
assertEqual(
  navAttentionTone({ agents: 1, working: 0, ready: 1 }),
  "ready",
  "ready alone shows the ready tone",
);
assertEqual(
  navAttentionTone({ agents: 2, working: 1, ready: 1 }),
  "ready",
  "PINNED priority: ready outranks working when both are pending on the same row",
);

{
  const countsByRoot = {
    [ROOT_A]: { agents: 1, working: 1, ready: 0 },
    [ROOT_B]: { agents: 1, working: 0, ready: 1 },
  };
  assertEqual(
    aggregateNavAttentionTone(countsByRoot, [ROOT_A, ROOT_B]),
    "ready",
    "PINNED server roll-up: ready outranks working across a server's work roots",
  );
  assertEqual(
    aggregateNavAttentionTone(countsByRoot, [ROOT_B, ROOT_A]),
    "ready",
    "the roll-up is order-independent - a later working root cannot demote an earlier ready one",
  );
  assertEqual(
    aggregateNavAttentionTone(countsByRoot, [ROOT_A]),
    "working",
    "a server sees only the roots it is given, so a neighbour's ready never leaks in",
  );
  assertEqual(
    aggregateNavAttentionTone(countsByRoot, []),
    null,
    "a server with no work roots has no tone",
  );
  assertEqual(
    aggregateNavAttentionTone(countsByRoot, ["server-local/root-missing"]),
    null,
    "an unknown root key contributes nothing rather than throwing",
  );
}

// --- navAttentionCountsSignature ----------------------------------------

assertEqual(
  navAttentionCountsSignature({}),
  "",
  "an empty count map has an empty signature",
);
assertEqual(
  navAttentionCountsSignature({
    [ROOT_B]: { agents: 1, working: 0, ready: 1 },
    [ROOT_A]: { agents: 2, working: 1, ready: 0 },
  }),
  "server-local/root-a:2:1:0,server-local/root-b:1:0:1",
  "the signature is sorted, so key insertion order alone never invalidates the memo",
);
assertEqual(
  navAttentionCountsSignature({ [ROOT_A]: { agents: 2, working: 1, ready: 0 } }) ===
    navAttentionCountsSignature({ [ROOT_A]: { agents: 2, working: 0, ready: 1 } }),
  false,
  "a working -> ready transition at a constant agent count MUST change the signature, or the nav row would never repaint on a turn boundary",
);
