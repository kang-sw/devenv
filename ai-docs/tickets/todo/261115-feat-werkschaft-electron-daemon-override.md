---
title: "Electron shell owns the daemon: override a plugin-started daemon without stopping session holders"
related:
  261115-feat-werkschaft-electron-shell: "records (1115)/(1124)=b as a reversal of its Decision 2 'in a later release'; says Phases 1-3 keep Decision 2 until this phase is ticketed"
  261115-feat-ws-daemon-release-auto-update: "earlier user-click daemon swap from ws-mcp GitHub releases; predates (1115) and must be reconciled with this ticket"
---

# Electron shell owns the daemon: override a plugin-started daemon without stopping session holders

## Background

Whiteboard topic `ship-electron` (Electron 배포). The owner reversed the
shell's attach-only daemon rule (Decision 2 of
`261115-feat-werkschaft-electron-shell`). Owner (1115):

> electron이 데몬을 띄우는 역할입니다. 정확히는, 클로드 코드가 띄운 데몬이
> 있으면 일렉트론 데몬이 내리고 자기 것을 띄우는 식으로, 플러그인 데몬을
> 오버라이드하는 식으로 갑니다.

Timing (1124)=b: this does not ship in v0.50.0. It ships afterwards, through
the Electron auto-updater that did ship in v0.50.0 ((1116)=a, `4d950d704`).

The owner set one hard constraint (1124):

> 데몬 홀더라는 건 에이전트 세션 홀딩하는 그 파이썬 프로세스들을 말합니다.
> 이건 절대로 데몬 교체 때 임의로 내리지 않습니다. (세션 종료시에만 내려가는
> 프로세스)

The Electron ticket records this decision and says Phases 1-3 keep
Decision 2 "until that phase is ticketed". This ticket is that phase.

## Decisions

- When a plugin-started daemon (for example, one Claude Code started) is
  running, the shell replaces it with its own beside-exe daemon ((1115)).
- A daemon replacement never stops, signals or restarts a session holder
  process. Holders exit only when their session ends; they detach and
  re-attach to the new daemon ((1124)).
- It ships after v0.50.0 through the auto-updater ((1124)=b).

## Open Decisions

- Version precedence: a plugin launcher's newest-wins rule may replace the
  shell's daemon again with a newer plugin build. Which daemon wins, and how
  is a replace loop between the shell and a launcher avoided?
- Minor lockout: the beside-exe daemon may be a newer minor than an attached
  plugin session. Does the override warn and list affected sessions, as
  (868)=a does in `261115-feat-ws-daemon-release-auto-update`, or proceed
  silently?
- Relationship to `261115-feat-ws-daemon-release-auto-update`: does this
  override supersede that ticket's ws-mcp-release download-and-swap path,
  or do both remain?
- Compatibility tier for any change to the daemon handoff, `endpoint.json`
  or the lock, if one is needed.

## Phases

### Phase 1: Shell-owned daemon override

Acceptance:

- With a plugin-started daemon running and live hosted sessions attached,
  starting the shell replaces the daemon with its own build. Every holder
  process keeps its PID before and after, and every session reconnects. The
  test uses an isolated `WS_CACHE_HOME` and an isolated daemon, never the
  shared daemon.
- No shell code path stops, signals or kills a holder process. A test or
  audit asserts this.
- The behaviour settled under Open Decisions is covered by tests on macOS
  and Windows packaged builds.
