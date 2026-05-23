import { requestOpenWorkRoot, openWorkRootEndpoint } from "./openWorkRoot.js";
import type { DashboardResourcesView } from "./resourceModel.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

const sameLabelView: DashboardResourcesView = {
  server: {
    id: "server-local",
    label: "Local ws dashboard",
    state: { status: "online", loading: false, stale: false, error: null },
    actions: [],
  },
  workspaces: [
    {
      id: "workspace-first",
      label: "first",
      state: { status: "ready", loading: false, stale: false, error: null },
      compactable: false,
      actions: [],
      workRoots: [
        {
          id: "root-first-same-name",
          resourcePath: {
            serverId: "server-local",
            workspaceId: "workspace-first",
            workRootId: "root-first-same-name",
            instanceId: null,
          },
          label: "same-name",
          kind: "plainDirectory",
          activation: "online",
          availability: "available",
          status: "online",
          state: { status: "ready", loading: false, stale: false, error: null },
          compactable: false,
          mainInstances: [],
          actions: [],
        },
      ],
    },
    {
      id: "workspace-second",
      label: "second",
      state: { status: "ready", loading: false, stale: false, error: null },
      compactable: false,
      actions: [],
      workRoots: [
        {
          id: "root-second-same-name",
          resourcePath: {
            serverId: "server-local",
            workspaceId: "workspace-second",
            workRootId: "root-second-same-name",
            instanceId: null,
          },
          label: "same-name",
          kind: "plainDirectory",
          activation: "online",
          availability: "available",
          status: "online",
          state: { status: "ready", loading: false, stale: false, error: null },
          compactable: false,
          mainInstances: [],
          actions: [],
        },
      ],
    },
  ],
};

const originalFetch = globalThis.fetch;
let capturedUrl = "";
let capturedBody = "";
globalThis.fetch = (async (input, init) => {
  capturedUrl = String(input);
  capturedBody = String(init?.body ?? "");
  return new Response(JSON.stringify(sameLabelView), {
    status: 200,
    headers: {
      "content-type": "application/json",
      "x-ws-dashboard-opened-work-root-id": "root-second-same-name",
    },
  });
}) as typeof fetch;

const result = await requestOpenWorkRoot("/tmp/second/same-name");
assertEqual(capturedUrl, openWorkRootEndpoint, "open workRoot endpoint is stable");
assertEqual(
  JSON.parse(capturedBody).path,
  "/tmp/second/same-name",
  "open workRoot request carries submitted path to daemon",
);
assertEqual(
  result.openedWorkRootId,
  "root-second-same-name",
  "wrapper returns daemon-opened id from response header instead of deriving from label",
);
assertEqual(
  result.view.workspaces[0].workRoots[0].id,
  "root-first-same-name",
  "ambiguous first aggregate row remains distinct from opened id header",
);

globalThis.fetch = originalFetch;
