---
title: "ws-ade: a standalone desktop app (Tauri or similar) around the dashboard daemon, and renaming the product to ws-ade"
related:
  261110-epic-ws-dashboard-agent-host: "its Scope defers a desktop shell until the browser dashboard is complete ((18) a, (19)); this research does not change that order"
  261110-research-ws-dashboard-agent-host-foundations: "holds the earlier Tauri-vs-Electron question; this ticket carries it forward"
---

# ws-ade: desktop shell and product rename

## Background

User, 2026-11-12 (whiteboard topic `ws-ade`): package the dashboard as a
standalone application (Tauri named) that runs without the user first
starting a harness such as Claude Code in a terminal, and make it a product
called ws-ade, renaming the project from devenv.

The browser dashboard already opens headless lead sessions, hosts their
processes in the session holder, and gives a browser terminal, so the app
is mainly a shell that bundles the daemon as a sidecar and opens a window.

## Topic 1: desktop shell

- Tauri against Electron (or another option) for wrapping the existing web
  UI with the Go daemon as a sidecar: bundle size, WebView consistency on
  macOS, Windows, and Linux, auto-update, code signing.
- What the shell adds beyond the browser: a trusted local origin, native
  notifications, tray/menu, OS login items, deep links.
- How the app finds or installs the harness CLIs (claude, pi, codex) it
  starts, and how it coexists with a daemon a terminal session already
  started.

## Topic 2: rename to ws-ade

- Every place the repository or root directory name is a key: per-project
  cache and state paths (for example `~/.cache/ws@<owner>-<repo>/`), the
  worktree pool default `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)`,
  dashboard session registrations, whiteboard and tracker store paths,
  marketplace entries, `install.sh`, GitHub URLs, docs.
- A migration path so a renamed checkout keeps its sessions, boards, and
  registrations.
- Where the product name shows (app title, dashboard header) versus ids that
  stay.

## Outcome Ledger

### Verified Findings
- None yet.

### Confirmed Decisions
- Research the desktop shell now; its implementation keeps the agent-host
  epic's order, after the browser dashboard is complete (user, (574) b).
- Rename scope: the repository/project and the user-visible product name
  become ws-ade (app and dashboard labels); the plugin ids `ws` and `wsflow`
  stay, since changing them would break every downstream install
  (user, (575) b).

### Proposals
- None yet.

### Open Questions
- Topics 1 and 2 above.

### Rejected Alternatives
- Starting the desktop shell implementation now ((574) a): the dashboard
  still changes quickly, so a shell built now would be reworked.
- Renaming the plugin ids ((575) c): a T2 break for every downstream install.
