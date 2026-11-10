---
title: "Dashboard agent host: a Python session holder that owns agent processes and PTYs across daemon restarts"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-mcp-daemon-hosted-proxy-launcher: "the daemon now hosts every MCP server and is replaced on version change; its Decision 7 (replacement drain, transparent proxy reconnect) is the restart behavior a holder must survive"
  261110-research-ws-dashboard-agent-host-foundations: "evidence for the archived terminal (no process survival) and the python3 prerequisite"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f5ee2f2e452c7bcc
sage-review-completeness-reviewed: f5ee2f2e452c7bcc
---

# Dashboard agent host: session holder

## Background

The user runs very long sessions and expects to update the dashboard daemon
often. A daemon restart must not end a daemon-hosted agent or terminal
(user, 2026-11-10 whiteboard, (03) a, (06) a).

The archived ws-dashboard did not provide this. Its `TerminalRegistry` lived
in daemon memory and the daemon owned each PTY child, and
`260523-feat-ws-dashboard-terminal-tab-restore` accepted that "live terminal
processes do not need to survive daemon death": restored tabs were new
shells (`git show archive/ws-dashboard:ws-dashboard/crates/daemon/src/terminal.rs`).
This ticket builds the missing piece: a small per-session holder process that
owns the child process and outlives the daemon.

The holder is shared by both epic tracks: track A
(`261110-feat-ws-dashboard-headless-lead-sessions`) puts pipe-based agent
processes in it, track B (`261110-feat-ws-dashboard-terminal`) puts PTYs in
it.

## Decisions

1. **Process survival, not only conversation survival (user, (06) a).** A
   holder owns its child (an agent process over pipes, or a shell over a
   PTY) and keeps it running while the daemon exits and restarts; the daemon
   reattaches afterwards, so an in-flight turn or shell continues.
   Rejected: conversation-only survival, where the daemon's exit ends the
   process and the daemon resumes through the backend session ID ((06) b),
   and staging that first ((06) c). The user's reason: very long sessions are
   common.
2. **The holder is a Python script using the standard library only
   (user, (06) note; technical, (09)).** python3 is already a prerequisite on
   every host: Claude and Codex start ws through
   `python3 ./bin/ws-mcp-launcher.py` (`agents-plugin/.mcp.json`), Pi spawns
   the same launcher (`agents-plugin-pi/src/mcp-stdio-client.ts`), and
   Windows requires `python3` on PATH (`ai-docs/manuals/ws-mcp.md`). A `.py`
   file can be replaced while a holder runs it, so the Windows lock on a
   running executable does not block updates. No third-party package: the
   plugin distribution has no pip install step.
   Rejected: a holder that is the ws-mcp executable (a running `.exe` cannot
   be overwritten on Windows, and each new executable run triggers the
   endpoint protector that `261110-feat-ws-mcp-daemon-hosted-proxy-launcher`
   exists to avoid); a separate native holder binary; pywinpty or any other
   pip dependency.
3. **PTY: `pty` on POSIX, ConPTY through `ctypes` on Windows (technical,
   (09)).** The standard library `pty` module is POSIX-only; on Windows the
   holder calls `CreatePseudoConsole` and its companions through `ctypes`.
   Uncertainty: ctypes ConPTY behavior across Windows versions; verify with
   `ai-docs/manuals/windows-dogfood.md`.
4. **Holder protocol version (technical, (10)).** On attach the holder
   reports its protocol version. The daemon supports the current and the
   previous version (N-1). An older holder is reported to the user with a
   prompt to restart that session when it is idle. Rejected: no negotiation
   (a daemon update could fail to reattach to existing sessions, which
   defeats Decision 1).
5. **Discovery and orphan handling (technical, (25)).** Each holder registers
   its PID, its attach socket, and its session descriptor in a state
   directory. On start the daemon scans the directory and reattaches. The
   dashboard lists holders no daemon is attached to, with a stop button. A
   holder whose child has exited, and to which nobody attaches for a set
   period, exits on its own. The daemon never kills a live holder
   automatically: losing a very long session costs more than an idle
   process. Rejected: automatic reaping of detached holders.
