---
title: "Dashboard memo pad: a post-it panel beside the session rail, stored per workspace in the daemon"
---

# Dashboard memo pad: a post-it panel beside the session rail, stored per workspace in the daemon

## Background

User request (2026-11-12): Ctrl+` opens the session rail as an overlay below
1600px (`assets/dashboard.js` `railKey`/`railOpen`, the `rail-open` class on
the root; at 1600px and wider the rail is a permanent page column and the key
does nothing, see `railWide()`). The user wants a "post-it" memo panel there:
one panel that appears to the right of the session rail, backed by a very
simple note store local to the daemon, where the user adds and removes short
memos for the root the current session belongs to.

The user approved the mockup ("포스트잇도 마음에 들고", 2026-11-12); its
behaviour is restated below so the ticket does not depend on the scratch file.

## Decisions

- **Scope is the rail workspace** (whiteboard (477) a): memos belong to a rail
  workspace entry (the `Workspaces` the rail template renders, keyed by its
  `Path`): one memo list per repository, shared by every worktree session of
  that repository; a non-git folder workspace has its own list. Rejected: one
  list per worktree root (pooled worktrees are reused and removed, so memos
  would stay on unrelated work or vanish).
- **Panel behaviour**: opening the rail also shows the memo panel at its right
  edge; its header names the workspace. A single input at the top adds a memo
  on Enter (Shift+Enter for a newline) and the new memo goes first; opening
  the panel focuses the input. Clicking a memo's text edits it in place (Enter
  saves, Esc cancels); a hover ✕ deletes it, with a 5-second undo. Text is
  plain; only URLs become links. Each rail workspace bar shows a memo-count
  badge. Choosing another session in the rail switches the panel to that
  session's workspace.
- **Wide screens** (whiteboard (478) a): from 1600px, where the rail is a
  permanent column, Ctrl+` toggles the memo panel alone, beside the rail
  column. Rejected: a memo area fixed under the rail column (both get short
  when the rail is long); a button-only opener on wide screens.
- **One motion for both widths** (user, 2026-11-12): narrow: the rail slides
  in from the left, then the memo panel slides out from behind the rail's
  right edge. Wide: the always-visible rail lifts slightly (a stronger shadow
  and a few pixels up, above the scrim), then the memo panel slides out from
  behind it the same way. `prefers-reduced-motion` shows both without motion.
- **Storage** (whiteboard (479), technical): one small JSON file per
  workspace in the daemon's state directory, read when the panel opens and
  rewritten whole on each change; no index, no live sync between tabs (another
  tab sees changes the next time it opens the panel). Memos never go to git or
  to another machine.
- **Who may write** (whiteboard (480), technical): only a viewer the daemon
  treats as this machine (`localViewer` in `internal/wsdashboard/access.go`,
  the gate terminals use) may add, edit, or delete; a LAN viewer sees the memos
  read-only.
- **Not in 0.49** (whiteboard (476)): implemented after the 0.49 release.

## Constraints

- New daemon routes and a new on-disk store: additive, compatibility tier T0
  (new route, new store with no prior shape). Give the store file a
  `schema_version` so a later change can be detected.
- The memo panel must not break the existing rail behaviour: Escape and the
  scrim close both; the rail's own Ctrl+` hand-over from a focused topic body
  frame keeps working.
- Layout works at phone width (the panel may stack under the rail there).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Memo store, routes, and the panel beside the rail

Add the daemon-side memo store and read/write routes gated as in
`## Decisions`, the memo panel and its motion in the dashboard assets, and the
workspace badge in the rail. Tests: store round trip and schema version,
route refusal for a non-local viewer's write, the badge count in the rail
render, and the panel's add/edit/delete/undo flow in the existing asset test
harness where one exists.

Verification: the wsdashboard package tests through
`~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test ./internal/wsdashboard/ -count=1`
(never run heavy Go builds or tests locally), plus headless Chromium
screenshots of the open panel at a narrow and a wide width.
