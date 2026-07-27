import {
  NotificationSection,
  notificationAvailability,
  SETTINGS_SECTIONS,
  SettingsNotificationContext,
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
  2,
  "Phase 8 grows the registry to Terminal + Notifications",
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

// --- Notifications section registry (260725 Phase 8) ------------------------

const notificationDescriptor = SETTINGS_SECTIONS[1];

assertEqual(
  notificationDescriptor.id,
  "notifications",
  "the Notifications descriptor's id is stable",
);

assertEqual(
  notificationDescriptor.title,
  "Notifications",
  "the Notifications descriptor's title is stable",
);

assertEqual(
  notificationDescriptor.Component === NotificationSection,
  true,
  "the registry Component is the stable module-scope NotificationSection reference",
);

assertEqual(
  typeof notificationDescriptor.Component,
  "function",
  "the registry Component is a renderable function component",
);

assertEqual(
  NotificationSection.length,
  0,
  "NotificationSection takes no props - it reads enabled/setter from context",
);

assertEqual(
  typeof SettingsNotificationContext,
  "object",
  "SettingsNotificationContext is a React context object",
);

// --- Notification availability, all four states -----------------------------
//
// The guard used to test `typeof Notification` first and only consult
// `window.isSecureContext` inside that branch. Measured in Chromium, a
// plain-http LAN page - the dashboard's routine access mode - still defines the
// `Notification` global, so that ordering made the insecure-context copy
// unreachable and reported a bare "denied" instead. These four assertions pin
// the corrected ordering AND keep the undefined-global branch alive; the second
// one is the state the defect got wrong.

assertEqual(
  notificationAvailability(false, false, "default"),
  "unavailable - this page is not a secure context",
  "insecure context, no global (Safari/Firefox-shaped): reports insecure, not browser-unsupported",
);

assertEqual(
  notificationAvailability(false, true, "granted"),
  "unavailable - this page is not a secure context",
  "insecure context, global present (Chromium-shaped): reports insecure, NOT the raw permission - this is the ticket's core fix",
);

assertEqual(
  notificationAvailability(true, false, "default"),
  "unavailable in this browser",
  "secure context, no global: reports browser-unsupported",
);

assertEqual(
  notificationAvailability(true, true, "denied"),
  "denied",
  "secure context, global present: passes the live permission through verbatim",
);

assertEqual(true, true, "settingsSections tests completed");
