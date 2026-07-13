// Phase-1 local stub provider for
// `260711-feat-ws-dashboard-agent-activity-chat-ui`.
//
// CONTRACT: this module stands in for two not-yet-real dependencies, per the
// ticket's explicit allowance ("the static layout/shell work can proceed in
// parallel against a stub provider"):
//   - `260620-feat-ws-dashboard-agent-client-activity-sources`'s real
//     `activity.session.create`/`start` routes (currently inert types only,
//     see `activitySessionApi.ts`);
//   - `260624-feat-ws-dashboard-managed-cli-recent-sessions`'s real
//     cross-harness vendor-history list (still `idea/`, unimplemented).
//
// It is in-memory only (no `fetch`, no persistence across reload) and
// returns synthetic-but-shape-conformant data so the tile-launch and
// resume-popup flows are independently testable end-to-end before either
// real dependency lands. Request/response shapes stay conformant to
// `activitySessionApi.ts` so a later real handler can replace call sites
// here without reshaping callers. Do not add a workaround for a genuine
// `260620`/`260624` capability gap here — that belongs in those tickets.

import type {
  ActivityHistoryListRequest,
  ActivityHistoryListResponse,
  ActivitySessionCreateRequest,
  ActivitySessionCreateResponse,
  ActivitySessionStartRequest,
  ActivitySessionStartResponse,
} from "./activitySessionApi.js";
import type { AgentChatHarness, AgentChatSessionView } from "./agentChatSessions.js";
import type {
  ActivityItem,
  ActivitySourceDisplay,
  ActivityTranscript,
  TranscriptBlock,
} from "./workRootActivity.js";

export const agentChatHarnessLabel: Record<AgentChatHarness, string> = {
  codex: "Codex",
  opencode: "OpenCode",
  claude: "Claude",
};

let stubActivitySequence = 0;

function stubSourceDisplay(harness: AgentChatHarness): ActivitySourceDisplay {
  return {
    kind: `agent.${harness}`,
    label: agentChatHarnessLabel[harness],
    backend: harness,
    harness,
    tier: "stub",
    model: null,
  };
}

function stubTranscriptBlock(
  title: string,
  text: string,
  timestamp: string,
  cursor: string,
): TranscriptBlock {
  return {
    cursor,
    timestamp,
    renderKind: "markdown",
    title,
    text,
    data: null,
    degraded: false,
  };
}

function stubTranscript(
  workRootId: string,
  activityId: string,
  harness: AgentChatHarness,
  blocks: TranscriptBlock[],
): ActivityTranscript {
  return {
    workRootId,
    activityId,
    status: "available",
    sourceStatus: "ok",
    live: true,
    source: stubSourceDisplay(harness),
    blocks,
    nextCursor: null,
    hasMore: false,
    diagnostics: [],
  };
}

export function stubActivityId(
  workRootId: string,
  harness: AgentChatHarness,
): string {
  stubActivitySequence += 1;
  return `agent.${harness}:stub-${workRootId}-${stubActivitySequence}`;
}

export async function stubCreateActivitySession(
  request: ActivitySessionCreateRequest & { readonly harness: AgentChatHarness },
): Promise<ActivitySessionCreateResponse> {
  return { activityId: stubActivityId(request.workRootId, request.harness) };
}

export async function stubStartActivitySession(
  request: ActivitySessionStartRequest & { readonly harness: AgentChatHarness },
): Promise<ActivitySessionStartResponse & { readonly session: AgentChatSessionView }> {
  const now = new Date().toISOString();
  const session: AgentChatSessionView = {
    activityId: request.activityId,
    workRootId: request.workRootId,
    serverRoute: request.serverRoute,
    harness: request.harness,
    title: `${agentChatHarnessLabel[request.harness]} conversation`,
    createdAtMs: Date.now(),
    transcript: stubTranscript(request.workRootId, request.activityId, request.harness, [
      stubTranscriptBlock(
        "Session started",
        `New ${agentChatHarnessLabel[request.harness]} conversation started (stub provider — no real ${request.harness} adapter is wired in yet).`,
        now,
        "0",
      ),
    ]),
  };
  return { activityId: request.activityId, session };
}

/**
 * Convenience wrapper mirroring the tile-click flow: create then start a
 * brand-new stub session for a chosen harness, returning the resulting
 * synthetic session/transcript view directly.
 */
export async function stubStartNewAgentChatSession(
  workRootId: string,
  harness: AgentChatHarness,
  serverRoute?: string | null,
): Promise<AgentChatSessionView> {
  const created = await stubCreateActivitySession({
    workRootId,
    serverRoute,
    harness,
  });
  const started = await stubStartActivitySession({
    workRootId,
    activityId: created.activityId,
    serverRoute,
    harness,
  });
  return started.session;
}

/**
 * Convenience wrapper mirroring the resume-popup flow: start (resume) the
 * stub session behind an existing cross-harness history entry.
 */
export async function stubResumeAgentChatSession(
  item: ActivityItem,
  workRootId: string,
  serverRoute?: string | null,
): Promise<AgentChatSessionView> {
  const harness = agentChatHarnessFromSourceKind(item.kind);
  const started = await stubStartActivitySession({
    workRootId,
    activityId: item.id,
    serverRoute,
    harness,
  });
  const now = new Date().toISOString();
  return {
    ...started.session,
    title: item.label,
    transcript: stubTranscript(workRootId, item.id, harness, [
      stubTranscriptBlock(
        "Resumed",
        `Resumed "${item.label}" (stub provider, synthetic transcript).`,
        now,
        "0",
      ),
    ]),
  };
}

function agentChatHarnessFromSourceKind(kind: string): AgentChatHarness {
  if (kind === "agent.opencode") return "opencode";
  if (kind === "agent.claude") return "claude";
  return "codex";
}

// --- Cross-harness history stub (stands in for 260624) --------------------
// CONTRACT: scoped to the requesting `workRootId` only — never a global
// cross-work-root list, per the ticket's owner-confirmed scope decision.

const stubHistorySeed: ReadonlyArray<{
  readonly harness: AgentChatHarness;
  readonly alias: string;
  readonly ageMinutes: number;
  readonly sizeBytes: number;
}> = [
  {
    harness: "codex",
    alias: "refactor auth module",
    ageMinutes: 42,
    sizeBytes: 18_200,
  },
  {
    harness: "claude",
    alias: "investigate flaky test",
    ageMinutes: 130,
    sizeBytes: 9_400,
  },
  {
    harness: "opencode",
    alias: "draft release notes",
    ageMinutes: 1_440,
    sizeBytes: 4_100,
  },
];

export async function stubActivityHistoryList(
  request: ActivityHistoryListRequest,
): Promise<ActivityHistoryListResponse> {
  const now = Date.now();
  const items: ActivityItem[] = stubHistorySeed.map((seed, index) => {
    const updatedAt = new Date(now - seed.ageMinutes * 60_000).toISOString();
    return {
      id: `agent.${seed.harness}:history-${request.workRootId}-${index}`,
      kind: `agent.${seed.harness}`,
      label: seed.alias,
      status: "completed",
      live: false,
      attention: false,
      startedAt: updatedAt,
      updatedAt,
      finishedAt: updatedAt,
      source: stubSourceDisplay(seed.harness),
      transcript: { status: "available", available: true, cursor: null },
      diagnostics: [],
      metadata: { sizeBytes: seed.sizeBytes, stub: true },
    };
  });
  return { items };
}

export function stubHistoryEntrySizeBytes(item: ActivityItem): number | null {
  const value = item.metadata?.sizeBytes;
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}
