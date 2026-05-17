import {
  fetchWorkRootActivity,
  workRootActivityEndpoint,
  type WorkRootActivityView,
} from "./workRootActivity.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
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

const originalFetch = globalThis.fetch;
try {
  globalThis.fetch = (async (input, init) => {
    assertEqual(
      String(input),
      "/api/dashboard/work-roots/root-local-abc/activity",
      "fetch helper uses the WorkRoot Activity endpoint",
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

  const activity = await fetchWorkRootActivity("root-local-abc");
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
