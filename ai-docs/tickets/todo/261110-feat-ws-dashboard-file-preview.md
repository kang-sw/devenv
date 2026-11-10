---
title: "Dashboard agent host: a read-only file browser with image, video, and sandboxed HTML preview"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope (no file explorer) is partly reopened here, for browsing and preview only"
---

# Dashboard file preview

## Background

Track B of the epic: let the user see what an agent produced without leaving
the dashboard: browse a folder and preview HTML, images, and video
(2026-11-10 whiteboard, (01), (04)).

## Decisions

1. **Browse and preview only; editing is out of scope entirely (user,
   (04) a and note).** A file tree, and previews of HTML, images, and video,
   plus Markdown and diffs where useful, and an "open in external editor"
   action. Rejected: light single-file editing ((04) b); a full editor
   ((04) c). The 2026-07-30 drop for Orca already judged the editor side as
   territory others do better (`260730-research-ws-dashboard-drop-for-orca`).
2. **HTML and SVG render in isolation (technical, (07)).** HTML from a
   workspace renders on an origin separate from the dashboard, or in a
   sandboxed iframe without `allow-same-origin`, so it cannot reach the
   dashboard token or API; dashboard access already equals execution
   authority ((05)). SVG can carry script, so it is treated like HTML.
   Images and video are served statically from the dashboard origin.

## Constraints

- Independent of the session holder and of daemon lead sessions; it can
  land at any time.
- The browsing root comes from the workspace model
  (`261110-feat-ws-dashboard-workspaces`) once it exists; before then, a
  folder the user picks or the session's working directory.
- Reads stay inside the chosen root: no path traversal out of it.
- To settle at promotion: where the browser lives in the dashboard; size
  limits and streaming for video; which other text types get a preview;
  whether LAN viewers may browse (read-only) or only trusted origins.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`.

## Phases

### Phase 1: tree and previews

Scope and verification are settled at promotion, after the Constraints'
open items.
