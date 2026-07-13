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
// `readJson<T>` helper, one exported async function per REST operation. No
// polling/streaming loop yet — `beginRealStreamingTurn` does a single
// transcript fetch, matching Phase 1's non-goal of real streaming/polling
// delivery (that is Phase 2).

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
 * table). Wires the request against the future `/control` `Fork` action
 * (`260713-feat-ws-dashboard-activity-session-fork-cursor`'s Phase 1 target)
 * even though no `CodexControlRequest::Fork` variant or fork route exists
 * yet (`codex_routes.rs:88-93`) — that lands in Phase 3. Until then this
 * call is expected to reject (the daemon rejects the unrecognized `"fork"`
 * action tag), which is consistent with Phase 1's verification bar being
 * "correct request shape," not "succeeds end-to-end."
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
  await readJson<RealControlResponse>(response, "codex fork failed");
  // Phase 3 owns defining what a real Fork response actually returns (e.g. a
  // new `activityId`); Phase 1 has nothing to project it into yet, so the
  // original activityId/cutCursor are echoed back rather than invented.
  return { activityId: request.activityId, cutCursor: request.cutCursor ?? null };
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
 * Minimal real counterpart to `stubBeginStreamingTurn`: fetches the
 * transcript exactly once and hands the resulting blocks to `onUpdate`, then
 * fires `onComplete`. No polling loop yet — Phase 2 upgrades this to
 * continuous diff-polling. `stop()` is a no-op today (nothing to cancel for
 * a single fetch) but is kept so call sites can treat both handles
 * uniformly.
 *
 * CONTRACT: this does not itself send the prompt — the caller
 * (`AgentChatPaneBody`'s `beginSimulatedTurn`) already triggers the real send
 * via `actions.onSendMessage` -> `sendAgentChatMessage` ->
 * `sendAgentChatPrompt` before starting the "stream," so re-sending here
 * would double-POST the same turn. This function only performs the
 * transcript fetch half of the turn.
 */
export function beginRealStreamingTurn(
  workRootId: string,
  harness: RealAgentChatHarness,
  activityId: string,
  serverRoute: string | null | undefined,
  onUpdate: (blocks: readonly TranscriptBlock[]) => void,
  onComplete?: () => void,
  onError?: (error: unknown) => void,
): RealStreamingHandle {
  void fetchRealTranscript(workRootId, harness, activityId, serverRoute)
    .then((transcript) => {
      onUpdate(transcript.blocks);
    })
    .catch((error) => {
      onError?.(error);
    })
    .finally(() => {
      onComplete?.();
    });
  return { stop: () => undefined };
}
