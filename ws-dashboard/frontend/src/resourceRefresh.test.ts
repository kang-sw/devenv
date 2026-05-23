import {
  createResourceRefreshCoordinator,
  requestDashboardResources,
  resourceEndpoint,
  type ResourceRefreshResult,
} from "./resourceRefresh.js";
import type { DashboardResourcesView } from "./resourceModel.js";

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

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

function workRootId(resources: DashboardResourcesView | null): string | undefined {
  return resources?.workspaces[0]?.workRoots[0]?.id;
}

function view(id: string, availability: "available" | "missing" = "available"): DashboardResourcesView {
  return {
    server: {
      id: "server-local",
      label: "Local ws dashboard",
      state: { status: "online", loading: false, stale: false, error: null },
      actions: [{ id: "refresh", label: "Refresh", enabled: true }],
    },
    workspaces: [
      {
        id: `workspace-${id}`,
        label: id,
        state: { status: "ready", loading: false, stale: false, error: null },
        compactable: false,
        actions: [],
        workRoots: [
          {
            id: `workRoot-${id}`,
            resourcePath: {
              serverId: "server-local",
              workspaceId: `workspace-${id}`,
              workRootId: `workRoot-${id}`,
              instanceId: null,
            },
            label: id,
            kind: "plainDirectory",
            activation: "online",
            availability,
            status: availability === "available" ? "online" : "moved",
            state: { status: "ready", loading: false, stale: false, error: null },
            compactable: false,
            mainInstances: [],
            actions: [],
          },
        ],
      },
    ],
  };
}

let capturedUrl = "";
let capturedAccept = "";
await requestDashboardResources((async (input, init) => {
  capturedUrl = String(input);
  capturedAccept = String((init?.headers as Record<string, string> | undefined)?.Accept ?? "");
  return new Response(JSON.stringify(view("canonical")), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}) as typeof fetch);
assertEqual(capturedUrl, resourceEndpoint, "resource refresh uses canonical resources endpoint");
assertEqual(capturedAccept, "application/json", "resource refresh asks for JSON");

{
  const first = deferred<DashboardResourcesView>();
  let fetches = 0;
  let applied: DashboardResourcesView | null = null;
  const results: ResourceRefreshResult[] = [];
  const coordinator = createResourceRefreshCoordinator({
    fetchResources: () => {
      fetches += 1;
      return first.promise;
    },
    applyResources: (resources) => {
      applied = resources;
    },
  });

  const initial = coordinator.refresh("poll").then((result) => results.push(result));
  const skipped = await coordinator.refresh("poll");
  assertEqual(fetches, 1, "polling does not start overlapping resource requests");
  assertDeepEqual(
    skipped,
    { status: "skipped", reason: "poll", cause: "inFlight" },
    "overlapping poll is skipped while a refresh is in flight",
  );

  first.resolve(view("polled"));
  await initial;
  assertEqual(results[0]?.status, "applied", "first poll eventually applies");
  assertEqual(workRootId(applied), "workRoot-polled", "poll applied resources");
}

{
  const pending = deferred<DashboardResourcesView>();
  let fetches = 0;
  let applied: DashboardResourcesView | null = null;
  const coordinator = createResourceRefreshCoordinator({
    fetchResources: () => {
      fetches += 1;
      return fetches === 1 ? pending.promise : Promise.resolve(view("explicit"));
    },
    applyResources: (resources) => {
      applied = resources;
    },
  });

  const poll = coordinator.refresh("poll");
  const skipped = await coordinator.refresh("explicit");
  assertEqual(skipped.status, "skipped", "explicit refresh waits instead of overlapping an in-flight poll");
  assertEqual(fetches, 1, "queued explicit refresh does not overlap poll request");
  pending.resolve(view("poll"));
  await poll;
  await new Promise((resolve) => setTimeout(resolve, 0));
  assertEqual(fetches, 2, "queued explicit refresh runs after the poll completes");
  assertEqual(workRootId(applied), "workRoot-explicit", "explicit refresh wins after queued run");
}

{
  const pending = deferred<DashboardResourcesView>();
  let applied = view("initial");
  let error: string | null = null;
  const coordinator = createResourceRefreshCoordinator({
    fetchResources: () => pending.promise,
    applyResources: (resources) => {
      applied = resources;
    },
    setError: (nextError) => {
      error = nextError;
    },
  });

  const poll = coordinator.refresh("poll");
  coordinator.applyExternalResources(view("opened"));
  pending.resolve(view("stale"));
  const result = await poll;
  assertEqual(result.status, "stale", "slower poll result is recognized as stale after an external open response");
  assertEqual(workRootId(applied), "workRoot-opened", "stale poll cannot overwrite opened resources");
  assertEqual(error, null, "external resource application clears stale refresh errors");
}

{
  let applied = view("last-known");
  let error: string | null = null;
  const coordinator = createResourceRefreshCoordinator({
    fetchResources: () => Promise.reject(new Error("HTTP 503")),
    applyResources: (resources) => {
      applied = resources;
    },
    setError: (nextError) => {
      error = nextError;
    },
  });

  const result = await coordinator.refresh("poll");
  assertEqual(result.status, "failed", "poll failure is bounded to a failed result");
  assertEqual(error, "HTTP 503", "poll failure records a bounded error");
  assertEqual(
    workRootId(applied),
    "workRoot-last-known",
    "poll failure preserves the last known resource tree",
  );
}

{
  const pending = deferred<DashboardResourcesView>();
  let applied: DashboardResourcesView | null = null;
  let loadingChanges = 0;
  let errorChanges = 0;
  const coordinator = createResourceRefreshCoordinator({
    fetchResources: () => pending.promise,
    applyResources: (resources) => {
      applied = resources;
    },
    setLoading: () => {
      loadingChanges += 1;
    },
    setError: () => {
      errorChanges += 1;
    },
  });

  const poll = coordinator.refresh("poll");
  coordinator.dispose();
  pending.resolve(view("after-dispose"));
  const result = await poll;
  assertEqual(result.status, "stale", "disposed polling result is ignored");
  assertEqual(applied, null, "cleanup on unmount prevents later resource application");
  assertEqual(loadingChanges, 0, "polling cleanup does not leave foreground loading changes");
  assertEqual(errorChanges, 0, "polling cleanup does not publish errors after dispose");
}
