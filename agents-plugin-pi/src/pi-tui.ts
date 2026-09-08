/**
 * `260908-feat-ws-pi-conversation-view-component` Phase 1: the single
 * resolution point this package uses to reach `@earendil-works/pi-tui`.
 *
 * ## Addendum (plan `09-0122-260908-conversation-view-phase1.md`, 2026-09-09)
 *
 * Step 1 discovered that `@earendil-works/pi-coding-agent` ships its own
 * `npm-shrinkwrap.json`, pinning a nested, unhoistable
 * `pi-coding-agent/node_modules/@earendil-works/pi-tui@0.84.4` — confirmed via
 * a fresh install, `npm dedupe`, and an `overrides` attempt, all of which
 * leave `npm ls @earendil-works/pi-tui` reporting TWO physical copies (both
 * `0.84.4`, so not a range conflict — a structural property of the published
 * package). A plain top-level static import of this package's own
 * `@earendil-works/pi-tui@0.84.4` dependency (added below `package.json`'s
 * `dependencies` for the test/types path) resolves to a DIFFERENT physical
 * module than the one `pi-coding-agent`'s own
 * `dist/modes/interactive/interactive-mode.js` constructs its live `tui`
 * from — a dual-package hazard that would make any `instanceof` identity
 * check against this file's classes fail against the real host `tui`.
 *
 * The ticket's own "Runtime instance" clause pre-committed the fix: this
 * file resolves `pi-tui` through the host AT RUNTIME (the same guarded
 * dynamic `import("@earendil-works/pi-tui")` shim shape `push-render.ts`'s
 * `loadPushTuiModules` and `tool-result-render.ts`'s `loadToolResultTuiModules`
 * already use to reach host modules — Pi's own jiti-based extension loading
 * resolves that dynamic
 * import against the SAME nested copy `pi-coding-agent`'s own code uses,
 * independent of whatever this package's own `node_modules` layout is),
 * while ALSO exposing a plain static import for the test/types/default path
 * (`node --test` has no live Pi host to resolve through, and `pi-tui.ts`
 * needs static types regardless). Runtime therefore uses the host's single
 * instance — "not a duplicated instance" is satisfied at the instance
 * actually used, even though two copies sit on disk. The owner-run
 * `tui instanceof TuiMainScreen || tui instanceof TuiAltScreen` identity
 * check (this ticket's step 7, not agent-cleared) is expected to pass
 * against `loadHostPiTui()`'s classes, never against this file's static
 * import's classes.
 *
 * `TuiMainScreen`/`TuiAltScreen` are exported specifically so that check has
 * a value to `instanceof` against — `TUI` is a TypeScript interface
 * (type-only export), and `instanceof` requires a value, not a type.
 *
 * `push-render.ts` and `tool-result-render.ts` (the two importers this
 * ticket names) now go through this file's `loadHostPiTui()` instead of each
 * carrying its own dynamic import + "unavailable" fallback branch — the
 * per-importer duplication collapses into this one resolution point.
 *
 * Phase 2 (`260908-feat-ws-pi-conversation-view-component`) added a third
 * caller: `ask.ts`'s live `ConversationViewComponent` factory now calls
 * `loadHostPiTui()` for its render primitives too, replacing the old
 * per-thread overlay module's own guarded `loadMarkdownRenderer` dynamic
 * import (that module, and its dynamic import, are deleted).
 */

import * as piTuiStatic from "@earendil-works/pi-tui";
import type {
  Component,
  EditorOptions,
  EditorTheme,
  MarkdownTheme,
  ScrollViewOptions,
  SelectListTheme,
  TUI,
} from "@earendil-works/pi-tui";

/** Structural shape of the `@earendil-works/pi-tui` module, shared by the static export below and `loadHostPiTui`'s resolved value. */
export type PiTuiModule = typeof piTuiStatic;

// Static/test/types/default path: this package's own top-level
// `@earendil-works/pi-tui@0.84.4` dependency. Used directly by `node --test`
// (no live Pi host to resolve through) and as `conversation-view.ts`'s
// default `primitives` — see the Addendum above for why this is NOT the path
// an owner-run live identity check should ever be run against.
export const { Box, Editor, Markdown, ScrollView, Text, TuiAltScreen, TuiMainScreen, stripTerminalSequences, truncateToWidth, visibleWidth } =
  piTuiStatic;
export type { Component, EditorOptions, EditorTheme, MarkdownTheme, ScrollViewOptions, SelectListTheme, TUI };

/**
 * Runtime path: resolves `@earendil-works/pi-tui` through the host (see the
 * Addendum above). Falls back to the static import when the dynamic import
 * ever fails — defensive only; per the Addendum, the live host always has
 * pi-tui, and under `node --test` the dynamic import resolves to this
 * package's own static copy anyway (no live host to differ from), so this
 * function never actually needs the fallback in either environment today.
 */
export async function loadHostPiTui(): Promise<PiTuiModule> {
  try {
    return (await import("@earendil-works/pi-tui")) as unknown as PiTuiModule;
  } catch {
    return piTuiStatic;
  }
}
