import { createContext, useContext, useEffect, useState } from "react";
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
  { id: "advanced", title: "Advanced", Component: AdvancedSection },
];
