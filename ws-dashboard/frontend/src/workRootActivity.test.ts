import {
  acknowledgeActivityItem,
  activityItemRevisionToken,
  applyActivityConsoleEvent,
  defaultActivitySelection,
  fetchWorkRootActivity,
  fetchWorkRootActivityTranscript,
  initializeActivityDirtyItems,
  mergeWorkRootActivityViews,
  orderActivityItems,
  parseActivityConsoleEvent,
  preserveActivitySelection,
  shouldApplyActivityStreamRequest,
  activityTranscriptDistanceFromTail,
  isActivityTranscriptAtTail,
  shouldApplyActivityTranscriptResponse,
  shouldApplyActivityTranscriptRequest,
  shouldFollowActivityTranscriptTail,
  shouldLoadMoreActivityTranscript,
  transcriptBlockView,
  workRootActivityBadge,
  workRootActivityEndpoint,
  workRootActivityEventsEndpoint,
  workRootActivityTranscriptEndpoint,
  type ActivityItem,
  type NamedAgentActivityView,
  type WorkRootActivitySummary,
  type ActivityTranscript,
  type TranscriptBlock,
  type WorkRootActivityView,
} from "./workRootActivity.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

async function assertRejects(action: () => Promise<unknown>, pattern: RegExp, label: string) {
  try {
    await action();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (!pattern.test(message)) {
      throw new Error(`${label}: error ${message} did not match ${pattern}`);
    }
    return;
  }

  throw new Error(`${label}: expected rejection`);
}

assertEqual(
  workRootActivityEndpoint("root/local test"),
  "/api/dashboard/work-roots/root%2Flocal%20test/activity",
  "activity endpoint addresses an encoded opaque workRoot id",
);
assertEqual(
  workRootActivityEndpoint("root-local-abc", { recentLimit: 30 }),
  "/api/dashboard/work-roots/root-local-abc/activity?recentLimit=30",
  "activity endpoint encodes a recent-limit refresh query",
);
assertEqual(
  workRootActivityTranscriptEndpoint("root/local test", "agent:reviewer", {
    cursor: "2",
    limit: 10,
  }),
  "/api/dashboard/work-roots/root%2Flocal%20test/activity/items/agent%3Areviewer/transcript?cursor=2&limit=10",
  "transcript endpoint addresses encoded opaque ids and bounded query options",
);

assertEqual(
  workRootActivityEventsEndpoint("root/local test", { after: "cursor:1/2" }),
  "/api/dashboard/work-roots/root%2Flocal%20test/activity/events?after=cursor%3A1%2F2",
  "activity event endpoint addresses an encoded opaque workRoot id and after cursor",
);
assertEqual(
  workRootActivityEventsEndpoint("root-local-abc"),
  "/api/dashboard/work-roots/root-local-abc/activity/events",
  "activity event endpoint omits the after query when no cursor exists",
);

