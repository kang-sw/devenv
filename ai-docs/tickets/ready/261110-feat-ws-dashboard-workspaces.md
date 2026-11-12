---
title: "Dashboard agent host: workspaces (root worktree or plain folder), per-workspace sessions, and conversation handoff between CLI and daemon"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-headless-lead-sessions: "prerequisite of Phases 2 and 3 only (conversation handoff needs its Phase 1); it builds the new-session + buttons and popover ((189))"
  261110-research-ws-dashboard-agent-host-foundations: "Topic 4 (multi-session and workspace management)"
  261110-feat-ws-dashboard-session-rail: "owns the rail's workspace and worktree grouping; this ticket adds user-added workspaces and the + buttons on top"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 22885b1e53fed13d
sage-review-completeness-reviewed: 22885b1e53fed13d
---

# Dashboard workspaces

## Background

The user wants to manage agents from the dashboard by folder: add a
workspace, see and start its sessions, and continue a conversation
(2026-11-10 whiteboard, (01)). Session spawn, the terminal's working
directory, and the file preview root are all chosen by workspace.

The session rail (`261110-feat-ws-dashboard-session-rail`) already groups
sessions by workspace and worktree ((62) a). This ticket adds what the rail
cannot derive from sessions: workspaces the user adds and keeps without
sessions, and the rail's + buttons. The confirmed mockup,
`ai-docs/ref/session-rail-mockup/index.html` (now v14; 605a4acf1 changed
only the idle robot slot and the workspace bar icons, so the + buttons are
as in v13), shows the + buttons and their three popovers.

Today the daemon has no stored workspace list, no endpoint that runs
`git worktree add` (the only add is `ws/worktree.acquire`, a session's tool
call), and no endpoint that starts a lead session (Explore, 2026-11-10;
since landed: the stored list and its routes in Phase 1,
`agents-plugin-tool/internal/wsdashboard/workspace_routes.go`, and
`POST /agents/leads` from `261110-feat-ws-dashboard-headless-lead-sessions`,
`agents-plugin-tool/internal/wsagent/http.go#L130-L140`).

## Decisions

1. **A workspace is a repository's root worktree or a plain non-git folder
   (user, (08) b and note; announced, (11)).** Only those can be added. A
   linked worktree shows under its root workspace, and a session binds to a
   worktree within its workspace, so a lead and its workers sit in one
   workspace. ws pooled worktrees from `ws/worktree.acquire` are not shown
   (user, 2026-11-10; announced, (54) ok), except a pooled worktree that
   holds a session, which the session rail shows while it does ((71) a).
   Rejected: every folder its own workspace, grouped in the UI only ((08) a);
   a free multi-root set of folders ((08) c).
2. **Conversation handoff in both directions (user, (15) a).** The daemon can
   take over a Claude or Pi session that a CLI process started, once that
   process has exited, and resume it headless; a daemon session can be
   handed to the terminal by stopping its process and giving the user a
   `claude --resume <id>` command (for Pi, `pi --session <id>`; a daemon Pi
   lead runs on `<state dir>/sessions/<reply-id>/pi/session.jsonl` with
   `--session-dir` set there, `agents-plugin-tool/internal/wsagent/pi.go#L280-L285`,
   and `pi --session` resolves a bare id only in its session dir, so the
   terminal command needs the file path or that `--session-dir`). One session ID is never held by two
   processes at once: before a handoff the previous process must have
   exited.
   Rejected: CLI to daemon only ((15) b); daemon sessions only ((15) c).
3. **Three + buttons on the rail (user, 2026-11-10; announced, (53) ok;
   mockup v13).** Each shows only while its line is hovered or focused
   (always, faintly, on touch screens) and sits right after its label: the
   rail header's "sessions +" adds a workspace; a workspace bar's
   "<path> +" adds a worktree, and a plain-folder bar shows no + (reversed
   from "opens the new-session popover" by
   `261110-feat-ws-dashboard-headless-lead-sessions` Decision 10, (290) a,
   because a lead session key is minted at a Git worktree root; announced,
   (324)); a worktree row's "<branch> · <folder> +" opens the
   new-session popover for that worktree. The two new-session + buttons
   are built by `261110-feat-ws-dashboard-headless-lead-sessions`
   Decision 10 ((189)); this ticket built the other two.
4. **Adding a workspace takes a pasted absolute path only (user, (55) c;
   technical, (60) ok).** No folder dialog: a dialog opened by the daemon
   would appear on the daemon's screen, the wrong PC for a LAN viewer, and a
   browser picker never reveals an absolute path. The user pastes a path
   from the file explorer.
   - The daemon normalizes the text by its own OS and answers a preview
     request while the user types; the popover shows the resolved path or
     the reason it is refused, and disables "add" on refusal. Common: strip
     surrounding quotes and a trailing slash; `~` is the daemon user's
     home; relative paths and network shares (`\\host\share`) are refused.
     Under WSL: `C:\…`, `C:/…`, `/c/…`, and `/mnt/c/…` become `/mnt/c/…`;
     `\\wsl$\<distro>\…` and `\\wsl.localhost\<distro>\…` become the Linux
     path only for the daemon's own distribution (`WSL_DISTRO_NAME`), other
     distributions are refused. On native Windows drive paths stay and
     `/c/…` becomes `C:\…`; on Linux and macOS only absolute paths and `~`.
     `/c/…` is read as a drive only when no such top-level folder exists.
     A path that does not exist or is not a folder is refused.
   - A path inside a repository resolves to that repository's root
     worktree before the rules below apply, and the toast names the root
     that was added (technical, (76) ok). Rejected: refusing a subfolder
     paste.
   - Results: a path that is already a linked worktree of a known workspace
     changes nothing; an existing workspace that is (or later becomes) a
     linked worktree of another root merges under that root; otherwise the
     workspace is added. Each result is a toast, and the rail shows the
     result at once, without waiting for its worktree-list cadence (user,
     (72) note).
   Rejected: the daemon opening the OS folder dialog (recommended at first,
   rejected for the LAN case); the browser folder picker with a name guess
   ((55) b); a separate fallback input next to a dialog ((56), superseded).
