import { useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent } from "react";
import { X } from "lucide-react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import {
  buildGitWorktreeAddCloseCommand,
  buildGitWorktreeAddSubmitCommand,
  type DashboardCommandDispatcher,
} from "./commands.js";
import {
  fetchGitWorktreeAddOptions,
  previewGitWorktreeAdd,
  GitWorktreeAddSubmitError,
  submitGitWorktreeAdd,
  type GitWorktreeAddOptions,
  type GitWorktreeAddPreview,
  type GitWorktreeAddPreviewRequest,
} from "./gitWorktreeAdd.js";
import type { DashboardResourcesView } from "./resourceModel.js";
import { ChromeIconButton, InlineNotice } from "./chrome.js";

export function GitWorktreeAddModal({
  target,
  onCommand,
  onClose,
  onCreated,
}: {
  target: { serverRoute: string; workspaceId: string } | null;
  onCommand: DashboardCommandDispatcher;
  onClose: () => void;
  onCreated: (response: {
    resources: DashboardResourcesView;
    createdWorkRootId?: string;
  }) => void;
}) {
  const workspaceId = target?.workspaceId ?? null;
  const serverRoute = target?.serverRoute ?? null;
  const [options, setOptions] = useState<GitWorktreeAddOptions | null>(null);
  const [worktreeName, setWorktreeName] = useState("");
  const [branchMode, setBranchMode] = useState<"auto" | "manual">("auto");
  const [manualBranch, setManualBranch] = useState("");
  const [pathMode, setPathMode] = useState<"auto" | "custom">("auto");
  const [customPath, setCustomPath] = useState("");
  const [preview, setPreview] = useState<GitWorktreeAddPreview | null>(null);
  const [previewRequestKey, setPreviewRequestKey] = useState<string | null>(
    null,
  );
  const previewSequenceRef = useRef(0);
  const currentRequestKeyRef = useRef<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId || !serverRoute) {
      setOptions(null);
      setPreview(null);
      setPreviewRequestKey(null);
      setError(null);
      return;
    }
    setWorktreeName("");
    setBranchMode("auto");
    setManualBranch("");
    setPathMode("auto");
    setCustomPath("");
    setPreview(null);
    setPreviewRequestKey(null);
    setError(null);
    setLoading(true);
    void fetchGitWorktreeAddOptions(workspaceId, serverRoute)
      .then(setOptions)
      .catch((nextError) =>
        setError(
          nextError instanceof Error
            ? nextError.message
            : "worktree options failed",
        ),
      )
      .finally(() => setLoading(false));
  }, [serverRoute, workspaceId]);

  const request = useMemo<GitWorktreeAddPreviewRequest | null>(() => {
    if (!workspaceId) {
      return null;
    }
    return {
      worktreeName,
      branch:
        branchMode === "auto"
          ? { mode: "auto" }
          : { mode: "manual", name: manualBranch },
      path:
        pathMode === "auto"
          ? { mode: "auto" }
          : { mode: "custom", targetPath: customPath },
    };
  }, [
    branchMode,
    customPath,
    manualBranch,
    pathMode,
    worktreeName,
    workspaceId,
  ]);

  const requestKey = request ? JSON.stringify(request) : null;

  useEffect(() => {
    currentRequestKeyRef.current = requestKey;
  }, [requestKey]);

  useEffect(() => {
    if (
      !workspaceId ||
      !request ||
      !requestKey ||
      worktreeName.trim().length === 0
    ) {
      setPreview(null);
      setPreviewRequestKey(null);
      return;
    }
    const sequence = previewSequenceRef.current + 1;
    previewSequenceRef.current = sequence;
    setPreview(null);
    setPreviewRequestKey(null);
    const timer = window.setTimeout(() => {
      void previewGitWorktreeAdd(workspaceId, request, serverRoute)
        .then((nextPreview) => {
          if (previewSequenceRef.current !== sequence) {
            return;
          }
          setPreview(nextPreview);
          setPreviewRequestKey(requestKey);
        })
        .catch((nextError) => {
          if (previewSequenceRef.current !== sequence) {
            return;
          }
          setError(
            nextError instanceof Error
              ? nextError.message
              : "worktree preview failed",
          );
        });
    }, 150);
    return () => window.clearTimeout(timer);
  }, [request, requestKey, serverRoute, worktreeName, workspaceId]);

  if (!workspaceId || !serverRoute) {
    return null;
  }

  const close = () => {
    onCommand(buildGitWorktreeAddCloseCommand(workspaceId, serverRoute), {
      "gitWorktreeAdd.close": onClose,
    });
  };
  const submitDisabled =
    submitting ||
    !request ||
    worktreeName.trim().length === 0 ||
    (branchMode === "manual" && manualBranch.trim().length === 0) ||
    (pathMode === "custom" && customPath.trim().length === 0) ||
    !preview ||
    previewRequestKey !== requestKey ||
    preview.status === "blocked" ||
    options?.git.available === false;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!request || submitDisabled) {
      return;
    }
    onCommand(buildGitWorktreeAddSubmitCommand(workspaceId, serverRoute), {
      "gitWorktreeAdd.submit": () => {
        const submittedRequestKey = requestKey;
        setSubmitting(true);
        setError(null);
        void submitGitWorktreeAdd(
          workspaceId,
          { ...request, activate: true },
          serverRoute,
        )
          .then(onCreated)
          .catch((nextError) => {
            if (
              nextError instanceof GitWorktreeAddSubmitError &&
              nextError.preview
            ) {
              if (currentRequestKeyRef.current !== submittedRequestKey) {
                return;
              }
              setPreview(nextError.preview);
              setPreviewRequestKey(submittedRequestKey);
              setError("Submit blocked by current server validation");
              return;
            }
            setError(
              nextError instanceof Error
                ? nextError.message
                : "worktree add failed",
            );
          })
          .finally(() => setSubmitting(false));
      },
    });
  };

  const severity = preview?.status ?? "blocked";
  const manualBranchOptions = (options?.branches ?? []).filter(
    (branch) => !branch.checkedOut,
  );
  const autoBranchDisplay = preview?.branchName ?? worktreeName.trim();
  const autoPathDisplay =
    preview?.targetPathLabel ??
    (worktreeName.trim()
      ? `${options?.defaults.worktreeBaseDirLabel ?? ".ws-dashboard/worktrees"}/${worktreeName.trim()}`
      : "");
  return (
    <ModalOverlay
      className="root-picker-backdrop"
      isDismissable
      isOpen
      onOpenChange={(isOpen) => {
        if (!isOpen) close();
      }}
    >
      <Modal className="root-picker-modal git-worktree-modal">
        <Dialog aria-label="Add Git worktree" className="root-picker-dialog">
          <div className="root-picker-titlebar">
            <Heading className="root-picker-title" slot="title">
              Add worktree
            </Heading>
            <div className="root-picker-window-actions">
              <ChromeIconButton
                className="root-picker-close-button"
                commandId="gitWorktreeAdd.close"
                icon={X}
                label="Close"
                onClick={close}
              />
            </div>
          </div>
          <div className="root-picker-current root-picker-context">
            {options?.git.rootLabel ?? "Loading Git workspace"}
          </div>
          <form className="git-worktree-form" onSubmit={submit}>
            <label className="git-worktree-field">
              <span className="section-label">Worktree name</span>
              <input
                className="root-picker-input"
                autoComplete="off"
                value={worktreeName}
                onChange={(event) => setWorktreeName(event.target.value)}
                placeholder="feature-name"
              />
            </label>
            <fieldset className="git-worktree-fieldset">
              <legend className="section-label">Branch</legend>
              <div className="git-worktree-radio-grid">
                <label>
                  <input
                    type="radio"
                    checked={branchMode === "auto"}
                    onChange={() => setBranchMode("auto")}
                  />{" "}
                  Auto from name
                </label>
                <label>
                  <input
                    type="radio"
                    checked={branchMode === "manual"}
                    onChange={() => setBranchMode("manual")}
                  />{" "}
                  Existing/manual
                </label>
              </div>
              {branchMode === "auto" ? (
                <input
                  className="root-picker-input git-worktree-derived-input"
                  readOnly
                  value={autoBranchDisplay}
                  placeholder="derived from worktree name"
                />
              ) : (
                <label
                  className="git-worktree-select-wrap"
                  aria-label="Existing or manual branch"
                >
                  <select
                    className="root-picker-input git-worktree-select"
                    value={manualBranch}
                    onChange={(event) => setManualBranch(event.target.value)}
                  >
                    <option value="">Select or type below…</option>
                    {manualBranchOptions.map((branch) => (
                      <option key={branch.name} value={branch.name}>
                        {branch.name}
                        {branch.current ? " (current)" : ""}
                      </option>
                    ))}
                  </select>
                  <input
                    className="root-picker-input"
                    value={manualBranch}
                    onChange={(event) => setManualBranch(event.target.value)}
                    placeholder="or type branch-name"
                  />
                </label>
              )}
            </fieldset>
            <fieldset className="git-worktree-fieldset">
              <legend className="section-label">Path</legend>
              <div className="git-worktree-radio-grid">
                <label>
                  <input
                    type="radio"
                    checked={pathMode === "auto"}
                    onChange={() => setPathMode("auto")}
                  />{" "}
                  Auto path
                </label>
                <label>
                  <input
                    type="radio"
                    checked={pathMode === "custom"}
                    onChange={() => setPathMode("custom")}
                  />{" "}
                  Custom path
                </label>
              </div>
              <input
                className="root-picker-input"
                readOnly={pathMode === "auto"}
                value={pathMode === "auto" ? autoPathDisplay : customPath}
                onChange={(event) => setCustomPath(event.target.value)}
                placeholder={
                  pathMode === "auto"
                    ? "derived from worktree name"
                    : "/path/to/worktree"
                }
              />
            </fieldset>
            {options && !options.git.available ? (
              <InlineNotice
                tone="error"
                title="Git unavailable"
                detail={options.git.reason ?? "workspace is not Git-capable"}
              />
            ) : null}
            {preview ? (
              <div
                className={`git-worktree-preview git-worktree-preview-${severity}`}
                role="status"
              >
                <strong>{preview.message}</strong>
                <span>
                  {preview.branchName
                    ? `Branch: ${preview.branchName}`
                    : "Branch pending"}
                </span>
                <span>
                  {preview.targetPathLabel
                    ? `Target: ${preview.targetPathLabel}`
                    : "Target pending"}
                </span>
                {preview.blockers.map((blocker) => (
                  <span key={`${blocker.code}:${blocker.field ?? ""}`}>
                    {blocker.message}
                  </span>
                ))}
              </div>
            ) : loading ? (
              <InlineNotice
                tone="info"
                title="Loading"
                detail="Git worktree options"
              />
            ) : null}
            {error ? (
              <InlineNotice tone="error" title="Add worktree" detail={error} />
            ) : null}
            <div className="root-picker-footer-actions">
              <button
                className="action-button action-button-primary"
                data-command-id="gitWorktreeAdd.submit"
                disabled={submitDisabled}
                type="submit"
              >
                {submitting ? "Creating" : "Create worktree"}
              </button>
              <button
                className="action-button"
                data-command-id="gitWorktreeAdd.close"
                type="button"
                onClick={close}
              >
                Cancel
              </button>
            </div>
          </form>
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