const originalFetch = globalThis.fetch;
try {
  globalThis.fetch = (async (input, init) => {
    assertEqual(
      String(input),
      "/api/dashboard/work-roots/root-local-abc/activity?recentLimit=30",
      "fetch helper uses the recent-limit WorkRoot Activity endpoint",
    );
    assertEqual(
      (init?.headers as Record<string, string>).Accept,
      "application/json",
      "fetch helper requests JSON",
    );
    const view: WorkRootActivityView = {
      workRootId: "root-local-abc",
      status: "ok",
      updateMode: "snapshot",
      feedCursor: "snapshot:0:",
      selectedItemId: null,
      summary: { total: 0, active: 0, blocked: 0, failed: 0, unavailable: 0 },
      items: [],
      agents: [],
    };
    return new Response(JSON.stringify(view), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  }) as typeof fetch;

  const activity = await fetchWorkRootActivity("root-local-abc", {
    recentLimit: 30,
  });
  assertEqual(activity.workRootId, "root-local-abc", "fetch helper returns the daemon view");
  assertEqual(activity.summary.total, 0, "Phase 1 no-agent summary can be consumed");

  globalThis.fetch = (async (input, init) => {
    assertEqual(
      String(input),
      "/api/dashboard/work-roots/root-local-abc/activity/items/agent%3Areviewer/transcript?cursor=1&limit=1",
      "transcript fetch helper uses the transcript endpoint",
    );
    assertEqual(
      (init?.headers as Record<string, string>).Accept,
      "application/json",
      "transcript fetch helper requests JSON",
    );
    const transcript: ActivityTranscript = {
      workRootId: "root-local-abc",
      activityId: "agent:reviewer",
      status: "available",
      sourceStatus: "ok",
      live: false,
      source: {
        kind: "namedAgent",
        label: "reviewer",
        backend: "codex",
        harness: null,
        tier: null,
        model: null,
      },
      blocks: [
        {
          cursor: "1",
          timestamp: null,
          renderKind: "markdown",
          title: null,
          text: "done without host paths",
          data: null,
          degraded: false,
        },
      ],
      nextCursor: "2",
      hasMore: false,
      diagnostics: [],
    };
    return new Response(JSON.stringify(transcript), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  }) as typeof fetch;
  const transcript = await fetchWorkRootActivityTranscript(
    "root-local-abc",
    "agent:reviewer",
    { cursor: "1", limit: 1 },
  );
  assertEqual(transcript.blocks[0]?.renderKind, "markdown", "transcript shape is consumed");
  assertEqual(
    JSON.stringify(transcript).includes("/Users/"),
    false,
    "transcript helper shape does not require host paths",
  );

  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "unknown workRoot" }), {
      status: 404,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootActivity("root-local-missing"),
    /unknown workRoot/,
    "fetch helper surfaces bounded backend JSON errors",
  );

  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "unknown activity" }), {
      status: 404,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootActivityTranscript("root-local-abc", "agent:missing"),
    /unknown activity/,
    "transcript fetch helper surfaces bounded backend JSON errors",
  );
} finally {
  globalThis.fetch = originalFetch;
}

// --- Top-bar activity badge formatting -----------------------------------

function activitySummary(
  partial: Partial<WorkRootActivitySummary> = {},
): WorkRootActivitySummary {
  return { total: 0, active: 0, blocked: 0, failed: 0, unavailable: 0, ...partial };
}

function activityView(
  partial: Partial<Omit<WorkRootActivityView, "summary">> & {
    summary?: Partial<WorkRootActivitySummary>;
  } = {},
): WorkRootActivityView {
  return {
    workRootId: partial.workRootId ?? "root-local-abc",
    status: partial.status ?? "ok",
    updateMode: partial.updateMode ?? "snapshot",
    feedCursor: partial.feedCursor ?? "snapshot:0:",
    selectedItemId: partial.selectedItemId ?? null,
    summary: activitySummary(partial.summary),
    items: partial.items ?? [],
    agents: partial.agents ?? [],
  };
}

function activityItem(partial: Partial<ActivityItem> & { id: string }): ActivityItem {
  return {
    id: partial.id,
    kind: partial.kind ?? "namedAgent",
    label: partial.label ?? partial.id,
    status: partial.status ?? "idle",
    live: partial.live ?? false,
    attention: partial.attention ?? false,
    startedAt: partial.startedAt ?? null,
    updatedAt: partial.updatedAt ?? null,
    finishedAt: partial.finishedAt ?? null,
    source: partial.source ?? {
      kind: "namedAgent",
      label: partial.label ?? partial.id,
      backend: null,
      harness: null,
      tier: null,
      model: null,
    },
    transcript: partial.transcript ?? {
      status: "empty",
      available: false,
      cursor: null,
    },
    diagnostics: partial.diagnostics ?? [],
    metadata: partial.metadata ?? {},
  };
}


const parsedUpsert = parseActivityConsoleEvent({
  type: "itemUpserted",
  cursor: "event:1",
  item: activityItem({ id: "agent:streamed", updatedAt: "2026-05-21T12:05:00Z" }),
});
assertEqual(parsedUpsert?.type, "itemUpserted", "itemUpserted events parse with source-neutral payloads");
assertEqual(
  parseActivityConsoleEvent({ type: "itemRemoved", cursor: "event:2", path: "/Users/nope" }),
  null,
  "activity events reject payloads without source-neutral activity ids",
);
assertEqual(
  parseActivityConsoleEvent({
    type: "modeChanged",
    cursor: "event:mode",
    updateMode: "pollFallback",
  })?.type,
  "modeChanged",
  "modeChanged pollFallback events parse",
);