6. **Attach over loopback TCP with a per-holder token (technical, (92)
   ok).** The token sits in the holder's descriptor file (mode 0600).
   Rejected: a Unix socket or a named pipe (AF_UNIX on Windows depends on
   the Python build, named pipes need ctypes; the daemon and the proxy
   launcher already use loopback TCP plus a token in standard-library
   Python).
7. **Bounded output ring for reattach (technical, (93) ok).** The holder
   keeps the child's output as whole records (stream-json lines, or PTY
   chunks) in a 4 MiB ring with sequence numbers. The daemon reattaches from
   the last sequence it saw (Decision 17); when that sequence has left the ring, the holder
   reports the gap and the daemon surfaces it. Rejected: unbounded spooling
   to disk.
8. **State directory `<cache root>/dashboard/holders/<holder-id>/`
   (technical, (94) ok).** Rejected: `dashboard/sessions/<reply-id>/`,
   because a terminal has no reply ID and session directories are pruned
   after 7 days (`RegistryRetention`, `registry.go`).
9. **Self-exit 24 hours after the last detach, once the child has exited
   (technical, (95) ok).** A daemon down overnight still collects the exit
   status and the final output. A live child never triggers it
   (Decision 5). Rejected: a short period.
10. **Python 3.10 floor (announced, (96) ok).** The launcher already needs
    3.10 (runtime `X | None` annotations, `ws-mcp-launcher.py#L16`), so the
    holder adds no new requirement.
11. **wsagent stays a direct daemon child in Phase 1 (technical, (97) ok).**
    The support agent's conversations keep their current resume-based
    survival. Consumers move onto the holder in their own tickets
    (`261110-feat-ws-dashboard-headless-lead-sessions`,
    `261110-feat-ws-dashboard-terminal`). Rejected: moving wsagent's process
    start onto the holder now.
12. **Refused job breakaway on Windows (technical, (98) ok).** When the
    environment refuses `CREATE_BREAKAWAY_FROM_JOB` (the fallback in
    `spawn_windows.go`), the holder still starts and the dashboard marks it
    "not restart-safe". Rejected: refusing to start.
13. **The unattached-holder list is a drawer panel card (technical, (99)
    ok).** It sits beside the worktrees and settings cards and carries a stop
    button per holder (Decision 5). Rejected: a new surface.
14. **The script is embedded in the ws-mcp binary and staged by content
    (technical, design review).** The Go binary embeds the holder script and
    writes it to `<cache root>/dashboard/holders/bin/holder-<content hash>.py`
    before spawning a holder; a running holder keeps its own staged file, so
    an update never rewrites a script in use (Decision 2's reason stands).
    The interpreter is the `sys.executable` of the launcher that starts the
    daemon, passed in an environment variable that survives both hops
    (`daemon-start`, then the detached daemon); a daemon started without one
    falls back to `python3` on PATH, and a failed holder start is reported
    as an error on the drawer card. On start the daemon deletes staged
    scripts that no live holder's descriptor names (each descriptor records
    its script hash). Rejected: a script file beside
    `ws-mcp-launcher.py` in each package (the daemon runs from a staged copy
    in its own directory and one daemon serves ws, wsflow, and Pi
    launchers, so no package path is the right one); PATH `python3` as the
    first choice (on Windows it may be the Store alias).
15. **The newest authenticated attach takes over (technical, design
    review).** A holder accepts an attach that presents its token, closes
    the previous attach connection, and from then on takes input only from
    the new one, so a replaced daemon and its successor never both write to
    one child. "Unattached" means one of three causes: the endpoint does not
    answer, the protocol is older than N-1, or the token is refused. The
    daemon rescans unattached holders on its worktree reread cadence as well
    as on start. Rejected: refusing a second attach (the successor would
    list a live holder as unattached until its next restart).
