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

export type TerminalStylePrefs = {
  readonly fontFamilyOverride: string;
  readonly fontSize: number;
  readonly themeBackground: string;
};

// Reproduces today's exact hardcoded terminal look when nothing is persisted
// yet: empty override (fallback stack unchanged), fontSize 12, background
// "#0b0d10".
export const DEFAULT_TERMINAL_STYLE_PREFS: TerminalStylePrefs = {
  fontFamilyOverride: "",
  fontSize: 12,
  themeBackground: "#0b0d10",
};

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
