import { useEffect, useState } from "react";
import { Eye, Languages, Pencil, RotateCcw, Save } from "lucide-react";
import {
  DocumentViewer,
  buildDocumentTranslationRequestPayload,
  fetchTranslationProviders,
  overlayFromTranslationResponse,
  requestDocumentTranslation,
  type DocumentTranslationOverlay,
} from "./documentViewer.js";
import { DocumentRawEditor } from "./documentRawEditor.js";
import {
  buildDocumentModeSetCommand,
  buildDocumentRevertCommand,
  buildDocumentSaveCommand,
  buildDocumentTranslationToggleCommand,
  type DashboardCommandDispatcher,
} from "./commands.js";
import {
  documentDraftContentChangeDecision,
  documentSaveStateForError,
  writeWorkRootTextFile,
  type DocumentSaveState,
  type ReadOnlyFilePane,
} from "./workRootFiles.js";
import type { WorkRootView } from "./resourceModel.js";

export function ReadOnlyDocumentPane({
  pane,
  root,
  renderMarkdown,
  onCommand,
  onDocumentSaved,
}: {
  pane: ReadOnlyFilePane;
  root: WorkRootView;
  renderMarkdown: boolean;
  onCommand: DashboardCommandDispatcher;
  onDocumentSaved: (source: {
    serverRoute?: string;
    workRootId: string;
    path: string;
    content: string;
    contentHash: string;
    sizeBytes: number;
  }) => void;
}) {
  const [translationEnabled, setTranslationEnabled] = useState(false);
  const [translationStatus, setTranslationStatus] = useState<
    "idle" | "loading" | "ready" | "unavailable" | "error"
  >("idle");
  const [translationMessage, setTranslationMessage] = useState<string | null>(
    null,
  );
  const [translationOverlay, setTranslationOverlay] = useState<
    DocumentTranslationOverlay | undefined
  >();
  const [documentMode, setDocumentMode] = useState<"view" | "edit">("view");
  const [draft, setDraft] = useState(pane.content);
  const [baseContentHash, setBaseContentHash] = useState(pane.contentHash);
  const [saveState, setSaveState] = useState<DocumentSaveState>("idle");
  const [saveMessage, setSaveMessage] = useState<string | null>(null);

  useEffect(() => {
    const decision = documentDraftContentChangeDecision(saveState);
    if (decision.action === "preserveDraft") {
      setSaveState(decision.saveState);
      setSaveMessage(decision.message);
      return;
    }
    setDraft(pane.content);
    setBaseContentHash(pane.contentHash);
    setTranslationOverlay(undefined);
  }, [pane.content, pane.contentHash]);

  const setModeCommand = (mode: "view" | "edit") => {
    const command = buildDocumentModeSetCommand(
      pane.workRootId,
      pane.path,
      mode,
      pane.serverRoute,
    );
    onCommand(command, { [command.commandId]: () => setDocumentMode(mode) });
  };

  const revertDraft = () => {
    const command = buildDocumentRevertCommand(
      pane.workRootId,
      pane.path,
      pane.serverRoute,
    );
    onCommand(command, {
      [command.commandId]: () => {
        setDraft(pane.content);
        setBaseContentHash(pane.contentHash);
        setSaveState("idle");
        setSaveMessage(null);
      },
    });
  };

  const saveDraft = () => {
    const command = buildDocumentSaveCommand(
      pane.workRootId,
      pane.path,
      pane.serverRoute,
    );
    onCommand(command, {
      [command.commandId]: () => {
        if (!baseContentHash) {
          setSaveState("error");
          setSaveMessage("Missing base content hash");
          return;
        }
        setSaveState("saving");
        setSaveMessage("Saving");
        void writeWorkRootTextFile(
          pane.workRootId,
          {
            path: pane.path,
            baseContentHash,
            content: draft,
          },
          pane.serverRoute,
        )
          .then((response) => {
            setBaseContentHash(response.contentHash);
            setSaveState("saved");
            setSaveMessage("Saved");
            setTranslationOverlay(undefined);
            onDocumentSaved({
              serverRoute: pane.serverRoute,
              workRootId: pane.workRootId,
              path: pane.path,
              content: draft,
              contentHash: response.contentHash,
              sizeBytes: response.sizeBytes,
            });
          })
          .catch((error) => {
            const message =
              error instanceof Error ? error.message : "Save failed";
            setSaveState(documentSaveStateForError(message));
            setSaveMessage(message);
          });
      },
    });
  };

  useEffect(() => {
    if (!renderMarkdown || !translationEnabled || pane.status !== "loaded") {
      return;
    }
    let cancelled = false;
    setTranslationStatus("loading");
    setTranslationMessage("Requesting document translation");
    const payload = buildDocumentTranslationRequestPayload({
      markdown: pane.content,
      workRootId: pane.workRootId,
      path: pane.path,
      title: pane.title,
      targetLocale: "ko",
    });
    void fetchTranslationProviders()
      .then((providers) => {
        const provider = providers.providers.find(
          (candidate) => candidate.configured,
        );
        if (!provider) {
          throw new Error("No translation provider configured");
        }
        return requestDocumentTranslation({
          ...payload,
          provider: {
            id: provider.id,
            model: provider.defaultModel ?? provider.models[0]?.id,
          },
        });
      })
      .then((response) => {
        if (cancelled) {
          return;
        }
        setTranslationOverlay(overlayFromTranslationResponse(response));
        setTranslationStatus(response.status === "failed" ? "error" : "ready");
        setTranslationMessage(
          response.status === "completed"
            ? `Translated to ${response.targetLocale}`
            : `Translation ${response.status}`,
        );
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        setTranslationOverlay(undefined);
        setTranslationStatus("unavailable");
        setTranslationMessage(
          error instanceof Error ? error.message : "Translation unavailable",
        );
      });
    return () => {
      cancelled = true;
    };
  }, [
    pane.content,
    pane.path,
    pane.status,
    pane.title,
    pane.workRootId,
    pane.serverRoute,
    renderMarkdown,
    translationEnabled,
  ]);

  const documentFormatLabel = renderMarkdown
    ? "markdown"
    : (pane.languageHint ?? pane.extension ?? "text");
  const translationButtonLabel = translationEnabled
    ? "Disable Korean translation"
    : "Enable Korean translation";
  const translationStatusVisible =
    translationStatus === "loading" ||
    translationStatus === "ready" ||
    translationStatus === "unavailable" ||
    translationStatus === "error";
  const documentPathLabel = pane.path;
  const documentPathTitle = pane.path.startsWith("/")
    ? pane.path
    : `${root.label} / ${pane.path}`;
  const saveStatusLabel =
    saveState === "idle"
      ? draft === pane.content
        ? "clean"
        : "dirty"
      : saveState;
  const showSaveStatusChip =
    documentMode === "edit" && pane.status === "loaded";

  return (
    <div className="readonly-text-pane document-pane ws-pane">
      <div className="readonly-text-pane-header readonly-text-pane-ribbon ws-toolbar">
        <div className="document-ribbon-file">
          <div className="readonly-text-pane-path" title={documentPathTitle}>
            {documentPathLabel}
          </div>
          <div className="readonly-text-pane-badges">
            <span className="meta-chip ws-chip">{pane.mode}</span>
            <span className="meta-chip ws-chip">{documentFormatLabel}</span>
            {showSaveStatusChip ? (
              <span
                className={`meta-chip ws-chip document-save-chip document-save-chip-${saveStatusLabel}`}
                data-document-save-state={saveState}
                title={saveMessage ?? saveStatusLabel}
              >
                {saveStatusLabel}
              </span>
            ) : null}
          </div>
        </div>
        <div className="document-ribbon-controls">
          {pane.status === "loaded" ? (
            <div
              className="document-viewer-segmented"
              role="group"
              aria-label="Document mode"
            >
              <button
                type="button"
                className={`document-viewer-segment${documentMode === "view" ? " is-active" : ""}`}
                aria-label="View document"
                title="View"
                data-command-id="document.mode.set"
                data-document-mode="view"
                onClick={() => setModeCommand("view")}
              >
                <Eye aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
              <button
                type="button"
                className={`document-viewer-segment${documentMode === "edit" ? " is-active" : ""}`}
                aria-label="Edit document"
                title="Edit"
                data-command-id="document.mode.set"
                data-document-mode="edit"
                onClick={() => setModeCommand("edit")}
              >
                <Pencil aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
            </div>
          ) : null}
          {pane.status === "loaded" &&
          renderMarkdown &&
          documentMode === "view" ? (
            <button
              type="button"
              className={`document-translation-toggle${translationEnabled ? " is-active" : ""}`}
              aria-label={translationButtonLabel}
              aria-pressed={translationEnabled}
              title={`${translationButtonLabel}; target: Korean`}
              data-command-id="document.translation.toggle"
              onClick={() => {
                const command = buildDocumentTranslationToggleCommand(
                  pane.workRootId,
                  pane.path,
                  pane.serverRoute,
                );
                onCommand(command, {
                  [command.commandId]: () => {
                    setTranslationEnabled((current) => !current);
                    setTranslationOverlay(undefined);
                    setTranslationStatus("idle");
                    setTranslationMessage(null);
                  },
                });
              }}
            >
              <Languages aria-hidden="true" size={13} strokeWidth={1.8} />
            </button>
          ) : null}
          {translationStatusVisible ? (
            <span
              className="document-translation-status"
              data-translation-status={translationStatus}
            >
              {translationMessage ?? translationStatus}
            </span>
          ) : null}
          {documentMode === "edit" && pane.status === "loaded" ? (
            <div className="document-edit-actions">
              <button
                type="button"
                className="icon-button document-edit-icon-button"
                data-command-id="document.save"
                disabled={saveState === "saving" || draft === pane.content}
                title="Save"
                aria-label="Save document"
                onClick={saveDraft}
              >
                <Save aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
              <button
                type="button"
                className="icon-button document-edit-icon-button"
                data-command-id="document.revert"
                disabled={saveState === "saving" || draft === pane.content}
                title="Revert"
                aria-label="Revert document draft"
                onClick={revertDraft}
              >
                <RotateCcw aria-hidden="true" size={13} strokeWidth={1.8} />
              </button>
            </div>
          ) : null}
        </div>
      </div>
      {pane.status === "loading" ? (
        <div className="readonly-text-pane-state ws-state-surface">
          Loading file content
        </div>
      ) : pane.status === "error" ? (
        <div className="readonly-text-pane-state readonly-text-pane-error ws-state-surface">
          {pane.error ?? "file read failed"}
        </div>
      ) : (
        <>
          {documentMode === "edit" ? (
            <DocumentRawEditor
              value={draft}
              source={pane}
              ariaLabel={`Raw editor for ${pane.path}`}
              onChange={(nextDraft) => {
                setDraft(nextDraft);
                setSaveState("dirty");
                setSaveMessage("Unsaved changes");
              }}
            />
          ) : renderMarkdown ? (
            <DocumentViewer
              markdown={pane.content}
              path={pane.path}
              overlay={translationOverlay}
            />
          ) : (
            <DocumentRawEditor
              value={pane.content}
              source={pane}
              ariaLabel={`Read-only source viewer for ${pane.path}`}
              editable={false}
            />
          )}
        </>
      )}
    </div>
  );
}
