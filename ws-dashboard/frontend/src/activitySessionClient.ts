// Phase-1 real fetch client for
// `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` — the real
// counterpart to `activitySessionStub.ts`'s synthetic provider, targeting
// the REST-nested Codex/Claude routes actually registered in
// `codex_routes.rs`/`claude_routes.rs` (`router.rs:186-219,387-422`, both the
// server-local and `/servers/{serverRoute}/...` forms).
//
// CONTRACT: only "codex" and "claude" have a real adapter wired here.
// OpenCode has no real adapter at all yet and stays on `activitySessionStub.ts`
// — callers must branch by harness and never route an OpenCode session
// through this module (see `App.tsx` call sites).
//
// Mirrors `gitToolbar.ts`'s fetch-client idiom: a base-URL helper, a shared
// `readJson<T>` helper, one exported async function per REST operation.
//
// Phase 2 (`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`, Phase 2
// plan): `beginRealStreamingTurn` now polls the transcript endpoint on an
// injectable interval (mirroring `gitToolbar.ts`'s
// `GitRefreshSchedulerEnvironment` pattern) until the daemon reports
// `live: false`, diffing each poll's full-refetch block array against the
// previously-seen length via `blocksSincePolledLength`. This is deliberately
// polling, not SSE/websocket — see the Phase 2 plan's Codebase Findings for
// why the polling transport is an accepted pattern in this codebase, not a
// stopgap.

import { blocksSincePolledLength } from "./agentChatStreamMerge.js";
import { localCompatibleDashboardApiRoute } from "./resourceModel.js";
import {
  agentChatHarnessLabel,
  stubCapabilitiesForHarness,
} from "./activitySessionStub.js";
import type {
  ActivityHistoryListRequest,
  ActivityHistoryListResponse,
  ActivitySessionForkRequest,
  ActivitySessionForkResponse,
} from "./activitySessionApi.js";
import type { AgentChatSessionView } from "./agentChatSessions.js";
import type {
  ActivityItem,
  ActivitySourceDisplay,
  ActivityTranscript,
  TranscriptBlock,
} from "./workRootActivity.js";

// The subset of `AgentChatHarness` that has a real daemon route today.
export type RealAgentChatHarness = "codex" | "claude";

function sessionSegment(harness: RealAgentChatHarness): "codex-sessions" | "claude-sessions" {
  return harness === "codex" ? "codex-sessions" : "claude-sessions";
}

function sessionsBase(
  workRootId: string,
  harness: RealAgentChatHarness,
  serverRoute?: string | null,
): string {
  return localCompatibleDashboardApiRoute(serverRoute, [
    "work-roots",
    workRootId,
    "activity",
    sessionSegment(harness),
  ]);
}

function sessionBase(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  serverRoute?: string | null,
): string {
  return `${sessionsBase(workRootId, harness, serverRoute)}/${encodeURIComponent(activityId)}`;
}

async function readJson<T>(response: Response, fallback: string): Promise<T> {
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as {
      error?: string;
    } | null;
    throw new Error(body?.error ?? `HTTP ${response.status}: ${fallback}`);
  }
  return (await response.json()) as T;
}

function realSourceDisplay(harness: RealAgentChatHarness): ActivitySourceDisplay {
  // CONTRACT: `tier: "core"` matches the real-adapter source-display
  // convention already shipped by `codex_source_display`/`claude_cli.rs`'s
  // in-daemon `ActivitySourceDisplay` construction (`tier: Some("core")`),
  // distinguishing a real session from the stub's `tier: "stub"`.
  return {
    kind: `agent.${harness}`,
    label: agentChatHarnessLabel[harness],
    backend: harness,
    harness,
    tier: "core",
    model: null,
  };
}

function sessionViewFromTranscript(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  serverRoute: string | null | undefined,
  transcript: ActivityTranscript,
  title: string,
): AgentChatSessionView {
  return {
    activityId,
    workRootId,
    serverRoute,
    harness,
    title,
    createdAtMs: Date.now(),
    transcript,
    // No live daemon route reports `AgentClientCapabilities` yet for either
    // harness (see `stubCapabilitiesForHarness`'s export comment) — reuse
    // the same forward-declared table rather than duplicating it.
    capabilities: stubCapabilitiesForHarness(harness),
  };
}

