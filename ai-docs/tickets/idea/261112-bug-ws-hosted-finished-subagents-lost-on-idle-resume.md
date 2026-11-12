---
title: "Hosted sessions: finished subagents vanish from the dashboard after an idle stop and resume"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: Phase 2 idle stop and resume
  261110-feat-ws-dashboard-agent-panel-telemetry: agents panel shows retained finished agents
---

# Hosted sessions: finished subagents vanish from the dashboard after an idle stop and resume

## Background

Found while dogfooding a daemon-hosted Claude lead session (2026-11-12,
reply-id `autisticcavalierharpistdairy`, backend session
`0dc1bb52-5631-4a42-889a-7fff5fe22da5`).

- A background Explore subagent ran about 13 s (about 02:11-02:12Z). The user
  later looked and saw no trace of it on the dashboard.
- A second background Explore subagent (02:56:46-02:57:17Z) showed as running.
  After it finished, `snapshot.json` still listed it with `status: done`. So
  live display and finished-agent retention both work within one process.
- Between the two, the hosted process was replaced. `registration.json` and
  `agent.json` showed `host_pid` 67539 at 02:01Z and 114033 at 02:56Z, with
  the same `backend_session_id`. This is consistent with the Phase 2 idle stop
  and a resume on the next message. The conversation context survived.
- The snapshot at 02:57Z held only the second agent. `FinishedAgentCap` is 5
  (`agents-plugin-tool/internal/wslive/snapshot.go:60`, retention in
  `internal/wslive/state.go` `retain`), so the cap did not drop the first one.
- `ring/` entries are conversation-only (user, reply, tools, turn, mail). They
  carry no agent events, so the ring cannot restore agent rows.

Hypothesis (unverified): the live agent state is rebuilt empty when the
hosted process restarts, so finished subagents from before an idle stop
disappear. A short subagent that finishes before the user looks, followed by
an idle stop, leaves no trace on the page. Not yet ruled out: the page simply
missed a 13 s run, with no restart involved.

## Corrected cause (2026-11-12)

The title and the hypothesis above are outdated. An idle stop and resume
keeps the same per-session live state: `processEnded` marks running agents
failed through `endAgents`, and the conversation's live writer survives. The
rows are lost on a daemon restart (for example a dev rebuild replacing it):
`Runner.Start` calls `startConv`, which builds a fresh `newLiveWriter` and
writes the snapshot at once, overwriting the previous `snapshot.json`, which
nothing read back. Fix: the new live writer seeds the previous snapshot's
done and failed agents (`wslive.State.SeedFinished`) before its first write;
agents the old snapshot showed running are left out, and `retain` /
`FinishedAgentCap` still apply. Compatibility tier: T0 (reads an existing
file; snapshot and record shapes unchanged).

## Questions

- Does the daemon carry `agents[]` over a resume (from the previous
  snapshot), or start a new `wslive.State`? Confirm in the resume path.
- If it starts fresh, should finished agents from before the restart carry
  over (they are history), or is losing them acceptable with the agents
  panel saying so?
- Is there a cheaper signal, such as the lead's conversation-view Agent tool
  line, that already leaves enough trace?
