// Pure predicate for Phase 1 of
// 260727-bug-dashboard-terminal-refocus-ime-composition-loss: decides
// whether the deferred `refocusActiveTerminal()` callback (fired via
// `window.setTimeout(..., 0)` in terminalPaneBody.tsx) should actually move
// focus back onto the terminal's helper textarea. Kept DOM-free so the
// guard logic is unit-testable without jsdom; the caller
// (terminalPaneBody.tsx) is responsible for reading real ref/DOM state into
// the minimal shape below at fire time.

export type RefocusGuardState = {
  readonly composing: boolean;
  readonly keepFocus: boolean;
  readonly visible: boolean;
  readonly active: boolean;
};

export function shouldRefocusTerminal(state: RefocusGuardState): boolean {
  return (
    !state.composing && state.keepFocus && state.visible && state.active
  );
}
