import {
  shouldSuppressBrowserShortcut,
  SUPPRESSED_CTRL_KEYS,
} from "./keydownSuppression.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

// --- Each suppressed ctrl combo -> true ---
// Sourced from the implementation's own exported set so adding a key there
// automatically extends this coverage without a hand-duplicated list here.

const suppressedKeys = [...SUPPRESSED_CTRL_KEYS];

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

// --- Ctrl+R / Cmd+R -> true (reserved for in-app reverse-history-search) ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "r",
    targetIsEditable: false,
  }),
  true,
  "Ctrl+R is suppressed (reserved for in-app reverse-history-search)",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: true,
    key: "r",
    targetIsEditable: false,
  }),
  true,
  "Cmd+R is suppressed (reserved for in-app reverse-history-search)",
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

// --- Uppercase / shift-cased `key` values -> still normalized via
// toLowerCase() before the suppressed-set lookup ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "S",
    targetIsEditable: false,
  }),
  true,
  "Ctrl+Shift+S (uppercase 'S') is still suppressed",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "P",
    targetIsEditable: false,
  }),
  true,
  "Ctrl+Shift+P (uppercase 'P') is still suppressed",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "R",
    targetIsEditable: false,
  }),
  true,
  "Ctrl+Shift+R (uppercase 'R') is still suppressed (reverse-history-search)",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: true,
    key: "R",
    targetIsEditable: false,
  }),
  true,
  "Cmd+Shift+R (uppercase 'R') is still suppressed (reverse-history-search)",
);

// --- Ctrl/Cmd+Backspace: the ctrlOrMeta branch returns before the
// Backspace branch is ever reached, so these are NOT suppressed regardless
// of whether the target is editable. This pins that current interaction. ---

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "Backspace",
    targetIsEditable: false,
  }),
  false,
  "Ctrl+Backspace is not suppressed (modifier branch returns first)",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: false,
    metaKey: true,
    key: "Backspace",
    targetIsEditable: false,
  }),
  false,
  "Cmd+Backspace is not suppressed (modifier branch returns first)",
);

assertEqual(
  shouldSuppressBrowserShortcut({
    ctrlKey: true,
    metaKey: false,
    key: "Backspace",
    targetIsEditable: true,
  }),
  false,
  "Ctrl+Backspace in an editable target is not suppressed either",
);

// --- Meta-key (Cmd) parity across more of the suppressed set, plus
// allowed clipboard keys via metaKey, showing ctrl/meta are treated
// symmetrically by the `ctrlKey || metaKey` guard ---

const metaSuppressedKeys = ["p", "f", "g", "o", "j", "+", "0"];

for (const key of metaSuppressedKeys) {
  assertEqual(
    shouldSuppressBrowserShortcut({
      ctrlKey: false,
      metaKey: true,
      key,
      targetIsEditable: false,
    }),
    true,
    `Cmd+${key} is suppressed (meta parity)`,
  );
}

const metaAllowedEditingKeys = ["c", "v", "a"];

for (const key of metaAllowedEditingKeys) {
  assertEqual(
    shouldSuppressBrowserShortcut({
      ctrlKey: false,
      metaKey: true,
      key,
      targetIsEditable: false,
    }),
    false,
    `Cmd+${key} (editing/clipboard) is never suppressed`,
  );
}

console.log("keydownSuppression.test.ts passed");
