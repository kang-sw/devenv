// Pure predicate for Phase 2 of
// 260721-feat-dashboard-suppress-browser-shortcuts: decides whether a
// Class-A browser-shortcut keydown should be preventDefault()-ed. Kept
// DOM-free so the block/allow set is unit-testable without jsdom; the
// caller (App.tsx) is responsible for reading real DOM state into the
// minimal shape below.

export type SuppressibleKeydownEvent = {
  readonly ctrlKey: boolean;
  readonly metaKey: boolean;
  readonly key: string;
  readonly targetIsEditable: boolean;
};

const SUPPRESSED_CTRL_KEYS = new Set([
  "s", "p", "f", "g", "d", "o", "u", "j",
  "+", "=", "-", "_", "0",
]);

export function shouldSuppressBrowserShortcut(
  evt: SuppressibleKeydownEvent,
): boolean {
  const ctrlOrMeta = evt.ctrlKey || evt.metaKey;
  if (ctrlOrMeta) {
    const key = evt.key.toLowerCase();
    if (key === "r") {
      return false; // explicit reload whitelist — never suppress
    }
    return SUPPRESSED_CTRL_KEYS.has(key);
  }
  if (evt.key === "Backspace") {
    return !evt.targetIsEditable;
  }
  return false;
}
