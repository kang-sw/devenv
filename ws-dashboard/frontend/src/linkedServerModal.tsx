import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { X } from "lucide-react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import {
  defaultLinkedServerId,
  linkEndpointServer,
  linkServerPassphrase,
} from "./linkedServers.js";
import {
  isValidServerRouteSegment,
  type ServerConnectionView,
} from "./resourceModel.js";
import { ChromeIconButton, InlineNotice } from "./chrome.js";

export type ServerModalState =
  | { mode: "add" }
  | { mode: "auth"; server: ServerConnectionView };

export function LinkedServerModal({
  state,
  onClose,
  onLinked,
}: {
  state: ServerModalState | null;
  onClose: () => void;
  onLinked: (server: ServerConnectionView) => void;
}) {
  const [label, setLabel] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!state) {
      return;
    }
    setError(null);
    setSubmitting(false);
    setPassphrase("");
    if (state.mode === "add") {
      setLabel("");
      setEndpoint("");
    } else {
      setLabel(state.server.label);
      setEndpoint("");
    }
  }, [state]);

  if (!state) {
    return null;
  }

  const addMode = state.mode === "add";
  const title = addMode ? "Add server" : `Authenticate ${state.server.label}`;
  const submitDisabled =
    submitting ||
    (addMode && (label.trim().length === 0 || endpoint.trim().length === 0)) ||
    (!addMode && passphrase.trim().length === 0);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (submitDisabled) {
      return;
    }
    setError(null);
    const passphraseValue = passphrase.trim();
    let request: Promise<ServerConnectionView>;
    if (addMode) {
      const serverRoute = defaultLinkedServerId(label, endpoint);
      if (!isValidServerRouteSegment(serverRoute)) {
        setError(
          "Server route must contain only letters, digits, hyphen, or underscore (dot is reserved).",
        );
        return;
      }
      request = linkEndpointServer({
        serverId: serverRoute,
        label: label.trim(),
        endpoint: endpoint.trim(),
        ...(passphraseValue ? { passphrase: passphraseValue } : {}),
      });
    } else {
      request = linkServerPassphrase(state.server.id, passphraseValue);
    }
    setSubmitting(true);
    void request
      .then((server) => {
        onLinked(server);
        if (server.status === "connected") {
          onClose();
          return;
        }
        if (addMode && passphraseValue.length === 0) {
          onClose();
          return;
        }
        setError(
          "Passphrase was not accepted; the server is saved and still requires authentication.",
        );
      })
      .catch((nextError) => {
        setError(
          nextError instanceof Error ? nextError.message : "server link failed",
        );
      })
      .finally(() => setSubmitting(false));
  };

  return (
    <ModalOverlay
      className="root-picker-backdrop"
      isDismissable
      isOpen
      onOpenChange={(isOpen) => {
        if (!isOpen) {
          onClose();
        }
      }}
    >
      <Modal className="root-picker-modal linked-server-modal">
        <Dialog aria-label={title} className="root-picker-dialog">
          <div className="root-picker-titlebar">
            <Heading className="root-picker-title" slot="title">
              {title}
            </Heading>
            <div className="root-picker-window-actions">
              <ChromeIconButton
                className="root-picker-close-button"
                commandId="resource.action.server.modal.close"
                icon={X}
                label="Close"
                onClick={onClose}
              />
            </div>
          </div>
          <form className="linked-server-form" onSubmit={submit}>
            {addMode ? (
              <>
                <label className="linked-server-field">
                  <span className="section-label">Name</span>
                  <input
                    className="root-picker-input"
                    autoComplete="off"
                    value={label}
                    onChange={(event) => setLabel(event.target.value)}
                    placeholder="Remote dev"
                  />
                </label>
                <label className="linked-server-field">
                  <span className="section-label">Endpoint</span>
                  <input
                    className="root-picker-input"
                    autoComplete="off"
                    spellCheck={false}
                    value={endpoint}
                    onChange={(event) => setEndpoint(event.target.value)}
                    placeholder="http://127.0.0.1:49170"
                  />
                </label>
                <div className="linked-server-hint">
                  Use an endpoint already reachable from this host, including a
                  loopback tunnel you created outside the dashboard.
                </div>
              </>
            ) : (
              <div className="linked-server-hint">
                Enter the daemon-lifetime passphrase printed by the remote
                dashboard daemon.
              </div>
            )}
            <label className="linked-server-field">
              <span className="section-label">Passphrase</span>
              <input
                className="root-picker-input"
                autoComplete="off"
                spellCheck={false}
                type="password"
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                placeholder={addMode ? "Optional" : "Required"}
              />
            </label>
            {error ? (
              <InlineNotice tone="error" title="Server link" detail={error} />
            ) : null}
            <div className="root-picker-footer-actions">
              <button
                className="action-button action-button-primary"
                data-command-id="resource.action.server.link.submit"
                disabled={submitDisabled}
                type="submit"
              >
                {submitting
                  ? "Connecting"
                  : addMode
                    ? "Connect"
                    : "Authenticate"}
              </button>
              <button
                className="action-button"
                data-command-id="resource.action.server.modal.close"
                type="button"
                onClick={onClose}
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
