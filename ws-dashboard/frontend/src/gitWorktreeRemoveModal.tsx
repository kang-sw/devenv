import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import { X } from "lucide-react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import {
  buildWorktreeRemoveCloseCommand,
  buildWorktreeRemoveSubmitCommand,
  type DashboardCommandDispatcher,
} from "./commands.js";
import {
  fetchGitWorktreeRemovePreview,
  submitGitWorktreeRemove,
  GitWorktreeRemoveSubmitError,
  type GitWorktreeRemovePreview,
} from "./gitWorktreeRemove.js";
import type { DashboardResourcesView } from "./resourceModel.js";
import { ChromeIconButton, InlineNotice } from "./chrome.js";

// 260525 Phase 3 B-1/B-2: worktree removal ALWAYS opens this real confirmation
// modal (never a bare `window.confirm` — worktree add/remove is heavy
// regardless of dirty state). A red data-loss banner is shown ONLY when the
// preview reports uncommitted/untracked changes; a default-OFF "delete branch"
// checkbox shows a red parenthetical when the branch is unmerged, and even if
// checked the daemon refuses to force-delete it.
export function GitWorktreeRemoveModal({
  target,
  onCommand,
  onClose,
  onRemoved,
}: {
  target: { serverRoute: string; workRootId: string } | null;
  onCommand: DashboardCommandDispatcher;
  onClose: () => void;
  onRemoved: (
    response: {
      resources: DashboardResourcesView;
      removedWorkRootId?: string;
      branchDeleted: boolean;
      branchDeleteSkippedUnmerged: boolean;
    },
    context: { workRootId: string; serverRoute: string },
  ) => void;
}) {
  const workRootId = target?.workRootId ?? null;
  const serverRoute = target?.serverRoute ?? null;
  const [preview, setPreview] = useState<GitWorktreeRemovePreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [deleteBranch, setDeleteBranch] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refreshPreview = useCallback(() => {
    if (!workRootId || !serverRoute) {
      return;
    }
    setLoading(true);
    void fetchGitWorktreeRemovePreview(workRootId, serverRoute)
      .then(setPreview)
      .catch((nextError) =>
        setError(
          nextError instanceof Error
            ? nextError.message
            : "worktree remove preview failed",
        ),
      )
      .finally(() => setLoading(false));
  }, [serverRoute, workRootId]);

  useEffect(() => {
    if (!workRootId || !serverRoute) {
      setPreview(null);
      setDeleteBranch(false);
      setError(null);
      return;
    }
    setPreview(null);
    setDeleteBranch(false);
    setError(null);
    refreshPreview();
  }, [refreshPreview, serverRoute, workRootId]);

  if (!workRootId || !serverRoute) {
    return null;
  }

  const close = () => {
    onCommand(buildWorktreeRemoveCloseCommand(workRootId, serverRoute), {
      "worktreeRemove.close": onClose,
    });
  };

  const submitDisabled =
    submitting || loading || !preview || preview.available === false;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!preview || submitDisabled) {
      return;
    }
    onCommand(buildWorktreeRemoveSubmitCommand(workRootId, serverRoute), {
      "worktreeRemove.submit": () => {
        setSubmitting(true);
        setError(null);
        // The confirmation modal itself IS the force confirmation: submitting
        // after seeing the red data-loss banner authorizes `--force`.
        void submitGitWorktreeRemove(
          workRootId,
          { deleteBranch, force: preview.hasUncommittedChanges },
          serverRoute,
        )
          .then((response) => onRemoved(response, { workRootId, serverRoute }))
          .catch((nextError) => {
            setError(
              nextError instanceof Error
                ? nextError.message
                : "worktree remove failed",
            );
            // Force-gate 409: the tree turned dirty after a clean preview.
            // Refresh so the red data-loss banner appears and a deliberate
            // re-submit can authorize `--force`, instead of forcing the owner
            // to close and reopen the modal.
            if (
              nextError instanceof GitWorktreeRemoveSubmitError &&
              nextError.status === 409
            ) {
              refreshPreview();
            }
          })
          .finally(() => setSubmitting(false));
      },
    });
  };

  const branchName = preview?.branchName ?? null;
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
        <Dialog aria-label="Remove Git worktree" className="root-picker-dialog">
          <div className="root-picker-titlebar">
            <Heading className="root-picker-title" slot="title">
              Remove worktree
            </Heading>
            <div className="root-picker-window-actions">
              <ChromeIconButton
                className="root-picker-close-button"
                commandId="worktreeRemove.close"
                icon={X}
                label="Close"
                onClick={close}
              />
            </div>
          </div>
          <div className="root-picker-current root-picker-context">
            {preview?.targetPathLabel ?? "Loading worktree"}
          </div>
          <form className="git-worktree-form" onSubmit={submit}>
            <p className="git-worktree-remove-copy">
              This removes the linked worktree with{" "}
              <code>git worktree remove</code>. The worktree directory will be
              deleted from disk; the repository and other worktrees are
              untouched.
            </p>
            {preview && preview.available === false ? (
              <InlineNotice
                tone="error"
                title="Unavailable"
                detail={preview.reason ?? "worktree cannot be removed"}
              />
            ) : null}
            {preview?.hasUncommittedChanges ? (
              <InlineNotice
                tone="error"
                title="Uncommitted changes will be lost"
                detail={`${preview.modifiedFiles} modified · ${preview.untrackedFiles} untracked file(s) in this worktree will be permanently deleted.`}
              />
            ) : null}
            {branchName ? (
              <label className="git-worktree-remove-branch">
                <input
                  type="checkbox"
                  checked={deleteBranch}
                  onChange={(event) => setDeleteBranch(event.target.checked)}
                />{" "}
                <span>
                  Delete branch <code>{branchName}</code> too
                </span>
                {preview?.branchUnmerged ? (
                  <span className="git-worktree-remove-unmerged">
                    {" "}
                    아직 머지되지 않았습니다 (unmerged — will be kept)
                  </span>
                ) : null}
              </label>
            ) : null}
            {error ? (
              <InlineNotice
                tone="error"
                title="Remove worktree"
                detail={error}
              />
            ) : null}
            <div className="root-picker-footer-actions">
              <button
                className="action-button action-button-danger"
                data-command-id="worktreeRemove.submit"
                disabled={submitDisabled}
                type="submit"
              >
                {submitting ? "Removing" : "Remove worktree"}
              </button>
              <button
                className="action-button"
                data-command-id="worktreeRemove.close"
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
