// `260711-feat-ws-dashboard-agent-activity-chat-ui` Phase 2 — messenger-style
// bubble layout for the agent chat transcript.
//
// CONTRACT: this module derives chat bubbles from `TranscriptBlock[]` (the
// shape that mirrors the eventual `260620` daemon projection API) rather than
// inventing a separate "chat message" model. It reuses:
//   - `documentViewer.tsx`'s `renderMarkdownFragment` for Obsidian-flavored
//     markdown rendering (no second markdown parser);
//   - `workRootActivity.ts`'s `transcriptBlockView`/`transcriptCompactSummary`
//     heuristics for tool-use bubble summary/tone (no new summarization
//     logic);
//   - `documentViewer.tsx`'s existing `navigator.clipboard?.writeText(text)`
//     copy pattern (no new clipboard abstraction).
// Turn/tool-invocation grouping keys off the additive, optional
// `TranscriptBlock.turnId`/`role` fields (see the CONTRACT comment next to
// their definition in `workRootActivity.ts`) — when a real adapter does not
// populate them yet, every block simply becomes its own bubble, which is a
// safe/degenerate fallback rather than a crash or a miscategorization.

import { useState, type ReactNode } from "react";
import { renderMarkdownFragment } from "./documentViewer.js";
import type { AgentChatCapabilities } from "./agentChatSessions.js";
import {
  ResumeFromHereButton,
  isResumeFromHereEnabled,
} from "./agentChatResumeFromHere.js";
import {
  transcriptBlockView,
  type TranscriptBlock,
  type TranscriptBlockView,
} from "./workRootActivity.js";

export type ChatBubbleKind = "user" | "agentTurn" | "tool";

export type ChatBubbleThinkingEntry = {
  id: string;
  text: string;
};

export type ChatBubble = {
  id: string;
  kind: ChatBubbleKind;
  align: "left" | "right";
  blocks: TranscriptBlock[];
  text: string;
  view: TranscriptBlockView | null;
  thinking: ChatBubbleThinkingEntry[];
  degraded: boolean;
};

function chatBlockRole(block: TranscriptBlock): "user" | "agent" | "tool" | "thinking" {
  if (block.role === "user") {
    return "user";
  }
  if (block.renderKind === "thinking") {
    return "thinking";
  }
  if (block.role === "tool") {
    return "tool";
  }
  const key = `${block.renderKind} ${block.title ?? ""}`.toLowerCase();
  if (key.includes("tool") || key.includes("mcp")) {
    return "tool";
  }
  return "agent";
}

type OpenBubble = ChatBubble & { turnKey: string | null };

/**
 * Partition a flat transcript block list into per-turn agent bubbles,
 * per-tool-invocation bubbles, user bubbles, and thinking entries attached to
 * their surrounding agent bubble. Pure and side-effect free so it is directly
 * unit-testable against synthetic/partial (streaming) block lists.
 */
