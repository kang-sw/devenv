---
title: 레일 하트비트 정체 근본 수정 (하트비트 sweeper 분리 + 세션별 잠금)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b6632e56b1fd0be6
sage-review-completeness-reviewed: b6632e56b1fd0be6
---

# Rail heartbeat starvation: independent heartbeat and per-session summary lock

## Background

On 2026-11-20 rail rows for live hosted sessions disappeared for about five
minutes and then returned one by one. Diagnosis (read-only, file:line
verified at develop b5842fabd):

- The conv loop owns the periodic `HeartbeatAt` refresh. Its default tick is
  every 30 s and it rewrites `registration.json` every
  `wsdashboard.HeartbeatInterval` (agents-plugin-tool/internal/wsagent/runner_conv.go#L478-L479,
  agents-plugin-tool/internal/wsagent/runner_conv.go#L566-L588).
- The beat called `c.live.heartbeatSnapshot()` before `writeRegistration()`.
  The snapshot publish takes the runner-wide `summaryMu`
  (`internal/wsagent/runner_summary.go`, `internal/wsagent/record.go`), which
  other sessions hold across ring I/O. One slow disk write can therefore stall
  other hosted lead sessions' snapshot publication and, once their loops block,
  their later registration heartbeats (agents-plugin-tool/internal/wsagent/runner_summary.go#L19-L29,
  agents-plugin-tool/internal/wsagent/record.go#L611-L624).
- The rail classifies a row whose heartbeat is older than five minutes as
  pending deletion before it checks "asleep"
  (`internal/wsdashboard/session_visibility.go`), so the row is hidden.
- The overnight 15-17 minute ws call hangs are plausibly the same stall;
  unproven, because no goroutine dump was captured.

Fix 1 (refresh registration before the snapshot) landed in 46dc9c67e as a
mitigation. It only reorders one tick: a conv loop already blocked inside
another `summaryMu` path still misses its heartbeat. This ticket is the root
fix.

## Decisions

- Owner (1831)=a, 2026-11-20: fix 1 lands immediately; fixes 2 and 3 below
  are captured in this ticket for the next cycle.
- Fix 2: the registration heartbeat no longer depends on the conv loop being
  free. A daemon-side heartbeat path refreshes `HeartbeatAt` for every live
  conv whose loop is blocked outside its own row lock. Shape: the sweeper does
  only a read-modify-write of `HeartbeatAt` on the existing row, under the
  per-session registration flock, and skips foreign rows; it never rebuilds
  the row from `c.record()`, so it cannot roll back a newer `HostPID` or
  `HostStopped` the loop wrote. The `lastReg` due time and `foreignRow` are
  synchronized between the loop's `heartbeat()` and the sweeper so the two
  never double-write within one `HeartbeatInterval`.
  Rejected: calling `writeRegistration` from the sweeper - it races on
  conv-loop-owned fields and can write a stale full row.
- The per-session registration flock fence stays (Prior Decisions b931a0261,
  7225f1a83). A session whose own ring writer is stalled while holding its own
  flock past `LiveWindow` is expected to age out of the rail; fixes 2 and 3
  keep the stall from spreading to other sessions.
  Rejected: letting the heartbeat bypass the flock, or moving ring I/O out
  from under the row lock - either removes the ownership fence.
- Fix 3: replace the runner-wide `summaryMu` with a lock per reply-id that
  persists across conv generations (plus a brief runner-level guard for the
  map lookup), so one session's ring I/O cannot block another session's
  snapshot publish while `summaryOwners[replyID] == c` stays fixed across the
  write. Rejected: a mutex stored on each conv - a newer generation for the
  same reply-id would not be fenced.
- Rejected: adding diagnostic fields to `registration.json` - a patch-level
  older daemon rewrites the file and drops unknown fields with no recovery path
  for the diagnostics; this is neither T1 nor T2 and is not patch-safe as-is
  (ai-docs/manuals/release-compatibility.md#L114-L114).

## Constraints

- Compatibility tier T0: no change to `registration.json` shape, ring
  segment format, MCP tools, or tracker events.
- `ai-docs/manuals/performance.md` applies: the heartbeat path must not add
  per-tick file touches beyond today's (one registration write per
  `HeartbeatInterval`, none otherwise), and must keep handles as today.
- `ai-docs/manuals/release-compatibility.md` applies to the wsagent and
  wslive surfaces.
- Fix 4 from the diagnosis (rail checks "asleep" before liveness) is out of
  scope here; it only hides the symptom.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

## Prior Decisions

- f1a29be5b (2026-11-20, commit): "Refresh a due hosted registration before snapshot publication and sequence persistence so a stalled summary writer cannot age an otherwise healthy dashboard row." — bearing: supports
- b931a0261 (2026-11-19, commit): "Revert LockRegistrationMemory and regCache (Findings 1 & 2), restoring OS-flock LockRegistration + ReadRegistration (7225f1a83) so external cross-process writers and unlocked reads cannot poison ownership." — bearing: constrains
- 7225f1a83 (2026-11-18, commit): "The hosted live writer's ownership fence (lockSummaryWriter) runs for every ring append and twice per snapshot flush of a hosted lead." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/runner.go, agents-plugin-tool/internal/wsagent/runner_conv.go, agents-plugin-tool/internal/wsagent/runner_summary.go, agents-plugin-tool/internal/wsagent/registration_idle_test.go |
| scope.surface | internal | No exported symbol change is specified; the phase names internal/wsagent behavior. |
| scope.new_public_symbol | no | No public symbol is specified in Phase 1. |
| scope.new_type_contract | no | No new type or signature is specified in Phase 1. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/registration_idle_test.go#L101-L150 already tests a blocked snapshot heartbeat. |
| complexity.reuse_points | confirmed | Existing writeRegistration and lockSummaryWriter were read at agents-plugin-tool/internal/wsagent/runner_conv.go#L769-L809 and agents-plugin-tool/internal/wsagent/runner_summary.go#L14-L29. |
| complexity.side_effect_risk | high | Heartbeats and summary writes share registration ownership and conversation state; agents-plugin-tool/internal/wsagent/runner_conv.go#L769-L809. |
| risk.correctness | high | A blocked conversation loop can still miss later registration deadlines; agents-plugin-tool/internal/wsagent/runner_conv.go#L566-L588. |
| risk.fit | moderate | Per-session locking must preserve the summary owner generation check; agents-plugin-tool/internal/wsagent/runner_summary.go#L19-L29. |
| risk.test | moderate | The existing blocked-snapshot test covers ordering, while Phase 1 requires a blocked-loop and cross-session regression; agents-plugin-tool/internal/wsagent/registration_idle_test.go#L101-L150. |
| risk.security_or_contract | moderate | The registration lock fences cross-process terminal handoff; agents-plugin-tool/internal/wsagent/runner_summary.go#L10-L29. |

## Phases

### Phase 1: Heartbeat independent of the conv loop, per-session summary lock

- Fix 2 and fix 3 as stated in `## Decisions`.
- Verification: regression tests that (a) hold one session's ring I/O
  blocked and assert another session's snapshot publishes and its
  `HeartbeatAt` advances, and (b) block one session's conv loop outside its
  own row lock and assert that session's `HeartbeatAt` still advances within
  one `HeartbeatInterval`, with at most one registration write per interval.
  `TestHeartbeatRegistrationPrecedesBlockedSnapshot` locks `r.summaryMu`
  directly; adapt it to the per-reply-id lock. Other existing
  `Registration|Heartbeat` tests in `internal/wsagent` stay green; `go test ./internal/wsagent ./internal/wslive
  ./internal/wsdashboard` green with narrow `-run` patterns where the package
  is slow.
