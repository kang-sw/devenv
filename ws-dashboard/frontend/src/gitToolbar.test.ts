import {
  fetchWorkRootGitStatus,
  fetchWorkRootGitBranches,
  switchWorkRootGitBranch,
  createWorkRootGitBranch,
  fetchWorkRootGit,
  pushWorkRootGit,
  pullWorkRootGitFfOnly,
  gitStatusSegments,
} from "./gitToolbar.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
}
function assertNotContains(value: string, forbidden: string, label: string) {
  if (value.includes(forbidden)) throw new Error(`${label}: leaked ${forbidden}`);
}

const privatePath = "/Users/example/private/repo";
const calls: Array<{ url: string; init?: RequestInit }> = [];
const status = {
  available: true,
  branch: { name: "main", upstream: "origin/main" },
  changes: { addedLines: 3, removedLines: 1, modifiedFiles: 2, untrackedFiles: 4 },
  sync: { ahead: 1, behind: 2, upstream: "origin/main" },
  operations: { canFetch: true, canPush: true, canPullFfOnly: true },
  refreshedAtMs: 1,
};
const responses: unknown[] = [
  status,
  { current: "main", branches: [{ name: "main", current: true, checkedOut: true }] },
  status,
  status,
  status,
  status,
  status,
];

globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  calls.push({ url: String(url), init });
  return new Response(JSON.stringify(responses.shift()), { status: 200, headers: { "Content-Type": "application/json" } });
}) as typeof fetch;

await fetchWorkRootGitStatus("root-local-private");
await fetchWorkRootGitBranches("root-local-private");
await switchWorkRootGitBranch("root-local-private", "feature/private");
await createWorkRootGitBranch("root-local-private", "new-private", "main");
await fetchWorkRootGit("root-local-private");
await pushWorkRootGit("root-local-private");
await pullWorkRootGitFfOnly("root-local-private");

assertEqual(calls[0].url, "/api/dashboard/work-roots/root-local-private/git/status", "status URL");
assertEqual(calls[1].url, "/api/dashboard/work-roots/root-local-private/git/branches", "branches URL");
assertEqual(calls[2].url, "/api/dashboard/work-roots/root-local-private/git/switch-branch", "switch URL");
assertEqual(calls[6].url, "/api/dashboard/work-roots/root-local-private/git/pull-ff-only", "ff-only pull URL");
for (const call of calls) assertNotContains(call.url, privatePath, "git toolbar route URL omits host path");
assertEqual(JSON.parse(String(calls[2].init?.body)).branchName, "feature/private", "switch body carries branch only");
assertEqual(gitStatusSegments(status), "+3 -1 *2 ?4 | ↑1 ↓2", "status segment grammar");
