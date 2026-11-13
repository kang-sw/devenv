---
title: "Werkschaft thin Tauri shell around the local dashboard daemon"
related:
  261112-research-ws-ade-desktop-shell-and-rename: "Werkschaft concept; (493)=b builds this shell now"
  261110-epic-ws-dashboard-agent-host: "its shell-after-dashboard order (18)(19) is reversed by (493)=b"
  261109-research-windows-code-signing-signpath: "signing for the shipped exe/zip"
  261113-refactor-ws-dashboard-drop-lan-serving: "local-only daemon"
---

# Werkschaft thin Tauri shell around the local dashboard daemon

## Background

Owner direction, 2026-11-13 ((493)=b, recorded in
`261112-research-ws-ade-desktop-shell-and-rename`, whose Proposals and Open
Questions hold the shell research): build a thin Tauri shell now, in
parallel with the browser dashboard. The owner's reasons: a thin shell
without self-update allows rendering verification in the native webviews
and is an easy way to hand someone one packaged exe (zip at first, an
installer as soon as possible); daemon updates far outnumber shell updates,
which suits dogfooding; the "updater" is just a daemon exe placed beside the
shell. Ticket creation approved by the owner as (523)=ok, with the EXEC
lead's daemon-ownership recommendation.

## Decisions

1. **Thin shell scope** (MAIN lead's scope, owner-approved via (493)=b and
   (523)=ok): the shell attaches to a running daemon, or launches the daemon
   exe placed beside it; it loads `http://127.0.0.1:<port>` directly (a
   `tauri://` origin is refused by the same-origin write routes); it runs as
   a single instance; external links open in the system browser; closing
   the window leaves the daemon running; the shell does not update itself.
   Rejected: a bundled frontend or custom protocol (breaks same-origin
   writes); Electron (research Proposals: the sidecar dominates size, and
   Electron stays the fallback only if the webview matrix proves painful).
2. **Daemon ownership** ((523)=ok, EXEC recommendation): until self-update
   lands, the plugin launcher's SHA256-pinned daemon stays the authority.
   The shell attaches to whatever daemon is running, starts its beside-exe
   daemon only when none is, and never replaces or stops a running daemon.
   Revisit when the app self-updates, because an app-driven daemon update
   can raise the minor and lock out plugin sessions on the previous one.
   Rejected for now: the app owning the daemon and the launcher deferring
   to it (two install paths competing before a cross-version policy
   exists).
3. **Attach contract** (research Proposals, shell-agnostic): run the bundled
   `ws-mcp daemon-start` and treat its JSON outcome as the attach/start
   result; read `port` from `endpoint.json` and confirm `GET /health` before
   loading; never hand the endpoint token to the webview.
4. **Packaging**: a zip of the shell plus the daemon exe first; signing is
   out of scope here and tracked by `261109-research-windows-code-signing-signpath`
   (unsigned builds hit SmartScreen on Windows and Gatekeeper on macOS).
5. **Compatibility tier: T0**: a new artifact; no daemon tool, store, or
   protocol change.

## Constraints

- The shell adds no new daemon route; anything it needs from the daemon goes
  through existing endpoints.
- Native notifications, tray, login items and deep links are out of scope
  for the thin shell (research Topic 1 lists them for later).
- Do not reintroduce `claude-plugin/` or add root-level module directories;
  the shell lives under a package directory chosen with the owner's layout
  rules (AGENTS.md Architecture Rule 2) - the worker proposes the directory
  in its first report before creating it.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: Thin shell that attaches or starts the local daemon

Build the shell per Decisions 1-3 for macOS (WKWebView) and Windows
(WebView2), packaged as a zip with the daemon exe (Decision 4).

Verification: with a daemon already running, the shell attaches and leaves
it running on close; with none running, it starts the beside-exe daemon; a
second launch focuses the first window; an external link opens the system
browser; a dashboard write route (for example sending a message) works from
the shell window; LOOK captures of the dashboard in WKWebView and WebView2
compared with headless Chromium, with differences listed.
