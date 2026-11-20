---
title: "agy 모델 대기 타임아웃 시 agy 프로세스를 종료하고 재기동 또는 재개"
blocked-by: 261117-feat-ws-exec-tools
related:
  261117-feat-ws-exec-tools: "prerequisite; exec must first separate exec lifetime from the agy harness process lifetime, so killing agy no longer kills running exec jobs"
---

# agy model-wait timeout kills and respawns or resumes the agy process

## Background

Owner directive, 2026-11-20 23:48 (verbatim):

> 이거 메인 세션도 알아야 할 의제인데, 일단 agy는 exec 쪽 구현해서 exec 수명이랑
> agy 하네스 수명 분리하고 나면 타임아웃 에러 떴을 때 아예 프로세스를 종료하는
> 식으로 대응해야 할 거 같아요. 프로세스 재로딩 이후에 해결되는지도 확인해보긴
> 해야할 것 같아서요.

In English: once ws/exec separates exec lifetime from the agy harness process
lifetime, a hosted agy model-wait timeout should terminate the agy process
outright instead of nudging it. Whether a process reload actually clears the
stall must be verified.

Trigger incident, 2026-11-20: a hosted agy worker hit a persistent stall. Its
automatic `continue` attempts failed. A lead `mailbox.send` retry failed, and
so did an interrupt followed by a send. A freshly spawned agy worker in the
same worktree, on the same task, then ran normally. The stall therefore looks
bound to the live agy process or its ACP session, not to the task, the
worktree, or the model service.

Current behavior: the watchdog in `internal/wsagent/agy_watchdog.go` arms on
raw ACP `session/update` silence (`DefaultAgyModelWaitTimeoutSeconds`, raised
to 120 s on branch `impl/develop/agy-model-wait-120`). On timeout,
`modelWaitTimedOut` interrupts the process and, while the retry budget
(`DefaultAgyModelWaitRetries` = 3) lasts, sends `continue` after a jittered
backoff. When the budget is exhausted, it fails the turn. The agy process and
its ACP session survive every step, so a stall bound to that process persists
into the next turn.

## Correction (2026-11-21)

The "bound to the live process or session" premise above did not hold. A
read-only diagnosis built from agy's own transcript timestamps found:
- The fresh worker hit the same stall within minutes. The healthy-looking agy
  lead session got 27 automatic `continue` messages that day and had 2 turns
  fail after the retry budget ran out.
- The agy process, its ACP session, and the cached context (about 90K tokens)
  were alive throughout. No permission request was open.
- Cause: gemini-4-neon-high streams nothing over ACP while it generates a
  large `edit_file` call. The silence timer fired, `session/cancel` discarded
  the edit, and `continue` regenerated the same edit into the same cutoff.
  The retry budget is per turn (reset only in `disarmModelWait`), so earlier
  stalls used up the retries that later ones needed.
- ws cannot tell long tool-call generation apart from a dead upstream; only
  agy can. agy's own 10-minute stream idle timeout surfaces in-band as
  `⚠ API error (attempt N): request failed: stream idle timeout (10m0s): ...`.

Owner answer (1472), 2026-11-21 00:18: switch off ws's silence detection and
keep only agy's in-band error handling with our automatic `continue`. Landed
as develop 586c0c2cc (`agySilenceWatchdogEnabled = false` in
`agy_watchdog.go`), with the daemon swapped.

What remains for this ticket: kill-and-respawn applies only when agy's
in-band failure retries are exhausted, or when a real upstream hang leaves
the process unrecoverable. Before this ticket is promoted, check whether that
case still occurs with silence detection off. If it does not, drop the
ticket.

## Phases

### Phase 1: Kill and respawn or resume agy on model-wait exhaustion

Goals:
- When the model-wait retry budget is exhausted, or, if it proves better, on
  the first timeout, the runner terminates the agy process (process tree).
  It then either respawns it with ACP session resume (`session/load`, when
  agy advertises it) or starts a fresh session that carries enough context to
  continue the turn. The ws agent record, reply id, and mailbox identity stay
  the same; the parent sees one continued turn or one clean failure, never a
  silent hang.
- Running `ws/exec` jobs owned by the agent survive the agy process kill. This
  is the dependency on 261117-feat-ws-exec-tools.

Open questions to settle before ready:
- Kill on the first timeout, or only after the in-process `continue` budget is
  exhausted. Evidence from the verification below should decide it.
- Resume (keeping the ACP session) versus a fresh session. If the stall is
  bound to the ACP session state rather than to the process, resume may carry
  the stall over. The verification below must distinguish the two.
- The interaction with the `held` and `approvals` guards in
  `modelWaitTimedOut`, and with the backoff fence.

Verification (required):
- Prove that a process reload clears the per-session stall. Use a reproduction
  derived from the 2026-11-20 stall diagnosis: reproduce the stuck state
  (scripted ACP fixture first, in the style of `agy_fixture_test.go`). Show
  that interrupt plus `continue` does not recover it, and that kill plus
  respawn does, both with resume and with a fresh session. If a fixture cannot
  reproduce the real stall, record what the diagnosis identified as the
  per-session cause and test that cause directly.
- Show that an exec job started before the kill keeps running and remains
  readable by the respawned agent.
- Real-agy proof needs the owner's explicit approval at run time (the real
  `agy` binary is not run otherwise).

Compatibility: the expected tier is T0. Runner behavior changes only; no MCP
tool, hello, store shape, or manifest field changes. Confirm this when the
design settles; if a new config key is added, it is a manifest addition and
still T0.
