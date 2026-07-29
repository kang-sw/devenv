import { createContext } from "react";
import { loadNamespacedPrefs, saveNamespacedPrefs } from "./settingsStore.js";
import { browserStorage } from "./workRootFiles.js";

// The literal fallback stack currently hardcoded at the `Terminal({...})`
// constructor call in `App.tsx`'s terminal mount effect - kept here as the
// single source of truth so `buildEffectiveTerminalFontFamily` can prepend a
// user override onto it without duplicating the string.
export const TERMINAL_FONT_FALLBACK_STACK =
  '"MesloLGS NF", "JetBrainsMono Nerd Font", "CaskaydiaCove Nerd Font", ' +
  '"FiraCode Nerd Font", "Hack Nerd Font", ui-monospace, SFMono-Regular, ' +
  'Menlo, Consolas, "Liberation Mono", monospace';

// Autocomplete suggestions for the font-family override input: the named
// (non-generic) entries from `TERMINAL_FONT_FALLBACK_STACK`, in the same
// order, so a user who wants one of the built-in fallback fonts explicitly
// (e.g. to reorder it ahead of MesloLGS NF for ligature support) doesn't
// have to type the full family name from memory. Keep in sync with the
// fallback stack's named entries if that ever changes.
export const TERMINAL_FONT_SUGGESTIONS = [
  "MesloLGS NF",
  "JetBrainsMono Nerd Font",
  "CaskaydiaCove Nerd Font",
  "FiraCode Nerd Font",
  "Hack Nerd Font",
];

export type TerminalStylePrefs = {
  readonly fontFamilyOverride: string;
  readonly fontSize: number;
  readonly themeBackground: string;
  readonly gpuAcceleration: boolean;
  readonly ligaturesEnabled: boolean;
};

// Reproduces today's exact hardcoded terminal look when nothing is persisted
// yet: empty override (fallback stack unchanged), fontSize 12, background
// "#0b0d10", GPU-accelerated renderer on, ligatures off.
export const DEFAULT_TERMINAL_STYLE_PREFS: TerminalStylePrefs = {
  fontFamilyOverride: "",
  fontSize: 12,
  themeBackground: "#0b0d10",
  gpuAcceleration: true,
  ligaturesEnabled: false,
};

// Live fan-out for terminal-style prefs: open terminal panes subscribe via
// `useContext` and apply `terminal.options.*` on change (see
// `TerminalPaneBody`'s post-mount subscription effect) instead of only
// reading the value at construction time. The default matches
// `DEFAULT_TERMINAL_STYLE_PREFS` so any consumer rendered outside the
// Provider (there should be none) still reproduces today's hardcoded look.
export const TerminalPrefsContext = createContext<TerminalStylePrefs>(
  DEFAULT_TERMINAL_STYLE_PREFS,
);

const terminalStylePrefsStorageKey = "ws-dashboard.settings.terminal.v1";
const terminalStylePrefsVersion = 1;

function parseTerminalStylePrefs(raw: unknown): TerminalStylePrefs | null {
  if (typeof raw !== "object" || raw === null) {
    return null;
  }
  const record = raw as Record<string, unknown>;
  if (typeof record.fontFamilyOverride !== "string") {
    return null;
  }
  if (
    typeof record.fontSize !== "number" ||
    !Number.isFinite(record.fontSize)
  ) {
    return null;
  }
  if (typeof record.themeBackground !== "string") {
    return null;
  }
  return {
    fontFamilyOverride: record.fontFamilyOverride,
    fontSize: record.fontSize,
    themeBackground: record.themeBackground,
    // Additive/backward-compatible: existing v1 users without these fields
    // yet fall back to defaults instead of invalidating the whole parse
    // (which would wipe fontFamilyOverride/fontSize/themeBackground via
    // loadNamespacedPrefs's version-mismatch fallback-to-defaults path).
    gpuAcceleration:
      typeof record.gpuAcceleration === "boolean"
        ? record.gpuAcceleration
        : DEFAULT_TERMINAL_STYLE_PREFS.gpuAcceleration,
    ligaturesEnabled:
      typeof record.ligaturesEnabled === "boolean"
        ? record.ligaturesEnabled
        : DEFAULT_TERMINAL_STYLE_PREFS.ligaturesEnabled,
  };
}

export function loadTerminalStylePrefs(
  storage: Pick<Storage, "getItem"> | null = browserStorage(),
): TerminalStylePrefs {
  return loadNamespacedPrefs(
    terminalStylePrefsStorageKey,
    terminalStylePrefsVersion,
    parseTerminalStylePrefs,
    DEFAULT_TERMINAL_STYLE_PREFS,
    storage,
  );
}

export function saveTerminalStylePrefs(
  prefs: TerminalStylePrefs,
  storage: Pick<Storage, "setItem"> | null = browserStorage(),
): void {
  saveNamespacedPrefs(
    terminalStylePrefsStorageKey,
    terminalStylePrefsVersion,
    prefs,
    storage,
  );
}

// Pure so it is trivially unit-testable independent of xterm: returns the
// fallback stack unchanged when `override` is empty/whitespace-only (today's
// exact hardcoded look), otherwise prepends the trimmed override.
export function buildEffectiveTerminalFontFamily(override: string): string {
  const trimmed = override.trim();
  if (!trimmed) {
    return TERMINAL_FONT_FALLBACK_STACK;
  }
  return `${trimmed}, ${TERMINAL_FONT_FALLBACK_STACK}`;
}

// xterm's option setter is equality-guarded: `OptionsService` only fires
// `onOptionChange` when `rawOptions[key] !== value`, and cell-metric
// re-measurement hangs off exactly that event
// (`CharSizeService` subscribes via
// `onMultipleOptionChange(["fontFamily", "fontSize"], () => this.measure())`,
// and the renderer's atlas/refresh handler subscribes to a superset list).
// So plainly re-assigning an UNCHANGED family string is a provable no-op -
// which is precisely the webfont race this exists for: a `fontFamilyOverride`
// persisted at boot is re-fetched by `reregisterDownloadedFonts()` AFTER the
// terminal already measured its glyph cell against the substituted fallback,
// and the family string never changes across that download, so nothing
// re-measures and the stale fallback cell metrics survive.
//
// Returns the assignment sequence that forces the change event to fire while
// still ending on `target`. The nudge value appends the generic `monospace`
// keyword, which `TERMINAL_FONT_FALLBACK_STACK` already ends with, so the
// intermediate assignment resolves to the identical face: the re-measure it
// triggers is already the correct one, and the final assignment restores the
// exact target string (and fires a second change event, so the renderer's
// glyph atlas - cached against the fallback face - is cleared too). Both
// assignments happen inside one synchronous task, so no frame is ever painted
// against the intermediate string.
export function terminalFontFamilyReapplySequence(
  current: string,
  target: string,
): string[] {
  if (current !== target) {
    return [target];
  }
  return [`${target}, monospace`, target];
}

// Pure validation for the Terminal settings section's font-size <input>:
// accepts the raw input string, returns a usable positive finite point size,
// or null when the entry is empty / NaN / zero / negative. Extracted from the
// JSX onChange guard so the "reject NaN and non-positive sizes" contract is
// unit-testable without a DOM.
export function parseTerminalFontSizeInput(raw: string): number | null {
  const nextSize = Number(raw);
  if (Number.isFinite(nextSize) && nextSize > 0) {
    return nextSize;
  }
  return null;
}
