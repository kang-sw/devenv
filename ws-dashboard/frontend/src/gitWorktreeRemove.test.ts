import {
  fetchGitWorktreeRemovePreview,
  gitWorktreeRemoveBase,
  submitGitWorktreeRemove,
  GitWorktreeRemoveSubmitError,
} from "./gitWorktreeRemove.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

function assertNotContains(value: string, forbidden: string, label: string) {
  if (value.includes(forbidden)) {
    throw new Error(
      `${label}: ${JSON.stringify(value)} contained ${JSON.stringify(forbidden)}`,
    );
  }
}

// --- route derivation (server-scoped + server-local compatibility) ---
assertEqual(
  gitWorktreeRemoveBase("root/id", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Fid/git-worktree-remove",
  "server-scoped remove base encodes server id and work-root id",
);
assertEqual(
  gitWorktreeRemoveBase("root/id", "server-local"),
  "/api/dashboard/work-roots/root%2Fid/git-worktree-remove",
  "server-local remove base preserves local compatibility route",
);

const calls: Array<{ url: string; init?: RequestInit }> = [];
const primaryPath = "/Users/example/private/primary-repo";

// Preview never leaks the primary path in its URL; it carries a label instead.
const responses: unknown[] = [
  {
    workRootId: "root-worktree-1",
    targetPathLabel: "feature-one",
    branchName: "feature-one",
    hasUncommittedChanges: false,
    modifiedFiles: 0,
    untrackedFiles: 0,
    branchUnmerged: false,
    available: true,
    reason: null,
  },
  {
    resources: {
      server: {
        id: "server-local",
        label: "Local",
        state: { status: "online", loading: false, stale: false },
        actions: [],
      },
      workspaces: [],
    },
    removedWorkRootId: "root-worktree-1",
    branchDeleted: true,
    branchDeleteSkippedUnmerged: false,
  },
];

globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  calls.push({ url: String(url), init });
  const body = responses.shift();
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}) as typeof fetch;

const preview = await fetchGitWorktreeRemovePreview("root-worktree-1");
assertEqual(
  calls[0].url,
  "/api/dashboard/work-roots/root-worktree-1/git-worktree-remove/preview",
  "preview endpoint is work-root scoped",
);
assertNotContains(
  calls[0].url,
  primaryPath,
  "preview URL omits primary repo path",
);
assertEqual(
  preview.branchName,
  "feature-one",
  "preview parses the branch name",
);
assertEqual(
  preview.hasUncommittedChanges,
  false,
  "preview parses the clean dirty-state flag",
);

const removed = await submitGitWorktreeRemove("root-worktree-1", {
  deleteBranch: true,
  force: false,
});
assertEqual(
  calls[1].url,
  "/api/dashboard/work-roots/root-worktree-1/git-worktree-remove",
  "submit endpoint is work-root scoped",
);
assertEqual(calls[1].init?.method, "POST", "submit uses POST");
assertEqual(
  JSON.parse(String(calls[1].init?.body)).deleteBranch,
  true,
  "submit body carries the deleteBranch flag",
);
assertEqual(
  removed.removedWorkRootId,
  "root-worktree-1",
  "submit parses the removed work-root id",
);
assertEqual(
  removed.branchDeleted,
  true,
  "submit parses the branchDeleted outcome",
);

// A linked server keeps the remove request on the local gateway's
// server-scoped route rather than dialing the remote daemon directly.
responses.push({
  resources: {
    server: {
      id: "server-remote-1",
      label: "Remote",
      state: { status: "online", loading: false, stale: false },
      actions: [],
    },
    workspaces: [],
  },
  removedWorkRootId: "root-remote-1",
  branchDeleted: false,
  branchDeleteSkippedUnmerged: true,
});
const remote = await submitGitWorktreeRemove(
  "root-remote-1",
  { deleteBranch: true, force: false },
  "server-remote-1",
);
assertEqual(
  calls[2].url,
  "/api/dashboard/servers/server-remote-1/work-roots/root-remote-1/git-worktree-remove",
  "remote submit URL stays on local gateway server-scoped route",
);
assertEqual(
  remote.branchDeleteSkippedUnmerged,
  true,
  "submit surfaces the unmerged branch-keep outcome",
);

// A dirty worktree without force is rejected by the daemon with 409; the
// client surfaces the server error message on the typed error.
responses.push({ error: "worktree has uncommitted or untracked changes; force required" });
globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  calls.push({ url: String(url), init });
  const body = responses.shift();
  return new Response(JSON.stringify(body), {
    status: 409,
    headers: { "Content-Type": "application/json" },
  });
}) as typeof fetch;

try {
  await submitGitWorktreeRemove("root-worktree-1", {
    deleteBranch: false,
    force: false,
  });
  throw new Error("dirty submit without force unexpectedly succeeded");
} catch (error) {
  if (!(error instanceof GitWorktreeRemoveSubmitError)) {
    throw error;
  }
  assertEqual(error.status, 409, "force-gated submit preserves 409 status");
  assertEqual(
    error.serverError,
    "worktree has uncommitted or untracked changes; force required",
    "force-gated submit surfaces the daemon error message",
  );
}

console.log("gitWorktreeRemove.test.ts passed");
