import { createContext, useContext } from "react";
import type { SettingsSectionDescriptor } from "./settingsStore.js";
import {
  DEFAULT_TERMINAL_STYLE_PREFS,
  parseTerminalFontSizeInput,
  type TerminalStylePrefs,
} from "./terminalPrefs.js";

// Settings-scoped Terminal context. Unlike the read-only `TerminalPrefsContext`
// in `App.tsx` (consumed by every open `TerminalPaneBody` to live-restyle its
// emulator), this context also carries the setter, so the Terminal settings
// section reads BOTH the current prefs and its single write path from context
// rather than from props threaded through the modal shell. Keeping the section
// prop-free is what lets its `Component` be a stable module-scope function
// (never an inline arrow rebuilt on every App re-render); that stable identity
// is what stops the section's <input>s from unmounting/remounting - and losing
// focus - on every keystroke.
export type SettingsTerminalContextValue = {
  readonly prefs: TerminalStylePrefs;
  readonly onChange: (next: TerminalStylePrefs) => void;
};

export const SettingsTerminalContext =
  createContext<SettingsTerminalContextValue>({
    prefs: DEFAULT_TERMINAL_STYLE_PREFS,
    onChange: () => {},
  });

// Terminal-style settings section. Takes NO props: it reads the live prefs and
// its write path from `SettingsTerminalContext`, so the settings shell can
// render it generically as a bare `SettingsSectionDescriptor.Component` with no
// Terminal-typed props threaded through the shell.
export function TerminalStyleSection() {
  const { prefs, onChange } = useContext(SettingsTerminalContext);
  return (
    <div className="settings-field-group">
      <label className="settings-field">
        <span className="settings-field-label">Font family</span>
        <input
          className="root-picker-input"
          placeholder="System default (Nerd Font stack)"
          spellCheck={false}
          type="text"
          value={prefs.fontFamilyOverride}
          onChange={(event) =>
            onChange({ ...prefs, fontFamilyOverride: event.target.value })
          }
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">Font size</span>
        <input
          className="root-picker-input"
          max={32}
          min={8}
          type="number"
          value={prefs.fontSize}
          onChange={(event) => {
            const nextSize = parseTerminalFontSizeInput(event.target.value);
            if (nextSize !== null) {
              onChange({ ...prefs, fontSize: nextSize });
            }
          }}
        />
      </label>
      <label className="settings-field">
        <span className="settings-field-label">Background color</span>
        <input
          className="root-picker-input"
          placeholder="#0b0d10"
          spellCheck={false}
          type="text"
          value={prefs.themeBackground}
          onChange={(event) =>
            onChange({ ...prefs, themeBackground: event.target.value })
          }
        />
      </label>
    </div>
  );
}

// Module-scope section registry: a stable, ordered list of descriptors with
// stable `Component` identities. The Settings modal shell receives this as an
// injected `sections` prop and iterates it generically - it only ever consumes
// `{ id, title, Component }` and has no Terminal-typed knowledge. A future
// section (e.g. a hotkey-rebind editor) registers by appending a descriptor
// here, supplying its own context for any lifted state, without editing the
// shell.
export const SETTINGS_SECTIONS: readonly SettingsSectionDescriptor[] = [
  { id: "terminal", title: "Terminal", Component: TerminalStyleSection },
];
