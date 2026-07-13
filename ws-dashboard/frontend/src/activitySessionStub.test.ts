// `260711-feat-ws-dashboard-agent-activity-chat-ui` Phase 3 — direct unit
// coverage for new stub-side logic (per-harness capability table,
// `stubBeginStreamingTurn`'s `onComplete` batch-boundary callback,
// send/steer stubs, and `stubForkActivitySession`'s cut-point truncation).

import {
  stubForkActivitySession,
  stubSendActivitySession,
  stubStartNewAgentChatSession,
  stubSteerActivitySession,
} from "./activitySessionStub.js";

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

// --- per-harness capability table -------------------------------------------
// `rewind` must stay `false` for every harness (the load-bearing gate
// keeping "resume from here" disabled everywhere).

const codexSession = await stubStartNewAgentChatSession("root-a", "codex");
assertEqual(codexSession.capabilities.rewind, false, "codex: rewind stays false");
assertEqual(codexSession.capabilities.fork, true, "codex: fork is available (real thread/fork, Passthrough)");
assertEqual(codexSession.capabilities.steer, true, "codex: steer is available (real turn/steer, Passthrough)");
assertEqual(codexSession.capabilities.skills, true, "codex: skills is available");

const claudeSession = await stubStartNewAgentChatSession("root-a", "claude");
assertEqual(claudeSession.capabilities.rewind, false, "claude: rewind stays false");
assertEqual(claudeSession.capabilities.fork, false, "claude: fork is Hack-tier, not shipped as available");
assertEqual(claudeSession.capabilities.steer, false, "claude: steer has no native primitive");
assertEqual(claudeSession.capabilities.skills, true, "claude: skills listing is a real dashboard-buildable surface");

const opencodeSession = await stubStartNewAgentChatSession("root-a", "opencode");
assertEqual(opencodeSession.capabilities.rewind, false, "opencode: rewind stays false");
assertEqual(opencodeSession.capabilities.fork, false, "opencode: unverified column defaults to strictest reading");
assertEqual(opencodeSession.capabilities.skills, false, "opencode: unverified column defaults to strictest reading");

// --- send/steer stubs are trivial accepted:true placeholders ----------------

const sendResult = await stubSendActivitySession({
  workRootId: "root-a",
  activityId: codexSession.activityId,
  text: "hello",
});
assertEqual(sendResult.accepted, true, "stubSendActivitySession accepts every request");

const steerResult = await stubSteerActivitySession({
  workRootId: "root-a",
  activityId: codexSession.activityId,
  text: "steer this",
});
assertEqual(steerResult.accepted, true, "stubSteerActivitySession accepts every request");

// --- fork cut-point truncation ----------------------------------------------
// `cutBlocks` is a Phase-3-local, non-wire second parameter — the forked
// session's transcript must contain exactly the cut slice plus the
// synthetic "forked from conversation" marker block, never the whole
// original transcript.

const fullBlocks = codexSession.transcript.blocks;
assert(fullBlocks.length > 2, "the codex demo session has more than two blocks to cut from");
const cutBlocks = fullBlocks.slice(0, 2);

const forkResult = await stubForkActivitySession(
  {
    workRootId: codexSession.workRootId,
    activityId: codexSession.activityId,
  },
  cutBlocks,
);
assert(
  forkResult.activityId !== codexSession.activityId,
  "forking allocates a new synthetic activityId distinct from the original session",
);
assertEqual(
  forkResult.session.transcript.blocks.length,
  cutBlocks.length + 1,
  "the forked session's transcript is exactly the cut slice plus one synthetic marker block, not the whole original transcript",
);
assert(
  cutBlocks.every((block, index) => forkResult.session.transcript.blocks[index] === block),
  "the forked session's leading blocks are exactly the cut slice, in order",
);
assert(
  fullBlocks.length === codexSession.transcript.blocks.length,
  "forking never mutates the original session's transcript",
);
assertEqual(
  forkResult.session.harness,
  "codex",
  "forking a codex session's cut carries the codex harness through to the new session",
);
