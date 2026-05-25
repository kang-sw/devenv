import {
  fetchGitWorktreeAddOptions,
  gitWorktreeAddBase,
  previewGitWorktreeAdd,
  submitGitWorktreeAdd,
  GitWorktreeAddSubmitError,
} from "./gitWorktreeAdd.js";

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

assertEqual(
  gitWorktreeAddBase("workspace/id", "server remote/1"),
  "/api/dashboard/servers/server%20remote%2F1/workspaces/workspace%2Fid/git-worktree-add",
  "server-scoped git worktree add base encodes server id and workspace id",
);
assertEqual(
  gitWorktreeAddBase("workspace/id", "server-local"),
  "/api/dashboard/workspaces/workspace%2Fid/git-worktree-add",
  "server-local git worktree add base preserves local compatibility route",
);

const calls: Array<{ url: string; init?: RequestInit }> = [];
const privatePath = "/Users/example/private/repo";
const responses: unknown[] = [
  {
    workspaceId: "workspace-local-abc",
    git: { available: true, rootLabel: "repo" },
    branches: [{ name: "main", checkedOut: true, current: true }],
    defaults: { worktreeBaseDirLabel: ".git/ws-worktree" },
  },
  {
    branchName: "feature-one",
    filesystemName: "feature-one",
    targetPathLabel: privatePath,
    status: "willCreateBranch",
    message: "new branch will be created",
    blockers: [],
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
    createdWorkRootId: "root-local-created",
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

const options = await fetchGitWorktreeAddOptions("workspace-local-abc");
assertEqual(
  calls[0].url,
  "/api/dashboard/workspaces/workspace-local-abc/git-worktree-add/options",
  "options endpoint is workspace scoped",
);
assertEqual(options.git.available, true, "options parse Git availability");

await previewGitWorktreeAdd("workspace-local-abc", {
  worktreeName: "Feature One",
  branch: { mode: "auto" },
  path: { mode: "custom", targetPath: privatePath },
});
assertEqual(
  calls[1].url,
  "/api/dashboard/workspaces/workspace-local-abc/git-worktree-add/preview",
  "preview endpoint is workspace scoped",
);
assertEqual(calls[1].init?.method, "POST", "preview uses POST");
assertNotContains(
  calls[1].url,
  privatePath,
  "preview URL omits private target path",
);
assertEqual(
  JSON.parse(String(calls[1].init?.body)).path.targetPath,
  privatePath,
  "preview body carries authenticated target path",
);

const submitted = await submitGitWorktreeAdd("workspace-local-abc", {
  worktreeName: "Feature One",
  branch: { mode: "auto" },
  path: { mode: "custom", targetPath: privatePath },
  activate: true,
});
assertEqual(
  calls[2].url,
  "/api/dashboard/workspaces/workspace-local-abc/git-worktree-add",
  "submit endpoint is workspace scoped",
);
assertNotContains(
  calls[2].url,
  privatePath,
  "submit URL omits private target path",
);
assertEqual(
  submitted.createdWorkRootId,
  "root-local-created",
  "submit parses daemon-created workRoot id",
);

responses.push({
  branchName: "main",
  filesystemName: "main-copy",
  targetPathLabel: privatePath,
  status: "blocked",
  message: "branch is already checked out in another worktree",
  blockers: [
    {
      code: "branchAlreadyCheckedOut",
      field: "branch",
      message: "branch is already checked out in another worktree",
    },
  ],
});
globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  calls.push({ url: String(url), init });
  const body = responses.shift();
  return new Response(JSON.stringify(body), {
    status: 400,
    headers: { "Content-Type": "application/json" },
  });
}) as typeof fetch;

try {
  await submitGitWorktreeAdd("workspace-local-abc", {
    worktreeName: "Main Copy",
    branch: { mode: "manual", name: "main" },
    path: { mode: "custom", targetPath: privatePath },
    activate: true,
  });
  throw new Error("blocked submit unexpectedly succeeded");
} catch (error) {
  if (!(error instanceof GitWorktreeAddSubmitError)) {
    throw error;
  }
  assertEqual(error.status, 400, "blocked submit preserves status");
  assertEqual(
    error.preview?.status,
    "blocked",
    "blocked submit preserves daemon preview",
  );
  assertEqual(
    error.preview?.blockers[0]?.code,
    "branchAlreadyCheckedOut",
    "blocked submit preserves blocker code",
  );
}
