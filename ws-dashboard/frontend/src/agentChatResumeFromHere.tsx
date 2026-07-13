// `260711-feat-ws-dashboard-agent-activity-chat-ui` Phase 3 — isolated
// "resume from here" scaffold.
//
// CONTRACT (owner decision, ticket Decisions text): "resume from here" must
// be its own isolated component/module behind its own capability gate,
// separate from "fork from here" and the shared bubble/turn rendering —
// removable/disable-able by flipping one flag or deleting one module. This
// entire module is that isolation boundary.
//
// Per the Phase 3 plan's Resolved Strategic Question 1
// (`ai-docs/.plans/2026-07/13-1150-chat-ui-resume-fork-phase3.md`): no
// current harness qualifies as a clean Passthrough/Overlay match for
// point-based "resume from here" —
//   - Codex's only rewind primitive, `thread/rollback`, is Passthrough but
//     confirmed deprecated for removal, and is coarse turn-count-based
//     (drops N turns from the end), not point-based — wrong if turns were
//     forked/reordered, and does not revert file changes.
//   - Claude's only reachable rewind/fork path is a Hack (unofficial
//     transcript-file truncation) — Hack-tier cells require a dedicated
//     ticket with experimental UI labeling and owner risk sign-off before
//     backing any shipped method, which this phase is not.
//   - OpenCode's equivalent is unverified/unconfirmed (OpenCode not
//     installed).
// Also confirmed against `ws-dashboard/crates/core/src/agent_client_provider.rs`:
// no adapter anywhere backs `rewind` in a shipped route today, independent
// of the tiering question above.
//
// `isResumeFromHereEnabled` therefore always returns `false`, regardless of
// `capabilities.rewind` — this is deliberate, not a bug. Flipping it later
// (once a real, point-based rewind route exists for some harness) is
// exactly the one-flag isolation this module exists to provide. Do not wire
// this button to Codex's deprecated `thread/rollback` or any live
// `activity.session.rewind` call.

import type { AgentChatCapabilities } from "./agentChatSessions.js";

export function isResumeFromHereEnabled(
  capabilities: AgentChatCapabilities,
): boolean {
  void capabilities;
  return false;
}

export function ResumeFromHereButton({
  onResume,
}: {
  onResume: () => void;
}) {
  return (
    <button
      type="button"
      className="agent-chat-bubble-resume"
      data-command-id="agentChat.bubble.resumeFromHere"
      onClick={onResume}
    >
      Resume from here
    </button>
  );
}
