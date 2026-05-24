import type { DashboardResourcesView } from "./resourceModel.js";

export type GitWorktreeAddOptions = {
  workspaceId: string;
  git: { available: boolean; reason?: string; rootLabel: string };
  branches: Array<{
    name: string;
    checkedOut: boolean;
    current: boolean;
    disabledReason?: string;
  }>;
  defaults: { worktreeBaseDirLabel: string };
};

export type GitWorktreeAddPreviewRequest = {
  worktreeName: string;
  branch: { mode: "auto" } | { mode: "manual"; name: string };
  path: { mode: "auto" } | { mode: "custom"; targetPath: string };
};

export type GitWorktreeAddPreview = {
  branchName: string;
  filesystemName: string;
  targetPathLabel: string;
  status: "willCreateBranch" | "willCheckoutExisting" | "blocked";
  message: string;
  blockers: Array<{
    code:
      | "invalidWorktreeName"
      | "invalidBranchName"
      | "branchAlreadyCheckedOut"
      | "targetExists"
      | "targetParentMissing"
      | "notGitWorkspace";
    field?: "worktreeName" | "branch" | "path";
    message: string;
  }>;
};

export type AddGitWorktreeRequest = GitWorktreeAddPreviewRequest & {
  activate: boolean;
};

export type AddGitWorktreeResponse = {
  resources: DashboardResourcesView;
  createdWorkRootId?: string;
};

const base = (workspaceId: string) =>
  `/api/dashboard/workspaces/${encodeURIComponent(workspaceId)}/git-worktree-add`;

async function readJsonResponse<T>(response: Response, fallback: string): Promise<T> {
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${fallback}`);
  }
  return (await response.json()) as T;
}

export async function fetchGitWorktreeAddOptions(
  workspaceId: string,
): Promise<GitWorktreeAddOptions> {
  const response = await fetch(`${base(workspaceId)}/options`, {
    headers: { Accept: "application/json" },
  });
  return readJsonResponse<GitWorktreeAddOptions>(response, "worktree options failed");
}

export async function previewGitWorktreeAdd(
  workspaceId: string,
  request: GitWorktreeAddPreviewRequest,
): Promise<GitWorktreeAddPreview> {
  const response = await fetch(`${base(workspaceId)}/preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(request),
  });
  return readJsonResponse<GitWorktreeAddPreview>(response, "worktree preview failed");
}

export async function submitGitWorktreeAdd(
  workspaceId: string,
  request: AddGitWorktreeRequest,
): Promise<AddGitWorktreeResponse> {
  const response = await fetch(base(workspaceId), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(request),
  });
  return readJsonResponse<AddGitWorktreeResponse>(response, "worktree add failed");
}
