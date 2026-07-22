import { useEffect } from "react";
import type { RefObject } from "react";

// Generic click-outside/Escape dismissal hook with no App-state closure —
// shared by WorkbenchShell's overflow/branch menus and AgentChatPaneBody's
// history popover. Relocated out of App.tsx (rather than duplicated) so
// both call sites consume the same implementation.
export function useDismissableMenu(
  open: boolean,
  containerRef: RefObject<HTMLElement | null>,
  onDismiss: () => void,
) {
  useEffect(() => {
    if (!open) {
      return;
    }
    const dismissIfOutside = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Node) || containerRef.current?.contains(target)) {
        return;
      }
      onDismiss();
    };
    const dismissOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onDismiss();
      }
    };
    document.addEventListener("click", dismissIfOutside);
    document.addEventListener("keydown", dismissOnEscape, true);
    return () => {
      document.removeEventListener("click", dismissIfOutside);
      document.removeEventListener("keydown", dismissOnEscape, true);
    };
  }, [containerRef, onDismiss, open]);
}
