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

export class GitWorktreeAddSubmitError extends Error {
  readonly status: number;
  readonly preview: GitWorktreeAddPreview | null;

  constructor(status: number, fallback: string, preview: GitWorktreeAddPreview | null) {
    super(`HTTP ${status}: ${preview?.message ?? fallback}`);
    this.name = "GitWorktreeAddSubmitError";
    this.status = status;
    this.preview = preview;
  }
}

async function readJsonResponse<T>(response: Response, fallback: string): Promise<T> {
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${fallback}`);
  }
  return (await response.json()) as T;
}

function isGitWorktreeAddPreview(value: unknown): value is GitWorktreeAddPreview {
  if (!value || typeof value !== "object") {
    return false;
  }
  const candidate = value as Partial<GitWorktreeAddPreview>;
  return (
    typeof candidate.branchName === "string" &&
    typeof candidate.filesystemName === "string" &&
    typeof candidate.targetPathLabel === "string" &&
    (candidate.status === "willCreateBranch" ||
      candidate.status === "willCheckoutExisting" ||
      candidate.status === "blocked") &&
    typeof candidate.message === "string" &&
    Array.isArray(candidate.blockers)
  );
}

async function readSubmitResponse(response: Response): Promise<AddGitWorktreeResponse> {
  const body = (await response.json().catch(() => null)) as unknown;
  if (!response.ok) {
    throw new GitWorktreeAddSubmitError(
      response.status,
      "worktree add failed",
      isGitWorktreeAddPreview(body) ? body : null,
    );
  }
  return body as AddGitWorktreeResponse;
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
  return readSubmitResponse(response);
}
