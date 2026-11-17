# Steer ("즉시 전달") must not fall back to turn end after one slow drain

## Context

Owner, 2026-11-17 12:55: dashboard "instant delivery" (steer) messages to a
Claude Code lead do not go out for 30+ seconds, with repeated 6-8 s delays. The
owner's Windows PC amplifies every file syscall 10-100x (AhnLab V3). The owner
set a release gate: no release until interactive paths stop waiting on slow file
I/O, because corporate PCs are slower still.

An independent read of develop 28f523038 found the following. Re-check the
lines; they may drift.

- The Claude mod claims a steered message only at a main-thread tool boundary:
  - `agents-plugin/hooks/claude-mod/watcher.ts:91-104`;
  - `steerBoard`, `:925-990`.
- It gives the `dashboard_inbox_drain(steer_only)` MCP call
  `STEER_DRAIN_WAIT_MS = 2000` (`:278`). If the drain takes more than 2 s:
  - that boundary claims nothing;
  - no later boundary drains while the first call is still outstanding
    (`:929-932`);
  - the late claim is carried to the turn end (`:951-958`).

  So one 2 s daemon hiccup turns "instant" into "end of turn", which is 30 s to
  minutes.
- The drain on the daemon side is `DrainSteer`. It reads every entry and then
  claims with an exclusive no-share rename (`wsinbox/fileops_windows.go:31-74`).
  On ERROR_SHARING_VIOLATION or ACCESS_DENIED it retries for up to 2 s
  (`:90-111`). The steer route has just written a brand-new entry file
  (`wsinbox/inbox.go:886`), and that is exactly when AV holds a scan handle on
  it. The steer route's own exclusive claim (`:864`) has the same 2 s retry,
  under the daemon-wide inboxMu.

Parallel workers:
- `impl/develop/daemon-io-r2a` owns the answer-replay lock in
  `wsdashboard/inbox_routes.go` `scheduleAnswerReplay`, and `wsinbox` held
  counts.
- `impl/develop/daemon-io-r2b` owns the rail and Index rereads.

Do not touch those areas.

## Outcome

1. **Mod side (main fix).** A drain that completes after its boundary's wait
   must not be lost to the turn end.
   - Deliver its claimed messages at the next tool boundary.
   - Or the next boundary must be able to drain again; pick the simplest correct
     form.
   - Keep these rules:
     - a claimed message is delivered exactly once;
     - it is still delivered at turn end if no further boundary occurs;
     - ordering is preserved.
   - Revisit the 2000 ms value only if the code makes clear a different bound is
     better. Do not just raise it as the whole fix.
2. **Daemon side (only if small and contained to `wsinbox` claim/drain for
   steer).**
   - Today a freshly written entry that AV holds open makes the claim spin for
     up to 2 s.
   - If a narrow change avoids that, do it. Examples:
     - retry briefly and then return the entries not yet claimable, without
       blocking;
     - claim without an exclusive open.
   - Do it only if it does not change on-disk shape or cross-version semantics
     (an old ws-mcp or mod reads the same inbox).
   - Otherwise report it as a follow-up with the lines.

## Constraints

- Read before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/wsflow-mirroring.md`, since the mod lives in
    `agents-plugin/hooks/`;
  - `ai-docs/manuals/release-compatibility.md`, if you touch
    `agents-plugin-tool/internal/`.
- Regenerate any mirrors or generated copies with the repo's scripts. Never
  hand-merge generated files.
- State the compatibility tier with source lines: an old daemon with the new mod,
  and a new daemon with the old mod. Anything above T0 means stop and report.
- Simplest complete change. No new config knobs.
- NO local `go test`, Go test executables, ws/ws-mcp executables or the launcher
  on this PC (owner AV rule). Do not start the daemon or Electron.
  - Node tests for the mod run locally.
  - Go tests, if you change Go, run only on w5090:
    `D:/sys_lock.sh remote w5090 run steer-fix 20 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsinbox ./internal/wsdashboard ./internal/mcp'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/steer-late-claim/`, deleted afterwards.
- Commit with no Co-Authored-By trailer. Do not push.

## Verification

- A mod test: the first boundary's drain resolves after the wait. The message is
  delivered at the next boundary, not at turn end, exactly once. No boundary
  occurs: it is still delivered at turn end.
- If the daemon side changed: a wsinbox test in which a held, not-yet-claimable
  fresh entry does not block the drain for 2 s.
- Report:
  - commits;
  - files;
  - the tier;
  - how the change reaches a running Claude Code session: plugin reinstall,
    daemon swap, or session restart. The lead needs this to roll it out.
