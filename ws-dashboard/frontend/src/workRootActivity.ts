import { apiErrorDetail } from "./apiError.js";

export type WorkRootActivitySummary = {
  total: number;
  active: number;
  blocked: number;
  failed: number;
  unavailable: number;
};

export type NamedAgentCallActivityView = {
  status: "queued" | "running" | "completed" | "failed" | "cancelled" | string;
  active: boolean;
  terminal: boolean;
  executionId: string | null;
  startedAt: string | null;
  updatedAt: string | null;
  finishedAt: string | null;
  cleanupNeeded: boolean;
  error: string | null;
};

export type ActivitySourceDisplay = {
  kind: "namedAgent" | "exec" | string;
  label: string;
  backend: string | null;
  harness: string | null;
  tier: string | null;
  model: string | null;
};

export type ActivityTranscriptAvailability = {
  status: "available" | "empty" | "unavailable" | "degraded" | string;
  available: boolean;
  cursor: string | null;
};

export type ActivityItem = {
  id: string;
  kind: "namedAgent" | "exec" | string;
  label: string;
  status: string;
  live: boolean;
  attention: boolean;
  startedAt: string | null;
  updatedAt: string | null;
  finishedAt: string | null;
  source: ActivitySourceDisplay;
  transcript: ActivityTranscriptAvailability;
  diagnostics: string[];
  metadata: Record<string, unknown>;
};

export type TranscriptBlock = {
  cursor: string;
  timestamp: string | null;
  renderKind: "markdown" | "text" | "json" | string;
  title: string | null;
  text: string | null;
  data: unknown | null;
  degraded: boolean;
};

export type ActivityTranscript = {
  workRootId: string;
  activityId: string;
  status: "available" | "empty" | "unavailable" | "degraded" | string;
  sourceStatus: "ok" | "missing" | "unsupported" | "degraded" | string;
  live: boolean;
  source: ActivitySourceDisplay;
  blocks: TranscriptBlock[];
  nextCursor: string | null;
  hasMore: boolean;
  diagnostics: string[];
};

export type NamedAgentActivityView = {
  agentId: string;
  name: string | null;
  backend: string | null;
  harness: string | null;
  tier: string | null;
  model: string | null;
  effort: string | null;
  status: "idle" | "running" | "blocked" | "failed" | "unavailable" | string;
  lastCallAt: string | null;
  sessionPresent: boolean;
  currentCall: NamedAgentCallActivityView | null;
  detailHints: string[];
  diagnostics: string[];
};

export type WorkRootActivityView = {
  // CONTRACT: This mirrors the daemon projection API. It intentionally omits
  // host paths, cache paths, process ids, stream paths, and session ids.
  workRootId: string;
  status: "ok" | "unavailable" | "degraded" | string;
  updateMode: "snapshot" | "replace" | "append" | string;
  feedCursor: string | null;
  selectedItemId: string | null;
  summary: WorkRootActivitySummary;
  items: ActivityItem[];
  // Compatibility projection for the existing read-only named-agent pane.
  agents: NamedAgentActivityView[];
};


export type ActivityConsoleUpdateMode = "watch" | "pollFallback" | "snapshot" | string;

export type ActivityConsoleEvent =
  | { type: "itemUpserted"; cursor: string; item: ActivityItem }
  | { type: "itemRemoved"; cursor: string; activityId: string }
  | {
      type: "transcriptUpdated";
      cursor: string;
      activityId: string;
      transcriptCursor: string | null;
    }
  | {
      type: "snapshotInvalidated";
      cursor: string;
      reason: "overflow" | "watchReset" | "fallback" | string;
    }
  | { type: "modeChanged"; cursor: string; updateMode: ActivityConsoleUpdateMode }
  | { type: "heartbeat"; cursor: string };

export type ActivityConsoleEventApplication = {
  readonly view: WorkRootActivityView;
  readonly refetchSnapshot: boolean;
  readonly transcriptActivityId: string | null;
  readonly updateMode: ActivityConsoleUpdateMode | null;
};

export type ActivityConsoleStreamRequest = {
  readonly workRootId: string;
  readonly requestId: number;
};

