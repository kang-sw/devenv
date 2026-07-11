// Phase-1 method-shape draft for
// `260620-feat-ws-dashboard-agent-client-activity-sources`.
//
// CONTRACT: these are inert request/response type shapes only. No fetch
// helper, no route registration, and no handler exists yet — Phase 2+ will
// register these under the same dual server-scoped/local-gateway pattern
// used by the existing read routes (see `workRootActivityEndpoint` /
// `workRootActivityTranscriptEndpoint` in `./workRootActivity.js` and
// `#remote-activity-git-workspace-operations` in
// `ai-docs/spec/ws-web-dashboard/index.md`), under the existing
// `/api/dashboard/servers/{serverRoute}/...`-scoped, `workRootId`/
// `activityId` identity model — not a new identity/routing scheme.
//
// Method names below are illustrative, not a final route contract. The
// common subset (`activity.history.list`, `activity.session.*` create/
// start/send/usage) is intended to work the same way across every
// interactive provider source. The per-harness-gated methods
// (`activity.session.compact/steer/goal.*/rewind/fork/skills`) must be
// hidden/disabled by the caller unless the active harness's adapter
// reports the matching `AgentClientCapabilities` flag from
// `ws-dashboard-core`'s `agent_client_provider` module — see
// `ai-docs/mental-model/ws-dashboard-agent-harness.md`'s Passthrough/
// Overlay/Hack/Unavailable tiering for which (harness, capability) cells
// may back a shipped method at all.

import type { ActivityItem, ActivityTranscript } from "./workRootActivity.js";

export type ActivityHistoryListRequest = {
  readonly workRootId: string;
  readonly serverRoute?: string | null;
};

export type ActivityHistoryListResponse = {
  readonly items: ActivityItem[];
};

export type ActivitySessionCreateRequest = {
  readonly workRootId: string;
  readonly serverRoute?: string | null;
  readonly initialPrompt?: string;
};

export type ActivitySessionCreateResponse = {
  readonly activityId: string;
};

export type ActivitySessionStartRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionStartResponse = {
  readonly activityId: string;
};

export type ActivitySessionSendRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly text: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionSendResponse = {
  readonly accepted: boolean;
};

// Read-only usage display; never a control/limit-enforcement surface.
export type ActivitySessionUsageRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionUsageResponse = {
  readonly contextTokensUsed: number | null;
  readonly contextTokensLimit: number | null;
};

// --- Per-harness-gated methods -------------------------------------------
// Every request below carries the same (workRootId, activityId) identity
// pair as the common subset. Callers must gate visibility on the active
// harness's reported `AgentClientCapabilities` flag (`compact`, `steer`,
// `goal`, `rewind`, `fork`, `skills`), not assume availability.

export type ActivitySessionCompactRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionCompactResponse = {
  readonly accepted: boolean;
};

export type ActivitySessionSteerRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly text: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionSteerResponse = {
  readonly accepted: boolean;
};

export type ActivitySessionGoalSetRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly goal: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionGoalGetRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionGoalGetResponse = {
  readonly goal: string | null;
};

export type ActivitySessionGoalClearRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionRewindRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  // CONTRACT: coarse turn-count semantics only (matches the fixture-verified
  // Codex `thread/rollback` shape, itself confirmed deprecated for removal
  // per the mental model) — never a point-based/file-reverting rewind.
  readonly turnsToDrop: number;
  readonly serverRoute?: string | null;
};

export type ActivitySessionRewindResponse = {
  readonly activityId: string;
};

export type ActivitySessionForkRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionForkResponse = {
  readonly activityId: string;
};

export type ActivitySessionSkillsRequest = {
  readonly workRootId: string;
  readonly activityId: string;
  readonly serverRoute?: string | null;
};

export type ActivitySessionSkillSummary = {
  readonly name: string;
  readonly description: string | null;
};

export type ActivitySessionSkillsResponse = {
  readonly skills: ActivitySessionSkillSummary[];
};

// Kept for reference: a fetched transcript by way of the existing read
// route stays the shared browser contract for backfill; no parallel
// transcript type exists in this file.
export type ActivitySessionTranscriptRef = ActivityTranscript;
