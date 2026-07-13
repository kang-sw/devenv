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
  ActivitySessionForkRequest,
  ActivitySessionForkResponse,
  ActivitySessionSendRequest,
  ActivitySessionSendResponse,
  ActivitySessionSteerRequest,
  ActivitySessionSteerResponse,
  ActivitySessionStartRequest,
  ActivitySessionStartResponse,
} from "./activitySessionApi.js";
import type {
  AgentChatCapabilities,
  AgentChatHarness,
  AgentChatSessionView,
} from "./agentChatSessions.js";
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

// --- Phase 3 per-harness capability table ---------------------------------
// CONTRACT: mirrors the fixture-verified Cross-Harness Feature Matrix in
// `ai-docs/mental-model/ws-dashboard-agent-harness.md` and the Rust
// `AgentClientCapabilities` shape in
// `ws-dashboard/crates/core/src/agent_client_provider.rs#L38-L47`. `rewind`
// stays `false` for every harness, deliberately: no harness's rewind
// primitive is a clean Passthrough/Overlay match for point-based "resume
// from here" (Codex's `thread/rollback` is deprecated-for-removal and
// coarse turn-count-based; Claude's only path is an unofficial Hack;
// OpenCode is unverified) — this is the load-bearing gate keeping "resume
// from here" disabled everywhere (see `agentChatResumeFromHere.tsx`).
function stubCapabilitiesForHarness(harness: AgentChatHarness): AgentChatCapabilities {
  switch (harness) {
    case "codex":
      return {
        compact: true,
        steer: true,
        goal: true,
        rewind: false,
        fork: true,
        skills: true,
      };
    case "claude":
      return {
        compact: false,
        steer: false,
        goal: false,
        rewind: false,
        fork: false,
        skills: true,
      };
    case "opencode":
    default:
      return {
        compact: false,
        steer: false,
        goal: false,
        rewind: false,
        fork: false,
        skills: false,
      };
  }
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
 *
 * `onComplete` (Phase 3,
 * `260711-feat-ws-dashboard-agent-activity-chat-ui`) fires exactly once,
 * when the stream naturally exhausts `STUB_STREAM_LINES` — the concrete,
 * minimal "next tool-call batch boundary" signal this phase's mid-turn
 * queuing state machine dequeues on (see `App.tsx`'s
 * `AgentChatPaneBody`). External cancellation via the returned `stop()`
 * must not also fire `onComplete`: `stop()` only ever clears the interval
 * from the outside, so the natural-completion branch inside `emit` (the
 * only place `onComplete` is invoked) is never reached through it. This
 * single callback deliberately collapses "batch boundary" and "turn
 * completion" into one event, since the stub only ever simulates one
 * linear, non-batched stream per turn — a real Codex adapter's actual
 * mid-turn `turn/steer` batch points are a finer-grained future
 * replacement this callback shape does not need to anticipate further.
 *
 * `turnId` (Phase 3) defaults to the Phase 2 canned demo's shared
 * `STUB_STREAM_TURN_ID` so that call site's grouping behavior is unchanged.
 * A Phase 3 user-triggered simulated turn must pass its own distinct
 * `turnId` (not just a distinct `cursor`) — `groupTranscriptIntoBubbles`
 * merges adjacent same-kind blocks that share a `turnId` into one bubble,
 * so two concurrently-emitting streams sharing `STUB_STREAM_TURN_ID` would
 * wrongly collapse into a single agent-turn bubble despite having distinct
 * cursors.
 */
export function stubBeginStreamingTurn(
  onUpdate: (block: TranscriptBlock) => void,
  options: {
    intervalMs?: number;
    cursor?: string;
    turnId?: string;
    onComplete?: () => void;
  } = {},
): StubStreamingHandle {
  const cursor = options.cursor ?? "stream-1";
  const turnId = options.turnId ?? STUB_STREAM_TURN_ID;
  const intervalMs = options.intervalMs ?? 200;
  let index = 0;
  let text = "";
  const emit = () => {
    if (index >= STUB_STREAM_LINES.length) {
      clearInterval(timer);
      options.onComplete?.();
      return;
    }
    text = text.length > 0 ? `${text}\n${STUB_STREAM_LINES[index]}` : STUB_STREAM_LINES[index];
    index += 1;
    onUpdate(
      stubTranscriptBlock("", text, new Date().toISOString(), cursor, {
        role: "agent",
        turnId,
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
    capabilities: stubCapabilitiesForHarness(request.harness),
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

// CONTRACT: matches by prefix, not exact equality — `ActivityItem.kind` is
// an exact `"agent.<harness>"` string, but `stubForkActivitySession` (below)
// also feeds this the fuller `agent.<harness>:stub-...` `activityId` shape
// minted by `stubActivityId`, and both must resolve to the same harness.
function agentChatHarnessFromSourceKind(kind: string): AgentChatHarness {
  if (kind.startsWith("agent.opencode")) return "opencode";
  if (kind.startsWith("agent.claude")) return "claude";
  return "codex";
}

// --- Phase 3 send/steer/fork stub functions --------------------------------
// CONTRACT: `stubSendActivitySession`/`stubSteerActivitySession` match the
// already-drafted, inert `ActivitySessionSendRequest`/`ActivitySessionSteerRequest`
// wire shapes in `activitySessionApi.ts` byte-for-byte — no new wire type
// drafting needed. Both are trivial `{ accepted: true }` stubs today; a real
// adapter replaces each call site wholesale later, in particular Codex's
// real `turn/steer` for `stubSteerActivitySession`.

export async function stubSendActivitySession(
  request: ActivitySessionSendRequest,
): Promise<ActivitySessionSendResponse> {
  void request;
  return { accepted: true };
}

export async function stubSteerActivitySession(
  request: ActivitySessionSteerRequest,
): Promise<ActivitySessionSteerResponse> {
  void request;
  return { accepted: true };
}

/**
 * "Fork from here" (shipped live, Phase 3). `request` uses the unchanged,
 * `260620`-owned `ActivitySessionForkRequest` shape (`{ workRootId,
 * activityId, serverRoute? }`). `cutBlocks` is a Phase-3-local, non-wire
 * second parameter carrying the transcript slice to seed the new session
 * with — plain call-site plumbing, not a change to
 * `ActivitySessionForkRequest`'s fields (see
 * `ai-docs/tickets/idea/260713-feat-ws-dashboard-activity-session-fork-cursor.md`
 * for the real-adapter follow-up this stands in for). Allocates a new
 * synthetic `activityId` and never mutates the original session.
 */
export async function stubForkActivitySession(
  request: ActivitySessionForkRequest,
  cutBlocks: readonly TranscriptBlock[],
): Promise<ActivitySessionForkResponse & { readonly session: AgentChatSessionView }> {
  const harness = agentChatHarnessFromSourceKind(request.activityId);
  const activityId = stubActivityId(request.workRootId, harness);
  const now = new Date().toISOString();
  const session: AgentChatSessionView = {
    activityId,
    workRootId: request.workRootId,
    serverRoute: request.serverRoute,
    harness,
    title: `${agentChatHarnessLabel[harness]} conversation (forked)`,
    createdAtMs: Date.now(),
    transcript: stubTranscript(request.workRootId, activityId, harness, [
      ...cutBlocks,
      stubTranscriptBlock(
        "Forked from conversation",
        `Forked from conversation ${request.activityId} (stub provider, synthetic transcript).`,
        now,
        `fork-origin-${activityId}`,
        { role: "agent" },
      ),
    ]),
    capabilities: stubCapabilitiesForHarness(harness),
  };
  return { activityId, session };
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