export function workRootActivityEventsEndpoint(
  workRootId: string,
  options: { readonly after?: string | null } = {},
) {
  const path = `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/activity/events`;
  if (!options.after) {
    return path;
  }
  const params = new URLSearchParams();
  params.set("after", options.after);
  return `${path}?${params.toString()}`;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string";
}

export function parseActivityConsoleEvent(value: unknown): ActivityConsoleEvent | null {
  if (!isObject(value) || !isString(value.type) || !isString(value.cursor)) {
    return null;
  }
  switch (value.type) {
    case "itemUpserted":
      return isObject(value.item) && isString(value.item.id)
        ? ({ type: "itemUpserted", cursor: value.cursor, item: value.item as ActivityItem })
        : null;
    case "itemRemoved":
      return isString(value.activityId)
        ? { type: "itemRemoved", cursor: value.cursor, activityId: value.activityId }
        : null;
    case "transcriptUpdated":
      return isString(value.activityId) && isNullableString(value.transcriptCursor)
        ? {
            type: "transcriptUpdated",
            cursor: value.cursor,
            activityId: value.activityId,
            transcriptCursor: value.transcriptCursor,
          }
        : null;
    case "snapshotInvalidated":
      return isString(value.reason)
        ? { type: "snapshotInvalidated", cursor: value.cursor, reason: value.reason }
        : null;
    case "modeChanged":
      return isString(value.updateMode)
        ? { type: "modeChanged", cursor: value.cursor, updateMode: value.updateMode }
        : null;
    case "heartbeat":
      return { type: "heartbeat", cursor: value.cursor };
    default:
      return null;
  }
}

function summarizeActivityItems(items: readonly ActivityItem[]): WorkRootActivitySummary {
  return items.reduce<WorkRootActivitySummary>(
    (summary, item) => {
      summary.total += 1;
      if (item.live) summary.active += 1;
      if (item.attention) summary.blocked += 1;
      const status = item.status.toLowerCase();
      if (status.includes("fail") || status === "error") summary.failed += 1;
      if (status === "unavailable") summary.unavailable += 1;
      return summary;
    },
    { total: 0, active: 0, blocked: 0, failed: 0, unavailable: 0 },
  );
}

function withEventCursor(view: WorkRootActivityView, cursor: string): WorkRootActivityView {
  return { ...view, feedCursor: cursor };
}

export function applyActivityConsoleEvent(
  current: WorkRootActivityView,
  event: ActivityConsoleEvent,
): ActivityConsoleEventApplication {
  let view = withEventCursor(current, event.cursor);
  let refetchSnapshot = false;
  let transcriptActivityId: string | null = null;
  let updateMode: ActivityConsoleUpdateMode | null = null;

  if (event.type === "itemUpserted") {
    const itemsById = new Map(current.items.map((item) => [item.id, item] as const));
    itemsById.set(event.item.id, event.item);
    const items = orderActivityItems(Array.from(itemsById.values()));
    view = {
      ...view,
      items,
      summary: summarizeActivityItems(items),
      selectedItemId: preserveActivitySelection(items, current.selectedItemId),
    };
  } else if (event.type === "itemRemoved") {
    const items = current.items.filter((item) => item.id !== event.activityId);
    view = {
      ...view,
      items,
      summary: summarizeActivityItems(items),
      selectedItemId: preserveActivitySelection(items, current.selectedItemId),
    };
  } else if (event.type === "transcriptUpdated") {
    transcriptActivityId = event.activityId;
    const items = current.items.map((item) =>
      item.id === event.activityId
        ? {
            ...item,
            transcript: {
              ...item.transcript,
              cursor: event.transcriptCursor,
            },
          }
        : item,
    );
    view = { ...view, items };
  } else if (event.type === "snapshotInvalidated") {
    refetchSnapshot = true;
  } else if (event.type === "modeChanged") {
    updateMode = event.updateMode;
    view = { ...view, updateMode: event.updateMode };
  }

  return { view, refetchSnapshot, transcriptActivityId, updateMode };
}

export function shouldApplyActivityStreamRequest(
  expected: ActivityConsoleStreamRequest,
  current: { readonly workRootId: string | null; readonly requestId: number },
): boolean {
  return expected.workRootId === current.workRootId && expected.requestId === current.requestId;
}

export type WorkRootActivityFetchOptions = {
  readonly recentLimit?: number;
};

export type ActivityTranscriptFetchOptions = {
  readonly cursor?: string;
  readonly limit?: number;
};

