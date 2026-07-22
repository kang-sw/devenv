import {
  buildEffectiveTerminalFontFamily,
  DEFAULT_TERMINAL_STYLE_PREFS,
  loadTerminalStylePrefs,
  saveTerminalStylePrefs,
  TERMINAL_FONT_FALLBACK_STACK,
  type TerminalStylePrefs,
} from "./terminalPrefs.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

function assertDeepEqual<T>(actual: T, expected: T, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

// --- buildEffectiveTerminalFontFamily ---------------------------------------
//
// The literal fallback stack currently hardcoded inline at the `Terminal`
// constructor call in `App.tsx`'s terminal mount effect, kept byte-for-byte
// in sync here to prove an empty override reproduces today's exact
// hardcoded look.
const hardcodedFallbackStack =
  '"MesloLGS NF", "JetBrainsMono Nerd Font", "CaskaydiaCove Nerd Font", ' +
  '"FiraCode Nerd Font", "Hack Nerd Font", ui-monospace, SFMono-Regular, ' +
  'Menlo, Consolas, "Liberation Mono", monospace';

assertEqual(
  TERMINAL_FONT_FALLBACK_STACK,
  hardcodedFallbackStack,
  "the exported fallback stack matches App.tsx's hardcoded literal byte-for-byte",
);

assertEqual(
  buildEffectiveTerminalFontFamily(""),
  hardcodedFallbackStack,
  "an empty override returns the exact hardcoded fallback stack unchanged",
);

assertEqual(
  buildEffectiveTerminalFontFamily("   "),
  hardcodedFallbackStack,
  "a whitespace-only override returns the exact hardcoded fallback stack unchanged",
);

assertEqual(
  buildEffectiveTerminalFontFamily("Iosevka Term"),
  `Iosevka Term, ${hardcodedFallbackStack}`,
  "a non-empty override is prepended, trimmed, before the fallback stack",
);

assertEqual(
  buildEffectiveTerminalFontFamily("  Iosevka Term  "),
  `Iosevka Term, ${hardcodedFallbackStack}`,
  "a non-empty override is trimmed before being prepended",
);

// --- Defaults reproduce today's hardcoded look ------------------------------

assertDeepEqual(
  DEFAULT_TERMINAL_STYLE_PREFS,
  { fontFamilyOverride: "", fontSize: 12, themeBackground: "#0b0d10" },
  "defaults reproduce today's hardcoded fontSize/theme values",
);

// --- Persistence round trip -------------------------------------------------

{
  const fakeStorage = new Map<string, string>();
  const storage = {
    getItem: (key: string) => fakeStorage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      fakeStorage.set(key, value);
    },
    removeItem: (key: string) => {
      fakeStorage.delete(key);
    },
  };

  assertDeepEqual(
    loadTerminalStylePrefs(storage),
    DEFAULT_TERMINAL_STYLE_PREFS,
    "loading with nothing saved yet returns the default terminal style prefs",
  );

  const prefs: TerminalStylePrefs = {
    fontFamilyOverride: "Iosevka Term",
    fontSize: 16,
    themeBackground: "#101820",
  };
  saveTerminalStylePrefs(prefs, storage);
  assertDeepEqual(
    loadTerminalStylePrefs(storage),
    prefs,
    "a saved terminal style pref round-trips through storage",
  );

  // Malformed payload -> falls back to defaults rather than throwing.
  fakeStorage.set("ws-dashboard.settings.terminal.v1", "{not json");
  assertDeepEqual(
    loadTerminalStylePrefs(storage),
    DEFAULT_TERMINAL_STYLE_PREFS,
    "malformed JSON falls back to the default terminal style prefs",
  );

  // Version-mismatched payload -> falls back to defaults.
  fakeStorage.set(
    "ws-dashboard.settings.terminal.v1",
    JSON.stringify({ version: 2, value: prefs }),
  );
  assertDeepEqual(
    loadTerminalStylePrefs(storage),
    DEFAULT_TERMINAL_STYLE_PREFS,
    "a version-mismatched payload falls back to the default terminal style prefs",
  );
}

assertEqual(true, true, "terminalPrefs tests completed");
