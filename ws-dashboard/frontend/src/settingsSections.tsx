import { createContext, useContext, useState } from "react";
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

// Settings-scoped Notifications context (260725 Phase 8). Same shape as
// `SettingsTerminalContext`: carries both the current value and its single
// write path, so `NotificationSection` stays a zero-prop component the shell
// can render generically. `enabled` is the ONLY persisted field (see
// `notificationPrefs.ts`) - `Notification.permission` is read live from the
// browser inside the section itself, never lifted into this context, since
// permission is not this app's state to own.
export type SettingsNotificationContextValue = {
  readonly enabled: boolean;
  readonly onChange: (next: boolean) => void;
};

export const SettingsNotificationContext =
  createContext<SettingsNotificationContextValue>({
    enabled: false,
    onChange: () => {},
  });

// `window.isSecureContext` and `Notification.permission` are both readable
// with no permission prompt of their own, so the section can show the actual
// limitation up front (ticket text: "Settings copy stating plainly that
// OS-level notification requires localhost or TLS") rather than only
// surprising the user after a click does nothing. Checked in this order
// because a plain-http LAN page lacks the whole `Notification` global, not
// merely a granted permission - `window.isSecureContext` alone would not
// distinguish "insecure" from "secure but denied".
function currentNotificationAvailability(): string {
  if (typeof Notification === "undefined") {
    return window.isSecureContext
      ? "unavailable in this browser"
      : "unavailable - this page is not a secure context";
  }
  return Notification.permission;
}

// Notifications settings section. Takes NO props, same reasoning as
// `TerminalStyleSection`: it reads the live pref and its write path from
// `SettingsNotificationContext`.
//
// CONTRACT (ticket hard requirement): `Notification.requestPermission()` is
// called ONLY from this checkbox's own `onChange` handler below - a real user
// gesture - never from a mount-time effect (contrast case: `main.tsx`'s `sw.js`
// registration, the one existing "act automatically on page load" precedent
// this section must NOT mirror). Guarded on `typeof Notification ===
// "undefined"` rather than `window.isSecureContext` alone, because a
// plain-http LAN page lacks the whole API, not just permission.
export function NotificationSection() {
  const { enabled, onChange } = useContext(SettingsNotificationContext);
  // Forces a re-render once the permission prompt settles (review cycle 1,
  // Minor 1). `currentNotificationAvailability()` below always reads the
  // LIVE `Notification.permission` at render time - nothing is cached in
  // this state, it exists only to schedule the re-render that would
  // otherwise not happen until some unrelated state change, leaving the note
  // stuck on "Current permission: default" right after the user answers.
  const [, forceRerenderOnPermissionSettled] = useState(0);
  return (
    <div className="settings-field-group">
      <label className="settings-notification-toggle">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(event) => {
            const next = event.target.checked;
            onChange(next);
            if (next && typeof Notification !== "undefined") {
              // `Promise.resolve(...)` wraps the call rather than chaining
              // `.then` directly on its result (review cycle 2, correctness
              // Minor): macOS Safari <= 15 implements only the legacy
              // callback form of `requestPermission` and returns `undefined`,
              // which would make a direct `.then` throw synchronously inside
              // this handler - the same "the Notification API varies per
              // browser" class already handled defensively one commit
              // earlier around the `new Notification(...)` call itself.
              Promise.resolve(Notification.requestPermission())
                .then((permission) => {
                  if (permission === "denied") {
                    // Reconcile the persisted opt-in against a denied
                    // permission (review cycle 1, Minor 2): otherwise the
                    // box stays checked and `{ enabled: true }` stays
                    // persisted for a tier that can never fire.
                    onChange(false);
                  }
                  forceRerenderOnPermissionSettled((count) => count + 1);
                })
                .catch(() => {
                  // No current browser rejects `requestPermission()` instead
                  // of resolving "denied", but leaving the promise unhandled
                  // would produce an unhandled rejection if one ever did
                  // (review cycle 1, Minor 1).
                  forceRerenderOnPermissionSettled((count) => count + 1);
                });
            }
          }}
        />{" "}
        <span>Show an OS notification when an agent needs you</span>
      </label>
      <p className="settings-field-note">
        OS-level notifications require a secure context (localhost or a TLS
        origin) - a plain-http LAN page cannot request or show them. Current
        permission: {currentNotificationAvailability()}.
      </p>
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
  {
    id: "notifications",
    title: "Notifications",
    Component: NotificationSection,
  },
];