5. **Adding a worktree (technical, (58), (59) ok).** The form takes a new
   branch (its base defaults to the root's current branch) or an existing
   branch, listing only branches not checked out in another worktree. The
   folder defaults to `../.ws-dashboard-worktrees/<root-dir-name>/<YYMMDD-HHMM>`
   beside the root; a same-minute clash appends `-2`, `-3`. "Custom" takes
   an absolute path, refused when it exists and is not empty. A created
   worktree shows in the rail at once (user, (72) note).
6. **The new-session popover reuses the tracker support chat's selection
   popover (announced, (57) ok).** Harness, model search, and effort knob
   come from `supDrawPop` (`assets/dashboard.js`) and `GET /support/choices`,
   plus a line naming the target worktree and a start button. The popover
   is built, with a permission-mode row, by
   `261110-feat-ws-dashboard-headless-lead-sessions` Decision 10 ((189)).
   Rejected: a second picker.
7. **Stored list in the config home (technical, (110) ok).** The list lives
   at `<config home>/dashboard/workspaces.json`, beside the tracker's
   per-machine store registry (`wstrackerhost`). Rejected: the cache root
   (`wsdashboard.Dir`), because the list is user-curated and a cache clear
   must not lose it.
8. **Matching sessions to stored workspaces (technical, (111) ok).** A
   session joins a stored git workspace when its resolved root worktree
   (`261110-feat-ws-dashboard-session-rail` Decision 10) equals the stored
   root. A session whose root does not resolve to a repository joins a
   stored plain folder when its root is at or under that folder (longest
   prefix wins); a session inside a repository always joins its repository
   workspace, so adding a parent folder such as `~/projects` never pulls
   repository sessions out of their grouping. A stored and a derived
   workspace with the same root are one bar. Rejected: path equality only.
9. **Forget a stored workspace (user, (112) a).** A stored workspace bar
   shows a "forget" action on hover, beside its +. It removes the entry
   from the list only and never touches files; a workspace that still has
   sessions keeps showing as a derived one. Rejected: no removal in Phase 1
   ((112) b).
10. **Gate: sameOrigin only (user, (113) a; settles (78)).** Adding a
    workspace, adding a worktree, the path preview, and forget use
    `sameOrigin`, like the git push, pull, and fetch routes, so a LAN token
    holder can use them and Decision 4's LAN reasoning stands. Rejected:
    `localViewer` plus `sameOrigin` ((113) b); the epic's trusted-origin
    boundary guards shell execution and tool approval, and `git worktree
    add` is no stronger than push and pull.
11. **Stored workspaces join the worktree reread set (technical, (114) ok).**
    A stored workspace with no session gets its worktree list on the rail's
    cadence (`261110-feat-ws-dashboard-session-rail` Decision 9).
    This extends the rail's disclosure constraint ((73): worktree branches
    come only from repositories whose sessions a viewer can open): a stored
    workspace's worktrees and branches show without a session. The
    extension follows from Decision 10, since any viewer who can add a
    workspace can already list its worktrees; the rail's code or tests that
    encode (73) are updated to allow stored workspaces. Rejected: listing it
    without worktrees.