export function groupTranscriptIntoBubbles(
  blocks: readonly TranscriptBlock[],
  sourceKind: string,
): ChatBubble[] {
  const bubbles: ChatBubble[] = [];
  let open: OpenBubble | null = null;

  const flush = () => {
    if (open) {
      const { turnKey: _turnKey, ...bubble } = open;
      bubbles.push(bubble);
    }
    open = null;
  };

  for (const block of blocks) {
    const role = chatBlockRole(block);
    if (role === "thinking") {
      const entry: ChatBubbleThinkingEntry = { id: block.cursor, text: block.text ?? "" };
      // A thinking segment attaches to the bubble currently being built only
      // when it shares that bubble's turn (the real-adapter case: text,
      // thinking, more text, all one turn). Otherwise it stands alone as its
      // own collapsible bubble, interleaved in transcript order between the
      // surrounding agent/tool bubbles — still a safe default when no
      // `turnId` correlation exists yet.
      if (open && block.turnId != null && open.turnKey === block.turnId) {
        open.thinking.push(entry);
      } else {
        flush();
        bubbles.push({
          id: block.cursor,
          kind: "agentTurn",
          align: "left",
          blocks: [block],
          text: "",
          view: null,
          thinking: [entry],
          degraded: block.degraded,
        });
      }
      continue;
    }

    const kind: ChatBubbleKind = role === "user" ? "user" : role === "tool" ? "tool" : "agentTurn";
    const turnKey = block.turnId ?? null;
    const canMerge =
      open !== null &&
      open.kind === kind &&
      kind !== "user" &&
      turnKey !== null &&
      open.turnKey === turnKey;

    if (canMerge && open) {
      open.blocks.push(block);
      open.text = [open.text, block.text ?? ""].filter(Boolean).join("\n");
      if (block.degraded) open.degraded = true;
      if (kind === "tool") open.view = transcriptBlockView(block, sourceKind);
      continue;
    }

    flush();
    open = {
      id: block.cursor,
      kind,
      align: kind === "user" ? "right" : "left",
      blocks: [block],
      text: block.text ?? "",
      view: kind === "tool" ? transcriptBlockView(block, sourceKind) : null,
      thinking: [],
      degraded: block.degraded,
      turnKey,
    };
  }
  flush();

  return bubbles;
}

function copyText(text: string) {
  void navigator.clipboard?.writeText(text);
}

function CopyButton({ text, label }: { text: string; label: string }) {
  return (
    <button
      type="button"
      className="agent-chat-bubble-copy"
      data-command-id="agentChat.bubble.copy"
      aria-label={label}
      onClick={() => copyText(text)}
    >
      Copy
    </button>
  );
}

function ThinkingBlock({ entry }: { entry: ChatBubbleThinkingEntry }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div
      className="agent-chat-thinking-block"
      data-agent-chat-thinking-expanded={expanded ? "true" : "false"}
    >
      <button
        type="button"
        className="agent-chat-thinking-toggle"
        data-command-id="agentChat.thinking.toggle"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
      >
        {expanded ? "Hide thinking" : "Show thinking"}
      </button>
      {expanded ? (
        <div className="agent-chat-thinking-detail">{renderMarkdownFragment(entry.text)}</div>
      ) : null}
    </div>
  );
}

function BubbleShell({
  bubble,
  className,
  children,
}: {
  bubble: ChatBubble;
  className: string;
  children: ReactNode;
}) {
  return (
    <div
      className={`agent-chat-bubble ${className}${bubble.degraded ? " is-degraded" : ""}`}
      data-agent-chat-bubble-kind={bubble.kind}
      data-agent-chat-bubble-align={bubble.align}
    >
      {bubble.thinking.length > 0
        ? bubble.thinking.map((entry) => <ThinkingBlock entry={entry} key={entry.id} />)
        : null}
      {children}
    </div>
  );
}

// CONTRACT: `capabilities`/`onForkFromBubble`/`onResumeFromBubble` are
// optional and additive — every other bubble kind (`AgentTurnBubble`,
// `ToolUseBubble`) is untouched, since "fork from here"/"resume from here"
// are per-bubble affordances scoped to user turns only (Phase 3,
// `260711-feat-ws-dashboard-agent-activity-chat-ui`). "Fork from here"
// renders only when `capabilities.fork` is true (hidden, not disabled, for
// Unavailable-tier cells, per the ticket's Constraints). "Resume from here"
// is rendered through the isolated `agentChatResumeFromHere.tsx` module,
// gated on `isResumeFromHereEnabled` — which always returns `false` today
// (see that module's header) — so this branch never actually renders for
// any current harness.
function UserBubble({
  bubble,
  capabilities,
  onForkFromBubble,
  onResumeFromBubble,
}: {
  bubble: ChatBubble;
  capabilities?: AgentChatCapabilities;
  onForkFromBubble?: (bubble: ChatBubble) => void;
  onResumeFromBubble?: (bubble: ChatBubble) => void;
}) {
  return (
    <BubbleShell bubble={bubble} className="agent-chat-bubble-user">
      <div className="agent-chat-bubble-body">{renderMarkdownFragment(bubble.text)}</div>
      <div className="agent-chat-bubble-actions">
        <CopyButton text={bubble.text} label="Copy message" />
        {capabilities?.fork ? (
          <button
            type="button"
            className="agent-chat-bubble-fork"
            data-command-id="agentChat.bubble.forkFromHere"
            onClick={() => onForkFromBubble?.(bubble)}
          >
            Fork from here
          </button>
        ) : null}
        {capabilities && isResumeFromHereEnabled(capabilities) ? (
          <ResumeFromHereButton onResume={() => onResumeFromBubble?.(bubble)} />
        ) : null}
      </div>
    </BubbleShell>
  );
}