16. **Stop: through the protocol, else a guarded kill; gate `localViewer`
    plus `sameOrigin` (technical, design review).** Stop asks the holder to
    end its child and exit. For a holder the daemon cannot stop through the
    protocol (any of Decision 15's three unattached causes), the daemon
    kills the holder and its child (the process group on POSIX, the process
    tree on Windows, so a pipe child such as `claude -p` does not outlive
    it) only when the process still matches the start time recorded in the
    descriptor, which guards against PID reuse. The holder writes its own
    wall-clock start time at startup and keeps it on every rewrite
    (Decision 18); the daemon compares it with the OS creation time of the
    PID within 2 seconds. The stop
    route uses `localViewer` plus `sameOrigin`, because holders own terminal
    PTYs and the epic's trusted-origin boundary covers the terminal; a LAN
    token holder sees the card without the stop button. Rejected:
    `sameOrigin` alone like the git routes (a LAN viewer could end a long
    session or a shell).
17. **Replay after a daemon restart (technical, design review).** A daemon
    process remembers the last sequence per holder only in memory: a
    reattach within one daemon process continues from it, and a new daemon
    process replays the whole ring. Records carry their sequence numbers, so
    consumers drop the ones they already hold. Rejected: persisting the last
    sequence per holder (a second state file to keep consistent with the
    ring).
18. **The holder rewrites a missing descriptor (technical, design
    review).** Every 60 seconds the holder checks that its descriptor file
    exists and rewrites it (mode 0600) when it does not, so clearing the
    cache root does not strand a live holder. Rejected: moving the state
    directory out of the cache root (Decision 8 stands; its rejected
    alternative was about pruning, which this check also survives).
19. **Protocol operations in version 1 (technical, design review).** Spawn
    spec (argv, cwd, environment, pipes or PTY, initial PTY size), input
    write, PTY resize, signal and stop, exit status report, version report,
    ring replay from a sequence, and gap report. Consumers that land later
    use these without a version bump. Rejected: the version, replay, and
    gap operations alone (the first consumer would force version 2).

## Constraints

- The holder must outlive the process that spawned it: it is detached from
  the daemon's process group and session on POSIX, and from any job object
  that kills its members when closed on Windows.
- The holder only owns processes and relays bytes. Approval, mailbox
  delivery, workspace binding, and rendering belong to the children
  `261110-feat-ws-dashboard-headless-lead-sessions`,
  `261110-feat-ws-dashboard-workspaces`, and `261110-feat-ws-dashboard-terminal`.
- An agent process inside a holder reaches ws MCP through the launcher proxy,
  which reconnects on its own when the daemon is replaced
  (`261110-feat-ws-mcp-daemon-hosted-proxy-launcher` Decision 7). Cite that
  ticket's Decisions, not the current code, for restart behavior; its proxy
  design was still being corrected on 2026-11-10.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the holder ships to
  downstream projects), `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/windows-dogfood.md`, `ai-docs/manuals/wsflow-mirroring.md`
  (the launcher change is mirrored into the wsflow launcher copy under its
  drift tests).
