import { shouldRefocusTerminal } from "./terminalRefocusGuard.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

const allTrue = { composing: false, keepFocus: true, visible: true, active: true };

// --- All conditions satisfied -> true ---

assertEqual(shouldRefocusTerminal(allTrue), true, "all-true state refocuses");

// --- Each single condition flipped away from satisfied -> false ---

assertEqual(
  shouldRefocusTerminal({ ...allTrue, composing: true }),
  false,
  "active IME composition blocks refocus",
);

assertEqual(
  shouldRefocusTerminal({ ...allTrue, keepFocus: false }),
  false,
  "keepFocus=false blocks refocus",
);

assertEqual(
  shouldRefocusTerminal({ ...allTrue, visible: false }),
  false,
  "visible=false blocks refocus",
);

assertEqual(
  shouldRefocusTerminal({ ...allTrue, active: false }),
  false,
  "active=false (not the active pane) blocks refocus",
);

// --- Combined-false case ---

assertEqual(
  shouldRefocusTerminal({
    composing: true,
    keepFocus: false,
    visible: false,
    active: false,
  }),
  false,
  "all conditions unsatisfied blocks refocus",
);

console.log("terminalRefocusGuard.test.ts passed");