const eventBase = activityView({
  selectedItemId: "agent:keep",
  items: [
    activityItem({ id: "agent:keep", label: "keep", updatedAt: "2026-05-21T12:00:00Z" }),
    activityItem({ id: "agent:remove", label: "remove", updatedAt: "2026-05-21T11:00:00Z" }),
  ],
});
const upsertedEvent = applyActivityConsoleEvent(eventBase, {
  type: "itemUpserted",
  cursor: "event:upsert",
  item: activityItem({
    id: "agent:new",
    label: "new",
    live: true,
    updatedAt: "2026-05-21T12:10:00Z",
  }),
});
assertDeepEqual(
  upsertedEvent.view.items.map((item) => item.id),
  ["agent:new", "agent:keep", "agent:remove"],
  "itemUpserted merges the item into the ordered feed",
);
assertEqual(upsertedEvent.view.selectedItemId, "agent:keep", "itemUpserted preserves existing selection");
assertEqual(upsertedEvent.view.summary.total, 3, "itemUpserted recomputes source-neutral summary totals");
assertEqual(upsertedEvent.refetchSnapshot, false, "itemUpserted does not request snapshot refetch");

const removedUnselectedEvent = applyActivityConsoleEvent(eventBase, {
  type: "itemRemoved",
  cursor: "event:remove-unselected",
  activityId: "agent:remove",
});
assertDeepEqual(
  removedUnselectedEvent.view.items.map((item) => item.id),
  ["agent:keep"],
  "itemRemoved removes unselected feed items",
);
assertEqual(
  removedUnselectedEvent.view.selectedItemId,
  "agent:keep",
  "itemRemoved preserves selection when selected item still exists",
);
const removedSelectedEvent = applyActivityConsoleEvent(eventBase, {
  type: "itemRemoved",
  cursor: "event:remove-selected",
  activityId: "agent:keep",
});
assertEqual(
  removedSelectedEvent.view.selectedItemId,
  "agent:remove",
  "itemRemoved reconciles selection when selected item disappears",
);

const invalidatedEvent = applyActivityConsoleEvent(eventBase, {
  type: "snapshotInvalidated",
  cursor: "event:invalidated",
  reason: "watchReset",
});
assertEqual(invalidatedEvent.refetchSnapshot, true, "snapshotInvalidated requests a read-model refetch");
assertEqual(invalidatedEvent.view.feedCursor, "event:invalidated", "snapshotInvalidated advances the event cursor");

const transcriptEvent = applyActivityConsoleEvent(eventBase, {
  type: "transcriptUpdated",
  cursor: "event:transcript",
  activityId: "agent:keep",
  transcriptCursor: "transcript:2",
});
assertEqual(
  transcriptEvent.transcriptActivityId,
  "agent:keep",
  "transcriptUpdated exposes the activity id for selected-transcript refresh decisions",
);
assertEqual(
  transcriptEvent.view.items.find((item) => item.id === "agent:keep")?.transcript.cursor,
  "transcript:2",
  "transcriptUpdated backfills the item transcript cursor without rebuilding transcript blocks",
);

