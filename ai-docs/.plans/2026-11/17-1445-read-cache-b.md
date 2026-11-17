# Read-cache bundle B: ticket inventory, whiteboard re-render skip, session-switch reads

## Context

On the owner's Windows PC, AhnLab V3 scans every file touch, so each syscall
costs 10-100x. The release is gated on daemon file-system load (clone note
`release-gate-daemon-file-io-20261117`). The owner approved (whiteboard (1297)
= a, 2026-11-17 14:44) a zero-state-risk bundle: read caches and
skip-if-unchanged writes only.

This plan is half B. A parallel worker does half A on
`impl/develop/read-cache-a`: per-tool-call key/config/endpoint/rsrc caches,
`wsstate.Ensure`, and the launcher. Stay out of A's areas.

Evidence:
- Researcher B's report, `D:/Temp/file-touch-map-mcp/report.md`.
- The research ticket
  `ai-docs/tickets/todo/261117-research-ws-daemon-file-touch-map.md`,
  especially its session-switch read path section and Outcome Ledger.

Re-locate every line; the code moved since.

## Outcome

1. **Ticket inventory.** Every stem query and every `git.commit` scans the
   whole ticket board, about 1,411 files, giving 70-400 reads/s in bursts.
   Serve these from an in-memory inventory per board root.
   - Revalidate it cheaply: directory mtimes of the status directories plus a
     per-file stat only where needed, or an existing watcher or wake if one
     exists. Do not rescan everything.
   - Results must match a fresh scan after any ticket move, create, edit or
     close, including changes made by another process or by plain git.
2. **Whiteboard refresh.** A refresh writes 8 or more fsync'd output files
   even when nothing changed. Skip writing an output file whose bytes are
   identical to what is on disk or to the last-written content held in
   memory.
   - Keep the folder store's commit protocol, which covers `committed.json`,
     `pending.json` and the generation, exactly as it is.
   - Skip only the derived render outputs, never the store's commit records.
3. **Session switch** (research ticket, session-switch section):
   - Opening a session page renders once for the page request and again for
     the stream. Serve the stream's first frame from the page render, or share
     one render, so a page open costs one read pass.
   - Hosted lead pages re-fetch runner state (inbox and permission reads) on
     every header update. Cache it per render, keyed to the runner's change
     signal, so an unchanged runner costs no rereads.
   - The wslive observed-evidence cache clears all slots once it holds 256
     sessions (`observed_cache.go:120`). Replace the full wipe with eviction
     of the least recently used sessions, so busy sessions stay warm.
4. **SSE write deadline.** The SSE sink writes with no deadline
   (`wsdashboard/sse.go` ~783-786). Give it a write deadline like the
   WebSocket path's 10 s (`stream_ws.go` ~39, 155-156), so a stalled client
   is dropped instead of holding its goroutine.

## Possible addition (lead may send it later)

A separate profiling agent is tracing a new hot read site,
`wsstate/replace_file_windows.go:44` `wsstate.ReadFile`, at 34 reads/s; it is
probably whiteboard folder reads. If the lead forwards a fix for it, fold it
into this branch.

## Constraints

- Read these manuals before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`
  - `ai-docs/manuals/release-compatibility.md`
  - `ai-docs/manuals/ws-mcp.md`
- **Compatibility.**
  - No on-disk shape change and no write-timing change, except skipping
    byte-identical rewrites of derived outputs.
  - Expected tier: T0.
  - State the tier per item, with source lines, in the commit.
- Simplest complete change. No config knobs.
- NO local `go test`, Go test executables, ws/ws-mcp executables, the
  launcher, the daemon or Electron on this PC (owner antivirus rule).
  - Node and Playwright against static files are fine.
  - Go builds and tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run read-cache-b 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test ./...'`.
  - You may narrow the package list if the full run is too slow. Cover every
    package you touched, plus `./internal/mcp` and `./internal/wsdashboard`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/read-cache-b/`.
- Commit on `impl/develop/read-cache-b` with your own model's Co-Authored-By
  trailer. Do not push.

## Verification

- Tests show that:
  - the inventory matches a fresh scan after create, move, close and an
    external edit, and that a repeated query does not rescan;
  - an unchanged whiteboard refresh writes no output files, and a changed one
    writes only the changed outputs;
  - a page open plus stream subscribe costs one read pass;
  - an unchanged runner header does not reread inbox or permissions;
  - evidence cache eviction keeps recently used sessions;
  - a stalled SSE client is dropped within the deadline.
- One passing w5090 run.
- Report:
  - the commit;
  - the files;
  - before and after read counts for each item, estimated from code or
    measured in a test;
  - the test results;
  - the tier per item.
