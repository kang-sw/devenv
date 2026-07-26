// Pure builders for the 260725 Phase 8 browser-level attention cue. See
// ai-docs/.plans/2026-07/26-1811-pty-agent-browser-notification.md.
//
// CONTRACT: this module stays free of React/DOM/`Notification` calls, mirroring
// `agentAttention.ts`'s "pure predicate, wired in App.tsx" split - App.tsx owns
// the `document.title`/`<link rel="icon">` writes and the actual
// `new Notification(...)` call; this module only decides WHAT string/boolean
// those call sites should use. It is a sibling of `agentAttention.ts` (imported
// by App.tsx, never importing App.tsx or anything under `workbench/`), so it
// carries no risk of dragging the whole app into the NodeNext route-tests
// program (mental model `ws-web-dashboard` `## Common Mistakes`).
//
// "No second watermark" (the binding carry-forward from Phase 7's Result):
// every tier here is a pure function of the SAME `globalAttentionTone` value
// App.tsx derives from `aggregateNavAttentionTone` (itself built on
// `pendingAttentionStateFor`) - nothing in this module keeps its own notion of
// "already notified" beyond the one `previousTone` argument
// `shouldFireAttentionNotification` is handed each call.

export type AttentionTone = "ready" | "working" | null;

// Title-flash string builder (Tier 1, half A). `active` is
// `globalAttentionTone !== null`; `flashOn` is the caller's interval-driven
// toggle. Returns `baseTitle` unchanged whenever there is nothing to flash or
// the current tick is the "off" phase of the flash, so the base title is
// never duplicated in this module - the caller always supplies its own
// captured `document.title` (see App.tsx's mount-time ref).
export function attentionTitleFor(
  baseTitle: string,
  active: boolean,
  flashOn: boolean,
): string {
  if (!active || !flashOn) {
    return baseTitle;
  }
  return `● Attention needed - ${baseTitle}`;
}

// Favicon badge (Tier 1, half B): a plain SVG data URI, not a canvas
// rasterization of `icon-192.png`. Per the plan's Implementation Plan step 3,
// a canvas approach would need an async `Image` load with its own onload/race
// handling and would only be observable from inside a DOM effect - this stays
// a pure function of one boolean, testable as a plain string comparison, and
// it does not attempt to reproduce `icon-192.png` pixel-for-pixel (a simple
// monochrome glyph with a badge dot is enough).
const FAVICON_BASE_SVG =
  '<circle cx="16" cy="16" r="13" fill="#3b4252" stroke="#c8ccd6" stroke-width="2"/>';
const FAVICON_BADGE_SVG =
  '<circle cx="16" cy="16" r="13" fill="#3b4252" stroke="#c8ccd6" stroke-width="2"/>' +
  '<circle cx="24" cy="8" r="7" fill="#e58e26" stroke="#1c1f26" stroke-width="1.5"/>';

export function buildAttentionFaviconHref(active: boolean): string {
  const body = active ? FAVICON_BADGE_SVG : FAVICON_BASE_SVG;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">${body}</svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

// Tier 2 edge detector: true ONLY on a transition INTO "ready". This is the
// entire "no second watermark" mechanism - the only state a caller needs to
// hold is the previous render's `globalAttentionTone` (a plain `useRef` in
// App.tsx, not persisted, not keyed by terminal id), so it structurally
// cannot diverge from the tab/nav badges, which are driven by that exact same
// derived tone in the exact same render.
//
// `working` is normal background progress, not an actionable interruption
// (Implementation Plan step 3's pinned tone vocabulary), so a transition INTO
// `working` never fires - only a transition into `ready` does, and it fires
// again on every subsequent `ready` entry (`ready` -> `working` -> `ready`),
// never on staying at `ready` (`ready` -> `ready` does not re-fire).
export function shouldFireAttentionNotification(
  previousTone: AttentionTone,
  currentTone: AttentionTone,
): boolean {
  return previousTone !== "ready" && currentTone === "ready";
}