type RealSessionCreateResponse = {
  readonly activityId: string;
};

async function createRealSession(
  workRootId: string,
  harness: RealAgentChatHarness,
  serverRoute?: string | null,
): Promise<string> {
  const response = await fetch(sessionsBase(workRootId, harness, serverRoute), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({}),
  });
  const result = await readJson<RealSessionCreateResponse>(
    response,
    `${harness} session create failed`,
  );
  return result.activityId;
}

async function fetchRealTranscript(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  serverRoute?: string | null,
): Promise<ActivityTranscript> {
  const response = await fetch(
    `${sessionBase(workRootId, harness, activityId, serverRoute)}/transcript`,
    { headers: { Accept: "application/json" } },
  );
  return readJson<ActivityTranscript>(response, `${harness} transcript fetch failed`);
}

/**
 * Real counterpart to `stubStartNewAgentChatSession`: create then hydrate a
 * brand-new Codex/Claude session against the real daemon routes.
 */
export async function startNewAgentChatSession(
  workRootId: string,
  harness: RealAgentChatHarness,
  serverRoute?: string | null,
): Promise<AgentChatSessionView> {
  const activityId = await createRealSession(workRootId, harness, serverRoute);
  const transcript = await fetchRealTranscript(workRootId, harness, activityId, serverRoute);
  return sessionViewFromTranscript(
    workRootId,
    harness,
    activityId,
    serverRoute,
    transcript,
    `${agentChatHarnessLabel[harness]} conversation`,
  );
}

/**
 * Real counterpart to `stubResumeAgentChatSession`: hydrate an existing
 * history entry's transcript from the real daemon route (no create call —
 * the session already exists). `harness` is the caller's already-resolved
 * `agentChatHarnessFromSourceKind(item.kind)` result, not re-derived here.
 */
export async function resumeAgentChatSession(
  item: ActivityItem,
  harness: RealAgentChatHarness,
  workRootId: string,
  serverRoute?: string | null,
): Promise<AgentChatSessionView> {
  const transcript = await fetchRealTranscript(workRootId, harness, item.id, serverRoute);
  return sessionViewFromTranscript(
    workRootId,
    harness,
    item.id,
    serverRoute,
    transcript,
    item.label,
  );
}

type RealSessionSummary = {
  readonly activityId: string;
  readonly label: string;
  readonly status: string;
  readonly updatedAt: string | null;
};

type RealSessionListResult = {
  readonly sessions: readonly RealSessionSummary[];
};

function activityItemFromSummary(
  harness: RealAgentChatHarness,
  summary: RealSessionSummary,
): ActivityItem {
  const live = summary.status === "running";
  return {
    id: summary.activityId,
    kind: `agent.${harness}`,
    label: summary.label,
    status: summary.status,
    live,
    attention: false,
    startedAt: null,
    updatedAt: summary.updatedAt,
    finishedAt: null,
    source: realSourceDisplay(harness),
    transcript: { status: "available", available: true, cursor: null },
    diagnostics: [],
    metadata: {},
  };
}

async function fetchRealSessionList(
  workRootId: string,
  harness: RealAgentChatHarness,
  serverRoute?: string | null,
): Promise<ActivityItem[]> {
  const response = await fetch(sessionsBase(workRootId, harness, serverRoute), {
    headers: { Accept: "application/json" },
  });
  const result = await readJson<RealSessionListResult>(
    response,
    `${harness} session list failed`,
  );
  return result.sessions.map((summary) => activityItemFromSummary(harness, summary));
}

/**
 * Real counterpart to `stubActivityHistoryList`, scoped to the two harnesses
 * with a real adapter (Codex, Claude). Callers that also need OpenCode
 * history must separately merge in `stubActivityHistoryList`'s OpenCode-only
 * entries (see `App.tsx`'s `onLoadHistory` call site) — this function alone
 * never covers OpenCode.
 */
export async function activityHistoryList(
  request: ActivityHistoryListRequest,
): Promise<ActivityHistoryListResponse> {
  const [codexItems, claudeItems] = await Promise.all([
    fetchRealSessionList(request.workRootId, "codex", request.serverRoute),
    fetchRealSessionList(request.workRootId, "claude", request.serverRoute),
  ]);
  return { items: [...codexItems, ...claudeItems] };
}

