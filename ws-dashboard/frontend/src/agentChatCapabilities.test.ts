// `260711-feat-ws-dashboard-agent-activity-chat-ui` Phase 3 gating tests —
// asserts per-harness-capability-gated bubble affordances render/hide
// correctly against hand-constructed `AgentChatCapabilities` values (not the
// stub's harness table), so every combination is exercised directly,
// including the ticket's explicit "resume from here never renders,
// regardless of capability combination" assertion.

import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { AgentChatTranscriptBubbles } from "./agentChatBubbles.js";
import { isResumeFromHereEnabled } from "./agentChatResumeFromHere.js";
import type { AgentChatCapabilities } from "./agentChatSessions.js";
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

function capabilities(overrides: Partial<AgentChatCapabilities> = {}): AgentChatCapabilities {
  return {
    compact: false,
    steer: false,
    goal: false,
    rewind: false,
    fork: false,
    skills: false,
    ...overrides,
  };
}

const userBlock: TranscriptBlock = {
  cursor: "u1",
  timestamp: null,
  renderKind: "markdown",
  title: null,
  text: "revert this bubble to fork/resume from",
  data: null,
  degraded: false,
  role: "user",
  turnId: undefined,
};

function renderUserBubble(caps: AgentChatCapabilities): string {
  return renderToStaticMarkup(
    createElement(AgentChatTranscriptBubbles, {
      blocks: [userBlock],
      sourceKind: "agent.codex",
      capabilities: caps,
      onForkFromBubble: () => undefined,
      onResumeFromBubble: () => undefined,
    }),
  );
}

// --- "fork from here" renders only when capabilities.fork is true ----------

const forkEnabledHtml = renderUserBubble(capabilities({ fork: true }));
assert(
  forkEnabledHtml.includes('data-command-id="agentChat.bubble.forkFromHere"'),
  "fork from here renders when capabilities.fork is true",
);

const forkDisabledHtml = renderUserBubble(capabilities({ fork: false }));
assert(
  !forkDisabledHtml.includes('data-command-id="agentChat.bubble.forkFromHere"'),
  "fork from here is hidden (not just disabled) when capabilities.fork is false",
);

// --- "resume from here" never renders, regardless of capability combination,
// including the explicit ticket-called-out `{ rewind: true, ...everything
// else true }` combination. --------------------------------------------------

const capabilityCombinations: AgentChatCapabilities[] = [
  capabilities(),
  capabilities({ rewind: true }),
  capabilities({ fork: true, rewind: true }),
  capabilities({ compact: true, steer: true, goal: true, rewind: true, fork: true, skills: true }),
];

for (const [index, caps] of capabilityCombinations.entries()) {
  const html = renderUserBubble(caps);
  assert(
    !html.includes('data-command-id="agentChat.bubble.resumeFromHere"'),
    `combination ${index}: resume from here never renders (capabilities=${JSON.stringify(caps)})`,
  );
  assertEqual(
    isResumeFromHereEnabled(caps),
    false,
    `combination ${index}: isResumeFromHereEnabled always returns false, even with rewind:true and every other capability true`,
  );
}