const pollFallbackEvent = applyActivityConsoleEvent(eventBase, {
  type: "modeChanged",
  cursor: "event:poll",
  updateMode: "pollFallback",
});
assertEqual(pollFallbackEvent.updateMode, "pollFallback", "modeChanged pollFallback activates fallback decisions");
assertEqual(pollFallbackEvent.view.updateMode, "pollFallback", "modeChanged records the daemon update mode");
const watchEvent = applyActivityConsoleEvent(eventBase, {
  type: "modeChanged",
  cursor: "event:watch",
  updateMode: "watch",
});
assertEqual(watchEvent.updateMode, "watch", "modeChanged watch suppresses fallback polling decisions");
const locallyAcknowledged = acknowledgeActivityItem({}, eventBase.items[0]!);
const dirtyAfterStreamMerge = initializeActivityDirtyItems(
  upsertedEvent.view.items,
  locallyAcknowledged,
  { "agent:keep": activityItemRevisionToken(eventBase.items[0]!) },
);
assertDeepEqual(
  Array.from(dirtyAfterStreamMerge),
  ["agent:new"],
  "local dirty acknowledgements survive stream feed merges",
);
const selectedRevisionEvent = applyActivityConsoleEvent(eventBase, {
  type: "itemUpserted",
  cursor: "event:selected-revision",
  item: activityItem({
    id: "agent:keep",
    label: "keep",
    updatedAt: "2026-05-21T12:30:00Z",
  }),
});
assertDeepEqual(
  Array.from(
    initializeActivityDirtyItems(selectedRevisionEvent.view.items, locallyAcknowledged, {
      "agent:keep": activityItemRevisionToken(eventBase.items[0]!),
    }),
  ),
  ["agent:keep"],
  "a streamed revision change for the selected item remains dirty until explicit acknowledgement",
);

assertEqual(
  shouldApplyActivityStreamRequest(
    { workRootId: "root-a", requestId: 1 },
    { workRootId: "root-a", requestId: 1 },
  ),
  true,
  "matching activity stream request may update state",
);
assertEqual(
  shouldApplyActivityStreamRequest(
    { workRootId: "root-a", requestId: 1 },
    { workRootId: "root-b", requestId: 2 },
  ),
  false,
  "stale workRoot stream completions are ignored after root switch",
);

assertEqual(
  activityTranscriptDistanceFromTail({ scrollTop: 240, clientHeight: 160, scrollHeight: 400 }),
  0,
  "transcript scroll metrics report zero distance at the tail",
);
assertEqual(
  isActivityTranscriptAtTail({ scrollTop: 230, clientHeight: 160, scrollHeight: 400 }, 12),
  true,
  "transcript follow policy treats near-tail scroll as following",
);
assertEqual(
  shouldFollowActivityTranscriptTail({ scrollTop: 120, clientHeight: 160, scrollHeight: 400 }, 12),
  false,
  "transcript follow policy pauses when the user scrolls away from the tail",
);
assertEqual(
  shouldFollowActivityTranscriptTail({ scrollTop: 240, clientHeight: 160, scrollHeight: 400 }, 12),
  true,
  "transcript follow policy resumes when the user returns to the tail",
);


function activityAgent(
  partial: Partial<NamedAgentActivityView> & { agentId: string },
): NamedAgentActivityView {
  return {
    agentId: partial.agentId,
    name: partial.name ?? partial.agentId,
    backend: partial.backend ?? null,
    harness: partial.harness ?? null,
    tier: partial.tier ?? null,
    model: partial.model ?? null,
    effort: partial.effort ?? null,
    status: partial.status ?? "idle",
    lastCallAt: partial.lastCallAt ?? null,
    sessionPresent: partial.sessionPresent ?? false,
    currentCall: partial.currentCall ?? null,
    detailHints: partial.detailHints ?? [],
    diagnostics: partial.diagnostics ?? [],
  };
}

