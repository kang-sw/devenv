import { shouldSuppressBrowserShortcut } from "./keydownSuppression.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

// --- Each suppressed ctrl combo -> true ---

const suppressedKeys = [
  "s", "p", "f", "g", "d", "o", "u", "j",
  "+", "=", "-", "_", "0",
];

for (const key of suppressedKeys) {
  assertEqual(
    shouldSuppressBrowserShortcut({
      ctrlKey: true,
      metaKey: false,
      key,
      targetIsEditable: false,
    }),
    true,
    `Ctrl+${key} is suppressed`,
  );
}

// --- Ctrl+R / Cmd+R -> false (explicit reload whitelist) ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "r",
    targetIsEditable: false,
  }),
  false,
  "Ctrl+R is never suppressed (reload whitelist)",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: true,
    key: "r",
    targetIsEditable: false,
  }),
  false,
  "Cmd+R is never suppressed (reload whitelist)",
);

// --- Normal editing/clipboard combos -> false ---

const allowedEditingKeys = ["c", "v", "x", "a", "z", "y"];

for (const key of allowedEditingKeys) {
  assertEqual(
    shouldSuppressBrowserShortcut({
      ctrlKey: true,
      metaKey: false,
      key,
      targetIsEditable: false,
    }),
    false,
    `Ctrl+${key} (editing/clipboard) is never suppressed`,
  );
}

// --- Plain keys with no modifier -> false ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: false,
    key: "a",
    targetIsEditable: false,
  }),
  false,
  "plain 'a' with no modifier is never suppressed",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: false,
    key: "Enter",
    targetIsEditable: false,
  }),
  false,
  "plain Enter with no modifier is never suppressed",
);

// --- Backspace: editable vs. non-editable target ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: false,
    key: "Backspace",
    targetIsEditable: true,
  }),
  false,
  "Backspace inside an editable target is never suppressed",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: false,
    key: "Backspace",
    targetIsEditable: false,
  }),
  true,
  "Backspace outside an editable target is suppressed (back-navigation guard)",
);

// --- Meta-key (Cmd) variants of a suppressed combo ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: true,
    key: "s",
    targetIsEditable: false,
  }),
  true,
  "Cmd+S is suppressed (ctrlKey || metaKey guard covers Cmd)",
);

// --- A ctrl combo not in the suppressed set -> false (allow-by-default) ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "k",
    targetIsEditable: false,
  }),
  false,
  "Ctrl+K (not in the suppressed set) is never suppressed",
);

console.log("keydownSuppression.test.ts passed");
