---
title: "Tracker wiki renders HTML pages in a sandboxed frame"
---

# Tracker wiki renders HTML pages in a sandboxed frame

## Background

Wiki branches are `refs/heads/wiki/<name>`; pages are `.md` files and attachments live at `_attachments/<sha256>.<ext>` (`agents-plugin-tool/internal/wsledger/wiki.go:14`, `:59`). Wiki reads run through `trackerText` and `RenderText` with HTML disabled, escaping raw tags (`agents-plugin-tool/cmd/ws-mcp/tracker_host.go:266`, `:278`, `:577`; `agents-plugin-tool/internal/wswhiteboard/markdown_text.go:43`); `wkPageRead` inserts the rendered HTML into the host DOM (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js:1961`).

## Decisions

- Owner, 2026-11-13, board item (59): option A, standalone HTML pages. Reject B, sanitized HTML inside Markdown: the owner wants real HTML pages with scripts and styles, which sanitizing cannot provide.
- Decide page kind by the `.html` extension. Create and edit through the existing wiki update path (tool and UI), accepting `.html` names. Render HTML only in a sandboxed `srcdoc` iframe, never inline.
- Compatibility: T0, a new optional page kind. Correction to the proposed rationale: older `WikiPages` readers **ignore**, rather than list, unknown `.html` files; reads append `.md` (`agents-plugin-tool/internal/wsledger/wiki.go:143`, `:541`, `:572`). Existing Markdown behavior must remain intact.

## Phases

### Phase 1: Store and render HTML pages

Extend the store and wiki update tool to create and read `.html` pages; expose creation and editing in the tracker UI and render them in the isolated frame. Reuse the whiteboard frame and height-sync pattern (`agents-plugin-tool/internal/wswhiteboard/render.go:476`, `:503`; `agents-plugin-tool/internal/wswhiteboard/assets/doc-frame.js:1`). Add tests for create/read, sandboxed rendering, and continued escaping of HTML in `.md` pages.

## Constraints

The dashboard can bind to the LAN with token access (`agents-plugin-tool/internal/wsdashboard/access.go:18`). HTML must never execute in the dashboard origin: omit `allow-same-origin`; preserve the whiteboard's sandbox isolation. Do not expose privileged host actions through frame messages.

## Open

Make existing wiki attachment references resolve from HTML pages if feasible without weakening isolation; otherwise record the limitation and follow-up. Sandboxed frames have an opaque origin and cannot rely on the viewer's cookie (`agents-plugin-tool/internal/wsdashboard/access.go:28`).
