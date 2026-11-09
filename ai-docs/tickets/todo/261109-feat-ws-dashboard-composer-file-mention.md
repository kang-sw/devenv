---
title: "ws dashboard composer: @ file mention with a fuzzy popup (root search and absolute-path browsing)"
related:
  261107-epic-ws-dashboard-messenger-round2: the composer this extends
  261109-feat-ws-dashboard-open-out-of-root-images: the loopback-only rule for reaching outside the root, reused here
  261109-feat-ws-tracker-shared-markdown-editor: a later consumer of the reusable popup module
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
  mode refused for a non-loopback viewer.
- The `[Image #n]` marker behavior, input-mode send keys, and Tab indent stay
  as they are when no popup is open.
- Migrating the tracker comment box to the new module is out of scope here.

## Phases

### Phase 1: files route, popup module, composer wiring

- Server: the `files` route (root mode: cached file list plus fuzzy ranking,
  top 50; absolute mode: one directory listing filtered by the typed
  basename, loopback only), with tests for ranking, `.gitignore` exclusion,
  the non-git fallback caps, containment, and the loopback refusal.
- Page: the popup module (token detection for paths, anchor and placement,
  ARIA listbox wiring, key and mouse handling, directory continuation),
  the composer's highlight layer for `@path` pills, reopen-on-continue, and
  stripping `@` in `sendMessage`. Labels go through the dashboard i18n
  tables.
- Verification: `go test ./internal/wsdashboard/`; `node --check` on edited
  JS; i18n table test green. There is no JS test harness, so the Result
  reasons through the key paths (popup open: Enter, Tab on file and on
  directory, Esc; popup closed: unchanged behavior) and lists the browser
  hand check as unverified if not done.
