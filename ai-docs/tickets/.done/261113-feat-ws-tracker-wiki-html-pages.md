---
title: "Tracker wiki renders HTML pages in a sandboxed frame"
sage-review-design: completed
sage-review-completeness: completed
related:
  261110-feat-ws-dashboard-file-preview: "prior sandboxed HTML frame (allow-scripts, no allow-same-origin) and its finding that a framed page can carry what it reads elsewhere"
  261108-feat-ws-tracker-wiki-git-branch: "the wiki store, the .md page addressing this ticket extends, and the page-read response {path, content, commit, html}"
  261113-feat-ws-dashboard-iframe-shell: "the tracker page becomes a frame inside the shell, so the wiki HTML frame nests one level deeper"
sage-review-design-reviewed: 60521c76fe7ba918
sage-review-completeness-reviewed: 60521c76fe7ba918
completed: 2026-11-14
---

# Tracker wiki renders HTML pages in a sandboxed frame

## Background

Wiki branches are `refs/heads/wiki/<name>`; pages are `.md` files and attachments live at `_attachments/<sha256>.<ext>` (`agents-plugin-tool/internal/wsledger/wiki.go:14`, `:59`). Wiki reads run through `trackerText` and `RenderText` with HTML disabled, escaping raw tags (`agents-plugin-tool/cmd/ws-mcp/tracker_host.go:266`, `:278`, `:577`; `agents-plugin-tool/internal/wswhiteboard/markdown_text.go:43`); `wkPageRead` inserts the rendered HTML into the host DOM (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js#L1966-L1977`, the insertion at `:1977`).

The `.md` suffix is baked into every page operation through `pageFile` (`wiki.go:143`): `ValidWikiPath` refuses any segment ending in `.md` (`:120`), `WikiPages` lists only `*.md` blobs (`:543`), `WikiSearch` greps `*.md` (`:597`), `WikiHistory`/`WikiDiff`/`WikiRead` and every `wikiApply` op address `path + ".md"`, `renameGuard` looks for a moved page only among `.md` files (`:1371`), and `wikiConflict` trims `.md` off the conflict path (`:1407`). `mergeTrees` is extension-agnostic: any `100644` blob both sides changed goes through `git merge-file` (`wiki.go` `mergeTrees`, default case).

`tracker.js` `render()` rebuilds the drawer with `body.innerHTML = drawerHtml(...)` on every draw (`tracker.js:5444`), including draws caused by unrelated store stream events.

## Decisions

- Owner, 2026-11-13, board item (59): option A, standalone HTML pages. Reject B, sanitized HTML inside Markdown: the owner wants real HTML pages with scripts and styles, which sanitizing cannot provide.
- (498)=a (owner, 2026-11-13; premise: there are no downstream users of wiki HTML yet): the path's extension decides the page kind. `guide.html` is an HTML page; `guide.md` (or no extension) is a Markdown page. A Markdown page path ending in `.html` (today stored as `x.html.md`) and a folder whose name ends in `.html` are refused. There is no legacy or compatibility path for `x.html.md` and no rename path. Rejected: (b) extension-less paths plus a kind argument (`guide` + kind html stored as `guide.html`). Rationale: with no downstream users, b's only advantage (keeping today's behaviour) disappears, and a's compatibility burden goes away; the remaining real conflict, blob `a.html` against folder `a.html/` in the git tree, is prevented by refusing `.html` folders. Caveat: a daemon not yet rebuilt (for example the office machine) mishandles HTML pages created meanwhile until it updates. Precisely, from the code: an old daemon's listing skips `.html` blobs (`WikiPages` keeps `*.md` only), its read of `guide.html` looks up `guide.html.md` and answers page-not-found, and the dashboard then opens a `wiki:` link to it as a new page whose save writes `guide.html.md`, a Markdown page an updated daemon refuses to address and skips.
- Create and edit HTML pages through the existing wiki update path (tool and dashboard UI). Render HTML only in a sandboxed `srcdoc` iframe, never inline.
- Path validation and addressing (review 2026-11-13, autonomous; settles completeness #1 under (498)=a). `ValidWikiPath` keeps today's character and segment rules and adds:
  - A folder segment (every segment but the last) ending in `.md` or `.html`, case-insensitively, is refused, so a folder can never share a tree name with a page file.
  - A last segment ending exactly in lowercase `.html`, with a non-empty stem, is an HTML page stored at that path as is.
  - A last segment ending exactly in lowercase `.md`, with a non-empty stem, is a Markdown page; the path is canonicalized by dropping `.md`, so `guide.md` and `guide` address the same file `guide.md`.
  - Any other last segment is a Markdown page name, refused when it ends in `.md` or `.html` case-insensitively (`a.html.md`, `a.md.md`, `a.HTML`, `a.MD`).
  Exact lowercase extensions over case-insensitive kind detection - `a.HTML` beside `a.html` would collide on a case-insensitive checkout, and today's rule already refuses `.MD` case variants. Canonical extension-less Markdown paths over `.md`-suffixed ones in every result (listing, search hits, read and save results, commit subjects, conflicts) - every existing page address and `wiki:<name>/<path>` link stays valid. `ErrWikiBadPath`'s message states the new rule.
- Same-stem coexistence allowed over refused (review 2026-11-13, autonomous): `guide` (file `guide.md`) and `guide.html` may both exist. The extension is part of the address, so the two never resolve ambiguously, and a refusal could not be enforced against two clones creating each concurrently without a new merge-time rule, since the tree merge sees two distinct paths.
- Blobs that are no longer valid pages (review 2026-11-13, autonomous): files such as `x.html.md` or anything under an `a.html/` folder, written before this change or by an old daemon, are skipped by listing and search and cannot be addressed by read or save, over surfacing them under a legacy name - (498)=a rules out a legacy path, and git history still holds them.
- One page-file mapping for every operation (review 2026-11-13, autonomous; design #3, completeness #2). A page path maps to its tree file (`guide.html` -> `guide.html`, `guide`/`guide.md` -> `guide.md`) and a tree file maps back to a page path or to "not a page" through one pair of helpers, over per-call-site `.md` handling - the suffix is baked into eight call sites and one shared mapping keeps them from drifting. Behavior per operation, stated as acceptance criteria in Phase 1:
  - Listing returns Markdown pages extension-less and HTML pages with `.html`, sorted together.
  - Search greps `*.md` and `*.html` outside `_attachments/` and matches HTML pages' raw source lines (tags included), each hit's path the page path.
  - History and diff of `guide.html` follow `guide.html`; those of `guide` follow `guide.md`.
  - Restore puts back the file the page path maps to; its attachment re-add scan runs on either kind unchanged (an HTML page's `attachment:` text is inert, see the attachment decision).
  - Rename keeps the blob and may change kind (`guide` -> `guide.html`), over refusing a cross-kind rename - it converts a page without retyping it, the result still renders only in the sandbox, and refusing needs an extra rule for no safety gain.
  - The rename guard's moved-page search covers every page file of either kind, over `.md` only - otherwise a concurrently renamed HTML page would be resurrected under both names.
  - Merge needs no change: concurrent non-overlapping edits of one HTML page merge line-wise, overlapping ones refuse as a text conflict whose path is the page path (`guide.html`).
- Images with an HTML write refused over accepted (review 2026-11-13, autonomous): a write whose path is an HTML page and that carries `images` is refused as a malformed edit, since such a page cannot show them (attachment decision below); lifting the refusal later is additive.
- Page-read contract (review 2026-11-13, autonomous; completeness #3). The dashboard page read (`GET .../wiki/<name>/page?path=`, `TrackerWikiPage`) gains `kind`, always present, `"markdown"` or `"html"`; `path` is the canonical page path. For a Markdown page `html` is unchanged (the escaped `trackerText` rendering). For an HTML page `html` carries the sandboxed iframe markup the generic builder produces from `content`, over an empty `html` the browser fills from `content` - the host keeps rendering every page (the 261108 contract that the browser never renders twice) and the security-critical sandbox attribute stays in Go, pinned by Go tests, not in browser script. `wiki.query op=read` adds `kind` to its JSON value and names the kind in its text header; `op=pages` stays a path list, since the kind is readable from the path.
- Generic sandboxed builder (review 2026-11-13, autonomous; design #2). A new exported `wswhiteboard` builder takes a title and a whole HTML document and returns `<iframe title=... sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox" srcdoc=...>` with the document escaped into `srcdoc` verbatim: no injected stylesheet, script, `<base>`, or CSP meta. It lives beside `framedDoc`, which keeps its whiteboard specifics (page stylesheet, `doc-frame.js`, relays, mermaid) and reads the same sandbox-attribute constant, over reusing `framedDoc` (whiteboard-specific: topic keys, theme pinning, relay list) and over a copy in `wsdashboard` - one constant cannot drift, and `cmd/ws-mcp/tracker_host.go` already imports `wswhiteboard` for the page read.
- Frame sizing and lifetime (review 2026-11-13, autonomous; design #2). The HTML page frame fills the wiki page area and scrolls inside itself, and lives in a persistent layer outside the subtree `render()` replaces, positioned over a slot element the draw leaves in the page area (as the item window `tkwin` lives outside `body`); it is re-created only when (store, wiki, path, commit) changes. Chosen over an auto-height frame with a `tracker.js` height listener - an iframe reloads whenever it is detached, so with `render()` rebuilding `body.innerHTML` on every draw (`tracker.js:5444`) an inline frame restarts the page's scripts on every unrelated store event; a frame that keeps its height fixed to the page area needs no scroll-synced overlay, needs no script injected into the author's document, and leaves `tracker.js` with no message listener for wiki frames at all.
- Script and network posture inside the frame (review 2026-11-13, autonomous; completeness #5). Scripts run (`allow-scripts`) in an opaque origin (no `allow-same-origin`); popups open as ordinary pages (`allow-popups allow-popups-to-escape-sandbox`, as the whiteboard frame); no `allow-forms`, `allow-top-navigation`, `allow-modals`, or `allow-downloads`. Outbound network is not restricted: no CSP meta, over a CSP restricting `connect-src` and friends - the owner wants real pages, and loading a library from a CDN is the common case; the frame holds only its own page text, which every reader of the wiki can read, and no credential; a CSP meta would have to be spliced into the author's `<head>` (prepending it before a doctype throws the page into quirks mode); and CSP does not govern a frame navigating itself anyway (261110 Result). Accepted residual: a page can tell an outside host that it was opened (viewer address, time).
- Isolation holds without the LAN guard (review 2026-11-13, autonomous). Owner (490) drops LAN serving; the isolation argument does not rely on it. A local viewer's requests pass the guard without any cookie (`localViewer`, `access.go:146`), so the frame's requests to the daemon are not refused for lacking credentials. What keeps the frame away from privileged actions: every write route requires `sameOrigin`, which refuses an `Origin` of `null` (`inbox_routes.go:141`); no route grants CORS to `null` except the capability routes, which need a capability path the frame never receives and refuse fetches (`fs_capability.go` `capRefused`); and the host receives no message from wiki frames.
- Attachments not resolved in HTML pages (review 2026-11-13, autonomous; completeness #6). An HTML page's `attachment:<sha256>` text stays literal, over inlining referenced wiki images into the `srcdoc` as `data:` URLs - author markup and scripts can build a reference in ways a host rewrite cannot find, inlining multiplies the read payload, and topic bodies already set the precedent that references resolve in Markdown but not in HTML (d192e81f). Completion rule: this ticket is complete without attachment resolution; the `wiki.update` description states that an HTML page cannot show pasted images, and the worker's result records the limitation with a forward note naming the candidate mechanism (host-side `data:` inlining bounded like `ImageDataURL`'s 2 MiB) for a follow-up ticket.
- HTML editing in the dashboard (review 2026-11-13, autonomous). An HTML page edits as source in the existing editor with no Preview tab and no image paste or upload, over a render-route preview for the html kind - the render route stays Markdown-only with no contract change, and the saved view is the preview.
- Compatibility: T0. The page kind and the response field `kind` are additive and optional: an older reader of the page read ignores `kind`; older `WikiPages` readers **ignore**, rather than list, `.html` files, and older reads append `.md` (`agents-plugin-tool/internal/wsledger/wiki.go:143`, `:541`, `:572`); no store schema, tracker event, or MCP argument changes, and the wiki's on-disk shape gains only a new file extension. Existing Markdown behavior remains intact. The old-daemon caveat under (498)=a is a cross-version behavior gap, not a protocol break. The tier covers the wire shape only; the narrowing of previously valid paths (`x.html` Markdown names and `.html` folders become unaddressable, existing `x.html.md` blobs are skipped) is owner-approved under (498)=a, not an open tier question.

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
| scope.span | multi-file | agents-plugin-tool/internal/wsledger/wiki.go, cmd/ws-mcp/tracker_host.go, internal/wswhiteboard/render.go, internal/wsdashboard/assets/tracker.js, internal/mcp/server.go (tool descriptions), internal/mcp/wiki_tools.go |
| scope.surface | cross-module | store (wsledger), wiki tools, host page read, dashboard UI, shared sandbox builder in wswhiteboard |
| scope.new_public_symbol | yes | a wsledger page-kind helper and kind constants ("markdown", "html"), and an exported wswhiteboard sandboxed-page builder; names are the worker's (Decisions: page-file mapping, generic builder) |
| scope.new_type_contract | yes | TrackerWikiPage gains `kind` (always present) and `html` carries iframe markup for an HTML page; wiki.query op=read JSON gains `kind`; additive, T0 (Decisions: page-read contract) |
| scope.test_surface | existing | internal/wsledger/wiki_test.go, cmd/ws-mcp/tracker_host_test.go, internal/mcp/wiki_tools_test.go, internal/wsdashboard/tracker_wiki_test.go, internal/wswhiteboard/*_test.go, tracker.js node tests (runTrackerNode) |
| complexity.reuse_points | confirmed | framedDoc's sandbox attribute at internal/wswhiteboard/render.go#L547-L569 becomes a shared constant; mergeTrees needs no change |
| complexity.side_effect_risk | moderate | the .md suffix is baked into path validation, listing, search, and rename/conflict code at wiki.go:143, :543, :597, :1371 (Background's anchors), so a second page kind touches shared paths |
| risk.correctness | moderate | extension-keyed kind must not break the .md listing, search, rename guard, and merge paths; the frame layer must survive render()'s full innerHTML rebuild |
| risk.fit | moderate | a new generic builder beside framedDoc; the frame layer nests inside the planned dashboard iframe shell (261113-feat-ws-dashboard-iframe-shell, ready) |
| risk.test | moderate | frame sandbox attributes are pinned as markup in Go; frame lifetime and isolation are browser behavior checked by node tests and a manual browser check |
| risk.security_or_contract | high | author-controlled scripts with unrestricted egress; isolation rests on no allow-same-origin, sameOrigin-gated writes, no CORS grant to null, and no host message listener for wiki frames; one added sandbox token or listener breaks it; the contract change is additive T0 |

## Phases

### Phase 1: Store, tools, and page-read contract

Implement the path validation, page-file mapping, and per-operation behavior in `wsledger` (Decisions: path validation, coexistence, invalid blobs, page-file mapping, images refusal); add the generic sandboxed builder in `wswhiteboard` with `framedDoc` reading the shared sandbox constant; return `kind` and, for an HTML page, the builder's markup in `html` from the dashboard page read and `kind` from `wiki.query op=read`; update the `wiki.query`/`wiki.update` descriptions (path: Markdown paths without suffix, `.md` accepted as the same page, `.html` naming an HTML page; content: Markdown or a whole HTML document; images: not for an HTML page, which cannot show them).

Acceptance criteria:

- `ValidWikiPath` accepts `a`, `a.md`, `a.html`, `f/a.html`; refuses `a.html.md`, `a.md.md`, `a.HTML`, `a.MD`, `.html`, `a.html/b`, `a.md/b`, `a.HTML/b`; `a.md` and `a` address one page.
- `guide` and `guide.html` coexist; listing returns `guide`, `guide.html`; a pre-existing `x.html.md` blob is absent from listing and search.
- Search finds text in an HTML page's source with hit path `x.html`; history, diff, and restore of `x.html` follow `x.html`.
- Rename `guide` -> `guide.html` succeeds; a delete or rename of an HTML page another clone renamed is refused as a rename conflict.
- Concurrent non-overlapping edits of an HTML page merge; overlapping edits refuse with conflict path `x.html`.
- A write to an HTML page carrying `images` is refused.
- The page read of an HTML page returns `kind: "html"` and `html` holding exactly one iframe whose `sandbox` is `allow-scripts allow-popups allow-popups-to-escape-sandbox` (no `allow-same-origin`) and whose `srcdoc` is the escaped content; a Markdown page returns `kind: "markdown"` and still renders raw tags as escaped text.
- `wiki.query op=read` JSON carries `kind` and its text header names the kind; the `wiki.update` and `wiki.query` descriptions describe HTML pages, including that an HTML page cannot show images or resolve `attachment:` references.
- Existing whiteboard frame tests pass unchanged.

#### Verification

`go test ./...` in `agents-plugin-tool`, with new cases in `internal/wsledger/wiki_test.go`, `cmd/ws-mcp/tracker_host_test.go`, `internal/mcp/wiki_tools_test.go`, and `internal/wswhiteboard` builder tests covering each criterion above.

### Result (b698db898) - 2026-11-14

- `wsledger`: `CanonicalWikiPath` (exported; `ValidWikiPath` wraps it), `WikiPageKind`, `WikiKindMarkdown`/`WikiKindHTML`, and the `pageFile`/`pageOf` pair drive listing, search (`*.md` and `*.html`), read, history, diff, write, rename (cross-kind allowed), delete, restore, the rename guard, and conflict paths. `checkWikiEdit` canonicalizes `Path`/`To`, so subjects and conflicts carry `guide`, not `guide.md`. An HTML write with images is `ErrWikiBadEdit`. `ErrWikiBadPath` states the new rule.
- Review fix (e20ae1cef): a legacy folder `x.html/` is not the page `x.html`: write and restore refuse (`ErrWikiBadEdit`), delete answers page-not-found, instead of replacing or removing the whole folder; the `.html` stem follows the `.md` stem's `.`/`..`/whitespace rules.
- `wswhiteboard`: shared `frameSandbox` constant read by `framedDoc`; new `SandboxedPage(title, doc)` escapes the document into `srcdoc` with nothing injected; its comment states the CSP-inheritance trap.
- Page read: `TrackerWikiPage.Kind` (always set); for an HTML page `html` is the `SandboxedPage` frame and the image lookup is skipped. `wiki.query op=read` adds `kind` (JSON) and `(<kind>)` in its text header; `op=read`/`op=write` results echo the canonical path. Tool descriptions describe HTML pages, the images refusal, and the unresolved `attachment:` text.
- Deviation: the session page (one of the two tracker hosts) sends `frame-src 'self'; frame-ancestors 'none'`, not `frame-ancestors` alone. `srcdoc` frames are not governed by `frame-src`, so author pages still load CDN scripts; they cannot frame another origin. Both hosts' policies are pinned exactly in `TestTrackerPagesPolicyLeavesWikiHTMLFramesFree`.
- Verification: `go test ./internal/wsledger ./internal/wswhiteboard ./internal/wsdashboard` pass, and every wiki test in `./internal/mcp` and `./cmd/ws-mcp` passes (new: `TestWikiPathKinds`, `TestWikiHTMLPages`, `TestWikiRemoteHTMLMergesAndConflicts`, `TestSandboxedPageFrame`, `TestDashboardTrackerWikiHTMLPage`, `TestWikiToolsHTMLPages`, the CSP pin; `TestTrackerWikiRoutes` pins `kind`). The other failures (`internal/wsagent` Pi/Claude settings; `internal/mcp` render root `/private/var` vs `/var` cases; `cmd/ws-mcp` `TestHostedConnectionSeeingALANChangeReplacesTheDaemon`) reproduce on base commit 0b6180fbf and are unrelated; `TestIntakeStopsWithinRecheckOfARemovedKey` failed once under load and passes alone.
- Limitation (Decisions: attachments): an HTML page's `attachment:<sha256>` text stays literal and an HTML page cannot show pasted images.

> Forward: wiki HTML pages could show wiki images through host-side `data:` inlining of referenced `attachment:` images into the `srcdoc`, bounded like `ImageDataURL`'s 2 MiB; a follow-up ticket decides it.

### Phase 2: Dashboard rendering and editing

Render an HTML page's `html` in the persistent frame layer (Decisions: frame sizing and lifetime); keep Markdown pages on the current path. Allow `.html` page paths in the new-page path field; edit an HTML page as source without Preview tab or image paste/upload (Decisions: HTML editing).

Acceptance criteria:

- An HTML page shows in a frame filling the page area, outside the `.md` article, and bypasses `refLinksHtml`.
- A draw that keeps (store, wiki, path, commit) keeps the same frame element attached (no reload); a change of any of the four replaces it.
- While the slot is absent but the (store, wiki, path, commit) key is unchanged (item window open, edit or conflict view, drawer closed), the layer is hidden and the frame survives, so returning does not reload it; a key change (another page, store switched) empties the layer.
- The layer repositions with the slot's geometry on drawer resize and layout changes (pin width, wide layout, iframe-shell resizing), not only on redraw.
- A `wiki:` link to an `x.html` path, from a Markdown page or an item, opens like any wiki path and the kind-dispatching page read renders it in the frame layer.
- `tracker.js` registers no `message` handler for wiki frames.
- Creating `notes/x.html`, saving, and reopening shows the rendered page; the editor for it has no Preview tab and ignores image paste.

#### Verification

`go test ./internal/wsdashboard/...` with `runTrackerNode` cases for kind dispatch, frame-layer reuse across draws, layer hiding, and editor options. Manual browser check on a local viewer: a page script runs; inside the frame `document.cookie` is empty, `parent.document` throws, and a `fetch` POST to a tracker write route is refused (403); the frame survives an unrelated store update without reloading.

### Result (5ad779513) - 2026-11-14

- `tracker.js`: `wkLoadDoc` keeps `kind` (absent counts as markdown, for an older daemon); an HTML page's read view draws an empty slot (`.wk-fslot`, keyed) in place of the article and never passes the markup through `refLinksHtml`. A persistent body-level layer `#tkframe` (beside `tkwin`) holds the host-built frame keyed by (store, wiki, path, commit); `wkFrameSync` after every draw keeps the same iframe element for the same key, hides the layer and keeps the frame while the slot is absent (item window, edit/conflict view, drawer closed, other panel, the page's own menus), and empties it on a key change. It sits `position: fixed` over the slot, clipped to the slot's clipping ancestors, repositioned by ResizeObserver, window resize, and capture-phase scroll; z-index 41 sits above the drawer and below the item window, menus, toasts, and tooltips.
- No `message` listener and no sandbox tokens in `tracker.js` (static test). `wkCanon` drops a `.md` suffix for page-list membership, navigation, and save paths. A new `.html` page seeds a minimal HTML skeleton; the editor for an HTML page has no Write/Preview tabs, no mention popup (`@`/`#` are common in HTML/CSS source), refuses image paste/drop with the existing "images cannot be attached here" notice, and saves with no attachments.
- Verification: `go test ./internal/wsdashboard/...` pass (new `TestTrackerWikiHTMLPageFrame`, `TestTrackerWikiHTMLPageEditor`, `TestTrackerNoFrameMessages`). Browser check in headless Chromium on a harness page loading `tracker.css` and the real frame-layer block from `tracker.js`: the page script ran; `document.cookie` and `parent.document` both throw `SecurityError` (origin `null`); same-key redraws and an editor round trip kept the frame without reload; the layer followed a container resize; a new commit replaced the frame; a POST from the frame carried `Origin: null` and no cookie. Not run: the mounted dashboard against a real daemon (the 403 from `sameOrigin` on `Origin: null` was not exercised live), the drawer's real transition and drag, page mode, the narrow layout, and live z-order against the item window.

## Constraints

HTML must never execute in the dashboard origin: omit `allow-same-origin`; keep the whiteboard's sandbox token set; do not expose host actions through frame messages. The isolation must hold for a local viewer, independent of LAN serving, which owner (490) is dropping (today the dashboard can bind to the LAN with token access, `agents-plugin-tool/internal/wsdashboard/access.go:18`).

- A `srcdoc` frame inherits the tracker page's CSP (its policy container), even when sandboxed. Today the tracker page sends only `frame-ancestors 'none'` (`writeHTMLStatus`, `pages.go:1286`), so author pages load CDN scripts as intended; any later `script-src`, `connect-src`, or `default-src` on the tracker page (including iframe-shell's `?frame=1` headers) would silently break them. Leave a comment at the HTML frame builder site and pin the tracker page's policy in a test (design review 2026-11-13).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/; the tool descriptions and `ErrWikiBadPath` are shipped text)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Sage Review Round 1 (2026-11-13)

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