type RealControlResponse = {
  readonly applied: boolean;
  readonly data?: unknown;
};

/**
 * Real counterpart to `stubSteerActivitySession`. Codex-only: Claude has no
 * `/control` route or `ClaudeControlRequest` at all today
 * (`claude_routes.rs`), and the capability table already only enables
 * `steer` for Codex, so no call site ever needs a Claude variant of this.
 */
export async function steerActivitySession(
  workRootId: string,
  activityId: string,
  text: string,
  serverRoute?: string | null,
): Promise<RealControlResponse> {
  const response = await fetch(
    `${sessionBase(workRootId, "codex", activityId, serverRoute)}/control`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ action: "steer", text }),
    },
  );
  return readJson<RealControlResponse>(response, "codex steer failed");
}

/**
 * Real counterpart to `stubForkActivitySession`. Codex-only: Claude
 * fork-from-here is Hack-tier and explicitly out of scope
 * (`AgentChatCapabilities.fork` is `false` for Claude in every tiering
 * table). POSTs the `/control` `Fork` action
 * (`260713-feat-ws-dashboard-activity-session-fork-cursor`'s Phase 1 target)
 * and, now that the daemon's `CodexControlRequest::Fork` handler exists
 * (`codex_routes.rs`, `260713` Phase 3), reads the daemon's actual
 * `data.activityId`/`data.cutCursor` out of the response rather than echoing
 * the request back. Falls back to the request's own values only if the
 * daemon response is missing `data` entirely (a shape the Phase-3 handler
 * should never produce, but `readJson`'s `unknown`-typed `data` still needs a
 * defensive default so a malformed response degrades instead of throwing).
 */
export async function forkActivitySession(
  request: ActivitySessionForkRequest,
): Promise<ActivitySessionForkResponse> {
  const response = await fetch(
    `${sessionBase(request.workRootId, "codex", request.activityId, request.serverRoute)}/control`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ action: "fork", cutCursor: request.cutCursor ?? null }),
    },
  );
  const result = await readJson<RealControlResponse>(response, "codex fork failed");
  const data = result.data as { activityId?: unknown; cutCursor?: unknown } | undefined;
  const activityId =
    typeof data?.activityId === "string" ? data.activityId : request.activityId;
  // Only fall back to the request's own `cutCursor` when `data` itself is
  // missing entirely (the malformed-response defensive case). When `data` is
  // present, trust its `cutCursor` as-is — including a legitimate `null`,
  // which the daemon now returns when the requested cut point failed to
  // resolve to a known turn (full-thread fork). Falling back to the request's
  // raw `cutCursor` in that case would silently re-introduce the "echoes the
  // request instead of the actually-applied cut" bug this contract exists to
  // fix (260713 Phase 3 review).
  const cutCursor =
    data === undefined
      ? request.cutCursor ?? null
      : typeof data.cutCursor === "string"
        ? data.cutCursor
        : null;
  return { activityId, cutCursor };
}

/**
 * Hydrate a full `AgentChatSessionView` directly from an already-created
 * `activityId`, with no create call — the Codex-fork counterpart to
 * `startNewAgentChatSession`/`resumeAgentChatSession`. Needed because
 * `forkActivitySession`'s response is a bare `{activityId, cutCursor}` (the
 * daemon's `Fork` control response never inlines a transcript, unlike
 * `thread/fork`'s own provider-side response, which the daemon already
 * consumed to seed the new session's projector) — `App.tsx`'s
 * `forkAgentChatFromBubble` calls this immediately after a successful fork to
 * turn that bare id into the same shape the pane-registration flow expects.
 */
export async function hydrateForkedAgentChatSession(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  serverRoute: string | null | undefined,
  title: string,
): Promise<AgentChatSessionView> {
  const transcript = await fetchRealTranscript(workRootId, harness, activityId, serverRoute);
  return sessionViewFromTranscript(workRootId, harness, activityId, serverRoute, transcript, title);
}

/**
 * Real counterpart to `stubSendActivitySession`/the prompt half of a turn:
 * POSTs the prompt text to the real per-harness `/prompt` route. Used both
 * directly (`sendAgentChatMessage`) and by `beginRealStreamingTurn` below.
 */
