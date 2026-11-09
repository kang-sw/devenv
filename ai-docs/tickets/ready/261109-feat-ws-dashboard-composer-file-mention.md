---
title: "ws dashboard composer: @ file mention with a fuzzy popup (root search and absolute-path browsing)"
related:
  261107-epic-ws-dashboard-messenger-round2: the composer this extends
  261109-feat-ws-dashboard-open-out-of-root-images: the loopback-only rule for reaching outside the root, reused here
  261109-feat-ws-tracker-shared-markdown-editor: a later consumer of the reusable popup module
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f158aae35f54195b
sage-review-completeness-reviewed: f158aae35f54195b
---

# ws dashboard composer: @ file mention with a fuzzy popup (root search and absolute-path browsing)

## Background

The user (2026-11-09) wants to reference files from the dashboard
conversation composer by typing `@`: a context popup opens above the caret
with fuzzy matches, Up/Down navigate, Tab inserts. Two modes: search from the
session root, or browse by absolute path.

Today the composer has no popup or autocomplete; its keydown handles only the
IME guard, Tab indent, Esc (arms Tab-out), and Enter/Ctrl+Enter send
(`assets/dashboard.js`, the `if (ctext)` composer wiring; `sendMessage` posts
the raw text to `/s/<rid>/message`). The daemon has no file listing or search
route (`wsdashboard/pages.go` `handleSessionRoutes`); `git ls-files` is used
only inside the MCP `git.search` tool, and there is no fuzzy matcher in the
Go tree. The closest precedent is the tracker comment box's `@`/`#` mention
popup (`assets/tracker.js` `mentionToken`, `mentionSync`, `mentionDraw`,
`mentionAnchor`, `mentionPlace`, `mentionPick`), which is bound to `#cbox`
and `#tkmpop` and so cannot be reused as is. Only loopback viewers
(`localViewer`, `wsdashboard/access.go`) may reach outside the root (image
view and system open).

## Decisions

- **Interaction** (user; board (23) ok): typing `@` opens a popup above the
  composer at the caret with fuzzy-matched candidates; Up/Down move, Tab
  inserts. Two modes: root-relative search and absolute-path browsing.
- **Absolute browsing is loopback only** (board (24) ok): listing a directory
  outside the root is served only to a loopback viewer (`localViewer`,
  enforced on the server, not just hidden in the page); a LAN viewer gets
  root search only. Follows the out-of-root images ticket's rule.
- **Root search route** (board (25) ok): a new GET route
  `/s/<rid>/files?q=<query>` answers root mode. The file list comes from
  `git ls-files --cached --others --exclude-standard` (so `.gitignore`
  applies and build output or `node_modules` do not flood the results),
  cached briefly; the server ranks matches fuzzily and returns only the top
  50. A non-git root falls back to a `WalkDir` with depth and count caps.
  Rejected: sending the whole list to the browser (heavy in large
  repositories).
- **Absolute mode completes one directory at a time** (board (26) ok, (30)
  ok): fuzzy matching runs only inside the directory being typed, like shell
  path completion; no recursive search under an absolute path (unbounded
  cost on something like `D:/`). Choosing a directory with Tab inserts
  `dir/` and keeps the popup open on that directory's contents; choosing a
  file with Tab completes it and closes the popup.
- **Keys while the popup is open** (board (27) ok): Enter also picks (never
  sends); Esc closes only the popup (it arms Tab-out only when no popup is
  open); `@` opens the popup only at line start or right after whitespace
  (an email address does not). The IME guard stays first.
- **Inserted text and sending** (board (28) ok, refined by (31) ok): the
  composer keeps a completed mention as `@<path>` text, root-relative inside
  the root and absolute outside it. A highlight layer behind the textarea
  draws each completed mention as a pill. When the caret sits at the end of
  a mention and the user keeps typing, the popup reopens with that mention
  as its query. On send, the `@` is stripped and only the path is sent, so
  the agent receives a plain path and the sent bubble renders it as a path
  link. Rejected: a contenteditable editor (IME, paste, undo, and the
  `[Image #n]` marker sync would all have to be rebuilt).
