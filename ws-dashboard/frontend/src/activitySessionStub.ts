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
  extra: Partial<Pick<TranscriptBlock, "role" | "turnId" | "renderKind" | "data" | "degraded">> = {},
): TranscriptBlock {
  return {
    cursor,
    timestamp,
    renderKind: extra.renderKind ?? "markdown",
    title,
    text,
    data: extra.data ?? null,
    degraded: extra.degraded ?? false,
    role: extra.role,
    turnId: extra.turnId,
  };
}

// --- Phase 2 streaming/bubble demo content --------------------------------
// CONTRACT: this section is stub-scoped only (see file header) — it exists so
// the messenger bubble layout, collapsible thinking blocks, tool-use bubble
// grouping, and per-line streaming markdown rendering
// (`260711-feat-ws-dashboard-agent-activity-chat-ui` Phase 2) are demoable
// and testable end-to-end without a live daemon/harness backend. Nothing
// here stands in for real `260620`/`260624` behavior beyond what Phase 1's
// stub already claims.

const STUB_STREAM_TURN_ID = "turn-stream-demo";
const STUB_TOOL_TURN_ID = "turn-tool-demo";

/**
 * Static demo blocks appended after the "Session started" block: a user
 * prompt (right-aligned bubble), a collapsible thinking segment, and a
 * paired tool-call/tool-result (one grouped tool-use bubble via matching
 * `turnId`) — giving every Phase 2 bubble kind at least one concrete example
 * as soon as a stub session starts.
 */
function stubDemoBubbleBlocks(now: string): TranscriptBlock[] {
  return [
    stubTranscriptBlock(
      "",
      "Show me a streamed reply that includes a tool call.",
      now,
      "1",
      { role: "user" },
    ),
    stubTranscriptBlock(
      "",
      "Plan: run a quick lookup, then stream the formatted answer back line by line.",
      now,
      "2",
      { role: "agent", renderKind: "thinking", turnId: STUB_TOOL_TURN_ID },
    ),
    stubTranscriptBlock(
      "Tool call",
      "Called stub.lookup",
      now,
      "3",
      {
        role: "tool",
        turnId: STUB_TOOL_TURN_ID,
        data: { name: "stub.lookup", argumentsBytes: 42 },
      },
    ),
    stubTranscriptBlock(
      "Tool output",
      "stub.lookup completed",
      now,
      "4",
      {
        role: "tool",
        turnId: STUB_TOOL_TURN_ID,
        data: { outcome: "ok", exitCode: 0, outputBytes: 128 },
      },
    ),
  ];
}

const STUB_STREAM_LINES: readonly string[] = [
  "Here is the streamed answer, rendered incrementally line by line:",
  "",
  "- point one lands first",
  "- point two arrives a moment later",
  "",
  "```ts",
  "const streamed = true;",
  "```",
  "",
  "Markdown formatting stays correct at every intermediate length.",
];

export type StubStreamingHandle = {
  stop: () => void;
};

/**
 * Begin a stub-side incremental "streaming" agent turn: grows a single
 * `TranscriptBlock`'s `text` by one more line every tick and invokes
 * `onUpdate` with the updated block each time, so callers can observe
 * genuinely incremental rendering (component tests can also just feed
 * successive partial strings directly through the same block shape without
 * needing the timer).
 */
export function stubBeginStreamingTurn(
  onUpdate: (block: TranscriptBlock) => void,
  options: { intervalMs?: number; cursor?: string } = {},
): StubStreamingHandle {
  const cursor = options.cursor ?? "stream-1";
  const intervalMs = options.intervalMs ?? 200;
  let index = 0;
  let text = "";
  const emit = () => {
    if (index >= STUB_STREAM_LINES.length) {
      clearInterval(timer);
      return;
    }
    text = text.length > 0 ? `${text}\n${STUB_STREAM_LINES[index]}` : STUB_STREAM_LINES[index];
    index += 1;
    onUpdate(
      stubTranscriptBlock("", text, new Date().toISOString(), cursor, {
        role: "agent",
        turnId: STUB_STREAM_TURN_ID,
      }),
    );
  };
  const timer = setInterval(emit, intervalMs);
  return {
    stop: () => clearInterval(timer),
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
        { role: "agent" },
      ),
      ...stubDemoBubbleBlocks(now),
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
