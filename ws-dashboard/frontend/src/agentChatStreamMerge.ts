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

export { mergeStreamingTranscriptBlocks };
