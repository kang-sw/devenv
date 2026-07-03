import {
  fetchWorkRootGitStatus,
  gitBase,
  fetchWorkRootGitBranches,
  switchWorkRootGitBranch,
  createWorkRootGitBranch,
  fetchWorkRootGit,
  pushWorkRootGit,
  pullWorkRootGitFfOnly,
  gitChangeStatusSegments,
  gitSyncStatusSegments,
  gitStatusSegments,
  shouldRefreshGitWhileVisible,
  startGitRefreshScheduler,
} from "./gitToolbar.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected)
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
}
function assertNotContains(value: string, forbidden: string, label: string) {
  if (value.includes(forbidden))
    throw new Error(`${label}: leaked ${forbidden}`);
}

const privatePath = "/Users/example/private/repo";
assertEqual(
  gitBase("root/id", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Fid/git",
  "server-scoped Git base encodes server id and workRoot id",
);
assertEqual(
  gitBase("root/id", "server-local"),
  "/api/dashboard/work-roots/root%2Fid/git",
  "server-local Git base preserves local compatibility route",
);
const calls: Array<{ url: string; init?: RequestInit }> = [];
const status = {
  available: true,
  branch: { name: "main", upstream: "origin/main" },
  changes: {
    addedLines: 3,
    removedLines: 1,
    modifiedFiles: 2,
    untrackedFiles: 4,
  },
  sync: { ahead: 1, behind: 2, upstream: "origin/main" },
  operations: { canFetch: true, canPush: true, canPullFfOnly: true },
  refreshedAtMs: 1,
};
const branches = {
  current: "main",
  branches: [{ name: "main", current: true, checkedOut: true }],
};
const responses: unknown[] = [
  status,
  branches,
  status,
  status,
  status,
  status,
  status,
  status,
  branches,
  status,
  status,
  status,
  status,
  status,
];

globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  calls.push({ url: String(url), init });
  return new Response(JSON.stringify(responses.shift()), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}) as typeof fetch;

await fetchWorkRootGitStatus("root-local-private");
await fetchWorkRootGitBranches("root-local-private");
await switchWorkRootGitBranch("root-local-private", "feature/private");
await createWorkRootGitBranch("root-local-private", "new-private", "main");
await fetchWorkRootGit("root-local-private");
await pushWorkRootGit("root-local-private");
await pullWorkRootGitFfOnly("root-local-private");

assertEqual(
  calls[0].url,
  "/api/dashboard/work-roots/root-local-private/git/status",
  "status URL",
);
assertEqual(
  calls[1].url,
  "/api/dashboard/work-roots/root-local-private/git/branches",
  "branches URL",
);
assertEqual(
  calls[2].url,
  "/api/dashboard/work-roots/root-local-private/git/switch-branch",
  "switch URL",
);
assertEqual(
  calls[6].url,
  "/api/dashboard/work-roots/root-local-private/git/pull-ff-only",
  "ff-only pull URL",
);

// A linked server sharing the same bare workRoot id must keep every Git
// toolbar request on the local gateway's server-scoped route rather than the
// bare local route.
await fetchWorkRootGitStatus("root-same", "server-remote-1");
await fetchWorkRootGitBranches("root-same", "server-remote-1");
await switchWorkRootGitBranch("root-same", "feature/private", "server-remote-1");
await createWorkRootGitBranch("root-same", "new-private", "main", "server-remote-1");
await fetchWorkRootGit("root-same", "server-remote-1");
await pushWorkRootGit("root-same", "server-remote-1");
await pullWorkRootGitFfOnly("root-same", "server-remote-1");

assertEqual(
  calls[7].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/status",
  "remote status URL stays on local gateway server-scoped route",
);
assertEqual(
  calls[8].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/branches",
  "remote branches URL stays on local gateway server-scoped route",
);
assertEqual(
  calls[9].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/switch-branch",
  "remote switch URL stays on local gateway server-scoped route",
);
assertEqual(
  calls[10].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/branches",
  "remote create-branch URL stays on local gateway server-scoped route",
);
assertEqual(
  calls[11].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/fetch",
  "remote fetch URL stays on local gateway server-scoped route",
);
assertEqual(
  calls[12].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/push",
  "remote push URL stays on local gateway server-scoped route",
);
assertEqual(
  calls[13].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-same/git/pull-ff-only",
  "remote pull URL stays on local gateway server-scoped route",
);
for (const call of calls)
  assertNotContains(
    call.url,
    privatePath,
    "git toolbar route URL omits host path",
  );
assertEqual(
  JSON.parse(String(calls[2].init?.body)).branchName,
  "feature/private",
  "switch body carries branch only",
);
assertEqual(
  gitStatusSegments(status),
  "+3 -1 *2 ?4 | ↑1 ↓2",
  "status segment grammar",
);
assertEqual(
  gitChangeStatusSegments(status)
    .map((segment) => segment.tone)
    .join(","),
  "added,removed,modified,untracked",
  "change segments keep per-tone styling metadata",
);
assertEqual(
  gitSyncStatusSegments(status)
    .map((segment) => `${segment.commandId}:${segment.label}`)
    .join(","),
  "git.push:↑1,git.pullFfOnly:↓2",
  "sync segments carry interactive command ids",
);
assertEqual(
  shouldRefreshGitWhileVisible(false),
  true,
  "visible document refreshes Git polling",
);
assertEqual(
  shouldRefreshGitWhileVisible(true),
  false,
  "hidden document pauses Git polling",
);

let hidden = false;
const refreshes: string[] = [];
const documentListeners = new Map<string, () => void>();
const windowListeners = new Map<string, () => void>();
let intervalListener: (() => void) | null = null;
let clearedInterval = 0;
const cleanupScheduler = startGitRefreshScheduler(
  (reason) => refreshes.push(reason),
  {
    isDocumentHidden: () => hidden,
    addDocumentListener: (event, listener) =>
      documentListeners.set(event, listener),
    removeDocumentListener: (event, listener) => {
      if (documentListeners.get(event) === listener)
        documentListeners.delete(event);
    },
    addWindowListener: (event, listener) =>
      windowListeners.set(event, listener),
    removeWindowListener: (event, listener) => {
      if (windowListeners.get(event) === listener)
        windowListeners.delete(event);
    },
    setInterval: (listener) => {
      intervalListener = listener;
      return 7;
    },
    clearInterval: (handle) => {
      clearedInterval = handle;
    },
  },
);
const runInterval = () => {
  if (!intervalListener)
    throw new Error("interval listener was not registered");
  intervalListener();
};
hidden = true;
documentListeners.get("visibilitychange")?.();
runInterval();
assertEqual(
  refreshes.join(","),
  "",
  "hidden document suppresses visibility and polling refreshes",
);
hidden = false;
documentListeners.get("visibilitychange")?.();
runInterval();
windowListeners.get("focus")?.();
assertEqual(
  refreshes.join(","),
  "git visibility refresh,git poll,git focus refresh",
  "visible document resumes Git refresh triggers",
);
cleanupScheduler();
assertEqual(
  documentListeners.size,
  0,
  "scheduler cleanup removes document listener",
);
assertEqual(
  windowListeners.size,
  0,
  "scheduler cleanup removes window listener",
);
assertEqual(clearedInterval, 7, "scheduler cleanup clears poll interval");
