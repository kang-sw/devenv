// Phase-1 type-level contract test for
// `260620-feat-ws-dashboard-agent-client-activity-sources`. There is no
// fetch helper or route to exercise yet (see the CONTRACT note atop
// `activitySessionApi.ts`); this file only asserts the drafted request/
// response shapes compile and carry the expected identity fields, so a
// later Phase 2+ route implementation has a stable shape to conform to.

import type {
  ActivityHistoryListRequest,
  ActivitySessionCompactRequest,
  ActivitySessionCreateRequest,
  ActivitySessionForkRequest,
  ActivitySessionGoalGetRequest,
  ActivitySessionRewindRequest,
  ActivitySessionSendRequest,
  ActivitySessionSkillsResponse,
  ActivitySessionStartRequest,
  ActivitySessionSteerRequest,
  ActivitySessionUsageResponse,
} from "./activitySessionApi.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

// Common subset: identity shape matches the existing workRootId/activityId
// model, not a parallel scheme.
const historyRequest: ActivityHistoryListRequest = {
  workRootId: "root-local-abc",
};
assertEqual(
  historyRequest.workRootId,
  "root-local-abc",
  "activity.history.list request carries the existing opaque workRootId identity",
);

const createRequest: ActivitySessionCreateRequest = {
  workRootId: "root-local-abc",
  initialPrompt: "review this diff",
};
assertEqual(
  createRequest.initialPrompt,
  "review this diff",
  "activity.session.create request accepts an optional initial prompt",
);

const startRequest: ActivitySessionStartRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
};
assertEqual(
  startRequest.activityId,
  "agent.codex:thread-1",
  "activity.session.start request carries the opaque activityId, not a provider thread id",
);

const sendRequest: ActivitySessionSendRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
  text: "continue",
};
assertEqual(
  sendRequest.text,
  "continue",
  "activity.session.send request carries prompt text",
);

const usageResponse: ActivitySessionUsageResponse = {
  contextTokensUsed: 1200,
  contextTokensLimit: 128000,
};
assertEqual(
  usageResponse.contextTokensUsed,
  1200,
  "activity.session.usage response stays read-only display data",
);

// Per-harness-gated methods: same (workRootId, activityId) identity pair as
// the common subset, gated by AgentClientCapabilities at the call site (not
// enforced by the type shape itself, since gating is runtime behavior).
const compactRequest: ActivitySessionCompactRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
};
assertEqual(
  compactRequest.workRootId,
  "root-local-abc",
  "per-harness-gated activity.session.compact request reuses the common identity pair",
);

const steerRequest: ActivitySessionSteerRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
  text: "focus on the failing test",
};
assertEqual(
  steerRequest.text,
  "focus on the failing test",
  "activity.session.steer request carries steering text",
);

const goalGetRequest: ActivitySessionGoalGetRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
};
assertEqual(
  goalGetRequest.activityId,
  "agent.codex:thread-1",
  "activity.session.goal.get request reuses the common identity pair",
);

const rewindRequest: ActivitySessionRewindRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
  turnsToDrop: 2,
};
assertEqual(
  rewindRequest.turnsToDrop,
  2,
  "activity.session.rewind request models coarse turn-count semantics, not point-based rewind",
);

const forkRequest: ActivitySessionForkRequest = {
  workRootId: "root-local-abc",
  activityId: "agent.codex:thread-1",
};
assertEqual(
  forkRequest.activityId,
  "agent.codex:thread-1",
  "activity.session.fork request reuses the common identity pair",
);

const skillsResponse: ActivitySessionSkillsResponse = {
  skills: [{ name: "code-review", description: null }],
};
assertEqual(
  skillsResponse.skills[0]?.name,
  "code-review",
  "activity.session.skills response carries a bounded skill summary list",
);
