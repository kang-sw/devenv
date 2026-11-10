---
title: "Dashboard agent host: workspaces (root worktree or plain folder), per-workspace sessions, and conversation handoff between CLI and daemon"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-headless-lead-sessions: "prerequisite of Phase 2 only (new session, conversation handoff)"
  261110-research-ws-dashboard-agent-host-foundations: "Topic 4 (multi-session and workspace management)"
  261110-feat-ws-dashboard-session-rail: "owns the rail's workspace and worktree grouping; this ticket adds user-added workspaces and the + buttons on top"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0e5d2b5693854b6b
sage-review-completeness-reviewed: 0e5d2b5693854b6b
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

## Constraints

- The rail's grouping, row content, and worktree listing belong to
  `261110-feat-ws-dashboard-session-rail`; this ticket does not build a
  second list (2026-11-10 whiteboard, (36) ok, (62) a).
- To settle at Phase 2 promotion: stopping and archiving sessions; how the
  agents panel and the session page scale to many sessions.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "One trusted-origin boundary. Tool approval and the terminal are both execution authority and share one boundary: loopback or a desktop-shell origin by default, never the LAN token alone. LAN viewers can watch and chat." — bearing: constrains
- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "Workspace = root worktree or plain folder. Linked worktrees show under their root; idle ws pooled worktrees are not shown ((54)); one that holds a session shows while it does ((71) a)." — bearing: supports
- 261110-feat-ws-dashboard-session-rail (2026-11-10, Decisions): "The rail groups by workspace, then worktree ... A workspace is a repository (its root worktree) or a plain non-git folder ... rereads a repository's worktree list at most every 3 minutes and only while some page showing the rail is open" — bearing: constrains
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "A workspace is a root worktree or a plain folder ((08) b, (11)); conversation handoff works both ways between CLI and daemon ((15) a); child 261110-feat-ws-dashboard-workspaces." — bearing: supports
- 12831114 (2026-11-10, commit): "Detection reads <Path>/.git only, never runs git: a gitdir: file whose target's parent is worktrees, common dir taken from the gitdir's commondir when present (else two levels up)" — bearing: constrains
- 260524-feat-ws-dashboard-add-git-worktree-ui (2026-05-24, Decisions): "Add worktree... is a Git operation, not a root-picker open operation. It should create a linked Git worktree through git worktree add, then rely on the existing linked-worktree discovery" (archived MVP, owner-authenticated routes) — bearing: supports
- 260524-feat-ws-dashboard-add-git-worktree-ui (2026-05-24, Decisions): "Do not add git worktree remove, branch deletion, branch rename, remote branch tracking, prunable cleanup, repair/recover flows, or direct child workRoot forget/remove controls in this ticket." — bearing: constrains
- 341d1913 (2026-05-15, commit): "Recorded the safety boundary that the dashboard should not offer generic folder deletion, while future child tickets may expose guarded Git worktree lifecycle actions for linked worktrees." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 needs a daemon workspace store, path preview and add routes, worktree options and add routes, the rail merge, and popovers in agents-plugin-tool/internal/wsdashboard/assets/dashboard.js; ticket names no Go file |
| scope.surface | cross-module | new daemon HTTP routes consumed by the page JS; stored workspaces feed the session rail grouping owned by 261110-feat-ws-dashboard-session-rail |
| scope.new_public_symbol | yes | new daemon routes for workspace preview and add and for worktree add; names not yet chosen |
| scope.new_type_contract | yes | a persisted workspace list file and the preview and add request and response JSON |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard route tests such as sysopen_route_test.go and gitpush_test.go, plus runNode JS cases |
| complexity.reuse_points | unconfirmed | sameOrigin inbox_routes.go L122, localViewer access.go L136, writePrivateFile endpoint.go L97, support.json store pattern wsagent/support.go L35, worktreeMain peerfolder.go L22 read; rail grouping is pending 261110-feat-ws-dashboard-session-rail |
| complexity.side_effect_risk | high | daemon runs git worktree add, creates branches and folders, and persists a list on the daemon machine |
| risk.correctness | high | path normalization across WSL, native Windows, Linux, macOS plus subfolder-to-root resolution and merge rules |
| risk.fit | moderate | builds on the session rail grouping, which has not landed |
| risk.test | moderate | OS-specific normalization needs per-OS cases; popover and toast behavior needs a browser check |
| risk.security_or_contract | high | host-writing routes open to LAN token holders under sameOrigin only (Decision 10) |

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

### Phase 2: new session and conversation handoff

Decisions 2, 3 (the new-session + buttons), and 6. Needs
`261110-feat-ws-dashboard-headless-lead-sessions` done first. Scope and
verification are settled at promotion.