function AgentTurnBubble({ bubble }: { bubble: ChatBubble }) {
  return (
    <BubbleShell bubble={bubble} className="agent-chat-bubble-agent">
      {bubble.text ? (
        <div className="agent-chat-bubble-body">{renderMarkdownFragment(bubble.text)}</div>
      ) : null}
      <CopyButton text={bubble.text} label="Copy message" />
    </BubbleShell>
  );
}

function ToolUseBubble({ bubble }: { bubble: ChatBubble }) {
  const [expanded, setExpanded] = useState(false);
  const view = bubble.view;
  const detailVisible = expanded || view?.mode === "expanded" || view?.mode === "terminal";
  const copyText = view?.detail ?? bubble.text;
  return (
    <BubbleShell bubble={bubble} className="agent-chat-bubble-tool">
      <div className="agent-chat-bubble-tool-head">
        <span className="agent-chat-bubble-tool-summary">{view?.summary ?? bubble.text}</span>
        {view && view.mode === "compact" && view.detail ? (
          <button
            type="button"
            className="agent-chat-bubble-tool-toggle"
            data-command-id="agentChat.tool.toggle"
            aria-expanded={expanded}
            onClick={() => setExpanded((current) => !current)}
          >
            {expanded ? "Less" : "More"}
          </button>
        ) : null}
      </div>
      {detailVisible && view?.detail ? (
        <pre className="agent-chat-bubble-tool-detail">{view.detail}</pre>
      ) : null}
      <CopyButton text={copyText} label="Copy tool detail" />
    </BubbleShell>
  );
}

export function ChatBubbleView({
  bubble,
  capabilities,
  onForkFromBubble,
  onResumeFromBubble,
}: {
  bubble: ChatBubble;
  capabilities?: AgentChatCapabilities;
  onForkFromBubble?: (bubble: ChatBubble) => void;
  onResumeFromBubble?: (bubble: ChatBubble) => void;
}) {
  if (bubble.kind === "user") {
    return (
      <UserBubble
        bubble={bubble}
        capabilities={capabilities}
        onForkFromBubble={onForkFromBubble}
        onResumeFromBubble={onResumeFromBubble}
      />
    );
  }
  if (bubble.kind === "tool") {
    return <ToolUseBubble bubble={bubble} />;
  }
  return <AgentTurnBubble bubble={bubble} />;
}

export function AgentChatTranscriptBubbles({
  blocks,
  sourceKind,
  capabilities,
  onForkFromBubble,
  onResumeFromBubble,
}: {
  blocks: readonly TranscriptBlock[];
  sourceKind: string;
  capabilities?: AgentChatCapabilities;
  onForkFromBubble?: (bubble: ChatBubble) => void;
  onResumeFromBubble?: (bubble: ChatBubble) => void;
}) {
  const bubbles = groupTranscriptIntoBubbles(blocks, sourceKind);
  return (
    <>
      {bubbles.map((bubble) => (
        <ChatBubbleView
          bubble={bubble}
          capabilities={capabilities}
          onForkFromBubble={onForkFromBubble}
          onResumeFromBubble={onResumeFromBubble}
          key={bubble.id}
        />
      ))}
    </>
  );
}
