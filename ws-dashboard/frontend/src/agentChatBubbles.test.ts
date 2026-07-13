import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import {
  AgentChatTranscriptBubbles,
  groupTranscriptIntoBubbles,
} from "./agentChatBubbles.js";
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

// --- groupTranscriptIntoBubbles ---------------------------------------------

const demoBlocks: TranscriptBlock[] = [
  block({ cursor: "0", text: "Session started", role: "agent" }),
  block({ cursor: "1", text: "Show me a tool call", role: "user" }),
  block({
    cursor: "2",
    text: "Plan the lookup",
    renderKind: "thinking",
    role: "agent",
    turnId: "turn-tool",
  }),
  block({
    cursor: "3",
    title: "Tool call",
    text: "Called stub.lookup",
    role: "tool",
    turnId: "turn-tool",
    data: { name: "stub.lookup", argumentsBytes: 42 },
  }),
  block({
    cursor: "4",
    title: "Tool output",
    text: "stub.lookup completed",
    role: "tool",
    turnId: "turn-tool",
    data: { outcome: "ok", exitCode: 0, outputBytes: 128 },
  }),
];

const bubbles = groupTranscriptIntoBubbles(demoBlocks, "agent.codex");

assertEqual(bubbles.length, 4, "session/user/thinking/tool blocks group into four bubbles");
assertEqual(bubbles[0]?.kind, "agentTurn", "session-started block is a plain agent-turn bubble");
assertEqual(bubbles[0]?.align, "left", "agent-turn bubbles are left-aligned");
assertEqual(bubbles[1]?.kind, "user", "user-role block becomes a user bubble");
assertEqual(bubbles[1]?.align, "right", "user bubbles are right-aligned");
assertEqual(
  bubbles[2]?.thinking.length,
  1,
  "a thinking block with no currently-open matching-turn bubble becomes its own collapsible bubble",
);
assertEqual(bubbles[3]?.kind, "tool", "tool-call/tool-result pair becomes a tool bubble");
assertEqual(
  bubbles[3]?.blocks.length,
  2,
  "tool call and tool result sharing a turnId merge into one bubble",
);
assert(
  bubbles[3]?.view?.summary.includes("exit 0"),
  "tool bubble summary reuses transcriptBlockView's tool-result heuristic (reflects the merged pair's result block)",
);
assert(
  bubbles[3]?.text.includes("stub.lookup"),
  "merged tool bubble retains the tool-call block's text alongside the result",
);

// --- streaming: same cursor, growing text, re-grouped each tick ------------

function streamingBlocksAt(text: string): TranscriptBlock[] {
  return [
    block({ cursor: "stream-1", text, role: "agent", turnId: "turn-stream" }),
  ];
}

const partial = groupTranscriptIntoBubbles(streamingBlocksAt("Here is the streamed"), "agent.codex");
const full = groupTranscriptIntoBubbles(
  streamingBlocksAt("Here is the streamed\n\n- point one\n- point two"),
  "agent.codex",
);
assertEqual(partial.length, 1, "a single growing block still yields exactly one bubble");
assertEqual(partial[0]?.text, "Here is the streamed", "bubble text reflects the partial chunk");
assertEqual(
  full[0]?.text,
  "Here is the streamed\n\n- point one\n- point two",
  "bubble text reflects the fully-grown chunk",
);

const partialHtml = renderToStaticMarkup(
  createElement(AgentChatTranscriptBubbles, {
    blocks: streamingBlocksAt("partial line one"),
    sourceKind: "agent.codex",
  }),
);
assert(partialHtml.includes("partial line one"), "intermediate streamed text renders through the markdown pipeline");

const fullHtml = renderToStaticMarkup(
  createElement(AgentChatTranscriptBubbles, {
    blocks: streamingBlocksAt("- alpha\n- beta"),
    sourceKind: "agent.codex",
  }),
);
assert(fullHtml.includes("<li"), "final streamed markdown renders semantic list markup, not raw text");
assert(fullHtml.includes("alpha") && fullHtml.includes("beta"), "final streamed content is fully present");

// --- collapsible thinking block defaults to collapsed ----------------------

const thinkingHtml = renderToStaticMarkup(
  createElement(AgentChatTranscriptBubbles, {
    blocks: [
      block({
        cursor: "t1",
        text: "hidden reasoning detail should not leak by default",
        renderKind: "thinking",
        role: "agent",
      }),
    ],
    sourceKind: "agent.codex",
  }),
);
assert(
  !thinkingHtml.includes("hidden reasoning detail should not leak by default"),
  "thinking block detail text is absent from static markup until expanded",
);
assert(
  thinkingHtml.includes("Show thinking"),
  "thinking block renders a collapsed-state toggle by default",
);

// --- copy button affordance on every bubble kind ---------------------------

const copyAffordanceHtml = renderToStaticMarkup(
  createElement(AgentChatTranscriptBubbles, { blocks: demoBlocks, sourceKind: "agent.codex" }),
);
const copyButtonCount = (copyAffordanceHtml.match(/agent-chat-bubble-copy/g) ?? []).length;
assertEqual(
  copyButtonCount,
  bubbles.length,
  "every rendered bubble (user, agent-turn, tool-use) includes a copy-button affordance",
);
