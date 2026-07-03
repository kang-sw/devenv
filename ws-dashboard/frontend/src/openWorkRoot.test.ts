import {
  requestOpenWorkRoot,
  openWorkRootEndpoint,
  serverOpenWorkRootEndpoint,
} from "./openWorkRoot.js";
import type { DashboardResourcesView } from "./resourceModel.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
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

assertEqual(
  serverOpenWorkRootEndpoint("server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/open",
  "server-scoped open workRoot endpoint encodes server id",
);
assertEqual(
  serverOpenWorkRootEndpoint("server-local"),
  openWorkRootEndpoint,
  "server-local open workRoot endpoint preserves local compatibility route",
);

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
assertEqual(
  capturedUrl,
  openWorkRootEndpoint,
  "open workRoot endpoint is stable",
);
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

const remoteResult = await requestOpenWorkRoot(
  "C:/Remote/same-name",
  "server-remote",
);
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server-remote/work-roots/open",
  "remote open workRoot request targets server-scoped local gateway endpoint",
);
assertEqual(
  JSON.parse(capturedBody).path,
  "C:/Remote/same-name",
  "remote open workRoot request carries remote path to daemon gateway",
);
assertEqual(
  remoteResult.openedWorkRootId,
  "root-second-same-name",
  "remote open wrapper preserves daemon-opened id header unchanged",
);

globalThis.fetch = originalFetch;
