import { fetchWithTimeout } from "./fetchWithTimeout.js";
import { localCompatibleDashboardApiRoute } from "./resourceModel.js";
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

export type GitStatusSegmentTone =
  | "added"
  | "removed"
  | "modified"
  | "untracked"
  | "push"
  | "pull"
  | "clean";

export type GitStatusSegment = {
  key: string;
  label: string;
  tone: GitStatusSegmentTone;
  commandId?: "git.push" | "git.pullFfOnly";
  disabled?: boolean;
};

export type GitRefreshSchedulerEnvironment = {
  isDocumentHidden: () => boolean;
  addDocumentListener: (
    event: "visibilitychange",
    listener: () => void,
  ) => void;
  removeDocumentListener: (
    event: "visibilitychange",
    listener: () => void,
  ) => void;
  addWindowListener: (event: "focus", listener: () => void) => void;
  removeWindowListener: (event: "focus", listener: () => void) => void;
  setInterval: (listener: () => void, ms: number) => number;
  clearInterval: (handle: number) => void;
};

export const gitBase = (workRootId: string, serverRoute?: string | null) =>
  localCompatibleDashboardApiRoute(serverRoute, ["work-roots", workRootId, "git"]);

async function readJson<T>(response: Response, fallback: string): Promise<T> {
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as {
      error?: string;
    } | null;
    throw new Error(body?.error ?? `HTTP ${response.status}: ${fallback}`);
  }
  return (await response.json()) as T;
}

export async function fetchWorkRootGitStatus(
  workRootId: string,
  serverRoute?: string | null,
): Promise<WorkRootGitStatus> {
  const response = await fetchWithTimeout(
    `${gitBase(workRootId, serverRoute)}/status`,
    { headers: { Accept: "application/json" } },
  );
  return readJson<WorkRootGitStatus>(response, "git status failed");
}

export async function fetchWorkRootGitBranches(
  workRootId: string,
  serverRoute?: string | null,
): Promise<GitBranchList> {
  const response = await fetchWithTimeout(
    `${gitBase(workRootId, serverRoute)}/branches`,
    { headers: { Accept: "application/json" } },
  );
  return readJson<GitBranchList>(response, "git branches failed");
}

export async function switchWorkRootGitBranch(
  workRootId: string,
  branchName: string,
  serverRoute?: string | null,
): Promise<WorkRootGitStatus> {
  const response = await fetch(
    `${gitBase(workRootId, serverRoute)}/switch-branch`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify({ branchName }),
    },
  );
  return readJson<WorkRootGitStatus>(response, "git branch switch failed");
}

export async function createWorkRootGitBranch(
  workRootId: string,
  branchName: string,
  baseBranch?: string,
  serverRoute?: string | null,
): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId, serverRoute)}/branches`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ branchName, baseBranch, switchTo: true }),
  });
  return readJson<WorkRootGitStatus>(response, "git branch create failed");
}

export async function fetchWorkRootGit(
  workRootId: string,
  serverRoute?: string | null,
): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId, serverRoute)}/fetch`, {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  return readJson<WorkRootGitStatus>(response, "git fetch failed");
}

export async function pushWorkRootGit(
  workRootId: string,
  serverRoute?: string | null,
): Promise<WorkRootGitStatus> {
  const response = await fetch(`${gitBase(workRootId, serverRoute)}/push`, {
    method: "POST",
    headers: { Accept: "application/json" },
  });
  return readJson<WorkRootGitStatus>(response, "git push failed");
}

export async function pullWorkRootGitFfOnly(
  workRootId: string,
  serverRoute?: string | null,
): Promise<WorkRootGitStatus> {
  const response = await fetch(
    `${gitBase(workRootId, serverRoute)}/pull-ff-only`,
    { method: "POST", headers: { Accept: "application/json" } },
  );
  return readJson<WorkRootGitStatus>(response, "git pull --ff-only failed");
}

export function gitStatusSegments(status: WorkRootGitStatus): string {
  const segments = gitChangeStatusSegments(status).map(
    (segment) => segment.label,
  );
  const sync = gitSyncStatusSegments(status).map((segment) => segment.label);
  return `${segments.length ? segments.join(" ") : "clean"}${sync.length ? ` | ${sync.join(" ")}` : ""}`;
}

export function gitChangeStatusSegments(
  status: WorkRootGitStatus,
): GitStatusSegment[] {
  return [
    status.changes.addedLines > 0
      ? { key: "added", label: `+${status.changes.addedLines}`, tone: "added" }
      : null,
    status.changes.removedLines > 0
      ? {
          key: "removed",
          label: `-${status.changes.removedLines}`,
          tone: "removed",
        }
      : null,
    status.changes.modifiedFiles > 0
      ? {
          key: "modified",
          label: `*${status.changes.modifiedFiles}`,
          tone: "modified",
        }
      : null,
    status.changes.untrackedFiles > 0
      ? {
          key: "untracked",
          label: `?${status.changes.untrackedFiles}`,
          tone: "untracked",
        }
      : null,
  ].filter((segment): segment is GitStatusSegment => segment !== null);
}

export function gitSyncStatusSegments(
  status: WorkRootGitStatus,
): GitStatusSegment[] {
  const segments: GitStatusSegment[] = [];
  if (status.sync.ahead > 0) {
    segments.push({
      key: "push",
      label: `↑${status.sync.ahead}`,
      tone: "push",
      commandId: "git.push",
      disabled: !status.operations?.canPush,
    });
  }
  if (status.sync.behind > 0) {
    segments.push({
      key: "pull",
      label: `↓${status.sync.behind}`,
      tone: "pull",
      commandId: "git.pullFfOnly",
      disabled: !status.operations?.canPullFfOnly,
    });
  }
  return segments;
}

export function shouldRefreshGitWhileVisible(documentHidden: boolean): boolean {
  return !documentHidden;
}

export function startGitRefreshScheduler(
  refreshGit: (reason: string) => void,
  env: GitRefreshSchedulerEnvironment,
  intervalMs = 5000,
): () => void {
  const refreshIfVisible = (reason: string) => {
    if (shouldRefreshGitWhileVisible(env.isDocumentHidden()))
      refreshGit(reason);
  };
  const onVisible = () => refreshIfVisible("git visibility refresh");
  const onFocus = () => refreshGit("git focus refresh");
  env.addDocumentListener("visibilitychange", onVisible);
  env.addWindowListener("focus", onFocus);
  const interval = env.setInterval(
    () => refreshIfVisible("git poll"),
    intervalMs,
  );
  return () => {
    env.removeDocumentListener("visibilitychange", onVisible);
    env.removeWindowListener("focus", onFocus);
    env.clearInterval(interval);
  };
}
