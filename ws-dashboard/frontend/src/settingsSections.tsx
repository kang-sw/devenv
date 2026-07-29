import { createContext, useContext, useEffect, useState } from "react";
import { AlertTriangle, Check, Download, RefreshCw } from "lucide-react";
import type { SettingsSectionDescriptor } from "./settingsStore.js";
import {
  DEFAULT_TERMINAL_STYLE_PREFS,
  parseTerminalFontSizeInput,
  TERMINAL_FONT_SUGGESTIONS,
  type TerminalStylePrefs,
} from "./terminalPrefs.js";
import {
  requestDaemonBuildInfo,
  requestDaemonShutdown,
  type DaemonBuildInfo,
} from "./resourceRefresh.js";
import { killAllTerminals } from "./terminals.js";
import {
  DOWNLOADABLE_FONTS,
  downloadFont,
  FontDownloadError,
  loadDownloadedFontIds,
  saveDownloadedFontIds,
  type DownloadableFontEntry,
  type FontDownloadReason,
} from "./downloadableFonts.js";

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

// Per-entry download state for the downloadable-fonts row list below the
// Font family field: `"idle"` (not yet fetched), `"loading"` (fetch in
// flight), `"done"` (registered and ready to apply), or `{ error }` (the last
// attempt failed, with the reason for the tooltip).
type DownloadableFontStatus =
  | "idle"
  | "loading"
  | "done"
  | { error: FontDownloadReason };

function downloadErrorTitle(reason: FontDownloadReason): string {
  return reason === "network"
    ? "Could not reach the font source"
    : "Font file failed to load";
}

