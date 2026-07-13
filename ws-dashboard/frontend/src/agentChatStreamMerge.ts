// `260711-feat-ws-dashboard-agent-activity-chat-ui` Phase 2 — overlays live
// streaming block updates onto the canonical transcript for `AgentChatPaneBody`.
//
// CONTRACT: pure and side-effect free so it is directly unit-testable
// (see `agentChatStreamMerge.test.ts`) independent of `App.tsx`'s render tree.

import type { TranscriptBlock } from "./workRootActivity.js";

/**
 * Merge a canonical transcript block list with an in-flight streaming-block
 * overlay keyed by cursor.
 *
 * - A streaming block whose cursor matches a canonical block's cursor
 *   *replaces* that canonical block wholesale (the streaming block is always
 *   assumed to be the newer, more-complete text for that cursor).
 * - A streaming block whose cursor has no canonical counterpart yet is
 *   appended after all canonical blocks, in `Object.values(streaming)`
 *   iteration order (insertion order of the `streaming` record) — this does
 *   not attempt to interleave it by timestamp relative to canonical blocks.
 */
function mergeStreamingTranscriptBlocks(
  blocks: readonly TranscriptBlock[],
  streaming: Record<string, TranscriptBlock>,
): TranscriptBlock[] {
  const merged = blocks.map((block) => streaming[block.cursor] ?? block);
  const appended = Object.values(streaming).filter(
    (block) => !blocks.some((existing) => existing.cursor === block.cursor),
  );
  return [...merged, ...appended];
}

/**
 * Diff a full-refetch poll's block array against the previously-seen block
 * count, returning the slice that a caller should hand to `onUpdate`.
 *
 * The daemon's transcript endpoint is full-refetch, not incremental (see
 * `activitySessionClient.ts`'s `beginRealStreamingTurn` poll loop) — each
 * poll returns every block from the start of the transcript, not just what
 * changed since the last poll. This helper re-derives the "what's new"
 * subset:
 *
 * - Blocks strictly before `lastSeenLength - 1` are dropped (already fully
 *   seen and assumed immutable once a later block exists after them).
 * - The block at index `lastSeenLength - 1` (the previously-seen tail block)
 *   is re-included, because it may have grown/mutated text since it was last
 *   the in-progress tail (e.g. a streaming agent block whose text is still
 *   being appended to by the daemon between polls).
 * - Any blocks at index `lastSeenLength` onward are newly appended since the
 *   last poll.
 *
 * `lastSeenLength <= 0` (including the initial poll) returns the full array,
 * since there is no previously-seen tail block to re-diff against.
 */
function blocksSincePolledLength(
  blocks: readonly TranscriptBlock[],
  lastSeenLength: number,
): TranscriptBlock[] {
  const start = Math.max(0, lastSeenLength - 1);
  return blocks.slice(start);
}

export { blocksSincePolledLength, mergeStreamingTranscriptBlocks };
