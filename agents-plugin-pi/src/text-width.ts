/**
 * `260908-feat-ws-pi-conversation-view-component` Phase 1: `pi-tui` already
 * exports `visibleWidth` (`node_modules/.../pi-tui/dist/utils.d.ts`, confirmed
 * during Phase 1 planning) — there is nothing to reimplement. This module
 * exists so callers that need display-width measurement (`agent-widget.ts`,
 * `conversation-view.ts`) go through the package's one static `pi-tui`
 * resolution point (`./pi-tui.ts`) rather than each importing the package
 * directly. The Phase 2 overlay migration
 * (`260908-feat-ws-pi-conversation-view-component`) removed the old
 * per-thread overlay module, which had carried its own local `visibleWidth`
 * copy — that duplication is gone now that `ask.ts` builds its live view on
 * `conversation-view.ts` instead. It also holds `sanitizePreviewText`, the
 * control-character normalization shared by raw previews and summary rows.
 */
export { visibleWidth } from "./pi-tui.ts";

/**
 * Normalize unsafe controls before width accounting; tabs become stable
 * four-column spaces. Lives here, not in `tool-result-render.ts`, so
 * `display-summary-render.ts` (which that module imports) can share it.
 */
export function sanitizePreviewText(text: string): string {
  return text
    .replace(/\r\n?/g, "\n")
    .replace(/\t/g, "    ")
    .replace(/[\u0000-\u0008\u000b-\u001f\u007f]/g, "?");
}