// Terminal-style settings section. Takes NO props: it reads the live prefs and
// its write path from `SettingsTerminalContext`, so the settings shell can
// render it generically as a bare `SettingsSectionDescriptor.Component` with no
// Terminal-typed props threaded through the shell.
export function TerminalStyleSection() {
  const { prefs, onChange } = useContext(SettingsTerminalContext);
  const [fontStatus, setFontStatus] = useState<
    Record<string, DownloadableFontStatus>
  >(() => {
    const downloadedIds = new Set(loadDownloadedFontIds());
    const initial: Record<string, DownloadableFontStatus> = {};
    for (const entry of DOWNLOADABLE_FONTS) {
      initial[entry.id] = downloadedIds.has(entry.id) ? "done" : "idle";
    }
    return initial;
  });

  const handleDownloadableFontClick = (entry: DownloadableFontEntry) => {
    const status = fontStatus[entry.id] ?? "idle";
    if (status === "loading") {
      return;
    }
    if (status === "done") {
      onChange({ ...prefs, fontFamilyOverride: entry.googleFontsFamily });
      return;
    }
    setFontStatus((prev) => ({ ...prev, [entry.id]: "loading" }));
    downloadFont(entry)
      .then(() => {
        setFontStatus((prev) => {
          const next: Record<string, DownloadableFontStatus> = {
            ...prev,
            [entry.id]: "done",
          };
          const doneIds = DOWNLOADABLE_FONTS.filter(
            (candidate) => next[candidate.id] === "done",
          ).map((candidate) => candidate.id);
          saveDownloadedFontIds(doneIds);
          return next;
        });
      })
      .catch((error: unknown) => {
        const reason: FontDownloadReason =
          error instanceof FontDownloadError ? error.reason : "load-failed";
        setFontStatus((prev) => ({ ...prev, [entry.id]: { error: reason } }));
      });
  };

  return (
    <div className="settings-field-group">
      <label className="settings-field settings-field-checkbox">
        <input
          type="checkbox"
          checked={prefs.ligaturesEnabled}
          onChange={(event) =>
            onChange({ ...prefs, ligaturesEnabled: event.target.checked })
          }
        />
        <span className="settings-field-label">
          Font ligatures (experimental)
        </span>
      </label>
      <label
        className="settings-field settings-field-checkbox"
        title={
          prefs.ligaturesEnabled
            ? "Disabled while font ligatures are enabled — GPU renderers can't render ligature glyphs"
            : undefined
        }
      >
        <input
          type="checkbox"
          checked={prefs.gpuAcceleration && !prefs.ligaturesEnabled}
          disabled={prefs.ligaturesEnabled}
          onChange={(event) =>
            onChange({ ...prefs, gpuAcceleration: event.target.checked })
          }
        />
        <span className="settings-field-label">GPU acceleration</span>
      </label>
      <span className="settings-advanced-muted">
        Applies to newly opened terminal panes.
      </span>
      <label className="settings-field">
        <span className="settings-field-label">Font family</span>
        <input
          className="root-picker-input"
          list="terminal-font-suggestions"
          placeholder="System default (Nerd Font stack)"
          spellCheck={false}
          type="text"
          value={prefs.fontFamilyOverride}
          onChange={(event) =>
            onChange({ ...prefs, fontFamilyOverride: event.target.value })
          }
        />
        <datalist id="terminal-font-suggestions">
          {TERMINAL_FONT_SUGGESTIONS.map((font) => (
            <option key={font} value={font} />
          ))}
        </datalist>
      </label>
      <div className="settings-field">
        <span className="settings-field-label">Downloadable fonts</span>
        <div className="settings-downloadable-fonts">
          {DOWNLOADABLE_FONTS.map((entry) => {
            const status = fontStatus[entry.id] ?? "idle";
            const errored = typeof status === "object";
            return (
              <button
                key={entry.id}
                className="settings-downloadable-font-row"
                disabled={status === "loading"}
                title={errored ? downloadErrorTitle(status.error) : undefined}
                type="button"
                onClick={() => handleDownloadableFontClick(entry)}
              >
                <span className="settings-downloadable-font-label">
                  {entry.label}
                </span>
                {status === "idle" ? (
                  <Download
                    className="settings-downloadable-font-icon"
                    size={13}
                  />
                ) : status === "loading" ? (
                  <RefreshCw
                    className="settings-downloadable-font-icon git-spinner"
                    size={13}
                  />
                ) : status === "done" ? (
                  <Check
                    className="settings-downloadable-font-icon"
                    size={13}
                  />
                ) : (
                  <AlertTriangle
                    className="settings-downloadable-font-icon"
                    size={13}
                  />
                )}
              </button>
            );
          })}
        </div>
      </div>
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
// surprising the user after a click does nothing.
//
// Exported so all four (isSecureContext, hasNotificationGlobal, permission)
// states are assertable directly from `settingsSections.test.ts`, with no DOM
// and no real browser - the cheap substitute for a plain-http LAN browser
// gate. Follows the `parseNotificationPrefs`/`loadNotificationPrefs` idiom in
// `notificationPrefs.ts`: an exported pure core plus a thin wrapper that
// supplies the live values.
//
// Order is load-bearing. On Chromium a plain-http LAN page still HAS the
// `Notification` global defined, so `window.isSecureContext` - not
// `typeof Notification` - is what distinguishes "insecure" from "secure but
// denied". Checking `hasNotificationGlobal` second keeps the undefined-global
// branch reachable for the case it actually covers: a SECURE context whose
// browser lacks the API entirely (iOS Safari < 16.4 over HTTPS, embedded
// webviews) - this is a reorder, never a swap.
export function notificationAvailability(
  isSecureContext: boolean,
  hasNotificationGlobal: boolean,
  permission: NotificationPermission,
): string {
  if (!isSecureContext) {
    return "unavailable - this page is not a secure context";
  }
  if (!hasNotificationGlobal) {
    return "unavailable in this browser";
  }
  return permission;
}

function currentNotificationAvailability(): string {
  const hasNotificationGlobal = typeof Notification !== "undefined";
  return notificationAvailability(
    window.isSecureContext,
    hasNotificationGlobal,
    hasNotificationGlobal ? Notification.permission : "default",
  );
}

// Notifications settings section. Takes NO props, same reasoning as
// `TerminalStyleSection`: it reads the live pref and its write path from
// `SettingsNotificationContext`.
//
// CONTRACT (ticket hard requirement): `Notification.requestPermission()` is
// called ONLY from this checkbox's own `onChange` handler below - a real user
// gesture - never from a mount-time effect (contrast case: `main.tsx`'s `sw.js`
// registration, the one existing "act automatically on page load" precedent
// this section must NOT mirror). Availability is decided by
// `window.isSecureContext` FIRST and `typeof Notification` second: on a
// plain-http LAN page Chromium still defines the global, so only the
// secure-context flag separates "insecure" from "secure but denied". On an
// insecure context the checkbox is disabled outright - no click there can ever
// change the permission, and a disabled `<input>` never fires `onChange`, so
// the `requestPermission()` path below becomes naturally unreachable without
// an extra guard inside the handler.
export function NotificationSection() {
  const { enabled, onChange } = useContext(SettingsNotificationContext);
  const insecureContext = !window.isSecureContext;
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
          disabled={insecureContext}
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
        {insecureContext ? " The toggle above is disabled here." : ""}
      </p>
    </div>
  );
}

function formatBuildTime(secs: number | null): string {
  if (secs == null) {
    return "unknown";
  }
  return new Date(secs * 1000).toLocaleString();
}

// A destructive action guarded by an inline arm/confirm step, so the settings
// nesting (Advanced section) is not the only thing standing between a stray
// click and an irreversible teardown.
function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
}: {
  label: string;
  confirmLabel: string;
  onConfirm: () => void;
}) {
  const [armed, setArmed] = useState(false);
  if (!armed) {
    return (
      <button
        className="settings-danger-button"
        type="button"
        onClick={() => setArmed(true)}
      >
        {label}
      </button>
    );
  }
  return (
    <div className="settings-danger-confirm">
      <span className="settings-danger-confirm-text">{confirmLabel}</span>
      <button
        className="settings-danger-button settings-danger-button-armed"
        type="button"
        onClick={() => {
          setArmed(false);
          onConfirm();
        }}
      >
        Confirm
      </button>
      <button
        className="settings-ghost-button"
        type="button"
        onClick={() => setArmed(false)}
      >
        Cancel
      </button>
    </div>
  );
}

