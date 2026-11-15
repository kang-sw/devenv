---
title: "Session-less local browsing anchor for the workspace picker and the worktree page"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "its Phase 4 (workspace-picker consumer) is gated on this ticket's decision"
  261114-feat-ws-dashboard-worktree-view: "consumer: a worktree page with no agent session needs files and terminals rooted at a worktree"
  261115-feat-ws-dashboard-explorer-root-switcher: "neighbour: shares the worktree enumeration source"
---

# Session-less local browsing anchor for the workspace picker and the worktree page

## Background

Two approved consumers need to browse local folders without an agent session:

- The directories-only workspace picker (epic; 755=a home and local-volume
  roots labeled with the daemon host, 756=a local-viewer only, 757=b hidden
  entries translucent). Its natural host is the rail add-workspace popover,
  which has no session.
- The worktree page (`261114-feat-ws-dashboard-worktree-view`; 784=b new page,
  785=ok explorer reuse and terminals with `cwd` at the worktree, no agent
  session).

Today (evidence gathered 2026-11-15):

- Every explorer route is session-scoped: `/s/<reply-id>/fs/{list,preview,raw,find,search}`
  and `POST /fs/act`. `browseRoot` (`fs_routes.go`) needs a live indexed
  session with a project; `root=<abs>` is honored only for a local viewer
  (loopback peer and loopback Host, `access.go` `localViewer`) that is not a
  cross-site fetch, and `browseRootParam` accepts any absolute plain-volume
  directory.
- Terminals are session-scoped too: `/s/<reply-id>/terminals` needs
  `offersTerminals` (terminals enabled and a local viewer), same-origin, and a
  live session; `terminalFolder` already accepts `folder:"path"` with any
  existing absolute directory, and terminals are keyed and orphan-tracked by
  session ID (`terminal_routes.go`).
- Session-less routes exist as precedent: `/rail/workspaces/{preview,add,forget}`
  and `/rail/worktrees/{options,add,delete}` take folder paths, gated by POST
  and `sameOrigin` (not `localViewer`, except worktree delete), normalizing
  with `normalizeWorkspacePath` (`workspace_routes.go`, `workspacepath.go`).
  The index page `/` serves the rail without a session. No session-less route
  lists a directory.
- The daemon knows its own home (`daemonPathHost`) but the browser does not
  receive the home path, a local-volume list, or a host label.

## Decisions

Pending the Open Decision Queue below.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Read-only; no filesystem mutation, no new symlink permissions; normal Files
  root guards unchanged. Any new route is additive (T0) and changes no
  existing route's semantics.
- Not authority to create an agent session, set a root flag, or control
  another session (worktree-view Constraints).
- Implementation waits for MAIN's start signal after
  `261114-refactor-ws-dashboard-whiteboard-asset-split`.

## Open Decision Queue

Last ID: (16) (working labels; owner-visible questions go to MAIN unnumbered)

(14) [open] [announced] Session-less browsing and terminals are local-viewer only and refuse cross-site fetches; remote (LAN) viewers get neither the picker nor the worktree page's files and terminals - 756=a makes the picker local-viewer only, terminals already require a local viewer, and `root=` browsing is local-viewer only today; widening remote access is outside the epic.
(15) [open] [policy] Which anchor serves session-less browsing and terminals.
- Context: the picker and the worktree page both need folder listings (and the worktree page, terminals) with no agent session; all existing routes require one.
- Alternative a: a daemon-owned host anchor - one reserved, non-session reply-id that the existing `/s/<id>/fs/*` and `/s/<id>/terminals` handlers accept only for a local viewer and only with an explicit root/folder; all guards and client code are reused; terminals created under it are listed per worktree path.
- Alternative b: a separate session-less route family (for example `/local/fs/list` and `/local/terminals`) mirroring the session routes with an explicit root; explicit and isolated, but duplicates handler surface and tests.
- Alternative c: borrow a live session of the same repository and pass `root=`/`folder:"path"`; zero backend change, but unusable when no session of that repository is live, and terminals die or orphan with the borrowed session.
> Recommend a: one additive, local-viewer-only anchor reuses every existing guard and the existing explorer and terminal clients, and serves both consumers; b costs a parallel surface for the same rules; c fails the stored-workspace-without-sessions case the rail already shows.
(16) [open] [technical] The picker's starting roots (daemon host label, home, local volumes) come from one additive local-viewer-only field or route alongside the anchor, over hard-coding paths in the browser - the browser cannot know the daemon user's home folder or mounted volumes, and `daemonPathHost` already resolves home server-side.

## Phases

### Phase 1: Session-less anchor and root discovery

Drafted after the queue settles. It will add the chosen anchor and the
picker's root discovery, with verification that remote viewers and cross-site
fetches are refused, that listings stay read-only and symlink-guarded, that
existing session routes behave unchanged, and that a terminal opened through
the anchor starts in the requested worktree path. The picker consumer (card
ticket Phase 4) and the worktree page then build on it.
