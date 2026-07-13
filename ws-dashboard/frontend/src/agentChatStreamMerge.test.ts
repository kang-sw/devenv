import { mergeStreamingTranscriptBlocks } from "./agentChatStreamMerge.js";
import type { TranscriptBlock } from "./workRootActivity.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assert(condition: unknown, label: string) {
  if (!condition) {
    throw new Error(label);
  }
}

function block(overrides: Partial<TranscriptBlock> & Pick<TranscriptBlock, "cursor">): TranscriptBlock {
  return {
    cursor: overrides.cursor,
    timestamp: overrides.timestamp ?? null,
    renderKind: overrides.renderKind ?? "markdown",
    title: overrides.title ?? null,
    text: overrides.text ?? null,
    data: overrides.data ?? null,
    degraded: overrides.degraded ?? false,
    role: overrides.role,
    turnId: overrides.turnId,
  };
}

// --- cursor-overwrite collision: streaming block replaces the canonical ----
// block at the same cursor wholesale (no partial-append merge of the text).

const canonical = [
  block({ cursor: "0", text: "Session started", role: "agent" }),
  block({ cursor: "1", text: "canonical partial", role: "agent" }),
  block({ cursor: "2", text: "later canonical block", role: "agent" }),
];

const collided = mergeStreamingTranscriptBlocks(canonical, {
  "1": block({ cursor: "1", text: "streamed replacement", role: "agent" }),
});

assertEqual(collided.length, 3, "cursor-collision merge does not change the block count");
assertEqual(
  collided[1]?.text,
  "streamed replacement",
  "a streaming block whose cursor matches a canonical block's cursor fully overwrites the canonical block's text",
);
assertEqual(
  collided[0]?.text,
  "Session started",
  "non-colliding canonical blocks before the collision are left untouched",
);
assertEqual(
  collided[2]?.text,
  "later canonical block",
  "non-colliding canonical blocks after the collision are left untouched",
);

// --- appended-but-unmatched streaming block lands at the end regardless of --
// its timestamp relative to interleaved canonical blocks that arrive later.

const earlyCanonical = [block({ cursor: "0", text: "first canonical block", role: "agent" })];

const appendedOnly = mergeStreamingTranscriptBlocks(earlyCanonical, {
  "stream-1": block({
    cursor: "stream-1",
    text: "in-flight streamed turn that started before the next canonical block",
    timestamp: "2026-07-13T00:00:00.000Z",
    role: "agent",
  }),
});

assertEqual(appendedOnly.length, 2, "an unmatched streaming block is appended alongside the canonical block");
assertEqual(
  appendedOnly[0]?.cursor,
  "0",
  "the pre-existing canonical block keeps its original position",
);
assertEqual(
  appendedOnly[1]?.cursor,
  "stream-1",
  "the unmatched streaming block lands at the end of the merged array",
);

// Now simulate the next tick: a *later* canonical block for a different
// cursor arrives (e.g. the daemon flushed a subsequent turn) while the
// earlier-timestamped streaming block is still in flight and unmatched.
// The merge does not reorder by timestamp, so the streaming block stays
// after the newly-arrived canonical block even though the streaming block
// logically started earlier.

const laterCanonical = [
  block({ cursor: "0", text: "first canonical block", role: "agent" }),
  block({
    cursor: "1",
    text: "a later canonical block that arrived after the stream started",
    timestamp: "2026-07-13T00:00:05.000Z",
    role: "agent",
  }),
];

const interleaved = mergeStreamingTranscriptBlocks(laterCanonical, {
  "stream-1": block({
    cursor: "stream-1",
    text: "in-flight streamed turn that started before the next canonical block",
    timestamp: "2026-07-13T00:00:00.000Z",
    role: "agent",
  }),
});

assertEqual(interleaved.length, 3, "the merged array includes both canonical blocks plus the unmatched streaming block");
assertEqual(
  interleaved[2]?.cursor,
  "stream-1",
  "the unmatched streaming block still lands at the end even though its timestamp precedes the second canonical block's",
);
assert(
  (interleaved[1]?.timestamp ?? "") > (interleaved[2]?.timestamp ?? ""),
  "demonstrates the ordering gap: the block at index 1 has a later timestamp than the appended block at index 2, "
    + "yet the appended block is placed after it",
);

// --- multi-tick stability: repeated merges with a growing same-cursor -------
// streaming block keep the canonical block replaced in place, tick after
// tick, without accumulating extra entries.

const base = [block({ cursor: "0", text: "Session started", role: "agent" })];

const tick1 = mergeStreamingTranscriptBlocks(base, {
  "0": block({ cursor: "0", text: "Session started, still", role: "agent" }),
});
const tick2 = mergeStreamingTranscriptBlocks(base, {
  "0": block({ cursor: "0", text: "Session started, still streaming more", role: "agent" }),
});
const tick3 = mergeStreamingTranscriptBlocks(base, {
  "0": block({ cursor: "0", text: "Session started, still streaming more text now complete", role: "agent" }),
});

assertEqual(tick1.length, 1, "tick 1: colliding overlay keeps a single merged block");
assertEqual(tick2.length, 1, "tick 2: colliding overlay keeps a single merged block");
assertEqual(tick3.length, 1, "tick 3: colliding overlay keeps a single merged block");
assertEqual(tick1[0]?.text, "Session started, still", "tick 1 reflects the overlay's growing text");
assertEqual(tick2[0]?.text, "Session started, still streaming more", "tick 2 reflects the overlay's growing text");
assertEqual(
  tick3[0]?.text,
  "Session started, still streaming more text now complete",
  "tick 3 reflects the overlay's fully-grown text",
);