// Advanced settings: build provenance at the top (so a stale Windows dogfood
// binary/bundle is visible at a glance), destructive daemon controls at the
// bottom. The two controls mirror the two OS-level kill modes - a
// terminal-preserving daemon stop vs a full terminal+helper teardown - as
// explicit, guarded UI actions so operators never reach for taskkill.
// See ticket 260725-feat-dashboard-graceful-shutdown-from-settings.
export function AdvancedSection() {
  const [buildInfo, setBuildInfo] = useState<DaemonBuildInfo | null>(null);
  const [buildError, setBuildError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    requestDaemonBuildInfo()
      .then((info) => {
        if (alive) {
          setBuildInfo(info);
        }
      })
      .catch((error: unknown) => {
        if (alive) {
          setBuildError(error instanceof Error ? error.message : "request failed");
        }
      });
    return () => {
      alive = false;
    };
  }, []);

  const onShutdown = () => {
    setStatus("Dashboard is shutting down. Relaunch the daemon to reconnect.");
    void requestDaemonShutdown();
  };
  const onKillAll = () => {
    killAllTerminals()
      .then((closed) =>
        setStatus(
          `Closed ${closed} terminal${closed === 1 ? "" : "s"} (helpers included).`,
        ),
      )
      .catch((error: unknown) =>
        setStatus(
          error instanceof Error ? error.message : "Failed to close terminals.",
        ),
      );
  };

  return (
    <div className="settings-advanced">
      <div className="settings-field">
        <span className="settings-field-label">Build information</span>
        {buildError ? (
          <span className="settings-advanced-muted">
            Unavailable: {buildError}
          </span>
        ) : buildInfo ? (
          <dl className="settings-buildinfo">
            <div className="settings-buildinfo-row">
              <dt>Version</dt>
              <dd>{buildInfo.version}</dd>
            </div>
            <div className="settings-buildinfo-row">
              <dt>Daemon binary</dt>
              <dd>{formatBuildTime(buildInfo.daemonBuildUnixSecs)}</dd>
            </div>
            <div className="settings-buildinfo-row">
              <dt>Frontend bundle</dt>
              <dd>{formatBuildTime(buildInfo.frontendBuildUnixSecs)}</dd>
            </div>
          </dl>
        ) : (
          <span className="settings-advanced-muted">Loading…</span>
        )}
      </div>

      <div className="settings-advanced-danger">
        <span className="settings-field-label">Danger zone</span>
        <div className="settings-advanced-danger-row">
          <div className="settings-advanced-danger-copy">
            <strong>Shut down dashboard</strong>
            <span className="settings-advanced-muted">
              Stops the daemon. Open terminals keep running and reattach when the
              daemon is relaunched.
            </span>
          </div>
          <ConfirmButton
            confirmLabel="Shut down the dashboard daemon?"
            label="Shut down"
            onConfirm={onShutdown}
          />
        </div>
        <div className="settings-advanced-danger-row">
          <div className="settings-advanced-danger-copy">
            <strong>Close all terminal sessions</strong>
            <span className="settings-advanced-muted">
              Terminates every terminal and its helper process. This cannot be
              undone.
            </span>
          </div>
          <ConfirmButton
            confirmLabel="Close every terminal and helper?"
            label="Close all"
            onConfirm={onKillAll}
          />
        </div>
        {status ? (
          <div className="settings-advanced-status">{status}</div>
        ) : null}
      </div>
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
  { id: "advanced", title: "Advanced", Component: AdvancedSection },
];
