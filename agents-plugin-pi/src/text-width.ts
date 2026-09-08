/**
 * `260908-feat-ws-pi-conversation-view-component` Phase 1: `pi-tui` already
 * exports `visibleWidth` (`node_modules/.../pi-tui/dist/utils.d.ts`, confirmed
 * during Phase 1 planning) — there is nothing to reimplement. This module
 * exists only so callers that need display-width measurement (`agent-widget.ts`,
 * `conversation-view.ts`) go through the package's one static `pi-tui`
 * resolution point (`./pi-tui.ts`) rather than each importing the package
 * directly. `overlay-chat.ts` keeps its own local `visibleWidth` — that file
 * runs untouched until Phase 2 deletes it.
 */
export { visibleWidth } from "./pi-tui.ts";
