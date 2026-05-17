import {
  fetchWorkRootActivity,
  mergeWorkRootActivityViews,
  workRootActivityBadge,
  workRootActivityEndpoint,
  type NamedAgentActivityView,
  type WorkRootActivitySummary,
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
      summary: { total: 0, active: 0, blocked: 0, failed: 0, unavailable: 0 },
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
    summary: activitySummary(partial.summary),
    agents: partial.agents ?? [],
  };
}

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
    summary: { total: 2, active: 1 },
    agents: [
      activityAgent({ agentId: "agent-a", status: "running" }),
      activityAgent({ agentId: "agent-b", status: "idle" }),
    ],
  }),
  activityView({
    summary: { total: 2, blocked: 1, unavailable: 1 },
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