- Forward note for `261110-feat-ws-dashboard-headless-lead-sessions` and
  `261110-feat-ws-dashboard-terminal`: a consumer inside the daemon (for
  example the stream-json ingest into the conversation ring) persists the
  last holder sequence it applied, or Decision 17's full replay becomes
  duplicate entries.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 261110-feat-ws-mcp-daemon-hosted-proxy-launcher (2026-11-10, Decisions): "Version mismatch. Within the same minor (0.48.x) the newest daemon wins ... When the daemon is replaced under a live connection, the proxy reconnects transparently" — bearing: constrains
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "PTYs live in the session holder, so shells survive a daemon restart. Each terminal is a holder-owned PTY; after a restart the daemon reattaches instead of creating new shells." — bearing: supports
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-10, Decisions): "Headless, not a TUI in a PTY. Daemon leads run as `claude -p` stream-json and Pi RPC processes owned by a session holder." — bearing: supports
- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "Survival across daemon restarts. Daemon-hosted agents and terminals keep running while the daemon is updated or restarted: a per-session holder owns each process and the daemon reattaches." — bearing: supports
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "Resident sessions mean process survival across daemon restarts through a per-session Python holder" — bearing: supports
- 260523-feat-ws-dashboard-terminal-tab-restore (2026-05-23, facfac97): "The implementation intentionally recreates new daemon terminal sessions instead of pretending PTY processes survive daemon death." — bearing: supports
- 260523-feat-ws-dashboard-persist-open-workroots (2026-05-23, Decisions): "Terminal process survival is out of scope. A later phase may restore terminal tab placeholders or recreate shells in a captured PWD" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | new holder script embedded in the ws-mcp binary (Decision 14), daemon-side spawn, discovery, and reattach in agents-plugin-tool/internal/wsdashboard, and the launcher passing its interpreter at daemon start |
| scope.surface | cross-module | holder attach protocol between a Python script and the Go daemon; state-directory format read by the dashboard |
| scope.new_public_symbol | yes | holder script and its attach protocol, exact names unsettled until promotion |
| scope.new_type_contract | yes | holder protocol version handshake and the per-holder registration descriptor of Decisions 4 and 5 |
| scope.test_surface | new-files | no holder tests exist; launcher tests live in agents-plugin/tests/test_ws_mcp_launcher_*.py and wsagent tests in agents-plugin-tool/internal/wsagent/*_test.go |
| complexity.reuse_points | unconfirmed | candidates not named by the ticket: wsdashboard startDetached in spawn_unix.go and spawn_windows.go, launcher proxy_cache_root, wsdashboard.SessionDir registry layout |
| complexity.side_effect_risk | high | detached long-lived processes that outlive the daemon, orphan holders, and a state directory scanned on every daemon start |
| risk.correctness | high | process detachment, PTY and ConPTY relay, reattach without losing output, and self-exit timing across three OSes |
| risk.fit | moderate | wsagent.Runner already hosts headless conversations and ends their processes on Close with lazy resume, which the holder must replace without a stated integration plan |
| risk.test | high | requires live daemon-restart verification on Linux, macOS, and native Windows; ctypes ConPTY is unexercised in this repo |
| risk.security_or_contract | high | an attach endpoint grants control of an agent or shell, and the N-1 protocol contract binds future daemons |

## Phases

### Phase 1: holder process, attach protocol, discovery

Decisions 1-19: the holder script with the version 1 operations
(Decision 19), its registration in the state directory, the daemon side
that stages and spawns a holder, scans the directory on start and on the
reread cadence, and reattaches, and the drawer card for unattached holders.
A holder stays unattached when its protocol is older than N-1, its endpoint
does not answer, or its token is refused (Decision 15). The script lives in
the Go source tree and is embedded (Decision 14); no package copies. No
consumer moves onto the holder in this phase (Decision 11); the tests drive
it with a test child.

Verification (announced, (100) ok):

- Python unit tests: version report; N and N-1 acceptance, with N-1 faked
  in the test while only one version exists; ring replay from a sequence and
  the gap report once a sequence has left the ring; attach refused without
  the token and the descriptor written with mode 0600 on POSIX; self-exit
  after the idle period once the child has exited, and never while it
  lives (with a shortened period); a newer authenticated attach closes the
  previous one; spawn, input, resize, signal, and exit status with a test
  child; a deleted descriptor is rewritten.
- Launcher tests (`agents-plugin/tests/test_ws_mcp_launcher_*.py`): the
  launcher passes its `sys.executable` when it starts the daemon.
- Go tests: staging the embedded script by content hash; discovery on
  daemon start and on the reread cadence, reattach, the unattached list and
  its three causes; stop through the protocol and the start-time-guarded
  kill for each of the three unattached causes, ending the child too, and
  refused on a start-time mismatch; removal of unreferenced staged scripts; the interpreter fallback to
  `python3` on PATH and a failed holder start shown as a drawer-card error;
  the stop route's gate in `TestLANModeGuardsEveryRoute`; the "not
  restart-safe" mark.
- A browser check: the drawer card lists an unattached holder and its stop
  button ends it.
- A daemon restart with a live child on Linux, macOS, and native Windows
  (`ai-docs/manuals/windows-dogfood.md`); the child keeps running and its
  output produced while detached arrives after reattach, and a test client
  outside the daemon that keeps its last sequence across the restart
  receives no duplicate (the daemon
  keeps no dedupe state of its own, Decision 17).
