import { useState } from "react";
import { X } from "lucide-react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import {
  buildSettingsCloseCommand,
  type DashboardCommandDispatcher,
} from "./commands.js";
import type { SettingsSectionDescriptor } from "./settingsStore.js";
import { ChromeIconButton } from "./chrome.js";

export function SettingsModal({
  open,
  sections,
  onCommand,
  onClose,
}: {
  open: boolean;
  sections: readonly SettingsSectionDescriptor[];
  onCommand: DashboardCommandDispatcher;
  onClose: () => void;
}) {
  // The shell is section-agnostic: it receives the section list as an injected
  // prop and only ever consumes `{ id, title, Component }`. It threads no
  // section-specific (e.g. Terminal-typed) props, so registering a new section
  // means appending a descriptor to `SETTINGS_SECTIONS` - each section sources
  // its own state from its own context - with no change to this component.
  const [activeSectionId, setActiveSectionId] = useState<string | undefined>(
    () => sections[0]?.id,
  );

  if (!open) {
    return null;
  }

  const close = () => {
    onCommand(buildSettingsCloseCommand(), { "settings.close": onClose });
  };

  const activeSection =
    sections.find((section) => section.id === activeSectionId) ?? sections[0];

  return (
    <ModalOverlay
      className="root-picker-backdrop"
      isDismissable
      isOpen
      onOpenChange={(isOpen) => {
        if (!isOpen) close();
      }}
    >
      <Modal className="root-picker-modal settings-modal">
        <Dialog aria-label="Settings" className="root-picker-dialog">
          <div className="root-picker-titlebar">
            <Heading className="root-picker-title" slot="title">
              Settings
            </Heading>
            <div className="root-picker-window-actions">
              <ChromeIconButton
                className="root-picker-close-button"
                commandId="settings.close"
                icon={X}
                label="Close"
                onClick={close}
              />
            </div>
          </div>
          <div className="root-picker-content">
            <nav
              className="root-picker-places settings-section-nav"
              aria-label="Settings sections"
            >
              {sections.map((section) => {
                const isActive = section.id === activeSection?.id;
                return (
                  <button
                    key={section.id}
                    aria-current={isActive ? "true" : undefined}
                    className={`root-picker-place settings-section-nav-button${
                      isActive ? " settings-section-nav-button-active" : ""
                    }`}
                    type="button"
                    onClick={() => setActiveSectionId(section.id)}
                  >
                    {section.title}
                  </button>
                );
              })}
            </nav>
            <div className="root-picker-list-region settings-section-body">
              {activeSection ? <activeSection.Component /> : null}
            </div>
          </div>
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