- **Reusable module** (board (29) ok): the popup is a standalone module that
  takes the input element (and a candidate source) as arguments, so the
  support-chat composer and the tracker editor can reuse it; it does not
  bind fixed element IDs the way the tracker's mention popup does.
- **Route contract** (completeness review, auto-confirmed): the route picks
  absolute mode when `q` starts with `/` or a drive letter and colon
  (`C:`, either slash after it), root mode otherwise, and answers
  `{"mode": "root"|"abs", "dir": "<listed directory, abs mode only>",
  "items": [{"path": "...", "dir": true|false}]}`, `path` root-relative in
  root mode and absolute in abs mode, at most 50 items. The root file list is
  cached per root for 10 seconds; `git ls-files` runs under a 5-second
  timeout; the non-git `WalkDir` stops at depth 8 or 20,000 entries,
  whichever comes first, and skips dot-directories. Rejected: an explicit
  `mode=` parameter (the typed text already says which mode the user means).
- **Paths with whitespace** (design review, auto-confirmed): a completed
  mention whose path contains whitespace is kept in the composer as
  `` @`<path>` `` and sent as `` `<path>` ``, so the agent reads one
  unambiguous path; token detection, reopen-on-continue, and the pill layer
  treat the backticked form as one token; a path that itself contains a
  backtick is wrapped in a longer backtick run, as markdown code spans
  are (delta design review, auto-confirmed). The sent bubble links a mention
  only where the existing path renderer already does (a code span or bare
  text matching `barePath`, commit 2477a110d); paths with spaces and
  single-segment root-relative files (`README.md`) render as code text,
  and widening `barePath` is out of scope. Rejected: hiding candidates with
  spaces (common on Windows drive paths, the reason absolute browsing
  exists).
- **What a completed mention is** (design review, auto-confirmed): a range
  the composer recorded when a popup pick inserted it; typing inside it
  drops the record, and typed text such as `@mary` is never a mention and
  keeps its `@` on send. A restored draft restores its ranges only when the
  draft store keeps them; otherwise its text is sent as typed. Rejected:
  treating any `@token` that resolves to a path as a mention (rewrites text
  the user never picked).
- **Fuzzy scorer** (board (40) ok): ranking is a small Go subsequence scorer
  in `wsdashboard` (basename, consecutive-run, and path-boundary bonuses).
  Rejected: a third-party fuzzy library (no matcher exists in the tree and a
  scorer this size needs no new dependency).
- **Tracker popup stays** (board (61) ok): the tracker comment box keeps its
  own mention popup; migrating it to the new module is out of scope here.
  Rejected: migrating it in this ticket (keeps this ticket and the parallel
  tracker editor ticket independent).
