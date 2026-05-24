export type WorkRootGitStatus = {
  available: boolean;
  reason?: string;
  branch?: { name?: string; detachedOid?: string; upstream?: string };
  changes: {
    addedLines: number;
    removedLines: number;
    modifiedFiles: number;
    untrackedFiles: number;
  };
  sync: { ahead: number; behind: number; upstream?: string };
  operations?: { canFetch: boolean; canPush: boolean; canPullFfOnly: boolean };
  refreshedAtMs: number;
};

export type GitBranchList = {
  current?: string;
  detachedOid?: string;
  branches: Array<{
    name: string;
    current: boolean;
    checkedOut: boolean;
    upstream?: string;
    ahead?: number;
    behind?: number;
    disabledReason?: string;
  }>;
};

const gitBase = (workRootId: string) =>
  `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/git`;

async function readJson<T>(response: Response, fallback: string): Promise<T> {
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { error?: string } | null;
    throw new Error(body?.error ?? `HTTP ${response.status}: ${fallback}`);
  }
  return (await response.json()) as T;
}

export async function fetchWorkRootGitStatus(workRootId: string): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId)}/status`, { headers: { Accept: "application/json" } });
  return readJson<WorkRootGitStatus>(response, "git status failed");
}

export async function fetchWorkRootGitBranches(workRootId: string): Promise<GitBranchList> {
  const response = await fetch(`${gitBase(workRootId)}/branches`, { headers: { Accept: "application/json" } });
  return readJson<GitBranchList>(response, "git branches failed");
}

export async function switchWorkRootGitBranch(workRootId: string, branchName: string): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId)}/switch-branch`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ branchName }),
  });
  return readJson<WorkRootGitStatus>(response, "git branch switch failed");
}

export async function createWorkRootGitBranch(workRootId: string, branchName: string, baseBranch?: string): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId)}/branches`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ branchName, baseBranch, switchTo: true }),
  });
  return readJson<WorkRootGitStatus>(response, "git branch create failed");
}

export async function fetchWorkRootGit(workRootId: string): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId)}/fetch`, { method: "POST", headers: { Accept: "application/json" } });
  return readJson<WorkRootGitStatus>(response, "git fetch failed");
}

export async function pushWorkRootGit(workRootId: string): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId)}/push`, { method: "POST", headers: { Accept: "application/json" } });
  return readJson<WorkRootGitStatus>(response, "git push failed");
}

export async function pullWorkRootGitFfOnly(workRootId: string): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId)}/pull-ff-only`, { method: "POST", headers: { Accept: "application/json" } });
  return readJson<WorkRootGitStatus>(response, "git pull --ff-only failed");
}

export function gitStatusSegments(status: WorkRootGitStatus): string {
  const segments = [
    status.changes.addedLines > 0 ? `+${status.changes.addedLines}` : null,
    status.changes.removedLines > 0 ? `-${status.changes.removedLines}` : null,
    status.changes.modifiedFiles > 0 ? `*${status.changes.modifiedFiles}` : null,
    status.changes.untrackedFiles > 0 ? `?${status.changes.untrackedFiles}` : null,
  ].filter(Boolean);
  const sync = [
    status.sync.ahead > 0 ? `↑${status.sync.ahead}` : null,
    status.sync.behind > 0 ? `↓${status.sync.behind}` : null,
  ].filter(Boolean);
  return `${segments.length ? segments.join(" ") : "clean"}${sync.length ? ` | ${sync.join(" ")}` : ""}`;
}
