import {
  attentionTitleFor,
  buildAttentionFaviconHref,
  shouldFireAttentionNotification,
} from "./browserAttentionCue.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

// --- attentionTitleFor -------------------------------------------------

assertEqual(
  attentionTitleFor("ws dashboard", false, false),
  "ws dashboard",
  "inactive + flash-off returns the base title unchanged",
);

assertEqual(
  attentionTitleFor("ws dashboard", false, true),
  "ws dashboard",
  "inactive (tone null) returns the base title even if flashOn is true",
);

assertEqual(
  attentionTitleFor("ws dashboard", true, false),
  "ws dashboard",
  "active but the current tick is the flash-off phase returns the base title",
);

assertEqual(
  attentionTitleFor("ws dashboard", true, true),
  "● Attention needed - ws dashboard",
  "active + flash-on returns the attention-labeled variant",
);

assertEqual(
  attentionTitleFor("custom title", true, true),
  "● Attention needed - custom title",
  "the base title is never hardcoded in this module - it is always the caller's own value",
);

// --- buildAttentionFaviconHref ------------------------------------------

const inactiveHref = buildAttentionFaviconHref(false);
const activeHref = buildAttentionFaviconHref(true);

assertEqual(
  inactiveHref.startsWith("data:image/svg+xml,"),
  true,
  "the inactive favicon is a plain SVG data URI, not a canvas/PNG reference",
);

assertEqual(
  activeHref.startsWith("data:image/svg+xml,"),
  true,
  "the active favicon is a plain SVG data URI, not a canvas/PNG reference",
);

assertEqual(
  inactiveHref === activeHref,
  false,
  "the active/inactive favicon hrefs are distinct strings",
);

assertEqual(
  buildAttentionFaviconHref(true),
  activeHref,
  "buildAttentionFaviconHref is a pure function of its boolean argument",
);

// --- shouldFireAttentionNotification -------------------------------------
//
// The edge-only contract this ticket's binding carry-forward pins: proving
// there is no independent "already notified" watermark beyond the one-slot
// previous-tone argument the caller supplies.

assertEqual(
  shouldFireAttentionNotification(null, "working"),
  false,
  "a transition into 'working' never fires - it is background progress, not an interruption",
);

assertEqual(
  shouldFireAttentionNotification("working", "ready"),
  true,
  "a transition from 'working' into 'ready' fires",
);

assertEqual(
  shouldFireAttentionNotification(null, "ready"),
  true,
  "a transition from no tone directly into 'ready' fires",
);

assertEqual(
  shouldFireAttentionNotification("ready", "ready"),
  false,
  "staying at 'ready' does NOT re-fire - proves there is no independent watermark beyond the previous-tone slot",
);

assertEqual(
  shouldFireAttentionNotification("ready", "working"),
  false,
  "a transition OUT of 'ready' into 'working' does not fire",
);

assertEqual(
  shouldFireAttentionNotification("working", "ready") &&
    shouldFireAttentionNotification("ready", "working") === false,
  true,
  "sanity: working->ready fires and the immediately following ready->working does not",
);

// ready -> working -> ready fires again: the one-slot previous-tone ref is
// exactly what makes this possible - a second independent "already notified"
// watermark would suppress this second entry.
{
  let previous: "ready" | "working" | null = "ready";
  const firstReentry = shouldFireAttentionNotification(previous, "working");
  previous = "working";
  const secondReentry = shouldFireAttentionNotification(previous, "ready");
  assertEqual(
    firstReentry,
    false,
    "ready -> working (leaving ready) does not fire",
  );
  assertEqual(
    secondReentry,
    true,
    "ready -> working -> ready fires again on the second entry into ready",
  );
}

assertEqual(true, true, "browserAttentionCue tests completed");