const mergedActivity = mergeWorkRootActivityViews(
  activityView({
    updateMode: "snapshot",
    feedCursor: "snapshot:old",
    selectedItemId: "agent:agent-a",
    summary: { total: 2, active: 1 },
    items: [activityItem({ id: "agent:agent-a", status: "running", live: true })],
    agents: [
      activityAgent({ agentId: "agent-a", status: "running" }),
      activityAgent({ agentId: "agent-b", status: "idle" }),
    ],
  }),
  activityView({
    updateMode: "snapshot",
    feedCursor: "snapshot:new",
    selectedItemId: "agent:agent-b",
    summary: { total: 2, blocked: 1, unavailable: 1 },
    items: [activityItem({ id: "agent:agent-b", status: "blocked", attention: true })],
    agents: [
      activityAgent({ agentId: "agent-b", status: "blocked" }),
      activityAgent({
        agentId: "agent-c",
        status: "unavailable",
        diagnostics: ["agent status unavailable"],
      }),
    ],
  }),
);
assertDeepEqual(
  mergedActivity.agents.map((agent) => [agent.agentId, agent.status]),
  [
    ["agent-a", "running"],
    ["agent-b", "blocked"],
    ["agent-c", "unavailable"],
  ],
  "recent activity refresh merges updated and new agents by id",
);
assertDeepEqual(
  mergedActivity.summary,
  { total: 3, active: 1, blocked: 1, failed: 0, unavailable: 1 },
  "recent activity refresh recomputes the merged summary",
);
assertEqual(
  mergedActivity.status,
  "degraded",
  "recent activity refresh preserves degraded status from merged diagnostics",
);
assertDeepEqual(
  mergedActivity.items.map((item) => item.id),
  ["agent:agent-b"],
  "recent activity refresh carries the source-neutral feed items from the latest update",
);
assertEqual(
  mergedActivity.feedCursor,
  "snapshot:new",
  "recent activity refresh carries the feed cursor from the latest update",
);
assertEqual(
  mergedActivity.selectedItemId,
  "agent:agent-b",
  "recent activity refresh carries the selected item hint from the latest update",
);
assertEqual(
  mergedActivity.updateMode,
  "snapshot",
  "recent activity refresh carries the update mode from the latest update",
);

const loadingBadge = workRootActivityBadge({ phase: "loading" });
assertEqual(loadingBadge.tone, "loading", "loading badge uses the loading tone");
assertEqual(loadingBadge.label, "agents", "loading badge keeps a compact label");
assertEqual(loadingBadge.summary, "loading", "loading badge marks the loading state");

const errorBadge = workRootActivityBadge({ phase: "error" });
assertEqual(errorBadge.tone, "error", "error badge uses the error tone");
assertEqual(
  errorBadge.summary,
  "unavailable",
  "error badge collapses to a bounded unavailable state",
);

const unavailableBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ status: "unavailable" }),
});
assertEqual(
  unavailableBadge.tone,
  "error",
  "unavailable projection status renders the error tone",
);
assertEqual(
  unavailableBadge.summary,
  "unavailable",
  "unavailable projection status renders a bounded summary",
);

const emptyBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ summary: { total: 0 } }),
});
assertEqual(emptyBadge.tone, "idle", "no agents renders the idle tone");
assertEqual(emptyBadge.label, "no agents", "no agents renders a compact label");
assertEqual(emptyBadge.summary, "", "no agents renders no secondary text");

const singleIdleBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ summary: { total: 1 } }),
});
assertEqual(
  singleIdleBadge.label,
  "1 agent",
  "a single agent uses the singular agent label",
);
assertEqual(singleIdleBadge.tone, "idle", "all-idle agents render the idle tone");
assertEqual(singleIdleBadge.summary, "idle", "all-idle agents render an idle summary");

const activeBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ summary: { total: 3, active: 2 } }),
});
assertEqual(activeBadge.label, "3 agents", "multiple agents use the plural label");
assertEqual(activeBadge.tone, "active", "active agents render the active tone");
assertEqual(activeBadge.summary, "2 active", "active agents summarize the active count");

const failedBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ summary: { total: 4, active: 1, failed: 1 } }),
});
assertEqual(
  failedBadge.tone,
  "attention",
  "failed agents render the attention tone",
);
assertEqual(
  failedBadge.summary,
  "1 active · 1 failed",
  "failed agents summarize active and failed counts compactly",
);

const blockedBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ summary: { total: 2, blocked: 1 } }),
});
assertEqual(
  blockedBadge.tone,
  "attention",
  "blocked agents render the attention tone",
);

const unavailableCountBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ summary: { total: 3, unavailable: 2 } }),
});
assertEqual(
  unavailableCountBadge.tone,
  "attention",
  "unavailable agent rows render the attention tone",
);
assertEqual(
  unavailableCountBadge.summary,
  "2 unavailable",
  "unavailable agent rows are included in the compact summary",
);

const degradedBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({ status: "degraded", summary: { total: 2, active: 1 } }),
});
assertEqual(
  degradedBadge.tone,
  "attention",
  "a degraded projection renders the attention tone",
);
assertEqual(
  degradedBadge.title.includes("(degraded)"),
  true,
  "a degraded projection notes degradation in the bounded title",
);

const longTitleBadge = workRootActivityBadge({
  phase: "ready",
  view: activityView({
    status: "degraded",
    summary: {
      total: 999_999_999_999_999,
      active: 999_999_999_999_999,
      blocked: 999_999_999_999_999,
      failed: 999_999_999_999_999,
      unavailable: 999_999_999_999_999,
    },
  }),
});
assertEqual(
  longTitleBadge.title.length,
  120,
  "an over-limit activity badge title is truncated to the limit",
);
assertEqual(
  longTitleBadge.title.endsWith("…"),
  true,
  "an over-limit activity badge title ends with an ellipsis",
);

const orderedItems = orderActivityItems([
  activityItem({ id: "old-live", live: true, updatedAt: "2026-05-21T10:00:00Z" }),
  activityItem({ id: "new-idle", updatedAt: "2026-05-21T12:00:00Z" }),
  activityItem({ id: "attention", attention: true, updatedAt: "2026-05-21T09:00:00Z" }),
  activityItem({ id: "same-a", updatedAt: "2026-05-21T08:00:00Z" }),
  activityItem({ id: "same-b", updatedAt: "2026-05-21T08:00:00Z" }),
]);
assertDeepEqual(
  orderedItems.map((item) => item.id),
  ["old-live", "attention", "new-idle", "same-a", "same-b"],
  "activity ribbon ordering prefers live/attention, then latest, then stable id",
);
assertEqual(
  defaultActivitySelection(orderedItems),
  "old-live",
  "default selection prefers the first live/attention item",
);
assertEqual(
  preserveActivitySelection(orderedItems, "new-idle"),
  "new-idle",
  "selection is preserved when the item still exists",
);
assertEqual(
  preserveActivitySelection(orderedItems, "missing"),
  "old-live",
  "selection falls back to default when the selected item disappears",
);

const ackSource = activityItem({
  id: "ackable",
  updatedAt: "2026-05-21T12:00:00Z",
  transcript: { status: "available", available: true, cursor: "cursor:1" },
});
const acknowledgements = acknowledgeActivityItem({}, ackSource);
assertDeepEqual(
  Array.from(initializeActivityDirtyItems([ackSource], acknowledgements)),
  [],
  "local acknowledgement clears dirty state for the acknowledged item revision",
);
assertDeepEqual(
  Array.from(
    initializeActivityDirtyItems(
      [
        { ...ackSource, updatedAt: "2026-05-21T12:01:00Z" },
        activityItem({ id: "attention-dirty", attention: true }),
        activityItem({ id: "first-load-idle", updatedAt: "2026-05-21T12:02:00Z" }),
      ],
      acknowledgements,
      { ackable: "2026-05-21T12:00:00Z", "first-load-idle": "2026-05-21T12:02:00Z" },
    ),
  ),
  ["ackable", "attention-dirty"],
  "dirty initialization compares local acknowledgements and seen revisions without marking every first-load item dirty",
);

