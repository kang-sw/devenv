import {
  buildEffectiveTerminalFontFamily,
  DEFAULT_TERMINAL_STYLE_PREFS,
  loadTerminalStylePrefs,
  parseTerminalFontSizeInput,
  saveTerminalStylePrefs,
  terminalFontFamilyReapplySequence,
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
  {
    fontFamilyOverride: "",
    fontSize: 12,
    themeBackground: "#0b0d10",
    gpuAcceleration: true,
    ligaturesEnabled: false,
  },
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
    gpuAcceleration: false,
    ligaturesEnabled: true,
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

  // A pre-existing v1 payload saved before gpuAcceleration/ligaturesEnabled
  // existed (same version, missing fields) must NOT be rejected wholesale -
  // it should keep its other fields and only default the two new ones.
  fakeStorage.set(
    "ws-dashboard.settings.terminal.v1",
    JSON.stringify({
      version: 1,
      value: {
        fontFamilyOverride: "Iosevka Term",
        fontSize: 16,
        themeBackground: "#101820",
      },
    }),
  );
  assertDeepEqual(
    loadTerminalStylePrefs(storage),
    {
      fontFamilyOverride: "Iosevka Term",
      fontSize: 16,
      themeBackground: "#101820",
      gpuAcceleration: DEFAULT_TERMINAL_STYLE_PREFS.gpuAcceleration,
      ligaturesEnabled: DEFAULT_TERMINAL_STYLE_PREFS.ligaturesEnabled,
    },
    "a pre-existing v1 payload missing gpuAcceleration/ligaturesEnabled keeps its other fields and defaults only the new two",
  );
}

// --- Font-size input validation ---------------------------------------------
//
// The Terminal settings section's font-size <input> guards its onChange through
// this pure helper: only finite, strictly-positive sizes are accepted; empty,
// NaN, zero, and negative entries are rejected (return null) so a partially
// typed or invalid value never clobbers the persisted size.

assertEqual(
  parseTerminalFontSizeInput("14"),
  14,
  "a valid positive size string parses to that number",
);

assertEqual(
  parseTerminalFontSizeInput("13.5"),
  13.5,
  "a fractional positive size is accepted",
);

assertEqual(
  parseTerminalFontSizeInput(""),
  null,
  "an empty input is rejected (returns null)",
);

assertEqual(
  parseTerminalFontSizeInput("abc"),
  null,
  "a non-numeric input (NaN) is rejected",
);

assertEqual(
  parseTerminalFontSizeInput("0"),
  null,
  "zero is rejected as a non-positive size",
);

assertEqual(
  parseTerminalFontSizeInput("-4"),
  null,
  "a negative size is rejected",
);

// --- terminalFontFamilyReapplySequence --------------------------------------
//
// Driven against a faithful stand-in for xterm's `OptionsService` setter,
// whose real (minified) body is
// `this.rawOptions[e] !== i && (this.rawOptions[e] = i, this._onOptionChange.fire(e))`
// - i.e. an equality guard in front of the one event that
// `CharSizeService.measure()` (and the renderer's atlas invalidation) hangs
// off. The point of these cases is to prove the re-apply actually re-measures,
// not merely that it assigns.

function makeEqualityGuardedFontFamilyOption(initial: string) {
  const state = { value: initial, changeEvents: 0 };
  return {
    state,
    set(next: string) {
      if (state.value !== next) {
        state.value = next;
        state.changeEvents += 1;
      }
    },
  };
}

function applyReapplySequence(
  option: ReturnType<typeof makeEqualityGuardedFontFamilyOption>,
  target: string,
) {
  for (const value of terminalFontFamilyReapplySequence(
    option.state.value,
    target,
  )) {
    option.set(value);
  }
}

const unchangedStack = buildEffectiveTerminalFontFamily("Fira Code");

// The defect this replaces: a bare re-assignment of the unchanged string.
const bareReassign = makeEqualityGuardedFontFamilyOption(unchangedStack);
bareReassign.set(unchangedStack);
assertEqual(
  bareReassign.state.changeEvents,
  0,
  "baseline: re-assigning the unchanged family string fires no option-change event (so nothing re-measures)",
);

const reapplied = makeEqualityGuardedFontFamilyOption(unchangedStack);
applyReapplySequence(reapplied, unchangedStack);
assertEqual(
  reapplied.state.changeEvents > 0,
  true,
  "re-applying the unchanged family string forces at least one option-change event",
);
assertEqual(
  reapplied.state.value,
  unchangedStack,
  "the re-apply sequence ends on the exact target family string",
);

assertDeepEqual(
  terminalFontFamilyReapplySequence(unchangedStack, unchangedStack),
  [`${unchangedStack}, monospace`, unchangedStack],
  "an unchanged target nudges through an equivalent stack before restoring the exact string",
);

assertEqual(
  terminalFontFamilyReapplySequence(unchangedStack, unchangedStack)[0].endsWith(
    ", monospace",
  ) && unchangedStack.endsWith("monospace"),
  true,
  "the nudge only appends a generic keyword the stack already ends with, so it resolves to the same face",
);

assertDeepEqual(
  terminalFontFamilyReapplySequence("old-stack, monospace", unchangedStack),
  [unchangedStack],
  "a genuinely changed target needs no nudge - the plain assignment already fires the change event",
);

const changedTarget = makeEqualityGuardedFontFamilyOption("old-stack, monospace");
applyReapplySequence(changedTarget, unchangedStack);
assertEqual(
  changedTarget.state.changeEvents,
  1,
  "a genuinely changed target fires exactly one option-change event",
);

assertEqual(true, true, "terminalPrefs tests completed");
