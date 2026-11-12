# Plan: investigate ws-ade desktop shell and rename (research ticket)

## Intent

User decision (2026-11-12, (574) b): research the desktop shell now;
implementation waits. Ticket:
`ai-docs/tickets/todo/261112-research-ws-ade-desktop-shell-and-rename.md`.
Fill its Outcome Ledger with evidence; do not change Confirmed Decisions.

## Work

- Topic 1 (desktop shell): compare Tauri 2.x and Electron (and note any
  serious third option) for wrapping the existing dashboard web UI with the
  Go daemon binary as a sidecar. Cover: sidecar support and lifecycle,
  bundle size, WebView differences (WKWebView / WebView2 / WebKitGTK) that
  would hit this dashboard's features (SSE, WebSocket, xterm-style browser
  terminal, container queries, clipboard), auto-update and code signing on
  macOS/Windows, native notifications. Use official docs; cite URLs and
  versions. Read the dashboard's web stack in
  `agents-plugin-tool/internal/wsdashboard/` (assets, server, auth/origin
  handling) to find what the shell must provide (port discovery via the
  daemon's `endpoint.json`, auth token, loopback origin).
- How the app would start or attach to the daemon when a terminal session
  already started one, and how it locates harness CLIs.
- Topic 2 (rename): inventory every place the repo or root directory name is
  used as a key or appears user-visibly. Grep the Go tooling for how cache
  roots, project ids (e.g. `ws@<owner>-<repo>`), worktree pool paths, and
  dashboard/session/whiteboard/tracker paths are derived; list each with
  file:line and whether a rename breaks continuity. Also marketplace files,
  `install.sh`, plugin manifests, docs. Propose a migration approach.

## Output

- Edit only the research ticket: Verified Findings (cited), Proposals
  (labelled non-authoritative), Open Questions. Keep it concise.
- Commit the ticket and this plan with `## AI Context`.
- Return a short summary with the recommendation and the biggest risks.