assertEqual(
  shouldApplyActivityTranscriptResponse(
    { workRootId: "root-a", activityId: "agent:a", requestId: 2 },
    { workRootId: "root-a", activityId: "agent:a" },
    { workRootId: "root-a", activityId: "agent:a", requestId: 2 },
  ),
  true,
  "matching transcript response may update selected transcript state",
);
assertEqual(
  shouldApplyActivityTranscriptResponse(
    { workRootId: "root-a", activityId: "agent:a", requestId: 2 },
    { workRootId: "root-a", activityId: "agent:a" },
    { workRootId: "root-b", activityId: "agent:b", requestId: 3 },
  ),
  false,
  "stale transcript response for an old root or request is ignored",
);
assertEqual(
  shouldApplyActivityTranscriptRequest(
    { workRootId: "root-a", activityId: "agent:a", requestId: 2 },
    { workRootId: "root-b", activityId: "agent:a", requestId: 2 },
  ),
  false,
  "error paths also require the expected workRoot/activity/request tuple",
);
assertEqual(
  shouldApplyActivityTranscriptRequest(
    { workRootId: "root-a", activityId: "agent:a", requestId: 1 },
    { workRootId: "root-a", activityId: "agent:a", requestId: 2 },
    { workRootId: "root-a", activityId: "agent:a" },
  ),
  false,
  "an unavailable same-activity state with a newer request id rejects an older successful response",
);
assertEqual(
  shouldApplyActivityTranscriptRequest(
    { workRootId: "root-a", activityId: "agent:a", requestId: 1 },
    { workRootId: "root-a", activityId: "agent:a", requestId: 2 },
  ),
  false,
  "an unavailable same-activity state with a newer request id rejects an older rejection",
);
assertEqual(
  shouldLoadMoreActivityTranscript(
    { scrollTop: 460, clientHeight: 500, scrollHeight: 1_000 },
    true,
    false,
  ),
  true,
  "near-end transcript scroll triggers load-more when more blocks exist",
);
assertEqual(
  shouldLoadMoreActivityTranscript(
    { scrollTop: 100, clientHeight: 500, scrollHeight: 1_000 },
    true,
    false,
  ),
  false,
  "far-from-end transcript scroll does not trigger load-more",
);

function block(partial: Partial<TranscriptBlock>): TranscriptBlock {
  return {
    cursor: partial.cursor ?? "1",
    timestamp: partial.timestamp ?? null,
    renderKind: partial.renderKind ?? "markdown",
    title: partial.title ?? null,
    text: partial.text ?? "hello",
    data: partial.data ?? null,
    degraded: partial.degraded ?? false,
  };
}

assertEqual(
  transcriptBlockView(block({ renderKind: "markdown", text: "assistant output" }), "namedAgent").mode,
  "expanded",
  "assistant/output-like transcript blocks expand by default",
);
assertEqual(
  transcriptBlockView(block({ renderKind: "json", title: "tool call", data: { ok: true } }), "namedAgent").tone,
  "tool",
  "tool-like transcript blocks default to compact tool summaries",
);
assertEqual(
  transcriptBlockView(
    block({
      renderKind: "toolCall",
      title: "Tool call",
      text: "Called functions.exec_command",
      data: { name: "functions.exec_command", argumentsBytes: 128 },
    }),
    "namedAgent",
  ).summary,
  "functions.exec_command · 128 arg bytes",
  "tool call compact summaries include the safe tool name and argument size",
);
assertEqual(
  transcriptBlockView(
    block({
      renderKind: "toolResult",
      title: "Tool output",
      text: "Tool output captured",
      data: { outputBytes: 4_096 },
    }),
    "namedAgent",
  ).summary,
  "Tool output · 4096 bytes",
  "tool result compact summaries include bounded output size",
);
assertEqual(
  transcriptBlockView(block({ renderKind: "status", title: "running" }), "namedAgent").mode,
  "compact",
  "status transcript blocks default to compact summaries",
);
assertEqual(
  transcriptBlockView(
    block({ renderKind: "status", title: "Task started", text: "Agent turn started" }),
    "namedAgent",
  ).summary,
  "Agent turn started",
  "status compact summaries prefer meaningful text over category titles",
);
assertEqual(
  transcriptBlockView(
    block({
      renderKind: "status",
      title: "Unsupported transcript record",
      text: "Skipped unsupported native transcript record",
      data: { eventType: "unsupported", payloadType: "unsupported" },
      degraded: true,
    }),
    "namedAgent",
  ).summary,
  "Skipped unsupported native transcript record",
  "degraded unsupported-like compact summaries avoid repeating the generic title",
);
assertEqual(
  transcriptBlockView(block({ renderKind: "error", title: "failed", degraded: true }), "namedAgent").tone,
  "error",
  "error transcript blocks use error tone",
);
assertEqual(
  transcriptBlockView(block({ renderKind: "text", title: "exec", text: "$ echo ok\nok" }), "exec").mode,
  "terminal",
  "exec transcript blocks render with terminal mode",
);
