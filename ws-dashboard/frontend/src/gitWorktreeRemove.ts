import {
  localCompatibleDashboardApiRoute,
  type DashboardResourcesView,
} from "./resourceModel.js";

// Non-mutating preview for the worktree-remove modal: dirty-state signal
// (B-1's red data-loss banner) plus the branch-unmerged signal (B-2's red
// "delete branch" warning). Mirrors the daemon's GitWorktreeRemovePreview.
export type GitWorktreeRemovePreview = {
  workRootId: string;
  targetPathLabel: string;
  branchName: string | null;
  hasUncommittedChanges: boolean;
  modifiedFiles: number;
  untrackedFiles: number;
  branchUnmerged: boolean;
  available: boolean;
  reason: string | null;
};

export type RemoveGitWorktreeRequest = {
  deleteBranch: boolean;
  force: boolean;
};

export type RemoveGitWorktreeResponse = {
  resources: DashboardResourcesView;
  removedWorkRootId?: string;
  branchDeleted: boolean;
  branchDeleteSkippedUnmerged: boolean;
};

export const gitWorktreeRemoveBase = (
  workRootId: string,
  serverRoute?: string | null,
) =>
  localCompatibleDashboardApiRoute(serverRoute, [
    "work-roots",
    workRootId,
    "git-worktree-remove",
  ]);

export class GitWorktreeRemoveSubmitError extends Error {
  readonly status: number;
  readonly serverError: string | null;

  constructor(status: number, fallback: string, serverError: string | null) {
    super(`HTTP ${status}: ${serverError ?? fallback}`);
    this.name = "GitWorktreeRemoveSubmitError";
    this.status = status;
    this.serverError = serverError;
  }
}

export async function fetchGitWorktreeRemovePreview(
  workRootId: string,
  serverRoute?: string | null,
): Promise<GitWorktreeRemovePreview> {
  const response = await fetch(
    `${gitWorktreeRemoveBase(workRootId, serverRoute)}/preview`,
    { headers: { Accept: "application/json" } },
  );
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: worktree remove preview failed`);
  }
  return (await response.json()) as GitWorktreeRemovePreview;
}

export async function submitGitWorktreeRemove(
  workRootId: string,
  request: RemoveGitWorktreeRequest,
  serverRoute?: string | null,
): Promise<RemoveGitWorktreeResponse> {
  const response = await fetch(gitWorktreeRemoveBase(workRootId, serverRoute), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(request),
  });
  const body = (await response.json().catch(() => null)) as unknown;
  if (!response.ok) {
    const serverError =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : null;
    throw new GitWorktreeRemoveSubmitError(
      response.status,
      "worktree remove failed",
      serverError,
    );
  }
  return body as RemoveGitWorktreeResponse;
}