export async function sendAgentChatPrompt(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  text: string,
  serverRoute?: string | null,
): Promise<void> {
  const response = await fetch(
    `${sessionBase(workRootId, harness, activityId, serverRoute)}/prompt`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ text }),
    },
  );
  await readJson<unknown>(response, `${harness} prompt send failed`);
}

export type RealStreamingHandle = {
  readonly stop: () => void;
};

/**
 * Injectable timer environment for `beginRealStreamingTurn`'s poll loop,
 * mirroring `gitToolbar.ts`'s `GitRefreshSchedulerEnvironment` — passed in
 * rather than calling `globalThis.setInterval`/`clearInterval` directly, so
 * tests can supply a manual-tick fake instead of waiting on real intervals
 * (see `activitySessionClient.test.ts`).
 */
export type RealStreamingPollEnvironment = {
  setInterval: (listener: () => void, ms: number) => number;
  clearInterval: (handle: number) => void;
};

const defaultRealStreamingPollEnvironment: RealStreamingPollEnvironment = {
  setInterval: (listener, ms) => globalThis.setInterval(listener, ms) as unknown as number,
  clearInterval: (handle) => globalThis.clearInterval(handle),
};

// No real incremental/delta transcript endpoint exists (see the Phase 2
// plan's Codebase Findings on `workRootActivity.ts`'s `ActivityTranscript`
// shape) — this is how often `beginRealStreamingTurn` re-fetches the full
// transcript and diffs it via `blocksSincePolledLength`. Distinct from
// `terminalOutputPollIntervalMs` (`App.tsx`), which polls a different feature
// at a different cadence.
export const realStreamingPollIntervalMs = 1500;

/**
 * Real counterpart to `stubBeginStreamingTurn`: polls the transcript
 * endpoint on `realStreamingPollIntervalMs` until the daemon reports
 * `live: false`, diffing each poll's full-refetch block array against the
 * previously-seen length (`blocksSincePolledLength`) so `onUpdate` only ever
 * receives the re-included in-progress tail block plus newly appended
 * blocks, not the whole transcript every tick.
 *
 * `codex_activity_transcript`/`claude_activity_transcript`
 * (`codex_app_server.rs:881-895`, `claude_cli.rs:882-896`) set
 * `live: projector.is_turn_active()` on every transcript response — this is
 * the only turn-completion signal available; there is no separate
 * "finished" event or SSE/websocket push (that stays deferred future work,
 * see the Phase 2 plan's Out of Scope).
 *
 * CONTRACT: this does not itself send the prompt — the caller
 * (`AgentChatPaneBody`'s `beginSimulatedTurn`) already triggers the real send
 * via `actions.onSendMessage` -> `sendAgentChatMessage` ->
 * `sendAgentChatPrompt` before starting the "stream," so re-sending here
 * would double-POST the same turn. This function only performs the
 * transcript poll half of the turn.
 */
export function beginRealStreamingTurn(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  serverRoute: string | null | undefined,
  onUpdate: (blocks: readonly TranscriptBlock[]) => void,
  onComplete?: () => void,
  onError?: (error: unknown) => void,
  env: RealStreamingPollEnvironment = defaultRealStreamingPollEnvironment,
): RealStreamingHandle {
  let lastSeenLength = 0;
  let intervalHandle: number | null = null;
  let stopped = false;

  const clearScheduledPoll = () => {
    if (intervalHandle !== null) {
      env.clearInterval(intervalHandle);
      intervalHandle = null;
    }
  };

  const stop = () => {
    stopped = true;
    clearScheduledPoll();
  };

  const poll = () => {
    void fetchRealTranscript(workRootId, harness, activityId, serverRoute)
      .then((transcript) => {
        if (stopped) return;
        const delta = blocksSincePolledLength(transcript.blocks, lastSeenLength);
        lastSeenLength = transcript.blocks.length;
        if (delta.length > 0) {
          onUpdate(delta);
        }
        if (!transcript.live) {
          stopped = true;
          clearScheduledPoll();
          onComplete?.();
        }
      })
      .catch((error) => {
        if (stopped) return;
        stopped = true;
        onError?.(error);
        clearScheduledPoll();
        onComplete?.();
      });
  };

  // Fire one immediate poll at call time (don't wait a full interval for the
  // first update), then schedule the interval for subsequent polls.
  poll();
  intervalHandle = env.setInterval(poll, realStreamingPollIntervalMs);

  return { stop };
}
