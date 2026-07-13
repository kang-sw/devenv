import { blocksSincePolledLength, mergeStreamingTranscriptBlocks } from "./agentChatStreamMerge.js";
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

// --- blocksSincePolledLength: diffing a full-refetch poll against the -------
// previously-seen block count.

const pollBlocks = [
  block({ cursor: "0", text: "user prompt", role: "user" }),
  block({ cursor: "1", text: "agent reply, still streaming", role: "agent" }),
  block({ cursor: "2", text: "a brand new block", role: "agent" }),
];

// Initial poll (lastSeenLength 0, and the plan's documented <= 0 case)
// returns everything: there is no previously-seen tail block to re-diff.
assertEqual(
  blocksSincePolledLength(pollBlocks, 0).length,
  3,
  "an initial poll (lastSeenLength 0) returns the full block array",
);
assertEqual(
  blocksSincePolledLength(pollBlocks, -1).length,
  3,
  "a negative lastSeenLength is clamped to the full block array, same as 0",
);

// A subsequent poll re-includes the last previously-seen block (it may have
// grown) plus any newly appended blocks after it.
const diffAfterOne = blocksSincePolledLength(pollBlocks, 1);
assertEqual(
  diffAfterOne.length,
  3,
  "lastSeenLength 1 re-includes the previously-seen tail block (index 0) plus the two new blocks",
);
assertEqual(diffAfterOne[0]?.cursor, "0", "the re-included tail block is the previously-seen block at index 0");

const diffAfterTwo = blocksSincePolledLength(pollBlocks, 2);
assertEqual(
  diffAfterTwo.length,
  2,
  "lastSeenLength 2 re-includes the tail block at index 1 plus the one new block at index 2",
);
assertEqual(diffAfterTwo[0]?.cursor, "1", "the re-included tail block is the previously-seen block at index 1");
assertEqual(diffAfterTwo[1]?.cursor, "2", "the newly appended block at index 2 is included");

// --- blocksSincePolledLength: multi-poll stability (idempotence) -----------
// Repeated calls with a stable lastSeenLength against an unchanged tail
// block yield the same diff every time.

const stableBlocks = [
  block({ cursor: "0", text: "user prompt", role: "user" }),
  block({ cursor: "1", text: "agent reply complete", role: "agent" }),
];
const stableDiffA = blocksSincePolledLength(stableBlocks, stableBlocks.length);
const stableDiffB = blocksSincePolledLength(stableBlocks, stableBlocks.length);
const stableDiffC = blocksSincePolledLength(stableBlocks, stableBlocks.length);
// Each call is checked against a hardcoded oracle (not just against each
// other) so this cannot pass vacuously as a self-comparison of a pure
// function against itself.
for (const [label, diff] of [
  ["poll 1", stableDiffA],
  ["poll 2", stableDiffB],
  ["poll 3", stableDiffC],
] as const) {
  assertEqual(diff.length, 1, `${label}: the stable diff contains only the unchanged tail block, no phantom growth`);
  assertEqual(diff[0]?.cursor, "1", `${label}: the stable diff's sole block is the unchanged tail block at cursor 1`);
  assertEqual(
    diff[0]?.text,
    "agent reply complete",
    `${label}: the stable diff's sole block text matches the unchanged tail block verbatim`,
  );
}

// --- blocksSincePolledLength: unmatched-append-ordering ---------------------
// A poll that appends more than one new block at once still diffs correctly,
// and merging the diff back through mergeStreamingTranscriptBlocks (keyed by
// cursor) produces the same result as merging the full array would.

const beforeMultiAppend = [
  block({ cursor: "0", text: "user prompt", role: "user" }),
  block({ cursor: "1", text: "agent reply, still streaming", role: "agent" }),
];
const afterMultiAppend = [
  block({ cursor: "0", text: "user prompt", role: "user" }),
  block({ cursor: "1", text: "agent reply, now complete", role: "agent" }),
  block({ cursor: "2", text: "tool call block", role: "tool" }),
  block({ cursor: "3", text: "second agent block appended in the same poll", role: "agent" }),
];

const multiAppendDiff = blocksSincePolledLength(afterMultiAppend, beforeMultiAppend.length);
assertEqual(
  multiAppendDiff.length,
  3,
  "a poll appending two new blocks at once diffs to the re-included tail block plus both new blocks",
);
assertEqual(multiAppendDiff[0]?.cursor, "1", "the diff re-includes the previously-seen tail block at cursor 1");
assertEqual(multiAppendDiff[1]?.cursor, "2", "the diff includes the first newly appended block at cursor 2");
assertEqual(multiAppendDiff[2]?.cursor, "3", "the diff includes the second newly appended block at cursor 3, in poll order");

const streamingOverlay: Record<string, ReturnType<typeof block>> = {};
for (const diffBlock of multiAppendDiff) {
  streamingOverlay[diffBlock.cursor] = diffBlock;
}
const mergedFromDiff = mergeStreamingTranscriptBlocks(beforeMultiAppend, streamingOverlay);
assertEqual(
  mergedFromDiff.length,
  4,
  "merging the diffed slice through mergeStreamingTranscriptBlocks yields the full 4-block transcript",
);
assertEqual(
  mergedFromDiff.map((b) => b.cursor).join(","),
  "0,1,2,3",
  "merging the diff produces blocks in the same cursor order as the full poll response",
);
assertEqual(
  mergedFromDiff[1]?.text,
  "agent reply, now complete",
  "merging the diff updates the previously-streaming tail block's text in place",
);
