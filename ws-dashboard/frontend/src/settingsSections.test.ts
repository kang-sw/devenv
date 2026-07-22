import {
  SETTINGS_SECTIONS,
  SettingsTerminalContext,
  TerminalStyleSection,
} from "./settingsSections.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

// --- Decoupled registry contract --------------------------------------------
//
// The settings shell (`SettingsModal`) iterates an injected `sections` list and
// only consumes `{ id, title, Component }`. These assertions pin that contract
// so a regression that reintroduces shell-embedded, per-render section
// construction (the focus-losing anti-pattern) cannot pass silently.

assertEqual(
  Array.isArray(SETTINGS_SECTIONS),
  true,
  "the registry is an injectable array of descriptors",
);

assertEqual(
  SETTINGS_SECTIONS.length,
  1,
  "Phase 1 registers exactly the Terminal section",
);

const terminalDescriptor = SETTINGS_SECTIONS[0];

assertEqual(
  terminalDescriptor.id,
  "terminal",
  "the Terminal descriptor's id is stable",
);

assertEqual(
  terminalDescriptor.title,
  "Terminal",
  "the Terminal descriptor's title is stable",
);

// --- Stable section identity ------------------------------------------------
//
// The descriptor's `Component` must be the module-scope `TerminalStyleSection`
// reference, NOT an arrow rebuilt on every render. Comparing to the exported
// named component proves the registry holds a stable identity: an inline
// per-render arrow could never be `===` this reference.
assertEqual(
  terminalDescriptor.Component === TerminalStyleSection,
  true,
  "the registry Component is the stable module-scope TerminalStyleSection reference",
);

assertEqual(
  typeof terminalDescriptor.Component,
  "function",
  "the registry Component is a renderable function component",
);

// --- Prefs-from-context wiring ----------------------------------------------
//
// The section sources its prefs + write path from context, so it declares zero
// props. If it were reverted to a prop-driven `({ prefs, onChange })` shape
// (requiring the shell to thread Terminal-typed props), its arity would become
// non-zero and this assertion would fail.
assertEqual(
  TerminalStyleSection.length,
  0,
  "TerminalStyleSection takes no props - it reads prefs/setter from context",
);

// The settings-scoped Terminal context exists and is distinct from the section
// component, confirming the wiring seam is a real context object.
assertEqual(
  typeof SettingsTerminalContext,
  "object",
  "SettingsTerminalContext is a React context object",
);

assertEqual(true, true, "settingsSections tests completed");
