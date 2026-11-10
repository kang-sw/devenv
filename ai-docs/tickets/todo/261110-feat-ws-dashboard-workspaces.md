---
title: "Dashboard agent host: workspaces (root worktree or plain folder), per-workspace sessions, and conversation handoff between CLI and daemon"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-headless-lead-sessions: "prerequisite of Phase 2 only (new session, conversation handoff)"
  261110-research-ws-dashboard-agent-host-foundations: "Topic 4 (multi-session and workspace management)"
  261110-feat-ws-dashboard-session-rail: "owns the rail's workspace and worktree grouping; this ticket adds user-added workspaces and the + buttons on top"
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
`ai-docs/ref/session-rail-mockup/index.html` (v13), shows the + buttons and
their three popovers.

Today the daemon has no stored workspace list, no endpoint that runs
`git worktree add` (the only add is `ws/worktree.acquire`, a session's tool
call), and no endpoint that starts a lead session (Explore, 2026-11-10).

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
   `claude --resume <id>` command. One session ID is never held by two
   processes at once: before a handoff the previous process must have
   exited.
   Rejected: CLI to daemon only ((15) b); daemon sessions only ((15) c).
3. **Three + buttons on the rail (user, 2026-11-10; announced, (53) ok;
   mockup v13).** Each shows only while its line is hovered or focused
   (always, faintly, on touch screens) and sits right after its label: the
   rail header's "sessions +" adds a workspace; a workspace bar's
   "<path> +" adds a worktree (on a plain folder it opens the new-session
   popover instead); a worktree row's "<branch> · <folder> +" opens the
   new-session popover for that worktree.
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
   plus a line naming the target worktree and a start button. Rejected:
   a second picker.

## Constraints

- The rail's grouping, row content, and worktree listing belong to
  `261110-feat-ws-dashboard-session-rail`; this ticket does not build a
  second list (2026-11-10 whiteboard, (36) ok, (62) a).
- To settle at promotion: where workspaces are stored; how an existing CLI
  session is matched to a workspace; stop, archive, and remove; how the
  agents panel and the session page scale to many sessions; whether adding
  a workspace or a worktree sits behind the epic's trusted-origin boundary
  (technical, (78) ok: both write on the daemon's machine, and the
  boundary's rule is still being shaped by the headless-lead and terminal
  children).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.

## Phases

### Phase 1: stored workspaces, add workspace, add worktree

Decisions 1, 3 (the workspace and worktree + buttons), 4, and 5. No
prerequisite beyond the session rail's grouping. Scope and verification
are settled at promotion, after the Constraints' open items.

### Phase 2: new session and conversation handoff

Decisions 2, 3 (the new-session + buttons), and 6. Needs
`261110-feat-ws-dashboard-headless-lead-sessions` done first. Scope and
verification are settled at promotion.
