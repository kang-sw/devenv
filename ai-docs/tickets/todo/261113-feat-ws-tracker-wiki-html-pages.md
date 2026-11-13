---
title: "Tracker wiki renders HTML pages in a sandboxed frame"
sage-review-design: blocked
sage-review-completeness: blocked
---

# Tracker wiki renders HTML pages in a sandboxed frame

## Background

Wiki branches are `refs/heads/wiki/<name>`; pages are `.md` files and attachments live at `_attachments/<sha256>.<ext>` (`agents-plugin-tool/internal/wsledger/wiki.go:14`, `:59`). Wiki reads run through `trackerText` and `RenderText` with HTML disabled, escaping raw tags (`agents-plugin-tool/cmd/ws-mcp/tracker_host.go:266`, `:278`, `:577`; `agents-plugin-tool/internal/wswhiteboard/markdown_text.go:43`); `wkPageRead` inserts the rendered HTML into the host DOM (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L1966-L1977`, the insertion at `:1977`).

## Decisions

- Owner, 2026-11-13, board item (59): option A, standalone HTML pages. Reject B, sanitized HTML inside Markdown: the owner wants real HTML pages with scripts and styles, which sanitizing cannot provide.
- Decide page kind by the `.html` extension. Create and edit through the existing wiki update path (tool and UI), accepting `.html` names. Render HTML only in a sandboxed `srcdoc` iframe, never inline.
- Compatibility: T0, a new optional page kind. Correction to the proposed rationale: older `WikiPages` readers **ignore**, rather than list, unknown `.html` files; reads append `.md` (`agents-plugin-tool/internal/wsledger/wiki.go:143`, `:541`, `:572`). Existing Markdown behavior must remain intact.

## Prior Decisions

- 261110-feat-ws-dashboard-file-preview (2026-11-10, Decisions): "HTML and SVG load in an iframe with `sandbox="allow-scripts"` and no `allow-same-origin`, from a preview route whose path carries a short-lived, root-scoped read capability" — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-11, Result): "A previewed HTML file can read any file under the session root through its capability, and CSP does not govern a frame navigating itself, so such a page could carry what it read to another origin." — bearing: constrains
- 261107-feat-ws-whiteboard-xml-commands-topic-content (2026-11-07, Decisions): "Topic bodies render in the sandboxed frame the Now slot used: an opaque-origin frame that sends no cookie, one frame per body" — bearing: supports
- d192e81f (2026-11-07, commit): "topic-body tests: raw HTML written as is, references resolve in markdown but not in HTML, sandboxed frame trust, mermaid in a frame not on the host page" — bearing: supports
- 69d028fe (2026-11-12, commit): "A sandboxed frame satisfies localViewer: capabilityURL returns a bare path, so the frame and its sub-resources resolve against the session page's origin" — bearing: constrains
- 261108-feat-ws-tracker-wiki-git-branch (2026-11-09, Result): "`.../page?path=` returns `{path, content, commit, html}` (the host renders the page in wiki mode so the browser never renders twice)" — bearing: constrains
- 260525-feat-ws-dashboard-markdown-renderer-polish (2026-05-25, Decisions): "Keep raw HTML disabled or inert; this polish does not reopen the deferred sanitized/sandboxed HTML decision." — bearing: supports
- 261113-feat-ws-dashboard-iframe-shell (2026-11-13, Decisions): "(419) All dashboard pages (tracker, wiki, ...) open as frames inside the shell, so the sidebar never re-renders on any navigation." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsledger/wiki.go, cmd/ws-mcp/tracker_host.go, internal/wswhiteboard/render.go, internal/wsdashboard/assets/tracker.js |
| scope.surface | cross-module | store (wsledger), wiki update tool, host page read, dashboard UI, whiteboard frame reuse |
| scope.new_public_symbol | unknown | page-kind helpers or exported constants are not named; wiki.go pageFile/pageParts are unexported and append .md at wiki.go#L143-L144 |
| scope.new_type_contract | unknown | whether TrackerWikiPage or the page-read response gains a kind field is not stated |
| scope.test_surface | existing | internal/wstrackerhost/wiki_test.go, cmd/ws-mcp/tracker_host_test.go, internal/mcp/wiki_tools_test.go, internal/wswhiteboard/*_test.go |
| complexity.reuse_points | confirmed | framedDoc sandboxed srcdoc iframe at internal/wswhiteboard/render.go#L547-L569 and assets/doc-frame.js |
| complexity.side_effect_risk | moderate | the .md suffix is baked into path validation, listing, search, and rename/conflict code at wiki.go#L134, #L559, #L632, #L1391, so a second page kind touches shared paths |
| risk.correctness | moderate | extension-keyed kind must not break the .md-only listing, search, and merge paths |
| risk.fit | moderate | framedDoc and doc-frame.js are whiteboard-specific and nest inside the planned dashboard iframe shell (261113-feat-ws-dashboard-iframe-shell, todo) |
| risk.test | moderate | frame sandbox attributes and height-sync are browser behavior that Go tests only check as markup |
| risk.security_or_contract | high | author-controlled scripts on a LAN-bindable dashboard; one added allow-same-origin or privileged frame message breaks isolation |

## Phases

### Phase 1: Store and render HTML pages

Extend the store and wiki update tool to create and read `.html` pages; expose creation and editing in the tracker UI and render them in the isolated frame. Reuse the whiteboard frame and height-sync pattern (`agents-plugin-tool/internal/wswhiteboard/render.go:476`, `:503`; `agents-plugin-tool/internal/wswhiteboard/assets/doc-frame.js:1`). Add tests for create/read, sandboxed rendering, and continued escaping of HTML in `.md` pages.

## Constraints

The dashboard can bind to the LAN with token access (`agents-plugin-tool/internal/wsdashboard/access.go:18`). HTML must never execute in the dashboard origin: omit `allow-same-origin`; preserve the whiteboard's sandbox isolation. Do not expose privileged host actions through frame messages.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Open

Make existing wiki attachment references resolve from HTML pages if feasible without weakening isolation; otherwise record the limitation and follow-up. Sandboxed frames have an opaque origin and cannot rely on the viewer's cookie (`agents-plugin-tool/internal/wsdashboard/access.go:28`).

## Blocked (2026-11-13)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The .html page kind collides with Markdown page paths that are valid today (ValidWikiPath accepts x.html -> stored x.html.md; folder a.html/ vs blob a.html; older daemons) | important | missing |
| 2 | The frame helper framedDoc is whiteboard-specific; a generic srcdoc builder plus a tracker.js height listener is needed | minor | autonomous |
| 3 | Unstated behavior for search, rename guard, history, diff, restore and listing that key on .md | minor | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Page addressing and .md/.html coexistence unspecified | important |
| 2 | Listing, search and merge behavior for .html pages not stated as acceptance criteria | important |
| 3 | Page-read response contract for the html kind unspecified | important |
| 4 | Phase 1 combines store, tool, UI editing and rendering with no ordered completion boundary | minor |
| 5 | Script network and egress posture inside the frame is only implied | minor |
| 6 | Open item on attachment references has no completion rule | minor |
