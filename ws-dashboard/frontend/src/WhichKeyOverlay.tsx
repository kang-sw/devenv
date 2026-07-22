// `260722-feat-dashboard-which-key-hint-overlay` Phase 1 — bottom-right
// lazyvim-style hint popup shown while a `Ctrl+Space` leader sequence is
// pending.
//
// CONTRACT: purely additive presentation layer over the hotkey framework's
// own leader-mode state machine (`hotkeys.ts`) - it renders `null` outside
// `{kind: "pending"}` and derives its rows from `describeLeaderChildren`
// (the same children-win group/leaf precedence `stepLeaderState` already
// enforces), never inventing group names, labels, or bindings the registry
// doesn't have (R1, transitively). It does not capture or consume keyboard
// input itself - `App.tsx`'s existing capture-phase `keydown` listener
// remains the sole input path; this component only reacts to the
// `leaderState` prop threaded down from that listener's parallel UI-state
// mirror.
import { useEffect, useRef, useState } from "react";
import {
  describeLeaderChildren,
  type HotkeyDispatchContext,
  type LeaderState,
} from "./hotkeys";

// Finalized which-key spec: "appears after a configurable delay (default
// 250ms)". Phase 1 ships the default only - no settings UI exists yet to
// configure it (out of scope, see the plan's Out of Scope section).
const APPEARANCE_DELAY_MS = 250;

function displayKey(key: string): string {
  return key === " " ? "Space" : key;
}

export function WhichKeyOverlay({
  leaderState,
}: {
  readonly leaderState: LeaderState<HotkeyDispatchContext>;
}) {
  const [visible, setVisible] = useState(false);
  // Tracks whether the previous render was already `pending`, so a
  // narrowing keystroke (which replaces `leaderState` with a new object
  // every step, per `stepLeaderState`) does not restart the appearance
  // delay - only the idle -> pending transition should. Read together with
  // `timerRef` below rather than `leaderState.enteredAtMs`, which resets on
  // every narrowing step and therefore can't gate a "delay since the
  // INITIAL leader press" timer on its own.
  const wasPendingRef = useRef(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    if (leaderState.kind !== "pending") {
      wasPendingRef.current = false;
      setVisible(false);
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      return;
    }
    if (wasPendingRef.current) {
      // A narrowing step within an already-pending session: leave any
      // in-flight appearance timer (or the already-visible overlay) alone.
      return;
    }
    wasPendingRef.current = true;
    timerRef.current = window.setTimeout(() => {
      setVisible(true);
      timerRef.current = null;
    }, APPEARANCE_DELAY_MS);
  }, [leaderState]);

  // Belt-and-suspenders cleanup on unmount only - the effect above already
  // clears the timer on every idle transition, but the component itself
  // could unmount mid-delay (e.g. a route change).
  useEffect(() => {
    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
    };
  }, []);

  if (leaderState.kind !== "pending" || !visible) {
    return null;
  }

  const entries = describeLeaderChildren(leaderState.node);

  return (
    <div className="which-key-overlay" role="status" aria-live="polite">
      <div className="which-key-overlay-title">Leader</div>
      <ul className="which-key-overlay-list">
        {entries.map((entry) => (
          <li key={entry.key} className="which-key-overlay-row">
            <kbd className="which-key-overlay-key">
              {displayKey(entry.key)}
            </kbd>
            <span className="which-key-overlay-arrow" aria-hidden="true">
              →
            </span>
            <span className="which-key-overlay-label">
              {entry.kind === "group" ? "+group" : entry.label ?? entry.key}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