export function workRootActivityEndpoint(
  workRootId: string,
  options: WorkRootActivityFetchOptions = {},
) {
  const path = `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/activity`;
  if (options.recentLimit === undefined) {
    return path;
  }
  const params = new URLSearchParams();
  params.set("recentLimit", String(options.recentLimit));
  return `${path}?${params.toString()}`;
}

export function workRootActivityTranscriptEndpoint(
  workRootId: string,
  activityId: string,
  options: ActivityTranscriptFetchOptions = {},
) {
  const path = `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/activity/items/${encodeURIComponent(activityId)}/transcript`;
  const params = new URLSearchParams();
  if (options.cursor !== undefined) {
    params.set("cursor", options.cursor);
  }
  if (options.limit !== undefined) {
    params.set("limit", String(options.limit));
  }
  const query = params.toString();
  return query ? `${path}?${query}` : path;
}

export async function fetchWorkRootActivity(
  workRootId: string,
  options: WorkRootActivityFetchOptions = {},
): Promise<WorkRootActivityView> {
  const response = await fetch(workRootActivityEndpoint(workRootId, options), {
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as WorkRootActivityView;
}

export async function fetchWorkRootActivityTranscript(
  workRootId: string,
  activityId: string,
  options: ActivityTranscriptFetchOptions = {},
): Promise<ActivityTranscript> {
  const response = await fetch(
    workRootActivityTranscriptEndpoint(workRootId, activityId, options),
    {
      headers: { Accept: "application/json" },
    },
  );

  if (!response.ok) {
    throw new Error(await apiErrorDetail(response));
  }

  return (await response.json()) as ActivityTranscript;
}

export function mergeWorkRootActivityViews(
  current: WorkRootActivityView,
  update: WorkRootActivityView,
): WorkRootActivityView {
  if (current.workRootId !== update.workRootId) {
    return update;
  }

  const agentsById = new Map(
    current.agents.map((agent) => [agent.agentId, agent] as const),
  );
  for (const agent of update.agents) {
    agentsById.set(agent.agentId, agent);
  }
  const agents = Array.from(agentsById.values()).sort((left, right) =>
    left.agentId.localeCompare(right.agentId),
  );
  const summary = summarizeWorkRootActivityAgents(agents);
  const degraded =
    current.status === "degraded" ||
    update.status === "degraded" ||
    agents.some((agent) => agent.diagnostics.length > 0);
  return {
    ...current,
    status:
      update.status === "unavailable" ? "unavailable" : degraded ? "degraded" : "ok",
    updateMode: update.updateMode,
    feedCursor: update.feedCursor,
    selectedItemId: update.selectedItemId,
    summary,
    items: update.items.length > 0 ? update.items : current.items,
    agents,
  };
}

export type ActivityAcknowledgements = Record<string, string>;

export type TranscriptBlockRenderMode =
  | "expanded"
  | "compact"
  | "terminal";

export type TranscriptBlockView = {
  mode: TranscriptBlockRenderMode;
  tone: "normal" | "status" | "tool" | "error" | "terminal";
  summary: string;
  detail: string | null;
};

export function activityItemRevisionToken(item: ActivityItem): string {
  return (
    item.updatedAt ??
    item.transcript.cursor ??
    item.finishedAt ??
    item.startedAt ??
    `${item.status}:${item.live ? "live" : "idle"}:${item.attention ? "attention" : "normal"}`
  );
}

function activitySortKey(item: ActivityItem): string {
  return item.updatedAt ?? item.finishedAt ?? item.startedAt ?? "";
}

export function orderActivityItems(
  items: readonly ActivityItem[],
): ActivityItem[] {
  return [...items].sort((left, right) => {
    const leftPriority = left.live || left.attention ? 1 : 0;
    const rightPriority = right.live || right.attention ? 1 : 0;
    if (leftPriority !== rightPriority) {
      return rightPriority - leftPriority;
    }
    const timeCompare = activitySortKey(right).localeCompare(activitySortKey(left));
    if (timeCompare !== 0) {
      return timeCompare;
    }
    return left.id.localeCompare(right.id);
  });
}

export function defaultActivitySelection(
  items: readonly ActivityItem[],
): string | null {
  return orderActivityItems(items)[0]?.id ?? null;
}

export function preserveActivitySelection(
  items: readonly ActivityItem[],
  selectedItemId: string | null,
): string | null {
  if (selectedItemId && items.some((item) => item.id === selectedItemId)) {
    return selectedItemId;
  }
  return defaultActivitySelection(items);
}

export function initializeActivityDirtyItems(
  items: readonly ActivityItem[],
  acknowledgements: ActivityAcknowledgements,
  seenRevisions: ActivityAcknowledgements = {},
): Set<string> {
  const dirty = new Set<string>();
  for (const item of items) {
    const token = activityItemRevisionToken(item);
    const acknowledgedToken = acknowledgements[item.id];
    const seenToken = seenRevisions[item.id];
    if (
      (!acknowledgedToken && (item.live || item.attention)) ||
      (acknowledgedToken !== undefined && acknowledgedToken !== token) ||
      (seenToken !== undefined && seenToken !== token)
    ) {
      dirty.add(item.id);
    }
  }
  return dirty;
}

export function acknowledgeActivityItem(
  acknowledgements: ActivityAcknowledgements,
  item: ActivityItem,
): ActivityAcknowledgements {
  return {
    ...acknowledgements,
    [item.id]: activityItemRevisionToken(item),
  };
}

export function shouldApplyActivityTranscriptResponse(
  expected: { workRootId: string; activityId: string; requestId: number },
  response: { workRootId: string; activityId: string },
  current: { workRootId: string; activityId: string | null; requestId: number },
): boolean {
  return shouldApplyActivityTranscriptRequest(expected, current, response);
}

export function shouldApplyActivityTranscriptRequest(
  expected: { workRootId: string; activityId: string; requestId: number },
  current: { workRootId: string; activityId: string | null; requestId: number },
  response?: { workRootId: string; activityId: string },
): boolean {
  return (
    expected.requestId === current.requestId &&
    expected.workRootId === current.workRootId &&
    expected.activityId === current.activityId &&
    (response === undefined ||
      (response.workRootId === current.workRootId &&
        response.activityId === current.activityId))
  );
}

export type ActivityTranscriptScrollMetrics = {
  readonly scrollTop: number;
  readonly clientHeight: number;
  readonly scrollHeight: number;
};

export function activityTranscriptDistanceFromTail(
  metrics: ActivityTranscriptScrollMetrics,
): number {
  return Math.max(0, metrics.scrollHeight - (metrics.scrollTop + metrics.clientHeight));
}

export function isActivityTranscriptAtTail(
  metrics: ActivityTranscriptScrollMetrics,
  thresholdPx = 8,
): boolean {
  return activityTranscriptDistanceFromTail(metrics) <= thresholdPx;
}

export function shouldFollowActivityTranscriptTail(
  metrics: ActivityTranscriptScrollMetrics,
  thresholdPx = 8,
): boolean {
  return isActivityTranscriptAtTail(metrics, thresholdPx);
}

export function shouldLoadMoreActivityTranscript(
  metrics: ActivityTranscriptScrollMetrics,
  hasMore: boolean,
  loading: boolean,
  thresholdPx = 48,
): boolean {
  if (!hasMore || loading) {
    return false;
  }
  return activityTranscriptDistanceFromTail(metrics) <= thresholdPx;
}

function transcriptBlockText(block: TranscriptBlock): string {
  if (typeof block.text === "string" && block.text.length > 0) {
    return block.text;
  }
  if (block.data !== null && block.data !== undefined) {
    try {
      return JSON.stringify(block.data);
    } catch {
      return String(block.data);
    }
  }
  return "";
}

export function transcriptBlockView(
  block: TranscriptBlock,
  sourceKind: string,
): TranscriptBlockView {
  const key = `${block.renderKind} ${block.title ?? ""}`.toLowerCase();
  const text = transcriptBlockText(block);
  const summary = block.title ?? text.split(/\r?\n/, 1)[0] ?? block.renderKind;
  if (sourceKind === "exec" || key.includes("terminal") || block.renderKind === "ansi") {
    return { mode: "terminal", tone: "terminal", summary, detail: text || null };
  }
  if (block.degraded || key.includes("error") || block.renderKind === "error") {
    return { mode: "compact", tone: "error", summary, detail: text || null };
  }
  if (
    key.includes("tool") ||
    key.includes("mcp") ||
    key.includes("command") ||
    key.includes("status") ||
    block.renderKind === "json"
  ) {
    return { mode: "compact", tone: key.includes("tool") || key.includes("mcp") ? "tool" : "status", summary, detail: text || null };
  }
  return { mode: "expanded", tone: "normal", summary, detail: text || null };
}

function summarizeWorkRootActivityAgents(
  agents: readonly NamedAgentActivityView[],
): WorkRootActivitySummary {
  const summary: WorkRootActivitySummary = {
    total: agents.length,
    active: 0,
    blocked: 0,
    failed: 0,
    unavailable: 0,
  };
  for (const agent of agents) {
    if (agent.status === "running") summary.active += 1;
    if (agent.status === "blocked") summary.blocked += 1;
    if (agent.status === "failed") summary.failed += 1;
    if (agent.status === "unavailable") summary.unavailable += 1;
  }
  return summary;
}

// CONTRACT: The top-bar activity badge is a compact summary/entrypoint only.
// It mirrors the Phase 1 daemon projection counts and stays bounded: no row
// diagnostics, host paths, or long error text reach the toolbar.

export type WorkRootActivityBadgeTone =
  | "idle"
  | "active"
  | "attention"
  | "loading"
  | "error";

export type WorkRootActivityBadgeView = {
  // `tone` drives the badge dot color; `label` is the always-shown primary
  // text and `summary` is secondary text that may be truncated/hidden under
  // constrained toolbar width. `title` is the bounded full tooltip text.
  tone: WorkRootActivityBadgeTone;
  label: string;
  summary: string;
  title: string;
};

export type WorkRootActivityBadgeInput =
  | { phase: "loading" }
  | { phase: "error" }
  | { phase: "ready"; view: WorkRootActivityView };

const ACTIVITY_BADGE_TITLE_LIMIT = 120;

function boundedActivityTitle(text: string): string {
  return text.length <= ACTIVITY_BADGE_TITLE_LIMIT
    ? text
    : `${text.slice(0, ACTIVITY_BADGE_TITLE_LIMIT - 1)}…`;
}

/**
 * Project a selected workRoot's activity fetch state into the compact top-bar
 * badge model. Loading, error, and unavailable states all collapse to bounded
 * short badge states rather than surfacing diagnostics in the toolbar.
 */
export function workRootActivityBadge(
  input: WorkRootActivityBadgeInput,
): WorkRootActivityBadgeView {
  if (input.phase === "loading") {
    return {
      tone: "loading",
      label: "agents",
      summary: "loading",
      title: "Agent activity loading",
    };
  }
  if (input.phase === "error") {
    return {
      tone: "error",
      label: "agents",
      summary: "unavailable",
      title: "Agent activity unavailable",
    };
  }

  const { status, summary } = input.view;
  if (status === "unavailable") {
    return {
      tone: "error",
      label: "agents",
      summary: "unavailable",
      title: "Agent activity unavailable",
    };
  }

  if (summary.total === 0) {
    return {
      tone: "idle",
      label: "no agents",
      summary: "",
      title: "No named agents",
    };
  }

  const parts: string[] = [];
  if (summary.active > 0) {
    parts.push(`${summary.active} active`);
  }
  if (summary.blocked > 0) {
    parts.push(`${summary.blocked} blocked`);
  }
  if (summary.failed > 0) {
    parts.push(`${summary.failed} failed`);
  }
  if (summary.unavailable > 0) {
    parts.push(`${summary.unavailable} unavailable`);
  }

  const degraded = status === "degraded";
  const needsAttention =
    degraded ||
    summary.blocked > 0 ||
    summary.failed > 0 ||
    summary.unavailable > 0;
  const tone: WorkRootActivityBadgeTone = needsAttention
    ? "attention"
    : summary.active > 0
      ? "active"
      : "idle";

  const label = `${summary.total} ${summary.total === 1 ? "agent" : "agents"}`;
  const secondary = parts.length > 0 ? parts.join(" · ") : "idle";
  const title = boundedActivityTitle(
    `${label} — ${parts.length > 0 ? parts.join(", ") : "idle"}${
      degraded ? " (degraded)" : ""
    }`,
  );

  return { tone, label, summary: secondary, title };
}