- **Standalone ticket** (board (37) ok): standalone with `related:` links.
  Rejected: a child of `261107-epic-ws-dashboard-messenger-round2` (that
  epic awaits closure after the user's live check).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- The new route follows the existing route conventions: containment checks
  like the file route for root mode, every refusal a plain 404, absolute
  mode refused for a non-loopback viewer, and Windows UNC and device paths
  (`\\server\...`, `\\?\...`, `\\.\...`, `\??\...`) refused before
  the filesystem is touched, as the out-of-root image route does (8295d5c83).
- The `[Image #n]` marker behavior, input-mode send keys, and Tab indent stay
  as they are when no popup is open.
- Migrating the tracker comment box to the new module is out of scope here.

## Prior Decisions

- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, Decisions): "Out-of-root images are served only to a loopback viewer (`localViewer`). A LAN viewer, token or not, gets the route's ordinary 404, so the route says nothing about which paths exist." — bearing: supports
- 261109-feat-ws-dashboard-open-out-of-root-images (2026-11-09, Decisions): "Containment for an out-of-root image: the path must be absolute ...; Windows UNC and device paths (`\\server\...`, `\\?\...`, `\\.\...`) are refused before the filesystem is touched" — bearing: constrains
- 2c1ae57e3 (2026-11-09, commit): "Route: `handleImage` falls back to `outsideImage` only for `localViewer` and when `Sec-Fetch-Site` is absent or `same-origin`. Such a response carries `Cross-Origin-Resource-Policy: same-origin`." — bearing: constrains
- 8295d5c83 (2026-11-09, commit): "review Critical (\??\UNC namespace bypass) fixed in 8295d5c83" (refuse NT-namespace and non-drive volumes for out-of-root paths) — bearing: constrains
- c779de58 (2026-11-08, commit): "Anchored at the @ rather than the moving caret (mirror-div measurement), and placed above it when it fits ...; Rows pick on mousedown (preventDefault) so the textarea keeps focus and caret." — bearing: supports
- c779de58 (2026-11-08, commit): "The keydown guard returns early on isComposing or keyCode 229, so an IME-commit Enter neither picks a row nor sends (Safari fires keyCode 229 after compositionend)." — bearing: constrains
- 261109-feat-ws-tracker-shared-markdown-editor (2026-11-09, Decisions): "the editor keeps the tracker's existing `@`/`#` mention popup ... Rejected: adopting the composer file-mention ticket's new module now (keeps the two parallel tickets independent)." — bearing: supports
- 261107-feat-ws-dashboard-composer-image-attach (2026-11-07, Decisions): "Thumbnails ... through a new `GET /s/<rid>/attachment/<name>` route with the image route's access rules (loopback, or the LAN token in LAN mode), the name confined to that session's attachments dir." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/pages.go, a new files route file, assets/dashboard.js, assets/dashboard.css, every internal/wsi18n/tables/*.json table |
| scope.surface | public-interface | new HTTP route GET /s/<rid>/files served by the dashboard daemon, a new reusable JS popup module |
| scope.new_public_symbol | yes | /s/<rid>/files route and its handler, the reusable popup module entry point |
| scope.new_type_contract | yes | JSON response shape of the files route for root and absolute modes, popup module arguments input element plus candidate source |
| scope.test_surface | new-files | no files route test exists; existing neighbors internal/wsdashboard/file_route_test.go, access_test.go, i18n_test.go |
| complexity.reuse_points | confirmed | localViewer in wsdashboard/access.go#L136-L138, openContained containment in file_route.go, tracker.js mentionAnchor/mentionPlace as precedent only |
| complexity.side_effect_risk | moderate | composer keydown handler gains popup-open branches beside IME guard, Tab indent, Esc tab-out, and Enter send; sendMessage rewrites text |
| risk.correctness | moderate | fuzzy ranking, git ls-files caching, non-git WalkDir caps, and key routing while the popup is open have many edge cases with no JS test harness |
| risk.fit | moderate | new module must not bind fixed IDs and must coexist with the [Image #n] marker sync and the textarea highlight layer |
| risk.test | moderate | server side is testable in Go but the popup, pill layer, and key paths are verified only by reasoning and hand check |
| risk.security_or_contract | high | the route lists directories outside the session root; loopback enforcement, containment, and UNC/device-path refusal must hold server-side |

## Phases

### Phase 1: files route, popup module, composer wiring

- Server: the `files` route (root mode: cached file list plus fuzzy ranking,
  top 50; absolute mode: one directory listing filtered by the typed
  basename, loopback only), with tests for ranking, `.gitignore` exclusion,
  the non-git fallback caps, containment, the loopback refusal, mode
  detection, and the UNC/device-path refusal.
- Page: the popup module (token detection for paths, anchor and placement,
  ARIA listbox wiring, key and mouse handling, directory continuation),
  the composer's highlight layer for `@path` pills (backticked form for
  paths with whitespace), recorded mention ranges, reopen-on-continue, and
  stripping `@` from recorded mentions only in `sendMessage`. Labels go through the dashboard i18n
  tables.
- Verification: `go test ./internal/wsdashboard/`; `node --check` on edited
  JS; i18n table test green. There is no JS test harness, so the Result
  reasons through the key paths (popup open: Enter, Tab on file and on
  directory, Esc; popup closed: unchanged behavior) and lists the browser
  hand check as unverified if not done.