12. **A takeover keeps the session's ws key (announced, (316)).** In both
    directions the session keeps its ws key, so its reply-id, rail row,
    page URL, agenda, todos, and board continue: the daemon spawns the
    takeover with that key (`261110-feat-ws-hosted-session-mode-flag`
    Decision 7, "taking over another session's track becomes a daemon spawn
    with that key"). `Runner.create` always mints a lead key today, so a
    takeover needs its own create path that adopts the existing key.
    Rejected: minting a new lead key, which loses the agenda and todos and
    is refused by the hosted relogin rule when the resumed transcript uses
    its old key.
13. **The harness session reaches the daemon through the ws key record
    (technical, (317)).** At login the Claude mod and the Pi extension
    report the harness session (the Claude session id; the Pi session file
    path) and the CLI's working directory to ws-mcp, which stores them on
    the ws key record. A takeover resumes from that record in that
    directory (Claude resolves `--resume` per working directory; Pi lead
    args need a path for an externally supplied session file instead of the
    daemon's own). Both sides already hold the pair
    (`agents-plugin/hooks/claude-mod/manual.ts` KEY_STORE_PREFIX; the Pi
    bridge's custom session entry, `agents-plugin-pi/src/bridge.ts`).
    Rejected: scanning harness-private stores (`~/.claude/projects`, Pi
    session dirs), whose formats are not a contract (Architecture Rule 3).
    A taken-over session's record carries that working directory apart
    from `Record.Root` (the canonical git root the holder spawns in today),
    and every later restart of it (idle resume, MCP restart, a settings
    change) uses it (technical, design review).
14. **A live harness PID blocks a takeover (user, (318) a).** A CLI row
    records its harness PID (`WS_MCP_HARNESS_PID`, already known to every
    ws-mcp). Before a takeover the daemon refuses while that PID is alive;
    the PID only ever blocks and never shows a session as live, so a reused
    PID can only refuse a takeover. This amends ee0520fa's no-PID registry
    rule, whose reason (PID reuse must not fake a live session) still holds.
    Liveness on the rail stays heartbeat-only. A row with no recorded PID
    (written by a ws-mcp older than this phase) allows a takeover only when
    it carries EndedAt, so a still-running CLI idle past 12 h is not taken
    over (technical, design review).
    Rejected: heartbeat and EndedAt only with a user confirmation for an
    unclean end; always asking the user.
15. **A CLI login to a key the daemon holds is refused (technical,
    (319)).** A CLI ws-mcp that logs in with a key whose row is hosted by a
    live daemon process refuses the key and tells the user to hand the
    session to the terminal from the dashboard first. The daemon cannot stop
    a terminal `claude --resume`; the key login is the one point where a
    second holder is caught. Rejected: letting both run.
    "Live" is judged by the row's daemon-written spawned harness PID
    (`HostPID`), not by the daemon being up: a daemon lead runs on under
    its session holder when the daemon exits (`Runner.Close`). As in
    Decision 14, a reused PID can only wrongly refuse. A ws-mcp started
    inside the hosted session itself (its hosted flag, or a harness PID
    matching the recorded spawned PID) is neither refused nor treated as a
    hand-off. A hosted row with no `HostPID` refuses unless it carries
    EndedAt, the same safe default as Decision 14. The refusal message also
    covers an orphaned lead still running while the daemon is down (start
    the dashboard, then hand off) (technical, review).
    A CLI login to a hosted row with no live process (idle-stopped, closed,
    or orphaned) is accepted as an implicit hand to terminal: the row
    becomes a CLI row carrying the harness PID (user, (331) a). The daemon
    then releases the conversation: before every row write, every mail
    drain, and at start it checks the row is still its own, and on finding
    it handed off it stops the heartbeat and the mail watch and records the
    conversation as handed off (Decision 16's state), so recovery skips it;
    mail it had already claimed into its record goes back to the key's
    inbox for the terminal session. The check and the write or spawn it
    guards are one step against a concurrent login rewriting the row: the
    daemon and the CLI ws-mcp are separate processes, so this needs a file
    lock or a compare-and-write on the registry row, and returning mail
    needs a put on the runner's mail interface, which offers only `Drain`
    and `Watch` today (technical, review). Rejected: refusing every CLI login to a hosted
    row, live or not, which needs the daemon up to hand off first.
    On a refusal the Claude mod and the Pi extension stay keyless and show
    a visible transcript message telling the user to hand the session to
    the terminal from the dashboard first (user, (332) a). Today both fall
    through to minting a fresh key (the Claude mod with only a debug line).
    Neither choice stops two processes holding one transcript, since the
    terminal harness has loaded it before its ws-mcp logs in; staying
    keyless gives the second process no ws identity and tells the user.
    Rejected: minting a fresh key with a warning, which gives the second
    process a ws identity of its own and hides the conflict.
16. **Hand to terminal is its own conversation state (technical, (320);
    announced, (323)).** It is distinct from stop and close: the process
    stops, mail no longer resumes it, the daemon turns its hosted row back
    into an ordinary CLI row (a CLI ws-mcp never heartbeats a hosted row,
    `internal/mcp/dashboard_sessions.go`; a daemon restart neither recovers
    a handed-off record nor rewrites its row as hosted), and the page shows the resume
    command: `cd <dir> && claude --resume <id>`, or for Pi
    `pi --session <session file path>`, since `pi --session` resolves a
    bare id only within its session dir and a daemon Pi lead runs on
    `<state dir>/sessions/<reply-id>/pi/session.jsonl`. Rejected: reusing
    stop, which the next mail resumes ((176)); reusing close, which ends the
    conversation.
17. **Who can hand off, and from where (technical, (321)).** Claude and Pi
    top-level lead sessions (Decision 2; codex out; worker keys never
    register). CLI to daemon starts from a row in the worktree's ended list;
    daemon to terminal from the hosted session's harness chip menu beside
    stop, restart, and close. Both sit behind the loopback lead-control
    gate of `261110-feat-ws-dashboard-headless-lead-sessions` Decision 14,
    since a takeover spawns with an existing bearer key. Plain-folder
    sessions get no takeover entry, since a lead key is minted at a Git
    worktree root (`261110-feat-ws-dashboard-headless-lead-sessions`
    Decision 10). An ended row with no recorded harness session (Decision
    13) shows the entry disabled, with the reason as its tooltip
    (technical, completeness and design review).
18. **Revive: reopen a closed hosted session and take over older ended
    sessions (user, (315) a; announced, (330)).** A closed hosted session
    can be reopened: its record keeps the backend session ID and session
    directory, and reopening resumes it with the same key. This reverses
    the "close ends the conversation for good" of
    `261110-feat-ws-dashboard-headless-lead-sessions` ((176)): close still
    stops the process and drops held messages, and the session moves to the
    ended list from which it can be reopened. The ended list also reaches
    sessions older than the rail's 24 h window, as long as their registry
    directory exists: `RegistryRetention` (7 days) bounds what can be
    revived. Sessions ws never registered stay out (Decision 13).
    Closed and handed-off hosted records are pruned after the same
    `RegistryRetention` as other ended sessions; the tracker support
    agent's record stays exempt (user, (333) a). The age is measured from
    the row (its last heartbeat or EndedAt), not from ClosedAt, so a closed
    session later resumed in a terminal is not pruned while its CLI row is
    live (technical, review). `PruneRegistrations` skips
    every session directory holding an agent record today, which would
    leave them unbounded.
    Rejected: reopening closed hosted sessions only; no revive in this
    ticket; keeping closed records indefinitely or for a longer retention
    such as 30 days.

## Constraints

- The rail's grouping, row content, and worktree listing belong to
  `261110-feat-ws-dashboard-session-rail`; this ticket does not build a
  second list (2026-11-10 whiteboard, (36) ok, (62) a).
- Stopping and closing sessions belong to
  `261110-feat-ws-dashboard-headless-lead-sessions` Decision 8 and its
  header chip menu ((191) b); Decision 16 adds hand to terminal and
  Decision 18 reopen beside them.
- Scale: this ticket sets no cap on live hosted sessions and no paging in
  the rail or agents panel; revisit with telemetry once many-session use
  shows a real limit (user, (322) a).
- Manuals: `ai-docs/manuals/wsflow-mirroring.md` (Decision 13 touches
  `agents-plugin/hooks/claude-mod/`, mirrored in `agents-plugin-wsflow/`),
  `ai-docs/manuals/ws-mcp.md`.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261110-feat-ws-hosted-session-mode-flag (2026-11-10, Decisions): "A hosted session's key never changes ... a different key is refused ... Taking over another session's track becomes a daemon spawn with that key, in the consumer ticket if needed." — bearing: constrains
- ee0520fa (2026-11-06, commit): "The registry never records a session key. The daemon maps reply-id to key by scanning the key store's file names ... Liveness rests on the heartbeat alone; no pid is recorded, so pid reuse cannot fake a live session" — bearing: constrains (amended by Decision 14: a block-only harness PID; rail liveness stays heartbeat-only)
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, Decisions): "a plain-folder workspace bar (261110-feat-ws-dashboard-workspaces Decision 3) does not show it, because a lead session key is minted at a Git worktree root (user, (290) a, reversing the earlier plain-folder entry)" — bearing: contradiction-candidate
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, Decisions): "The hosted flag holds only for the process the daemon spawned ... The daemon-written row records the spawned harness PID" (b24a3f48: a one-shot CLI call from a hosted shell may rewrite the daemon's row) — bearing: constrains
- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "One trusted-origin boundary. Tool approval and the terminal are both execution authority and share one boundary: loopback or a desktop-shell origin by default, never the LAN token alone." — bearing: constrains
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Decisions): "Lazy resume after a daemon restart: sustained processes end with the daemon; each conversation restarts on its next input with the backend's resume." — bearing: supports
- 261110-feat-ws-dashboard-session-rail (2026-11-10, Decisions): "The rail groups by workspace, then worktree ... A workspace is a repository (its root worktree) or a plain non-git folder" — bearing: constrains
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "conversation handoff works both ways between CLI and daemon ((15) a); child 261110-feat-ws-dashboard-workspaces." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phases 2-3: agents-plugin/hooks/claude-mod/manual.ts (and its wsflow mirror), agents-plugin-pi/src/bridge.ts, agents-plugin-tool/internal/wsagent/runner.go, record.go, http.go; agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl, dashboard.js, registry.go; likely agents-plugin-tool/internal/mcp/dashboard_sessions.go |
| scope.surface | cross-module | new runner routes beside POST /agents/leads consumed by the page; the hosted registration row is read by the CLI ws-mcp at internal/mcp/dashboard_sessions.go#L229-L240 |
| scope.new_public_symbol | yes | takeover and hand-to-terminal routes under /agents; names not yet chosen |
| scope.new_type_contract | yes | Record needs an externally supplied backend session or Pi session file and a handed-off state; a takeover needs the CLI session's harness session ID, which no registry or session record holds today |
| scope.test_surface | existing | internal/wsagent lead_runner_test.go, lead_http_test.go, lead_recovery_test.go; internal/wsdashboard railscript_test.go and TestLANModeGuardsEveryRoute in access_test.go |
| complexity.reuse_points | confirmed | Runner.create and startConv with Record.SessionID resume claude.go#L121, DashboardSessionSource reply-id to key scan mcp/dashboard_sessions.go#L544, Stop and CloseConversation, the trusted lead-control gate http.go#L113-L121 |
| complexity.side_effect_risk | high | two processes on one harness session corrupt its transcript; the daemon claims mail for a stopped lead and resumes it, so a handed-off record must stop doing so |
| risk.correctness | high | CLI exit detection rests on a heartbeat that also stops after 12 h idle while the CLI still runs, and on EndedAt only at clean exit |
| risk.fit | moderate | takeover adopts an existing lead key through a new runner create path (Decision 12); hosted rows and runner recovery must tell handed-off records apart (Decision 16) |
| risk.test | moderate | route and state tests exist, but a real CLI exit and resume needs a live claude or pi run, which earlier phases could not run locally |
| risk.security_or_contract | high | a takeover hands an existing bearer session key to a daemon spawn and may change the registry's no-pid contract; must sit behind the loopback gate of headless Decision 14 |

## Phases

### Phase 1: stored workspaces, add workspace, add worktree

Decisions 1, 3 (only the rail header's add-workspace + and a git
workspace bar's add-worktree +), 4, 5, and 7-11. No
prerequisite beyond the session rail's grouping, which lands first. The
worktree row's + and a plain-folder bar's + open the new-session popover
(Decision 6), so neither appears before Phase 2. When a stored workspace
turns out to be a linked worktree of another root (Decision 4's merge),
detected at add time and on the worktree reread cadence, its stored entry
is rewritten to that root. Route names and JSON shapes follow the existing
dashboard route patterns.

Verification (announced, (115) ok):

- Go tests: the path normalization table (WSL, native Windows, Linux and
  macOS cases, and each refusal); subfolder-to-root resolution; the no-op,
  merge, and add results; worktree add with a new and an existing branch,
  the default folder and its same-minute suffix, and a non-empty custom
  path refused; the store round trip and forget; the gate cases in
  `TestLANModeGuardsEveryRoute`.
- A browser check: the rail header's + and a git workspace bar's + with
  their popovers as in the mockup, no + on worktree rows or plain-folder
  bars, forget, and the rail showing an added workspace and a created
  worktree at once.

### Result (76a19c0af) - 2026-11-10

Landed in 6c5fe1f2a (store, path reading, worktree add, routes, rail
grouping, route strings), 2df881ff6 and 93ac75598 (rail markup, popovers,
forget, page strings; the first holds only a test-file rename), and
76a19c0af (preview wording).

- Decision 7: `workspaces.go`. The list is
  `<config home>/dashboard/workspaces.json` (`WorkspacesFilePath` beside the
  global config file, wired in `cmd/ws-mcp/dashboard.go`), a versioned JSON
  document of paths written atomically and owner-only. Whether an entry is a
  repository is derived at each read, not stored. A daemon started without
  `DaemonOptions.WorkspacesFile` keeps no store: no routes, no + buttons.
- Decisions 1, 4: `workspacepath.go` reads a pasted path as a pure function
  of a host value (WSL, native Windows, Linux and macOS rules, each
  refusal). `locateWorkspace` resolves a folder to its workspace by the
  nearest `.git` at or above it, without running git: a `.git` directory is
  a root; a linked worktree's `.git` file leads to its root; any other
  `.git` file (a submodule) leaves the folder a plain workspace, as the rail
  shows a session rooted there. Results are `added`, `noop`, and `merged`.
  A merge is also detected by a sweep that runs before each full
  worktree-list read (the reread cadence) and rewrites the entry.
- Decision 5: `railworktreeadd.go`. New branch from a base (default: the
  root's current HEAD) or an existing branch no worktree has checked out;
  default folder `<root>/../.ws-dashboard-worktrees/<root-dir-name>/<YYMMDD-HHMM>`
  with `-2`, `-3` on a clash; a custom folder must be absolute and empty or
  absent. The daemon refuses what git should not see (option-like or
  invalid names, a taken or checked-out branch, a missing base) before it
  runs git, under the one-git-action-per-root slot.
- Decisions 10, 11 and the routes: `workspace_routes.go`. All are POST with
  a JSON body behind `sameOrigin` only:
  `/rail/workspaces/preview`, `/rail/workspaces/add`,
  `/rail/workspaces/forget`, `/rail/worktrees/options`,
  `/rail/worktrees/add`. The preview is a POST because the gate needs an
  Origin header. The worktree routes act only on a repository the rail
  shows (a stored one or a listed session's). Every answer text is in the
  page's language. Add, forget, and a created worktree wake the rail at
  once; a created worktree also rereads that one repository's list
  (`railLister.reread`).
- Decision 8: `buildRailView` takes the stored list. A session in no
  repository joins the longest stored plain folder at or above its root; a
  repository session stays under its repository; a stored and a derived
  workspace of one root are one bar; a stored repository lists its
  worktrees without a session.
- Decisions 3, 9: the rail header's + (add workspace) and a repository
  bar's + (add worktree) show only on a daemon that keeps stored
  workspaces; a stored bar has forget; worktree rows and plain-folder bars
  have no +. The workspace bar became a container holding the collapse
  toggle and sibling buttons, since buttons cannot nest. The popovers live
  outside the rail region and survive its swaps. The mockup's paste-example
  buttons were left out as a demo aid.

Verification:

- `go test ./internal/wsdashboard/ ./internal/wsi18n/ ./cmd/ws-mcp/ -count=1`
  and `go vet` on the same packages pass at 76a19c0af (run on the remote
  build machine). Tests added: `TestNormalizeWorkspacePath`,
  `TestLocateWorkspace`, `TestWorkspaceStoreAddResults`,
  `TestWorkspaceStoreRoundTripAndForget`, `TestWorkspaceStoreSweep`,
  `TestWorkspaceRoutesNeedTheDaemonsOrigin`,
  `TestWorkspacePreviewAddAndForget`, `TestWorktreeAddRoute` (real git),
  `TestAddWorktreeDefaultFolder`, `TestRailViewShowsStoredWorkspaces`,
  `TestRailListerSweepsOnFullReadsAndRereadsOneRepository`,
  `TestRailMarkupAddButtonsAndForget`,
  `TestSessionPageOffersTheRailAddButtons`, and the five routes in
  `TestLANModeGuardsEveryRoute`.
- Browser check: run in headless Chromium at 1700px, 1200px, and 390px
  against a rendered fixture page with the five routes stubbed; every
  listed item was seen (both + buttons and their popovers, no + on worktree
  rows or plain-folder bars, forget, the rail showing an added workspace
  and a created worktree at once). Not seen in a browser: a real daemon
  with real git behind the page (no daemon build on this machine; the Go
  route tests cover that side), and a hover-capable pointer (headless
  reports `hover: none`; the hover reveal was checked with that rule
  removed).
- Lite review: clean, two minor notes left as they are (an index-less
  daemon renders an empty rail though the routes exist, unreachable in the
  shipped wiring; quote stripping takes every leading and trailing quote
  character, not one matching pair).

Found on the way: the rail script tests had never run. Go reads the `_js`
suffix of `rail_js_test.go` as a build constraint for the js platform; the
file is now `railscript_test.go` and both tests run and pass.

### Phase 2: harness session record, PID, and login guard

Decisions 13, 14, 15, and the handed-off state of 16. Needs
`261110-feat-ws-dashboard-headless-lead-sessions` (done). The new-session +
buttons of Decision 3 and the popover of Decision 6 were built by that
ticket instead ((189)); Phases 2 and 3 build handoff and revive only. This
phase lays the records and the guard that make two holders of one harness
session impossible before any takeover exists.

1. **Harness session on the key record (Decision 13).** The Claude mod
   (and its wsflow mirror) and the Pi extension report the harness session
   (Claude session id; Pi session file path) and the working directory at
   login; ws-mcp stores them on the ws key record, the directory apart from
   `Record.Root`.
2. **Block-only harness PID (Decision 14).** CLI rows record the harness
   PID; rail liveness stays heartbeat-only.
3. **Login guard (Decision 15).** A CLI login to a key whose hosted row's
   `HostPID` is alive is refused, except from inside the hosted session
   itself; the Claude mod and the Pi extension stay keyless and show the
   hand-off message. A CLI login to a hosted row with no live process
   rewrites it into a CLI row with the harness PID, and the daemon releases
   the conversation: the ownership check before every row write, mail
   drain, and mail resume and at start, as one step with the guarded
   action; heartbeat and mail watch stopped; the record handed off; claimed
   mail returned to the inbox.

Verification:

- Go tests for: the key-record fields written at login, the directory kept
  apart from `Record.Root`; the PID recorded on a CLI row; the CLI login
  refusal against a live `HostPID`, and the hosted session's own login
  still accepted; the implicit hand-off on a non-live hosted row, after
  which the daemon neither rewrites the row nor drains or resumes on its
  mail, claimed mail reaches the inbox again, and a daemon restart does not
  reclaim it.
- Pi and Claude mod tests for the reported session fields and for staying
  keyless with a visible message on a refused login (Claude mod and its
  wsflow mirror).

### Result (29a561a21) - 2026-11-12

Landed in 6a991d903 (ws-mcp records and guard, runner release), 10bfc7915
(Claude mod and its wsflow mirror), and 29a561a21 (Pi bridge).

- Decision 13: `ferrule` takes two unadvertised arguments on a top-lead
  login, `harness_session` and `harness_cwd`, and stores them with the
  harness name on the key record (`harness`, `harness_session`,
  `harness_cwd`; `internal/mcp/hosted_login.go`). The pair is recorded only
  whole (a non-empty session and an absolute directory); anything else
  records nothing and the login still succeeds. The Claude mod reports the
  session id and `$.session.cwd()` on the mint and on every re-login; the
  Pi bridge reports the session file path and its cwd.
- Decision 14: a terminal ws-mcp writes `harness_pid` on its row. Liveness
  still rests on the heartbeat; `HostMayRun` and `HarnessMayRun`
  (`internal/wsdashboard/registry.go`) are the only readers of the PIDs.
- Decision 15: `hostedLoginGuard` refuses a re-login to a hosted row whose
  process may run, with the marker `hosted-session-held`; both adapters
  match the marker, mint nothing, keep their stored key entry, and show the
  refusal. A hosted row with no process is rewritten into the terminal's
  row, and the runner releases the conversation (`conv.release`): record
  `handed_off_at`, mail watch and heartbeat ended, claimed mail returned
  through `LeadMail.Return`, state `handed_off`.
- Added beyond the plan text, to close a window the plan's `HostPID`-alive
  test leaves open: a hosted row with `host_pid` 0 is written both while a
  spawn is under way and while the session is stopped. The runner now
  marks the second (`host_stopped`), the guard refuses the first, and the
  runner's check-and-spawn and the guard's check-and-rewrite both run under
  a row lock (`registration.json.lock`), so neither can decide on a row the
  other is changing.
- A terminal ws-mcp whose dashboard key turns off keeps a row whose
  directory holds the runner's record instead of removing it: with the row
  gone the daemon would write its own again and resume the conversation.

Verification:

- `go vet ./internal/wsdashboard/ ./internal/mcp/ ./internal/wsagent/ ./cmd/...`: clean.
- `go test ./... -count=1` at 6a991d903: every package ok. Tests added:
  `hosted_login_test.go` (key-record fields and the directory apart from
  the root, the PID on a terminal row, the refusal against a live
  `host_pid` and against a spawn under way, the hosted session's own login,
  the row taken when no process runs, mail text round trip and return),
  `lead_handoff_test.go` (the row's stopped mark, the release with no row
  write, mail claim, or spawn after it and across a daemon restart, a row
  taken before a spawn, a row taken while no daemon ran),
  `registry_holder_test.go`.
- `TestHostedReconnectAdoptsOnlyLiveSamePIDRecords` failed in two of the
  full runs of this ticket and passed 25 of 25 when repeated alone. It
  seeds a mailbox record named `alpha-stem` and expects it first in name
  order, and fails when a randomly minted name sorts before it
  (`aloe-banner-flashbulb`); it does not touch this phase's code.
- Claude mod: `claude plugin test` on both packages, 732 pass, 0 fail
  (6 new); the two `claude-mod` directories are byte-identical and
  `go test ./internal/wsrsrc/` (the mirror drift tests) passes.
- Pi: `npm test -- test/lead-key-hosted.test.ts test/lead-key-resume.test.ts`,
  27 pass, 0 fail. The full Pi suite has 33 failures in eight files that
  do not touch the login (`agent-channel*`, `local-devenv`,
  `persistent-explore`, `spawner`, `web-*`); the same files fail 34 at the
  commit before the change.

### Phase 3: takeover, hand to terminal, and revive

Decisions 2, 12, and 14-18. Needs Phase 2.

1. **Takeover, CLI to daemon (Decisions 12, 14, 17).** A runner create path
   that adopts an existing ws key and resumes the recorded harness session
   in the recorded directory (Pi: the external session file), refused while
   the recorded PID is alive, and for a row with no recorded PID unless it
   carries EndedAt; entry from an ended-list row, behind the loopback gate;
   plain-folder sessions get no entry, and a row with no recorded harness
   session shows it disabled with its reason.
2. **Hand to terminal (Decisions 16, 17).** Reuses Phase 2's handed-off
   state and adds the explicit stop, the hosted row turned back into a CLI
   row, the resume command on the page,
   the harness chip menu entry.
3. **Revive (Decision 18).** Reopen for a closed hosted session from the
   ended list; the ended list reaches sessions older than 24 h while their
   registry directory exists; reopen is refused once the row is a CLI row
   (the Phase 2 ownership check); closed and handed-off hosted records are
   pruned after `RegistryRetention` measured from the row, the tracker
   support agent exempt.

Verification:

- Go tests for: the PID block (alive refuses, dead allows, absent allows
  only with EndedAt); takeover adopting the key and passing `--resume <id>`
  with the recorded directory (Pi: the session file), and later restarts
  keeping that directory; hand to terminal not resuming on mail and
  rewriting the row, and a daemon restart after it not reclaiming the row;
  key continuity from daemon to terminal (a terminal resume re-logs in with
  the same key, Claude mod store entry and Pi session file, never a fresh
  mint); reopen of a closed hosted record, and reopen refused after an
  implicit hand-off; the ended list past 24 h; the prune of closed and
  handed-off records by row age with the support agent exempt;
  every new route in `TestLANModeGuardsEveryRoute` and behind the loopback
  lead-control gate.
- Browser check of the ended-list takeover and reopen entries (including
  the disabled entry) and the chip menu's hand-to-terminal entry with its
  resume command.
- A live round trip when a harness is available: a terminal Claude lead
  exits, the daemon takes it over and continues the same transcript, hands
  it back, and `claude --resume` continues it in the terminal; record it as
  not run otherwise.

### Result (f2a6fbab5) - 2026-11-12

Landed in 907d3b859 (runner moves, routes, rail data, prune), 6dd41e687
(hand-off retries a leftover holder), cf90d9887 (page mark for a handed-off
session), f80a0b2ad (page UI and strings), and f2a6fbab5 (review fixes).
Every surface this phase and Phase 2 touch is additive (new routes, new
optional row, record, and key-record fields, two unadvertised `ferrule`
arguments): compatibility tier T0.

- Decisions 12, 14, 17 (takeover): `Runner.TakeOver`
  (`internal/wsagent/handoff.go`), `POST /agents/<id>/takeover`. It finds
  the key behind the row (`AgentKeyStore.LeadSession`), keeps it, and
  resumes the harness conversation the key record names in the directory
  that harness ran in; both stay on the agent record (`cwd`,
  `session_file`), so later starts run there too. Pi runs on the reported
  session file without `--session-dir`. Under the row lock it is refused
  while `Registration.HarnessMayRun`: a recorded harness PID alive, or no
  PID on a row with no `ended_at`. Also refused: a hosted row, an agent
  conversation, a harness other than claude and pi, no lead key at a git
  root, no recorded conversation, a directory or Pi file that is gone.
- Decisions 16, 17 (hand to terminal): `Runner.HandOff`,
  `POST /agents/<id>/handoff`. The process ends (a leftover holder is
  retried and a failure fails the move), the row becomes an ended terminal
  row with no PIDs, the record is released as in Phase 2, and the state
  carries `resume_command`: `cd <dir> && claude --resume <id>` or
  `cd <dir> && pi --session <file>`. The `cd` is added to the decision's Pi
  command so project context loads as in the daemon's run. The command is
  answered to a `can_control` viewer only, since it names local paths.
- Decision 18 (revive): `Runner.Reopen`, `POST /agents/<id>/reopen`,
  refused once the row is a terminal's and for a handed-off record. The
  rail's ended list has no 24-hour cut (the index page keeps its own);
  `RailRow.Revive` and `ReviveBlocked` say how a row comes back. Prune ages
  a closed or handed-off lead by its row; every other agent record keeps
  the runner's lifetime.
- All three routes sit behind the trusted gate for every id (a takeover
  names a session the runner does not hold yet).
- Page: an ended row carries Take over or Reopen beside its link (disabled
  with its reason when the key record names no conversation); the chip
  menu has Hand to terminal; a handed-off session shows a notice with the
  resume command and a copy button, also after a reload (`data-handed`,
  set while the row is an ended terminal row and the record is handed
  off). The PID block is answered at the click (409 text), not precomputed
  on the rail.
- A terminal ws-mcp re-reads the row under the lock before each row write
  and stops once the row is hosted, so a server that outlived its harness
  cannot write a taken-over row back.
- Lite review: two important findings, both fixed in f2a6fbab5.
  - An accepted re-login to a row that is not hosted now marks it as the
    terminal's at once (`harness_pid`, no `ended_at`, fresh heartbeat),
    whatever the dashboard key says. Before, a session handed to a
    terminal whose dashboard key was off kept the hand-off's ended row,
    and a takeover would have run beside the terminal.
  - Prune keeps one retired lead: a handed-off Pi session whose session
    file is inside its session directory, because the terminal's Pi runs
    on that file. This narrows Decision 18's "handed-off records are
    pruned" for that one case to avoid deleting a conversation the user
    still holds; such rows stay in the ended list. It wants the user's
    confirmation.
  - Minor notes left: a hosted session's own login is accepted by PID
    match even if that PID were stale and reused (as Decision 15 states);
    the runner keeps a closed record in memory after another process
    pruned its directory, so a reopen then fails on the record write
    instead of answering unknown.

Verification:

- `go vet ./...`: clean. `go test ./... -count=1` at f2a6fbab5: every
  package ok.
- Tests by the plan's list: the PID block
  (`TestTakeoverWaitsForTheTerminalHarnessToExit`); adoption, `--resume`,
  the directory, and restarts keeping it
  (`TestTakeoverAdoptsTheKeyAndResumesWhereTheHarnessRan`,
  `TestTakeoverOfAPiSessionRunsOnItsOwnSessionFile`,
  `TestTakeoverRefusals`); hand to terminal, mail not resuming, the row,
  and a restart not reclaiming it
  (`TestHandOffGivesTheSessionToTheTerminal`,
  `TestHandOffThroughTheRoute`, `TestHandOffNeedsTheProcessEnded`,
  `TestResumeCommand`); key continuity
  (`TestReloginAfterAHandOffKeepsTheKey`,
  `TestReloginMarksTheRowAsHeldByTheTerminal`; the Claude mod's hosted
  start stores the daemon key under the session id and a later start
  re-logs in with a stored key, each pinned in `manual-engine.test.ts`; Pi
  in `lead-key-hosted.test.ts` and `lead-key-resume.test.ts`); reopen and
  its refusal (`TestReopenOpensAClosedHostedSession`); the ended list past
  24 h and the row entries (`rail_test.go`, `TestRailRowRevive`,
  `TestRailMarkupRevive`); the prune
  (`TestPruneRegistrationsAgesRetiredLeadsByTheirRow`); the routes in
  `TestLANModeGuardsEveryRoute` and the lead-control gate
  (`lead_http_test.go`); the page mark (`TestHandedOffSessionMark`,
  `TestPageMarksASessionHandedToATerminal`); the row guard
  (`TestTerminalServerLeavesARowTheDaemonTookOver`).
- Browser check: headless Chromium at 1700px, 1200px, and 390px against
  rendered fixture pages with the routes stubbed. Seen at each width: the
  Take over and Reopen entries in an ended list, the disabled entry with
  its reason (a click posts nothing), a refusal text after a failed
  takeover, Hand to terminal in the chip menu, the notice with the command
  and its copy button after a hand-off and on a page loaded with
  `data-handed`. Not seen: a real daemon behind the page, the notice going
  away again, the reload branch of a revive on the session's own page, and
  the hand-off confirmation with background tasks.
- Live round trip (terminal Claude lead, takeover, hand back,
  `claude --resume`): not run; no daemon build on this machine.
- `TestHostedReconnectAdoptsOnlyLiveSamePIDRecords` failed in some full
  runs for the reason recorded under Phase 2 and passed in the last one.
